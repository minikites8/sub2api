CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_accounts_node_oauth_assigned_node_id
    ON accounts ((extra ->> 'node_oauth_assigned_node_id'), id)
    WHERE deleted_at IS NULL
      AND status = 'active'
      AND schedulable = TRUE
      AND extra ? 'node_oauth_assigned_node_id';
