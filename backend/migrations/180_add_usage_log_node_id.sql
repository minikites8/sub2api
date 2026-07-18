-- Record the quota-lease node that handled a proxied user request.

ALTER TABLE usage_logs
    ADD COLUMN IF NOT EXISTS node_id TEXT;
