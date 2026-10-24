package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/lib/pq"
)

var _ service.GPTAutoExportRepository = (*gptAutoShadowRepository)(nil)

// gptAutoExportWhere builds the shared WHERE clause. Every value is a bind
// parameter; column names are fixed strings.
func gptAutoExportWhere(filter service.GPTAutoExportFilter) *gptAutoWhere {
	where := &gptAutoWhere{}
	where.add("created_at >= $%d", filter.StartTime)
	where.add("created_at < $%d", filter.EndTime)
	if filter.UserID != nil {
		where.add("user_id = $%d", *filter.UserID)
	}
	if filter.Mode != "" {
		where.add("mode = $%d", filter.Mode)
	}
	if filter.GroupID != nil {
		where.add("group_id = $%d", *filter.GroupID)
	}
	for _, item := range []struct{ column, value string }{
		{"actual_model", filter.RequestedModel},
		{"jev_model", filter.JevModel},
		{"final_model", filter.FinalModel},
		{"final_effort", filter.FinalEffort},
		{"category", filter.Category},
		{"request_status", filter.RequestStatus},
		{"evaluation_status", filter.EvaluationStatus},
		{"classifier_schema", filter.ClassifierSchema},
		{"state_version", filter.StateVersion},
		{"policy_version", filter.PolicyVersion},
	} {
		if item.value != "" {
			where.add(item.column+" = $%d", item.value)
		}
	}
	if filter.ComplexityMin != nil {
		where.add("complexity >= $%d", *filter.ComplexityMin)
	}
	if filter.ComplexityMax != nil {
		where.add("complexity <= $%d", *filter.ComplexityMax)
	}
	if filter.ConfidenceMin != nil {
		where.add("model_confidence >= $%d", *filter.ConfidenceMin)
	}
	if filter.ConfidenceMax != nil {
		where.add("model_confidence <= $%d", *filter.ConfidenceMax)
	}
	if filter.FallbackOnly {
		where.clauses = append(where.clauses, "(routing_reason = 'fallback' OR fallback_reason <> '' OR retry_count > 0)")
	}
	if filter.UpstreamAccount != nil {
		where.add("upstream_account_id = $%d", *filter.UpstreamAccount)
	}
	return where
}

func (r *gptAutoShadowRepository) ExportDecisions(ctx context.Context, filter service.GPTAutoExportFilter, cursorTime *time.Time, cursorID int64, limit int) ([]service.GPTAutoShadowDecision, error) {
	where := gptAutoExportWhere(filter)
	if cursorTime != nil {
		where.args = append(where.args, *cursorTime, cursorID)
		where.clauses = append(where.clauses, fmt.Sprintf("(created_at, id) < ($%d, $%d)", len(where.args)-1, len(where.args)))
	}
	where.args = append(where.args, limit)
	query := `SELECT ` + gptAutoDecisionColumns + `, features, classification, decision FROM gpt_auto_shadow_decisions` +
		where.sql() + fmt.Sprintf(` ORDER BY created_at DESC, id DESC LIMIT $%d`, len(where.args))
	rows, err := r.db.QueryContext(ctx, query, where.args...)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	out := make([]service.GPTAutoShadowDecision, 0, limit)
	for rows.Next() {
		var features, classification, decision []byte
		d, err := scanGPTAutoDecision(rows, &features, &classification, &decision)
		if err != nil {
			return nil, err
		}
		gptAutoDecodeDecisionJSON(d, features, classification, decision)
		out = append(out, *d)
	}
	return out, rows.Err()
}

func gptAutoDecodeDecisionJSON(d *service.GPTAutoShadowDecision, features, classification, decision []byte) {
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
}

func (r *gptAutoShadowRepository) CountDecisions(ctx context.Context, filter service.GPTAutoExportFilter, limit int64) (int64, error) {
	where := gptAutoExportWhere(filter)
	where.args = append(where.args, limit)
	var count int64
	err := r.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM (SELECT 1 FROM gpt_auto_shadow_decisions`+where.sql()+
		fmt.Sprintf(` LIMIT $%d) AS capped`, len(where.args)), where.args...).Scan(&count)
	return count, err
}

// ExportUsage looks up one batch of usage rows through the
// (request_id, api_key_id) unique index, bounded by a time window.
func (r *gptAutoShadowRepository) ExportUsage(ctx context.Context, keys []service.GPTAutoExportUsageKey, from, to time.Time) (map[service.GPTAutoExportUsageKey]*service.GPTAutoExportUsage, error) {
	out := make(map[service.GPTAutoExportUsageKey]*service.GPTAutoExportUsage, len(keys))
	if len(keys) == 0 {
		return out, nil
	}
	requestIDs := make([]string, len(keys))
	apiKeyIDs := make([]int64, len(keys))
	for i, key := range keys {
		requestIDs[i], apiKeyIDs[i] = key.RequestID, key.APIKeyID
	}
	rows, err := r.db.QueryContext(ctx, `SELECT u.request_id, u.api_key_id, u.model, COALESCE(u.requested_model, ''),
  COALESCE(u.upstream_model, ''), COALESCE(u.reasoning_effort, ''), u.input_tokens, u.output_tokens,
  u.cache_read_tokens, u.cache_creation_tokens, u.total_cost::float8, u.actual_cost::float8,
  COALESCE(u.account_stats_cost, u.total_cost * COALESCE(u.account_rate_multiplier, 1))::float8,
  u.rate_multiplier::float8, u.channel_id, u.created_at
FROM usage_logs u
JOIN unnest($1::text[], $2::bigint[]) AS k(request_id, api_key_id)
  ON u.request_id = k.request_id AND u.api_key_id = k.api_key_id
WHERE u.created_at BETWEEN $3 AND $4`, pq.Array(requestIDs), pq.Array(apiKeyIDs), from, to)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		var (
			key     service.GPTAutoExportUsageKey
			usage   service.GPTAutoExportUsage
			channel sql.NullInt64
		)
		if err := rows.Scan(&key.RequestID, &key.APIKeyID, &usage.Model, &usage.RequestedModel, &usage.UpstreamModel,
			&usage.ReasoningEffort, &usage.InputTokens, &usage.OutputTokens, &usage.CacheReadTokens,
			&usage.CacheCreationTokens, &usage.ListCost, &usage.ChargedCost, &usage.AccountCost,
			&usage.RateMultiplier, &channel, &usage.CreatedAt); err != nil {
			return nil, err
		}
		usage.ChannelID = gptAutoNullInt64(channel)
		usage.CreatedAt = usage.CreatedAt.UTC()
		if _, exists := out[key]; !exists {
			value := usage
			out[key] = &value
		}
	}
	return out, rows.Err()
}

func (r *gptAutoShadowRepository) InsertExportAudit(ctx context.Context, audit service.GPTAutoExportAudit) error {
	filters, err := json.Marshal(audit.Filters)
	if err != nil {
		return err
	}
	_, err = r.db.ExecContext(ctx, `INSERT INTO gpt_auto_export_audit
  (actor_id, scope, format, filters, row_count, truncated, status, duration_ms)
VALUES ($1, $2, $3, $4::jsonb, $5, $6, $7, $8)`,
		audit.ActorID, truncateGPTAuto(audit.Scope, 16), truncateGPTAuto(audit.Format, 8), string(filters),
		audit.RowCount, audit.Truncated, truncateGPTAuto(strings.TrimSpace(audit.Status), 16), audit.DurationMS)
	return err
}
