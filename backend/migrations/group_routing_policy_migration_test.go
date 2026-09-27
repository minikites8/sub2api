package migrations

import (
	"github.com/stretchr/testify/require"
	"testing"
)

func TestGroupRoutingPolicyMigrationDefaultsAndConstraint(t *testing.T) {
	content, err := FS.ReadFile("253_group_bps_scheduling.sql")
	require.NoError(t, err)
	sql := string(content)
	require.Contains(t, sql, "enable_bps BOOLEAN NOT NULL DEFAULT FALSE")
	require.Contains(t, sql, "scheduling_strategy VARCHAR(32) NOT NULL DEFAULT 'balanced'")
	require.Contains(t, sql, "CHECK (scheduling_strategy IN ('balanced', 'priority_5h', 'priority_weekly'))")
	require.Contains(t, sql, "ADD COLUMN IF NOT EXISTS")
	require.Contains(t, sql, "IF NOT EXISTS (SELECT 1 FROM pg_constraint")
}
