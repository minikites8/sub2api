-- Include TypeSafe / Jev in new installations and configurations created before
-- the provider existed. Keep existing provider switches and model lists intact.
ALTER TABLE channel_monitor_v2_config ALTER COLUMN platforms SET DEFAULT $platforms$
[
  {"platform":"anthropic","enabled":true,"models":[]},
  {"platform":"openai","enabled":true,"models":[]},
  {"platform":"grok","enabled":true,"models":[]},
  {"platform":"kiro","enabled":true,"models":[]},
  {"platform":"gemini","enabled":true,"models":[]},
  {"platform":"antigravity","enabled":true,"models":[]},
  {"platform":"typesafe","enabled":true,"models":[]}
]
$platforms$::jsonb;

UPDATE channel_monitor_v2_config AS config
SET platforms = config.platforms || '[{"platform":"typesafe","enabled":true,"models":[]}]'::jsonb,
    -- An empty list retains the all-groups scope. When introducing this provider
    -- into a selected scope, include its existing active groups exactly once.
    group_ids = CASE
        WHEN cardinality(config.group_ids) = 0 THEN config.group_ids
        ELSE ARRAY(
            SELECT DISTINCT group_id
            FROM (
                SELECT unnest(config.group_ids) AS group_id
                UNION ALL
                SELECT id AS group_id FROM groups
                WHERE lower(btrim(platform)) = 'typesafe'
                  AND status = 'active' AND deleted_at IS NULL
            ) AS monitored_groups
            ORDER BY group_id
        )
    END,
    version = config.version + 1,
    updated_at = NOW()
WHERE config.id = 1
  AND NOT EXISTS (
      SELECT 1 FROM jsonb_array_elements(config.platforms) AS provider
      WHERE lower(btrim(provider->>'platform')) = 'typesafe'
  );
