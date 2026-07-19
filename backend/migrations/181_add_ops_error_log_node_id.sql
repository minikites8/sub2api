-- Track the quota lease node that handled failed requests.
ALTER TABLE ops_error_logs
    ADD COLUMN IF NOT EXISTS node_id TEXT;

CREATE INDEX IF NOT EXISTS idx_ops_error_logs_node_time
    ON ops_error_logs (node_id, created_at DESC)
    WHERE node_id IS NOT NULL;

COMMENT ON COLUMN ops_error_logs.node_id IS 'Quota lease node that handled the failed request.';
