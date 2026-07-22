package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
)

type quotaLeaseDemoPersistenceStore struct {
	db *sql.DB
}

func NewQuotaLeaseDemoPersistenceStore(sqlDB *sql.DB) service.QuotaLeaseDemoPersistenceStore {
	if sqlDB == nil {
		return nil
	}
	return &quotaLeaseDemoPersistenceStore{db: sqlDB}
}

func (s *quotaLeaseDemoPersistenceStore) LoadQuotaLeaseDemoState(ctx context.Context) (service.QuotaLeaseDemoPersistedState, error) {
	if s == nil || s.db == nil {
		return service.QuotaLeaseDemoPersistedState{}, nil
	}
	nodes, err := s.loadNodes(ctx)
	if err != nil {
		return service.QuotaLeaseDemoPersistedState{}, err
	}
	leases, err := s.loadLeases(ctx)
	if err != nil {
		return service.QuotaLeaseDemoPersistedState{}, err
	}
	events, err := s.loadLedgerEvents(ctx)
	if err != nil {
		return service.QuotaLeaseDemoPersistedState{}, err
	}
	pending, err := s.loadPendingUsageEvents(ctx)
	if err != nil {
		return service.QuotaLeaseDemoPersistedState{}, err
	}
	return service.QuotaLeaseDemoPersistedState{
		Nodes:              nodes,
		Leases:             leases,
		Events:             events,
		PendingUsageEvents: pending,
	}, nil
}

func (s *quotaLeaseDemoPersistenceStore) SaveQuotaLeaseDemoNode(ctx context.Context, node service.QuotaLeaseDemoNode) error {
	if s == nil || s.db == nil || strings.TrimSpace(node.NodeID) == "" {
		return nil
	}
	metadata, err := json.Marshal(nonNilStringMap(node.Metadata))
	if err != nil {
		return err
	}
	metrics, err := json.Marshal(nonNilFloatMap(node.Metrics))
	if err != nil {
		return err
	}
	syncStatus, err := marshalNullableJSON(node.SyncStatus)
	if err != nil {
		return err
	}
	var lastHeartbeat any
	if node.LastHeartbeatAt != nil && !node.LastHeartbeatAt.IsZero() {
		lastHeartbeat = node.LastHeartbeatAt.UTC()
	}
	_, err = s.db.ExecContext(ctx, `
		INSERT INTO quota_lease_demo_nodes (
			node_id, secret, region, base_url, public_key, metadata, status,
			inflight_requests, lease_remaining, metrics, sync_status,
			registered_at, last_heartbeat_at, updated_at
		) VALUES (
			$1, $2, $3, $4, $5, $6::jsonb, $7,
			$8, $9, $10::jsonb, $11::jsonb,
			$12, $13, $14
		)
		ON CONFLICT (node_id) DO UPDATE SET
			secret = EXCLUDED.secret,
			region = EXCLUDED.region,
			base_url = EXCLUDED.base_url,
			public_key = EXCLUDED.public_key,
			metadata = EXCLUDED.metadata,
			status = EXCLUDED.status,
			inflight_requests = EXCLUDED.inflight_requests,
			lease_remaining = EXCLUDED.lease_remaining,
			metrics = EXCLUDED.metrics,
			sync_status = EXCLUDED.sync_status,
			registered_at = EXCLUDED.registered_at,
			last_heartbeat_at = EXCLUDED.last_heartbeat_at,
			updated_at = EXCLUDED.updated_at
	`, node.NodeID, node.Secret, node.Region, node.BaseURL, node.PublicKey, string(metadata), node.Status,
		node.InflightRequests, node.LeaseRemaining, string(metrics), syncStatus,
		node.RegisteredAt, lastHeartbeat, node.UpdatedAt)
	return err
}

func (s *quotaLeaseDemoPersistenceStore) SaveQuotaLeaseDemoLease(ctx context.Context, lease service.QuotaLeaseDemoLease) error {
	if s == nil || s.db == nil || strings.TrimSpace(lease.ID) == "" {
		return nil
	}
	return upsertQuotaLeaseDemoLease(ctx, s.db, lease)
}

func (s *quotaLeaseDemoPersistenceStore) SaveQuotaLeaseDemoLedgerEvent(ctx context.Context, event service.QuotaLeaseDemoLedgerEvent) error {
	if s == nil || s.db == nil || strings.TrimSpace(event.EventID) == "" {
		return nil
	}
	return upsertQuotaLeaseDemoLedgerEvent(ctx, s.db, event)
}

func (s *quotaLeaseDemoPersistenceStore) SaveQuotaLeaseDemoPendingUsageEvent(ctx context.Context, event service.QuotaLeaseDemoUsageEvent) error {
	if s == nil || s.db == nil || strings.TrimSpace(event.EventID) == "" {
		return nil
	}
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO quota_lease_demo_pending_usage_events (
			event_id, lease_id, node_id, user_id, api_key_id, request_id,
			trace_id, amount, event_type, created_at, updated_at
		) VALUES (
			$1, $2, $3, $4, $5, $6,
			$7, $8, $9, $10, NOW()
		)
		ON CONFLICT (event_id) DO UPDATE SET
			lease_id = EXCLUDED.lease_id,
			node_id = EXCLUDED.node_id,
			user_id = EXCLUDED.user_id,
			api_key_id = EXCLUDED.api_key_id,
			request_id = EXCLUDED.request_id,
			trace_id = EXCLUDED.trace_id,
			amount = EXCLUDED.amount,
			event_type = EXCLUDED.event_type,
			created_at = EXCLUDED.created_at,
			updated_at = NOW()
	`, event.EventID, event.LeaseID, event.NodeID, event.UserID, event.APIKeyID, event.RequestID,
		event.TraceID, event.Amount, event.EventType, event.CreatedAt)
	return err
}

func (s *quotaLeaseDemoPersistenceStore) DeleteQuotaLeaseDemoPendingUsageEvent(ctx context.Context, eventID string) error {
	if s == nil || s.db == nil || strings.TrimSpace(eventID) == "" {
		return nil
	}
	_, err := s.db.ExecContext(ctx, `
		DELETE FROM quota_lease_demo_pending_usage_events
		WHERE event_id = $1
	`, strings.TrimSpace(eventID))
	return err
}

func (s *quotaLeaseDemoPersistenceStore) CleanupQuotaLeaseDemoRecords(ctx context.Context, cutoff time.Time, limit int) (service.QuotaLeaseDemoCleanupResult, error) {
	result := service.QuotaLeaseDemoCleanupResult{}
	if s == nil || s.db == nil || cutoff.IsZero() {
		return result, nil
	}
	if limit <= 0 {
		limit = 1000
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return result, err
	}
	defer func() {
		if tx != nil {
			_ = tx.Rollback()
		}
	}()

	if err := tx.QueryRowContext(ctx, `
		WITH candidate AS (
			SELECT id
			FROM quota_lease_demo_leases
			WHERE status IN ('closed', 'reclaimed')
				AND updated_at < $1
			ORDER BY updated_at ASC, id ASC
			LIMIT $2
		), deleted AS (
			DELETE FROM quota_lease_demo_ledger_events e
			USING candidate c
			WHERE e.lease_id = c.id
			RETURNING e.event_id
		)
		SELECT COUNT(*) FROM deleted
	`, cutoff.UTC(), limit).Scan(&result.LedgerEventCount); err != nil {
		return result, err
	}
	if err := tx.QueryRowContext(ctx, `
		WITH candidate AS (
			SELECT id
			FROM quota_lease_demo_leases
			WHERE status IN ('closed', 'reclaimed')
				AND updated_at < $1
			ORDER BY updated_at ASC, id ASC
			LIMIT $2
		), deleted AS (
			DELETE FROM quota_lease_demo_leases l
			USING candidate c
			WHERE l.id = c.id
			RETURNING l.id
		)
		SELECT COUNT(*) FROM deleted
	`, cutoff.UTC(), limit).Scan(&result.LeaseCount); err != nil {
		return result, err
	}
	if err := tx.Commit(); err != nil {
		return result, err
	}
	tx = nil
	return result, nil
}

func (r *usageBillingRepository) ApplyQuotaLeaseUsage(ctx context.Context, cmd *service.QuotaLeaseDemoUsageBillingCommand) (_ *service.UsageBillingApplyResult, err error) {
	if cmd == nil || cmd.Billing == nil {
		return &service.UsageBillingApplyResult{}, nil
	}
	if r == nil || r.db == nil {
		return nil, errors.New("usage billing repository db is nil")
	}

	cmd.Billing.Normalize()
	if cmd.Billing.RequestID == "" {
		return nil, service.ErrUsageBillingRequestIDRequired
	}

	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer func() {
		if tx != nil {
			_ = tx.Rollback()
		}
	}()

	applied, err := r.claimUsageBillingKey(ctx, tx, cmd.Billing)
	if err != nil {
		return nil, err
	}
	if !applied {
		return &service.UsageBillingApplyResult{Applied: false}, nil
	}

	result := &service.UsageBillingApplyResult{Applied: true}
	if err := r.applyUsageBillingEffects(ctx, tx, cmd.Billing, result); err != nil {
		return nil, err
	}
	if err := applyQuotaLeaseDemoLeaseUsage(ctx, tx, cmd.Lease, cmd.Event); err != nil {
		return nil, err
	}
	if err := upsertQuotaLeaseDemoLedgerEvent(ctx, tx, cmd.Event); err != nil {
		return nil, err
	}

	if err := tx.Commit(); err != nil {
		return nil, err
	}
	tx = nil
	return result, nil
}

func (s *quotaLeaseDemoPersistenceStore) loadNodes(ctx context.Context) ([]service.QuotaLeaseDemoNode, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT node_id, secret, region, base_url, public_key,
			metadata::text, status, inflight_requests, lease_remaining,
			metrics::text, sync_status::text, registered_at, last_heartbeat_at, updated_at
		FROM quota_lease_demo_nodes
		ORDER BY registered_at ASC, node_id ASC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var nodes []service.QuotaLeaseDemoNode
	for rows.Next() {
		var node service.QuotaLeaseDemoNode
		var metadataText, metricsText string
		var syncStatusText sql.NullString
		var lastHeartbeat sql.NullTime
		if err := rows.Scan(
			&node.NodeID,
			&node.Secret,
			&node.Region,
			&node.BaseURL,
			&node.PublicKey,
			&metadataText,
			&node.Status,
			&node.InflightRequests,
			&node.LeaseRemaining,
			&metricsText,
			&syncStatusText,
			&node.RegisteredAt,
			&lastHeartbeat,
			&node.UpdatedAt,
		); err != nil {
			return nil, err
		}
		node.Metadata = decodeStringMap(metadataText)
		node.Metrics = decodeFloatMap(metricsText)
		if syncStatusText.Valid {
			var status service.QuotaLeaseDemoNodeSyncStatus
			if err := json.Unmarshal([]byte(syncStatusText.String), &status); err != nil {
				return nil, err
			}
			node.SyncStatus = &status
		}
		if lastHeartbeat.Valid {
			t := lastHeartbeat.Time
			node.LastHeartbeatAt = &t
		}
		nodes = append(nodes, node)
	}
	return nodes, rows.Err()
}

func (s *quotaLeaseDemoPersistenceStore) loadLeases(ctx context.Context) ([]service.QuotaLeaseDemoLease, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, node_id, user_id, api_key_id, granted, consumed, reclaimed,
			version, trace_id, status, expires_at, reclaim_at, created_at, updated_at
		FROM quota_lease_demo_leases
		ORDER BY created_at ASC, id ASC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var leases []service.QuotaLeaseDemoLease
	for rows.Next() {
		var lease service.QuotaLeaseDemoLease
		if err := rows.Scan(
			&lease.ID,
			&lease.NodeID,
			&lease.UserID,
			&lease.APIKeyID,
			&lease.Granted,
			&lease.Consumed,
			&lease.Reclaimed,
			&lease.Version,
			&lease.TraceID,
			&lease.Status,
			&lease.ExpiresAt,
			&lease.ReclaimAt,
			&lease.CreatedAt,
			&lease.UpdatedAt,
		); err != nil {
			return nil, err
		}
		leases = append(leases, lease)
	}
	return leases, rows.Err()
}

func (s *quotaLeaseDemoPersistenceStore) loadLedgerEvents(ctx context.Context) ([]service.QuotaLeaseDemoLedgerEvent, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT event_id, lease_id, node_id, user_id, api_key_id, request_id,
			trace_id, amount, event_type, payload_hash, created_at
		FROM quota_lease_demo_ledger_events
		ORDER BY created_at ASC, event_id ASC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var events []service.QuotaLeaseDemoLedgerEvent
	for rows.Next() {
		var event service.QuotaLeaseDemoLedgerEvent
		if err := rows.Scan(
			&event.EventID,
			&event.LeaseID,
			&event.NodeID,
			&event.UserID,
			&event.APIKeyID,
			&event.RequestID,
			&event.TraceID,
			&event.Amount,
			&event.EventType,
			&event.PayloadHash,
			&event.CreatedAt,
		); err != nil {
			return nil, err
		}
		events = append(events, event)
	}
	return events, rows.Err()
}

func (s *quotaLeaseDemoPersistenceStore) loadPendingUsageEvents(ctx context.Context) ([]service.QuotaLeaseDemoUsageEvent, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT event_id, lease_id, node_id, user_id, api_key_id, request_id,
			trace_id, amount, event_type, created_at
		FROM quota_lease_demo_pending_usage_events
		ORDER BY created_at ASC, event_id ASC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var events []service.QuotaLeaseDemoUsageEvent
	for rows.Next() {
		var event service.QuotaLeaseDemoUsageEvent
		if err := rows.Scan(
			&event.EventID,
			&event.LeaseID,
			&event.NodeID,
			&event.UserID,
			&event.APIKeyID,
			&event.RequestID,
			&event.TraceID,
			&event.Amount,
			&event.EventType,
			&event.CreatedAt,
		); err != nil {
			return nil, err
		}
		events = append(events, event)
	}
	return events, rows.Err()
}

func upsertQuotaLeaseDemoLease(ctx context.Context, exec sqlExecutor, lease service.QuotaLeaseDemoLease) error {
	if exec == nil || strings.TrimSpace(lease.ID) == "" {
		return nil
	}
	_, err := exec.ExecContext(ctx, `
		INSERT INTO quota_lease_demo_leases (
			id, node_id, user_id, api_key_id, granted, consumed, reclaimed,
			version, trace_id, status, expires_at, reclaim_at, created_at, updated_at
		) VALUES (
			$1, $2, $3, $4, $5, $6, $7,
			$8, $9, $10, $11, $12, $13, $14
		)
		ON CONFLICT (id) DO UPDATE SET
			node_id = EXCLUDED.node_id,
			user_id = EXCLUDED.user_id,
			api_key_id = EXCLUDED.api_key_id,
			granted = EXCLUDED.granted,
			consumed = EXCLUDED.consumed,
			reclaimed = EXCLUDED.reclaimed,
			version = EXCLUDED.version,
			trace_id = EXCLUDED.trace_id,
			status = EXCLUDED.status,
			expires_at = EXCLUDED.expires_at,
			reclaim_at = EXCLUDED.reclaim_at,
			created_at = EXCLUDED.created_at,
			updated_at = EXCLUDED.updated_at
	`, lease.ID, lease.NodeID, lease.UserID, lease.APIKeyID, lease.Granted, lease.Consumed, lease.Reclaimed,
		lease.Version, lease.TraceID, lease.Status, lease.ExpiresAt, lease.ReclaimAt, lease.CreatedAt, lease.UpdatedAt)
	return err
}

func applyQuotaLeaseDemoLeaseUsage(ctx context.Context, tx *sql.Tx, lease service.QuotaLeaseDemoLease, event service.QuotaLeaseDemoLedgerEvent) error {
	if tx == nil || strings.TrimSpace(lease.ID) == "" {
		return nil
	}
	expectedVersion := lease.Version - 1
	if expectedVersion < 0 {
		expectedVersion = 0
	}
	result, err := tx.ExecContext(ctx, `
		UPDATE quota_lease_demo_leases
		SET
			node_id = $2,
			user_id = $3,
			api_key_id = $4,
			consumed = $5,
			reclaimed = $6,
			version = $7,
			trace_id = $8,
			status = $9,
			expires_at = $10,
			reclaim_at = $11,
			updated_at = $12
		WHERE id = $1 AND version = $13
	`, lease.ID, lease.NodeID, lease.UserID, lease.APIKeyID, lease.Consumed, lease.Reclaimed,
		lease.Version, lease.TraceID, lease.Status, lease.ExpiresAt, lease.ReclaimAt, lease.UpdatedAt, expectedVersion)
	if err != nil {
		return err
	}
	if rows, _ := result.RowsAffected(); rows > 0 {
		return nil
	}
	result, err = tx.ExecContext(ctx, `
		UPDATE quota_lease_demo_leases
		SET
			node_id = $2,
			user_id = $3,
			api_key_id = $4,
			consumed = quota_lease_demo_leases.consumed + $5,
			trace_id = $6,
			expires_at = GREATEST(quota_lease_demo_leases.expires_at, $7),
			reclaim_at = GREATEST(quota_lease_demo_leases.reclaim_at, $8),
			status = CASE
				WHEN ABS(quota_lease_demo_leases.granted - (quota_lease_demo_leases.consumed + $5) - quota_lease_demo_leases.reclaimed) <= 0.000000000001 THEN 'closed'
				ELSE 'active'
			END,
			updated_at = $9,
			version = quota_lease_demo_leases.version + 1
		WHERE id = $1
	`, lease.ID, lease.NodeID, lease.UserID, lease.APIKeyID, event.Amount, lease.TraceID, lease.ExpiresAt, lease.ReclaimAt, lease.UpdatedAt)
	if err != nil {
		return err
	}
	if rows, _ := result.RowsAffected(); rows > 0 {
		return nil
	}
	return upsertQuotaLeaseDemoLease(ctx, tx, lease)
}

func upsertQuotaLeaseDemoLedgerEvent(ctx context.Context, exec sqlExecutor, event service.QuotaLeaseDemoLedgerEvent) error {
	if exec == nil || strings.TrimSpace(event.EventID) == "" {
		return nil
	}
	_, err := exec.ExecContext(ctx, `
		INSERT INTO quota_lease_demo_ledger_events (
			event_id, lease_id, node_id, user_id, api_key_id, request_id,
			trace_id, amount, event_type, payload_hash, created_at
		) VALUES (
			$1, $2, $3, $4, $5, $6,
			$7, $8, $9, $10, $11
		)
		ON CONFLICT (event_id) DO UPDATE SET
			lease_id = EXCLUDED.lease_id,
			node_id = EXCLUDED.node_id,
			user_id = EXCLUDED.user_id,
			api_key_id = EXCLUDED.api_key_id,
			request_id = EXCLUDED.request_id,
			trace_id = EXCLUDED.trace_id,
			amount = EXCLUDED.amount,
			event_type = EXCLUDED.event_type,
			payload_hash = EXCLUDED.payload_hash,
			created_at = EXCLUDED.created_at
	`, event.EventID, event.LeaseID, event.NodeID, event.UserID, event.APIKeyID, event.RequestID,
		event.TraceID, event.Amount, event.EventType, event.PayloadHash, event.CreatedAt)
	return err
}

func marshalNullableJSON(value any) (any, error) {
	if value == nil {
		return nil, nil
	}
	payload, err := json.Marshal(value)
	if err != nil {
		return nil, err
	}
	return string(payload), nil
}

func nonNilStringMap(src map[string]string) map[string]string {
	if src == nil {
		return map[string]string{}
	}
	return src
}

func nonNilFloatMap(src map[string]float64) map[string]float64 {
	if src == nil {
		return map[string]float64{}
	}
	return src
}

func decodeStringMap(raw string) map[string]string {
	var out map[string]string
	if err := json.Unmarshal([]byte(raw), &out); err != nil || len(out) == 0 {
		return nil
	}
	return out
}

func decodeFloatMap(raw string) map[string]float64 {
	var out map[string]float64
	if err := json.Unmarshal([]byte(raw), &out); err != nil || len(out) == 0 {
		return nil
	}
	return out
}
