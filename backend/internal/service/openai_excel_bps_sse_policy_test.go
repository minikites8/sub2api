package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestExcelBPSStreamRequestScopedRejectionNeverReplays(t *testing.T) {
	const cyberMessage = "The requested Cyber access program is not authorized for this model."
	for _, tc := range []struct {
		name, code, errorType, message string
		status                         int
		echo                           bool
		retry                          bool
	}{
		{name: "blocked session", code: "session_blocked_by_cyber_policy"},
		{name: "blocked session explicit403", code: "session_blocked_by_cyber_policy", status: 403},
		{name: "cyber model denied", code: "permission_denied", errorType: "permission_error", message: cyberMessage, status: 403},
		{name: "content denied", code: "content_policy_violation", status: 403},
		{name: "echoed message is not policy", code: "permission_denied", message: "Account credential not found", status: 403, echo: true, retry: true},
	} {
		for _, stream := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/stream=%t", tc.name, stream), func(t *testing.T) {
				detail := map[string]any{"code": tc.code, "type": tc.errorType, "message": tc.message}
				if tc.status != 0 {
					detail["status"] = tc.status
				}
				payload := map[string]any{"type": "response.failed", "response": map[string]any{"id": "resp_policy", "status": "failed", "output": []any{}, "error": detail}}
				if tc.echo {
					payload["input"] = cyberMessage
				}
				raw, err := json.Marshal(payload)
				require.NoError(t, err)
				upstream := excelBPSRepeatedResponse(200, http.Header{"Content-Type": {"text/event-stream"}}, "data: "+string(raw)+"\n\n", 2)
				svc := openAIClientToolsTestService(upstream)
				account := excelAccount()
				rec := httptest.NewRecorder()
				c, _ := gin.CreateTestContext(rec)
				c.Request = httptest.NewRequest("POST", "/v1/responses", nil)
				_, err = svc.Forward(context.Background(), c, account, []byte(fmt.Sprintf(`{"model":"gpt-6-astra","input":"test","stream":%t}`, stream)))
				require.Error(t, err)
				var failover *UpstreamFailoverError
				require.Equal(t, tc.retry, errors.As(err, &failover))
				if tc.retry {
					require.Len(t, upstream.requests, 2)
					require.False(t, c.Writer.Written())
				} else {
					require.Len(t, upstream.requests, 1, "a request-scoped rejection cannot be made valid by replay")
					require.True(t, c.Writer.Written())
					require.False(t, svc.excelBPSFailureCooldownActive(account.ID))
					require.NotContains(t, rec.Body.String(), cyberMessage)
				}
			})
		}
	}
}

func TestExcelBPSStreamHiddenOutputNeverReplays(t *testing.T) {
	const failure = `{"type":"response.failed","response":{"id":"resp_failed","status":"failed","output":[],"error":{"code":"server_error"}}}`
	const toolDelta = `{"type":"response.function_call_arguments.delta","item_id":"call_test","delta":"SECRET_TOOL_INPUT"}`
	for _, tc := range []struct {
		name, prefix, terminal string
		retry                  bool
		usage                  int
	}{
		{name: "terminal-only text", terminal: `{"type":"response.failed","response":{"status":"failed","output":[{"type":"message","content":[{"type":"output_text","text":"visible generated text"}]}],"error":{"code":"server_error"}}}`},
		{name: "terminal-only tool", terminal: `{"type":"response.failed","response":{"status":"failed","output":[{"type":"function_call","name":"run_officejs","call_id":"call_test","arguments":"SECRET_TOOL_INPUT"}],"error":{"code":"server_error"}}}`},
		{name: "buffered tool arguments then failure", prefix: toolDelta, terminal: failure},
		{name: "buffered tool arguments then EOF", prefix: toolDelta},
		{name: "added tool with arguments then EOF", prefix: `{"type":"response.output_item.added","item":{"type":"function_call","arguments":"SECRET_TOOL_INPUT"}}`},
		{name: "completed pending tool then EOF", prefix: `{"type":"response.output_item.done","item":{"id":"item_test","type":"function_call","name":"run_officejs","call_id":"call_test","arguments":"{}"}}`},
		{name: "metered pending tool", prefix: toolDelta, terminal: `{"type":"response.failed","response":{"status":"failed","output":[],"usage":{"input_tokens":3,"output_tokens":2},"error":{"code":"server_error"}}}`, usage: 3},
		{name: "empty tool shell remains retryable", prefix: `{"type":"response.output_item.added","item":{"id":"item_test","type":"function_call","arguments":""}}`, terminal: failure, retry: true},
		{name: "metadata remains retryable", prefix: `{"type":"response.created","response":{"id":"resp_initial"}}`, terminal: failure, retry: true},
	} {
		for _, stream := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/stream=%t", tc.name, stream), func(t *testing.T) {
				wire := ""
				if tc.prefix != "" {
					wire += "data: " + tc.prefix + "\n\n"
				}
				if tc.terminal != "" {
					wire += "data: " + tc.terminal + "\n\n"
				}
				upstream := excelBPSRepeatedResponse(200, http.Header{"Content-Type": {"text/event-stream"}}, wire, 2)
				svc := openAIClientToolsTestService(upstream)
				rec := httptest.NewRecorder()
				c, _ := gin.CreateTestContext(rec)
				c.Request = httptest.NewRequest("POST", "/v1/responses", nil)
				result, err := svc.Forward(context.Background(), c, excelAccount(), []byte(fmt.Sprintf(`{"model":"gpt-6-astra","input":"test","stream":%t}`, stream)))
				require.Error(t, err)
				var failover *UpstreamFailoverError
				require.Equal(t, tc.retry, errors.As(err, &failover))
				require.NotContains(t, rec.Body.String(), "SECRET_TOOL_INPUT")
				if tc.retry {
					require.Len(t, upstream.requests, 2)
					require.False(t, c.Writer.Written())
				} else {
					require.Len(t, upstream.requests, 1)
					require.NotNil(t, result)
					require.True(t, c.Writer.Written())
					if tc.usage > 0 {
						require.Equal(t, tc.usage, result.Usage.InputTokens)
						require.Equal(t, 2, result.Usage.OutputTokens)
					}
				}
			})
		}
	}
}
