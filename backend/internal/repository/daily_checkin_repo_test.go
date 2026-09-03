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

func TestDailyCheckinRepositoryGetUserCheckinReadsExpiry(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	createdAt := time.Date(2026, 9, 3, 10, 0, 0, 0, time.UTC)
	expiresAt := createdAt.Add(30 * 24 * time.Hour)
	mock.ExpectQuery(regexp.QuoteMeta("SELECT user_id, checkin_date::text, reward::double precision, created_at, expires_at")).
		WithArgs(int64(42), "2026-09-03").
		WillReturnRows(sqlmock.NewRows([]string{"user_id", "checkin_date", "reward", "created_at", "expires_at"}).
			AddRow(int64(42), "2026-09-03", 0.25, createdAt, expiresAt))

	repo := &dailyCheckinRepository{db: db}
	record, err := repo.GetUserCheckin(context.Background(), 42, "2026-09-03")
	require.NoError(t, err)
	require.NotNil(t, record)
	require.Equal(t, int64(42), record.UserID)
	require.Equal(t, 0.25, record.Reward)
	require.NotNil(t, record.ExpiresAt)
	require.Equal(t, expiresAt, *record.ExpiresAt)
	require.NoError(t, mock.ExpectationsWereMet())
}
