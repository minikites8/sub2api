package service

import (
	"context"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
)

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
	accounts := []Account{unready, ready}
	acquired := []int64{}
	svc := &OpenAIGatewayService{
		cfg:                cfg,
		accountRepo:        schedulerTestOpenAIAccountRepo{accounts: accounts},
		concurrencyService: NewConcurrencyService(schedulerTestConcurrencyCache{acquiredIDs: &acquired}),
	}
	installTicket(svc, &ready, "ticket-model")
	scheduler := &defaultOpenAIAccountScheduler{service: svc, stats: newOpenAIAccountRuntimeStats()}

	compatible, reason := scheduler.isAccountRequestCompatibleReason(context.Background(), &unready, OpenAIAccountScheduleRequest{
		Platform:       PlatformOpenAI,
		RequestedModel: "ticket-model",
	})
	require.False(t, compatible)
	require.Equal(t, "codex_ticket_unavailable", reason)

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
