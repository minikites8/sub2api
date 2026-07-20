package migrations

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestAccountNodeAssignmentIndexMigration(t *testing.T) {
	content, err := FS.ReadFile("185_account_node_assignment_index_notx.sql")
	require.NoError(t, err)

	sql := strings.Join(strings.Fields(string(content)), " ")
	require.Contains(t, sql, "CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_accounts_node_oauth_assigned_node_id")
	require.Contains(t, sql, "ON accounts ((extra ->> 'node_oauth_assigned_node_id'), id)")
	require.Contains(t, sql, "WHERE deleted_at IS NULL")
	require.Contains(t, sql, "status = 'active'")
	require.Contains(t, sql, "schedulable = TRUE")
	require.Contains(t, sql, "extra ? 'node_oauth_assigned_node_id'")
}
