//go:build unit

package service

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/apicompat"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestCopyOpenAIUsageFromResponsesUsageTrustsCanonicalCacheCreationValue(t *testing.T) {
	usage := &apicompat.ResponsesUsage{
		InputTokens:              20,
		OutputTokens:             2,
		CacheCreationInputTokens: 0,
		InputTokensDetails: &apicompat.ResponsesInputTokensDetails{
			CachedTokens:     3,
			CacheWriteTokens: 19,
		},
	}

	got := copyOpenAIUsageFromResponsesUsage(usage)

	require.Equal(t, 20, got.InputTokens)
	require.Equal(t, 3, got.CacheReadInputTokens)
	require.Zero(t, got.CacheCreationInputTokens)
}

func TestHandleAnthropicStreamingResponseMergesKiroCredits(t *testing.T) {
	gin.SetMode(gin.TestMode)

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", nil)
	resp := &http.Response{
		Header: http.Header{"x-request-id": []string{"rid_messages_stream_kiro_credits"}},
		Body: io.NopCloser(strings.NewReader(strings.Join([]string{
			`data: {"type":"response.created","response":{"id":"resp_kiro","model":"gpt-5.6-terra","status":"in_progress","output":[]}}`,
			``,
			`data: {"type":"response.output_text.delta","delta":"ok"}`,
			``,
			`data: {"type":"response.completed","response":{"id":"resp_kiro","object":"response","model":"gpt-5.6-terra","status":"completed","output":[{"type":"message","id":"msg_1","role":"assistant","status":"completed","content":[{"type":"output_text","text":"ok"}]}],"usage":{"input_tokens":31,"output_tokens":7,"total_tokens":38,"input_tokens_details":{"cached_tokens":5},"_sub2api_kiro_credits":0.41}}}`,
			``,
		}, "\n"))),
	}

	svc := &OpenAIGatewayService{}
	result, err := svc.handleAnthropicStreamingResponse(resp, c, &Account{ID: 1, Platform: PlatformKiro}, "gpt-5.6-terra", "gpt-5.6-terra", "gpt-5.6-terra", time.Now())
	require.NoError(t, err)
	require.NotNil(t, result)
	require.Equal(t, 31, result.Usage.InputTokens)
	require.Equal(t, 7, result.Usage.OutputTokens)
	require.Equal(t, 5, result.Usage.CacheReadInputTokens)
	require.InDelta(t, 0.41, result.Usage.KiroCredits, 0.000001)
	require.Contains(t, rec.Body.String(), `message_stop`)
	require.NotContains(t, rec.Body.String(), `_sub2api_kiro_credits`)
}

// Some Responses-compatible providers report cumulative usage before completion,
// then omit usage or send a zero placeholder in the terminal response.
func TestMessagesResponsesProgressiveUsage(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, tc := range []struct {
		name, terminalUsage   string
		input, output, cached int
	}{
		{"missing", "", 31, 7, 5},
		{"zero", `,"usage":{"input_tokens":0,"output_tokens":0}`, 31, 7, 5},
		{"authoritative", `,"usage":{"input_tokens":19,"output_tokens":3}`, 19, 3, 0},
	} {
		for _, mode := range []string{"stream", "buffered"} {
			t.Run(tc.name+"/"+mode, func(t *testing.T) {
				rec := httptest.NewRecorder()
				c, _ := gin.CreateTestContext(rec)
				c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", nil)
				body := `data: {"type":"response.created","response":{"id":"resp_1"}}

data: {"type":"response.in_progress","usage":{"input_tokens":31,"input_tokens_details":{"cached_tokens":5},"output_tokens":7}}

data: {"type":"response.output_text.delta","delta":"ok"}

data: {"type":"response.completed","response":{"id":"resp_1","status":"completed"` + tc.terminalUsage + `}}` + "\n\n"
				resp := &http.Response{StatusCode: http.StatusOK, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(body))}
				svc := &OpenAIGatewayService{}
				handle := svc.handleAnthropicBufferedStreamingResponse
				if mode == "stream" {
					handle = svc.handleAnthropicStreamingResponse
				}
				result, err := handle(resp, c, &Account{ID: 1, Platform: PlatformOpenAI}, "model", "model", "model", time.Now())
				require.NoError(t, err)
				require.Equal(t, tc.input, result.Usage.InputTokens)
				require.Equal(t, tc.output, result.Usage.OutputTokens)
				require.Equal(t, tc.cached, result.Usage.CacheReadInputTokens)
				var wire struct {
					Usage *apicompat.AnthropicUsage `json:"usage"`
				}
				if mode == "stream" {
					_, delta, ok := strings.Cut(rec.Body.String(), "event: message_delta\ndata: ")
					require.True(t, ok)
					line, _, _ := strings.Cut(delta, "\n")
					require.NoError(t, json.Unmarshal([]byte(line), &wire))
				} else {
					require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &wire))
				}
				require.NotNil(t, wire.Usage)
				require.Equal(t, tc.input-tc.cached, wire.Usage.InputTokens)
				require.Equal(t, tc.cached, wire.Usage.CacheReadInputTokens)
				require.Equal(t, tc.output, wire.Usage.OutputTokens)
			})
		}
	}
}
