ALTER TABLE quota_lease_demo_leases
    ADD COLUMN IF NOT EXISTS version BIGINT NOT NULL DEFAULT 0;

CREATE INDEX IF NOT EXISTS idx_quota_lease_demo_leases_cleanup
    ON quota_lease_demo_leases (status, updated_at);
