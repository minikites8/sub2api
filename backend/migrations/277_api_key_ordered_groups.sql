-- Keep the first binding in group_id for older clients and existing billing code.
ALTER TABLE api_keys ADD COLUMN IF NOT EXISTS group_ids JSONB;
UPDATE api_keys SET group_ids = CASE WHEN group_id IS NULL THEN '[]'::jsonb ELSE jsonb_build_array(group_id) END
WHERE group_ids IS NULL;
CREATE INDEX IF NOT EXISTS idx_api_keys_group_ids ON api_keys USING gin (group_ids);
