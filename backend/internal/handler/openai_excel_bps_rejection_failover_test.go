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

	"github.com/Wei-Shaw/sub2api/internal/service/basispoints"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func excelBPSRejected(status int) *http.Response {
	return &http.Response{
		StatusCode: status,
		Header:     http.Header{"Content-Type": {"application/json"}},
		Body:       io.NopCloser(strings.NewReader(`{"error":{"code":"permission_denied","message":"PRIVATE_UPSTREAM account detail"}}`)),
	}
}

func newExcelBPSRejectionContext(ctx context.Context, stream, chat bool) (*gin.Context, *httptest.ResponseRecorder) {
	c, rec := newExcelBPSFailoverTestContext(ctx, stream)
	if chat {
		c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(fmt.Sprintf(
			`{"model":"gpt-6-astra","stream":%t,"messages":[{"role":"user","content":"hello"}]}`, stream))).WithContext(c.Request.Context())
		c.Request.Header.Set("Content-Type", "application/json")
	}
	return c, rec
}

func TestExcelBPSRejectionSwitchesAccountAndSkipsItOnNextRequest(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, status := range []int{403, 502, 503} {
		for _, chat := range []bool{false, true} {
			for _, stream := range []bool{false, true} {
				for _, loadBatch := range []bool{false, true} {
					t.Run(fmt.Sprintf("status=%d/chat=%t/stream=%t/load_batch=%t", status, chat, stream, loadBatch), func(t *testing.T) {
						upstream := &excelBPSFailoverUpstream{answer: func(call int) *http.Response {
							if call < 3 {
								return excelBPSRejected(status)
							}
							return excelBPSCompleted()
						}}
						handler := newExcelBPSFailoverTestHandler(t, upstream, loadBatch)
						forward := handler.Responses
						if chat {
							forward = handler.ChatCompletions
						}
						c, rec := newExcelBPSRejectionContext(context.Background(), stream, chat)
						forward(c)

						calls := upstream.calls()
						require.Len(t, calls, 4, "each BPS account gets one retry before switching: %s", rec.Body.String())
						require.Equal(t, calls[0], calls[1], "the first account must actually retry itself")
						require.Equal(t, calls[2], calls[3], "the next account also gets one retry")
						require.NotEqual(t, calls[0], calls[2])
						require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
						require.Contains(t, rec.Body.String(), "done")
						require.NotContains(t, rec.Body.String(), "PRIVATE_UPSTREAM")
						require.NotContains(t, rec.Body.String(), "basispoints_upstream_error")
						for _, url := range upstream.urls {
							require.Equal(t, basispoints.ResponsesURL, url, "BPS accounts must use the BPS protocol on both ingress routes")
						}
						for _, body := range upstream.bodies[1:] {
							require.Equal(t, gjson.GetBytes(upstream.bodies[0], "input").Raw, gjson.GetBytes(body, "input").Raw,
								"retrying and switching accounts must preserve the request history")
						}
						if chat {
							if stream {
								require.Contains(t, rec.Body.String(), `"object":"chat.completion.chunk"`)
								require.Contains(t, rec.Body.String(), "data: [DONE]")
							} else {
								require.Equal(t, "chat.completion", gjson.GetBytes(rec.Body.Bytes(), "object").String())
							}
						}

						c, rec = newExcelBPSRejectionContext(context.Background(), stream, chat)
						forward(c)
						require.Equal(t, []int64{calls[0], calls[0], calls[2], calls[2], calls[2]}, upstream.calls(), "a later request must skip the failed route without cooling an account whose retry recovered")
						require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
					})
				}
			}
		}
	}
}

func TestExcelBPSRejectionRetryBoundaries(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, mode := range []string{"all_rejected", "cancelled", "invalid_request"} {
		t.Run(mode, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			upstream := &excelBPSFailoverUpstream{answer: func(int) *http.Response {
				if mode == "invalid_request" {
					return excelBPSRejected(http.StatusBadRequest)
				}
				return excelBPSRejected(http.StatusForbidden)
			}}
			if mode == "cancelled" {
				upstream.onDo = func(int) { cancel() }
			}
			handler := newExcelBPSFailoverTestHandler(t, upstream, false)
			c, rec := newExcelBPSRejectionContext(ctx, false, false)
			handler.Responses(c)
			if mode == "all_rejected" {
				require.Len(t, upstream.calls(), 4, "an exhausted pool stops after one retry per account")
				require.Equal(t, upstream.calls()[0], upstream.calls()[1])
				require.Equal(t, upstream.calls()[2], upstream.calls()[3])
				require.NotEqual(t, upstream.calls()[0], upstream.calls()[2])
				require.GreaterOrEqual(t, rec.Code, 400)
			} else {
				require.Len(t, upstream.calls(), 1)
				if mode == "cancelled" {
					require.Equal(t, statusClientClosedRequest, c.Writer.Status())
					require.Zero(t, rec.Body.Len())
				} else {
					require.Equal(t, http.StatusBadRequest, rec.Code)
				}
			}
			require.NotContains(t, rec.Body.String(), "PRIVATE_UPSTREAM")
		})
	}
}

func TestExcelBPSRejectionAttemptsAtMostFiveAccounts(t *testing.T) {
	gin.SetMode(gin.TestMode)
	upstream := &excelBPSFailoverUpstream{answer: func(int) *http.Response { return excelBPSRejected(http.StatusForbidden) }}
	handler := newExcelBPSFailoverTestHandler(t, upstream, false, 7)
	c, rec := newExcelBPSRejectionContext(context.Background(), false, false)
	handler.Responses(c)
	calls := upstream.calls()
	require.Len(t, calls, 10, "five accounts, each with an initial attempt and one retry")
	seen := make(map[int64]bool)
	for i := 0; i < len(calls); i += 2 {
		require.Equal(t, calls[i], calls[i+1])
		require.False(t, seen[calls[i]], "an exhausted account must not be selected again")
		seen[calls[i]] = true
	}
	require.Len(t, seen, 5)
	require.GreaterOrEqual(t, rec.Code, 400)
}
