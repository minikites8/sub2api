ALTER TABLE codex_harvest_flow_events
    ADD COLUMN IF NOT EXISTS gateway VARCHAR(64) NOT NULL DEFAULT '';
