ALTER TABLE anti_abuse_events
    ADD COLUMN IF NOT EXISTS account_attempts JSONB NOT NULL DEFAULT '[]'::jsonb;
