//go:build unit

package handler

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"image"
	"image/png"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	middleware "github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestBPSLocalImageCapabilityFallsBackToNative(t *testing.T) {
	var pngBytes bytes.Buffer
	require.NoError(t, png.Encode(&pngBytes, image.NewRGBA(image.Rect(0, 0, 1, 1))))
	imageURL := "data:image/png;base64," + base64.StdEncoding.EncodeToString(pngBytes.Bytes())
	for _, chat := range []bool{false, true} {
		for _, stream := range []bool{false, true} {
			t.Run(fmt.Sprintf("chat=%t/stream=%t", chat, stream), func(t *testing.T) {
				content := []map[string]any{}
				for i := 0; i < 21; i++ {
					item := map[string]any{"type": "input_image", "image_url": imageURL}
					if chat {
						item = map[string]any{"type": "image_url", "image_url": map[string]any{"url": imageURL}}
					}
					content = append(content, item)
				}
				body := map[string]any{"model": "gpt-6-astra", "stream": stream}
				path := "/v1/responses"
				if chat {
					path = "/v1/chat/completions"
					body["messages"] = []any{map[string]any{"role": "user", "content": content}}
				} else {
					body["input"] = []any{map[string]any{"role": "user", "content": content}}
				}
				raw, err := json.Marshal(body)
				require.NoError(t, err)
				var accounts []service.Account
				for id := int64(801); id <= 808; id++ {
					accounts = append(accounts, service.Account{ID: id, Platform: service.PlatformOpenAI, Type: service.AccountTypeOAuth, Status: service.StatusActive, Schedulable: true, Concurrency: 1, Priority: int(id), Credentials: map[string]any{"access_token": "synthetic-token"}, Extra: map[string]any{"openai_excel_bps": id != 808}})
				}
				upstream := &excelBPSFailoverUpstream{answer: func(call int) *http.Response {
					if call == 0 {
						return &http.Response{StatusCode: 503, Header: http.Header{"Content-Type": {"application/json"}}, Body: io.NopCloser(strings.NewReader(`{"error":{"message":"temporary server failure"}}`))}
					}
					return excelBPSCompleted()
				}}
				router := newBPSLocalCapabilityRouter(t, upstream, accounts)
				rec := httptest.NewRecorder()
				router.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, path, bytes.NewReader(raw)))
				require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
				require.Equal(t, []int64{808, 808}, upstream.calls(), "local capability rejection must skip all other BPS routes without submitting")
				require.NotContains(t, rec.Body.String(), service.ExcelBPSLocalCapabilityReason)
				for i, sent := range upstream.bodies {
					require.Equal(t, 21, strings.Count(string(sent), imageURL), "native retry must retain all original images")
					require.NotContains(t, upstream.urls[i], "basispoints")
				}
				// No native route: reject once locally, never traverse the
				// remaining seven identical BPS routes or send inference.
				blocked := &excelBPSFailoverUpstream{answer: func(int) *http.Response { t.Fatal("BPS inference must not be sent"); return nil }}
				onlyBPS := newBPSLocalCapabilityRouter(t, blocked, accounts[:7])
				failed := httptest.NewRecorder()
				onlyBPS.ServeHTTP(failed, httptest.NewRequest(http.MethodPost, path, bytes.NewReader(raw)))
				require.GreaterOrEqual(t, failed.Code, 400)
				require.Empty(t, blocked.calls())

			})
		}
	}
}

type bpsLocalCapabilitySettings struct{ service.SettingRepository }

func (*bpsLocalCapabilitySettings) GetValue(context.Context, string) (string, error) {
	return "", service.ErrSettingNotFound
}
func (*bpsLocalCapabilitySettings) GetMultiple(context.Context, []string) (map[string]string, error) {
	return map[string]string{service.SettingKeyExcelBPSImageMode: service.ExcelBPSImageModeNative}, nil
}
func newBPSLocalCapabilityRouter(t *testing.T, upstream service.HTTPUpstream, accounts []service.Account) *gin.Engine {
	t.Helper()
	cfg := &config.Config{RunMode: config.RunModeSimple}
	settings := service.NewSettingService(&bpsLocalCapabilitySettings{}, cfg)
	gin.SetMode(gin.TestMode)
	repo := &openAI502503AdmissionRepo{grokCredentialHandlerRepo: &grokCredentialHandlerRepo{accounts: accounts}}
	cfg.Gateway.MaxAccountSwitches = 3
	billing := service.NewBillingCacheService(nil, nil, nil, nil, nil, nil, cfg, nil)
	t.Cleanup(billing.Stop)
	gateway := service.NewOpenAIGatewayService(repo, nil, nil, nil, nil, nil, nil, nil, cfg, nil, nil,
		service.NewBillingService(cfg, nil), nil, billing, upstream, &service.DeferredService{}, nil, nil, nil, nil, nil, settings, nil, nil)
	cache := &concurrencyCacheMock{
		acquireUserSlotFn:    func(context.Context, int64, int, string) (bool, error) { return true, nil },
		acquireAccountSlotFn: func(context.Context, int64, int, string) (bool, error) { return true, nil },
	}
	h := NewOpenAIGatewayHandler(gateway, service.NewConcurrencyService(cache), billing, &service.APIKeyService{}, nil, nil, nil, nil, nil, cfg)
	groupID := int64(901)
	key := &service.APIKey{AllowLongContext: true, ID: 902, GroupID: &groupID, User: &service.User{ID: 903, Status: service.StatusActive},
		Group: &service.Group{ID: groupID, Platform: service.PlatformOpenAI, Status: service.StatusActive, AllowMessagesDispatch: true}}
	r := gin.New()
	r.Use(func(c *gin.Context) {
		c.Set(string(middleware.ContextKeyAPIKey), key)
		c.Set(string(middleware.ContextKeyUser), middleware.AuthSubject{UserID: key.User.ID, Concurrency: 1})
		c.Next()
		if c.Writer.Status() >= 400 {
			if raw, exists := c.Get(service.OpsUpstreamErrorsKey); exists {
				if events, ok := raw.([]*service.OpsUpstreamErrorEvent); ok {
					for _, event := range events {
						t.Logf("native upstream error: %+v", *event)
					}
				}
			}
		}
	})
	r.POST("/v1/responses", h.Responses)
	r.POST("/v1/messages", h.Messages)
	r.POST("/v1/chat/completions", h.ChatCompletions)
	return r
}
