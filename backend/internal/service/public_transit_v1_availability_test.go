//go:build unit

package service

import (
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestPublicTransitV1AvailabilityUsesRequestOutcomes(t *testing.T) {
	for _, tc := range []struct {
		name    string
		metrics ChannelMonitorV2Metric
		health  string
		status  string
	}{
		{"one_success_unknown_health", ChannelMonitorV2Metric{RequestCount: 1, SuccessRequests: 1, SuccessRate: 1}, "unknown", "operational"},
		{"successful_slow_requests", ChannelMonitorV2Metric{RequestCount: 5, SuccessRequests: 5, SuccessRate: 1}, "critical", "operational"},
		{"successful_low_cache_requests", ChannelMonitorV2Metric{RequestCount: 100, SuccessRequests: 100, SuccessRate: 1}, "warning", "operational"},
		{"one_failure_unknown_health", ChannelMonitorV2Metric{RequestCount: 1, ErrorRequests: 1, ErrorRate: 1}, "unknown", "failed"},
		{"failures_healthy_health", ChannelMonitorV2Metric{RequestCount: 5, ErrorRequests: 5, ErrorRate: 1}, "healthy", "failed"},
		{"mixed_unknown_health", ChannelMonitorV2Metric{RequestCount: 5, SuccessRequests: 4, ErrorRequests: 1, SuccessRate: .8, ErrorRate: .2}, "unknown", "degraded"},
		{"ignored_errors_keep_actual_failures", ChannelMonitorV2Metric{RequestCount: 5, ErrorRequests: 5, ErrorRate: 0}, "healthy", "failed"},
		{"quiet_bucket", ChannelMonitorV2Metric{}, "healthy", "unknown"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			now := time.Date(2026, 10, 11, 0, 0, 0, 0, time.UTC)
			row := ChannelMonitorV2MatrixRow{
				Platform: "openai", Model: "test-model", Metrics: tc.metrics,
				Health:  ChannelMonitorV2Health{Overall: tc.health},
				Buckets: []ChannelMonitorV2TrendPoint{{BucketStart: now, Metrics: tc.metrics, Health: ChannelMonitorV2Health{Overall: tc.health}}},
			}
			payload := publicTransitV1JSON(t, &ChannelMonitorV2Matrix{Items: []ChannelMonitorV2MatrixRow{row}})
			require.Equal(t, tc.status, payload["primary_status"])
			require.Equal(t, tc.status, payload["models"].([]any)[0].(map[string]any)["latest_status"])
			// Composite V2 health stays available as a separate signal.
			require.Equal(t, tc.health, payload["health"].(map[string]any)["overall"])
			if tc.metrics.RequestCount > 0 {
				require.Len(t, payload["timeline"], 1)
				require.Equal(t, tc.status, payload["timeline"].([]any)[0].(map[string]any)["status"])
			} else {
				require.Equal(t, []any{}, payload["timeline"])
			}
		})
	}
}

func TestPublicTransitV1AvailabilitySmallSampleStableNineToOne(t *testing.T) {
	const total = 10000
	operational, degraded := 0, 0
	now := time.Date(2026, 10, 11, 0, 0, 0, 0, time.UTC)
	for i := 0; i < total; i++ {
		metric := ChannelMonitorV2Metric{RequestCount: 1, SuccessRequests: 1, SuccessRate: 1}
		row := ChannelMonitorV2MatrixRow{
			Platform: "openai", Model: fmt.Sprintf("sample-model-%d", i), Metrics: metric,
			Health:  ChannelMonitorV2Health{Overall: "unknown", MinimumSample: 50},
			Buckets: []ChannelMonitorV2TrendPoint{{BucketStart: now, Metrics: metric, Health: ChannelMonitorV2Health{Overall: "unknown", MinimumSample: 50}}},
		}
		matrix := &ChannelMonitorV2Matrix{Items: []ChannelMonitorV2MatrixRow{row}}
		first := publicTransitV1JSON(t, matrix)
		second := publicTransitV1JSON(t, matrix)
		require.Equal(t, first, second, "repeat scrapes keep the same draw")
		status := first["primary_status"].(string)
		require.Contains(t, []string{"operational", "degraded"}, status)
		require.Equal(t, status, first["models"].([]any)[0].(map[string]any)["latest_status"])
		require.Equal(t, status, first["timeline"].([]any)[0].(map[string]any)["status"])
		require.Equal(t, "v2_request_outcomes_small_sample_success_90_10", first["timeline"].([]any)[0].(map[string]any)["status_policy"])
		require.Equal(t, float64(100), first["availability_7d"])
		if status == "operational" {
			operational++
		} else {
			degraded++
		}
	}
	require.InDelta(t, total*.9, operational, total*.02)
	require.Equal(t, total, operational+degraded)
	t.Logf("stable small-sample labels: operational=%d degraded=%d total=%d", operational, degraded, total)
}

func TestPublicTransitV1AvailabilitySmallSampleThreshold(t *testing.T) {
	for _, minimum := range []int64{1, 5, 50, 100} {
		t.Run(fmt.Sprintf("minimum_%d", minimum), func(t *testing.T) {
			row := ChannelMonitorV2MatrixRow{
				Platform: "openai", Model: "test-model",
				Metrics: ChannelMonitorV2Metric{RequestCount: minimum, SuccessRequests: minimum, SuccessRate: 1},
				Health:  ChannelMonitorV2Health{Overall: "unknown", MinimumSample: minimum},
			}
			payload := publicTransitV1JSON(t, &ChannelMonitorV2Matrix{Items: []ChannelMonitorV2MatrixRow{row}})
			require.Equal(t, "operational", payload["primary_status"])
		})
	}
}
