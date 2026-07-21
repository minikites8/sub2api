CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_accounts_node_oauth_assigned_node_ids
    ON accounts
    USING GIN ((extra -> 'node_oauth_assigned_node_ids'))
    WHERE deleted_at IS NULL
      AND status = 'active'
      AND schedulable = TRUE
      AND type = 'apikey'
      AND extra ? 'node_oauth_assigned_node_ids';
