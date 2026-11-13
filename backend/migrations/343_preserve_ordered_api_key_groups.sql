-- Preserve existing ordered bindings when introducing the additional-group index.
INSERT INTO api_key_groups (api_key_id, group_id)
SELECT k.id, e.value::bigint
FROM api_keys k
CROSS JOIN LATERAL jsonb_array_elements_text(COALESCE(k.group_ids, '[]'::jsonb)) e
JOIN groups g ON g.id = e.value::bigint AND g.deleted_at IS NULL
WHERE k.deleted_at IS NULL AND e.value::bigint IS DISTINCT FROM k.group_id
ON CONFLICT DO NOTHING;

-- Keys created using the upstream representation retain their primary first.
UPDATE api_keys k
SET group_ids = COALESCE(k.group_ids, CASE WHEN k.group_id IS NULL THEN '[]'::jsonb ELSE jsonb_build_array(k.group_id) END)
    || COALESCE((SELECT jsonb_agg(b.group_id ORDER BY b.group_id)
                 FROM api_key_groups b JOIN groups g ON g.id = b.group_id AND g.deleted_at IS NULL
                 WHERE b.api_key_id = k.id
                   AND NOT COALESCE(k.group_ids, '[]'::jsonb) @> jsonb_build_array(b.group_id)
                   AND b.group_id IS DISTINCT FROM k.group_id), '[]'::jsonb)
WHERE k.deleted_at IS NULL;
