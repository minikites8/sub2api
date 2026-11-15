package service

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service/basispoints"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestExcelBPS422ClientErrorDoesNotExposeProtocol(t *testing.T) {
	for _, stream := range []bool{false, true} {
		for _, heartbeat := range []bool{false, true} {
			for _, inBand := range []bool{false, true} {
				t.Run(fmt.Sprintf("stream=%t/heartbeat=%t/in_band=%t", stream, heartbeat, inBand), func(t *testing.T) {
					const rejection = `{"error":{"message":"422: Invalid request body.","type":"server_error"}}`
					status, wire := http.StatusUnprocessableEntity, rejection
					if inBand {
						status = http.StatusOK
						wire = "event: response.failed\ndata: {\"type\":\"response.failed\",\"response\":{\"status\":\"failed\",\"output\":[],\"error\":{\"status\":422,\"code\":\"basispoints_upstream_error\",\"message\":\"PRIVATE BPS detail\"}}}\n\n"
					}
					upstream := excelBPSRepeatedResponse(status, http.Header{"Content-Type": {"text/event-stream"}}, wire, 1)
					gateway := openAIClientToolsTestService(upstream)
					gateway.cfg.Gateway.LogUpstreamErrorBody = true
					rec := httptest.NewRecorder()
					c, _ := gin.CreateTestContext(rec)
					c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
					if heartbeat {
						MarkOpenAICompactClientStream(c)
						stop := StartOpenAICompactSSEKeepalive(c, time.Hour)
						defer stop()
						require.True(t, c.MustGet(openAICompactSSEKeepaliveKey).(*openAICompactSSEKeepalive).beat())
					}
					_, err := gateway.Forward(context.Background(), c, excelAccount(), []byte(fmt.Sprintf(`{"model":"gpt-6-astra","input":"test","stream":%t}`, stream)))
					require.Error(t, err)
					require.Len(t, upstream.requests, 1, "422 must retain its existing non-retryable behavior")
					response := rec.Body.String()
					require.Contains(t, response, "422: Invalid request body.")
					require.NotContains(t, strings.ToLower(response), "bps")
					require.NotContains(t, strings.ToLower(response), "basispoints")
					require.NotContains(t, response, "PRIVATE")
					if heartbeat || inBand && stream {
						require.Equal(t, http.StatusOK, rec.Code)
					} else {
						require.Equal(t, http.StatusUnprocessableEntity, rec.Code)
						require.Equal(t, "422: Invalid request body.", gjson.Get(response, "error.message").String())
						require.Equal(t, "invalid_request_error", gjson.Get(response, "error.code").String())
					}
					if !inBand {
						events := c.MustGet(OpsUpstreamErrorsKey).([]*OpsUpstreamErrorEvent)
						require.Equal(t, http.StatusUnprocessableEntity, events[0].UpstreamStatusCode)
						require.Equal(t, basispoints.ResponsesURL, events[0].UpstreamURL)
						require.Contains(t, events[0].Detail, "422: Invalid request body.")
					}
				})
			}
		}
	}
}

func TestExcelBPSHTTPClientErrorsKeepTheirCategory(t *testing.T) {
	for _, tc := range []struct {
		status                   int
		code, errorType, message string
	}{
		{400, "invalid_request_error", "invalid_request_error", "400: Invalid request body."},
		{401, "authentication_error", "authentication_error", "401: Unauthorized."},
		{403, "permission_denied", "permission_error", "403: Forbidden."},
		{404, "not_found", "invalid_request_error", "404: Not Found."},
		{408, "request_timeout", "invalid_request_error", "408: Request Timeout."},
		{413, "request_too_large", "invalid_request_error", "413: Request body too large."},
		{422, "invalid_request_error", "invalid_request_error", "422: Invalid request body."},
		{500, "server_error", "server_error", "500: Internal Server Error."},
		{502, "bad_gateway", "server_error", "502: Bad gateway."},
		{503, "service_unavailable", "server_error", "503: Service unavailable."},
		{504, "gateway_timeout", "server_error", "504: Gateway timeout."},
	} {
		for _, stream := range []bool{false, true} {
			t.Run(fmt.Sprintf("%d/stream=%t", tc.status, stream), func(t *testing.T) {
				upstream := excelBPSRepeatedResponse(tc.status, http.Header{}, `{"error":{"code":"basispoints_upstream_error","message":"PRIVATE BPS routing detail","retryable":false}}`, 1)
				gateway := openAIClientToolsTestService(upstream)
				rec := httptest.NewRecorder()
				c, _ := gin.CreateTestContext(rec)
				c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
				_, err := gateway.Forward(context.Background(), c, excelAccount(), []byte(fmt.Sprintf(`{"model":"gpt-6-astra","input":"test","stream":%t}`, stream)))
				require.Error(t, err)
				require.Len(t, upstream.requests, 1, "serialization must not change replay safety")
				require.Equal(t, tc.status, rec.Code)
				require.Equal(t, tc.code, gjson.Get(rec.Body.String(), "error.code").String())
				require.Equal(t, tc.errorType, gjson.Get(rec.Body.String(), "error.type").String())
				require.Equal(t, tc.message, gjson.Get(rec.Body.String(), "error.message").String())
				require.NotContains(t, rec.Body.String(), "basispoints")
				require.NotContains(t, rec.Body.String(), "BPS")
				require.NotContains(t, rec.Body.String(), "PRIVATE")
			})
		}
	}
}

func TestExcelBPSChatFailureRetainsPublicStatus(t *testing.T) {
	for _, stream := range []bool{false, true} {
		for _, terminal := range []string{"error", "response.failed", "response.cancelled", "response.incomplete"} {
			t.Run(fmt.Sprintf("stream=%t/%s", stream, terminal), func(t *testing.T) {
				rec := httptest.NewRecorder()
				c, _ := gin.CreateTestContext(rec)
				writer := newExcelBPSChatWriter(c.Writer, "gpt-6-astra", stream)
				payload := []byte(fmt.Sprintf(`{"type":%q,"response":{"status":"failed","error":{"code":"basispoints_protocol_error","message":"PRIVATE BPS detail","failure_stage":"bridge_tool_validation"}}}`, terminal))
				_, err := writer.WriteString("data: " + string(excelBPSPublicFailureEvent(payload, 422)) + "\n\n")
				require.NoError(t, err)
				require.Contains(t, rec.Body.String(), "422: Invalid request body.")
				require.NotContains(t, rec.Body.String(), "basispoints")
				require.NotContains(t, rec.Body.String(), "PRIVATE")
				require.NotContains(t, rec.Body.String(), "failure_stage")
				if stream {
					require.Contains(t, rec.Body.String(), "data: [DONE]")
				} else {
					require.Equal(t, http.StatusUnprocessableEntity, rec.Code)
					require.Equal(t, "invalid_request_error", gjson.Get(rec.Body.String(), "error.type").String())
				}
			})
		}
	}
}

func TestExcelBPSPublicFailurePreservesRetrySafety(t *testing.T) {
	for _, stream := range []bool{false, true} {
		t.Run(fmt.Sprint(stream), func(t *testing.T) {
			wire := "data: {\"type\":\"response.failed\",\"response\":{\"status\":\"failed\",\"error\":{\"status\":429,\"code\":\"rate_limit_exceeded\",\"message\":\"PRIVATE BPS detail\",\"retryable\":false,\"headers\":{\"retry-after\":\"1\",\"private\":\"BPS internal\"}}}}\n\n"
			upstream := excelBPSRepeatedResponse(200, http.Header{}, wire, 1)
			gateway := openAIClientToolsTestService(upstream)
			rec := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(rec)
			c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
			_, err := gateway.Forward(context.Background(), c, excelAccount(), []byte(fmt.Sprintf(`{"model":"gpt-6-astra","input":"test","stream":%t}`, stream)))
			require.Error(t, err)
			require.Len(t, upstream.requests, 1)
			response := rec.Body.String()
			detail := gjson.Get(response, "error")
			if stream {
				for _, line := range strings.Split(response, "\n") {
					if strings.HasPrefix(line, "data: ") {
						detail = gjson.Get(strings.TrimPrefix(line, "data: "), "response.error")
					}
				}
			}
			require.Equal(t, "429: Too many requests.", detail.Get("message").String())
			require.Equal(t, gjson.False, detail.Get("retryable").Type)
			require.Equal(t, "1", detail.Get("headers.retry-after").String())
			require.NotContains(t, response, "BPS")
			require.NotContains(t, response, "PRIVATE")
			require.False(t, detail.Get("headers.private").Exists())
		})
	}
}
