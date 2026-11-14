package service

import (
	"context"
	"fmt"
	"github.com/Wei-Shaw/sub2api/internal/service/basispoints"
	"net/http"
	"net/http/httptest"
	"testing"

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
