package service

import (
	"context"
	"encoding/json"
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

func excelStructuredRequest(t *testing.T, model string, stream bool) []byte {
	t.Helper()
	body, err := json.Marshal(map[string]any{
		"model": model, "stream": stream, "input": "Return the answer.",
		"text": map[string]any{"format": map[string]any{
			"type": "json_schema", "name": "answer", "strict": true,
			"schema": map[string]any{"type": "object", "properties": map[string]any{"answer": map[string]any{"type": "integer"}}, "required": []string{"answer"}, "additionalProperties": false},
		}},
	})
	require.NoError(t, err)
	return body
}

func TestExcelBPSStructuredOutputForwardContract(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, model := range []string{"gpt-6-astra", "gpt-6-luna", "gpt-6-sol"} {
		for _, stream := range []bool{false, true} {
			for _, valid := range []bool{false, true} {
				t.Run(fmt.Sprintf("%s/stream=%t/valid=%t", model, stream, valid), func(t *testing.T) {
					answer := "UNVALIDATED_TEXT"
					if valid {
						answer = "{\"answer\":17}"
					}
					event, err := json.Marshal(map[string]any{"type": "response.completed", "response": map[string]any{
						"id": "resp_json", "status": "completed", "model": model,
						"output": []any{map[string]any{"type": "message", "id": "msg_json", "role": "assistant", "status": "completed", "content": []any{map[string]any{"type": "output_text", "text": answer, "annotations": []any{}}}}},
						"usage":  map[string]any{"input_tokens": 10, "output_tokens": 5},
					}})
					require.NoError(t, err)
					wire := "event: response.output_text.delta\ndata: {\"type\":\"response.output_text.delta\",\"delta\":\"UNVALIDATED_TEXT\"}\n\n" + "event: response.completed\ndata: " + string(event) + "\n\n"
					upstream := &httpUpstreamRecorder{resp: &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": {"text/event-stream"}}, Body: io.NopCloser(strings.NewReader(wire))}}
					svc := openAIClientToolsTestService(upstream)
					rec := httptest.NewRecorder()
					c, _ := gin.CreateTestContext(rec)
					c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
					account := excelAccount()
					result, err := svc.Forward(context.Background(), c, account, excelStructuredRequest(t, model, stream))
					require.Len(t, upstream.requests, 1)
					require.Equal(t, "bps.openai.com", upstream.lastReq.URL.Host)
					require.Equal(t, model, gjson.GetBytes(upstream.lastBody, "model").String())
					require.False(t, gjson.GetBytes(upstream.lastBody, "text").Exists())
					require.Contains(t, string(upstream.lastBody), "structured final answer")
					require.NotContains(t, rec.Body.String(), "UNVALIDATED_TEXT")
					require.True(t, account.Schedulable)
					require.NotNil(t, result)
					if valid {
						require.NoError(t, err)
						require.Equal(t, "response.completed", result.UpstreamTerminalEvent)
						require.Equal(t, 10, result.Usage.InputTokens)
						if stream {
							require.Contains(t, rec.Body.String(), "event: response.output_text.delta")
							require.Equal(t, 1, strings.Count(rec.Body.String(), "event: response.completed"))
						} else {
							require.Equal(t, 200, rec.Code)
							require.Equal(t, "json_schema", gjson.GetBytes(rec.Body.Bytes(), "text.format.type").String())
							require.JSONEq(t, answer, gjson.GetBytes(rec.Body.Bytes(), "output.0.content.0.text").String())
						}
					} else {
						require.Error(t, err)
						require.True(t, IsResponseCommitted(c))
						require.NotContains(t, rec.Body.String(), "event: response.completed")
						require.NotContains(t, rec.Body.String(), "event: response.output_text.delta")
						if stream {
							require.Equal(t, 1, strings.Count(rec.Body.String(), "event: response.failed"))
						} else {
							require.Equal(t, 502, rec.Code)
							require.True(t, json.Valid(rec.Body.Bytes()))
						}
					}
				})
			}
		}
	}
}

func TestExcelBPSStructuredOutputFallsBackOnModelAccess(t *testing.T) {
	for _, model := range []string{"gpt-6-luna", "gpt-6-sol"} {
		t.Run(model, func(t *testing.T) {
			ordinaryWire := fmt.Sprintf("event: response.completed\ndata: {\"type\":\"response.completed\",\"response\":{\"id\":\"ordinary-%s\",\"status\":\"completed\",\"model\":\"%s\",\"output\":[],\"usage\":{\"input_tokens\":2,\"output_tokens\":1}}}\n\n", model, model)
			upstream := &httpUpstreamRecorder{responses: []*http.Response{
				{StatusCode: http.StatusForbidden, Header: http.Header{}, Body: io.NopCloser(strings.NewReader("{\"error\":{\"code\":\"basispoints_model_access_changed\"}}"))},
				{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": {"text/event-stream"}}, Body: io.NopCloser(strings.NewReader(ordinaryWire))},
			}}
			svc := openAIClientToolsTestService(upstream)
			rec := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(rec)
			c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
			account := excelAccount()
			result, err := svc.Forward(context.Background(), c, account, excelStructuredRequest(t, model, true))
			require.NoError(t, err)
			require.NotNil(t, result)
			require.Len(t, upstream.requests, 2)
			require.Equal(t, "/basispoints/api/responses", upstream.requests[0].URL.Path)
			require.Equal(t, chatgptCodexURL, upstream.requests[1].URL.String())
			require.Equal(t, model, gjson.GetBytes(upstream.lastBody, "model").String())
			require.Equal(t, "json_schema", gjson.GetBytes(upstream.lastBody, "text.format.type").String())
			require.Contains(t, rec.Body.String(), "ordinary-")
			require.NotContains(t, rec.Body.String(), "basispoints_model_access_changed")
			require.True(t, account.Schedulable)
		})
	}
}
