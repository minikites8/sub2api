package service

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestOpenAIEncryptedReasoningStreamReplayBoundary(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, passthrough := range []bool{false, true} {
		for _, timeout := range []int{0, 1} {
			for _, tc := range []struct {
				name, output, firstEvent, firstItem string
				success                             bool
			}{
				{name: "overload_before_content"},
				{name: "text_before_overload", output: `{"type":"response.output_text.delta","delta":"partial"}`, firstEvent: "response.output_text.delta"},
				{name: "summary_before_overload", output: `{"type":"response.reasoning_summary_text.delta","delta":"thinking"}`, firstEvent: "response.reasoning_summary_text.delta"},
				{name: "tool_before_overload", output: `{"type":"response.output_item.added","item":{"type":"function_call","name":"execute","arguments":"{}"}}`, firstEvent: "response.output_item.added", firstItem: "function_call"},
				{name: "successful_text", output: `{"type":"response.output_text.delta","delta":"hello"}`, success: true},
				{name: "successful_reasoning_only", success: true},
			} {
				t.Run(fmt.Sprintf("passthrough=%t/timeout=%d/%s", passthrough, timeout, tc.name), func(t *testing.T) {
					cfg := &config.Config{Gateway: config.GatewayConfig{MaxLineSize: defaultMaxLineSize, OpenAIFirstOutputTimeoutSeconds: timeout, LogUpstreamErrorBody: true, LogUpstreamErrorBodyMaxBytes: 64}}
					svc := &OpenAIGatewayService{cfg: cfg}
					rec := httptest.NewRecorder()
					c, _ := gin.CreateTestContext(rec)
					c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
					// A previous account attempt must not pollute the first-output diagnostic.
					observeOpenAIStreamFirstOutputBoundary(c, nil, "previous_attempt")
					events := []string{
						`{"type":"response.created","sequence_number":0,"response":{"id":"resp_fixture"}}`,
						`{"type":"response.in_progress","sequence_number":1}`,
						`{"type":"response.output_item.added","sequence_number":2,"item":{"id":"rs_fixture","type":"reasoning","summary":[],"encrypted_content":"fixture-ciphertext"}}`,
						`{"type":"response.output_item.done","sequence_number":3,"item":{"id":"rs_fixture","type":"reasoning","summary":[],"encrypted_content":"fixture-ciphertext"}}`,
					}
					if tc.output != "" {
						events = append(events, tc.output)
					}
					if tc.success {
						events = append(events, `{"type":"response.completed","response":{"id":"resp_fixture","status":"completed","output":[{"id":"rs_fixture","type":"reasoning","summary":[],"encrypted_content":"fixture-ciphertext"}],"usage":{"input_tokens":1,"output_tokens":1}}}`)
					} else {
						events = append(events, `{"type":"response.failed","sequence_number":4,"response":{"id":"resp_fixture","instructions":"`+strings.Repeat("x", 256)+`","status":"failed","error":{"code":"server_is_overloaded","message":"Our servers are currently overloaded. Please try again later."}}}`)
					}
					stream := "data: " + strings.Join(events, "\n\ndata: ") + "\n\n"
					resp := &http.Response{StatusCode: http.StatusOK, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(stream))}
					account := &Account{ID: 42, Platform: PlatformOpenAI, Type: AccountTypeOAuth}
					var err error
					if passthrough {
						_, err = svc.handleStreamingResponsePassthrough(c.Request.Context(), resp, c, account, time.Now(), "gpt-6-astra", "gpt-6-astra")
					} else {
						_, err = svc.handleStreamingResponse(c.Request.Context(), resp, c, account, time.Now(), "gpt-6-astra", "gpt-6-astra")
					}
					if tc.success {
						require.NoError(t, err)
						require.Contains(t, rec.Body.String(), "fixture-ciphertext")
						require.Contains(t, rec.Body.String(), "response.completed")
						return
					}
					require.Error(t, err)
					var failover *UpstreamFailoverError
					if tc.output == "" {
						require.ErrorAs(t, err, &failover)
						require.True(t, failover.ShouldRetryNextAccount())
						require.False(t, failover.RetryableOnSameAccount)
						require.Empty(t, rec.Body.String())
						require.False(t, c.Writer.Written())
						return
					}
					require.NotErrorAs(t, err, &failover)
					require.Contains(t, rec.Body.String(), "fixture-ciphertext")
					rawEvents, exists := c.Get(OpsUpstreamErrorsKey)
					require.True(t, exists)
					opsEvents := rawEvents.([]*OpsUpstreamErrorEvent)
					event := opsEvents[len(opsEvents)-1]
					require.Equal(t, "stream_failed", event.Kind)
					require.Equal(t, "downstream_response_committed", event.FailoverSuppressedReason)
					require.Equal(t, tc.firstEvent, event.FirstOutputEventType)
					require.Equal(t, tc.firstItem, event.FirstOutputItemType)
					require.LessOrEqual(t, len(event.Detail), 64)
					// The diagnostic remains available even when the raw response was truncated.
					rawJSON := marshalOpsUpstreamErrors(opsEvents)
					require.NotNil(t, rawJSON)
					parsed, parseErr := ParseOpsUpstreamErrors(*rawJSON)
					require.NoError(t, parseErr)
					require.Equal(t, tc.firstEvent, parsed[len(parsed)-1].FirstOutputEventType)
				})
			}
		}
	}
}
