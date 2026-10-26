package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/ctxkey"
	"github.com/stretchr/testify/require"
)

type qosAdmissionRecordingCache struct {
	schedulerTestConcurrencyCache
	GPTQoSAdmissionStore
	entries []GPTQoSEntry
	closed  []string
	onPoll  func()
}

func (c *qosAdmissionRecordingCache) GPTQoSPoll(_ context.Context, e GPTQoSEntry) (GPTQoSGrant, error) {
	e.Candidates = append([]GPTQoSCandidate(nil), e.Candidates...)
	c.entries = append(c.entries, e)
	if c.onPoll != nil {
		c.onPoll()
	}
	return GPTQoSGrant{Status: "granted", Candidate: e.Candidates[0]}, nil
}
func (c *qosAdmissionRecordingCache) GPTQoSClose(_ context.Context, id string) error {
	c.closed = append(c.closed, id)
	return nil
}

func TestGPTQoSModelFilteringPrecedesQualityAndFairQueue(t *testing.T) {
	for _, allow := range []bool{false, true} {
		svc, req := qualityTestGateway(allow)
		req.RequiredTransport = OpenAIUpstreamTransportAny
		svc.settingService.gptQoSConfig.config.Enabled = true
		repo := svc.accountRepo.(schedulerTestOpenAIAccountRepo)
		repo.accounts[0].Credentials = map[string]any{"model_mapping": map[string]any{"other-model": "other-model"}}
		svc.accountRepo = repo
		cache := &qosAdmissionRecordingCache{}
		svc.concurrencyService = NewConcurrencyService(cache)
		ctx := context.WithValue(context.Background(), ctxkey.UserID, int64(42))
		selection, _, err := newDefaultOpenAIAccountScheduler(svc, nil).Select(ctx, req)
		if !allow {
			require.ErrorIs(t, err, ErrNoAvailableAccounts)
			require.Nil(t, selection)
			require.Empty(t, cache.entries, "unsupported Strong and unauthorized Basic/Unknown cannot enter QoS")
			continue
		}
		require.NoError(t, err)
		selection.ReleaseFunc()
		require.Equal(t, int64(2), selection.Account.ID)
		require.Len(t, cache.entries, 1)
		require.Len(t, cache.entries[0].Candidates, 1)
		require.Equal(t, int64(2), cache.entries[0].Candidates[0].AccountID)
		require.Zero(t, cache.entries[0].StrongWaitMs, "an incompatible Strong must not block qualified Basic when downgrade is explicitly allowed")
	}
}

func TestGPTQoSFiltersCompleteAccountsBeforeQueueAdmission(t *testing.T) {
	for _, tc := range []struct {
		name   string
		change func(*Account)
	}{
		{"model whitelist changed", func(a *Account) {
			a.Credentials = map[string]any{"model_mapping": map[string]any{"other-model": "other-model"}}
		}},
		{"group model restriction changed", func(a *Account) {
			a.AccountGroups = []AccountGroup{{AccountID: a.ID, GroupID: 1, AllowedModels: []string{"other-model"}}}
		}},
		{"group membership revoked", func(a *Account) { a.GroupIDs = []int64{999} }},
		{"quality changed to basic", func(a *Account) { a.Extra = map[string]any{AccountQualityTierKey: "basic"} }},
		{"disabled account", func(a *Account) { a.Status = StatusDisabled }},
		{"explicit multi proxy pool", func(a *Account) {
			a.ProxyPoolConfigured = true
			a.ProxyPool = []AccountProxyBinding{{ProxyID: 12, Concurrency: 1}, {ProxyID: 13, Concurrency: 1}}
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			svc, req := qualityTestGateway(false)
			req.RequiredTransport = OpenAIUpstreamTransportAny
			base := svc.accountRepo.(schedulerTestOpenAIAccountRepo).accounts[0]
			stale := base
			tc.change(&base)
			healthy := stale
			healthy.ID = 4
			svc.accountRepo = schedulerTestOpenAIAccountRepo{accounts: []Account{base, healthy}}
			svc.schedulerSnapshot = NewSchedulerSnapshotService(&openAISnapshotCacheStub{accountsByID: map[int64]*Account{base.ID: &base, healthy.ID: &healthy}}, nil, svc.accountRepo, nil, nil)
			cache := &qosAdmissionRecordingCache{}
			svc.concurrencyService = NewConcurrencyService(cache)
			scheduler := &defaultOpenAIAccountScheduler{service: svc}
			ctx := context.WithValue(context.Background(), ctxkey.UserID, int64(42))
			result, _, _, _, err := scheduler.selectGPTQoSQueue(ctx, req, DefaultGPTQoSConfig(), DefaultGPTQoSGroupConfig("premium"), []openAIAccountCandidateScore{{account: &stale}, {account: &healthy}}, nil, time.Now().Add(time.Second))
			require.NoError(t, err)
			t.Cleanup(result.ReleaseFunc)
			require.Equal(t, healthy.ID, result.Account.ID)
			require.Len(t, cache.entries, 1)
			require.Len(t, cache.entries[0].Candidates, 1, "ineligible accounts must never consume a fair-queue service opportunity")
			require.Equal(t, healthy.ID, cache.entries[0].Candidates[0].AccountID)
		})
	}
}

func TestGPTQoSCompleteSnapshotKeepsMappedModelAndProxySlot(t *testing.T) {
	svc, req := qualityTestGateway(false)
	req.RequiredTransport = OpenAIUpstreamTransportAny
	req.RequestedModel = "public-model"
	a := svc.accountRepo.(schedulerTestOpenAIAccountRepo).accounts[0]
	a.Credentials = map[string]any{"model_mapping": map[string]any{"public-model": "upstream-model"}}
	proxyID := int64(12)
	a.ProxyID = &proxyID
	stale := a
	stale.ProxyID = nil // sched:meta omits proxy bindings.
	svc.accountRepo = schedulerTestOpenAIAccountRepo{accounts: []Account{a}}
	svc.schedulerSnapshot = NewSchedulerSnapshotService(&openAISnapshotCacheStub{accountsByID: map[int64]*Account{a.ID: &a}}, nil, svc.accountRepo, nil, nil)
	cache := &qosAdmissionRecordingCache{}
	svc.concurrencyService = NewConcurrencyService(cache)
	scheduler := &defaultOpenAIAccountScheduler{service: svc}
	ctx := context.WithValue(context.Background(), ctxkey.UserID, int64(42))
	result, _, _, _, err := scheduler.selectGPTQoSQueue(ctx, req, DefaultGPTQoSConfig(), DefaultGPTQoSGroupConfig("premium"), []openAIAccountCandidateScore{{account: &stale}}, nil, time.Now().Add(time.Second))
	require.NoError(t, err)
	t.Cleanup(result.ReleaseFunc)
	require.Equal(t, AccountProxySlotID(a.ID, proxyID), cache.entries[0].Candidates[0].SlotID)
	require.Equal(t, "upstream-model", result.Account.GetMappedModel(req.RequestedModel))
}

func TestGPTQoSFinalRecheckStillRejectsChangedAccountAndReleases(t *testing.T) {
	for _, tc := range []struct {
		name, detail string
		change       func(*Account)
	}{
		{"disabled after grant", "account_unavailable", func(a *Account) { a.Status = StatusDisabled }},
		{"proxy changed after grant", "slot_binding_changed", func(a *Account) { id := int64(13); a.ProxyID = &id }},
		{"concurrency changed after grant", "concurrency_changed", func(a *Account) { a.Concurrency++ }},
		{"shared pool changed after grant", "resource_pool_changed", func(a *Account) {
			a.Extra = map[string]any{AccountQualityTierKey: "strong", AccountResourcePoolKey: "changed"}
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			svc, req := qualityTestGateway(false)
			req.RequiredTransport = OpenAIUpstreamTransportAny
			a := svc.accountRepo.(schedulerTestOpenAIAccountRepo).accounts[0]
			stale := a
			svc.accountRepo = schedulerTestOpenAIAccountRepo{accounts: []Account{a}}
			svc.schedulerSnapshot = NewSchedulerSnapshotService(&openAISnapshotCacheStub{accountsByID: map[int64]*Account{a.ID: &a}}, nil, svc.accountRepo, nil, nil)
			cache := &qosAdmissionRecordingCache{onPoll: func() {
				repo := svc.accountRepo.(schedulerTestOpenAIAccountRepo)
				tc.change(&repo.accounts[0])
			}}
			svc.concurrencyService = NewConcurrencyService(cache)
			scheduler := &defaultOpenAIAccountScheduler{service: svc}
			ctx := context.WithValue(context.Background(), ctxkey.UserID, int64(42))
			result, _, _, _, err := scheduler.selectGPTQoSQueue(ctx, req, DefaultGPTQoSConfig(), DefaultGPTQoSGroupConfig("premium"), []openAIAccountCandidateScore{{account: &stale}}, nil, time.Now().Add(time.Second))
			require.Nil(t, result)
			require.ErrorIs(t, err, ErrNoAvailableAccounts)
			var qosErr *GPTQoSSelectionError
			require.True(t, errors.As(err, &qosErr))
			require.Equal(t, "account_changed", qosErr.Reason)
			require.Equal(t, tc.detail, qosErr.detail)
			require.Contains(t, cache.closed, cache.entries[0].ID, "an invalidated grant must release its queue entry and semaphore")
		})
	}
}

type qosTerminalStatusCache struct {
	qosAdmissionRecordingCache
	poll func(context.Context, GPTQoSEntry) GPTQoSGrant
}

func (c *qosTerminalStatusCache) GPTQoSPoll(ctx context.Context, e GPTQoSEntry) (GPTQoSGrant, error) {
	c.entries = append(c.entries, e)
	return c.poll(ctx, e), nil
}

func TestGPTQoSClosedMarkerDoesNotHideWaitTimeoutOrRestartQueue(t *testing.T) {
	for _, tc := range []struct {
		name    string
		elapsed bool
		want    string
	}{
		{"legacy expiration", true, "queue_timeout"},
		{"early explicit close", false, "queue_closed"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			svc, req := qualityTestGateway(false)
			req.RequiredTransport = OpenAIUpstreamTransportAny
			a := svc.accountRepo.(schedulerTestOpenAIAccountRepo).accounts[0]
			cache := &qosTerminalStatusCache{poll: func(_ context.Context, e GPTQoSEntry) GPTQoSGrant {
				if tc.elapsed {
					time.Sleep(time.Duration(e.WaitMs+2) * time.Millisecond)
				}
				return GPTQoSGrant{Status: "closed"}
			}}
			svc.concurrencyService = NewConcurrencyService(cache)
			ctx := context.WithValue(context.Background(), ctxkey.UserID, int64(42))
			d := time.Now().Add(time.Second)
			if tc.elapsed {
				d = time.Now().Add(5 * time.Millisecond)
			}
			selection, _, _, _, err := (&defaultOpenAIAccountScheduler{service: svc}).selectGPTQoSQueue(ctx, req, DefaultGPTQoSConfig(), DefaultGPTQoSGroupConfig("premium"), []openAIAccountCandidateScore{{account: &a}}, nil, d)
			require.Nil(t, selection)
			var qosErr *GPTQoSSelectionError
			require.ErrorAs(t, err, &qosErr)
			require.Equal(t, tc.want, qosErr.Reason)
			require.Len(t, cache.entries, 1, "closed/expired admission must not silently re-enqueue or reset fairness")
			require.Equal(t, []string{cache.entries[0].ID}, cache.closed)
		})
	}
}

func TestGPTQoSCanceledGrantIsReleasedBeforeAccountRecheck(t *testing.T) {
	svc, req := qualityTestGateway(false)
	req.RequiredTransport = OpenAIUpstreamTransportAny
	a := svc.accountRepo.(schedulerTestOpenAIAccountRepo).accounts[0]
	ctx, cancel := context.WithCancel(context.WithValue(context.Background(), ctxkey.UserID, int64(42)))
	defer cancel()
	cache := &qosTerminalStatusCache{poll: func(_ context.Context, e GPTQoSEntry) GPTQoSGrant {
		cancel()
		return GPTQoSGrant{Status: "granted", Candidate: e.Candidates[0]}
	}}
	svc.concurrencyService = NewConcurrencyService(cache)
	selection, _, _, _, err := (&defaultOpenAIAccountScheduler{service: svc}).selectGPTQoSQueue(ctx, req, DefaultGPTQoSConfig(), DefaultGPTQoSGroupConfig("premium"), []openAIAccountCandidateScore{{account: &a}}, nil, time.Now().Add(time.Second))
	require.Nil(t, selection)
	require.ErrorIs(t, err, context.Canceled)
	require.Len(t, cache.entries, 1)
	require.Equal(t, []string{cache.entries[0].ID}, cache.closed)
}
