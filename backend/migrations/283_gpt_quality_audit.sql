-- Scheduling metadata stays in existing JSONB/settings; no billing columns change.
-- Existing rows have effective quality_tier=unknown and are never promoted.
CREATE TABLE IF NOT EXISTS account_quality_audit (
 id BIGSERIAL PRIMARY KEY,
 account_id BIGINT NOT NULL,
 actor_user_id BIGINT,
 old_tier VARCHAR(16) NOT NULL,
 new_tier VARCHAR(16) NOT NULL,
 created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS account_quality_audit_account_time ON account_quality_audit(account_id,created_at DESC);
CREATE OR REPLACE FUNCTION record_account_quality_change() RETURNS trigger AS $$
DECLARE before_tier TEXT; after_tier TEXT; actor BIGINT;
BEGIN
 before_tier := CASE WHEN TG_OP='INSERT' THEN 'unknown' ELSE COALESCE(OLD.extra->>'quality_tier','unknown') END;
 after_tier := COALESCE(NEW.extra->>'quality_tier','unknown');
 IF after_tier NOT IN ('strong','basic','unknown') THEN RAISE EXCEPTION 'invalid quality_tier'; END IF;
 IF before_tier IS DISTINCT FROM after_tier THEN
  actor := CASE WHEN COALESCE(NEW.extra->>'quality_tier_actor_id','') ~ '^[0-9]{1,18}$' THEN (NEW.extra->>'quality_tier_actor_id')::bigint ELSE NULL END;
  INSERT INTO account_quality_audit(account_id,actor_user_id,old_tier,new_tier) VALUES(NEW.id,actor,before_tier,after_tier);
 END IF;
 RETURN NEW;
END;
$$ LANGUAGE plpgsql;
DROP TRIGGER IF EXISTS accounts_quality_audit ON accounts;
CREATE TRIGGER accounts_quality_audit AFTER INSERT OR UPDATE OF extra ON accounts
FOR EACH ROW EXECUTE FUNCTION record_account_quality_change();
