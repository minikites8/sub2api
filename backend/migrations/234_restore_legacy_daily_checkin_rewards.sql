-- Migration 232 incorrectly assigned expiry timestamps and ledger rows to
-- existing check-ins. Restore those legacy grants to their non-expiring state.
CREATE TABLE IF NOT EXISTS daily_checkin_reward_recovery (
    daily_checkin_reward_id BIGINT PRIMARY KEY,
    user_id                BIGINT NOT NULL,
    daily_checkin_id       BIGINT NOT NULL,
    original_amount        DECIMAL(20, 8) NOT NULL,
    recovered_amount       DECIMAL(20, 8) NOT NULL DEFAULT 0,
    recovered_at            TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

DO $$
DECLARE
    migration_applied_at TIMESTAMPTZ;
BEGIN
    SELECT applied_at
    INTO migration_applied_at
    FROM schema_migrations
    WHERE filename = '232_daily_checkin_reward_expiry.sql';

    IF migration_applied_at IS NULL THEN
        migration_applied_at := NOW();
    END IF;

    WITH candidates AS (
        SELECT r.id,
               r.user_id,
               r.daily_checkin_id,
               r.amount,
               CASE
                   WHEN r.remaining_amount = 0
                    AND r.expires_at <= NOW()
                    AND r.updated_at > r.expires_at
                   THEN r.amount
                   ELSE 0
               END AS recovered_amount
        FROM daily_checkin_rewards r
        JOIN daily_checkins dc ON dc.id = r.daily_checkin_id
        WHERE dc.created_at < migration_applied_at
    ), inserted AS (
        INSERT INTO daily_checkin_reward_recovery
            (daily_checkin_reward_id, user_id, daily_checkin_id, original_amount, recovered_amount)
        SELECT id, user_id, daily_checkin_id, amount, recovered_amount
        FROM candidates
        ON CONFLICT (daily_checkin_reward_id) DO NOTHING
        RETURNING user_id, recovered_amount
    ), totals AS (
        SELECT user_id, SUM(recovered_amount) AS amount
        FROM inserted
        WHERE recovered_amount > 0
        GROUP BY user_id
    )
    UPDATE users u
    SET balance = u.balance + totals.amount,
        updated_at = NOW()
    FROM totals
    WHERE u.id = totals.user_id AND u.deleted_at IS NULL;

    DELETE FROM daily_checkin_rewards r
    USING daily_checkins dc
    WHERE dc.id = r.daily_checkin_id
      AND dc.created_at < migration_applied_at;

    UPDATE daily_checkins
    SET expires_at = NULL,
        updated_at = NOW()
    WHERE created_at < migration_applied_at;
END $$;
