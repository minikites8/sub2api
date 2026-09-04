-- Split the existing wallet into a compatibility total and a tracked gift bucket.
-- Gift credits cover registration grants and daily check-in rewards.
ALTER TABLE users
    ADD COLUMN IF NOT EXISTS gift_balance DECIMAL(20,8) NOT NULL DEFAULT 0,
    ADD COLUMN IF NOT EXISTS gift_balance_expires_at TIMESTAMPTZ,
    ADD COLUMN IF NOT EXISTS frozen_gift_balance DECIMAL(20,8) NOT NULL DEFAULT 0;

-- Existing accounts with recharge history keep their current wallet as paid
-- balance. Accounts without recharge history are treated as registration gifts.
UPDATE users
SET gift_balance = CASE
        WHEN COALESCE(total_recharged, 0) <= 0 THEN GREATEST(balance, 0)
        ELSE 0
    END,
    gift_balance_expires_at = NULL
WHERE gift_balance = 0;

-- Daily check-in rewards remain the source of expiry data. Include their
-- unspent amount for users who already have recharge history.
UPDATE users u
SET gift_balance = LEAST(
        GREATEST(u.balance, 0),
        u.gift_balance + COALESCE((
            SELECT SUM(r.remaining_amount)
            FROM daily_checkin_rewards r
            WHERE r.user_id = u.id AND r.remaining_amount > 0 AND r.expires_at > NOW()
        ), 0)
    ),
    gift_balance_expires_at = (
        SELECT MIN(r.expires_at)
        FROM daily_checkin_rewards r
        WHERE r.user_id = u.id AND r.remaining_amount > 0 AND r.expires_at > NOW()
    )
WHERE EXISTS (
    SELECT 1 FROM daily_checkin_rewards r
    WHERE r.user_id = u.id AND r.remaining_amount > 0 AND r.expires_at > NOW()
);

CREATE INDEX IF NOT EXISTS idx_users_gift_balance ON users(gift_balance);

CREATE OR REPLACE FUNCTION initialize_user_gift_balance()
RETURNS trigger
LANGUAGE plpgsql
AS $$
BEGIN
    IF COALESCE(NEW.gift_balance, 0) = 0
       AND COALESCE(NEW.total_recharged, 0) <= 0
       AND COALESCE(NEW.balance, 0) > 0 THEN
        NEW.gift_balance := NEW.balance;
    END IF;
    RETURN NEW;
END;
$$;

DROP TRIGGER IF EXISTS users_initialize_gift_balance ON users;
CREATE TRIGGER users_initialize_gift_balance
BEFORE INSERT ON users
FOR EACH ROW
EXECUTE FUNCTION initialize_user_gift_balance();

CREATE OR REPLACE FUNCTION clamp_user_gift_balance()
RETURNS trigger
LANGUAGE plpgsql
AS $$
BEGIN
    NEW.gift_balance := LEAST(
        GREATEST(COALESCE(NEW.gift_balance, OLD.gift_balance, 0), 0),
        GREATEST(COALESCE(NEW.balance, 0), 0)
    );
    IF NEW.gift_balance = 0 THEN
        NEW.gift_balance_expires_at := NULL;
    END IF;
    RETURN NEW;
END;
$$;

DROP TRIGGER IF EXISTS users_clamp_gift_balance ON users;
CREATE TRIGGER users_clamp_gift_balance
BEFORE UPDATE OF balance, gift_balance ON users
FOR EACH ROW
EXECUTE FUNCTION clamp_user_gift_balance();
