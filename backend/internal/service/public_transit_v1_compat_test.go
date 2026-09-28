//go:build unit

package service

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func publicTransitV1JSON(t *testing.T, matrix *ChannelMonitorV2Matrix) map[string]any {
	t.Helper()
	monitors := buildPublicTransitV2Monitors(matrix, nil, nil)
	require.Len(t, monitors, 1)
	raw, err := json.Marshal(monitors[0])
	require.NoError(t, err)
	var payload map[string]any
	require.NoError(t, json.Unmarshal(raw, &payload))
	return payload
}

func TestPublicTransitV1CompatibilityFromV2(t *testing.T) {
	now := time.Date(2026, 9, 1, 12, 0, 0, 0, time.UTC)
	avg := 1234.6
	latest := int64(720)
	matrix := &ChannelMonitorV2Matrix{
		Coverage: ChannelMonitorV2Coverage{DataThrough: now, CoverageComplete: true},
		Items: []ChannelMonitorV2MatrixRow{{
			Platform: "openai", Model: "test-model",
			Metrics: ChannelMonitorV2Metric{RequestCount: 100, SuccessRate: 0.97,
				Duration: ChannelMonitorV2Latency{AvgMs: &avg}},
			Health: ChannelMonitorV2Health{Overall: "warning"},
			Buckets: []ChannelMonitorV2TrendPoint{
				{BucketStart: now.Add(-2 * time.Hour), Metrics: ChannelMonitorV2Metric{RequestCount: 3}, Health: ChannelMonitorV2Health{Overall: "critical"}},
				{BucketStart: now.Add(-time.Hour), Metrics: ChannelMonitorV2Metric{RequestCount: 5, Duration: ChannelMonitorV2Latency{P50Ms: &latest}}, Health: ChannelMonitorV2Health{Overall: "healthy"}},
				{BucketStart: now, Health: ChannelMonitorV2Health{Overall: "unknown"}},
			},
		}},
	}
	payload := publicTransitV1JSON(t, matrix)
	require.Equal(t, "openai / test-model", payload["name"])
	require.Equal(t, "openai", payload["provider"])
	require.Equal(t, "test-model", payload["primary_model"])
	require.Equal(t, "operational", payload["primary_status"])
	require.Equal(t, float64(1235), payload["avg_latency_7d_ms"])
	require.Equal(t, float64(720), payload["latest_latency_ms"])
	require.Equal(t, now.Add(-time.Hour).Format(time.RFC3339), payload["last_checked_at"])
	require.Equal(t, []any{}, payload["extra_models"])
	models, ok := payload["models"].([]any)
	require.True(t, ok)
	require.Len(t, models, 1)
	model := models[0].(map[string]any)
	require.Equal(t, "test-model", model["model"])
	require.Equal(t, "operational", model["latest_status"])
	require.Equal(t, float64(720), model["latest_latency_ms"])
	require.Equal(t, float64(1235), model["avg_latency_7d_ms"])
	for _, period := range []string{"7d", "15d", "30d"} {
		require.InDelta(t, 97, payload["availability_"+period], 1e-9)
		require.Equal(t, payload["availability_"+period], model["availability_"+period])
	}
	timeline, ok := payload["timeline"].([]any)
	require.True(t, ok)
	require.Len(t, timeline, 2)
	require.Equal(t, "operational", timeline[0].(map[string]any)["status"])
	require.Equal(t, "failed", timeline[1].(map[string]any)["status"])
	require.Equal(t, payload["last_checked_at"], timeline[0].(map[string]any)["checked_at"])
	require.Equal(t, float64(720), timeline[0].(map[string]any)["latency_ms"])
	require.NotContains(t, payload, "latest_ping_latency_ms")
	require.NotContains(t, timeline[0], "ping_latency_ms")
	// Existing V2 clients retain their aggregate status, metrics and all buckets.
	require.Equal(t, "degraded", payload["status"])
	require.Equal(t, "openai", payload["platform"])
	require.Equal(t, "test-model", payload["model"])
	require.Len(t, payload["buckets"], 3)
	require.NotContains(t, payload["metrics"], "request_count")
}

func TestPublicTransitV1CompatibilityStatuses(t *testing.T) {
	for _, tc := range []struct{ health, status string }{
		{"healthy", "operational"}, {"warning", "degraded"},
		{"critical", "failed"}, {"unknown", "unknown"},
	} {
		t.Run(tc.health, func(t *testing.T) {
			payload := publicTransitV1JSON(t, &ChannelMonitorV2Matrix{Items: []ChannelMonitorV2MatrixRow{{
				Platform: "anthropic", Model: "test-model",
				Metrics: ChannelMonitorV2Metric{RequestCount: 1},
				Health:  ChannelMonitorV2Health{Overall: tc.health},
			}}})
			require.Equal(t, tc.status, payload["primary_status"])
			require.Equal(t, []any{}, payload["timeline"])
		})
	}
}

func TestPublicTransitV1CompatibilityEmptyTraffic(t *testing.T) {
	payload := publicTransitV1JSON(t, &ChannelMonitorV2Matrix{Items: []ChannelMonitorV2MatrixRow{{
		Platform: "openai", Model: "test-model", Health: ChannelMonitorV2Health{Overall: "healthy"},
	}}})
	require.Equal(t, "unknown", payload["primary_status"])
	require.Equal(t, []any{}, payload["extra_models"])
	require.Equal(t, []any{}, payload["timeline"])
	require.NotContains(t, payload, "latest_latency_ms")
	require.NotContains(t, payload, "avg_latency_7d_ms")
	require.NotContains(t, payload, "last_checked_at")
}

func TestPublicTransitV1CompatibilityGroupIdentity(t *testing.T) {
	groupID := int64(7)
	payload := publicTransitV1JSON(t, &ChannelMonitorV2Matrix{Items: []ChannelMonitorV2MatrixRow{{
		Platform: "openai", GroupID: &groupID, GroupName: "public-group", Model: "test-model",
	}}})
	require.Equal(t, "public-group", payload["group_name"])
	require.NotContains(t, payload, "group_id")
}

type publicTransitV1SettingRepo struct {
	SettingRepository
	values map[string]string
}

func (r publicTransitV1SettingRepo) GetMultiple(context.Context, []string) (map[string]string, error) {
	return r.values, nil
}

func TestPublicTransitV1DiscoveryContract(t *testing.T) {
	for _, pageEnabled := range []string{"false", "true"} {
		t.Run("page_"+pageEnabled, func(t *testing.T) {
			svc := &PublicTransitService{settingService: &SettingService{settingRepo: publicTransitV1SettingRepo{
				values: map[string]string{SettingKeyPublicTransitEnabled: "true", SettingKeyPublicTransitPageEnabled: pageEnabled},
			}}}
			discovery, err := svc.Discovery(context.Background(), "https://transit.example")
			require.NoError(t, err)
			raw, err := json.Marshal(discovery)
			require.NoError(t, err)
			var payload map[string]any
			require.NoError(t, json.Unmarshal(raw, &payload))
			require.Equal(t, "ai-transit.v1", payload["schema_version"])
			require.Equal(t, "sub2api", payload["system"])
			require.Equal(t, "/.well-known/ai-transit.json", PublicTransitWellKnownPath)
			require.Equal(t, "https://transit.example/api/public/transit/v1/snapshot", payload["snapshot_url"])
			_, err = time.Parse(time.RFC3339, payload["generated_at"].(string))
			require.NoError(t, err)
			if pageEnabled == "true" {
				require.Equal(t, "https://transit.example/public/transit", payload["homepage_url"])
			} else {
				require.NotContains(t, payload, "homepage_url")
			}
		})
	}
}

func TestPublicTransitV1CompatibilityKeepsLatestMissingLatency(t *testing.T) {
	now := time.Date(2026, 9, 1, 12, 0, 0, 0, time.UTC)
	oldLatency := int64(200)
	// Deliberately unordered: the adapter must preserve V2 bucket ordering.
	buckets := []ChannelMonitorV2TrendPoint{
		{BucketStart: now, Metrics: ChannelMonitorV2Metric{RequestCount: 1}, Health: ChannelMonitorV2Health{Overall: "critical"}},
		{BucketStart: now.Add(-time.Hour), Metrics: ChannelMonitorV2Metric{RequestCount: 1, Duration: ChannelMonitorV2Latency{P50Ms: &oldLatency}}, Health: ChannelMonitorV2Health{Overall: "healthy"}},
	}
	payload := publicTransitV1JSON(t, &ChannelMonitorV2Matrix{Items: []ChannelMonitorV2MatrixRow{{
		Platform: "openai", Model: "test-model", Buckets: buckets,
		Metrics: ChannelMonitorV2Metric{RequestCount: 2}, Health: ChannelMonitorV2Health{Overall: "warning"},
	}}})
	require.Equal(t, "failed", payload["primary_status"])
	require.Equal(t, now.Format(time.RFC3339), payload["last_checked_at"])
	require.NotContains(t, payload, "latest_latency_ms")
	require.NotContains(t, payload["models"].([]any)[0], "latest_latency_ms")
	require.Equal(t, now, buckets[0].BucketStart)
	require.Equal(t, now.Add(-time.Hour), buckets[1].BucketStart)
}

func TestPublicTransitV1CompatibilityPreservesAvailabilityWindows(t *testing.T) {
	matrix := func(rate float64) *ChannelMonitorV2Matrix {
		return &ChannelMonitorV2Matrix{Items: []ChannelMonitorV2MatrixRow{{
			Platform: "openai", Model: "test-model",
			Metrics: ChannelMonitorV2Metric{RequestCount: 100, SuccessRate: rate},
		}}}
	}
	monitors := buildPublicTransitV2Monitors(matrix(0.99), matrix(0.98), matrix(0.975))
	require.Len(t, monitors, 1)
	raw, err := json.Marshal(monitors[0])
	require.NoError(t, err)
	var payload map[string]any
	require.NoError(t, json.Unmarshal(raw, &payload))
	models, ok := payload["models"].([]any)
	require.True(t, ok)
	require.Len(t, models, 1)
	model := models[0].(map[string]any)
	require.InDelta(t, 99, model["availability_7d"], 1e-12)
	require.InDelta(t, 98, model["availability_15d"], 1e-12)
	require.InDelta(t, 97.5, model["availability_30d"], 1e-12)
}

func TestPublicTransitV1CompatibilityUsesV2Matrices(t *testing.T) {
	repo := &channelMonitorV2RepoStub{
		config: ChannelMonitorV2Config{Enabled: true},
		matrix: &ChannelMonitorV2Matrix{Items: []ChannelMonitorV2MatrixRow{{
			Platform: "openai", Model: "test-model",
			Metrics: ChannelMonitorV2Metric{RequestCount: 10, SuccessRate: 0.9},
			Health:  ChannelMonitorV2Health{Overall: "warning"},
		}}},
	}
	svc := &PublicTransitService{monitorService: NewChannelMonitorV2Service(repo)}
	monitors, err := svc.publicV2Monitors(context.Background(), ChannelMonitorV2GroupByPlatformModel)
	require.NoError(t, err)
	require.Len(t, monitors, 1)
	require.Equal(t, ChannelMonitorV2GroupByPlatformModel, repo.group)
	require.True(t, repo.admin)
	require.InDelta(t, 90, monitors[0].Availability7d, 1e-12)
	require.Len(t, monitors[0].Windows, 4)
	for _, window := range []string{"90m", "12h", "1d", "15d"} {
		require.Contains(t, monitors[0].Windows, window)
	}
	raw, err := json.Marshal(monitors[0])
	require.NoError(t, err)
	var payload map[string]any
	require.NoError(t, json.Unmarshal(raw, &payload))
	require.Equal(t, "test-model", payload["primary_model"])
	require.Equal(t, "degraded", payload["primary_status"])
}

type publicTransitV1MatrixRepo struct {
	channelMonitorV2RepoStub
	matrices map[string]*ChannelMonitorV2Matrix
}

func (r *publicTransitV1MatrixRepo) GetMatrix(_ context.Context, filter ChannelMonitorV2Filter, _ ChannelMonitorV2Config, _ ChannelMonitorV2GroupBy, _ bool) (*ChannelMonitorV2Matrix, error) {
	return r.matrices[filter.Range], nil
}

func TestPublicTransitV1CompatibilityRecentObservation(t *testing.T) {
	for _, recentTraffic := range []bool{true, false} {
		name := "recent_requests"
		if !recentTraffic {
			name = "quiet_recent_window"
		}
		t.Run(name, func(t *testing.T) {
			now := time.Date(2026, 9, 1, 12, 0, 0, 0, time.UTC)
			avg7d, avgRecent := 1500.0, 300.0
			row7d := ChannelMonitorV2MatrixRow{
				Platform: "openai", Model: "test-model",
				Metrics: ChannelMonitorV2Metric{RequestCount: 100, SuccessRate: 0.99, Duration: ChannelMonitorV2Latency{AvgMs: &avg7d}},
				Health:  ChannelMonitorV2Health{Overall: "healthy"},
				Buckets: []ChannelMonitorV2TrendPoint{{BucketStart: now.Add(-12 * time.Hour), Metrics: ChannelMonitorV2Metric{RequestCount: 100}, Health: ChannelMonitorV2Health{Overall: "healthy"}}},
			}
			rowRecent := ChannelMonitorV2MatrixRow{
				Platform: "openai", Model: "test-model",
				Metrics: ChannelMonitorV2Metric{Duration: ChannelMonitorV2Latency{AvgMs: &avgRecent}},
				Buckets: []ChannelMonitorV2TrendPoint{{BucketStart: now.Add(-5 * time.Minute), Health: ChannelMonitorV2Health{Overall: "critical"}}},
			}
			if recentTraffic {
				rowRecent.Buckets[0].Metrics.RequestCount = 1
			}
			repo := &publicTransitV1MatrixRepo{
				channelMonitorV2RepoStub: channelMonitorV2RepoStub{config: ChannelMonitorV2Config{Enabled: true}},
				matrices: map[string]*ChannelMonitorV2Matrix{
					"7d":  {Items: []ChannelMonitorV2MatrixRow{row7d}},
					"90m": {Items: []ChannelMonitorV2MatrixRow{rowRecent}},
				},
			}
			svc := &PublicTransitService{monitorService: NewChannelMonitorV2Service(repo)}
			monitors, err := svc.publicV2Monitors(context.Background(), ChannelMonitorV2GroupByPlatformModel)
			require.NoError(t, err)
			require.Len(t, monitors, 1)
			raw, err := json.Marshal(monitors[0])
			require.NoError(t, err)
			var payload map[string]any
			require.NoError(t, json.Unmarshal(raw, &payload))
			require.Equal(t, float64(1500), payload["avg_latency_7d_ms"])
			require.InDelta(t, 99, payload["availability_7d"], 1e-12)
			if recentTraffic {
				require.Equal(t, "failed", payload["primary_status"])
				require.Equal(t, now.Add(-5*time.Minute).Format(time.RFC3339), payload["last_checked_at"])
			} else {
				require.Equal(t, "operational", payload["primary_status"])
				require.Equal(t, now.Add(-12*time.Hour).Format(time.RFC3339), payload["last_checked_at"])
			}
		})
	}
}
