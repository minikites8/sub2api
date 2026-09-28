package routes

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/handler"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type publicTransitHTTPChannelRepo struct{ service.ChannelRepository }

func (publicTransitHTTPChannelRepo) ListAll(context.Context) ([]service.Channel, error) {
	return []service.Channel{}, nil
}

type publicTransitHTTPGroupRepo struct{ service.GroupRepository }

func (publicTransitHTTPGroupRepo) ListActive(context.Context) ([]service.Group, error) {
	return []service.Group{{ID: 7, Name: "public-group", Platform: "openai", Status: service.StatusActive, SubscriptionType: "standard", RateMultiplier: 1}}, nil
}

type publicTransitHTTPMonitorRepo struct {
	service.ChannelMonitorV2Repository
	enabled   bool
	configErr error
	requests  []string
}

func (r *publicTransitHTTPMonitorRepo) GetConfig(context.Context) (*service.ChannelMonitorV2Config, error) {
	if r.configErr != nil {
		return nil, r.configErr
	}
	return &service.ChannelMonitorV2Config{
		Enabled:   r.enabled,
		Platforms: []service.ChannelMonitorV2PlatformConfig{{Platform: "openai", Enabled: true}},
		GroupIDs:  []int64{7},
	}, nil
}

func (r *publicTransitHTTPMonitorRepo) GetMatrix(_ context.Context, filter service.ChannelMonitorV2Filter, _ service.ChannelMonitorV2Config, groupBy service.ChannelMonitorV2GroupBy, admin bool) (*service.ChannelMonitorV2Matrix, error) {
	if !admin {
		return nil, errors.New("public transit must retain counts internally for empty-traffic detection")
	}
	r.requests = append(r.requests, string(groupBy)+":"+filter.Range)
	observed := time.Date(2026, 9, 1, 12, 0, 0, 0, time.UTC)
	avg, latest := 1234.6, int64(720)
	rate := 0.99
	switch filter.Range {
	case "15d":
		rate = 0.98
	case "30d":
		rate = 0.97
	}
	health := "healthy"
	if filter.Range == "90m" {
		health = "critical"
	}
	row := service.ChannelMonitorV2MatrixRow{
		Platform: "openai", Model: "test-model",
		Metrics: service.ChannelMonitorV2Metric{RequestCount: 100, SuccessRate: rate, Duration: service.ChannelMonitorV2Latency{AvgMs: &avg}},
		Health:  service.ChannelMonitorV2Health{Overall: "healthy"},
		Buckets: []service.ChannelMonitorV2TrendPoint{{
			BucketStart: observed,
			Metrics:     service.ChannelMonitorV2Metric{RequestCount: 5, SuccessRate: 0.8, Duration: service.ChannelMonitorV2Latency{P50Ms: &latest}},
			Health:      service.ChannelMonitorV2Health{Overall: health},
		}},
	}
	if groupBy == service.ChannelMonitorV2GroupByPlatformGroupModel {
		id := int64(7)
		row.GroupID, row.GroupName = &id, "public-group"
	}
	return &service.ChannelMonitorV2Matrix{
		GroupBy:  groupBy,
		Coverage: service.ChannelMonitorV2Coverage{DataThrough: observed, CoverageComplete: true},
		Items:    []service.ChannelMonitorV2MatrixRow{row},
	}, nil
}

func newPublicTransitHTTPRouter(overrides map[string]string) (*gin.Engine, *publicTransitHTTPMonitorRepo) {
	gin.SetMode(gin.TestMode)
	settings := &channelMonitorRouteSettingRepoStub{values: map[string]string{
		service.SettingKeyPublicTransitEnabled:     "true",
		service.SettingKeyPublicTransitPageEnabled: "false",
		service.SettingKeyChannelMonitorEnabled:    "true",
		service.SettingKeyChannelMonitorMode:       service.ChannelMonitorModeV2,
	}}
	for key, value := range overrides {
		settings.values[key] = value
	}
	groups := publicTransitHTTPGroupRepo{}
	monitors := &publicTransitHTTPMonitorRepo{enabled: true}
	svc := service.NewPublicTransitService(
		service.NewChannelService(publicTransitHTTPChannelRepo{}, groups, nil, nil, nil),
		service.NewChannelMonitorV2Service(monitors),
		service.NewSettingService(settings, &config.Config{}),
		service.NewPaymentConfigService(nil, settings, nil),
		groups, nil,
	)
	router := gin.New()
	RegisterPublicTransitRoutes(router, router.Group("/api/v1"), &handler.Handlers{PublicTransit: handler.NewPublicTransitHandler(svc)})
	return router, monitors
}

func publicTransitHTTPPayload(t *testing.T, router http.Handler, path string) (*httptest.ResponseRecorder, map[string]any) {
	t.Helper()
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
	var payload map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &payload))
	return rec, payload
}

func TestPublicTransitHTTPDiscovery(t *testing.T) {
	for _, tc := range []struct{ name, url, forwardedProto, forwardedHost, expectedBase string }{
		{name: "direct_http", url: "http://station.example/.well-known/ai-transit.json", expectedBase: "http://station.example"},
		{name: "direct_https", url: "https://station.example/.well-known/ai-transit.json", expectedBase: "https://station.example"},
		{name: "reverse_proxy", url: "http://internal.example/.well-known/ai-transit.json", forwardedProto: "https", forwardedHost: "public.example", expectedBase: "https://public.example"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			router, monitors := newPublicTransitHTTPRouter(nil)
			req := httptest.NewRequest(http.MethodGet, tc.url, nil)
			req.Header.Set("X-Forwarded-Proto", tc.forwardedProto)
			req.Header.Set("X-Forwarded-Host", tc.forwardedHost)
			rec := httptest.NewRecorder()
			router.ServeHTTP(rec, req)
			require.Equal(t, http.StatusOK, rec.Code)
			require.Contains(t, rec.Header().Get("Content-Type"), "application/json")
			var payload map[string]any
			require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &payload))
			require.Equal(t, "ai-transit.v1", payload["schema_version"])
			require.Equal(t, "sub2api", payload["system"])
			require.Equal(t, tc.expectedBase+"/api/public/transit/v1/snapshot", payload["snapshot_url"])
			require.NotContains(t, payload, "data")
			require.NotContains(t, payload, "homepage_url")
			_, err := time.Parse(time.RFC3339, payload["generated_at"].(string))
			require.NoError(t, err)
			require.Empty(t, monitors.requests)
		})
	}
}

func TestPublicTransitHTTPSnapshotV1Contract(t *testing.T) {
	for _, path := range []string{"/api/public/transit/v1/snapshot", "/api/v1/public/transit/snapshot"} {
		t.Run(path, func(t *testing.T) {
			router, repo := newPublicTransitHTTPRouter(nil)
			rec, payload := publicTransitHTTPPayload(t, router, "https://station.example"+path)
			require.Equal(t, http.StatusOK, rec.Code)
			require.Equal(t, "public, max-age=60", rec.Header().Get("Cache-Control"))
			require.Contains(t, rec.Header().Get("Content-Type"), "application/json")
			require.Equal(t, "ai-transit.v1", payload["schema_version"])
			require.NotContains(t, payload, "data")
			require.NotContains(t, payload, "code")
			monitors, ok := payload["monitoring"].([]any)
			require.True(t, ok)
			require.Len(t, monitors, 1)
			monitor := monitors[0].(map[string]any)
			require.Equal(t, "openai / test-model", monitor["name"])
			require.Equal(t, "openai", monitor["provider"])
			require.Equal(t, "test-model", monitor["primary_model"])
			require.Equal(t, "failed", monitor["primary_status"])
			require.Equal(t, []any{}, monitor["extra_models"])
			require.InDelta(t, 99, monitor["availability_7d"], 1e-12)
			require.InDelta(t, 98, monitor["availability_15d"], 1e-12)
			require.InDelta(t, 97, monitor["availability_30d"], 1e-12)
			require.Equal(t, float64(1235), monitor["avg_latency_7d_ms"])
			require.Equal(t, float64(720), monitor["latest_latency_ms"])
			require.Equal(t, "2026-09-01T12:00:00Z", monitor["last_checked_at"])
			model := monitor["models"].([]any)[0].(map[string]any)
			require.Equal(t, monitor["primary_model"], model["model"])
			require.Equal(t, monitor["primary_status"], model["latest_status"])
			require.Equal(t, monitor["availability_30d"], model["availability_30d"])
			point := monitor["timeline"].([]any)[0].(map[string]any)
			require.Equal(t, "failed", point["status"])
			require.Equal(t, monitor["last_checked_at"], point["checked_at"])
			require.NotContains(t, point, "ping_latency_ms")
			require.NotContains(t, monitor, "group_id")
			require.NotContains(t, monitor["metrics"], "request_count")
			require.Equal(t, "operational", monitor["status"])
			require.Contains(t, monitor, "windows")
			group := payload["groups"].([]any)[0].(map[string]any)
			require.Equal(t, true, group["monitoring_enabled"])
			groupMonitor := group["monitoring"].([]any)[0].(map[string]any)
			require.Equal(t, "public-group", groupMonitor["group_name"])
			require.Equal(t, monitor["primary_status"], groupMonitor["primary_status"])
			require.NotContains(t, groupMonitor, "group_id")
			endpoints := payload["endpoints"].(map[string]any)
			require.Equal(t, "https://station.example/.well-known/ai-transit.json", endpoints["discovery_url"])
			require.Equal(t, "https://station.example/api/public/transit/v1/snapshot", endpoints["snapshot_url"])
			require.Equal(t, true, payload["completeness"].(map[string]any)["has_monitoring"])
			require.Len(t, repo.requests, 12)
		})
	}
}

func TestPublicTransitHTTPDisabledAPI(t *testing.T) {
	for _, path := range []string{"/.well-known/ai-transit.json", "/api/public/transit/v1/snapshot", "/api/v1/public/transit/snapshot"} {
		t.Run(path, func(t *testing.T) {
			router, repo := newPublicTransitHTTPRouter(map[string]string{service.SettingKeyPublicTransitEnabled: "false"})
			rec, payload := publicTransitHTTPPayload(t, router, path)
			require.Equal(t, http.StatusNotFound, rec.Code)
			require.Contains(t, rec.Body.String(), "PUBLIC_TRANSIT_DISABLED")
			require.Empty(t, rec.Header().Get("Cache-Control"))
			require.NotContains(t, payload, "monitoring")
			require.Empty(t, repo.requests)
		})
	}
}

func TestPublicTransitHTTPEmptyMonitoring(t *testing.T) {
	for _, tc := range []struct {
		name      string
		settings  map[string]string
		v2Enabled bool
	}{
		{name: "feature_disabled", settings: map[string]string{service.SettingKeyChannelMonitorEnabled: "false"}, v2Enabled: true},
		{name: "v1_mode", settings: map[string]string{service.SettingKeyChannelMonitorMode: service.ChannelMonitorModeV1}, v2Enabled: true},
		{name: "v2_config_disabled", v2Enabled: false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			router, repo := newPublicTransitHTTPRouter(tc.settings)
			repo.enabled = tc.v2Enabled
			rec, payload := publicTransitHTTPPayload(t, router, "/api/public/transit/v1/snapshot")
			require.Equal(t, http.StatusOK, rec.Code)
			require.Equal(t, []any{}, payload["monitoring"])
			require.Equal(t, false, payload["completeness"].(map[string]any)["has_monitoring"])
			require.Empty(t, repo.requests)
		})
	}
}

func TestPublicTransitHTTPMonitorError(t *testing.T) {
	router, repo := newPublicTransitHTTPRouter(nil)
	repo.configErr = errors.New("test monitor configuration unavailable")
	rec, payload := publicTransitHTTPPayload(t, router, "/api/public/transit/v1/snapshot")
	require.Equal(t, http.StatusInternalServerError, rec.Code)
	require.Empty(t, rec.Header().Get("Cache-Control"))
	require.NotContains(t, payload, "monitoring")
}

func TestPublicTransitHTTPOptionalPublicPage(t *testing.T) {
	router, _ := newPublicTransitHTTPRouter(map[string]string{service.SettingKeyPublicTransitPageEnabled: "true"})
	rec, payload := publicTransitHTTPPayload(t, router, "https://station.example/.well-known/ai-transit.json")
	require.Equal(t, http.StatusOK, rec.Code)
	require.Equal(t, "https://station.example/public/transit", payload["homepage_url"])
}
