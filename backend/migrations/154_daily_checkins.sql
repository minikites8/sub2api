-- Daily check-in reward records and per-day pool totals.
CREATE TABLE IF NOT EXISTS daily_checkins (
    id            BIGSERIAL PRIMARY KEY,
    user_id       BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    checkin_date  DATE NOT NULL,
    reward        DECIMAL(20, 8) NOT NULL,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at    TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT daily_checkins_reward_non_negative CHECK (reward >= 0),
    CONSTRAINT daily_checkins_user_date_unique UNIQUE (user_id, checkin_date)
);

CREATE INDEX IF NOT EXISTS idx_daily_checkins_checkin_date ON daily_checkins(checkin_date);
CREATE INDEX IF NOT EXISTS idx_daily_checkins_user_id_created_at ON daily_checkins(user_id, created_at DESC);

COMMENT ON TABLE daily_checkins IS 'User daily check-in reward records';
COMMENT ON COLUMN daily_checkins.checkin_date IS 'Site-local check-in date';
COMMENT ON COLUMN daily_checkins.reward IS 'Balance reward granted for this check-in';

CREATE TABLE IF NOT EXISTS daily_checkin_totals (
    checkin_date  DATE PRIMARY KEY,
    total_reward  DECIMAL(20, 8) NOT NULL DEFAULT 0,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at    TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT daily_checkin_totals_reward_non_negative CHECK (total_reward >= 0)
);

COMMENT ON TABLE daily_checkin_totals IS 'Per-day total daily check-in rewards for enforcing the site-wide cap';
COMMENT ON COLUMN daily_checkin_totals.checkin_date IS 'Site-local check-in date';
COMMENT ON COLUMN daily_checkin_totals.total_reward IS 'Total balance granted by daily check-ins on this date';
