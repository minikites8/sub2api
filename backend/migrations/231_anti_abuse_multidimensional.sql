CREATE TABLE IF NOT EXISTS anti_abuse_user_fingerprints (
    id BIGSERIAL PRIMARY KEY,
    user_id BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    fingerprint_hash VARCHAR(64) NOT NULL,
    fingerprint_bucket VARCHAR(16) NOT NULL,
    risk_score INTEGER NOT NULL DEFAULT 0,
    risk_factors JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (user_id, fingerprint_hash)
);

CREATE INDEX IF NOT EXISTS idx_anti_abuse_fingerprint_recent
    ON anti_abuse_user_fingerprints(fingerprint_hash, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_anti_abuse_fingerprint_bucket_recent
    ON anti_abuse_user_fingerprints(fingerprint_bucket, created_at DESC);

CREATE TABLE IF NOT EXISTS anti_abuse_events (
    id BIGSERIAL PRIMARY KEY,
    user_id BIGINT REFERENCES users(id) ON DELETE SET NULL,
    event_type VARCHAR(32) NOT NULL,
    action VARCHAR(16) NOT NULL,
    score INTEGER NOT NULL DEFAULT 0,
    factors JSONB NOT NULL DEFAULT '{}'::jsonb,
    reasons JSONB NOT NULL DEFAULT '[]'::jsonb,
    ip_address VARCHAR(64) NOT NULL DEFAULT '',
    email VARCHAR(320) NOT NULL DEFAULT '',
    user_agent VARCHAR(1024) NOT NULL DEFAULT '',
    fingerprint_hash_count INTEGER NOT NULL DEFAULT 0,
    ja3_hash VARCHAR(64) NOT NULL DEFAULT '',
    ja4_hash VARCHAR(64) NOT NULL DEFAULT '',
    gift_balance_deducted NUMERIC(20, 8) NOT NULL DEFAULT 0,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_anti_abuse_events_created
    ON anti_abuse_events(created_at DESC);
CREATE INDEX IF NOT EXISTS idx_anti_abuse_events_action_created
    ON anti_abuse_events(action, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_anti_abuse_events_user_created
    ON anti_abuse_events(user_id, created_at DESC);

INSERT INTO settings (key, value)
VALUES
    ('anti_abuse_enabled', 'true'),
    ('anti_abuse_score_threshold', '60'),
    ('anti_abuse_fingerprint_weight', '1'),
    ('anti_abuse_ip_weight', '1'),
    ('anti_abuse_email_weight', '1'),
    ('anti_abuse_user_agent_weight', '1'),
    ('anti_abuse_tls_fingerprint_weight', '1'),
    ('anti_abuse_ip_reputation_endpoint', ''),
    ('anti_abuse_ip_reputation_api_key', '')
ON CONFLICT (key) DO NOTHING;
