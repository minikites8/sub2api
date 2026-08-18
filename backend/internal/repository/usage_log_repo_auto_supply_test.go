package repository

import (
	"context"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/require"
)

func TestUsageLogRepositoryGetAutoSupplyUsageWindow(t *testing.T) {
	db, mock := newSQLMock(t)
	repo := newUsageLogRepositoryWithSQL(nil, db)
	start := time.Date(2026, 8, 19, 6, 0, 0, 0, time.UTC)
	midpoint := start.Add(3 * time.Hour)
	end := start.Add(6 * time.Hour)

	mock.ExpectQuery(`(?s)SELECT\s+COUNT\(duration_ms\).*FROM usage_logs.*WHERE group_id = \$1`).
		WithArgs(int64(42), start, midpoint, end).
		WillReturnRows(sqlmock.NewRows([]string{
			"previous_samples", "previous_duration_ms", "recent_samples", "recent_duration_ms",
		}).AddRow(int64(12), int64(7_200_000), int64(18), int64(14_400_000)))

	window, err := repo.GetAutoSupplyUsageWindow(context.Background(), 42, start, midpoint, end)
	require.NoError(t, err)
	require.Equal(t, int64(12), window.Previous.Samples)
	require.Equal(t, int64(7_200_000), window.Previous.TotalDurationMs)
	require.Equal(t, int64(18), window.Recent.Samples)
	require.Equal(t, int64(14_400_000), window.Recent.TotalDurationMs)
	require.NoError(t, mock.ExpectationsWereMet())
}
