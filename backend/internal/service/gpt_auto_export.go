package service

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"math"
	"strconv"
	"strings"
	"sync"
	"time"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/shopspring/decimal"
)

// GPT-Auto routing log export (CSV / JSONL).
//
// Exports stream straight from the database to the HTTP response with keyset
// pagination over (created_at DESC, id DESC): memory stays bounded by one
// batch regardless of the result size, the order is stable, and a cancelled
// download stops the query loop. The financial figures come from usage_logs
// (joined on request_id + api_key_id), never from a second ledger.
//
// Two schemas exist and never share a code path for row building:
//   - admin: every decision field, internal economics and, in JSONL, Jev
//     probabilities and content-free context diagnostics;
//   - customer: the caller's own live gpt-auto requests only, with their own
//     charge and no procurement cost, profit, upstream account, internal
//     failure classes or diagnostics.

// GPTAutoExportSchemaVersion is written into every exported row.
//
// v2 (Phase 2.1) appends the pricing columns after every v1 column; v1
// columns keep their names, meaning and order, so a v1 reader that selects
// columns by name keeps working.
const GPTAutoExportSchemaVersion = "gpt-auto-export-v2"

const (
	GPTAutoExportFormatCSV   = "csv"
	GPTAutoExportFormatJSONL = "jsonl"

	GPTAutoExportScopeAdmin    = "admin"
	GPTAutoExportScopeCustomer = "customer"
)

const (
	gptAutoExportBatchSize        = 1000
	gptAutoExportAdminMaxRows     = 1_000_000
	gptAutoExportCustomerMaxRows  = 100_000
	gptAutoExportAdminMaxWindow   = 366 * 24 * time.Hour
	gptAutoExportCustomerMaxWin   = 93 * 24 * time.Hour
	gptAutoExportMaxDuration      = 10 * time.Minute
	gptAutoExportMaxConcurrent    = 4
	gptAutoExportUsageMatchWindow = 24 * time.Hour
)

// GPTAutoExportFilter selects rows. UserID is set only by the server from the
// authenticated identity (customer scope).
type GPTAutoExportFilter struct {
	StartTime        time.Time `json:"start_time"`
	EndTime          time.Time `json:"end_time"`
	Mode             string    `json:"mode,omitempty"`
	GroupID          *int64    `json:"group_id,omitempty"`
	RequestedModel   string    `json:"requested_model,omitempty"`
	JevModel         string    `json:"jev_model,omitempty"`
	FinalModel       string    `json:"final_model,omitempty"`
	FinalEffort      string    `json:"final_effort,omitempty"`
	Category         string    `json:"task_category,omitempty"`
	ComplexityMin    *float64  `json:"complexity_min,omitempty"`
	ComplexityMax    *float64  `json:"complexity_max,omitempty"`
	ConfidenceMin    *float64  `json:"confidence_min,omitempty"`
	ConfidenceMax    *float64  `json:"confidence_max,omitempty"`
	RequestStatus    string    `json:"request_status,omitempty"`
	EvaluationStatus string    `json:"evaluation_status,omitempty"`
	FallbackOnly     bool      `json:"fallback_only,omitempty"`
	UpstreamAccount  *int64    `json:"upstream_account_id,omitempty"`
	ClassifierSchema string    `json:"classifier_schema,omitempty"`
	StateVersion     string    `json:"state_version,omitempty"`
	PolicyVersion    string    `json:"policy_version,omitempty"`
	UserID           *int64    `json:"user_id,omitempty"`
}

// GPTAutoExportUsageKey identifies a usage_logs row (unique index).
type GPTAutoExportUsageKey struct {
	RequestID string
	APIKeyID  int64
}

// GPTAutoExportUsage is the usage-log data an export row needs.
type GPTAutoExportUsage struct {
	GPTAutoActualUsage
	CacheCreationTokens int64
	ChannelID           *int64
	RateMultiplier      float64
}

// GPTAutoExportAudit is one audit record (no exported data).
type GPTAutoExportAudit struct {
	ActorID    int64
	Scope      string
	Format     string
	Filters    GPTAutoExportFilter
	RowCount   int64
	Truncated  bool
	Status     string
	DurationMS int
}

// GPTAutoExportRepository is implemented by the PostgreSQL repository. It is
// separate from GPTAutoShadowRepository so lightweight fakes stay valid.
type GPTAutoExportRepository interface {
	// ExportDecisions returns up to limit rows strictly after the cursor in
	// (created_at DESC, id DESC) order, with features and classification.
	ExportDecisions(ctx context.Context, filter GPTAutoExportFilter, cursorTime *time.Time, cursorID int64, limit int) ([]GPTAutoShadowDecision, error)
	// CountDecisions counts matching rows, stopping at limit.
	CountDecisions(ctx context.Context, filter GPTAutoExportFilter, limit int64) (int64, error)
	ExportUsage(ctx context.Context, keys []GPTAutoExportUsageKey, from, to time.Time) (map[GPTAutoExportUsageKey]*GPTAutoExportUsage, error)
	InsertExportAudit(ctx context.Context, audit GPTAutoExportAudit) error
}

// GPTAutoExportRequest is a validated export request.
type GPTAutoExportRequest struct {
	Scope   string
	Format  string
	ActorID int64
	Filter  GPTAutoExportFilter
	// Flush is called after each batch (http.Flusher).
	Flush func()
}

// GPTAutoExportSummary describes a finished export.
type GPTAutoExportSummary struct {
	Rows      int64 `json:"rows"`
	Truncated bool  `json:"truncated"`
	MaxRows   int64 `json:"max_rows"`
}

var (
	errGPTAutoExportUnavailable = infraerrors.ServiceUnavailable("GPT_AUTO_EXPORT_UNAVAILABLE", "export is not available")
	errGPTAutoExportBusy        = infraerrors.TooManyRequests("GPT_AUTO_EXPORT_BUSY", "another export is running; retry when it finishes")
	errGPTAutoExportDisabled    = infraerrors.Forbidden("GPT_AUTO_EXPORT_DISABLED", "GPT Auto routing log export is disabled")
)

// gptAutoExportLimiter bounds concurrent exports globally and per actor, so
// exports can never starve the gateway's database pool.
type gptAutoExportLimiter struct {
	mu     sync.Mutex
	total  int
	actors map[string]struct{}
}

var gptAutoExports = &gptAutoExportLimiter{actors: map[string]struct{}{}}

func (l *gptAutoExportLimiter) acquire(scope string, actor int64) (func(), bool) {
	key := scope + ":" + strconv.FormatInt(actor, 10)
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.total >= gptAutoExportMaxConcurrent {
		return nil, false
	}
	if _, busy := l.actors[key]; busy {
		return nil, false
	}
	l.total++
	l.actors[key] = struct{}{}
	return func() {
		l.mu.Lock()
		l.total--
		delete(l.actors, key)
		l.mu.Unlock()
	}, true
}

func (s *GPTAutoShadowService) exportRepo() (GPTAutoExportRepository, bool) {
	if s == nil || s.repo == nil {
		return nil, false
	}
	repo, ok := s.repo.(GPTAutoExportRepository)
	return repo, ok
}

// ValidateGPTAutoExportFilter checks the window and enum filters for a scope.
func ValidateGPTAutoExportFilter(scope string, filter *GPTAutoExportFilter) error {
	bad := func(msg string) error { return infraerrors.BadRequest("GPT_AUTO_EXPORT_INVALID", msg) }
	if filter.StartTime.IsZero() || filter.EndTime.IsZero() || !filter.StartTime.Before(filter.EndTime) {
		return bad("start_time must be before end_time")
	}
	window := gptAutoExportAdminMaxWindow
	if scope == GPTAutoExportScopeCustomer {
		window = gptAutoExportCustomerMaxWin
	}
	if filter.EndTime.Sub(filter.StartTime) > window {
		return bad("time range is too long (max " + strconv.Itoa(int(window.Hours()/24)) + " days)")
	}
	switch filter.Mode {
	case "", GPTAutoModeShadow, GPTAutoModeLive:
	default:
		return bad("mode must be shadow, live or empty")
	}
	switch filter.RequestStatus {
	case "", GPTAutoRequestSucceeded, GPTAutoRequestFailed, GPTAutoRequestRejected, GPTAutoRequestCancelled:
	default:
		return bad("invalid request_status")
	}
	switch filter.EvaluationStatus {
	case "", GPTAutoEvaluationSuccess, GPTAutoEvaluationFailed, GPTAutoEvaluationSkipped:
	default:
		return bad("invalid evaluation_status")
	}
	if filter.FinalEffort != "" && !IsGPTAutoEffort(filter.FinalEffort) {
		return bad("invalid final_effort")
	}
	for _, value := range []string{filter.RequestedModel, filter.JevModel, filter.FinalModel, filter.Category,
		filter.ClassifierSchema, filter.StateVersion, filter.PolicyVersion} {
		if len(value) > 128 {
			return bad("filter value too long")
		}
	}
	for _, pair := range [][2]*float64{{filter.ComplexityMin, filter.ComplexityMax}, {filter.ConfidenceMin, filter.ConfidenceMax}} {
		for _, value := range pair {
			if value != nil && (math.IsNaN(*value) || math.IsInf(*value, 0)) {
				return bad("invalid numeric range")
			}
		}
		if pair[0] != nil && pair[1] != nil && *pair[0] > *pair[1] {
			return bad("range minimum exceeds maximum")
		}
	}
	if scope == GPTAutoExportScopeCustomer {
		if filter.UserID == nil || *filter.UserID <= 0 {
			return bad("customer export requires an authenticated user")
		}
		filter.Mode = GPTAutoModeLive
	}
	return nil
}

// CustomerExportEnabled reports the admin switch for customer self-service.
func (s *GPTAutoShadowService) CustomerExportEnabled(ctx context.Context) bool {
	if s == nil || s.settings == nil {
		return false
	}
	return s.settings.GetGPTAutoShadowConfigCached(ctx).Live.CustomerExportEnabled
}

// CountExport previews how many rows an export would contain (capped at
// the scope's row limit plus one, so truncation can be reported).
func (s *GPTAutoShadowService) CountExport(ctx context.Context, scope string, filter GPTAutoExportFilter) (GPTAutoExportSummary, error) {
	repo, ok := s.exportRepo()
	if !ok {
		return GPTAutoExportSummary{}, errGPTAutoExportUnavailable
	}
	if scope == GPTAutoExportScopeCustomer && !s.CustomerExportEnabled(ctx) {
		return GPTAutoExportSummary{}, errGPTAutoExportDisabled
	}
	if err := ValidateGPTAutoExportFilter(scope, &filter); err != nil {
		return GPTAutoExportSummary{}, err
	}
	maxRows := gptAutoExportMaxRows(scope)
	count, err := repo.CountDecisions(ctx, filter, maxRows+1)
	if err != nil {
		return GPTAutoExportSummary{}, err
	}
	summary := GPTAutoExportSummary{Rows: count, MaxRows: maxRows}
	if count > maxRows {
		summary.Rows, summary.Truncated = maxRows, true
	}
	return summary, nil
}

// gptAutoExportCustomerRowCap is the effective customer row limit (a
// variable only so tests can exercise truncation with small fixtures).
var gptAutoExportCustomerRowCap int64 = gptAutoExportCustomerMaxRows

func gptAutoExportMaxRows(scope string) int64 {
	if scope == GPTAutoExportScopeCustomer {
		return gptAutoExportCustomerRowCap
	}
	return gptAutoExportAdminMaxRows
}

// PrepareExport validates and reserves an export slot before any byte is
// written, so errors can still be returned as JSON. The returned release
// must be called.
func (s *GPTAutoShadowService) PrepareExport(ctx context.Context, req *GPTAutoExportRequest) (func(), error) {
	if _, ok := s.exportRepo(); !ok {
		return nil, errGPTAutoExportUnavailable
	}
	if req.Scope != GPTAutoExportScopeAdmin && req.Scope != GPTAutoExportScopeCustomer {
		return nil, infraerrors.BadRequest("GPT_AUTO_EXPORT_INVALID", "invalid scope")
	}
	if req.Format != GPTAutoExportFormatCSV && req.Format != GPTAutoExportFormatJSONL {
		return nil, infraerrors.BadRequest("GPT_AUTO_EXPORT_INVALID", "format must be csv or jsonl")
	}
	if req.Scope == GPTAutoExportScopeCustomer && !s.CustomerExportEnabled(ctx) {
		return nil, errGPTAutoExportDisabled
	}
	if err := ValidateGPTAutoExportFilter(req.Scope, &req.Filter); err != nil {
		return nil, err
	}
	release, ok := gptAutoExports.acquire(req.Scope, req.ActorID)
	if !ok {
		return nil, errGPTAutoExportBusy
	}
	return release, nil
}

// Export streams the rows to w. Call PrepareExport first.
func (s *GPTAutoShadowService) Export(ctx context.Context, req GPTAutoExportRequest, w io.Writer) (GPTAutoExportSummary, error) {
	repo, ok := s.exportRepo()
	if !ok {
		return GPTAutoExportSummary{}, errGPTAutoExportUnavailable
	}
	started := s.now()
	ctx, cancel := context.WithTimeout(ctx, gptAutoExportMaxDuration)
	defer cancel()
	maxRows := gptAutoExportMaxRows(req.Scope)
	summary := GPTAutoExportSummary{MaxRows: maxRows}

	buffered := bufio.NewWriterSize(w, 64<<10)
	encoder := newGPTAutoExportEncoder(req.Format, buffered)
	columns := gptAutoAdminExportColumns
	if req.Scope == GPTAutoExportScopeCustomer {
		columns = gptAutoCustomerExportColumns
	}
	err := encoder.header(columns)

	var cursorTime *time.Time
	var cursorID int64
	for err == nil {
		if ctx.Err() != nil {
			err = ctx.Err()
			break
		}
		limit := gptAutoExportBatchSize
		if remaining := maxRows - summary.Rows; remaining < int64(limit) {
			limit = int(remaining) + 1 // one extra row detects truncation
		}
		var rows []GPTAutoShadowDecision
		rows, err = repo.ExportDecisions(ctx, req.Filter, cursorTime, cursorID, limit)
		if err != nil {
			break
		}
		if summary.Rows+int64(len(rows)) > maxRows {
			rows = rows[:maxRows-summary.Rows]
			summary.Truncated = true
		}
		var usage map[GPTAutoExportUsageKey]*GPTAutoExportUsage
		usage, err = s.exportUsageFor(ctx, repo, rows)
		if err != nil {
			break
		}
		for i := range rows {
			row := &rows[i]
			var record []gptAutoExportCell
			if req.Scope == GPTAutoExportScopeCustomer {
				record = gptAutoCustomerExportRecord(row, usage[gptAutoUsageKeyFor(row)])
			} else {
				record = gptAutoAdminExportRecord(row, usage[gptAutoUsageKeyFor(row)], req.Format == GPTAutoExportFormatJSONL)
			}
			if err = encoder.row(columns, record); err != nil {
				break
			}
			summary.Rows++
		}
		if err != nil {
			break
		}
		if err = buffered.Flush(); err != nil {
			break
		}
		if req.Flush != nil {
			req.Flush()
		}
		if summary.Truncated || len(rows) < limit || len(rows) == 0 {
			break
		}
		last := rows[len(rows)-1]
		lastTime := last.CreatedAt
		cursorTime, cursorID = &lastTime, last.ID
	}
	if err == nil {
		err = buffered.Flush()
	}

	status := "completed"
	switch {
	case errors.Is(err, context.Canceled):
		status = "cancelled"
	case errors.Is(err, context.DeadlineExceeded):
		status = "timeout"
	case err != nil:
		status = "failed"
	}
	audit := GPTAutoExportAudit{
		ActorID: req.ActorID, Scope: req.Scope, Format: req.Format, Filters: req.Filter,
		RowCount: summary.Rows, Truncated: summary.Truncated, Status: status,
		DurationMS: int(s.now().Sub(started).Milliseconds()),
	}
	auditCtx, auditCancel := context.WithTimeout(context.WithoutCancel(ctx), gptAutoPersistTimeout)
	if auditErr := repo.InsertExportAudit(auditCtx, audit); auditErr != nil {
		slog.Warn("gpt_auto.export_audit_failed", "error", auditErr)
	}
	auditCancel()
	slog.Info("gpt_auto.export", "scope", req.Scope, "actor_id", req.ActorID, "format", req.Format,
		"rows", summary.Rows, "truncated", summary.Truncated, "status", status)
	return summary, err
}

func gptAutoUsageKeyFor(row *GPTAutoShadowDecision) GPTAutoExportUsageKey {
	key := GPTAutoExportUsageKey{RequestID: row.RequestID}
	if row.APIKeyID != nil {
		key.APIKeyID = *row.APIKeyID
	}
	return key
}

func (s *GPTAutoShadowService) exportUsageFor(ctx context.Context, repo GPTAutoExportRepository, rows []GPTAutoShadowDecision) (map[GPTAutoExportUsageKey]*GPTAutoExportUsage, error) {
	if len(rows) == 0 {
		return nil, nil
	}
	keys := make([]GPTAutoExportUsageKey, 0, len(rows))
	from, to := rows[0].CreatedAt, rows[0].CreatedAt
	for i := range rows {
		key := gptAutoUsageKeyFor(&rows[i])
		if key.RequestID == "" || key.APIKeyID == 0 {
			continue
		}
		keys = append(keys, key)
		if rows[i].CreatedAt.Before(from) {
			from = rows[i].CreatedAt
		}
		if rows[i].CreatedAt.After(to) {
			to = rows[i].CreatedAt
		}
	}
	if len(keys) == 0 {
		return nil, nil
	}
	return repo.ExportUsage(ctx, keys, from.Add(-time.Minute), to.Add(gptAutoExportUsageMatchWindow))
}

// ---- cells and encoders ----------------------------------------------------

type gptAutoExportCellKind uint8

const (
	gptAutoCellNull gptAutoExportCellKind = iota
	gptAutoCellString
	gptAutoCellNumber // already formatted
	gptAutoCellBool
	gptAutoCellJSON // JSONL only: raw JSON value; CSV gets it as an escaped string
)

type gptAutoExportCell struct {
	kind  gptAutoExportCellKind
	value string
}

func cellString(value string) gptAutoExportCell {
	if value == "" {
		return gptAutoExportCell{}
	}
	return gptAutoExportCell{kind: gptAutoCellString, value: value}
}
func cellInt(value int64) gptAutoExportCell {
	return gptAutoExportCell{kind: gptAutoCellNumber, value: strconv.FormatInt(value, 10)}
}
func cellIntPtr(value *int) gptAutoExportCell {
	if value == nil {
		return gptAutoExportCell{}
	}
	return cellInt(int64(*value))
}
func cellInt64Ptr(value *int64) gptAutoExportCell {
	if value == nil {
		return gptAutoExportCell{}
	}
	return cellInt(*value)
}
func cellBool(value bool) gptAutoExportCell {
	return gptAutoExportCell{kind: gptAutoCellBool, value: strconv.FormatBool(value)}
}

// Money uses the NUMERIC(20,10) precision of the source columns; ratios use
// six decimals. Fixed-point strings keep CSV and JSON identical and avoid
// float artefacts such as 0.30000000000000004.
func cellUSD(value decimal.Decimal) gptAutoExportCell {
	return gptAutoExportCell{kind: gptAutoCellNumber, value: value.StringFixed(gptAutoUSDScale)}
}
func cellUSDPtr(value *float64) gptAutoExportCell {
	if value == nil {
		return gptAutoExportCell{}
	}
	return cellUSD(decimal.NewFromFloat(*value))
}
func cellRatio(value *float64) gptAutoExportCell {
	if value == nil || math.IsNaN(*value) || math.IsInf(*value, 0) {
		return gptAutoExportCell{}
	}
	return gptAutoExportCell{kind: gptAutoCellNumber, value: decimal.NewFromFloat(*value).StringFixed(gptAutoMarginScale)}
}
func cellJSON(value any) gptAutoExportCell {
	if value == nil {
		return gptAutoExportCell{}
	}
	raw, err := json.Marshal(value)
	if err != nil || string(raw) == "null" {
		return gptAutoExportCell{}
	}
	return gptAutoExportCell{kind: gptAutoCellJSON, value: string(raw)}
}

type gptAutoExportEncoder interface {
	header(columns []string) error
	row(columns []string, cells []gptAutoExportCell) error
}

func newGPTAutoExportEncoder(format string, w *bufio.Writer) gptAutoExportEncoder {
	if format == GPTAutoExportFormatJSONL {
		return &gptAutoJSONLEncoder{w: w}
	}
	return &gptAutoCSVEncoder{w: w}
}

type gptAutoCSVEncoder struct{ w *bufio.Writer }

func (e *gptAutoCSVEncoder) header(columns []string) error {
	// UTF-8 BOM so Excel detects the encoding; pandas strips it with
	// encoding="utf-8-sig".
	if _, err := e.w.WriteString("\ufeff"); err != nil {
		return err
	}
	cells := make([]gptAutoExportCell, len(columns))
	for i, column := range columns {
		cells[i] = gptAutoExportCell{kind: gptAutoCellString, value: column}
	}
	return e.row(columns, cells)
}

func (e *gptAutoCSVEncoder) row(_ []string, cells []gptAutoExportCell) error {
	for i, cell := range cells {
		if i > 0 {
			if err := e.w.WriteByte(','); err != nil {
				return err
			}
		}
		if _, err := e.w.WriteString(gptAutoCSVField(cell)); err != nil {
			return err
		}
	}
	_, err := e.w.WriteString("\r\n")
	return err
}

// gptAutoCSVField quotes per RFC 4180 and neutralises spreadsheet formulas:
// a text value beginning with = + - @ or a tab/CR is prefixed with an
// apostrophe so Excel/LibreOffice display it instead of evaluating it.
// Numbers formatted by the exporter are never prefixed.
func gptAutoCSVField(cell gptAutoExportCell) string {
	value := cell.value
	switch cell.kind {
	case gptAutoCellNull:
		return ""
	case gptAutoCellNumber, gptAutoCellBool:
		return value
	}
	if value != "" && strings.ContainsRune("=+-@\t\r", rune(value[0])) {
		value = "'" + value
	}
	if strings.ContainsAny(value, ",\"\r\n") || value != strings.TrimSpace(value) || strings.HasPrefix(value, "'") {
		return `"` + strings.ReplaceAll(value, `"`, `""`) + `"`
	}
	return value
}

type gptAutoJSONLEncoder struct{ w *bufio.Writer }

func (e *gptAutoJSONLEncoder) header([]string) error { return nil }

// row writes keys in column order (encoding/json would sort a map).
func (e *gptAutoJSONLEncoder) row(columns []string, cells []gptAutoExportCell) error {
	if err := e.w.WriteByte('{'); err != nil {
		return err
	}
	for i, column := range columns {
		if i > 0 {
			_ = e.w.WriteByte(',')
		}
		key, _ := json.Marshal(column)
		_, _ = e.w.Write(key)
		_ = e.w.WriteByte(':')
		cell := cells[i]
		switch cell.kind {
		case gptAutoCellNull:
			_, _ = e.w.WriteString("null")
		case gptAutoCellString:
			raw, _ := json.Marshal(cell.value)
			_, _ = e.w.Write(raw)
		default:
			_, _ = e.w.WriteString(cell.value)
		}
	}
	_, err := e.w.WriteString("}\n")
	return err
}

// ---- admin schema ----------------------------------------------------------

// gptAutoAdminExportColumns is the documented admin column order
// (docs/GPT_AUTO_ROUTING_LOG_EXPORT.md). Append only; never reorder.
var gptAutoAdminExportColumns = []string{
	"export_schema", "timestamp", "decision_id", "request_id", "mode",
	"user_id", "api_key_id", "group_id", "endpoint", "stream", "has_tools",
	"requested_model", "requested_effort",
	"jev_model", "final_model", "served_model", "usage_model",
	"jev_effort", "final_effort", "usage_effort",
	"complexity", "task_category", "model_confidence", "effort_confidence",
	"evaluation_status", "decision_status", "adjustments",
	"policy_version", "state_version", "classifier_schema", "classifier_config_hash",
	"routing_reason", "fallback_reason", "affinity_outcome",
	"upstream_account_id", "upstream_channel_id", "routing_weight", "billing_multiplier", "retry_count",
	"input_tokens", "cached_input_tokens", "cache_creation_tokens", "output_tokens",
	"classifier_input_tokens", "classifier_output_tokens", "classifier_tokens", "classifier_cost_usd",
	"customer_charge_usd", "list_cost_usd", "upstream_cost_usd",
	"gross_profit_usd", "gross_margin", "fully_loaded_profit_usd", "fully_loaded_margin",
	"est_revenue_usd", "est_upstream_cost_usd", "est_gross_profit_usd", "est_gross_margin", "est_model_gross_margin", "estimate_quality",
	"economics_basis", "billing_status",
	"classifier_latency_ms", "gateway_latency_ms", "upstream_ttft_ms", "total_latency_ms",
	"request_status", "http_status", "error_category",
	// JSONL carries structured values; CSV carries the same JSON as text.
	"model_probabilities", "effort_probabilities", "complexity_probabilities", "category_probabilities", "context_diagnostics",
	// v2: pricing identity and the tariff snapshot captured before execution.
	// Empty on rows recorded before Phase 2.1; never recomputed from today's
	// tariff. customer_charge_usd stays usage_logs.actual_cost.
	"executed_model", "pricing_model", "pricing_mode", "pricing_config_version", "pricing_source",
	"tariff_input_per_mtok", "tariff_cached_input_per_mtok", "tariff_output_per_mtok", "tariff_cache_write_per_mtok",
}

func gptAutoRowMode(row *GPTAutoShadowDecision) string {
	if row.Mode == GPTAutoModeLive {
		return GPTAutoModeLive
	}
	return GPTAutoModeShadow
}

// gptAutoExportActualEconomics derives realised figures from the usage-log
// row. Negative margins are kept; a zero charge leaves margins empty.
type gptAutoExportActualEconomics struct {
	charge, list, upstream     decimal.Decimal
	profit, loaded             decimal.Decimal
	margin, loadedMargin       *float64
	basis, billingStatus       string
	inputTokens, cachedTokens  *int64
	outputTokens, cacheCreated *int64
}

func gptAutoExportEconomics(row *GPTAutoShadowDecision, usage *GPTAutoExportUsage) gptAutoExportActualEconomics {
	out := gptAutoExportActualEconomics{}
	mode := gptAutoRowMode(row)
	if usage == nil {
		out.billingStatus = "not_recorded"
		if mode == GPTAutoModeLive && row.RequestStatus == GPTAutoRequestSucceeded {
			// Usage is written asynchronously after the response.
			out.billingStatus = "pending"
		}
		if mode == GPTAutoModeShadow {
			out.basis = "hypothetical_estimate"
		}
		return out
	}
	out.billingStatus = "final"
	out.basis = "actual_usage"
	if mode == GPTAutoModeShadow {
		// The shadow request ran on the customer's own model; these are its
		// real figures, not the recommendation's.
		out.basis = "actual_usage_of_requested_model"
	}
	out.charge = gptAutoDecimal(usage.ChargedCost)
	out.list = gptAutoDecimal(usage.ListCost)
	out.upstream = gptAutoDecimal(usage.AccountCost)
	out.profit = out.charge.Sub(out.upstream)
	classifier := gptAutoDecimal(row.ClassifierCost)
	out.loaded = out.profit.Sub(classifier)
	if out.charge.IsPositive() {
		margin, _ := out.profit.Div(out.charge).Round(gptAutoMarginScale).Float64()
		loaded, _ := out.loaded.Div(out.charge).Round(gptAutoMarginScale).Float64()
		out.margin, out.loadedMargin = &margin, &loaded
	}
	in, cached, output, creation := usage.InputTokens, usage.CacheReadTokens, usage.OutputTokens, usage.CacheCreationTokens
	out.inputTokens, out.cachedTokens, out.outputTokens, out.cacheCreated = &in, &cached, &output, &creation
	return out
}

func gptAutoAdminExportRecord(row *GPTAutoShadowDecision, usage *GPTAutoExportUsage, structured bool) []gptAutoExportCell {
	economics := gptAutoExportEconomics(row, usage)
	var usageModel, usageEffort string
	var channel *int64
	if usage != nil {
		usageModel, usageEffort, channel = usage.Model, usage.ReasoningEffort, usage.ChannelID
	}
	var adjustments []string
	if row.Decision != nil {
		for _, adjustment := range row.Decision.Adjustments {
			adjustments = append(adjustments, adjustment.Rule)
		}
	}
	var modelProbs, effortProbs, complexityProbs, categoryProbs any
	if cls := row.Classification; cls != nil {
		if len(cls.ModelProbabilities) > 0 {
			modelProbs = cls.ModelProbabilities
		}
		if len(cls.EffortProbabilities) > 0 {
			effortProbs = cls.EffortProbabilities
		}
		if len(cls.ComplexityProbabilities) > 0 {
			complexityProbs = cls.ComplexityProbabilities
		}
		if len(cls.CategoryProbabilities) > 0 {
			categoryProbs = cls.CategoryProbabilities
		}
	}
	var diagnostics any
	if row.Features.Context != nil {
		diagnostics = row.Features.Context
	}
	jsonCell := func(value any) gptAutoExportCell {
		cell := cellJSON(value)
		if !structured && cell.kind == gptAutoCellJSON {
			cell.kind = gptAutoCellString
		}
		return cell
	}
	classifierTokens := int64(row.ClassifierInputTokens + row.ClassifierOutputTokens)
	usd := func(value decimal.Decimal) gptAutoExportCell {
		if usage == nil {
			return gptAutoExportCell{}
		}
		return cellUSD(value)
	}
	var multiplier *float64
	if row.BillingMultiplier != nil {
		value := *row.BillingMultiplier
		multiplier = &value
	}
	return []gptAutoExportCell{
		cellString(GPTAutoExportSchemaVersion),
		cellString(row.CreatedAt.UTC().Format(time.RFC3339Nano)),
		cellInt(row.ID),
		cellString(row.RequestID),
		cellString(gptAutoRowMode(row)),
		cellInt64Ptr(row.UserID), cellInt64Ptr(row.APIKeyID), cellInt64Ptr(row.GroupID),
		cellString(row.Endpoint), cellBool(row.Stream), cellBool(row.HasTools),
		cellString(row.ActualModel), cellString(row.ActualEffort),
		cellString(row.JevModel), cellString(row.FinalModel), cellString(row.ServedModel), cellString(usageModel),
		cellString(row.JevEffort), cellString(row.FinalEffort), cellString(usageEffort),
		cellRatio(row.Complexity), cellString(row.Category), cellRatio(row.ModelConfidence), cellRatio(row.EffortConfidence),
		cellString(row.EvaluationStatus), cellString(row.DecisionStatus), cellString(strings.Join(adjustments, ";")),
		cellString(row.PolicyVersion), cellString(row.StateVersion), cellString(row.ClassifierSchema), cellString(row.ClassifierConfigHash),
		cellString(row.RoutingReason), cellString(row.FallbackReason), cellString(row.AffinityOutcome),
		cellInt64Ptr(row.UpstreamAccountID), cellInt64Ptr(channel), cellIntPtr(row.RoutingWeight), cellRatio(multiplier), cellInt(int64(row.RetryCount)),
		cellInt64Ptr(economics.inputTokens), cellInt64Ptr(economics.cachedTokens), cellInt64Ptr(economics.cacheCreated), cellInt64Ptr(economics.outputTokens),
		cellInt(int64(row.ClassifierInputTokens)), cellInt(int64(row.ClassifierOutputTokens)), cellInt(classifierTokens), cellUSD(gptAutoDecimal(row.ClassifierCost)),
		usd(economics.charge), usd(economics.list), usd(economics.upstream),
		usd(economics.profit), cellRatio(economics.margin), usd(economics.loaded), cellRatio(economics.loadedMargin),
		cellUSDPtr(row.EstRevenue), cellUSDPtr(row.EstUpstreamCost), cellUSDPtr(row.EstGrossProfit), cellRatio(row.EstGrossMargin), cellRatio(row.EstModelGrossMargin), cellString(row.EstimateQuality),
		cellString(economics.basis), cellString(economics.billingStatus),
		gptAutoClassifierLatencyCell(row), cellIntPtr(row.GatewayLatencyMS), cellIntPtr(row.UpstreamTTFTMS), cellIntPtr(row.TotalLatencyMS),
		cellString(row.RequestStatus), cellIntPtr(row.HTTPStatus), cellString(gptAutoErrorCategory(row)),
		jsonCell(modelProbs), jsonCell(effortProbs), jsonCell(complexityProbs), jsonCell(categoryProbs), jsonCell(diagnostics),
		cellString(gptAutoExecutedModel(row)), cellString(row.PricingModel), cellString(row.PricingMode),
		cellIntPtr(row.PricingConfigVersion), cellString(row.PricingSource),
		cellDecimalPtr(row.TariffInputPerMTok), cellDecimalPtr(row.TariffCachedPerMTok),
		cellDecimalPtr(row.TariffOutputPerMTok), cellDecimalPtr(row.TariffCacheWritePerMTok),
	}
}

// gptAutoExecutedModel is the model that actually ran upstream: live rows
// only (a shadow recommendation never executed), empty when rejected.
func gptAutoExecutedModel(row *GPTAutoShadowDecision) string {
	if gptAutoRowMode(row) != GPTAutoModeLive || row.RequestStatus == GPTAutoRequestRejected {
		return ""
	}
	return row.FinalModel
}

func cellDecimalPtr(value *decimal.Decimal) gptAutoExportCell {
	if value == nil {
		return gptAutoExportCell{}
	}
	return cellUSD(*value)
}

// gptAutoClassifierLatencyCell is empty when Jev was never called.
func gptAutoClassifierLatencyCell(row *GPTAutoShadowDecision) gptAutoExportCell {
	if row.Attempts == 0 && row.ClassifierLatencyMS == 0 {
		return gptAutoExportCell{}
	}
	return cellInt(int64(row.ClassifierLatencyMS))
}

func gptAutoErrorCategory(row *GPTAutoShadowDecision) string {
	if row.ErrorCategory != "" {
		return row.ErrorCategory
	}
	return row.ErrorClass
}

// ---- customer schema -------------------------------------------------------

// gptAutoCustomerExportColumns is the documented customer column order.
var gptAutoCustomerExportColumns = []string{
	"export_schema", "timestamp", "request_id", "api_key_id", "endpoint", "stream",
	"requested_model", "executed_model", "reasoning_effort",
	"status", "http_status",
	"input_tokens", "cached_input_tokens", "output_tokens",
	"charge_usd", "billing_status",
	"total_latency_ms", "time_to_first_token_ms",
	"fallback", "routing_explanation",
	// v2
	"pricing_model",
}

// gptAutoCustomerExplanation is a fixed, non-sensitive description. Internal
// failure classes (classifier credentials, budgets, circuit state) are not
// exposed.
func gptAutoCustomerExplanation(row *GPTAutoShadowDecision) string {
	switch row.RoutingReason {
	case GPTAutoRouteClassified:
		return "Model and reasoning effort selected automatically for this request."
	case GPTAutoRouteAffinity:
		return "Kept on the same model as the earlier turns of this conversation."
	case GPTAutoRouteFallback:
		return "Automatic selection was unavailable; a compatible default model was used."
	}
	if row.RequestStatus == GPTAutoRequestRejected {
		return "The request could not be routed to a compatible model."
	}
	return ""
}

func gptAutoCustomerExportRecord(row *GPTAutoShadowDecision, usage *GPTAutoExportUsage) []gptAutoExportCell {
	economics := gptAutoExportEconomics(row, usage)
	charge := gptAutoExportCell{}
	if usage != nil {
		charge = cellUSD(economics.charge)
	}
	executed := row.FinalModel
	if row.RequestStatus == GPTAutoRequestRejected {
		executed = ""
	}
	return []gptAutoExportCell{
		cellString(GPTAutoExportSchemaVersion),
		cellString(row.CreatedAt.UTC().Format(time.RFC3339Nano)),
		cellString(row.RequestID),
		cellInt64Ptr(row.APIKeyID),
		cellString(row.Endpoint), cellBool(row.Stream),
		cellString(row.ActualModel), cellString(executed), cellString(row.FinalEffort),
		cellString(row.RequestStatus), cellIntPtr(row.HTTPStatus),
		cellInt64Ptr(economics.inputTokens), cellInt64Ptr(economics.cachedTokens), cellInt64Ptr(economics.outputTokens),
		charge, cellString(economics.billingStatus),
		cellIntPtr(row.TotalLatencyMS), cellIntPtr(row.UpstreamTTFTMS),
		cellBool(row.RoutingReason == GPTAutoRouteFallback || row.RetryCount > 0),
		cellString(gptAutoCustomerExplanation(row)),
		cellString(row.PricingModel),
	}
}

// GPTAutoCustomerLogEntry is the customer-facing list DTO (dashboard). It is
// built from the same safe fields as the customer export.
type GPTAutoCustomerLogEntry struct {
	CreatedAt       time.Time `json:"created_at"`
	RequestID       string    `json:"request_id"`
	APIKeyID        *int64    `json:"api_key_id,omitempty"`
	Endpoint        string    `json:"endpoint"`
	RequestedModel  string    `json:"requested_model"`
	ExecutedModel   string    `json:"executed_model"`
	ReasoningEffort string    `json:"reasoning_effort"`
	Status          string    `json:"status"`
	TotalLatencyMS  *int      `json:"total_latency_ms,omitempty"`
	Fallback        bool      `json:"fallback"`
	Explanation     string    `json:"routing_explanation"`
}

// ListCustomerLogs pages one user's live gpt-auto decisions.
func (s *GPTAutoShadowService) ListCustomerLogs(ctx context.Context, userID int64, filter GPTAutoDecisionFilter) ([]GPTAutoCustomerLogEntry, int64, error) {
	if userID <= 0 {
		return nil, 0, infraerrors.Unauthorized("UNAUTHORIZED", "authentication required")
	}
	filter.UserID = &userID
	filter.Mode = GPTAutoModeLive
	rows, page, err := s.ListDecisions(ctx, filter)
	if err != nil {
		return nil, 0, err
	}
	out := make([]GPTAutoCustomerLogEntry, 0, len(rows))
	for i := range rows {
		row := &rows[i]
		executed := row.FinalModel
		if row.RequestStatus == GPTAutoRequestRejected {
			executed = ""
		}
		out = append(out, GPTAutoCustomerLogEntry{
			CreatedAt: row.CreatedAt, RequestID: row.RequestID, APIKeyID: row.APIKeyID, Endpoint: row.Endpoint,
			RequestedModel: row.ActualModel, ExecutedModel: executed, ReasoningEffort: row.FinalEffort,
			Status: row.RequestStatus, TotalLatencyMS: row.TotalLatencyMS,
			Fallback:    row.RoutingReason == GPTAutoRouteFallback || row.RetryCount > 0,
			Explanation: gptAutoCustomerExplanation(row),
		})
	}
	return out, page.Total, nil
}

// GPTAutoNow is the clock used for default export windows.
func GPTAutoNow() time.Time { return time.Now() }

// CustomerHasHistory reports whether the user has any live gpt-auto request
// in the customer window, so the dashboard section is not shown to customers
// who were never given access.
func (s *GPTAutoShadowService) CustomerHasHistory(ctx context.Context, userID int64) bool {
	repo, ok := s.exportRepo()
	if !ok || userID <= 0 {
		return false
	}
	end := s.now().UTC()
	count, err := repo.CountDecisions(ctx, GPTAutoExportFilter{
		StartTime: end.Add(-gptAutoExportCustomerMaxWin), EndTime: end.Add(time.Minute),
		Mode: GPTAutoModeLive, UserID: &userID,
	}, 1)
	return err == nil && count > 0
}
