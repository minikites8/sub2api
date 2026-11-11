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
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/ctxkey"
	"github.com/Wei-Shaw/sub2api/internal/repository"
	"github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/alicebob/miniredis/v2"
	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestExcelBPSNativeBypassWithGPTQoS(t *testing.T) {
	for _, stream := range []bool{false, true} {
		for _, tc := range []struct {
			name, quality string
			busy          []int64
			wantAccount   int64
		}{
			{"ordinary strong", "strong", nil, 803},
			{"ordinary basic", "basic", nil, 803},
			{"skips busy ordinary", "strong", []int64{803}, 804},
			{"ordinary capacity exhausted", "strong", []int64{803, 804}, 0},
		} {
			t.Run(fmt.Sprintf("%s/stream=%t", tc.name, stream), func(t *testing.T) {
				gin.SetMode(gin.TestMode)
				m := miniredis.RunT(t)
				rdb := redis.NewClient(&redis.Options{Addr: m.Addr()})
				t.Cleanup(func() { _ = rdb.Close() })
				cache := repository.NewConcurrencyCache(rdb, 15, 60)
				concurrency := service.NewConcurrencyService(cache)
				policy := service.DefaultGPTQoSConfig()
				policy.Version, policy.Enabled, policy.QualityRoutingEnabled = 1, true, true
				policy.LogSampleRate = 0
				group := service.DefaultGPTQoSGroupConfig("economy")
				group.StrongWaitMs, group.BasicWaitMs, group.Concurrency = 0, 50, 1
				policy.Groups["901"] = group
				raw, err := json.Marshal(policy)
				require.NoError(t, err)
				cfg := &config.Config{RunMode: config.RunModeSimple}
				cfg.Gateway.MaxAccountSwitches = 3
				settings := service.NewSettingService(qosHandlerSettings{raw: string(raw)}, cfg)
				repo := &excelBPSRateLimitAccountRepo{}
				for id := int64(801); id <= 804; id++ {
					lastUsed := time.Unix(id, 0)
					extra := map[string]any{"openai_passthrough": true, service.AccountQualityTierKey: tc.quality}
					if id <= 802 {
						extra["openai_excel_bps"] = true
						extra[service.AccountQualityTierKey] = "strong"
					}
					repo.accounts = append(repo.accounts, service.Account{ID: id, Name: fmt.Sprint(id), Platform: service.PlatformOpenAI,
						Type: service.AccountTypeOAuth, Status: service.StatusActive, Schedulable: true, Concurrency: 1, GroupIDs: []int64{901}, LastUsedAt: &lastUsed,
						Credentials: map[string]any{"access_token": "fixture-only", "chatgpt_account_id": fmt.Sprint("fixture-", id), "expires_at": time.Now().Add(time.Hour).UTC().Format(time.RFC3339)}, Extra: extra})
				}
				snapshotCache := repository.NewSchedulerCache(rdb)
				bucket := service.SchedulerBucket{Platform: service.PlatformOpenAI, Mode: service.SchedulerModeSingle}
				token, err := snapshotCache.CaptureBucketWriteToken(context.Background(), bucket)
				require.NoError(t, err)
				require.NoError(t, snapshotCache.SetSnapshot(context.Background(), bucket, token, repo.accounts))
				snapshot := service.NewSchedulerSnapshotService(snapshotCache, nil, repo, nil, cfg)
				upstream := &excelBPSRateLimitUpstream{}
				billing := service.NewBillingCacheService(nil, nil, nil, nil, nil, nil, cfg, nil)
				t.Cleanup(billing.Stop)
				gateway := service.NewOpenAIGatewayService(repo, nil, nil, nil, nil, nil, nil, nil, cfg, snapshot, concurrency,
					service.NewBillingService(cfg, nil), nil, billing, upstream, &service.DeferredService{}, nil, nil, nil, nil, nil, settings, nil, nil)
				t.Cleanup(func() { require.NoError(t, gateway.CloseExcelBPSImages()) })
				h := NewOpenAIGatewayHandler(gateway, concurrency, billing, &service.APIKeyService{}, nil, nil, nil, nil, nil, cfg)
				for _, id := range tc.busy {
					ok, err := cache.AcquireAccountSlot(context.Background(), id, 1, "fixture-busy")
					require.NoError(t, err)
					require.True(t, ok)
				}
				groupID := int64(901)
				key := &service.APIKey{ID: 902, GroupID: &groupID, User: &service.User{ID: 903, Status: service.StatusActive},
					Group: &service.Group{ID: groupID, Platform: service.PlatformOpenAI, Status: service.StatusActive, EnableBPS: true}}
				var opsEntry service.OpsInsertErrorLogInput
				r := gin.New()
				r.Use(func(c *gin.Context) {
					c.Set(string(middleware.ContextKeyAPIKey), key)
					c.Set(string(middleware.ContextKeyUser), middleware.AuthSubject{UserID: 903, Concurrency: 1})
					c.Request = c.Request.WithContext(context.WithValue(c.Request.Context(), ctxkey.UserID, int64(903)))
					c.Next()
					applyOpsUpstreamFieldsFromContext(c, &opsEntry)
				})
				r.POST("/v1/responses", h.Responses)
				input := `[{"type":"agent_message","content":[{"type":"input_text","text":"retain assignment"},{"type":"encrypted_content","data":"fixture-encrypted"}]}]`
				body := fmt.Sprintf(`{"model":"gpt-6.1-sol","input":%s,"stream":%t}`, input, stream)
				rec := httptest.NewRecorder()
				request := httptest.NewRequest(http.MethodPost, "/v1/responses", strings.NewReader(body))
				request.Header.Set("Content-Type", "application/json")
				request.Header.Set("X-Sub2API-Retry-Account", "true")
				request.Header.Set("X-Sub2API-Max-Retries", "0")
				r.ServeHTTP(rec, request)

				require.NotEmpty(t, opsEntry.UpstreamErrors, "BPS bypass must be recorded before ordinary selection")
				require.Equal(t, "bps_native_bypass", opsEntry.UpstreamErrors[0].Kind)
				require.Equal(t, service.ExcelBPSEncryptedAgentAssignmentReason, opsEntry.UpstreamErrors[0].Reason)
				require.NotContains(t, repo.admissionIDs, int64(802), "all BPS accounts must be excluded after capability bypass")
				if tc.wantAccount == 0 {
					require.Equal(t, http.StatusServiceUnavailable, rec.Code, rec.Body.String())
					require.Equal(t, "gpt_qos_queue_timeout", gjson.GetBytes(rec.Body.Bytes(), "error.code").String())
					require.Empty(t, upstream.hits)
					require.Nil(t, opsEntry.UpstreamErrorMessage, "a routing notice must not mask the final queue error")
				} else {
					require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
					require.Equal(t, []int64{tc.wantAccount}, upstream.hits)
					require.Equal(t, []string{"/backend-api/codex/responses"}, upstream.paths)
					require.Contains(t, string(upstream.bodies[0]), "fixture-encrypted")
				}
				require.Zero(t, rdb.ZCard(context.Background(), "gptqos:{v1}:waiting").Val())
				require.Zero(t, rdb.ZCard(context.Background(), "gptqos:{v1}:leases").Val())
				require.Zero(t, rdb.ZCard(context.Background(), "gptqos:{v1}:group:901").Val())
				for id := int64(801); id <= 802; id++ {
					require.Zero(t, rdb.ZCard(context.Background(), fmt.Sprintf("concurrency:account:%d", id)).Val())
				}
			})
		}
	}
}
