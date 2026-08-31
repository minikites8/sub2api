package repository

import (
	"context"
	"regexp"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/require"
)

func TestDailyCheckinRepositorySumsRecentCompletedRecharges(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	since := time.Date(2026, 8, 16, 12, 0, 0, 0, time.UTC)
	mock.ExpectQuery(regexp.QuoteMeta("SELECT COALESCE(SUM(amount), 0)::double precision")).
		WithArgs(int64(42), since).
		WillReturnRows(sqlmock.NewRows([]string{"sum"}).AddRow(1.25))

	repo := &dailyCheckinRepository{db: db}
	amount, err := repo.SumRechargeAmountSince(context.Background(), 42, since)
	require.NoError(t, err)
	require.Equal(t, 1.25, amount)
	require.NoError(t, mock.ExpectationsWereMet())
}
