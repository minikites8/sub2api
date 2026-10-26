package service

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/ctxkey"
	"github.com/stretchr/testify/require"
)

func TestGPTQoSMetricsCountSubmittedAccountTypes(t *testing.T) {
	got := summarizeGPTQoSMetrics([]GPTQoSMetric{
		{Tier: "premium", AccountType: AccountTypeAPIKey, UpstreamKind: "direct", Attempts: 3, Success: true},
		{Tier: "premium", AccountType: AccountTypeAPIKey, Attempts: 1, Canceled: true},
		{Tier: "premium", AccountType: AccountTypeOAuth, UpstreamKind: "vm2api", Attempts: 1, Success: true},
		{Tier: "premium", AccountType: AccountTypeAPIKey, Attempts: 0, Canceled: true},
		{Tier: "premium", AccountType: AccountTypeOAuth, Attempts: 0},
		{Tier: "premium", UpstreamKind: "vm2api", Attempts: 1}, // historical event: never infer protocol
		{Tier: "premium", UpstreamKind: "direct", Attempts: 1},
		{Tier: "premium", AccountType: "future-type", Attempts: 1},
	})["premium"]
	require.Equal(t, 8, got.Requests)
	require.Equal(t, 8, got.Attempts)
	require.Equal(t, 6, got.SentRequests)
	require.Equal(t, 2, got.APIRequests, "retries count as one request; unsent requests never count")
	require.Equal(t, 1, got.OAuthRequests, "protocol is independent of manual upstream source labels")
}

func TestGPTQoSGroupMetricsMatchTierTotalsAndHistoricalPolicy(t *testing.T) {
	events := []GPTQoSMetric{
		{Tier: "premium", GroupID: 20, AccountType: AccountTypeOAuth, Attempts: 1, Success: true, QueueMs: 200},
		{Tier: "premium", GroupID: 10, AccountType: AccountTypeAPIKey, Attempts: 2, Success: true, Quality: "strong", QueueMs: 10},
		{Tier: "standard", GroupID: 10, Attempts: 0, Canceled: true, QueueMs: 90},
		{Tier: "premium", GroupID: 20, Attempts: 0, Canceled: true, QueueMs: 400},
		{Tier: "economy", GroupID: 0, AccountType: AccountTypeAPIKey, Attempts: 1, Downgraded: true, QueueMs: 5},
	}
	groups := summarizeGPTQoSGroupMetrics(events)
	tiers := summarizeGPTQoSMetrics(events)
	require.Len(t, groups, 3)
	require.Len(t, groups["premium"], 2)
	require.Equal(t, int64(10), groups["premium"][0].GroupID, "stable numeric ID order")
	require.Equal(t, int64(20), groups["premium"][1].GroupID)
	require.Equal(t, int64(10), groups["standard"][0].GroupID, "a policy change does not move historical requests")
	require.Equal(t, int64(0), groups["economy"][0].GroupID)
	for tier, rows := range groups {
		sum := GPTQoSTierMetrics{}
		for _, row := range rows {
			sum.Requests += row.Requests
			sum.Attempts += row.Attempts
			sum.APIRequests += row.APIRequests
			sum.OAuthRequests += row.OAuthRequests
			sum.Canceled += row.Canceled
			sum.Successes += row.Successes
			sum.Downgrades += row.Downgrades
		}
		want := tiers[tier]
		require.Equal(t, want.Requests, sum.Requests, tier)
		require.Equal(t, want.Attempts, sum.Attempts, tier)
		require.Equal(t, want.APIRequests, sum.APIRequests, tier)
		require.Equal(t, want.OAuthRequests, sum.OAuthRequests, tier)
		require.Equal(t, want.Canceled, sum.Canceled, tier)
		require.Equal(t, want.Successes, sum.Successes, tier)
		require.Equal(t, want.Downgrades, sum.Downgrades, tier)
	}
	require.Equal(t, float64(300), groups["premium"][1].AverageQueueMs)
	require.Equal(t, int64(400), groups["premium"][1].P95QueueMs)
	require.Equal(t, int64(200), tiers["premium"].P50QueueMs, "tier quantiles use raw waits, not group percentiles")
	require.Equal(t, float64(100), groups["premium"][1].FirstSuccessRate)
	raw, err := json.Marshal(groups)
	require.NoError(t, err)
	require.Contains(t, string(raw), `"group_id":20`)
	require.Contains(t, string(raw), `"APIRequests":1`)
	require.Empty(t, summarizeGPTQoSGroupMetrics(nil))
}

type qosMetricsTestStore struct {
	qosAdmissionRecordingCache
	events   []GPTQoSMetric
	filter   GPTQoSMetricFilter
	recorded chan GPTQoSMetric
}

func (s *qosMetricsTestStore) GPTQoSState(context.Context) (map[string]any, error) {
	return map[string]any{}, nil
}
func (s *qosMetricsTestStore) GPTQoSMetrics(_ context.Context, filter GPTQoSMetricFilter) ([]GPTQoSMetric, error) {
	s.filter = filter
	return s.events, nil
}
func (s *qosMetricsTestStore) GPTQoSRecord(_ context.Context, event GPTQoSMetric) error {
	s.recorded <- event
	return nil
}

func TestGPTQoSSnapshotIncludesGroupMetricsAndProtocolShares(t *testing.T) {
	store := &qosMetricsTestStore{events: []GPTQoSMetric{
		{GroupID: 7, Tier: "premium", AccountType: AccountTypeAPIKey, Attempts: 3},
		{GroupID: 7, Tier: "premium", AccountType: AccountTypeOAuth, Attempts: 1},
		{GroupID: 7, Tier: "premium", Attempts: 1, UpstreamKind: "vm2api"},
		{GroupID: 7, Tier: "premium", AccountType: AccountTypeOAuth, Attempts: 0, Canceled: true},
	}}
	svc := &OpenAIGatewayService{concurrencyService: NewConcurrencyService(store)}
	filter := GPTQoSMetricFilter{GroupID: 7, Model: "gpt-test", Effort: "high"}
	got := svc.GPTQoSSnapshotFiltered(context.Background(), filter)
	require.Equal(t, filter, store.filter, "one filtered event population drives totals and children")
	require.Equal(t, map[string]int{"apikey": 1, "oauth": 1, "unknown": 1}, got["account_type_requests"])
	groups := got["groups"].(map[string][]GPTQoSGroupMetrics)
	require.Equal(t, summarizeGPTQoSMetrics(store.events)["premium"], groups["premium"][0].GPTQoSTierMetrics)
}

func TestGPTQoSMetricsObserveActualSelectionAndSubmission(t *testing.T) {
	for _, queue := range []bool{false, true} {
		for _, accountType := range []string{AccountTypeAPIKey, AccountTypeOAuth} {
			t.Run(fmt.Sprintf("%s/queue=%t", accountType, queue), func(t *testing.T) {
				svc, req := qualityTestGateway(false)
				svc.settingService.gptQoSConfig.config.Enabled = queue
				svc.settingService.gptQoSConfig.config.LogSampleRate = 0
				repo := svc.accountRepo.(schedulerTestOpenAIAccountRepo)
				repo.accounts[0].Type = accountType
				svc.accountRepo = repo
				store := &qosMetricsTestStore{recorded: make(chan GPTQoSMetric, 1)}
				svc.concurrencyService = NewConcurrencyService(store)
				parent := context.WithValue(context.Background(), ctxkey.UserID, int64(42))
				ctx, finish := svc.PrepareGPTQoSRequest(parent, req.GroupID, []byte(`{"model":"gpt-test"}`))
				selection, _, err := newDefaultOpenAIAccountScheduler(svc, nil).Select(ctx, req)
				require.NoError(t, err)
				require.Equal(t, accountType, selection.Account.Type)
				require.NoError(t, MarkGPTQoSSubmission(ctx))
				require.NoError(t, MarkGPTQoSSubmission(ctx)) // retry: same original user request
				selection.ReleaseFunc()
				// A later selection rejected before submission must not rewrite provenance.
				state := ctx.Value(gptQoSRequestKey{}).(*gptQoSRequestState)
				state.mu.Lock()
				state.accountType = "unsent-account"
				state.mu.Unlock()
				ObserveGPTQoSOutcome(ctx, true)
				finish(true)
				defer close(svc.gptQoSMetricsQueue)
				select {
				case event := <-store.recorded:
					require.Equal(t, accountType, event.AccountType)
					require.Equal(t, int32(2), event.Attempts)
					require.True(t, event.Success)
					require.Equal(t, int64(1), event.GroupID)
				case <-time.After(time.Second):
					t.Fatal("completion did not publish its observation")
				}
			})
		}
	}
}
