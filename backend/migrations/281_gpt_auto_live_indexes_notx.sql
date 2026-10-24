-- GPT-Auto Phase 2 export/list filters. CONCURRENTLY avoids blocking decision writes.
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_gpt_auto_decisions_mode_created
    ON gpt_auto_shadow_decisions (mode, created_at DESC, id DESC);
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_gpt_auto_decisions_user_live_created
    ON gpt_auto_shadow_decisions (user_id, created_at DESC, id DESC)
    WHERE mode = 'live' AND user_id IS NOT NULL;
