package service

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestExcelBPSRateLimitReturnsAccountFailover(t *testing.T) {
	const rejection = "Rate limit reached for test-model on tokens per min (TPM). Please try again in 198ms."
	for _, tc := range []struct {
		name   string
		status int
		wire   string
	}{
		{"http", http.StatusTooManyRequests, "{\"error\":{\"code\":\"rate_limit_exceeded\",\"message\":\"" + rejection + "\"}}"},
		{"sse error", http.StatusOK, "event: error\ndata: {\"type\":\"error\",\"error\":{\"code\":\"rate_limit_exceeded\",\"headers\":{\"retry-after\":\"1\",\"retry-after-ms\":\"198\"},\"message\":\"" + rejection + "\"}}\n\n"},
		{"sse response failed", http.StatusOK, "event: response.failed\ndata: {\"type\":\"response.failed\",\"response\":{\"status\":\"failed\",\"error\":{\"code\":\"rate_limit_exceeded\",\"message\":\"" + rejection + "\"}}}\n\n"},
	} {
		for _, stream := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/stream=%t", tc.name, stream), func(t *testing.T) {
				upstream := &httpUpstreamRecorder{resp: &http.Response{StatusCode: tc.status, Header: http.Header{"Content-Type": {"text/event-stream"}, "Retry-After": {"1"}}, Body: io.NopCloser(strings.NewReader(tc.wire))}}
				svc := openAIClientToolsTestService(upstream)
				rec := httptest.NewRecorder()
				c, _ := gin.CreateTestContext(rec)
				c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
				_, err := svc.Forward(context.Background(), c, excelAccount(), []byte(fmt.Sprintf("{\"model\":\"gpt-6-astra\",\"input\":\"test\",\"stream\":%t}", stream)))
				var failover *UpstreamFailoverError
				require.ErrorAs(t, err, &failover)
				require.Equal(t, http.StatusTooManyRequests, failover.StatusCode)
				require.False(t, failover.RetryableOnSameAccount)
				require.True(t, failover.ShouldRetryNextAccount())
				require.False(t, c.Writer.Written())
				require.False(t, IsResponseCommitted(c))
				require.Empty(t, rec.Body.String())
				require.Len(t, upstream.requests, 1)
			})
		}
	}
}

func TestExcelBPSRateLimitStreamReplayBoundary(t *testing.T) {
	const rejection = "event: error\ndata: {\"error\":{\"code\":\"rate_limit_exceeded\",\"message\":\"PRIVATE_UPSTREAM\",\"headers\":{\"retry-after\":\"1\"}}}\n\n"
	const created = "event: response.created\ndata: {\"type\":\"response.created\",\"response\":{\"id\":\"resp_rejected\",\"output\":[]}}\n\n"
	for _, tc := range []struct {
		name, prefix               string
		heartbeat, probe, failover bool
	}{
		{name: "initial error", failover: true},
		{name: "created then error", prefix: created, failover: true},
		{name: "in progress then error", prefix: created + "data: {\"type\":\"response.in_progress\",\"response\":{\"output\":[]}}\n\n", failover: true},
		{name: "after heartbeat", prefix: created, heartbeat: true, failover: true},
		{name: "account probe", probe: true},
		{name: "text already emitted", prefix: created + "data: {\"type\":\"response.output_text.delta\",\"delta\":\"already streamed\"}\n\n"},
		{name: "created contains output", prefix: "data: {\"type\":\"response.created\",\"response\":{\"output\":[{\"type\":\"message\",\"content\":[{\"type\":\"output_text\",\"text\":\"already streamed\"}]}]}}\n\n"},
		{name: "bounded prelude", prefix: "data: {\"type\":\"response.created\",\"response\":{\"output\":[],\"metadata\":\"" + strings.Repeat("x", 65<<10) + "\"}}\n\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			upstream := &httpUpstreamRecorder{resp: &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": {"text/event-stream"}}, Body: io.NopCloser(strings.NewReader(tc.prefix + rejection))}}
			svc := openAIClientToolsTestService(upstream)
			rec := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(rec)
			c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
			if tc.probe {
				c.Set(bpsAccountProbeRequiredContextKey, true)
			}
			if tc.heartbeat {
				_, err := c.Writer.WriteString(": keepalive\n\n")
				require.NoError(t, err)
				c.Writer.Flush()
			}
			_, err := svc.Forward(context.Background(), c, excelAccount(), []byte(`{"model":"gpt-6-astra","input":"test","stream":true}`))
			require.Error(t, err)
			var failover *UpstreamFailoverError
			if tc.failover {
				require.ErrorAs(t, err, &failover)
				require.Equal(t, tc.heartbeat, failover.SafeToFailoverAfterWrite)
				require.Equal(t, "1", failover.ResponseHeaders.Get("Retry-After"))
				require.NotContains(t, string(failover.ResponseBody), "PRIVATE_UPSTREAM")
				require.NotContains(t, rec.Body.String(), "resp_rejected")
				require.NotContains(t, rec.Body.String(), "error")
				require.False(t, IsResponseCommitted(c))
				require.Equal(t, http.StatusTooManyRequests, c.GetInt(OpsUpstreamStatusCodeKey))
			} else {
				require.NotErrorAs(t, err, &failover)
				require.Contains(t, rec.Body.String(), "event: error")
				require.True(t, IsResponseCommitted(c))
			}
			require.Len(t, upstream.requests, 1)
		})
	}
}

func TestExcelBPSRateLimitClassification(t *testing.T) {
	for _, tc := range []struct {
		payload string
		match   bool
	}{
		{`{"error":{"code":"rate_limit_exceeded"}}`, true},
		{`{"response":{"error":{"type":"rate_limit_error"}}}`, true},
		{`{"error":{"status":429}}`, true},
		{`{"error":{"status_code":"429"}}`, true},
		{`{"code":"basispoints_rate_limited"}`, true},
		{`{"error":{"message":"Rate limit reached for test-model"}}`, true},
		{`{"error":{"message":"Rate limit exceeded"}}`, true},
		{`{"error":{"code":"invalid_request_error","message":"invalid field"}}`, false},
		{`{"error":{"code":"permission_denied"}}`, false},
		{`{"error":{"code":"model_not_found"}}`, false},
		{`{"message":"request includes rate limit reached text"}`, false},
		{`not json`, false},
	} {
		t.Run(tc.payload, func(t *testing.T) { require.Equal(t, tc.match, excelBPSRateLimitError([]byte(tc.payload))) })
	}
}

func TestExcelBPSPreludePreservedOnSuccess(t *testing.T) {
	wire := "event: response.created\ndata: {\"type\":\"response.created\",\"response\":{\"id\":\"resp_success\",\"output\":[]}}\n\n" +
		"data: {\"type\":\"response.output_text.delta\",\"delta\":\"hello\"}\n\n" +
		"data: {\"type\":\"response.completed\",\"response\":{\"id\":\"resp_success\",\"status\":\"completed\",\"output\":[]}}\n\n"
	upstream := &httpUpstreamRecorder{resp: &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": {"text/event-stream"}}, Body: io.NopCloser(strings.NewReader(wire))}}
	svc := openAIClientToolsTestService(upstream)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
	_, err := svc.Forward(context.Background(), c, excelAccount(), []byte(`{"model":"gpt-6-astra","input":"test","stream":true}`))
	require.NoError(t, err)
	body := rec.Body.String()
	require.Equal(t, 1, strings.Count(body, "event: response.created"))
	require.Less(t, strings.Index(body, "event: response.created"), strings.Index(body, "event: response.output_text.delta"))
	require.Contains(t, body, "hello")
	require.Contains(t, body, "event: response.completed")
}
