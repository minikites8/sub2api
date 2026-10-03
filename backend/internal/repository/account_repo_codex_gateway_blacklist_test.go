package repository

import (
	"context"
	"errors"
	"regexp"
	"testing"

	"entgo.io/ent/dialect"
	entsql "entgo.io/ent/dialect/sql"
	"github.com/DATA-DOG/go-sqlmock"
	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func TestCodexGatewayBlacklistAppendLocksLatestAccountExtra(t *testing.T) {
	for _, tc := range []struct {
		name    string
		gateway string
		payload string
		fail    bool
	}{
		{name: "preserves earlier additions", gateway: "unified-35", payload: `{"openai_codex_ticket_gateway_blacklist":["unified-12","unified-35"]}`},
		{name: "deduplicates", gateway: "unified-12", payload: `{"openai_codex_ticket_gateway_blacklist":["unified-12"]}`},
		{name: "rollback", gateway: "unified-35", payload: `{"openai_codex_ticket_gateway_blacklist":["unified-12","unified-35"]}`, fail: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			db, mock, err := sqlmock.New()
			require.NoError(t, err)
			t.Cleanup(func() { _ = db.Close() })
			client := dbent.NewClient(dbent.Driver(entsql.OpenDB(dialect.Postgres, db)))
			t.Cleanup(func() { _ = client.Close() })
			mock.ExpectBegin()
			mock.ExpectQuery(`SELECT.*FROM accounts.*FOR NO KEY UPDATE`).WithArgs(int64(41)).
				WillReturnRows(sqlmock.NewRows([]string{"extra"}).AddRow([]byte(`{"other":true,"codex_turn_ticket:model":{"state":"latest"},"openai_codex_ticket_gateway_blacklist":["unified-12"]}`)))
			mock.ExpectExec(regexp.QuoteMeta("UPDATE accounts SET extra = COALESCE(extra, '{}'::jsonb) || $1::jsonb, updated_at = NOW() WHERE id = $2 AND deleted_at IS NULL")).
				WithArgs(tc.payload, int64(41)).WillReturnResult(sqlmock.NewResult(0, 1))
			outbox := mock.ExpectExec(regexp.QuoteMeta("INSERT INTO scheduler_outbox")).
				WithArgs(service.SchedulerOutboxEventAccountChanged, int64(41), nil, nil, sqlmock.AnyArg())
			if tc.fail {
				outbox.WillReturnError(errors.New("outbox unavailable"))
				mock.ExpectRollback()
			} else {
				outbox.WillReturnResult(sqlmock.NewResult(1, 1))
				mock.ExpectCommit()
			}
			repo := newAccountRepositoryWithSQL(client, db, nil)
			err = repo.AddCodexTicketGatewayToBlacklist(context.Background(), 41, tc.gateway)
			if tc.fail {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
			require.NoError(t, mock.ExpectationsWereMet())
		})
	}
}

func TestCodexGatewayBlacklistSchedulerProjection(t *testing.T) {
	key := service.OpenAICodexTicketGatewayBlacklistExtraKey
	gateways := []string{"unified-12", "unified-35"}
	require.Equal(t, gateways, filterSchedulerExtra(map[string]any{key: gateways})[key])
	require.True(t, shouldEnqueueSchedulerOutboxForExtraUpdates(map[string]any{key: gateways}))
}
