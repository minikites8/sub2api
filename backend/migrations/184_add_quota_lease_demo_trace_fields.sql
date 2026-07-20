ALTER TABLE quota_lease_demo_leases
    ADD COLUMN IF NOT EXISTS trace_id TEXT NOT NULL DEFAULT '';

ALTER TABLE quota_lease_demo_ledger_events
    ADD COLUMN IF NOT EXISTS trace_id TEXT NOT NULL DEFAULT '';

ALTER TABLE quota_lease_demo_pending_usage_events
    ADD COLUMN IF NOT EXISTS trace_id TEXT NOT NULL DEFAULT '';

CREATE INDEX IF NOT EXISTS idx_quota_lease_demo_events_trace
    ON quota_lease_demo_ledger_events (trace_id)
    WHERE trace_id <> '';

CREATE INDEX IF NOT EXISTS idx_quota_lease_demo_leases_active_lookup
    ON quota_lease_demo_leases (node_id, user_id, api_key_id, expires_at)
    WHERE status = 'active';
