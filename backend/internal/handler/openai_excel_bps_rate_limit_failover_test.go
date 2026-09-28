package handler

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type excelBPSRateLimitAccountRepo struct {
	service.AccountRepository
	accounts []service.Account
}

func (r *excelBPSRateLimitAccountRepo) ListSchedulableByPlatform(_ context.Context, platform string) ([]service.Account, error) {
	var accounts []service.Account
	for _, account := range r.accounts {
		if account.Platform == platform && account.IsSchedulable() {
			accounts = append(accounts, account)
		}
	}
	return accounts, nil
}
func (r *excelBPSRateLimitAccountRepo) ListSchedulableByGroupIDAndPlatform(ctx context.Context, _ int64, platform string) ([]service.Account, error) {
	return r.ListSchedulableByPlatform(ctx, platform)
}
func (r *excelBPSRateLimitAccountRepo) ListSchedulableUngroupedByPlatform(ctx context.Context, platform string) ([]service.Account, error) {
	return r.ListSchedulableByPlatform(ctx, platform)
}
func (r *excelBPSRateLimitAccountRepo) GetByID(_ context.Context, id int64) (*service.Account, error) {
	for _, account := range r.accounts {
		if account.ID == id {
			copy := account
			return &copy, nil
		}
	}
	return nil, nil
}

func (r *excelBPSRateLimitAccountRepo) GetOpenAITurnAdmission(ctx context.Context, id int64) (*service.Account, *service.Account, error) {
	account, err := r.GetByID(ctx, id)
	return account, nil, err
}

type excelBPSRateLimitUpstream struct {
	service.HTTPUpstream
	sse, allFail bool
	hits         []int64
	paths        []string
	prefix       string
}

func (u *excelBPSRateLimitUpstream) Do(req *http.Request, _ string, id int64, _ int) (*http.Response, error) {
	u.hits = append(u.hits, id)
	u.paths = append(u.paths, req.URL.Path)
	if id == 801 || u.allFail {
		status := http.StatusTooManyRequests
		body := `{"error":{"code":"rate_limit_exceeded","headers":{"retry-after":"1"},"message":"Rate limit reached for test-model on tokens per min (TPM). Please try again in 198ms."}}`
		contentType := "application/json"
		if u.sse {
			status = http.StatusOK
			contentType = "text/event-stream"
			body = "event: response.created\ndata: {\"type\":\"response.created\",\"response\":{\"id\":\"resp_throttled\",\"output\":[]}}\n\n" + u.prefix + "event: error\ndata: " + body + "\n\n"
		}
		return &http.Response{StatusCode: status, Header: http.Header{"Content-Type": {contentType}, "Retry-After": {"1"}}, Body: io.NopCloser(strings.NewReader(body))}, nil
	}
	body := `{"type":"response.completed","response":{"id":"resp_healthy","object":"response","model":"gpt-6-sol","status":"completed","output":[{"id":"msg_healthy","type":"message","role":"assistant","status":"completed","content":[{"type":"output_text","text":"ok","annotations":[]}]}],"usage":{"input_tokens":1,"output_tokens":1}}}`
	return &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": {"text/event-stream"}}, Body: io.NopCloser(strings.NewReader("data: " + body + "\n\n"))}, nil
}

func newExcelBPSRateLimitRouter(t *testing.T, upstream *excelBPSRateLimitUpstream) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)
	repo := &excelBPSRateLimitAccountRepo{}
	for i := int64(801); i <= 802; i++ {
		repo.accounts = append(repo.accounts, service.Account{ID: i, Name: fmt.Sprint(i), Platform: service.PlatformOpenAI,
			Type: service.AccountTypeOAuth, Status: service.StatusActive, Schedulable: true, Concurrency: 1, Priority: int(i),
			Credentials: map[string]any{"access_token": "test-access", "chatgpt_account_id": fmt.Sprint("acct-", i), "expires_at": time.Now().Add(time.Hour).UTC().Format(time.RFC3339)},
			Extra:       map[string]any{"openai_passthrough": true, "openai_excel_bps": true}})
	}
	cfg := &config.Config{RunMode: config.RunModeSimple}
	cfg.Gateway.MaxAccountSwitches = 3
	billing := service.NewBillingCacheService(nil, nil, nil, nil, nil, nil, cfg, nil)
	t.Cleanup(billing.Stop)
	gateway := service.NewOpenAIGatewayService(repo, nil, nil, nil, nil, nil, nil, nil, cfg, nil, nil,
		service.NewBillingService(cfg, nil), nil, billing, upstream, &service.DeferredService{}, nil, nil, nil, nil, nil, nil, nil, nil)
	cache := &concurrencyCacheMock{
		acquireUserSlotFn:    func(context.Context, int64, int, string) (bool, error) { return true, nil },
		acquireAccountSlotFn: func(context.Context, int64, int, string) (bool, error) { return true, nil },
	}
	h := NewOpenAIGatewayHandler(gateway, service.NewConcurrencyService(cache), billing, &service.APIKeyService{}, nil, nil, nil, nil, nil, cfg)
	groupID := int64(901)
	key := &service.APIKey{ID: 902, GroupID: &groupID, User: &service.User{ID: 903, Status: service.StatusActive},
		Group: &service.Group{ID: groupID, Platform: service.PlatformOpenAI, Status: service.StatusActive, AllowMessagesDispatch: true, EnableBPS: true}}
	r := gin.New()
	r.Use(func(c *gin.Context) {
		c.Set(string(middleware.ContextKeyAPIKey), key)
		c.Set(string(middleware.ContextKeyUser), middleware.AuthSubject{UserID: key.User.ID, Concurrency: 1})
		c.Next()
	})
	r.POST("/v1/responses", h.Responses)
	return r
}

func TestExcelBPSRateLimitHandlerSwitchesAccount(t *testing.T) {
	for _, endpoint := range []struct{ path, body string }{
		{"/v1/responses", `{"model":"gpt-6-sol","input":"hello","stream":false}`},
		{"/v1/responses", `{"model":"gpt-6-sol","input":"hello","stream":true}`},
	} {
		for _, sse := range []bool{false, true} {
			for _, allFail := range []bool{false, true} {
				t.Run(fmt.Sprintf("%s/sse=%t/allFail=%t/%s", endpoint.path, sse, allFail, endpoint.body), func(t *testing.T) {
					upstream := &excelBPSRateLimitUpstream{sse: sse, allFail: allFail}
					router := newExcelBPSRateLimitRouter(t, upstream)
					rec := httptest.NewRecorder()
					req := httptest.NewRequest(http.MethodPost, endpoint.path, bytes.NewBufferString(endpoint.body))
					req.Header.Set("Content-Type", "application/json")
					router.ServeHTTP(rec, req)
					require.Equal(t, []int64{801, 802}, upstream.hits, "status=%d body=%s", rec.Code, rec.Body.String())
					for _, path := range upstream.paths {
						require.Contains(t, path, "basispoints/api/responses")
					}
					require.NotEmpty(t, rec.Body.String())
					require.NotContains(t, rec.Body.String(), "resp_throttled")
					if allFail {
						require.Equal(t, http.StatusTooManyRequests, rec.Code, rec.Body.String())
						require.Contains(t, rec.Body.String(), "error")
						require.Equal(t, "1", rec.Header().Get("Retry-After"))
					} else {
						require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
						require.Contains(t, rec.Body.String(), "ok")
						require.NotContains(t, rec.Body.String(), "rate_limit")
					}
				})
			}
		}
	}
}

func TestExcelBPSRateLimitHandlerSwitchesAfterEmptyEvents(t *testing.T) {
	for _, prefix := range []string{
		"data: {\"type\":\"response.output_item.added\",\"item\":{\"id\":\"rs_empty\",\"type\":\"reasoning\",\"summary\":[]}}\n\n",
		"data: {\"type\":\"response.output_item.added\",\"item\":{\"id\":\"msg_empty\",\"type\":\"message\",\"content\":[]}}\n\ndata: {\"type\":\"response.content_part.added\",\"part\":{\"type\":\"output_text\",\"text\":\"\"}}\n\n",
		"data: {\"type\":\"response.output_text.delta\",\"delta\":\"\"}\n\n",
		"data: {\"type\":\"response.created\",\"response\":{\"output\":[],\"metadata\":\"" + strings.Repeat("x", 65<<10) + "\"}}\n\n",
	} {
		for _, stream := range []bool{false, true} {
			for _, allFail := range []bool{false, true} {
				t.Run(fmt.Sprintf("prefixLen=%d/stream=%t/allFail=%t", len(prefix), stream, allFail), func(t *testing.T) {
					upstream := &excelBPSRateLimitUpstream{sse: true, allFail: allFail, prefix: prefix}
					router := newExcelBPSRateLimitRouter(t, upstream)
					rec := httptest.NewRecorder()
					request := httptest.NewRequest(http.MethodPost, "/v1/responses", strings.NewReader(fmt.Sprintf(`{"model":"gpt-6-sol","input":"hello","stream":%t}`, stream)))
					request.Header.Set("Content-Type", "application/json")
					router.ServeHTTP(rec, request)
					require.Equal(t, []int64{801, 802}, upstream.hits, "status=%d body=%s", rec.Code, rec.Body.String())
					require.NotContains(t, rec.Body.String(), "resp_throttled")
					require.NotContains(t, rec.Body.String(), "rs_empty")
					require.NotContains(t, rec.Body.String(), "msg_empty")
					require.NotEmpty(t, rec.Body.String())
					if allFail {
						require.Equal(t, http.StatusTooManyRequests, rec.Code)
						require.Contains(t, rec.Body.String(), "error")
						require.Equal(t, "1", rec.Header().Get("Retry-After"))
					} else {
						require.Equal(t, http.StatusOK, rec.Code)
						require.Contains(t, rec.Body.String(), "ok")
						require.NotContains(t, rec.Body.String(), "rate_limit")
					}
				})
			}
		}
	}
}
