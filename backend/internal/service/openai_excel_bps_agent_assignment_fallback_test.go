package service

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestExcelBPSNewEncryptedAssignmentReturnsNativeFailover(t *testing.T) {
	for _, ignoreHistory := range []bool{false, true} {
		for _, topLevel := range []bool{false, true} {
			for _, heartbeat := range []bool{false, true} {
				t.Run(fmt.Sprintf("ignore=%t/top=%t/heartbeat=%t", ignoreHistory, topLevel, heartbeat), func(t *testing.T) {
					input := make([]any, 121)
					for i := range input {
						input[i] = map[string]any{"role": "user", "content": "previous task"}
					}
					assignment := map[string]any{"type": "agent_message", "content": []any{map[string]any{"type": "input_text", "text": "current task header"}}}
					if topLevel {
						assignment["encrypted_content"] = "PRIVATE_ASSIGNMENT"
					} else {
						assignment["content"] = append(assignment["content"].([]any), map[string]any{"type": "encrypted_content", "data": "PRIVATE_ASSIGNMENT"})
					}
					input = append(input, assignment, map[string]any{"type": "additional_tools", "tools": []any{}})
					body, err := json.Marshal(map[string]any{"model": "gpt-6-astra", "stream": true, "input": input})
					require.NoError(t, err)
					original := bytes.Clone(body)
					upstream := &httpUpstreamRecorder{}
					svc := openAIClientToolsTestService(upstream)
					account := excelAccount()
					account.Extra[ExcelBPSIgnoreEncryptedContentKey] = ignoreHistory
					c, _ := gin.CreateTestContext(httptest.NewRecorder())
					c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", bytes.NewReader(body))
					if heartbeat {
						n, err := c.Writer.WriteString(": keepalive\n\n")
						require.NoError(t, err)
						recordOpenAIStreamKeepaliveBytes(c, n)
						c.Writer.Flush()
					}
					result, err := svc.Forward(c.Request.Context(), c, account, body)
					var failure *UpstreamFailoverError
					require.ErrorAs(t, err, &failure)
					require.Nil(t, result)
					require.Equal(t, GatewayFailureReason(ExcelBPSLocalCapabilityReason), failure.Reason)
					require.True(t, failure.ShouldRetryNextAccount())
					require.False(t, failure.RetryableOnSameAccount)
					require.False(t, failure.ShouldReportAccountScheduleFailure())
					require.Empty(t, upstream.requests, "reject before any upload, compaction or inference")
					require.Equal(t, -1, OpenAICompactKeepaliveAdjustedWrittenSize(c))
					require.False(t, IsResponseCommitted(c))
					require.True(t, account.Schedulable)
					require.False(t, svc.isExcelBPSCoolingDown(account, "gpt-6-astra"))
					require.Equal(t, original, body, "preserve the complete ciphertext for native forwarding")
					require.NotContains(t, string(failure.ResponseBody), "PRIVATE_ASSIGNMENT")
				})
			}
		}
	}
}

func TestExcelBPSNewEncryptedAssignmentDoesNotReplayCommittedOrCancelledRequest(t *testing.T) {
	for _, cancelled := range []bool{false, true} {
		t.Run(fmt.Sprint(cancelled), func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			upstream := &httpUpstreamRecorder{}
			svc := openAIClientToolsTestService(upstream)
			c, _ := gin.CreateTestContext(httptest.NewRecorder())
			c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil).WithContext(ctx)
			if cancelled {
				cancel()
			} else {
				MarkResponseCommitted(c)
			}
			_, err := svc.Forward(ctx, c, excelAccount(), []byte(`{"model":"gpt-6-astra","input":[{"type":"agent_message","encrypted_content":"PRIVATE_ASSIGNMENT"}]}`))
			var failure *UpstreamFailoverError
			require.Error(t, err)
			require.NotErrorAs(t, err, &failure)
			require.Empty(t, upstream.requests)
		})
	}
}
