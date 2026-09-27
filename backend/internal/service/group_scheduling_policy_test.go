package service

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/ctxkey"
	"github.com/stretchr/testify/require"
)

func groupPolicyContext(strategy string, bps bool) context.Context {
	return context.WithValue(context.Background(), ctxkey.Group, &Group{ID: 7, Platform: PlatformOpenAI, Status: StatusActive, Hydrated: true, EnableBPS: bps, SchedulingStrategy: strategy})
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

func TestGroupBPSForwardRequiresBothSwitches(t *testing.T) {
	for _, groupEnabled := range []bool{false, true} {
		for _, accountEnabled := range []bool{false, true} {
			t.Run(fmt.Sprintf("group=%t/account=%t", groupEnabled, accountEnabled), func(t *testing.T) {
				account := excelAccount()
				account.Extra["openai_excel_bps"] = accountEnabled
				upstream := &httpUpstreamRecorder{responses: []*http.Response{excelBPSOrdinaryResponse()}}
				svc := openAIClientToolsTestService(upstream)
				svc.accountRepo = &turnAdmissionRepo{account: account}
				body := []byte(`{"model":"gpt-5.6-sol","stream":false,"input":"hello"}`)
				c, _ := newExcelBPSFallbackContext(body, false)
				result, err := svc.Forward(groupPolicyContext(GroupSchedulingBalanced, groupEnabled), c, account, body)
				require.NoError(t, err)
				require.NotNil(t, result)
				require.Len(t, upstream.requests, 1)
				if groupEnabled && accountEnabled {
					require.Equal(t, "/basispoints/api/responses", upstream.requests[0].URL.Path)
				} else {
					require.Equal(t, chatgptCodexURL, upstream.requests[0].URL.String())
				}
				require.Equal(t, accountEnabled, account.IsExcelBPSEnabled(), "shared account setting remains unchanged")
			})
		}
	}
}

func TestGroupBPSSchedulerAndManifestUsePlatformWhenDisabled(t *testing.T) {
	account := excelAccount()
	svc := &OpenAIGatewayService{accountRepo: schedulerTestOpenAIAccountRepo{accounts: []Account{*account}}}
	accounts, err := svc.listSchedulableAccountsForRequest(groupPolicyContext(GroupSchedulingBalanced, false), nil, PlatformOpenAI, "gpt-5.6-sol", false, nil)
	require.NoError(t, err)
	require.Len(t, accounts, 1)
	require.False(t, accounts[0].IsExcelBPSEnabled())
	fresh := svc.recheckSelectedOpenAIAccountFromDBBeforeProfit(groupPolicyContext(GroupSchedulingBalanced, false), account, nil, PlatformOpenAI, "gpt-5.6-sol", false, "")
	require.NotNil(t, fresh)
	require.False(t, fresh.IsExcelBPSEnabled())
	require.True(t, account.IsExcelBPSEnabled())
	body := []byte(`{"models":[{"slug":"gpt-5.6-sol","multi_agent_version":"1"}]}`)
	group := &Group{ID: 7}
	got, changed, err := restrictExcelBPSCodexModelsManifest(body, []Account{*account}, group)
	require.NoError(t, err)
	require.False(t, changed)
	require.JSONEq(t, string(body), string(got))
	group.EnableBPS = true
	_, changed, err = restrictExcelBPSCodexModelsManifest(body, []Account{*account}, group)
	require.NoError(t, err)
	require.True(t, changed)
	groupID := int64(99)
	scoped := svc.openAIAccountForGroup(context.Background(), &groupID, account)
	require.False(t, scoped.IsExcelBPSEnabled(), "missing group requires explicit opt-in")
	encoded, err := json.Marshal(scoped)
	require.NoError(t, err)
	require.NotContains(t, string(encoded), "bpsDisabledByGroup")
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

func TestGroupBPSDisabledKeepsNativeWebSocketStickyAccount(t *testing.T) {
	account := excelAccount()
	account.Extra["openai_oauth_responses_websockets_v2_enabled"] = true
	cache := &schedulerTestGatewayCache{}
	svc := &OpenAIGatewayService{cfg: newSchedulerTestOpenAIWSV2Config(), accountRepo: schedulerTestOpenAIAccountRepo{accounts: []Account{*account}}, cache: cache}
	require.NoError(t, svc.BindStickySession(context.Background(), nil, "group-native", account.ID))
	scheduler := newDefaultOpenAIAccountScheduler(svc, nil)
	selection, decision, err := scheduler.Select(groupPolicyContext(GroupSchedulingBalanced, false), OpenAIAccountScheduleRequest{
		Platform: PlatformOpenAI, SessionHash: "group-native", RequestedModel: "gpt-5.6-sol", RequiredTransport: OpenAIUpstreamTransportResponsesWebsocketV2,
	})
	require.NoError(t, err)
	require.NotNil(t, selection)
	require.Equal(t, account.ID, selection.Account.ID)
	require.Equal(t, openAIAccountScheduleLayerSessionSticky, decision.Layer)
	require.False(t, selection.Account.IsExcelBPSEnabled())
	require.True(t, account.IsExcelBPSEnabled())
	require.Empty(t, cache.deletedSessions)
	if selection.ReleaseFunc != nil {
		selection.ReleaseFunc()
	}
}
