//go:build unit

package service

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/pkg/requestretry"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestRequestAccountRetryStreamOutputBoundary(t *testing.T) {
	for _, tc := range []struct {
		data string
		want bool
	}{
		{`{"type":"response.output_text.delta","delta":""}`, false},
		{`{"type":"response.reasoning_summary_text.delta","delta":""}`, false},
		{`{"type":"response.reasoning_text.delta","delta":""}`, false},
		{`{"type":"response.function_call_arguments.delta","delta":""}`, false},
		{`{"type":"response.custom_tool_call_input.delta","delta":""}`, false},
		{`{"type":"response.refusal.delta","delta":""}`, false},
		{`{"type":"response.audio.delta","delta":""}`, false},
		{`{"type":"response.output_text.done","text":""}`, false},
		{`{"type":"response.reasoning_text.done","text":""}`, false},
		{`{"type":"response.reasoning_summary_text.done","text":""}`, false},
		{`{"type":"response.content_part.done","part":{"type":"output_text","text":""}}`, false},
		{`{"type":"response.reasoning_summary_part.done","part":{"type":"summary_text","text":""}}`, false},
		{`{"type":"response.output_item.done","item":{"type":"reasoning","summary":[]}}`, false},
		{`{"type":"response.output_item.done","item":{"type":"message","content":[]}}`, false},
		{`{"type":"response.output_text.delta","delta":"partial"}`, true},
		{`{"type":"response.output_text.delta","delta":" "}`, true},
		{`{"type":"response.reasoning_summary_text.delta","delta":"thinking"}`, true},
		{`{"type":"response.function_call_arguments.delta","delta":"{}"}`, true},
		{`{"type":"response.output_text.done","text":"partial"}`, true},
		{`{"type":"response.output_item.done","item":{"type":"reasoning","summary":[],"encrypted_content":"ciphertext"}}`, false},
		{`{"type":"response.output_item.added","item":{"type":"reasoning","encrypted_content":"ciphertext","content":[{"type":"reasoning_text","text":"thinking"}]}}`, true},
		{`{"type":"response.output_item.done","item":{"type":"compaction","encrypted_content":"ciphertext"}}`, true},
		{`{"type":"response.output_item.done","item":{"type":"reasoning","content":[{"type":"reasoning_text","text":"thinking"}]}}`, true},
		{`{"type":"response.output_item.done","item":{"type":"function_call","name":"execute","arguments":""}}`, true},
		{`{"type":"response.content_part.done","part":{"type":"audio","audio":"bytes"}}`, true},
		{`{"type":"response.output_text.delta"}`, true},
		{`{"type":"response.output_text.delta","delta":null}`, true},
		{`{"type":"response.output_item.done"}`, true},
		{`{"type":"unknown_event","delta":""}`, true},
		{`{"type":"response.output_text.delta",`, true},
	} {
		t.Run(tc.data, func(t *testing.T) {
			eventType := gjson.Get(tc.data, "type").String()
			require.Equal(t, tc.want, openAIStreamDataStartsClientOutput(tc.data, eventType))
		})
	}
}

func TestRequestAccountRetryWSStagesEmptyProgress(t *testing.T) {
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest("GET", "/v1/responses", nil)
	c.Request.Header.Set(requestretry.AccountHeader, "True")
	stage := &openAIWSFirstOutputStage{requestContext: c}
	require.Empty(t, stage.messages([]byte(`{"type":"response.created"}`)))
	require.Empty(t, stage.messages([]byte(`{"type":"response.output_text.delta","delta":""}`)))
	require.Empty(t, stage.messages([]byte(`{"type":"response.output_item.done","item":{"type":"reasoning","summary":[]}}`)))
	failure := []byte(`{"type":"response.failed","response":{"error":{"code":"server_error","message":"Our servers are currently overloaded. Please try again later."}}}`)
	require.Equal(t, [][]byte{failure}, stage.messages(failure))
	require.False(t, stage.committed)
	require.Empty(t, stage.pending)
}

func TestRequestAccountRetryAnthropicError(t *testing.T) {
	for _, retryExhausted := range []bool{false, true} {
		for _, header := range []string{"", "false", "True"} {
			c, _ := gin.CreateTestContext(httptest.NewRecorder())
			c.Request = httptest.NewRequest("POST", "/v1/messages", nil)
			c.Request.Header.Set(requestretry.AccountHeader, header)
			body := `{"error":{"type":"invalid_request_error","message":"provider cannot process the request"}}`
			resp := &http.Response{StatusCode: 400, Header: http.Header{"X-Request-Id": {"upstream-request"}}, Body: io.NopCloser(strings.NewReader(body))}
			svc := &GatewayService{}
			account := &Account{ID: 1, Platform: PlatformAnthropic, Type: AccountTypeAPIKey}
			var err error
			if retryExhausted {
				_, err = svc.handleRetryExhaustedError(context.Background(), resp, c, account)
			} else {
				_, err = svc.handleErrorResponse(context.Background(), resp, c, account)
			}
			var failover *UpstreamFailoverError
			if header == "True" {
				require.ErrorAs(t, err, &failover)
				require.Equal(t, 400, failover.StatusCode)
				require.Equal(t, "upstream-request", failover.ResponseHeaders.Get("X-Request-Id"))
				require.False(t, c.Writer.Written())
			} else {
				require.NotErrorAs(t, err, &failover)
				require.True(t, c.Writer.Written())
			}
		}
	}
}

func TestRequestAccountRetryGeminiError(t *testing.T) {
	for _, account := range []*Account{geminiPoolModeAPIKeyAccount(), geminiCustomCodesAPIKeyAccount()} {
		svc, _ := newGeminiSkippedWriteService(400, geminiSkippedTestUpstreamBody())
		c, rec := newGeminiNativeTestContext(t)
		c.Request.Header.Set(requestretry.AccountHeader, "True")
		_, err := svc.ForwardNative(context.Background(), c, account,
			"gemini-2.5-flash", "generateContent", false, []byte(`{"contents":[{"role":"user","parts":[{"text":"hi"}]}]}`))
		var failover *UpstreamFailoverError
		require.ErrorAs(t, err, &failover)
		require.Equal(t, 400, failover.StatusCode)
		require.Empty(t, rec.Body.String())
		require.False(t, c.Writer.Written())
	}
}

func TestRequestAccountRetryWSFirstOutput(t *testing.T) {
	for _, header := range []string{"", "True"} {
		c, _ := gin.CreateTestContext(httptest.NewRecorder())
		c.Request = httptest.NewRequest("GET", "/v1/responses", nil)
		c.Request.Header.Set(requestretry.AccountHeader, header)
		stage := &openAIWSFirstOutputStage{requestContext: c}
		metadata := []byte(`{"type":"response.created","response":{"id":"failed-attempt"}}`)
		failure := []byte(`{"type":"error","error":{"type":"invalid_request_error","message":"provider cannot process request"}}`)
		require.Empty(t, stage.messages(metadata))
		if header == "True" {
			require.True(t, openAIWSProcessingFailure(failure, c))
			require.Equal(t, [][]byte{failure}, stage.messages(failure))
			require.False(t, stage.committed)
		} else {
			require.False(t, openAIWSProcessingFailure(failure, c))
			require.Equal(t, [][]byte{metadata, failure}, stage.messages(failure))
			require.True(t, stage.committed)
		}
	}
}
