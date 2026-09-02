ALTER TABLE daily_checkins
    ADD COLUMN IF NOT EXISTS expires_at TIMESTAMPTZ;

UPDATE daily_checkins
SET expires_at = created_at + INTERVAL '30 days'
WHERE expires_at IS NULL;

CREATE INDEX IF NOT EXISTS idx_daily_checkins_expires_at ON daily_checkins(expires_at);

CREATE TABLE IF NOT EXISTS daily_checkin_rewards (
    id               BIGSERIAL PRIMARY KEY,
    user_id          BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    daily_checkin_id BIGINT NOT NULL REFERENCES daily_checkins(id) ON DELETE CASCADE,
    amount           DECIMAL(20, 8) NOT NULL,
    remaining_amount DECIMAL(20, 8) NOT NULL,
    expires_at       TIMESTAMPTZ NOT NULL,
    created_at       TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at       TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT daily_checkin_rewards_amount_positive CHECK (amount > 0),
    CONSTRAINT daily_checkin_rewards_remaining_valid CHECK (remaining_amount >= 0 AND remaining_amount <= amount),
    CONSTRAINT daily_checkin_rewards_checkin_unique UNIQUE (daily_checkin_id)
);

CREATE INDEX IF NOT EXISTS idx_daily_checkin_rewards_user_expiry
    ON daily_checkin_rewards(user_id, expires_at, id);

INSERT INTO daily_checkin_rewards (user_id, daily_checkin_id, amount, remaining_amount, expires_at, created_at, updated_at)
SELECT user_id, id, reward, reward, expires_at, created_at, updated_at
FROM daily_checkins
WHERE expires_at IS NOT NULL AND reward > 0
ON CONFLICT (daily_checkin_id) DO NOTHING;
