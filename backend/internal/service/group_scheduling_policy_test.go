package service

import (
	"context"
	"fmt"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/ctxkey"
	"github.com/stretchr/testify/require"
)

func groupPolicyContext(strategy string, bps bool) context.Context {
	return context.WithValue(context.Background(), ctxkey.Group, &Group{ID: 7, Platform: PlatformOpenAI, Status: StatusActive, Hydrated: true, SchedulingStrategy: strategy})
}

func TestGroupSchedulingStrategyValidation(t *testing.T) {
	for _, tc := range []struct{ input, want string }{
		{"", GroupSchedulingBalanced}, {"balanced", GroupSchedulingBalanced},
		{" PRIORITY_5H ", GroupSchedulingPriority5h}, {"priority_weekly", GroupSchedulingPriorityWeekly},
	} {
		got, err := NormalizeGroupSchedulingStrategy(tc.input)
		require.NoError(t, err)
		require.Equal(t, tc.want, got)
	}
	_, err := NormalizeGroupSchedulingStrategy("random")
	require.Error(t, err)
	require.Equal(t, GroupSchedulingBalanced, (*Group)(nil).EffectiveSchedulingStrategy())
}

func quotaPolicyAccounts() []Account {
	return []Account{
		{ID: 1, Platform: PlatformOpenAI, Type: AccountTypeOAuth, Status: StatusActive, Schedulable: true, Concurrency: 1, Priority: 20, Extra: map[string]any{"codex_has_5h_limit": true}},
		{ID: 2, Platform: PlatformOpenAI, Type: AccountTypeOAuth, Status: StatusActive, Schedulable: true, Concurrency: 1, Priority: 10, Extra: map[string]any{"codex_has_5h_limit": false, "codex_7d_window_minutes": 10080, "codex_5h_used_percent": 0.0}},
		{ID: 3, Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Status: StatusActive, Schedulable: true, Concurrency: 1, Priority: 0},
	}
}

func TestGroupSchedulingStrategyLegacyAndAdvanced(t *testing.T) {
	for _, tc := range []struct {
		strategy string
		want     int64
	}{
		{GroupSchedulingBalanced, 3}, {GroupSchedulingPriority5h, 1}, {GroupSchedulingPriorityWeekly, 2},
	} {
		t.Run(tc.strategy, func(t *testing.T) {
			accounts := quotaPolicyAccounts()
			ctx := groupPolicyContext(tc.strategy, false)
			svc := &OpenAIGatewayService{}
			selected, _, _ := svc.selectBestAccount(ctx, nil, PlatformOpenAI, accounts, "gpt-5.1", nil, false, "", false)
			require.NotNil(t, selected)
			require.Equal(t, tc.want, selected.ID)
			cfg := &config.Config{}
			cfg.Gateway.Scheduling.LoadBatchEnabled = true
			svc = &OpenAIGatewayService{accountRepo: schedulerTestOpenAIAccountRepo{accounts: accounts}, cfg: cfg, concurrencyService: NewConcurrencyService(schedulerTestConcurrencyCache{})}
			selection, err := svc.selectAccountWithLoadAwareness(ctx, nil, PlatformOpenAI, "", "gpt-5.1", nil, false, "", false)
			require.NoError(t, err)
			require.NotNil(t, selection)
			require.Equal(t, tc.want, selection.Account.ID)
			if selection.ReleaseFunc != nil {
				selection.ReleaseFunc()
			}
			plan := openAIAccountLoadPlan{topK: 1}
			for i := range accounts {
				plan.candidates = append(plan.candidates, openAIAccountCandidateScore{account: &accounts[i], loadInfo: &AccountLoadInfo{}, score: float64(i)})
			}
			order := (&defaultOpenAIAccountScheduler{}).buildOpenAISelectionOrder(OpenAIAccountScheduleRequest{Platform: PlatformOpenAI, SchedulingStrategy: tc.strategy}, plan)
			require.NotEmpty(t, order)
			require.Equal(t, tc.want, order[0].account.ID)
			if tc.strategy != GroupSchedulingBalanced {
				require.GreaterOrEqual(t, len(order), 2, "retain regular candidates for fallback")
			}
		})
	}
}

func TestGroupSchedulingStrategyFallbackAndQuotaClassification(t *testing.T) {
	accounts := quotaPolicyAccounts()
	for _, strategy := range []string{GroupSchedulingPriority5h, GroupSchedulingPriorityWeekly} {
		excluded := map[int64]struct{}{1: {}, 2: {}}
		selected, _, _ := (&OpenAIGatewayService{}).selectBestAccount(groupPolicyContext(strategy, false), nil, PlatformOpenAI, accounts, "gpt-5.1", excluded, false, "", false)
		require.NotNil(t, selected)
		require.Equal(t, int64(3), selected.ID)
	}
	both := &Account{Platform: PlatformOpenAI, Type: AccountTypeOAuth, Extra: map[string]any{"codex_5h_used_percent": 0.0, "codex_7d_used_percent": 0.0}}
	require.Equal(t, 0, openAIGroupQuotaRank(both, GroupSchedulingPriority5h))
	require.Equal(t, 1, openAIGroupQuotaRank(both, GroupSchedulingPriorityWeekly))
	require.Equal(t, 1, openAIGroupQuotaRank(&Account{}, GroupSchedulingPriorityWeekly))
	require.Equal(t, 0, openAIGroupQuotaRank(&accounts[1], GroupSchedulingPriorityWeekly), "explicit no-5h marker overrides stale 5h fields")
}

func TestGroupSchedulingAdvancedSelectAndBusyFallback(t *testing.T) {
	for _, tc := range []struct {
		strategy  string
		preferred int64
	}{
		{GroupSchedulingBalanced, 3}, {GroupSchedulingPriority5h, 1}, {GroupSchedulingPriorityWeekly, 2},
	} {
		for _, busy := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/busy=%t", tc.strategy, busy), func(t *testing.T) {
				cfg := &config.Config{}
				cfg.Gateway.OpenAIWS.LBTopK = 1
				cfg.Gateway.OpenAIWS.SchedulerScoreWeights.Priority = 1
				accounts := quotaPolicyAccounts()
				cache := schedulerTestConcurrencyCache{acquireResults: map[int64]bool{tc.preferred: !busy}}
				svc := &OpenAIGatewayService{cfg: cfg, accountRepo: schedulerTestOpenAIAccountRepo{accounts: accounts}, concurrencyService: NewConcurrencyService(cache)}
				scheduler := newDefaultOpenAIAccountScheduler(svc, nil)
				selection, _, err := scheduler.Select(groupPolicyContext(tc.strategy, false), OpenAIAccountScheduleRequest{Platform: PlatformOpenAI, RequestedModel: "gpt-5.1"})
				require.NoError(t, err)
				require.NotNil(t, selection)
				if !busy {
					require.Equal(t, tc.preferred, selection.Account.ID)
				}
				if busy && tc.strategy != GroupSchedulingBalanced {
					require.Equal(t, int64(3), selection.Account.ID)
					require.True(t, selection.Acquired)
				}
				if selection.ReleaseFunc != nil {
					selection.ReleaseFunc()
				}
			})
		}
	}
}
