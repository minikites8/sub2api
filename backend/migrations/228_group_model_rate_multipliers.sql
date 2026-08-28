-- Per-model group billing multiplier overrides.
ALTER TABLE groups
    ADD COLUMN IF NOT EXISTS model_rate_multipliers JSONB NOT NULL DEFAULT '{}'::jsonb;

COMMENT ON COLUMN groups.model_rate_multipliers IS
    '按模型覆盖分组倍率；模型名大小写不敏感精确匹配';
