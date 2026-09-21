package service

import (
	"context"
	"errors"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
)

type codexTicketSchedulerRepo struct {
	schedulerTestOpenAIAccountRepo
	full map[int64]Account
}

func (r codexTicketSchedulerRepo) GetByID(_ context.Context, id int64) (*Account, error) {
	account, ok := r.full[id]
	if !ok {
		return nil, errors.New("account not found")
	}
	return &account, nil
}

func TestCodexTicketSchedulerSkipsUnreadyPrimaryAndUsesReadyFallback(t *testing.T) {
	cfg := &config.Config{}
	cfg.Gateway.OpenAICodexTicket = normalizeCodexTicketConfig(config.OpenAICodexTicketConfig{
		Enabled:    true,
		FailClosed: true,
		Models:     []string{"ticket-model"},
	})
	unready := Account{
		ID:          8101,
		Platform:    PlatformOpenAI,
		Type:        AccountTypeOAuth,
		Status:      StatusActive,
		Schedulable: true,
		Concurrency: 1,
		Credentials: map[string]any{
			"access_token":       "unready-token",
			"chatgpt_account_id": "unready-workspace",
			"plan_type":          "plus",
		},
	}
	ready := Account{
		ID:          8102,
		Platform:    PlatformOpenAI,
		Type:        AccountTypeOAuth,
		Status:      StatusActive,
		Schedulable: true,
		Concurrency: 1,
		IsFallback:  true,
		Credentials: map[string]any{
			"access_token":       "ready-token",
			"chatgpt_account_id": "ready-workspace",
			"plan_type":          "plus",
		},
	}
	unreadySnapshot := unready
	unreadySnapshot.Credentials = map[string]any{"plan_type": "plus"}
	readySnapshot := ready
	readySnapshot.Credentials = map[string]any{"plan_type": "plus"}
	snapshots := []Account{unreadySnapshot, readySnapshot}
	snapshotCache := &openAISnapshotCacheStub{
		snapshotAccounts: []*Account{&snapshots[0], &snapshots[1]},
		accountsByID:     map[int64]*Account{unready.ID: &snapshots[0], ready.ID: &snapshots[1]},
	}
	acquired := []int64{}
	svc := &OpenAIGatewayService{
		cfg:               cfg,
		schedulerSnapshot: &SchedulerSnapshotService{cache: snapshotCache},
		accountRepo: codexTicketSchedulerRepo{
			schedulerTestOpenAIAccountRepo: schedulerTestOpenAIAccountRepo{accounts: snapshots},
			full:                           map[int64]Account{unready.ID: unready, ready.ID: ready},
		},
		concurrencyService: NewConcurrencyService(schedulerTestConcurrencyCache{acquiredIDs: &acquired}),
	}
	installTicket(svc, &ready, "ticket-model")
	scheduler := &defaultOpenAIAccountScheduler{service: svc, stats: newOpenAIAccountRuntimeStats()}

	compatible, reason := scheduler.isAccountRequestCompatibleReason(context.Background(), &unreadySnapshot, OpenAIAccountScheduleRequest{
		Platform:       PlatformOpenAI,
		RequestedModel: "ticket-model",
	})
	require.False(t, compatible)
	require.Equal(t, "codex_ticket_unavailable", reason)

	compatible, reason = scheduler.isAccountRequestCompatibleReason(context.Background(), &readySnapshot, OpenAIAccountScheduleRequest{
		Platform:       PlatformOpenAI,
		RequestedModel: "ticket-model",
	})
	require.True(t, compatible)
	require.Empty(t, reason)

	selection, _, _, _, err := scheduler.selectByLoadBalance(context.Background(), OpenAIAccountScheduleRequest{
		Platform:       PlatformOpenAI,
		RequestedModel: "ticket-model",
	})
	require.NoError(t, err)
	require.NotNil(t, selection)
	require.NotNil(t, selection.Account)
	require.Equal(t, ready.ID, selection.Account.ID)
	require.Equal(t, []int64{ready.ID}, acquired)
	if selection.ReleaseFunc != nil {
		selection.ReleaseFunc()
	}
}

func TestCodexTicketSchedulerKeepsUngatedAccountsEligible(t *testing.T) {
	svc, account := ticketFixture()
	scheduler := &defaultOpenAIAccountScheduler{service: svc}

	compatible, reason := scheduler.isAccountRequestCompatibleReason(context.Background(), account, OpenAIAccountScheduleRequest{
		Platform:       PlatformOpenAI,
		RequestedModel: "ungated-model",
	})
	require.True(t, compatible)
	require.Empty(t, reason)
}

func TestCodexTicketSchedulerSnapshotValidatesWithoutSecrets(t *testing.T) {
	for _, plan := range []string{"plus", "team"} {
		t.Run(plan, func(t *testing.T) {
			svc, account := ticketFixture()
			account.Credentials["plan_type"] = plan
			installTicket(svc, account, "ticket-model")

			snapshot := *account
			snapshot.Credentials = map[string]any{"plan_type": plan}
			require.False(t, svc.codexTicketBlocksSchedulerSnapshot(context.Background(), &snapshot, "ticket-model", false))

			account.Credentials["access_token"] = "rotated-token"
			require.True(t, svc.codexTicketBlocksAccount(context.Background(), account, "ticket-model", false))
			require.False(t, svc.codexTicketBlocksSchedulerSnapshot(context.Background(), &snapshot, "ticket-model", false))
		})
	}
}
