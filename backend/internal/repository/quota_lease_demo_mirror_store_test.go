package repository

import (
	"context"
	"database/sql"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func TestQuotaLeaseDemoMirrorStoreReconcileAccountGroupsUsesDensePlaceholders(t *testing.T) {
	store := &quotaLeaseDemoMirrorStore{}
	exec := &quotaLeaseDemoMirrorSQLRecorder{}
	now := time.Now().UTC()

	err := store.reconcileMirrorAccountGroups(
		context.Background(),
		exec,
		[]service.Account{{ID: 101}},
		[]service.AccountGroup{
			{AccountID: 0, GroupID: 7, Priority: 1, CreatedAt: now},
			{AccountID: 101, GroupID: 8, Priority: 2, CreatedAt: now},
		},
		"node-1",
	)
	require.NoError(t, err)
	require.Len(t, exec.calls, 2)

	insert := exec.calls[1]
	require.Contains(t, insert.query, "($1, $2, $3, $4)")
	require.NotContains(t, insert.query, "$5")
	require.Len(t, insert.args, 4)
	require.Equal(t, []any{int64(101), int64(8), 2, now}, insert.args)
}

func TestQuotaLeaseDemoMirrorAccountGroupsFromIDsDefaultCreatedAt(t *testing.T) {
	groups := cloneQuotaLeaseDemoMirrorAccountGroups(nil, []service.Account{{
		ID:        202,
		GroupIDs:  []int64{9},
		Priority:  3,
		CreatedAt: time.Time{},
	}})

	require.Len(t, groups, 1)
	require.Equal(t, int64(202), groups[0].AccountID)
	require.Equal(t, int64(9), groups[0].GroupID)
	require.Equal(t, 3, groups[0].Priority)
	require.False(t, groups[0].CreatedAt.IsZero())
}

type quotaLeaseDemoMirrorSQLRecorder struct {
	calls []quotaLeaseDemoMirrorSQLCall
}

type quotaLeaseDemoMirrorSQLCall struct {
	query string
	args  []any
}

func (r *quotaLeaseDemoMirrorSQLRecorder) ExecContext(_ context.Context, query string, args ...any) (sql.Result, error) {
	r.calls = append(r.calls, quotaLeaseDemoMirrorSQLCall{
		query: strings.TrimSpace(query),
		args:  append([]any(nil), args...),
	})
	return quotaLeaseDemoMirrorSQLResult(0), nil
}

func (r *quotaLeaseDemoMirrorSQLRecorder) QueryContext(context.Context, string, ...any) (*sql.Rows, error) {
	return nil, nil
}

type quotaLeaseDemoMirrorSQLResult int64

func (r quotaLeaseDemoMirrorSQLResult) LastInsertId() (int64, error) {
	return 0, nil
}

func (r quotaLeaseDemoMirrorSQLResult) RowsAffected() (int64, error) {
	return int64(r), nil
}
