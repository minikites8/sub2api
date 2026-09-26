//go:build unit

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

type openAI502503Upstream struct {
	service.HTTPUpstream
	status                int
	sse, recover, allFail bool
	hits                  []int64
	cancel                context.CancelFunc
}

func (u *openAI502503Upstream) Do(_ *http.Request, _ string, id int64, _ int) (*http.Response, error) {
	u.hits = append(u.hits, id)
	if u.cancel != nil {
		u.cancel()
	}
	if (id == 801 || u.allFail) && !(u.recover && len(u.hits) > 1) {
		body := fmt.Sprintf(`{"type":"error","error":{"status_code":%d,"type":"server_error","message":"Upstream request failed"}}`, u.status)
		status, contentType := u.status, "application/json"
		if u.sse {
			body = "data: {\"type\":\"response.created\",\"response\":{\"id\":\"resp_failed_attempt\"}}\n\ndata: " + body + "\n\n"
			status, contentType = 200, "text/event-stream"
		}
		return &http.Response{StatusCode: status, Header: http.Header{"Content-Type": {contentType}}, Body: io.NopCloser(strings.NewReader(body))}, nil
	}
	body := `{"type":"response.completed","response":{"id":"resp_healthy","object":"response","model":"gpt-5.2","status":"completed","output":[{"id":"msg_healthy","type":"message","role":"assistant","status":"completed","content":[{"type":"output_text","text":"ok","annotations":[]}]}],"usage":{"input_tokens":1,"output_tokens":1}}}`
	return &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": {"text/event-stream"}}, Body: io.NopCloser(strings.NewReader("data: " + body + "\n\n"))}, nil
}

func newOpenAI502503Router(t *testing.T, upstream *openAI502503Upstream) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)
	repo := &grokCredentialHandlerRepo{}
	for i := int64(801); i <= 802; i++ {
		repo.accounts = append(repo.accounts, service.Account{ID: i, Name: fmt.Sprint(i), Platform: service.PlatformOpenAI,
			Type: service.AccountTypeOAuth, Status: service.StatusActive, Schedulable: true, Concurrency: 1, Priority: int(i),
			Credentials: map[string]any{"access_token": "test-access", "expires_at": time.Now().Add(time.Hour).UTC().Format(time.RFC3339)},
			Extra:       map[string]any{"openai_passthrough": true}})
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
		Group: &service.Group{ID: groupID, Platform: service.PlatformOpenAI, Status: service.StatusActive, AllowMessagesDispatch: true}}
	r := gin.New()
	r.Use(func(c *gin.Context) {
		c.Set(string(middleware.ContextKeyAPIKey), key)
		c.Set(string(middleware.ContextKeyUser), middleware.AuthSubject{UserID: key.User.ID, Concurrency: 1})
		c.Next()
	})
	r.POST("/v1/responses", h.Responses)
	r.POST("/v1/messages", h.Messages)
	r.POST("/v1/chat/completions", h.ChatCompletions)
	return r
}

func TestOpenAI502503HandlerRetriesAndSwitches(t *testing.T) {
	for _, endpoint := range []struct{ path, body string }{
		{"/v1/responses", `{"model":"gpt-5.2","input":"hello","stream":false}`},
		{"/v1/responses", `{"model":"gpt-5.2","input":"hello","stream":true}`},
		{"/v1/messages", `{"model":"gpt-5.2","messages":[{"role":"user","content":"hello"}],"max_tokens":32,"stream":true}`},
		{"/v1/chat/completions", `{"model":"gpt-5.2","messages":[{"role":"user","content":"hello"}],"stream":true}`},
	} {
		for _, status := range []int{502, 503} {
			for _, sse := range []bool{false, true} {
				t.Run(fmt.Sprintf("%s/%d/sse_%t/%s", endpoint.path, status, sse, endpoint.body), func(t *testing.T) {
					u := &openAI502503Upstream{status: status, sse: sse}
					r := newOpenAI502503Router(t, u)
					rec := httptest.NewRecorder()
					req := httptest.NewRequest(http.MethodPost, endpoint.path, bytes.NewBufferString(endpoint.body))
					req.Header.Set("Content-Type", "application/json")
					r.ServeHTTP(rec, req)
					require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
					require.Equal(t, []int64{801, 801, 801, 802}, u.hits)
					require.NotContains(t, rec.Body.String(), "Upstream request failed")
					require.NotContains(t, rec.Body.String(), "resp_failed_attempt")
				})
			}
		}
	}
}

func TestOpenAI502503HandlerRetryBoundaries(t *testing.T) {
	for _, mode := range []string{"recovery", "exhausted", "cancelled"} {
		t.Run(mode, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			u := &openAI502503Upstream{status: 503, recover: mode == "recovery", allFail: mode == "exhausted"}
			if mode == "cancelled" {
				u.cancel = cancel
			}
			r := newOpenAI502503Router(t, u)
			rec := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodPost, "/v1/responses", strings.NewReader(`{"model":"gpt-5.2","input":"hello","stream":false}`)).WithContext(ctx)
			r.ServeHTTP(rec, req)
			switch mode {
			case "recovery":
				require.Equal(t, []int64{801, 801}, u.hits)
				require.Equal(t, 200, rec.Code)
			case "exhausted":
				require.Equal(t, []int64{801, 801, 801, 802, 802, 802}, u.hits)
				require.GreaterOrEqual(t, rec.Code, 500)
			case "cancelled":
				require.Equal(t, []int64{801}, u.hits)
			}
		})
	}
}
