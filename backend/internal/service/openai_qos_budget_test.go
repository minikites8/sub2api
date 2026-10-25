package service

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestGPTQoSRequestBudgetAndAttemptsSurviveFailover(t *testing.T) {
	svc, req := qualityTestGateway(false)
	cfg := svc.settingService.gptQoSConfig.config
	cfg.Enabled = true
	cfg.RequestBudgetMs = 1000
	cfg.HighEffortBudgetMs = 2000
	cfg.MaxAttempts = 2
	cfg.LogSampleRate = 0
	svc.settingService.gptQoSConfig.config = cfg
	parent, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()
	ctx, finish := svc.PrepareGPTQoSRequest(parent, req.GroupID, []byte(`{"model":"gpt-test","reasoning":{"effort":"high"}}`))
	defer finish(false)
	deadline, _ := ctx.Deadline()
	original, _ := parent.Deadline()
	require.Equal(t, original, deadline, "QoS cannot extend the client deadline")
	require.NoError(t, MarkGPTQoSSubmission(ctx))
	ObserveGPTQoSOutcome(ctx, false)
	require.NoError(t, MarkGPTQoSSubmission(ctx))
	ObserveGPTQoSOutcome(ctx, true)
	require.ErrorIs(t, MarkGPTQoSSubmission(ctx), ErrGPTQoSUnavailable)
	state := ctx.Value(gptQoSRequestKey{}).(*gptQoSRequestState)
	require.Equal(t, int32(2), state.attempts.Load())
	require.True(t, state.lastSucceeded)
	cancel()
	require.ErrorIs(t, MarkGPTQoSSubmission(ctx), context.Canceled)
}
func TestGPTQoSMetricsSeparateRequestsAttemptsAndCancellation(t *testing.T) {
	got := summarizeGPTQoSMetrics([]GPTQoSMetric{
		{Tier: "premium", Attempts: 3, Success: true, Quality: "strong", QueueMs: 100},
		{Tier: "premium", Attempts: 1, Success: true, Quality: "strong", QueueMs: 200},
		{Tier: "premium", Attempts: 0, Canceled: true, QueueMs: 300},
		{Tier: "economy", Attempts: 1, Success: true, Downgraded: true, Quality: "basic", QueueMs: 10},
	})
	require.Equal(t, 3, got["premium"].Requests)
	require.Equal(t, 4, got["premium"].Attempts)
	require.Equal(t, 2, got["premium"].Successes)
	require.Equal(t, 1, got["premium"].FirstSuccesses)
	require.Equal(t, 1, got["premium"].Canceled)
	require.Equal(t, float64(200), got["premium"].AverageQueueMs)
	require.Equal(t, 1, got["economy"].Downgrades)
}
func TestGPTQoSWebSocketTurnBudgetDoesNotExpireIdleConnection(t *testing.T) {
	svc, req := qualityTestGateway(false)
	cfg := svc.settingService.gptQoSConfig.config
	cfg.Enabled = true
	cfg.RequestBudgetMs = 20
	cfg.HighEffortBudgetMs = 20
	cfg.LogSampleRate = 0
	svc.settingService.gptQoSConfig.config = cfg
	ctx, finish := svc.PrepareGPTQoSWebSocket(context.Background(), req.GroupID, []byte(`{"model":"gpt-test"}`))
	finish(true)
	time.Sleep(35 * time.Millisecond)
	require.NoError(t, ctx.Err(), "successful turn must stop its timer before connection idle")
	finish = svc.BeginGPTQoSWebSocketTurn(ctx, req.GroupID, []byte(`{"model":"gpt-test"}`))
	defer finish(false)
	require.Eventually(t, func() bool { return ctx.Err() != nil }, time.Second, time.Millisecond)
}
func TestGPTQoSRolloutScopeAndCapacityFreshness(t *testing.T) {
	cfg := DefaultGPTQoSConfig()
	cfg.GroupIDs = []int64{9}
	id := int64(8)
	require.False(t, cfg.Applies(&id))
	id = 9
	require.True(t, cfg.Applies(&id))
	require.False(t, cfg.Applies(nil))
	now := time.Now()
	v := UpstreamCapacitySnapshot{PoolID: "vm", EffectiveConcurrency: 2, ActiveConcurrency: 1, AvailableSlots: 1, UpdatedAt: now}
	require.True(t, ReliableUpstreamCapacity(v, now))
	v.UpdatedAt = now.Add(-time.Minute)
	require.False(t, ReliableUpstreamCapacity(v, now))
	v.UpdatedAt = now.Add(time.Second)
	require.False(t, ReliableUpstreamCapacity(v, now))
}
