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
			proxy                bool
		}{
			{"strong beats basic priority", true, false, false, 802, 200, false},
			{"quality guarantee refuses basic", true, true, false, 0, 503, false},
			{"explicit downgrade uses basic", true, true, true, 803, 200, false},
			{"all flags off retains legacy priority", false, false, false, 803, 200, false},
			{"cached single proxy retains its semaphore", true, false, false, 802, 200, true},
			{"cached busy proxy cannot bypass concurrency", true, true, false, 0, 503, true},
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
				strongSlot := int64(802)
				if tc.proxy {
					proxyID := int64(12)
					accounts[0].ProxyID = &proxyID
					accounts[0].Proxy = &service.Proxy{ID: proxyID, Status: service.StatusActive, Protocol: "http", Host: "fixture.invalid", Port: 8080}
					strongSlot = service.AccountProxySlotID(802, proxyID)
				}
				repo := &qosHandlerAccountRepo{&openAI502503AdmissionRepo{grokCredentialHandlerRepo: &grokCredentialHandlerRepo{accounts: accounts}}}
				// Production lists stripped candidate metadata, then hydrates the
				// complete account. Exercise both representations, not just a DB stub.
				snapshotCache := repository.NewSchedulerCache(rdb)
				bucket := service.SchedulerBucket{Platform: service.PlatformOpenAI, Mode: service.SchedulerModeSingle}
				token, err := snapshotCache.CaptureBucketWriteToken(context.Background(), bucket)
				require.NoError(t, err)
				require.NoError(t, snapshotCache.SetSnapshot(context.Background(), bucket, token, accounts))
				snapshot := service.NewSchedulerSnapshotService(snapshotCache, nil, repo, nil, cfg)
				upstream := &openAI502503Upstream{status: 503}
				billing := service.NewBillingCacheService(nil, nil, nil, nil, nil, nil, cfg, nil)
				defer billing.Stop()
				gateway := service.NewOpenAIGatewayService(repo, nil, nil, nil, nil, nil, nil, nil, cfg, snapshot, concurrency, service.NewBillingService(cfg, nil), nil, billing, upstream, &service.DeferredService{}, nil, nil, nil, nil, nil, settings, nil, nil)
				h := NewOpenAIGatewayHandler(gateway, concurrency, billing, &service.APIKeyService{}, nil, nil, nil, nil, nil, cfg)
				if tc.busy {
					ok, err := cache.AcquireAccountSlot(context.Background(), strongSlot, 1, "fixture-busy")
					require.NoError(t, err)
					require.True(t, ok)
					defer cache.ReleaseAccountSlot(context.Background(), strongSlot, "fixture-busy")
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
					var response struct {
						Error struct{ Code, Message string }
					}
					require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &response))
					require.Equal(t, "gpt_qos_queue_timeout", response.Error.Code)
					require.Contains(t, response.Error.Message, "wait budget")
				} else {
					require.Equal(t, []int64{tc.wantAccount}, upstream.hits)
				}
				require.Zero(t, rdb.ZCard(context.Background(), "gptqos:{v1}:waiting").Val())
				require.Zero(t, rdb.ZCard(context.Background(), "gptqos:{v1}:leases").Val())
				if tc.proxy {
					require.Zero(t, rdb.ZCard(context.Background(), "concurrency:account:802").Val(), "a proxy-bound account must not acquire the plain account semaphore")
				}
				if !tc.busy {
					require.Zero(t, rdb.ZCard(context.Background(), fmt.Sprintf("concurrency:account:%d", strongSlot)).Val())
				}
			})
		}
	}
}

func TestGPTQoSSelectionErrorsHaveSafeJSONAndResponsesSSEDiagnostics(t *testing.T) {
	for _, reason := range []string{"account_changed", "no_eligible_capacity", "queue_full", "queue_timeout", "queue_closed", "snapshot_unavailable", "unavailable"} {
		for _, started := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/stream_started=%t", reason, started), func(t *testing.T) {
				rec := httptest.NewRecorder()
				c, _ := gin.CreateTestContext(rec)
				c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
				err := fmt.Errorf("outer wrapper: %w", &service.GPTQoSSelectionError{Reason: reason})
				cls := classifyNoAccountErrorFromGin(c, nil, nil, "gpt-test", "gpt-test", service.PlatformOpenAI, err)
				require.Equal(t, http.StatusServiceUnavailable, cls.Status)
				require.Equal(t, "gpt_qos_"+reason, cls.Code)
				h := &OpenAIGatewayHandler{}
				h.handleStreamingAwareErrorWithCode(c, cls.Status, cls.ErrType, cls.Code, cls.Message, started, false)
				if started {
					require.Contains(t, rec.Body.String(), "event: response.failed")
					require.Contains(t, rec.Body.String(), cls.Code)
					require.NotContains(t, rec.Body.String(), "event: error\n")
				} else {
					require.Equal(t, http.StatusServiceUnavailable, rec.Code)
					var response struct {
						Error struct{ Code, Message string }
					}
					require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &response))
					require.Equal(t, cls.Code, response.Error.Code)
					require.Equal(t, cls.Message, response.Error.Message)
				}
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
