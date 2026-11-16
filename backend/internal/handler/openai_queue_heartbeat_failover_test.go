//go:build unit

package handler

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/synctest"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type queueHeartbeatFailureUpstream struct {
	service.HTTPUpstream
	status  int
	partial bool
	hits    []int64
}

func (u *queueHeartbeatFailureUpstream) Do(_ *http.Request, _ string, accountID int64, _ int) (*http.Response, error) {
	u.hits = append(u.hits, accountID)
	if accountID != 801 {
		return excelBPSCompleted(), nil
	}
	detail := `{"code":"upstream_unavailable","message":"Upstream service is unavailable.","type":"server_error","status":502}`
	if u.status == http.StatusTooManyRequests {
		detail = `{"code":"rate_limit_exceeded","message":"429: Too many requests.","type":"rate_limit_error","status":429,"headers":{"retry-after":"1"}}`
	}
	wire := "data: {\"type\":\"response.created\",\"response\":{\"id\":\"resp_failed_queue_attempt\"}}\n\n"
	if u.partial {
		wire += "data: {\"type\":\"response.output_text.delta\",\"delta\":\"partial output\"}\n\n"
	}
	if u.status == http.StatusTooManyRequests {
		wire += "event: error\ndata: {\"type\":\"error\",\"error\":" + detail + "}\n\n"
	} else {
		wire += "event: response.failed\ndata: {\"type\":\"response.failed\",\"response\":{\"status\":\"failed\",\"error\":" + detail + "}}\n\n"
	}
	return &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": {"text/event-stream"}}, Body: io.NopCloser(strings.NewReader(wire))}, nil
}

func queueHeartbeatAccounts(route string) []service.Account {
	var accounts []service.Account
	for _, id := range []int64{801, 802} {
		accounts = append(accounts, service.Account{
			ID: id, Platform: service.PlatformOpenAI, Type: service.AccountTypeOAuth,
			Status: service.StatusActive, Schedulable: true, Concurrency: 1, Priority: int(id),
			Credentials: map[string]any{"access_token": "test-access", "chatgpt_account_id": fmt.Sprint(id)},
			Extra:       map[string]any{"openai_excel_bps": route == "bps", "openai_passthrough": route == "passthrough"},
		})
	}
	return accounts
}

// Exercise the real slot-wait heartbeat before entering the forwarding path.
// Handler-local heartbeat accounting alone cannot tell the service that these
// bytes contain no model output.
func waitForQueueHeartbeat(t *testing.T, slot string, rec *httptest.ResponseRecorder) gin.HandlerFunc {
	t.Helper()
	acquire := func(context.Context, int64, int, string) (bool, error) { return rec.Flushed, nil }
	cache := &concurrencyCacheMock{acquireUserSlotFn: acquire, acquireAccountSlotFn: acquire}
	helper := NewConcurrencyHelper(service.NewConcurrencyService(cache), SSEPingFormatComment, time.Second)
	return func(c *gin.Context) {
		started := false
		release, err := helper.waitForSlotWithPingTimeout(c, slot, 1, 1, 3*time.Second, true, &started, true)
		require.NoError(t, err)
		require.True(t, started)
		require.True(t, gatewayStreamHasOnlyHeartbeats(c))
		defer release()
		c.Next()
	}
}

func TestOpenAIQueueHeartbeatStillRetriesAndSwitches(t *testing.T) {
	for _, route := range []string{"bps", "native", "passthrough"} {
		for _, status := range []int{http.StatusTooManyRequests, http.StatusBadGateway} {
			for _, slot := range []string{"user", "account"} {
				t.Run(fmt.Sprintf("%s/%d/%s", route, status, slot), func(t *testing.T) {
					upstream := &queueHeartbeatFailureUpstream{status: status}
					rec := httptest.NewRecorder()
					router := newBPSLocalCapabilityRouter(t, upstream, queueHeartbeatAccounts(route), waitForQueueHeartbeat(t, slot, rec))
					synctest.Test(t, func(t *testing.T) {
						req := httptest.NewRequest(http.MethodPost, "/v1/responses", strings.NewReader(`{"model":"gpt-6-astra","input":"hello","stream":true}`))
						router.ServeHTTP(rec, req)
					})
					require.GreaterOrEqual(t, len(upstream.hits), 2, rec.Body.String())
					require.Equal(t, int64(801), upstream.hits[0])
					require.Equal(t, int64(802), upstream.hits[len(upstream.hits)-1], rec.Body.String())
					require.LessOrEqual(t, len(upstream.hits), 3, "retry budget must remain bounded")
					if route == "bps" {
						require.Equal(t, []int64{801, 801, 802}, upstream.hits, "preserve the existing BPS same-account retry")
					}
					require.Equal(t, http.StatusOK, rec.Code)
					require.Contains(t, rec.Body.String(), string(SSEPingFormatComment))
					require.Contains(t, rec.Body.String(), "done")
					require.NotContains(t, rec.Body.String(), "upstream_unavailable")
					require.NotContains(t, rec.Body.String(), "rate_limit_exceeded")
					require.NotContains(t, rec.Body.String(), "resp_failed_queue_attempt")
				})
			}
		}
	}
}

func TestOpenAIQueueHeartbeatDoesNotAllowReplayAfterOutput(t *testing.T) {
	for _, route := range []string{"bps", "native", "passthrough"} {
		t.Run(route, func(t *testing.T) {
			upstream := &queueHeartbeatFailureUpstream{status: http.StatusTooManyRequests, partial: true}
			rec := httptest.NewRecorder()
			router := newBPSLocalCapabilityRouter(t, upstream, queueHeartbeatAccounts(route), waitForQueueHeartbeat(t, "account", rec))
			synctest.Test(t, func(t *testing.T) {
				router.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/v1/responses", strings.NewReader(`{"model":"gpt-6-astra","input":"hello","stream":true}`)))
			})
			require.Equal(t, []int64{801}, upstream.hits)
			require.Contains(t, rec.Body.String(), "partial output")
			require.Contains(t, rec.Body.String(), "rate_limit_exceeded")
		})
	}
}
