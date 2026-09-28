package migrations

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestGroupRoutingPolicyMigrationDollarQuotedBlock(t *testing.T) {
	content, err := FS.ReadFile("253_group_bps_scheduling.sql")
	require.NoError(t, err)
	// The DO body must have matching dollar quotes and a terminated PL/pgSQL block.
	require.Regexp(t, `(?s)DO\s+\$group_bps_scheduling\$\s+BEGIN\b.*END;\s*\$group_bps_scheduling\$;`, string(content))
}

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
