ALTER TABLE groups ADD COLUMN IF NOT EXISTS enable_bps BOOLEAN NOT NULL DEFAULT FALSE;
ALTER TABLE groups ADD COLUMN IF NOT EXISTS scheduling_strategy VARCHAR(32) NOT NULL DEFAULT 'balanced';
DO $ BEGIN
    IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'groups_scheduling_strategy_check' AND conrelid = 'groups'::regclass) THEN
        ALTER TABLE groups ADD CONSTRAINT groups_scheduling_strategy_check
            CHECK (scheduling_strategy IN ('balanced', 'priority_5h', 'priority_weekly'));
    END IF;
END $;
