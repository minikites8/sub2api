-- Per-group OpenAI service_tier request policy.
ALTER TABLE groups
    ADD COLUMN IF NOT EXISTS openai_service_tier_mode VARCHAR(20) NOT NULL DEFAULT 'passthrough',
    ADD COLUMN IF NOT EXISTS openai_service_tier VARCHAR(20) NOT NULL DEFAULT '';

COMMENT ON COLUMN groups.openai_service_tier_mode IS
    'OpenAI 分组 service_tier 策略：passthrough/set/clear';
COMMENT ON COLUMN groups.openai_service_tier IS
    'OpenAI 分组强制 service_tier 值；仅 set 模式生效';
