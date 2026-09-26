-- Restore the display-only column after 235/236 renamed the legacy column to model_allowlist.
-- Keep the existing admission allowlist intact: deployments may already have saved policy there.
ALTER TABLE groups
    ADD COLUMN IF NOT EXISTS models_list_config JSONB NOT NULL DEFAULT '{}'::jsonb;

-- Backfill rows from the renamed legacy data while retaining an independently edited
-- display configuration. This also makes repeated application idempotent.
UPDATE groups
   SET models_list_config = model_allowlist
 WHERE COALESCE(models_list_config, '{}'::jsonb) = '{}'::jsonb
   AND COALESCE(model_allowlist, '{}'::jsonb) <> '{}'::jsonb;

ALTER TABLE groups ALTER COLUMN models_list_config SET DEFAULT '{}'::jsonb;
ALTER TABLE groups ALTER COLUMN models_list_config SET NOT NULL;

COMMENT ON COLUMN groups.models_list_config IS
    'Display-only legacy /v1/models configuration, independent of model_allowlist admission';
