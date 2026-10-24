package service

import (
	"bytes"
	"context"
	"encoding/csv"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// gptAutoTestRepo doubles as an export repository: it applies the filters
// that matter for scope (user, mode, time) and the keyset cursor.
func (r *gptAutoTestRepo) ExportDecisions(_ context.Context, filter GPTAutoExportFilter, cursorTime *time.Time, cursorID int64, limit int) ([]GPTAutoShadowDecision, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.exportFilters = append(r.exportFilters, filter)
	var out []GPTAutoShadowDecision
	// rows are appended in time order; export is newest first.
	for i := len(r.rows) - 1; i >= 0 && len(out) < limit; i-- {
		row := r.rows[i]
		if row.CreatedAt.Before(filter.StartTime) || !row.CreatedAt.Before(filter.EndTime) {
			continue
		}
		if filter.UserID != nil && (row.UserID == nil || *row.UserID != *filter.UserID) {
			continue
		}
		if filter.Mode != "" && gptAutoRowMode(&row) != filter.Mode {
			continue
		}
		if cursorTime != nil && !(row.CreatedAt.Before(*cursorTime) || (row.CreatedAt.Equal(*cursorTime) && row.ID < cursorID)) {
			continue
		}
		out = append(out, row)
	}
	return out, nil
}

func (r *gptAutoTestRepo) CountDecisions(ctx context.Context, filter GPTAutoExportFilter, limit int64) (int64, error) {
	rows, _ := r.ExportDecisions(ctx, filter, nil, 0, int(limit))
	return int64(len(rows)), nil
}

func (r *gptAutoTestRepo) ExportUsage(_ context.Context, keys []GPTAutoExportUsageKey, _, _ time.Time) (map[GPTAutoExportUsageKey]*GPTAutoExportUsage, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := map[GPTAutoExportUsageKey]*GPTAutoExportUsage{}
	for _, key := range keys {
		if usage, ok := r.exportUsage[key]; ok {
			out[key] = usage
		}
	}
	return out, nil
}

func (r *gptAutoTestRepo) InsertExportAudit(_ context.Context, audit GPTAutoExportAudit) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.audits = append(r.audits, audit)
	return nil
}

func gptAutoExportFixture(t *testing.T) (*GPTAutoShadowService, *gptAutoTestRepo, time.Time) {
	t.Helper()
	svc, repo, _, _ := newGPTAutoTestService(t, nil)
	base := time.Date(2026, 10, 20, 12, 0, 0, 0, time.UTC)
	user1, user2, key1, key2 := int64(1), int64(2), int64(10), int64(20)
	multiplier, weight, status, latency := 1.5, 90, 200, 1234
	complexity, confidence := 3.2, 0.81
	repo.rows = []GPTAutoShadowDecision{
		{ID: 1, CreatedAt: base, Mode: "", RequestID: "shadow-old", UserID: &user1, APIKeyID: &key1,
			Endpoint: GPTAutoEndpointChatCompletions, ActualModel: GPTAutoModelGPT56Sol, JevModel: GPTAutoModelGPT56Sol,
			FinalModel: GPTAutoModelGPT56Sol, EvaluationStatus: GPTAutoEvaluationSuccess, PolicyVersion: "shadow-v1",
			EstRevenue: float64Ptr(0.01), EstUpstreamCost: float64Ptr(0.004)},
		{ID: 2, CreatedAt: base.Add(time.Minute), Mode: GPTAutoModeLive, RequestID: "live-1", UserID: &user1, APIKeyID: &key1,
			Endpoint: GPTAutoEndpointResponses, ActualModel: GPTAutoVirtualModel, JevModel: GPTAutoModelGPT61Sol,
			FinalModel: GPTAutoModelGPT61Sol, FinalEffort: GPTAutoEffortHigh, Category: GPTAutoCategoryCoding,
			Complexity: &complexity, ModelConfidence: &confidence,
			ClassifierCost: 0.0000126, ClassifierInputTokens: 280, ClassifierOutputTokens: 20, ClassifierLatencyMS: 410, Attempts: 1,
			RoutingReason: GPTAutoRouteClassified, RequestStatus: GPTAutoRequestSucceeded, HTTPStatus: &status,
			BillingMultiplier: &multiplier, RoutingWeight: &weight, TotalLatencyMS: &latency,
			StateVersion: GPTAutoClassifierStateVersion, ClassifierSchema: GPTAutoClassifierSchemaVersion,
			Classification: &GPTAutoClassification{ModelProbabilities: map[string]float64{GPTAutoModelGPT61Sol: 0.7, GPTAutoModelGPT6Luna: 0.3}},
			Features:       GPTAutoRequestFeatures{Context: &GPTAutoContextInfo{StateVersion: GPTAutoClassifierStateVersion}}},
		{ID: 3, CreatedAt: base.Add(2 * time.Minute), Mode: GPTAutoModeLive, RequestID: "=HYPERLINK(\"http://evil\")", UserID: &user2, APIKeyID: &key2,
			Endpoint: GPTAutoEndpointChatCompletions, ActualModel: GPTAutoVirtualModel, FinalModel: GPTAutoModelGPT6Luna,
			RoutingReason: GPTAutoRouteFallback, FallbackReason: GPTAutoErrAuth, RequestStatus: GPTAutoRequestSucceeded},
	}
	repo.exportUsage = map[GPTAutoExportUsageKey]*GPTAutoExportUsage{
		{RequestID: "live-1", APIKeyID: 10}: {GPTAutoActualUsage: GPTAutoActualUsage{
			Model: GPTAutoModelGPT61Sol, InputTokens: 1000, CacheReadTokens: 200, OutputTokens: 500,
			ListCost: 0.01, ChargedCost: 0.015, AccountCost: 0.02}},
	}
	return svc, repo, base
}

func runGPTAutoExport(t *testing.T, svc *GPTAutoShadowService, req GPTAutoExportRequest) (string, GPTAutoExportSummary) {
	t.Helper()
	release, err := svc.PrepareExport(context.Background(), &req)
	require.NoError(t, err)
	defer release()
	var buf bytes.Buffer
	summary, err := svc.Export(context.Background(), req, &buf)
	require.NoError(t, err)
	return buf.String(), summary
}

func gptAutoExportWindow(base time.Time) GPTAutoExportFilter {
	return GPTAutoExportFilter{StartTime: base.Add(-time.Hour), EndTime: base.Add(time.Hour)}
}

func parseGPTAutoCSV(t *testing.T, raw string) []map[string]string {
	t.Helper()
	require.True(t, strings.HasPrefix(raw, "\ufeff"), "UTF-8 BOM for Excel")
	records, err := csv.NewReader(strings.NewReader(strings.TrimPrefix(raw, "\ufeff"))).ReadAll()
	require.NoError(t, err)
	out := make([]map[string]string, 0, len(records)-1)
	for _, record := range records[1:] {
		row := map[string]string{}
		for i, column := range records[0] {
			row[column] = record[i]
		}
		out = append(out, row)
	}
	return out
}

func TestGPTAutoAdminCSVExport(t *testing.T) {
	svc, repo, base := gptAutoExportFixture(t)
	raw, summary := runGPTAutoExport(t, svc, GPTAutoExportRequest{Scope: GPTAutoExportScopeAdmin, Format: GPTAutoExportFormatCSV, ActorID: 1, Filter: gptAutoExportWindow(base)})
	require.Equal(t, int64(3), summary.Rows)
	rows := parseGPTAutoCSV(t, raw)
	require.Len(t, rows, 3)

	header := strings.SplitN(strings.TrimPrefix(raw, "\ufeff"), "\r\n", 2)[0]
	require.Equal(t, strings.Join(gptAutoAdminExportColumns, ","), header, "stable column order")

	// Newest first.
	require.Equal(t, "'=HYPERLINK(\"http://evil\")", rows[0]["request_id"], "formula injection is neutralised")
	live := rows[1]
	require.Equal(t, GPTAutoExportSchemaVersion, live["export_schema"])
	require.Equal(t, "2026-10-20T12:01:00Z", live["timestamp"])
	require.Equal(t, "live", live["mode"])
	require.Equal(t, "gpt-auto", live["requested_model"])
	require.Equal(t, "gpt-6.1-sol", live["final_model"])
	require.Equal(t, "1000", live["input_tokens"])
	require.Equal(t, "200", live["cached_input_tokens"])
	require.Equal(t, "500", live["output_tokens"])
	require.Equal(t, "300", live["classifier_tokens"])
	require.Equal(t, "0.0150000000", live["customer_charge_usd"])
	require.Equal(t, "0.0200000000", live["upstream_cost_usd"])
	require.Equal(t, "-0.0050000000", live["gross_profit_usd"], "a loss is kept negative, never clamped")
	require.Equal(t, "-0.333333", live["gross_margin"])
	require.Equal(t, "-0.0050126000", live["fully_loaded_profit_usd"])
	require.Equal(t, "final", live["billing_status"])
	require.Equal(t, "actual_usage", live["economics_basis"])
	require.Equal(t, "410", live["classifier_latency_ms"])
	require.Equal(t, "3.200000", live["complexity"])
	require.Contains(t, live["model_probabilities"], `"gpt-6.1-sol":0.7`)

	shadow := rows[2]
	require.Equal(t, "shadow", shadow["mode"], "pre-Phase-2 rows read as shadow")
	require.Equal(t, GPTAutoModelGPT56Sol, shadow["final_model"], "historical retired-model decisions stay exportable")
	require.Equal(t, "", shadow["customer_charge_usd"], "unknown money is empty, never zero")
	require.Equal(t, "", shadow["classifier_latency_ms"], "no Jev call, no latency")
	require.Equal(t, "hypothetical_estimate", shadow["economics_basis"])
	require.Equal(t, "0.0100000000", shadow["est_revenue_usd"])

	require.Len(t, repo.audits, 1)
	require.Equal(t, GPTAutoExportScopeAdmin, repo.audits[0].Scope)
	require.Equal(t, int64(3), repo.audits[0].RowCount)
	require.Equal(t, "completed", repo.audits[0].Status)
}

func TestGPTAutoAdminJSONLExport(t *testing.T) {
	svc, _, base := gptAutoExportFixture(t)
	raw, _ := runGPTAutoExport(t, svc, GPTAutoExportRequest{Scope: GPTAutoExportScopeAdmin, Format: GPTAutoExportFormatJSONL, ActorID: 1, Filter: gptAutoExportWindow(base)})
	lines := strings.Split(strings.TrimSpace(raw), "\n")
	require.Len(t, lines, 3)
	var live map[string]any
	require.NoError(t, json.Unmarshal([]byte(lines[1]), &live))
	probabilities, ok := live["model_probabilities"].(map[string]any)
	require.True(t, ok, "JSONL keeps nested probabilities structured")
	require.InDelta(t, 0.7, probabilities[GPTAutoModelGPT61Sol], 1e-9)
	diagnostics, ok := live["context_diagnostics"].(map[string]any)
	require.True(t, ok)
	require.Equal(t, GPTAutoClassifierStateVersion, diagnostics["state_version"])
	require.Nil(t, live["served_model"], "missing values are null")
	require.Equal(t, 1000.0, live["input_tokens"])
	// Key order follows the documented columns.
	require.True(t, strings.HasPrefix(lines[0], `{"export_schema":"`+GPTAutoExportSchemaVersion+`","timestamp":`))
	var injected map[string]any
	require.NoError(t, json.Unmarshal([]byte(lines[0]), &injected))
	require.Equal(t, `=HYPERLINK("http://evil")`, injected["request_id"], "JSON values are not mangled")
}

func TestGPTAutoCustomerExportIsScopedAndSafe(t *testing.T) {
	svc, repo, base := gptAutoExportFixture(t)
	user := int64(1)
	filter := gptAutoExportWindow(base)
	filter.UserID = &user
	filter.Mode = GPTAutoModeShadow // a customer cannot widen scope to shadow rows
	raw, summary := runGPTAutoExport(t, svc, GPTAutoExportRequest{Scope: GPTAutoExportScopeCustomer, Format: GPTAutoExportFormatCSV, ActorID: user, Filter: filter})
	require.Equal(t, int64(1), summary.Rows, "only the caller's own live rows")
	rows := parseGPTAutoCSV(t, raw)
	require.Equal(t, "live-1", rows[0]["request_id"])
	require.Equal(t, "gpt-6.1-sol", rows[0]["executed_model"])
	require.Equal(t, "0.0150000000", rows[0]["charge_usd"])
	require.Equal(t, GPTAutoModeLive, repo.exportFilters[0].Mode)

	header := strings.SplitN(strings.TrimPrefix(raw, "\ufeff"), "\r\n", 2)[0]
	for _, forbidden := range []string{"upstream_cost", "gross_profit", "margin", "upstream_account", "channel", "user_id",
		"classifier", "fallback_reason", "probabilities", "diagnostics", "jev_"} {
		require.NotContains(t, header, forbidden, "customer schema must not expose %s", forbidden)
	}
	require.NotContains(t, raw, "0.0200000000", "procurement cost never appears")

	// The other customer's fallback row hides the internal failure class.
	other := int64(2)
	filter.UserID = &other
	raw, _ = runGPTAutoExport(t, svc, GPTAutoExportRequest{Scope: GPTAutoExportScopeCustomer, Format: GPTAutoExportFormatJSONL, ActorID: other, Filter: filter})
	require.NotContains(t, raw, GPTAutoErrAuth)
	require.Contains(t, raw, `"fallback":true`)
	require.NotContains(t, raw, "live-1")
}

func TestGPTAutoCustomerExportRequiresIdentityAndSwitch(t *testing.T) {
	svc, _, base := gptAutoExportFixture(t)
	req := GPTAutoExportRequest{Scope: GPTAutoExportScopeCustomer, Format: GPTAutoExportFormatCSV, ActorID: 1, Filter: gptAutoExportWindow(base)}
	_, err := svc.PrepareExport(context.Background(), &req)
	require.Error(t, err, "a customer export without a server-set user id is refused")

	cfg := svc.settings.GetGPTAutoShadowConfigCached(context.Background())
	cfg.Live.CustomerExportEnabled = false
	require.NoError(t, svc.settings.SetGPTAutoShadowConfig(context.Background(), &cfg))
	user := int64(1)
	req.Filter.UserID = &user
	_, err = svc.PrepareExport(context.Background(), &req)
	require.ErrorIs(t, err, errGPTAutoExportDisabled)
	// The admin export is controlled independently.
	admin := GPTAutoExportRequest{Scope: GPTAutoExportScopeAdmin, Format: GPTAutoExportFormatCSV, ActorID: 1, Filter: gptAutoExportWindow(base)}
	release, err := svc.PrepareExport(context.Background(), &admin)
	require.NoError(t, err)
	release()
}

func TestGPTAutoExportValidation(t *testing.T) {
	svc, _, base := gptAutoExportFixture(t)
	for name, mutate := range map[string]func(*GPTAutoExportRequest){
		"format": func(r *GPTAutoExportRequest) { r.Format = "xlsx" },
		"inverted window": func(r *GPTAutoExportRequest) {
			r.Filter.StartTime, r.Filter.EndTime = r.Filter.EndTime, r.Filter.StartTime
		},
		"window too long": func(r *GPTAutoExportRequest) { r.Filter.StartTime = r.Filter.EndTime.AddDate(-2, 0, 0) },
		"mode":            func(r *GPTAutoExportRequest) { r.Filter.Mode = "all" },
		"effort":          func(r *GPTAutoExportRequest) { r.Filter.FinalEffort = "ultra" },
		"range": func(r *GPTAutoExportRequest) {
			lo, hi := 3.0, 1.0
			r.Filter.ComplexityMin, r.Filter.ComplexityMax = &lo, &hi
		},
	} {
		t.Run(name, func(t *testing.T) {
			req := GPTAutoExportRequest{Scope: GPTAutoExportScopeAdmin, Format: GPTAutoExportFormatCSV, ActorID: 1, Filter: gptAutoExportWindow(base)}
			mutate(&req)
			_, err := svc.PrepareExport(context.Background(), &req)
			require.Error(t, err)
		})
	}
}

func TestGPTAutoExportPaginatesAndTruncates(t *testing.T) {
	svc, repo, _, _ := newGPTAutoTestService(t, nil)
	base := time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)
	user := int64(5)
	total := gptAutoExportBatchSize*2 + 37
	for i := 0; i < total; i++ {
		// Pairs of rows share a timestamp to exercise the (created_at, id) cursor.
		repo.rows = append(repo.rows, GPTAutoShadowDecision{ID: int64(i + 1), CreatedAt: base.Add(time.Duration(i/2) * time.Second),
			Mode: GPTAutoModeLive, UserID: &user, RequestID: "r"})
	}
	filter := GPTAutoExportFilter{StartTime: base, EndTime: base.Add(24 * time.Hour)}
	raw, summary := runGPTAutoExport(t, svc, GPTAutoExportRequest{Scope: GPTAutoExportScopeAdmin, Format: GPTAutoExportFormatJSONL, ActorID: 1, Filter: filter})
	require.Equal(t, int64(total), summary.Rows)
	require.False(t, summary.Truncated)
	seen := map[float64]bool{}
	for _, line := range strings.Split(strings.TrimSpace(raw), "\n") {
		var row map[string]any
		require.NoError(t, json.Unmarshal([]byte(line), &row))
		id := row["decision_id"].(float64)
		require.False(t, seen[id], "no duplicate across pages")
		seen[id] = true
	}
	require.Len(t, seen, total, "no row lost across pages")

	count, err := svc.CountExport(context.Background(), GPTAutoExportScopeAdmin, filter)
	require.NoError(t, err)
	require.Equal(t, int64(total), count.Rows)

	// Customer scope truncates at its row cap and says so.
	repo.rows = repo.rows[:0]
	for i := 0; i < 50; i++ {
		repo.rows = append(repo.rows, GPTAutoShadowDecision{ID: int64(i + 1), CreatedAt: base.Add(time.Duration(i) * time.Second), Mode: GPTAutoModeLive, UserID: &user})
	}
	out := gptAutoTruncatedExport(t, svc, repo, filter, user, 20)
	require.Equal(t, int64(20), out.Rows)
	require.True(t, out.Truncated)
}

// gptAutoTruncatedExport runs a customer export with a lowered row cap.
func gptAutoTruncatedExport(t *testing.T, svc *GPTAutoShadowService, repo *gptAutoTestRepo, filter GPTAutoExportFilter, user int64, cap int64) GPTAutoExportSummary {
	t.Helper()
	previous := gptAutoExportCustomerRowCap
	gptAutoExportCustomerRowCap = cap
	defer func() { gptAutoExportCustomerRowCap = previous }()
	filter.UserID = &user
	_, summary := runGPTAutoExport(t, svc, GPTAutoExportRequest{Scope: GPTAutoExportScopeCustomer, Format: GPTAutoExportFormatCSV, ActorID: user, Filter: filter})
	_ = repo
	return summary
}

func TestGPTAutoExportConcurrencyLimit(t *testing.T) {
	svc, _, base := gptAutoExportFixture(t)
	req := GPTAutoExportRequest{Scope: GPTAutoExportScopeAdmin, Format: GPTAutoExportFormatCSV, ActorID: 77, Filter: gptAutoExportWindow(base)}
	release, err := svc.PrepareExport(context.Background(), &req)
	require.NoError(t, err)
	_, err = svc.PrepareExport(context.Background(), &req)
	require.ErrorIs(t, err, errGPTAutoExportBusy, "one export at a time per actor")
	release()
	release, err = svc.PrepareExport(context.Background(), &req)
	require.NoError(t, err)
	release()
}

func TestGPTAutoExportCancellation(t *testing.T) {
	svc, repo, base := gptAutoExportFixture(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	req := GPTAutoExportRequest{Scope: GPTAutoExportScopeAdmin, Format: GPTAutoExportFormatCSV, ActorID: 3, Filter: gptAutoExportWindow(base)}
	release, err := svc.PrepareExport(context.Background(), &req)
	require.NoError(t, err)
	defer release()
	var buf bytes.Buffer
	_, err = svc.Export(ctx, req, &buf)
	require.ErrorIs(t, err, context.Canceled)
	require.Equal(t, "cancelled", repo.audits[len(repo.audits)-1].Status)
}

func TestGPTAutoCSVFieldEscaping(t *testing.T) {
	for input, want := range map[string]string{
		"plain":       "plain",
		"=1+1":        `"'=1+1"`,
		"+cmd":        `"'+cmd"`,
		"-2":          `"'-2"`,
		"@SUM(A1)":    `"'@SUM(A1)"`,
		"\tx":         "\"'\tx\"",
		"a,b":         `"a,b"`,
		`say "hi"`:    `"say ""hi"""`,
		"line\nbreak": "\"line\nbreak\"",
	} {
		require.Equal(t, want, gptAutoCSVField(cellString(input)), input)
	}
	require.Equal(t, "-0.0050000000", gptAutoCSVField(gptAutoExportCell{kind: gptAutoCellNumber, value: "-0.0050000000"}), "numbers are never prefixed")
	require.Equal(t, "", gptAutoCSVField(gptAutoExportCell{}))
}
