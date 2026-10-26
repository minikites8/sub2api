package service

import (
	"context"
	"log/slog"
	"sort"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/ctxkey"
)

type gptQoSObservation struct {
	event  GPTQoSMetric
	fields []any
}

type GPTQoSMetric struct {
	At           int64  `json:"at"`
	GroupID      int64  `json:"group_id"`
	Tier         string `json:"qos_tier"`
	Model        string `json:"model"`
	Effort       string `json:"reasoning_effort"`
	Quality      string `json:"quality_tier"`
	AccountType  string `json:"account_type,omitempty"`
	UpstreamKind string `json:"upstream_kind"`
	QueueMs      int64  `json:"queue_ms"`
	Attempts     int32  `json:"attempts"`
	Success      bool   `json:"success"`
	Canceled     bool   `json:"canceled"`
	Downgraded   bool   `json:"downgraded"`
	Fallback     string `json:"fallback_reason"`
}
type GPTQoSMetricFilter struct {
	GroupID       int64
	Model, Effort string
}
type GPTQoSTierMetrics struct {
	Requests, Attempts, Successes, FirstSuccesses, Canceled, Downgrades, StrongRequests, VM2APIRequests, DirectRequests, SentRequests int
	APIRequests, OAuthRequests                                                                                                        int
	StrongHitRate                                                                                                                     float64 `json:"strong_hit_rate"`
	FirstSuccessRate                                                                                                                  float64 `json:"first_success_rate"`
	AverageQueueMs                                                                                                                    float64 `json:"average_queue_ms"`
	P50QueueMs                                                                                                                        int64   `json:"p50_queue_ms"`
	P95QueueMs                                                                                                                        int64   `json:"p95_queue_ms"`
	P99QueueMs                                                                                                                        int64   `json:"p99_queue_ms"`
}

type GPTQoSGroupMetrics struct {
	GroupID   int64  `json:"group_id"`
	GroupName string `json:"group_name,omitempty"`
	GPTQoSTierMetrics
}

// Reuse the request-level aggregation so group percentiles and success rates
// use the same population as the tier totals, including canceled/unsent requests.
func summarizeGPTQoSGroupMetrics(events []GPTQoSMetric) map[string][]GPTQoSGroupMetrics {
	byGroup := make(map[int64][]GPTQoSMetric)
	for _, event := range events {
		byGroup[event.GroupID] = append(byGroup[event.GroupID], event)
	}
	ids := make([]int64, 0, len(byGroup))
	for id := range byGroup {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	out := make(map[string][]GPTQoSGroupMetrics)
	for _, id := range ids {
		// A group's QoS can change within the observation window. Attribute each
		// event to the tier in effect when it ran, rather than today's group tier.
		for tier, metrics := range summarizeGPTQoSMetrics(byGroup[id]) {
			out[tier] = append(out[tier], GPTQoSGroupMetrics{GroupID: id, GPTQoSTierMetrics: metrics})
		}
	}
	return out
}

func summarizeGPTQoSMetrics(events []GPTQoSMetric) map[string]GPTQoSTierMetrics {
	out := map[string]GPTQoSTierMetrics{}
	waits := map[string][]int64{}
	for _, e := range events {
		v := out[e.Tier]
		v.Requests++
		v.Attempts += int(e.Attempts)
		if e.Attempts > 0 {
			v.SentRequests++
			switch e.AccountType {
			case AccountTypeAPIKey:
				v.APIRequests++
			case AccountTypeOAuth:
				v.OAuthRequests++
			}
		}
		if e.Success {
			v.Successes++
			if e.Attempts == 1 {
				v.FirstSuccesses++
			}
		}
		if e.Canceled {
			v.Canceled++
		}
		if e.Downgraded {
			v.Downgrades++
		}
		if e.Quality == "strong" && e.Attempts > 0 {
			v.StrongRequests++
		}
		if e.UpstreamKind == "vm2api" {
			v.VM2APIRequests++
		}
		if e.UpstreamKind == "direct" {
			v.DirectRequests++
		}
		v.AverageQueueMs += float64(e.QueueMs)
		out[e.Tier] = v
		waits[e.Tier] = append(waits[e.Tier], e.QueueMs)
	}
	for tier, v := range out {
		w := waits[tier]
		sort.Slice(w, func(i, j int) bool { return w[i] < w[j] })
		quantile := func(p float64) int64 { return w[min(len(w)-1, int(float64(len(w)-1)*p+0.5))] }
		v.AverageQueueMs /= float64(v.Requests)
		if v.SentRequests > 0 {
			v.StrongHitRate = 100 * float64(v.StrongRequests) / float64(v.SentRequests)
			v.FirstSuccessRate = 100 * float64(v.FirstSuccesses) / float64(v.SentRequests)
		}
		v.P50QueueMs = quantile(.5)
		v.P95QueueMs = quantile(.95)
		v.P99QueueMs = quantile(.99)
		out[tier] = v
	}
	return out
}
func (s *OpenAIGatewayService) recordGPTQoSMetrics(ctx context.Context, groupID *int64, state *gptQoSRequestState, success bool) {
	store := s.gptQoSStore()
	if store == nil {
		return
	}
	e := GPTQoSMetric{At: time.Now().UnixMilli(), GroupID: derefGroupID(groupID), Tier: state.group.Tier, Model: truncateQoSLabel(state.model, 128), Effort: truncateQoSLabel(state.effort, 16), Quality: state.quality, AccountType: state.submittedAccountType, UpstreamKind: state.upstreamKind, QueueMs: state.queueWait.Milliseconds(), Attempts: state.attempts.Load(), Success: success, Canceled: GPTQoSClientCanceled(ctx), Downgraded: state.downgraded, Fallback: state.fallback}
	// Bounded, non-blocking worker queue, initialized lazily on the gateway.
	s.gptQoSMetricsOnce.Do(func() {
		s.gptQoSMetricsQueue = make(chan gptQoSObservation, 1024)
		go func() {
			for observation := range s.gptQoSMetricsQueue {
				if observation.fields != nil {
					slog.Info("gpt_qos_decision", observation.fields...)
				}
				c, cancel := context.WithTimeout(context.Background(), time.Second)
				if err := store.GPTQoSRecord(c, observation.event); err != nil {
					s.gptQoSMetricsDropped.Add(1)
				}
				cancel()
			}
		}()
	})
	observation := gptQoSObservation{event: e}
	rid, _ := ctx.Value(ctxkey.RequestID).(string)
	rid = truncateQoSLabel(rid, 128)
	uid, _ := ctx.Value(ctxkey.UserID).(int64)
	if state.config.LogSampleRate > 0 && deriveOpenAISelectionSeed(OpenAIAccountScheduleRequest{SessionHash: rid})%10000 < uint64(state.config.LogSampleRate*10000) {
		observation.fields = []any{"request_id", rid, "user_id", uid, "group_id", derefGroupID(groupID), "qos_tier", state.group.Tier, "requested_model", truncateQoSLabel(state.model, 128), "effective_model", truncateQoSLabel(state.effectiveModel, 128), "reasoning_effort", truncateQoSLabel(state.effort, 16), "selected_quality_tier", state.quality, "selected_account_id", state.accountID, "selected_pool_id", state.pool, "scheduler_strategy", state.strategy, "sticky_reason", state.sticky, "queue_wait_ms", state.queueWait.Milliseconds(), "selection_duration_ms", state.selectionDuration.Milliseconds(), "attempts", state.attempts.Load(), "fallback_reason", state.fallback, "downgrade_occurred", state.downgraded, "peak_protection_active", state.peak, "config_version", state.config.Version, "success", success, "client_canceled", GPTQoSClientCanceled(ctx)}
	}
	select {
	case s.gptQoSMetricsQueue <- observation:
	default:
		s.gptQoSMetricsDropped.Add(1)
	}
}
func truncateQoSLabel(s string, n int) string {
	if len(s) > n {
		return s[:n]
	}
	return s
}
func (s *OpenAIGatewayService) GPTQoSSnapshot(ctx context.Context) map[string]any {
	return s.GPTQoSSnapshotFiltered(ctx, GPTQoSMetricFilter{})
}
func (s *OpenAIGatewayService) GPTQoSSnapshotFiltered(ctx context.Context, filter GPTQoSMetricFilter) map[string]any {
	if s == nil {
		return map[string]any{"available": false}
	}
	cfg := s.gptQoSConfigForContext(ctx)
	out := map[string]any{"config": cfg, "available": false, "metrics_window": "last 24 hours, at most 10000 completed requests", "metrics_dropped": s.gptQoSMetricsDropped.Load(), "tiers": map[string]GPTQoSTierMetrics{}, "groups": map[string][]GPTQoSGroupMetrics{}}
	if store := s.gptQoSStore(); store != nil {
		state, err := store.GPTQoSState(ctx)
		if err != nil {
			out["reason"] = "redis_unavailable"
			return out
		}
		out["available"] = true
		s.labelGPTQoSPools(ctx, state)
		out["capacity"] = state
		events, err := store.GPTQoSMetrics(ctx, filter)
		if err == nil {
			out["tiers"] = summarizeGPTQoSMetrics(events)
			out["groups"] = summarizeGPTQoSGroupMetrics(events)
			reasons, sources := map[string]int{}, map[string]int{}
			accountTypes := map[string]int{}
			for _, event := range events {
				if event.Downgraded {
					reasons[event.Fallback]++
				}
				if event.Attempts > 0 {
					accountType := event.AccountType
					if accountType != AccountTypeAPIKey && accountType != AccountTypeOAuth {
						accountType = "unknown"
					}
					accountTypes[accountType]++
					source := event.UpstreamKind
					if source == "" {
						source = "unknown"
					}
					sources[source]++
				}
			}
			out["downgrade_reasons"], out["upstream_requests"] = reasons, sources
			out["account_type_requests"] = accountTypes
		} else {
			out["metrics_error"] = "redis_unavailable"
		}
	}
	return out
}
