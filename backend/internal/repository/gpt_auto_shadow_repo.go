package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/lib/pq"
)

// gptAutoShadowRepository stores GPT-Auto shadow decisions (raw SQL,
// append-only with retention cleanup).
type gptAutoShadowRepository struct {
	db *sql.DB
}

// NewGPTAutoShadowRepository creates the shadow decision repository.
func NewGPTAutoShadowRepository(db *sql.DB) service.GPTAutoShadowRepository {
	return &gptAutoShadowRepository{db: db}
}

const gptAutoDecisionColumns = `id, created_at, request_id, user_id, api_key_id, group_id, endpoint,
actual_model, actual_effort, stream, has_tools, evaluation_status, error_class, decision_status,
jev_model, jev_effort, final_model, final_effort, complexity, category, model_confidence,
effort_confidence, classifier_model, classifier_input_tokens, classifier_output_tokens,
classifier_cost, classifier_latency_ms, queue_delay_ms, attempts, est_revenue, est_upstream_cost,
est_gross_profit, est_gross_margin, margin_below_target, policy_version, policy_snapshot_hash,
classifier_config_hash, est_model_gross_margin, model_margin_below_target, estimate_quality`

func truncateGPTAuto(value string, max int) string {
	if len(value) <= max {
		return value
	}
	return value[:max]
}

func (r *gptAutoShadowRepository) Insert(ctx context.Context, d *service.GPTAutoShadowDecision) error {
	if d == nil {
		return errors.New("nil decision")
	}
	features, err := json.Marshal(d.Features)
	if err != nil {
		return err
	}
	var classification, decision any
	if d.Classification != nil {
		raw, err := json.Marshal(d.Classification)
		if err != nil {
			return err
		}
		classification = string(raw)
	}
	if d.Decision != nil {
		raw, err := json.Marshal(d.Decision)
		if err != nil {
			return err
		}
		decision = string(raw)
	}
	createdAt := d.CreatedAt
	if createdAt.IsZero() {
		createdAt = time.Now().UTC()
	}
	return r.db.QueryRowContext(ctx, `
INSERT INTO gpt_auto_shadow_decisions (
  created_at, request_id, user_id, api_key_id, group_id, endpoint, actual_model, actual_effort,
  stream, has_tools, evaluation_status, error_class, decision_status, jev_model, jev_effort,
  final_model, final_effort, complexity, category, model_confidence, effort_confidence,
  classifier_model, classifier_input_tokens, classifier_output_tokens, classifier_cost,
  classifier_latency_ms, queue_delay_ms, attempts, est_revenue, est_upstream_cost,
  est_gross_profit, est_gross_margin, margin_below_target, policy_version, policy_snapshot_hash,
  features, classification, decision,
  classifier_config_hash, est_model_gross_margin, model_margin_below_target, estimate_quality
) VALUES (
  $1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19,$20,$21,$22,$23,$24,$25,
  $26,$27,$28,$29,$30,$31,$32,$33,$34,$35,$36::jsonb,$37::jsonb,$38::jsonb,$39,$40,$41,$42
) RETURNING id`,
		createdAt,
		truncateGPTAuto(d.RequestID, 128),
		d.UserID, d.APIKeyID, d.GroupID,
		truncateGPTAuto(d.Endpoint, 32),
		truncateGPTAuto(d.ActualModel, 128),
		truncateGPTAuto(d.ActualEffort, 16),
		d.Stream, d.HasTools,
		truncateGPTAuto(d.EvaluationStatus, 16),
		truncateGPTAuto(d.ErrorClass, 32),
		truncateGPTAuto(d.DecisionStatus, 32),
		truncateGPTAuto(d.JevModel, 64),
		truncateGPTAuto(d.JevEffort, 16),
		truncateGPTAuto(d.FinalModel, 64),
		truncateGPTAuto(d.FinalEffort, 16),
		d.Complexity,
		truncateGPTAuto(d.Category, 32),
		d.ModelConfidence, d.EffortConfidence,
		truncateGPTAuto(d.ClassifierModel, 64),
		d.ClassifierInputTokens, d.ClassifierOutputTokens, d.ClassifierCost,
		d.ClassifierLatencyMS, d.QueueDelayMS, d.Attempts,
		d.EstRevenue, d.EstUpstreamCost, d.EstGrossProfit, d.EstGrossMargin, d.MarginBelowTarget,
		truncateGPTAuto(d.PolicyVersion, 64),
		truncateGPTAuto(d.PolicySnapshotHash, 64),
		string(features), classification, decision,
		truncateGPTAuto(d.ClassifierConfigHash, 64), d.EstModelGrossMargin, d.ModelMarginBelowTarget,
		truncateGPTAuto(d.EstimateQuality, 32),
	).Scan(&d.ID)
}

func (r *gptAutoShadowRepository) UpsertPolicySnapshot(ctx context.Context, hash, policyVersion string, snapshot []byte) error {
	_, err := r.db.ExecContext(ctx, `
INSERT INTO gpt_auto_policy_snapshots (hash, policy_version, snapshot)
VALUES ($1, $2, $3::jsonb)
ON CONFLICT (hash) DO NOTHING`, hash, truncateGPTAuto(policyVersion, 64), string(snapshot))
	return err
}

func (r *gptAutoShadowRepository) GetPolicySnapshot(ctx context.Context, hash string) (*service.GPTAutoPolicySnapshot, error) {
	var raw []byte
	err := r.db.QueryRowContext(ctx, `SELECT snapshot FROM gpt_auto_policy_snapshots WHERE hash = $1`, hash).Scan(&raw)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, service.ErrGPTAutoDecisionNotFound
	}
	if err != nil {
		return nil, err
	}
	var snapshot service.GPTAutoPolicySnapshot
	if err := json.Unmarshal(raw, &snapshot); err != nil {
		return nil, err
	}
	return &snapshot, nil
}

func (r *gptAutoShadowRepository) UpsertClassifierConfig(ctx context.Context, record service.GPTAutoClassifierConfigRecord) error {
	raw, err := json.Marshal(record.Snapshot)
	if err != nil {
		return err
	}
	createdAt := record.CreatedAt
	if createdAt.IsZero() {
		createdAt = time.Now().UTC()
	}
	// The first writer of a hash keeps its revision, author and time: a later
	// shadow decision re-upserting the same content must not erase who saved it.
	_, err = r.db.ExecContext(ctx, `
INSERT INTO gpt_auto_classifier_configs (hash, revision, schema_version, snapshot, created_by, created_at)
VALUES ($1, $2, $3, $4::jsonb, $5, $6)
ON CONFLICT (hash) DO UPDATE SET created_by = COALESCE(gpt_auto_classifier_configs.created_by, EXCLUDED.created_by)`,
		truncateGPTAuto(record.Hash, 64), record.Revision, truncateGPTAuto(record.Snapshot.SchemaVersion, 32),
		string(raw), record.CreatedBy, createdAt)
	return err
}

func (r *gptAutoShadowRepository) GetClassifierConfig(ctx context.Context, hash string) (*service.GPTAutoClassifierConfigRecord, error) {
	row := r.db.QueryRowContext(ctx, `SELECT hash, revision, snapshot, created_by, created_at
FROM gpt_auto_classifier_configs WHERE hash = $1`, hash)
	record, err := scanGPTAutoClassifierConfig(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, service.ErrGPTAutoDecisionNotFound
	}
	return record, err
}

func (r *gptAutoShadowRepository) ListClassifierConfigs(ctx context.Context, limit int) ([]service.GPTAutoClassifierConfigRecord, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	rows, err := r.db.QueryContext(ctx, `SELECT hash, revision, snapshot, created_by, created_at
FROM gpt_auto_classifier_configs ORDER BY created_at DESC, hash LIMIT $1`, limit)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	out := []service.GPTAutoClassifierConfigRecord{}
	for rows.Next() {
		record, err := scanGPTAutoClassifierConfig(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *record)
	}
	return out, rows.Err()
}

func scanGPTAutoClassifierConfig(row gptAutoRowScanner) (*service.GPTAutoClassifierConfigRecord, error) {
	var (
		record service.GPTAutoClassifierConfigRecord
		raw    []byte
		author sql.NullInt64
	)
	if err := row.Scan(&record.Hash, &record.Revision, &raw, &author, &record.CreatedAt); err != nil {
		return nil, err
	}
	if err := json.Unmarshal(raw, &record.Snapshot); err != nil {
		return nil, err
	}
	record.CreatedBy = gptAutoNullInt64(author)
	record.CreatedAt = record.CreatedAt.UTC()
	return &record, nil
}

type gptAutoRowScanner interface {
	Scan(dest ...any) error
}

func scanGPTAutoDecision(row gptAutoRowScanner, extra ...any) (*service.GPTAutoShadowDecision, error) {
	var (
		d                                         service.GPTAutoShadowDecision
		userID, apiKeyID, groupID                 sql.NullInt64
		complexity, modelConf, effortConf, margin sql.NullFloat64
		revenue, upstreamCost, grossProfit        sql.NullFloat64
		modelMargin                               sql.NullFloat64
	)
	dest := []any{
		&d.ID, &d.CreatedAt, &d.RequestID, &userID, &apiKeyID, &groupID, &d.Endpoint,
		&d.ActualModel, &d.ActualEffort, &d.Stream, &d.HasTools, &d.EvaluationStatus, &d.ErrorClass,
		&d.DecisionStatus, &d.JevModel, &d.JevEffort, &d.FinalModel, &d.FinalEffort, &complexity,
		&d.Category, &modelConf, &effortConf, &d.ClassifierModel, &d.ClassifierInputTokens,
		&d.ClassifierOutputTokens, &d.ClassifierCost, &d.ClassifierLatencyMS, &d.QueueDelayMS,
		&d.Attempts, &revenue, &upstreamCost, &grossProfit, &margin, &d.MarginBelowTarget,
		&d.PolicyVersion, &d.PolicySnapshotHash,
		&d.ClassifierConfigHash, &modelMargin, &d.ModelMarginBelowTarget, &d.EstimateQuality,
	}
	dest = append(dest, extra...)
	if err := row.Scan(dest...); err != nil {
		return nil, err
	}
	d.UserID = gptAutoNullInt64(userID)
	d.APIKeyID = gptAutoNullInt64(apiKeyID)
	d.GroupID = gptAutoNullInt64(groupID)
	d.Complexity = nullFloat64Ptr(complexity)
	d.ModelConfidence = nullFloat64Ptr(modelConf)
	d.EffortConfidence = nullFloat64Ptr(effortConf)
	d.EstRevenue = nullFloat64Ptr(revenue)
	d.EstUpstreamCost = nullFloat64Ptr(upstreamCost)
	d.EstGrossProfit = nullFloat64Ptr(grossProfit)
	d.EstGrossMargin = nullFloat64Ptr(margin)
	d.EstModelGrossMargin = nullFloat64Ptr(modelMargin)
	d.CreatedAt = d.CreatedAt.UTC()
	return &d, nil
}

func gptAutoNullInt64(value sql.NullInt64) *int64 {
	if !value.Valid {
		return nil
	}
	out := value.Int64
	return &out
}

func (r *gptAutoShadowRepository) GetByID(ctx context.Context, id int64) (*service.GPTAutoShadowDecision, error) {
	var features, classification, decision []byte
	row := r.db.QueryRowContext(ctx, `SELECT `+gptAutoDecisionColumns+`, features, classification, decision
FROM gpt_auto_shadow_decisions WHERE id = $1`, id)
	d, err := scanGPTAutoDecision(row, &features, &classification, &decision)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, service.ErrGPTAutoDecisionNotFound
	}
	if err != nil {
		return nil, err
	}
	if len(features) > 0 {
		_ = json.Unmarshal(features, &d.Features)
	}
	if len(classification) > 0 {
		var value service.GPTAutoClassification
		if json.Unmarshal(classification, &value) == nil {
			d.Classification = &value
		}
	}
	if len(decision) > 0 {
		var value service.GPTAutoPolicyDecision
		if json.Unmarshal(decision, &value) == nil {
			d.Decision = &value
		}
	}
	return d, nil
}

type gptAutoWhere struct {
	clauses []string
	args    []any
}

func (w *gptAutoWhere) add(clause string, value any) {
	w.args = append(w.args, value)
	w.clauses = append(w.clauses, fmt.Sprintf(clause, len(w.args)))
}

func (w *gptAutoWhere) sql() string {
	if len(w.clauses) == 0 {
		return ""
	}
	return " WHERE " + strings.Join(w.clauses, " AND ")
}

func (r *gptAutoShadowRepository) List(ctx context.Context, filter service.GPTAutoDecisionFilter) ([]service.GPTAutoShadowDecision, *pagination.PaginationResult, error) {
	where := &gptAutoWhere{}
	if filter.StartTime != nil {
		where.add("created_at >= $%d", *filter.StartTime)
	}
	if filter.EndTime != nil {
		where.add("created_at < $%d", *filter.EndTime)
	}
	if filter.ActualModel != "" {
		where.add("actual_model = $%d", filter.ActualModel)
	}
	if filter.FinalModel != "" {
		where.add("final_model = $%d", filter.FinalModel)
	}
	if filter.FinalEffort != "" {
		where.add("final_effort = $%d", filter.FinalEffort)
	}
	if filter.EvaluationStatus != "" {
		where.add("evaluation_status = $%d", filter.EvaluationStatus)
	}
	if filter.DecisionStatus != "" {
		where.add("decision_status = $%d", filter.DecisionStatus)
	}
	if filter.GroupID != nil {
		where.add("group_id = $%d", *filter.GroupID)
	}
	if filter.RequestID != "" {
		where.add("request_id = $%d", filter.RequestID)
	}

	var total int64
	if err := r.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM gpt_auto_shadow_decisions`+where.sql(), where.args...).Scan(&total); err != nil {
		return nil, nil, err
	}
	page, pageSize := filter.Pagination.Page, filter.Pagination.PageSize
	args := append(append([]any(nil), where.args...), pageSize, (page-1)*pageSize)
	rows, err := r.db.QueryContext(ctx, `SELECT `+gptAutoDecisionColumns+` FROM gpt_auto_shadow_decisions`+where.sql()+
		fmt.Sprintf(` ORDER BY created_at DESC, id DESC LIMIT $%d OFFSET $%d`, len(where.args)+1, len(where.args)+2), args...)
	if err != nil {
		return nil, nil, err
	}
	defer func() { _ = rows.Close() }()
	items := make([]service.GPTAutoShadowDecision, 0, pageSize)
	for rows.Next() {
		d, err := scanGPTAutoDecision(rows)
		if err != nil {
			return nil, nil, err
		}
		items = append(items, *d)
	}
	if err := rows.Err(); err != nil {
		return nil, nil, err
	}
	pages := int((total + int64(pageSize) - 1) / int64(pageSize))
	return items, &pagination.PaginationResult{Total: total, Page: page, PageSize: pageSize, Pages: pages}, nil
}

func (r *gptAutoShadowRepository) Stats(ctx context.Context, filter service.GPTAutoStatsFilter) (*service.GPTAutoStoredStats, error) {
	where := &gptAutoWhere{}
	where.add("created_at >= $%d", filter.StartTime)
	where.add("created_at < $%d", filter.EndTime)
	if filter.GroupID != nil {
		where.add("group_id = $%d", *filter.GroupID)
	}
	base := ` FROM gpt_auto_shadow_decisions` + where.sql()
	stats := &service.GPTAutoStoredStats{}

	var avg, p50, p95 sql.NullFloat64
	err := r.db.QueryRowContext(ctx, `SELECT
  COUNT(*),
  COUNT(*) FILTER (WHERE evaluation_status = 'success'),
  COUNT(*) FILTER (WHERE evaluation_status = 'failed'),
  AVG(classifier_latency_ms) FILTER (WHERE evaluation_status = 'success'),
  percentile_cont(0.5) WITHIN GROUP (ORDER BY classifier_latency_ms) FILTER (WHERE evaluation_status = 'success'),
  percentile_cont(0.95) WITHIN GROUP (ORDER BY classifier_latency_ms) FILTER (WHERE evaluation_status = 'success'),
  COALESCE(SUM(classifier_input_tokens), 0),
  COALESCE(SUM(classifier_output_tokens), 0),
  COALESCE(SUM(classifier_cost), 0)::float8,
  COUNT(*) FILTER (WHERE decision_status = 'adjusted'),
  COUNT(*) FILTER (WHERE decision_status = 'no_eligible_candidate'),
  COUNT(*) FILTER (WHERE margin_below_target),
  COUNT(*) FILTER (WHERE model_margin_below_target),
  COUNT(*) FILTER (WHERE evaluation_status = 'success' AND estimate_quality = 'insufficient_data'),
  COUNT(*) FILTER (WHERE evaluation_status = 'success' AND final_model <> '' AND final_model <> jev_model),
  COUNT(*) FILTER (WHERE evaluation_status = 'success' AND final_effort <> '' AND final_effort <> jev_effort)`+base, where.args...).Scan(
		&stats.Total, &stats.Success, &stats.Failed, &avg, &p50, &p95,
		&stats.InputTokens, &stats.OutputTokens, &stats.ClassifierCost,
		&stats.Adjusted, &stats.NoEligible, &stats.BelowTargetCount,
		&stats.ModelBelowTargetCount, &stats.InsufficientEstimates,
		&stats.PolicyModelChanges, &stats.PolicyEffortChanges)
	if err != nil {
		return nil, err
	}
	stats.AvgLatencyMS, stats.P50LatencyMS, stats.P95LatencyMS = nullFloat64Ptr(avg), nullFloat64Ptr(p50), nullFloat64Ptr(p95)

	success := base + " AND evaluation_status = 'success'"
	distributions := []struct {
		target *[]service.GPTAutoCount
		expr   string
		from   string
	}{
		{&stats.FinalModels, "final_model", success},
		{&stats.JevModels, "jev_model", success},
		{&stats.FinalEfforts, "final_effort", success},
		{&stats.JevEfforts, "jev_effort", success},
		{&stats.ClassifierConfigs, "classifier_config_hash", success},
		{&stats.Categories, "category", success},
		{&stats.Complexity, "COALESCE(ROUND(complexity::numeric)::int::text, '')", success},
		{&stats.ErrorClasses, "error_class", base + " AND evaluation_status = 'failed'"},
	}
	for _, dist := range distributions {
		counts, err := r.countBy(ctx, dist.expr, dist.from, where.args)
		if err != nil {
			return nil, err
		}
		*dist.target = counts
	}

	pairRows, err := r.db.QueryContext(ctx, `SELECT actual_model, final_model, COUNT(*)`+success+
		` GROUP BY 1, 2 ORDER BY 3 DESC LIMIT 100`, where.args...)
	if err != nil {
		return nil, err
	}
	for pairRows.Next() {
		var pair service.GPTAutoPairCount
		if err := pairRows.Scan(&pair.Actual, &pair.Recommended, &pair.Count); err != nil {
			_ = pairRows.Close()
			return nil, err
		}
		stats.ActualVsFinal = append(stats.ActualVsFinal, pair)
	}
	_ = pairRows.Close()
	if err := pairRows.Err(); err != nil {
		return nil, err
	}

	bucket := "hour"
	if filter.EndTime.Sub(filter.StartTime) > 3*24*time.Hour {
		bucket = "day"
	}
	stats.TrendBucketSize = bucket
	trendRows, err := r.db.QueryContext(ctx, `SELECT date_trunc('`+bucket+`', created_at AT TIME ZONE 'UTC') AS bucket,
  COUNT(*) FILTER (WHERE evaluation_status = 'success'),
  COUNT(*) FILTER (WHERE evaluation_status = 'failed')`+base+` GROUP BY 1 ORDER BY 1`, where.args...)
	if err != nil {
		return nil, err
	}
	defer func() { _ = trendRows.Close() }()
	for trendRows.Next() {
		var point service.GPTAutoTrendPoint
		if err := trendRows.Scan(&point.Bucket, &point.Success, &point.Failed); err != nil {
			return nil, err
		}
		point.Bucket = time.Date(point.Bucket.Year(), point.Bucket.Month(), point.Bucket.Day(), point.Bucket.Hour(), 0, 0, 0, time.UTC)
		stats.Trend = append(stats.Trend, point)
	}
	return stats, trendRows.Err()
}

// countBy groups by a fixed, code-defined expression (never user input).
func (r *gptAutoShadowRepository) countBy(ctx context.Context, expr, from string, args []any) ([]service.GPTAutoCount, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT `+expr+` AS key, COUNT(*)`+from+` GROUP BY 1 ORDER BY 2 DESC LIMIT 50`, args...)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	counts := []service.GPTAutoCount{}
	for rows.Next() {
		var count service.GPTAutoCount
		if err := rows.Scan(&count.Key, &count.Count); err != nil {
			return nil, err
		}
		counts = append(counts, count)
	}
	return counts, rows.Err()
}

func (r *gptAutoShadowRepository) UsageSince(ctx context.Context, since time.Time) (service.GPTAutoUsageToday, error) {
	var usage service.GPTAutoUsageToday
	err := r.db.QueryRowContext(ctx, `SELECT COUNT(*), COALESCE(SUM(classifier_cost), 0)::float8
FROM gpt_auto_shadow_decisions WHERE created_at >= $1`, since).Scan(&usage.Calls, &usage.Cost)
	return usage, err
}

func (r *gptAutoShadowRepository) DeleteOlderThan(ctx context.Context, cutoff time.Time, batchSize int) (int64, error) {
	result, err := r.db.ExecContext(ctx, `DELETE FROM gpt_auto_shadow_decisions WHERE id IN (
  SELECT id FROM gpt_auto_shadow_decisions WHERE created_at < $1 ORDER BY created_at LIMIT $2)`, cutoff, batchSize)
	if err != nil {
		return 0, err
	}
	return result.RowsAffected()
}

// gptAutoActualUsageWindow bounds the usage_logs lookup around a decision so
// the (possibly partitioned) table is pruned by created_at. Long streams are
// logged when they finish, after the shadow decision was written.
const gptAutoActualUsageWindow = 6 * time.Hour

func (r *gptAutoShadowRepository) ActualUsage(ctx context.Context, requestID string, around time.Time) (*service.GPTAutoActualUsage, error) {
	if strings.TrimSpace(requestID) == "" {
		return nil, nil
	}
	var usage service.GPTAutoActualUsage
	err := r.db.QueryRowContext(ctx, `SELECT model, COALESCE(requested_model, ''), COALESCE(upstream_model, ''),
  COALESCE(reasoning_effort, ''), input_tokens, output_tokens, cache_read_tokens,
  total_cost::float8, actual_cost::float8,
  COALESCE(account_stats_cost, total_cost * COALESCE(account_rate_multiplier, 1))::float8, created_at
FROM usage_logs
WHERE request_id = $1 AND created_at BETWEEN $2 AND $3
ORDER BY created_at
LIMIT 1`, requestID, around.Add(-gptAutoActualUsageWindow), around.Add(gptAutoActualUsageWindow)).Scan(
		&usage.Model, &usage.RequestedModel, &usage.UpstreamModel, &usage.ReasoningEffort,
		&usage.InputTokens, &usage.OutputTokens, &usage.CacheReadTokens,
		&usage.ListCost, &usage.ChargedCost, &usage.AccountCost, &usage.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	usage.CreatedAt = usage.CreatedAt.UTC()
	return &usage, nil
}

func (r *gptAutoShadowRepository) OutputHistory(ctx context.Context, models []string, since time.Time) ([]service.GPTAutoOutputHistory, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT model, COALESCE(reasoning_effort, ''), AVG(output_tokens)::float8, COUNT(*)
FROM usage_logs
WHERE created_at >= $1 AND model = ANY($2) AND output_tokens > 0
GROUP BY 1, 2`, since, pq.Array(models))
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	var out []service.GPTAutoOutputHistory
	for rows.Next() {
		var row service.GPTAutoOutputHistory
		if err := rows.Scan(&row.Model, &row.Effort, &row.AvgOutput, &row.Samples); err != nil {
			return nil, err
		}
		out = append(out, row)
	}
	return out, rows.Err()
}
