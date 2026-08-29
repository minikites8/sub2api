package repository

import (
	"context"
	"regexp"
	"strings"
	"testing"
	"time"

	sqlmock "github.com/DATA-DOG/go-sqlmock"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func TestBuildContentModerationLogWhere_BlockedIncludesAllBlockActions(t *testing.T) {
	where, args := buildContentModerationLogWhere(service.ContentModerationLogFilter{Result: "blocked"})

	require.Empty(t, args)
	sql := strings.Join(where, " AND ")
	require.Contains(t, sql, "l.action IN ('block', 'keyword_block', 'hash_block')")
	require.NotContains(t, sql, "l.action = 'block'")
}

func TestContentModerationRepositoryCountFlaggedByUserSince_ExcludesHashBlock(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() { _ = db.Close() }()

	repo := NewContentModerationRepository(db)
	since := time.Now().Add(-time.Hour)
	mock.ExpectQuery(regexp.QuoteMeta("AND action <> 'hash_block'")).
		WithArgs(int64(1001), since, false).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(2))

	count, err := repo.CountFlaggedByUserSince(context.Background(), 1001, since, false)

	require.NoError(t, err)
	require.Equal(t, 2, count)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestContentModerationRepositoryCountFlaggedByUserSince_ExcludesCyberPolicyWhenRequested(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() { _ = db.Close() }()

	repo := NewContentModerationRepository(db)
	since := time.Now().Add(-time.Hour)
	mock.ExpectQuery(regexp.QuoteMeta("AND ($3::bool IS FALSE OR action <> 'cyber_policy')")).
		WithArgs(int64(1001), since, true).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(3))

	count, err := repo.CountFlaggedByUserSince(context.Background(), 1001, since, true)

	require.NoError(t, err)
	require.Equal(t, 3, count)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestContentModerationRepositoryListLogs_ResolvesRoutedAccount(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() { _ = db.Close() }()

	createdAt := time.Date(2026, time.August, 29, 18, 6, 50, 0, time.UTC)
	mock.ExpectQuery(regexp.QuoteMeta("SELECT COUNT(*) FROM content_moderation_logs l WHERE l.id IS NOT NULL")).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))
	mock.ExpectQuery(`(?s)SELECT.*routed\.account_id, COALESCE\(a\.name, ''\).*FROM usage_logs ul.*UNION ALL.*FROM ops_error_logs o.*ORDER BY route\.source_priority, route\.created_at DESC.*LEFT JOIN accounts a ON a\.id = routed\.account_id`).
		WithArgs(20, 0).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "request_id", "user_id", "user_email", "api_key_id", "api_key_name", "group_id", "group_name",
			"account_id", "account_name", "endpoint", "provider", "model", "mode", "action", "flagged",
			"highest_category", "highest_score", "category_scores", "threshold_snapshot", "input_excerpt",
			"upstream_latency_ms", "error", "violation_count", "auto_banned", "email_sent", "user_status",
			"queue_delay_ms", "matched_keyword", "created_at",
		}).AddRow(
			int64(7), "req-routed", int64(1001), "user@example.com", int64(88), "key-a", int64(9), "openai",
			int64(42), "upstream-account", "/v1/responses", "openai", "gpt-5.5", "observe", "allow", false,
			"", 0.0, []byte(`{}`), []byte(`{}`), "hello", int64(120), "", 0, false, false, "active",
			int64(3), "", createdAt,
		))

	repo := NewContentModerationRepository(db)
	items, page, err := repo.ListLogs(context.Background(), service.ContentModerationLogFilter{})

	require.NoError(t, err)
	require.Len(t, items, 1)
	require.NotNil(t, items[0].AccountID)
	require.Equal(t, int64(42), *items[0].AccountID)
	require.Equal(t, "upstream-account", items[0].AccountName)
	require.Equal(t, int64(1), page.Total)
	require.NoError(t, mock.ExpectationsWereMet())
}
