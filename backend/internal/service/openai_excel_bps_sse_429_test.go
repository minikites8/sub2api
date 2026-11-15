package service

import (
	"context"
	"fmt"
	"github.com/Wei-Shaw/sub2api/internal/service/basispoints"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestExcelBPSSSE429SafeRetryPreservesEventRetryAfter(t *testing.T) {
	for _, stream := range []bool{false, true} {
		for _, kind := range []string{"error", "response.failed"} {
			t.Run(fmt.Sprintf("stream=%t/%s", stream, kind), func(t *testing.T) {
				detail := `{"code":"rate_limit_exceeded","headers":{"retry-after":"1"},"message":"rejected"}`
				payload := `{"type":"error","error":` + detail + `}`
				if kind == "response.failed" {
					payload = `{"type":"response.failed","response":{"status":"failed","error":` + detail + `,"output":[],"usage":{"input_tokens":0,"output_tokens":0}}}`
				}
				upstream := excelBPSRepeatedResponse(200, http.Header{"Content-Type": {"text/event-stream"}}, "event: "+kind+"\ndata: "+payload+"\n\n", 2)
				svc := openAIClientToolsTestService(upstream)
				c, _ := gin.CreateTestContext(httptest.NewRecorder())
				c.Request = httptest.NewRequest("POST", "/v1/responses", nil)
				_, err := svc.Forward(context.Background(), c, excelAccount(), []byte(fmt.Sprintf(`{"model":"gpt-6-astra","input":"test","stream":%t}`, stream)))
				var failure *UpstreamFailoverError
				require.ErrorAs(t, err, &failure)
				require.Equal(t, 429, failure.StatusCode)
				require.Equal(t, "1", failure.ResponseHeaders.Get("Retry-After"))
				require.Len(t, upstream.requests, 2)
				require.False(t, c.Writer.Written())
				require.False(t, failure.ShouldReportAccountScheduleFailure())
			})
		}
	}
}

func TestExcelBPSSSE429FailureMessageIsNeutral(t *testing.T) {
	failure := basispoints.ParseUpstreamFailure([]byte(`{"type":"error","error":{"code":"rate_limit_exceeded","headers":{"retry-after":"1"}}}`))
	require.NotNil(t, failure)
	require.Equal(t, "Excel BPS upstream failure: rate_limit_exceeded", failure.Message)
	require.False(t, failure.ReplayUnsafe)
}

func TestExcelBPSSSE429ReplayAfterKeepalive(t *testing.T) {
	for _, mode := range []string{"stream_heartbeat", "compact_heartbeat", "untracked_write", "explicit_commit", "metered_failure", "generated_output", "retryable_false"} {
		t.Run(mode, func(t *testing.T) {
			wire := ""
			if mode == "generated_output" {
				wire = "data: {\"type\":\"response.output_text.delta\",\"delta\":\"accepted text\"}\n\n"
			}
			detail := `{"code":"rate_limit_exceeded","headers":{"retry-after":"1"}}`
			if mode == "retryable_false" {
				detail = `{"code":"rate_limit_exceeded","retryable":false}`
			}
			payload := `{"type":"response.failed","response":{"status":"failed","output":[],"error":` + detail
			if mode == "metered_failure" {
				payload += `,"usage":{"input_tokens":3,"output_tokens":2}`
			}
			wire += "data: " + payload + "}}\n\n"
			upstream := excelBPSRepeatedResponse(200, http.Header{"Content-Type": {"text/event-stream"}}, wire, 2)
			svc := openAIClientToolsTestService(upstream)
			c, _ := gin.CreateTestContext(httptest.NewRecorder())
			c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
			if mode == "compact_heartbeat" {
				MarkOpenAICompactClientStream(c)
				stop := StartOpenAICompactSSEKeepalive(c, time.Hour)
				defer stop()
				value, ok := c.Get(openAICompactSSEKeepaliveKey)
				require.True(t, ok)
				require.True(t, value.(*openAICompactSSEKeepalive).beat())
			} else {
				c.Header("Content-Type", "text/event-stream")
				n, err := c.Writer.WriteString(": keepalive\n\n")
				require.NoError(t, err)
				if mode != "untracked_write" {
					recordOpenAIStreamKeepaliveBytes(c, n)
				}
				c.Writer.Flush()
			}
			if mode == "explicit_commit" {
				MarkResponseCommitted(c)
			}
			require.True(t, c.Writer.Written())
			result, err := svc.Forward(context.Background(), c, excelAccount(), []byte(`{"model":"gpt-6-astra","input":"test","stream":true}`))
			var failure *UpstreamFailoverError
			if mode == "stream_heartbeat" || mode == "compact_heartbeat" {
				require.ErrorAs(t, err, &failure)
				require.Nil(t, result)
				require.Equal(t, http.StatusTooManyRequests, failure.StatusCode)
				require.Equal(t, "1", failure.ResponseHeaders.Get("Retry-After"))
				require.Len(t, upstream.requests, 2)
				require.Equal(t, -1, OpenAICompactKeepaliveAdjustedWrittenSize(c))
				require.False(t, IsResponseCommitted(c))
			} else {
				require.Error(t, err)
				require.NotErrorAs(t, err, &failure)
				require.NotNil(t, result)
				require.Len(t, upstream.requests, 1)
			}
		})
	}
}
