package repository

import (
	"context"
	"regexp"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func TestRecordAntiAbuseEventStoresIdentifiableAccountsWithDedupGuard(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() { _ = db.Close() }()

	createdAt := time.Date(2026, 9, 4, 0, 0, 0, 0, time.UTC)
	mock.ExpectQuery(regexp.QuoteMeta("WHERE $14::numeric > 0 OR NOT EXISTS")).
		WithArgs(
			int64(7), "gateway", "restrict", 60, sqlmock.AnyArg(), sqlmock.AnyArg(),
			"1.2.3.4", "current@example.com", "Mozilla/5.0", 0, "", "",
			`["first@example.com","second@example.com"]`, float64(0),
		).
		WillReturnRows(sqlmock.NewRows([]string{"id", "created_at"}).AddRow(int64(99), createdAt))

	repo := newUserRepositoryWithSQL(nil, db)
	userID := int64(7)
	event := &service.AntiAbuseEvent{
		UserID: &userID, EventType: "gateway", Action: service.AntiAbuseActionRestrict, Score: 60,
		Factors: map[string]int{"browser_account_attempts": 60}, Reasons: []string{"multiple accounts"},
		IPAddress: "1.2.3.4", Email: "current@example.com", UserAgent: "Mozilla/5.0",
		AccountAttempts: []string{"First@Example.com", "second@example.com", "invalid"},
	}

	require.NoError(t, repo.RecordAntiAbuseEvent(context.Background(), event))
	require.Equal(t, int64(99), event.ID)
	require.Equal(t, createdAt, event.CreatedAt)
	require.NoError(t, mock.ExpectationsWereMet())
}
