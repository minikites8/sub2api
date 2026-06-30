-- 允许用户平台维度配额记录 Grok。
--
-- 145_allow_kiro_user_platform_quotas 把 platform check 扩到 kiro，但漏了 grok；
-- 代码层（service.AllowedQuotaPlatforms）与 ent schema 均已支持 grok，
-- 运行时会在设置/记账时插入 platform='grok'，DB CHECK 约束需同步放行。

ALTER TABLE user_platform_quotas
  DROP CONSTRAINT IF EXISTS user_platform_quotas_platform_check;

ALTER TABLE user_platform_quotas
  ADD CONSTRAINT user_platform_quotas_platform_check
  CHECK (platform IN ('anthropic', 'openai', 'gemini', 'antigravity', 'kiro', 'grok'));
