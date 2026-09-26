-- Repair groups.models_list_config for databases that have a stale migration
-- record or an incomplete model allowlist upgrade.
--
-- The migration is deliberately conditional: older deployments can have either
-- model_allowlist, models_list_config, both columns, or neither column.
DO $$
DECLARE
    has_model_allowlist BOOLEAN;
    has_models_list_config BOOLEAN;
BEGIN
    SELECT EXISTS (
        SELECT 1
        FROM pg_attribute
        WHERE attrelid = 'groups'::regclass
          AND attname = 'model_allowlist'
          AND NOT attisdropped
    ) INTO has_model_allowlist;

    SELECT EXISTS (
        SELECT 1
        FROM pg_attribute
        WHERE attrelid = 'groups'::regclass
          AND attname = 'models_list_config'
          AND NOT attisdropped
    ) INTO has_models_list_config;

    IF NOT has_models_list_config THEN
        ALTER TABLE groups
            ADD COLUMN models_list_config JSONB NOT NULL DEFAULT '{}'::jsonb;
        has_models_list_config := TRUE;
    END IF;

    IF has_model_allowlist THEN
        UPDATE groups
           SET models_list_config = model_allowlist
         WHERE COALESCE(models_list_config, '{}'::jsonb) = '{}'::jsonb
           AND COALESCE(model_allowlist, '{}'::jsonb) <> '{}'::jsonb;
    END IF;
END
$$;

ALTER TABLE groups ALTER COLUMN models_list_config SET DEFAULT '{}'::jsonb;
ALTER TABLE groups ALTER COLUMN models_list_config SET NOT NULL;

COMMENT ON COLUMN groups.models_list_config IS
    'Display-only legacy /v1/models configuration, independent of model_allowlist admission';
