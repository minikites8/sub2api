//go:build integration

package repository

import (
	"context"
	"testing"

	dbmigrations "github.com/Wei-Shaw/sub2api/migrations"
	"github.com/lib/pq"
	"github.com/stretchr/testify/require"
)

func TestMigration253GroupRoutingPolicy(t *testing.T) {
	tx := testTx(t)
	ctx := context.Background()
	// Restore the pre-migration schema inside the test transaction.
	_, err := tx.ExecContext(ctx, "ALTER TABLE groups DROP COLUMN enable_bps, DROP COLUMN scheduling_strategy")
	require.NoError(t, err)
	var existingID int64
	require.NoError(t, tx.QueryRowContext(ctx, "INSERT INTO groups (name, platform) VALUES ('migration253-existing', 'openai') RETURNING id").Scan(&existingID))

	content, err := dbmigrations.FS.ReadFile("253_group_bps_scheduling.sql")
	require.NoError(t, err)
	_, err = tx.ExecContext(ctx, string(content))
	require.NoError(t, err, "execute the complete migration, including its PL/pgSQL block")

	var enabled bool
	var strategy string
	require.NoError(t, tx.QueryRowContext(ctx, "SELECT enable_bps, scheduling_strategy FROM groups WHERE id = $1", existingID).Scan(&enabled, &strategy))
	require.False(t, enabled, "existing groups default to platform upstreams")
	require.Equal(t, "balanced", strategy)
	require.NoError(t, tx.QueryRowContext(ctx, "INSERT INTO groups (name, platform) VALUES ('migration253-new', 'openai') RETURNING enable_bps, scheduling_strategy").Scan(&enabled, &strategy))
	require.False(t, enabled, "new groups default to platform upstreams")
	require.Equal(t, "balanced", strategy)

	for _, want := range []string{"balanced", "priority_5h", "priority_weekly"} {
		_, err = tx.ExecContext(ctx, "UPDATE groups SET enable_bps = TRUE, scheduling_strategy = $1 WHERE id = $2", want, existingID)
		require.NoError(t, err)
		_, err = tx.ExecContext(ctx, string(content))
		require.NoError(t, err, "migration replay preserves configured groups")
		require.NoError(t, tx.QueryRowContext(ctx, "SELECT enable_bps, scheduling_strategy FROM groups WHERE id = $1", existingID).Scan(&enabled, &strategy))
		require.True(t, enabled)
		require.Equal(t, want, strategy)
	}

	var constraints int
	require.NoError(t, tx.QueryRowContext(ctx, "SELECT COUNT(*) FROM pg_constraint WHERE conname = 'groups_scheduling_strategy_check' AND conrelid = 'groups'::regclass").Scan(&constraints))
	require.Equal(t, 1, constraints, "migration replay keeps one strategy constraint")

	for _, invalid := range []struct {
		name   string
		update string
		code   pq.ErrorCode
	}{
		{"unknown strategy", "scheduling_strategy = 'unknown'", "23514"},
		{"null strategy", "scheduling_strategy = NULL", "23502"},
		{"null BPS gate", "enable_bps = NULL", "23502"},
	} {
		_, err = tx.ExecContext(ctx, "SAVEPOINT invalid_group_routing_policy")
		require.NoError(t, err)
		_, err = tx.ExecContext(ctx, "UPDATE groups SET "+invalid.update+" WHERE id = $1", existingID)
		var pgErr *pq.Error
		require.ErrorAs(t, err, &pgErr, invalid.name)
		require.Equal(t, invalid.code, pgErr.Code, invalid.name)
		_, err = tx.ExecContext(ctx, "ROLLBACK TO SAVEPOINT invalid_group_routing_policy")
		require.NoError(t, err)
		_, err = tx.ExecContext(ctx, "RELEASE SAVEPOINT invalid_group_routing_policy")
		require.NoError(t, err)
	}
}
