-- Converge the platform constraints after the main and production migrations.
-- Both existing Kiro rows and new TypeSafe rows remain valid on upgrades and
-- fresh installations, including installations where migration 251 was applied.
ALTER TABLE user_platform_quotas DROP CONSTRAINT IF EXISTS user_platform_quotas_platform_check;
ALTER TABLE user_platform_quotas ADD CONSTRAINT user_platform_quotas_platform_check
    CHECK (platform IN ('anthropic', 'openai', 'gemini', 'antigravity', 'grok', 'kiro',
                        'kimi', 'zhipu', 'deepseek', 'minimax', 'opencode_go', 'typesafe'));

ALTER TABLE composite_model_routes DROP CONSTRAINT IF EXISTS composite_model_routes_target_platform_check;
ALTER TABLE composite_model_routes ADD CONSTRAINT composite_model_routes_target_platform_check
    CHECK (target_platform IN ('anthropic', 'openai', 'gemini', 'antigravity', 'grok', 'kiro',
                               'kimi', 'zhipu', 'deepseek', 'minimax', 'opencode_go', 'typesafe'));
