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
	"github.com/tidwall/gjson"
)

func TestExcelBPSRateLimitAfterEmptyEventsFailsOver(t *testing.T) {
	for _, event := range []string{
		`{"type":"response.output_item.added","output_index":0,"item":{"id":"rs_empty","type":"reasoning","summary":[]}}`,
		`{"type":"response.output_item.added","output_index":0,"item":{"id":"msg_empty","type":"message","role":"assistant","content":[]}}`,
		`{"type":"response.content_part.added","item_id":"msg_empty","content_index":0,"part":{"type":"output_text","text":"","annotations":[]}}`,
		`{"type":"response.reasoning_summary_part.added","item_id":"rs_empty","summary_index":0,"part":{"type":"summary_text","text":""}}`,
		`{"type":"response.output_text.delta","delta":""}`,
		`{"type":"response.reasoning_summary_text.delta","delta":""}`,
		`{"type":"response.output_item.done","item":{"id":"rs_empty","type":"reasoning","summary":[]}}`,
		`{"type":"response.created","response":{"id":"resp_empty","output":[{"id":"rs_empty","type":"reasoning","summary":[]}]}}`,
	} {
		for _, stream := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/stream=%t", event, stream), func(t *testing.T) {
				wire := "data: {\"type\":\"response.created\",\"response\":{\"id\":\"resp_empty\",\"output\":[]}}\n\n" + "data: " + event + "\n\n" +
					"event: error\ndata: {\"error\":{\"code\":\"rate_limit_exceeded\",\"headers\":{\"retry-after\":\"1\",\"retry-after-ms\":\"302\"},\"message\":\"Rate limit reached for test-model on tokens per min (TPM). Please try again in 302ms.\"}}\n\n"
				upstream := &httpUpstreamRecorder{resp: &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": {"text/event-stream"}}, Body: io.NopCloser(strings.NewReader(wire))}}
				svc := openAIClientToolsTestService(upstream)
				rec := httptest.NewRecorder()
				c, _ := gin.CreateTestContext(rec)
				c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
				_, err := svc.Forward(context.Background(), c, excelAccount(), []byte(fmt.Sprintf(`{"model":"gpt-6-sol","input":"test","stream":%t}`, stream)))
				var failover *UpstreamFailoverError
				require.ErrorAs(t, err, &failover)
				require.Equal(t, http.StatusTooManyRequests, failover.StatusCode)
				require.False(t, failover.RetryableOnSameAccount)
				require.Equal(t, "1", failover.ResponseHeaders.Get("Retry-After"))
				require.Empty(t, rec.Body.String())
				require.False(t, c.Writer.Written())
				require.False(t, IsResponseCommitted(c))
			})
		}
	}
}

func TestExcelBPSPreludeKeepsEffectiveOutputProtected(t *testing.T) {
	for _, payload := range []string{
		`{"type":"response.output_item.added","item":{"type":"reasoning","summary":[],"content":[{"type":"reasoning_text","text":"actual reasoning"}]}}`,

		`{"type":"response.output_text.delta","delta":"already sent"}`,
		`{"type":"response.reasoning_summary_text.delta","delta":"real reasoning"}`,
		`{"type":"response.output_item.added","item":{"type":"reasoning","encrypted_content":"opaque-reasoning","summary":[]}}`,
		`{"type":"response.output_item.added","item":{"type":"reasoning","summary":[{"type":"summary_text","text":"real reasoning"}]}}`,
		`{"type":"response.output_item.added","item":{"type":"message","content":[{"type":"refusal","refusal":"refusal text"}]}}`,
		`{"type":"response.content_part.added","part":{"type":"output_text","text":"already sent"}}`,
		`{"type":"response.content_part.done","part":{"type":"output_text","text":"already sent"}}`,
		`{"type":"response.output_item.done","item":{"type":"function_call","arguments":"{}"}}`,
		`{"type":"response.output_item.added","item":{"type":"custom_tool_call","input":"tool command"}}`,
		`{"type":"response.created","response":{"output":[{"type":"message","content":[{"type":"output_text","text":"already sent"}]}]}}`,
		`{"type":"response.completed","response":{"output":[]}}`,
		`{"type":"response.output_text.delta","delta":null}`,
		`{"type":"response.output_item.added","item":{"type":"unknown_kind"}}`,
		`{"type":"unknown_event"}`,
	} {
		t.Run(payload, func(t *testing.T) {
			kind := gjson.Get(payload, "type").String()
			require.False(t, excelBPSStreamEventIsPrelude([]byte(payload), kind))
		})
	}
}

func TestExcelBPSEmptyPreludeOverflowReturnsBoundedError(t *testing.T) {
	wire := "data: {\"type\":\"response.created\",\"response\":{\"output\":[],\"metadata\":\"" + strings.Repeat("x", openAIFirstOutputStageMaxBytes) + "\"}}\n\n"
	upstream := &httpUpstreamRecorder{resp: &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": {"text/event-stream"}}, Body: io.NopCloser(strings.NewReader(wire))}}
	svc := openAIClientToolsTestService(upstream)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
	_, err := svc.Forward(context.Background(), c, excelAccount(), []byte(`{"model":"gpt-6-sol","input":"test","stream":true}`))
	require.Error(t, err)
	require.Equal(t, http.StatusBadGateway, rec.Code)
	require.Contains(t, rec.Body.String(), "basispoints_prelude_limit")
	require.NotContains(t, rec.Body.String(), "response.created")
	require.Less(t, rec.Body.Len(), 1024)
}

func TestExcelBPSEmptyEventsPreservedWhenOutputSucceeds(t *testing.T) {
	wire := "data: {\"type\":\"response.created\",\"response\":{\"id\":\"resp_good\",\"output\":[]}}\n\n" +
		"data: {\"type\":\"response.output_item.added\",\"item\":{\"id\":\"msg_good\",\"type\":\"message\",\"content\":[]}}\n\n" +
		"data: {\"type\":\"response.content_part.added\",\"part\":{\"type\":\"output_text\",\"text\":\"\"}}\n\n" +
		"data: {\"type\":\"response.output_text.delta\",\"delta\":\"hello\"}\n\n" +
		"data: {\"type\":\"response.completed\",\"response\":{\"id\":\"resp_good\",\"status\":\"completed\",\"output\":[]}}\n\n"
	upstream := &httpUpstreamRecorder{resp: &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": {"text/event-stream"}}, Body: io.NopCloser(strings.NewReader(wire))}}
	svc := openAIClientToolsTestService(upstream)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
	result, err := svc.Forward(context.Background(), c, excelAccount(), []byte(`{"model":"gpt-6-sol","input":"test","stream":true}`))
	require.NoError(t, err)
	require.NotNil(t, result.FirstTokenMs)
	body := rec.Body.String()
	previous := -1
	for _, event := range []string{"response.created", "response.output_item.added", "response.content_part.added", "response.output_text.delta", "response.completed"} {
		needle := "event: " + event
		require.Equal(t, 1, strings.Count(body, needle))
		current := strings.Index(body, needle)
		require.Greater(t, current, previous)
		previous = current
	}
	require.Contains(t, body, "hello")
}
