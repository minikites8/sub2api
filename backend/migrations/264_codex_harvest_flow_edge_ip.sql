ALTER TABLE codex_harvest_flow_events
    ADD COLUMN IF NOT EXISTS edge_ip VARCHAR(64) NOT NULL DEFAULT '';
