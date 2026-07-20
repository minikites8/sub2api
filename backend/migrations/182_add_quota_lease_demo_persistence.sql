CREATE TABLE IF NOT EXISTS quota_lease_demo_nodes (
    node_id TEXT PRIMARY KEY,
    secret TEXT NOT NULL DEFAULT '',
    region TEXT NOT NULL DEFAULT '',
    base_url TEXT NOT NULL DEFAULT '',
    public_key TEXT NOT NULL DEFAULT '',
    metadata JSONB NOT NULL DEFAULT '{}'::jsonb,
    status TEXT NOT NULL DEFAULT 'offline',
    inflight_requests INTEGER NOT NULL DEFAULT 0,
    lease_remaining DECIMAL(20,8) NOT NULL DEFAULT 0,
    metrics JSONB NOT NULL DEFAULT '{}'::jsonb,
    sync_status JSONB,
    registered_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    last_heartbeat_at TIMESTAMPTZ,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_quota_lease_demo_nodes_status_updated
    ON quota_lease_demo_nodes (status, updated_at DESC);

CREATE TABLE IF NOT EXISTS quota_lease_demo_leases (
    id TEXT PRIMARY KEY,
    node_id TEXT NOT NULL,
    user_id BIGINT NOT NULL,
    api_key_id BIGINT NOT NULL,
    granted DECIMAL(20,8) NOT NULL DEFAULT 0,
    consumed DECIMAL(20,8) NOT NULL DEFAULT 0,
    reclaimed DECIMAL(20,8) NOT NULL DEFAULT 0,
    version BIGINT NOT NULL DEFAULT 0,
    status TEXT NOT NULL,
    expires_at TIMESTAMPTZ NOT NULL,
    reclaim_at TIMESTAMPTZ NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_quota_lease_demo_leases_lookup
    ON quota_lease_demo_leases (node_id, user_id, api_key_id, status, expires_at);

CREATE INDEX IF NOT EXISTS idx_quota_lease_demo_leases_reclaim
    ON quota_lease_demo_leases (status, reclaim_at);

CREATE INDEX IF NOT EXISTS idx_quota_lease_demo_leases_cleanup
    ON quota_lease_demo_leases (status, updated_at);

CREATE TABLE IF NOT EXISTS quota_lease_demo_ledger_events (
    event_id TEXT PRIMARY KEY,
    lease_id TEXT NOT NULL,
    node_id TEXT NOT NULL,
    user_id BIGINT NOT NULL,
    api_key_id BIGINT NOT NULL,
    request_id TEXT NOT NULL DEFAULT '',
    amount DECIMAL(20,8) NOT NULL DEFAULT 0,
    event_type TEXT NOT NULL,
    payload_hash VARCHAR(64) NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_quota_lease_demo_events_lease_created
    ON quota_lease_demo_ledger_events (lease_id, created_at);

CREATE INDEX IF NOT EXISTS idx_quota_lease_demo_events_node_created
    ON quota_lease_demo_ledger_events (node_id, created_at DESC);

CREATE TABLE IF NOT EXISTS quota_lease_demo_pending_usage_events (
    event_id TEXT PRIMARY KEY,
    lease_id TEXT NOT NULL,
    node_id TEXT NOT NULL,
    user_id BIGINT NOT NULL,
    api_key_id BIGINT NOT NULL,
    request_id TEXT NOT NULL,
    amount DECIMAL(20,8) NOT NULL DEFAULT 0,
    event_type TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_quota_lease_demo_pending_usage_node_created
    ON quota_lease_demo_pending_usage_events (node_id, created_at);
