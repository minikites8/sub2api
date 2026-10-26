package service

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestOpenAIFallbackCapacityOverflowAndPrimaryRecovery(t *testing.T) {
	for _, mode := range []string{"legacy-no-batch", "legacy-batch", "advanced", "priority", "quality"} {
		t.Run(mode, func(t *testing.T) {
			svc, req := qualityTestGateway(false)
			svc.settingService.gptQoSConfig.config.QualityRoutingEnabled = mode == "quality"
			svc.cfg.Gateway.Scheduling.LoadBatchEnabled = mode != "legacy-no-batch"
			req.RequiredTransport = OpenAIUpstreamTransportAny
			if mode == "priority" {
				cfg := DefaultPrioritySchedulingConfig()
				cfg.Enabled = true
				svc.settingService.prioritySchedulingConfig.config = cfg
				svc.settingService.prioritySchedulingConfig.expires = time.Now().Add(time.Hour)
				svc.usageLogRepo = &priorityReaderStub{}
			}
			repo := svc.accountRepo.(schedulerTestOpenAIAccountRepo)
			repo.accounts = repo.accounts[:2]
			repo.accounts[1].Type = AccountTypeAPIKey
			repo.accounts[1].IsFallback = true
			repo.accounts[1].Extra[AccountQualityTierKey] = "strong"
			repo.accounts[1].Priority = -100 // soft preferences cannot outrank a primary slot
			svc.accountRepo = repo
			svc.cache = &schedulerTestGatewayCache{sessionBindings: map[string]int64{}}
			req.SessionHash = "fallback-soft-session"
			loads := map[int64]*AccountLoadInfo{1: {AccountID: 1}, 2: {AccountID: 2}}
			acquire := map[int64]bool{1: true, 2: true}
			svc.concurrencyService = NewConcurrencyService(schedulerTestConcurrencyCache{loadMap: loads, acquireResults: acquire})
			selectAccount := func(want int64) {
				t.Helper()
				var result *AccountSelectionResult
				var err error
				if mode == "legacy-no-batch" || mode == "legacy-batch" {
					result, err = svc.selectAccountWithLoadAwareness(context.Background(), req.GroupID, req.Platform, req.SessionHash, req.RequestedModel, nil, false, "", true)
				} else {
					result, _, err = newDefaultOpenAIAccountScheduler(svc, nil).Select(context.Background(), req)
				}
				require.NoError(t, err)
				require.NotNil(t, result)
				require.True(t, result.Acquired, "available standby must be acquired before returning a primary wait plan")
				require.Equal(t, want, result.Account.ID)
				result.ReleaseFunc()
			}
			selectAccount(1)
			acquire[1] = false
			loads[1].LoadRate, loads[1].CurrentConcurrency = 100, repo.accounts[0].Concurrency
			selectAccount(2) // primary remains healthy and schedulable, but is full
			acquire[1] = true
			loads[1].LoadRate, loads[1].CurrentConcurrency = 0, 0
			selectAccount(1) // a prior standby/soft binding must not pin new work
		})
	}
}

func TestOpenAIFallbackCannotBypassRemainingPrimaryOutsideTopK(t *testing.T) {
	svc, req := qualityTestGateway(false)
	svc.settingService.gptQoSConfig.config.QualityRoutingEnabled = false
	req.RequiredTransport = OpenAIUpstreamTransportAny
	req.SessionHash = "fixed-seed"
	repo := svc.accountRepo.(schedulerTestOpenAIAccountRepo)
	repo.accounts[1].Type = AccountTypeAPIKey
	repo.accounts[1].IsFallback = true
	repo.accounts[1].Priority = -1000
	repo.accounts[2].Priority = 1000
	svc.accountRepo = repo
	svc.cfg.Gateway.OpenAIWS.LBTopK = 1
	svc.concurrencyService = NewConcurrencyService(schedulerTestConcurrencyCache{acquireResults: map[int64]bool{1: false, 2: true, 3: true}})
	result, _, err := newDefaultOpenAIAccountScheduler(svc, nil).Select(context.Background(), req)
	require.NoError(t, err)
	require.True(t, result.Acquired)
	require.Equal(t, int64(3), result.Account.ID, "primary Top-K overflow precedes standby even when standby's score is better")
	result.ReleaseFunc()
}

func TestOpenAIFallbackDoesNotBreakRequiredContinuation(t *testing.T) {
	for _, owner := range []string{"previous_response_id", "guardian_parent"} {
		t.Run(owner, func(t *testing.T) {
			svc, req := qualityTestGateway(false)
			svc.cache = &schedulerTestGatewayCache{}
			repo := svc.accountRepo.(schedulerTestOpenAIAccountRepo)
			repo.accounts[1].Type = AccountTypeAPIKey
			repo.accounts[1].IsFallback = true
			svc.accountRepo = repo
			req.RequiredTransport = OpenAIUpstreamTransportAny
			req.SessionHash = "required-owner-session"
			if owner == "previous_response_id" {
				require.NoError(t, svc.getOpenAIWSStateStore().BindResponseAccount(context.Background(), 1, "resp_backup_owner", 2, time.Hour))
				req.PreviousResponseID = "resp_backup_owner"
			} else {
				req.GuardianParentAccountID = 2
				req.PreserveStickyBinding = true
			}
			result, _, err := newDefaultOpenAIAccountScheduler(svc, nil).Select(context.Background(), req)
			require.NoError(t, err)
			require.Equal(t, int64(2), result.Account.ID, "free primary capacity cannot move an upstream-owned continuation")
			result.ReleaseFunc()
		})
	}
}

func TestOpenAIFallbackUnsupportedModelStillWaitsOnPrimary(t *testing.T) {
	svc, req := qualityTestGateway(false)
	svc.settingService.gptQoSConfig.config.QualityRoutingEnabled = false
	req.RequiredTransport = OpenAIUpstreamTransportAny
	repo := svc.accountRepo.(schedulerTestOpenAIAccountRepo)
	repo.accounts = repo.accounts[:2]
	repo.accounts[1].IsFallback = true
	repo.accounts[1].Credentials = map[string]any{"model_mapping": map[string]any{"other": "other"}}
	svc.accountRepo = repo
	svc.concurrencyService = NewConcurrencyService(schedulerTestConcurrencyCache{acquireResults: map[int64]bool{1: false, 2: true}})
	result, _, err := newDefaultOpenAIAccountScheduler(svc, nil).Select(context.Background(), req)
	require.NoError(t, err)
	require.False(t, result.Acquired)
	require.NotNil(t, result.WaitPlan)
	require.Equal(t, int64(1), result.Account.ID)
}

func TestOpenAIFallbackMovablePreviousResponseReturnsToPrimary(t *testing.T) {
	for _, mode := range []string{"legacy", "advanced", "quality"} {
		t.Run(mode, func(t *testing.T) {
			svc, req := qualityTestGateway(false)
			svc.settingService.gptQoSConfig.config.QualityRoutingEnabled = mode == "quality"
			svc.cache = &schedulerTestGatewayCache{}
			repo := svc.accountRepo.(schedulerTestOpenAIAccountRepo)
			repo.accounts = repo.accounts[:2]
			repo.accounts[1].Type = AccountTypeAPIKey
			repo.accounts[1].IsFallback = true
			svc.accountRepo = repo
			req.RequiredTransport = OpenAIUpstreamTransportAny
			req.PreviousResponseID = "resp_movable_backup"
			req.PreviousResponseCanMove = true
			require.NoError(t, svc.getOpenAIWSStateStore().BindResponseAccount(context.Background(), 1, req.PreviousResponseID, 2, time.Hour))
			var result *AccountSelectionResult
			var err error
			if mode == "legacy" {
				resetOpenAIAdvancedSchedulerSettingCacheForTest()
				defer resetOpenAIAdvancedSchedulerSettingCacheForTest()
				result, _, err = svc.selectAccountWithSchedulerOnce(context.Background(), req.GroupID, req.PreviousResponseID, "", req.RequestedModel, nil, req.RequiredTransport, "", "", false, req.Platform, true, true)
			} else {
				result, _, err = newDefaultOpenAIAccountScheduler(svc, nil).Select(context.Background(), req)
			}
			require.NoError(t, err)
			require.Equal(t, int64(1), result.Account.ID, "complete transferable context may return to primary")
			result.ReleaseFunc()
		})
	}
}
