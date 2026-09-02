-- Keep the risk-center event table focused on hard restrictions and deductions.
-- Historical allow/review assessments carry no enforcement state and can be removed.
DELETE FROM anti_abuse_events
WHERE action IN ('allow', 'review');

DROP INDEX IF EXISTS idx_anti_abuse_events_action_created;
CREATE INDEX IF NOT EXISTS idx_anti_abuse_events_restrict_created
    ON anti_abuse_events(created_at DESC)
    WHERE action = 'restrict';
