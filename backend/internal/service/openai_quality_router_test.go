package service

import (
	"context"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
)

func qualityTestGateway(allow bool) (*OpenAIGatewayService, OpenAIAccountScheduleRequest) {
	group := int64(1)
	cfg := DefaultGPTQoSConfig()
	cfg.QualityRoutingEnabled = true
	g := DefaultGPTQoSGroupConfig("standard")
	g.AllowBasicFallback = allow
	g.StrongWaitMs = 0
	cfg.Groups["1"] = g
	settings := &SettingService{settingRepo: &openAIAdvancedSchedulerSettingRepoStub{}}
	settings.gptQoSConfig.config = cfg
	settings.gptQoSConfig.expires = time.Now().Add(time.Hour)
	accounts := []Account{
		{ID: 1, Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Status: StatusActive, Schedulable: true, Concurrency: 2, Priority: 100, GroupIDs: []int64{1}, Extra: map[string]any{AccountQualityTierKey: "strong"}},
		{ID: 2, Platform: PlatformOpenAI, Type: AccountTypeOAuth, Status: StatusActive, Schedulable: true, Concurrency: 2, Priority: 0, GroupIDs: []int64{1}, Extra: map[string]any{AccountQualityTierKey: "basic"}},
		{ID: 3, Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Status: StatusActive, Schedulable: true, Concurrency: 2, Priority: -100, GroupIDs: []int64{1}},
	}
	svc := &OpenAIGatewayService{settingService: settings, accountRepo: schedulerTestOpenAIAccountRepo{accounts: accounts}, concurrencyService: NewConcurrencyService(schedulerTestConcurrencyCache{}), cfg: &config.Config{}}
	return svc, OpenAIAccountScheduleRequest{GroupID: &group, Platform: PlatformOpenAI, RequestedModel: "gpt-test", UseUpstreamTokenCost: true}
}
func TestGPTQualityStrongBeforePriorityProtocolAndSubscription(t *testing.T) {
	for _, subscription := range []bool{false, true} {
		svc, req := qualityTestGateway(true)
		req.SubscriptionPriority = subscription
		selection, _, err := newDefaultOpenAIAccountScheduler(svc, nil).Select(context.Background(), req)
		require.NoError(t, err)
		require.Equal(t, int64(1), selection.Account.ID)
		selection.ReleaseFunc()
	}
}
func TestGPTQualityFallbackPermissionAndUnknown(t *testing.T) {
	for _, allow := range []bool{false, true} {
		svc, req := qualityTestGateway(allow)
		svc.concurrencyService = NewConcurrencyService(schedulerTestConcurrencyCache{acquireResults: map[int64]bool{1: false, 2: true, 3: true}})
		selection, _, err := newDefaultOpenAIAccountScheduler(svc, nil).Select(context.Background(), req)
		if allow {
			require.NoError(t, err)
			require.Equal(t, int64(2), selection.Account.ID)
			selection.ReleaseFunc()
		} else {
			require.ErrorIs(t, err, ErrGPTQualityUnavailable)
			require.Nil(t, selection)
		}
	}
}
func TestGPTQualityIncompatibleStrongDoesNotBlockBasic(t *testing.T) {
	svc, req := qualityTestGateway(true)
	repo := svc.accountRepo.(schedulerTestOpenAIAccountRepo)
	repo.accounts[0].Credentials = map[string]any{"model_mapping": map[string]any{"other": "other"}}
	svc.accountRepo = repo
	selection, _, err := newDefaultOpenAIAccountScheduler(svc, nil).Select(context.Background(), req)
	require.NoError(t, err)
	require.Equal(t, int64(2), selection.Account.ID)
	selection.ReleaseFunc()
}
func TestGPTQualityLabelsAreExplicitAndValidated(t *testing.T) {
	for _, typ := range []string{AccountTypeAPIKey, AccountTypeOAuth} {
		a := &Account{Type: typ, Extra: map[string]any{}}
		require.Equal(t, "unknown", a.QualityTier())
		a.Extra[AccountQualityTierKey] = "strong"
		require.Equal(t, "strong", a.QualityTier())
	}
	ctx := WithQualityAuditActor(context.Background(), 42)
	extra := map[string]any{AccountQualityTierKey: "basic", "quality_tier_actor_id": 999}
	require.NoError(t, ValidateQualityExtra(ctx, extra))
	require.Equal(t, int64(42), extra["quality_tier_actor_id"])
	require.Error(t, ValidateQualityExtra(ctx, map[string]any{AccountQualityTierKey: "premium"}))
	require.Error(t, ValidateQualityExtra(ctx, map[string]any{AccountResourcePoolKey: "http://secret/key"}))
}
func TestGPTQualityForcedContinuationRetainsBasicOwner(t *testing.T) {
	svc, req := qualityTestGateway(false)
	svc.cache = &schedulerTestGatewayCache{}
	svc.cfg.Gateway.OpenAIWS.Enabled = true
	svc.cfg.Gateway.OpenAIWS.APIKeyEnabled = true
	svc.cfg.Gateway.OpenAIWS.ResponsesWebsocketsV2 = true
	repo := svc.accountRepo.(schedulerTestOpenAIAccountRepo)
	repo.accounts[1].Type = AccountTypeAPIKey
	repo.accounts[1].Extra["openai_apikey_responses_websockets_v2_enabled"] = true
	svc.accountRepo = repo
	require.NoError(t, svc.getOpenAIWSStateStore().BindResponseAccount(context.Background(), 1, "resp_owner", 2, time.Hour))
	req.PreviousResponseID = "resp_owner"
	selection, decision, err := newDefaultOpenAIAccountScheduler(svc, nil).Select(context.Background(), req)
	require.NoError(t, err)
	require.Equal(t, int64(2), selection.Account.ID)
	require.True(t, decision.StickyPreviousHit)
	selection.ReleaseFunc()
}
func TestGPTQoSConfigDefaultsAndValidation(t *testing.T) {
	c := DefaultGPTQoSConfig()
	require.False(t, c.Enabled)
	require.False(t, c.QualityRoutingEnabled)
	require.False(t, c.CapacityEnabled)
	require.False(t, c.Peak.Enabled)
	require.NoError(t, ValidateGPTQoSConfig(c))
	c.Enabled = true
	require.Error(t, ValidateGPTQoSConfig(c))
	c.QualityRoutingEnabled = true
	require.NoError(t, ValidateGPTQoSConfig(c))
	require.False(t, DefaultGPTQoSGroupConfig("premium").AllowBasicFallback)
	require.True(t, DefaultGPTQoSGroupConfig("economy").AllowBasicFallback)
	c.StandardWeight = 6
	require.Equal(t, 3, c.Group(nil).Weight, "the tier's configured weight must be applied only once")
}

func TestGPTQualityFailoverKeepsDowngradePermission(t *testing.T) {
	for _, allow := range []bool{false, true} {
		svc, req := qualityTestGateway(allow)
		req.ExcludedIDs = map[int64]struct{}{1: {}}
		selection, _, err := newDefaultOpenAIAccountScheduler(svc, nil).Select(context.Background(), req)
		if allow {
			require.NoError(t, err)
			require.Equal(t, "basic", selection.Account.QualityTier())
			selection.ReleaseFunc()
		} else {
			require.ErrorIs(t, err, ErrGPTQualityUnavailable)
			require.Nil(t, selection, "failover exclusion does not authorize silent downgrade")
		}
	}
}

func TestGPTQualityHardParentDoesNotEscapeUnavailableOwner(t *testing.T) {
	svc, req := qualityTestGateway(true)
	svc.cache = &schedulerTestGatewayCache{}
	req.SessionHash = "parent-chain"
	req.GuardianParentAccountID = 999
	req.PreserveStickyBinding = true
	selection, _, err := newDefaultOpenAIAccountScheduler(svc, nil).Select(context.Background(), req)
	require.ErrorIs(t, err, ErrNoAvailableAccounts)
	require.Nil(t, selection, "a missing hard parent must not become a free strong-pool request")
}

func TestGPTQoSBoundSingleProxyUsesExistingSemaphoreID(t *testing.T) {
	proxyID := int64(12)
	a := &Account{ID: 31, ProxyID: &proxyID, Concurrency: 2}
	c, ok := qosCandidate(a, DefaultGPTQoSConfig())
	require.True(t, ok)
	require.Equal(t, AccountProxySlotID(a.ID, proxyID), c.SlotID)
	require.NotEqual(t, a.ID, c.SlotID)
	a.ProxyPoolConfigured = true
	_, ok = qosCandidate(a, DefaultGPTQoSConfig())
	require.False(t, ok, "explicit multi-binding capacity requires a separate lifecycle")
}
