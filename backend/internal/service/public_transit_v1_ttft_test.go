//go:build unit

package service

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestPublicTransitV1TTFTLatency(t *testing.T) {
	for _, tc := range []struct {
		name       string
		group      bool
		recent     bool
		missing    bool
		zero       bool
		wantLatest int64
		wantAvg    int64
	}{
		{name: "historical site", wantLatest: 2500, wantAvg: 1533},
		{name: "historical group", group: true, wantLatest: 2500, wantAvg: 1533},
		{name: "recent group preserves seven day mean", group: true, recent: true, wantLatest: 500, wantAvg: 1533},
		{name: "missing TTFT remains omitted", missing: true},
		{name: "zero TTFT is observed", zero: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			now := time.Date(2026, 10, 11, 5, 0, 0, 0, time.UTC)
			ttftLatest, ttftAvg := int64(2500), 1532.6
			durationLatest, durationAvg := int64(30000), 41269.3
			ttft := ChannelMonitorV2Latency{P50Ms: &ttftLatest, AvgMs: &ttftAvg}
			if tc.missing {
				ttft = ChannelMonitorV2Latency{}
			}
			if tc.zero {
				ttftLatest, ttftAvg = 0, 0
			}
			row := ChannelMonitorV2MatrixRow{
				Platform: "openai", Model: "gpt-image-2",
				Metrics: ChannelMonitorV2Metric{RequestCount: 100, SuccessRequests: 95, ErrorRequests: 5, SuccessRate: .95,
					TTFT: ttft, Duration: ChannelMonitorV2Latency{P50Ms: &durationLatest, AvgMs: &durationAvg}},
				Buckets: []ChannelMonitorV2TrendPoint{{BucketStart: now.Add(-12 * time.Hour),
					Metrics: ChannelMonitorV2Metric{RequestCount: 100, SuccessRequests: 95, ErrorRequests: 5, SuccessRate: .95,
						TTFT: ttft, Duration: ChannelMonitorV2Latency{P50Ms: &durationLatest}}}},
			}
			if tc.group {
				groupID := int64(7)
				row.GroupID, row.GroupName = &groupID, "image2"
			}
			monitors := buildPublicTransitV2Monitors(&ChannelMonitorV2Matrix{Items: []ChannelMonitorV2MatrixRow{row}}, nil, nil)
			require.Len(t, monitors, 1)
			if tc.recent {
				recent := row
				recentTTFT, recentDuration, recentAvg := int64(500), int64(60000), 200.0
				recent.Metrics.TTFT.AvgMs = &recentAvg
				recent.Buckets = []ChannelMonitorV2TrendPoint{{BucketStart: now.Add(-5 * time.Minute),
					Metrics: ChannelMonitorV2Metric{RequestCount: 100, SuccessRate: .95,
						TTFT: ChannelMonitorV2Latency{P50Ms: &recentTTFT}, Duration: ChannelMonitorV2Latency{P50Ms: &recentDuration}}}}
				attachPublicTransitV1RecentTimeline(monitors, &ChannelMonitorV2Matrix{Items: []ChannelMonitorV2MatrixRow{recent}})
			}
			raw, err := json.Marshal(monitors[0])
			require.NoError(t, err)
			var payload map[string]any
			require.NoError(t, json.Unmarshal(raw, &payload))
			model := payload["models"].([]any)[0].(map[string]any)
			point := payload["timeline"].([]any)[0].(map[string]any)
			t.Logf("latest_latency_ms=%v avg_latency_7d_ms=%v timeline.latency_ms=%v; duration.avg_ms=%v",
				payload["latest_latency_ms"], payload["avg_latency_7d_ms"], point["latency_ms"], payload["metrics"].(map[string]any)["duration"])
			if tc.missing {
				for _, target := range []map[string]any{payload, model} {
					require.NotContains(t, target, "latest_latency_ms")
					require.NotContains(t, target, "avg_latency_7d_ms")
				}
				require.NotContains(t, point, "latency_ms")
			} else {
				for _, target := range []map[string]any{payload, model} {
					require.Equal(t, float64(tc.wantLatest), target["latest_latency_ms"])
					require.Equal(t, float64(tc.wantAvg), target["avg_latency_7d_ms"])
				}
				require.Equal(t, float64(tc.wantLatest), point["latency_ms"])
			}
			require.Equal(t, durationAvg, payload["metrics"].(map[string]any)["duration"].(map[string]any)["avg_ms"])
			require.Equal(t, float64(95), payload["availability_7d"])
			require.Equal(t, &durationLatest, row.Metrics.Duration.P50Ms)
			require.Equal(t, "degraded", payload["primary_status"])
		})
	}
}
