ALTER TABLE accounts
    ADD COLUMN IF NOT EXISTS is_fallback BOOLEAN NOT NULL DEFAULT FALSE;

CREATE INDEX IF NOT EXISTS idx_accounts_fallback_scheduler
    ON accounts (platform, is_fallback, priority)
    WHERE deleted_at IS NULL AND status = 'active' AND schedulable = TRUE;
