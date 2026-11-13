package service

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/ctxkey"
	"github.com/stretchr/testify/require"
)

// Exercise the real service entry: the client supplies a conversation identity,
// never an account ID or an admission preference.
func TestGPTQualityConversationAffinityThroughScheduler(t *testing.T) {
	for _, queue := range []bool{false, true} {
		t.Run(fmt.Sprintf("queue=%t", queue), func(t *testing.T) {
			resetOpenAIAdvancedSchedulerSettingCacheForTest()
			defer resetOpenAIAdvancedSchedulerSettingCacheForTest()
			svc, req := qualityTestGateway(false)
			svc.settingService.gptQoSConfig.config.Enabled = queue
			repo := qualityTestAccountRepo(t, svc)
			repo.accounts[1].Extra[AccountQualityTierKey] = "strong"
			svc.accountRepo = repo
			svc.cache = &schedulerTestGatewayCache{sessionBindings: map[string]int64{}}
			ctx := context.WithValue(context.Background(), ctxkey.UserID, int64(42))
			require.NoError(t, svc.BindStickySession(ctx, req.GroupID, "conversation", 2))
			loads := schedulerTestConcurrencyCache{loadMap: map[int64]*AccountLoadInfo{
				1: {AccountID: 1}, 2: {AccountID: 2, CurrentConcurrency: 1, LoadRate: 50},
			}}
			var recorder *qosAdmissionRecordingCache
			if queue {
				recorder = &qosAdmissionRecordingCache{schedulerTestConcurrencyCache: loads}
				svc.concurrencyService = NewConcurrencyService(recorder)
			} else {
				svc.concurrencyService = NewConcurrencyService(loads)
			}
			for turn := 0; turn < 12; turn++ {
				result, decision, err := svc.SelectAccountWithScheduler(ctx, req.GroupID, "", "conversation", req.RequestedModel, nil, OpenAIUpstreamTransportAny, false)
				require.NoError(t, err)
				require.True(t, result.Acquired)
				result.ReleaseFunc()
				require.Equal(t, int64(2), result.Account.ID, "a healthy same-conversation owner with free capacity should beat an idle peer")
				require.True(t, decision.StickySessionHit)
			}
			if queue {
				require.Len(t, recorder.entries, 12, "affinity must not bypass global fair admission")
			}
		})
	}
}

func TestGPTQualityMovableResponseAffinityThroughScheduler(t *testing.T) {
	resetOpenAIAdvancedSchedulerSettingCacheForTest()
	defer resetOpenAIAdvancedSchedulerSettingCacheForTest()
	for _, queue := range []bool{false, true} {
		t.Run(fmt.Sprintf("queue=%t", queue), func(t *testing.T) {
			svc, req := qualityTestGateway(false)
			svc.settingService.gptQoSConfig.config.Enabled = queue
			repo := qualityTestAccountRepo(t, svc)
			repo.accounts[1].Type = AccountTypeAPIKey
			repo.accounts[1].Extra[AccountQualityTierKey] = "strong"
			svc.accountRepo = repo
			svc.cache = &schedulerTestGatewayCache{}
			ctx := context.WithValue(context.Background(), ctxkey.UserID, int64(42))
			require.NoError(t, svc.getOpenAIWSStateStore().BindResponseAccount(ctx, 1, "resp_soft_owner", 2, time.Hour))
			state := &gptQoSRequestState{config: svc.settingService.gptQoSConfig.config}
			ctx = context.WithValue(ctx, gptQoSRequestKey{}, state)
			loads := schedulerTestConcurrencyCache{loadMap: map[int64]*AccountLoadInfo{1: {AccountID: 1}, 2: {AccountID: 2, LoadRate: 50}}}
			if queue {
				svc.concurrencyService = NewConcurrencyService(&qosAdmissionRecordingCache{schedulerTestConcurrencyCache: loads})
			} else {
				svc.concurrencyService = NewConcurrencyService(loads)
			}
			result, decision, err := svc.SelectAccountWithSchedulerForCapability(ctx, req.GroupID, "resp_soft_owner", "", req.RequestedModel, nil, OpenAIUpstreamTransportAny, "", false, true, true)
			require.NoError(t, err)
			result.ReleaseFunc()
			require.Equal(t, int64(2), result.Account.ID)
			require.True(t, decision.StickyPreviousHit, "movable context is a preference, not a reason to discard cache locality")
			require.Equal(t, "cache_affinity_previous_response", state.sticky)
			if queue {
				require.Equal(t, "quality_pool_global_weighted_fair_queue", state.strategy, "a soft response hit must not be reported as hard-owner bypass")
			}
		})
	}
}

func TestGPTQualityCacheAffinityConstraints(t *testing.T) {
	for _, scenario := range []string{"full", "excluded", "disabled", "model", "basic", "unknown", "standby", "errors", "slow", "no_session"} {
		t.Run(scenario, func(t *testing.T) {
			svc, req := qualityTestGateway(true)
			req.RequiredTransport = OpenAIUpstreamTransportAny
			req.SessionHash, req.StickyAccountID = "conversation", 2
			svc.cache = &schedulerTestGatewayCache{sessionBindings: map[string]int64{}}
			repo := qualityTestAccountRepo(t, svc)
			repo.accounts[1].Extra[AccountQualityTierKey] = "strong"
			acquire := map[int64]bool{1: true, 2: true}
			scheduler, ok := newDefaultOpenAIAccountScheduler(svc, nil).(*defaultOpenAIAccountScheduler)
			require.True(t, ok, "quality test requires the default scheduler")
			switch scenario {
			case "full":
				acquire[2] = false // stale load snapshot still reports spare capacity
			case "excluded":
				req.ExcludedIDs = map[int64]struct{}{2: {}}
			case "disabled":
				repo.accounts[1].Schedulable = false
			case "model":
				repo.accounts[1].Credentials = map[string]any{"model_mapping": map[string]any{"other": "other"}}
			case "basic", "unknown":
				repo.accounts[1].Extra[AccountQualityTierKey] = scenario
			case "standby":
				repo.accounts[1].IsFallback = true
			case "errors":
				for i := 0; i < 10; i++ {
					scheduler.ReportResult(2, false, nil)
				}
			case "slow":
				ttft := 30000
				scheduler.ReportResult(2, true, &ttft)
			case "no_session":
				req.SessionHash = ""
			}
			svc.accountRepo = repo
			svc.concurrencyService = NewConcurrencyService(schedulerTestConcurrencyCache{
				acquireResults: acquire,
				loadMap:        map[int64]*AccountLoadInfo{1: {AccountID: 1}, 2: {AccountID: 2, LoadRate: 50}},
			})
			result, _, err := scheduler.Select(context.Background(), req)
			require.NoError(t, err)
			result.ReleaseFunc()
			require.Equal(t, int64(1), result.Account.ID)
		})
	}
}

func TestGPTQualityCacheAffinityRecordsAndRebinds(t *testing.T) {
	svc, req := qualityTestGateway(false)
	req.RequiredTransport = OpenAIUpstreamTransportAny
	req.SessionHash, req.StickyAccountID = "conversation", 2
	svc.cache = &schedulerTestGatewayCache{sessionBindings: map[string]int64{}}
	repo := qualityTestAccountRepo(t, svc)
	repo.accounts[1].Extra[AccountQualityTierKey] = "strong"
	svc.accountRepo = repo
	state := &gptQoSRequestState{config: svc.settingService.gptQoSConfig.config}
	ctx := context.WithValue(context.Background(), gptQoSRequestKey{}, state)
	scheduler := newDefaultOpenAIAccountScheduler(svc, nil)
	result, decision, err := scheduler.Select(ctx, req)
	require.NoError(t, err)
	result.ReleaseFunc()
	require.Equal(t, int64(2), result.Account.ID)
	require.True(t, decision.StickySessionHit)
	require.Equal(t, "cache_affinity_session", state.sticky)
	// A subsequent failover must not retain the old hit reason in the log.
	req.ExcludedIDs = map[int64]struct{}{2: {}}
	result, decision, err = scheduler.Select(ctx, req)
	require.NoError(t, err)
	result.ReleaseFunc()
	require.Equal(t, int64(1), result.Account.ID)
	require.False(t, decision.StickySessionHit)
	require.Empty(t, state.sticky)
	bound, err := svc.getStickySessionAccountID(ctx, req.GroupID, req.SessionHash)
	require.NoError(t, err)
	require.Equal(t, int64(1), bound)
}
