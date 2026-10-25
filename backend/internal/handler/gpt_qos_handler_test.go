//go:build unit

package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/ctxkey"
	"github.com/Wei-Shaw/sub2api/internal/repository"
	"github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/alicebob/miniredis/v2"
	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
)

type qosHandlerSettings struct {
	service.SettingRepository
	raw string
}

func (r qosHandlerSettings) GetValue(_ context.Context, key string) (string, error) {
	if key == service.GPTQoSSettingKey {
		return r.raw, nil
	}
	return "", service.ErrSettingNotFound
}
func (r qosHandlerSettings) GetMultiple(context.Context, []string) (map[string]string, error) {
	return map[string]string{}, nil
}

type qosHandlerAccountRepo struct{ *openAI502503AdmissionRepo }

func (r *qosHandlerAccountRepo) ListModelAvailabilityCandidates(ctx context.Context, _ *int64, platforms []string, _ bool) ([]service.Account, error) {
	return r.ListSchedulableByPlatform(ctx, platforms[0])
}

func TestGPTQoSResponsesSSEAndFallbackIntegration(t *testing.T) {
	for _, stream := range []bool{false, true} {
		for _, tc := range []struct {
			name                 string
			enabled, busy, allow bool
			wantAccount          int64
			wantStatus           int
		}{
			{"strong beats basic priority", true, false, false, 802, 200},
			{"quality guarantee refuses basic", true, true, false, 0, 503},
			{"explicit downgrade uses basic", true, true, true, 803, 200},
			{"all flags off retains legacy priority", false, false, false, 803, 200},
		} {
			t.Run(fmt.Sprintf("%s/stream=%t", tc.name, stream), func(t *testing.T) {
				gin.SetMode(gin.TestMode)
				m := miniredis.RunT(t)
				rdb := redis.NewClient(&redis.Options{Addr: m.Addr()})
				defer rdb.Close()
				cache := repository.NewConcurrencyCache(rdb, 15, 60)
				concurrency := service.NewConcurrencyService(cache)
				policy := service.DefaultGPTQoSConfig()
				policy.Version = 1
				policy.QualityRoutingEnabled = tc.enabled
				policy.Enabled = tc.enabled
				policy.LogSampleRate = 0
				group := service.DefaultGPTQoSGroupConfig("premium")
				group.StrongWaitMs = 0
				group.AllowBasicFallback = tc.allow
				policy.Groups["901"] = group
				raw, err := json.Marshal(policy)
				require.NoError(t, err)
				cfg := &config.Config{RunMode: config.RunModeSimple}
				cfg.Gateway.MaxAccountSwitches = 3
				settings := service.NewSettingService(qosHandlerSettings{raw: string(raw)}, cfg)
				accounts := []service.Account{
					{ID: 802, Platform: service.PlatformOpenAI, Type: service.AccountTypeAPIKey, Status: service.StatusActive, Schedulable: true, Concurrency: 1, Priority: 100, GroupIDs: []int64{901}, Credentials: map[string]any{"api_key": "fixture-only", "base_url": "https://fixture.invalid/v1"}, Extra: map[string]any{service.AccountQualityTierKey: "strong", "openai_passthrough": true}},
					{ID: 803, Platform: service.PlatformOpenAI, Type: service.AccountTypeAPIKey, Status: service.StatusActive, Schedulable: true, Concurrency: 1, Priority: 0, GroupIDs: []int64{901}, Credentials: map[string]any{"api_key": "fixture-only", "base_url": "https://fixture.invalid/v1"}, Extra: map[string]any{service.AccountQualityTierKey: "basic", "openai_passthrough": true}},
				}
				repo := &qosHandlerAccountRepo{&openAI502503AdmissionRepo{grokCredentialHandlerRepo: &grokCredentialHandlerRepo{accounts: accounts}}}
				upstream := &openAI502503Upstream{status: 503}
				billing := service.NewBillingCacheService(nil, nil, nil, nil, nil, nil, cfg, nil)
				defer billing.Stop()
				gateway := service.NewOpenAIGatewayService(repo, nil, nil, nil, nil, nil, nil, nil, cfg, nil, concurrency, service.NewBillingService(cfg, nil), nil, billing, upstream, &service.DeferredService{}, nil, nil, nil, nil, nil, settings, nil, nil)
				h := NewOpenAIGatewayHandler(gateway, concurrency, billing, &service.APIKeyService{}, nil, nil, nil, nil, nil, cfg)
				if tc.busy {
					ok, err := cache.AcquireAccountSlot(context.Background(), 802, 1, "fixture-busy")
					require.NoError(t, err)
					require.True(t, ok)
					defer cache.ReleaseAccountSlot(context.Background(), 802, "fixture-busy")
				}
				groupID := int64(901)
				key := &service.APIKey{ID: 902, GroupID: &groupID, User: &service.User{ID: 903, Status: service.StatusActive}, Group: &service.Group{ID: groupID, Platform: service.PlatformOpenAI, Status: service.StatusActive}}
				r := gin.New()
				r.Use(func(c *gin.Context) {
					c.Set(string(middleware.ContextKeyAPIKey), key)
					c.Set(string(middleware.ContextKeyUser), middleware.AuthSubject{UserID: 903, Concurrency: 1})
					c.Request = c.Request.WithContext(context.WithValue(c.Request.Context(), ctxkey.UserID, int64(903)))
					c.Next()
				})
				r.POST("/v1/responses", h.Responses)
				rec := httptest.NewRecorder()
				request := httptest.NewRequest(http.MethodPost, "/v1/responses", strings.NewReader(fmt.Sprintf(`{"model":"gpt-5.2","input":"fixture","stream":%t}`, stream)))
				request.Header.Set("Content-Type", "application/json")
				r.ServeHTTP(rec, request)
				require.Equal(t, tc.wantStatus, rec.Code, rec.Body.String())
				if tc.wantAccount == 0 {
					require.Empty(t, upstream.hits)
				} else {
					require.Equal(t, []int64{tc.wantAccount}, upstream.hits)
				}
				require.Zero(t, rdb.ZCard(context.Background(), "gptqos:{v1}:waiting").Val())
				require.Zero(t, rdb.ZCard(context.Background(), "gptqos:{v1}:leases").Val())
			})
		}
	}
}

func TestGPTQoSLocalCancellationIsNotClient499(t *testing.T) {
	for _, tc := range []struct {
		cause  error
		status int
	}{
		{service.ErrGPTQoSRequestDeadline, http.StatusGatewayTimeout},
		{service.ErrGPTQoSLeaseLost, http.StatusServiceUnavailable},
		{context.Canceled, statusClientClosedRequest},
	} {
		ctx, cancel := context.WithCancelCause(context.Background())
		cancel(tc.cause)
		rec := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(rec)
		c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil).WithContext(ctx)
		require.True(t, failoverClientGone(c))
		c.Writer.WriteHeaderNow()
		require.Equal(t, tc.status, rec.Code)
		// Once output exists, a local deadline cannot replay or append a new
		// JSON response over an already committed SSE response.
		rec = httptest.NewRecorder()
		c, _ = gin.CreateTestContext(rec)
		c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil).WithContext(ctx)
		c.Writer.WriteHeader(http.StatusOK)
		_, _ = c.Writer.WriteString("data: fixture\n\n")
		require.True(t, failoverClientGone(c))
		require.Equal(t, "data: fixture\n\n", rec.Body.String())
		require.Equal(t, http.StatusOK, rec.Code)
	}
}
