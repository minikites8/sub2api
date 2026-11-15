package handler

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestExcelBPSExhaustedErrorsAreProtocolIndependent(t *testing.T) {
	for _, status := range []int{400, 401, 403, 404, 413, 422, 429, 500, 502, 503, 504} {
		for _, started := range []bool{false, true} {
			for _, anthropic := range []bool{false, true} {
				t.Run(fmt.Sprintf("%d/started=%t/anthropic=%t", status, started, anthropic), func(t *testing.T) {
					rec := httptest.NewRecorder()
					c, _ := gin.CreateTestContext(rec)
					path := "/v1/responses"
					if anthropic {
						path = "/v1/messages"
					}
					c.Request = httptest.NewRequest(http.MethodPost, path, nil)
					if started {
						_, err := c.Writer.WriteString(": keepalive\n\n")
						require.NoError(t, err)
						c.Writer.Flush()
					}
					failure := &service.UpstreamFailoverError{
						StatusCode: status, Scope: "bps", Reason: "basispoints_upstream_error",
						ResponseBody:    []byte(`{"error":{"code":"basispoints_upstream_error","message":"PRIVATE BPS account scheduling detail"}}`),
						ResponseHeaders: http.Header{"Retry-After": {"1"}},
					}
					if status == 429 {
						failure.Scope, failure.Reason = service.GatewayFailureScopeAccount, service.ExcelBPSRateLimitedReason
						failure.ClientStatusCode, failure.ClientMessage = status, "Excel BPS rate limit exceeded"
					}
					rule := newBodyLimitPassthroughRule(418, "PRIVATE BPS passthrough detail")
					rule.ErrorCodes = []int{status}
					h := newBodyLimitPassthroughHandler(t, rule)
					if anthropic {
						h.handleAnthropicFailoverExhausted(c, failure, started)
					} else {
						h.handleFailoverExhausted(c, failure, started)
					}
					response := rec.Body.String()
					require.NotContains(t, strings.ToLower(response), "bps")
					require.NotContains(t, response, "basispoints")
					require.NotContains(t, response, "PRIVATE")
					require.Contains(t, response, fmt.Sprintf("%d: ", status))
					if !started {
						require.Equal(t, status, rec.Code)
						if status == 422 {
							require.Equal(t, "422: Invalid request body.", gjson.Get(response, "error.message").String())
						}
					} else {
						require.Equal(t, http.StatusOK, rec.Code)
						require.Contains(t, response, "event:")
					}
					require.Equal(t, status, c.GetInt(service.OpsUpstreamStatusCodeKey))
					require.Contains(t, c.GetString(service.OpsUpstreamErrorMessageKey), "PRIVATE BPS")
				})
			}
		}
	}
}
