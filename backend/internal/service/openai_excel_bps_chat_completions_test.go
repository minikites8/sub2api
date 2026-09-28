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

func TestExcelBPSChatCompletionsRoutesThroughBridge(t *testing.T) {
	for _, stream := range []bool{false, true} {
		t.Run(fmt.Sprint(stream), func(t *testing.T) {
			wire := "data: {\"type\":\"response.created\",\"response\":{\"id\":\"resp_chat\",\"output\":[]}}\n\n" +
				"data: {\"type\":\"response.output_text.delta\",\"output_index\":0,\"delta\":\"hello\"}\n\n" +
				"data: {\"type\":\"response.completed\",\"response\":{\"id\":\"resp_chat\",\"status\":\"completed\",\"model\":\"gpt-6-astra\",\"output\":[{\"type\":\"message\",\"role\":\"assistant\",\"content\":[{\"type\":\"output_text\",\"text\":\"hello\"}]}],\"usage\":{\"input_tokens\":10,\"output_tokens\":2}}}\n\n"
			upstream := &httpUpstreamRecorder{resp: &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": {"text/event-stream"}}, Body: io.NopCloser(strings.NewReader(wire))}}
			svc := openAIClientToolsTestService(upstream)
			body := []byte(fmt.Sprintf(`{"model":"gpt-6-astra","stream":%t,"messages":[{"role":"user","content":"hello"}]}`, stream))
			rec := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(rec)
			c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(string(body)))
			result, err := svc.ForwardAsChatCompletions(context.Background(), c, excelAccount(), body, "", "")
			require.NoError(t, err)
			require.Equal(t, "/basispoints/api/responses", upstream.lastReq.URL.Path)
			require.Equal(t, "bps.openai.com", upstream.lastReq.URL.Host)
			require.NotNil(t, result)
			require.Equal(t, 10, result.Usage.InputTokens)
			require.Equal(t, 2, result.Usage.OutputTokens)
			require.Contains(t, rec.Body.String(), "hello")
			require.NotContains(t, rec.Body.String(), "event: response.")
			if stream {
				require.Contains(t, rec.Body.String(), "chat.completion.chunk")
				require.Contains(t, rec.Body.String(), "data: [DONE]")
			} else {
				require.Equal(t, "chat.completion", gjson.Get(rec.Body.String(), "object").String())
				require.Equal(t, "hello", gjson.Get(rec.Body.String(), "choices.0.message.content").String())
			}
		})
	}
}

func runExcelBPSChatCase(t *testing.T, ctx context.Context, account *Account, body, wire string) (*httptest.ResponseRecorder, *OpenAIForwardResult, *httpUpstreamRecorder, error) {
	t.Helper()
	upstream := &httpUpstreamRecorder{resp: excelBPSSSEResponse(wire)}
	svc := openAIClientToolsTestService(upstream)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(body)).WithContext(ctx)
	original := c.Writer
	result, err := svc.ForwardAsChatCompletions(ctx, c, account, []byte(body), "", "")
	require.Same(t, original, c.Writer, "restore actual writer before handler retry")
	return rec, result, upstream, err
}

func excelBPSChatSuccessWire() string {
	return "data: {\"type\":\"response.output_text.delta\",\"delta\":\"hello\"}\n\n" +
		"data: {\"type\":\"response.completed\",\"response\":{\"id\":\"resp_chat\",\"status\":\"completed\",\"model\":\"gpt-6-astra\",\"output\":[],\"usage\":{\"input_tokens\":10,\"output_tokens\":2}}}\n\n"
}

func TestExcelBPSChatCompletionsGroupPolicy(t *testing.T) {
	for _, groupEnabled := range []bool{false, true} {
		for _, accountEnabled := range []bool{false, true} {
			t.Run(fmt.Sprintf("group=%t/account=%t", groupEnabled, accountEnabled), func(t *testing.T) {
				account := excelAccount()
				account.Extra["openai_excel_bps"] = accountEnabled
				ctx := groupPolicyContext(GroupSchedulingBalanced, groupEnabled)
				body := "{\"model\":\"gpt-6-astra\",\"messages\":[{\"role\":\"user\",\"content\":\"hello\"}]}"
				rec, result, upstream, err := runExcelBPSChatCase(t, ctx, account, body, excelBPSChatSuccessWire())
				require.NoError(t, err)
				require.NotNil(t, result)
				path := "/backend-api/codex/responses"
				if groupEnabled && accountEnabled {
					path = "/basispoints/api/responses"
				}
				require.Equal(t, path, upstream.lastReq.URL.Path)
				require.Contains(t, rec.Body.String(), "hello")
				require.Equal(t, accountEnabled, account.IsExcelBPSEnabled(), "group policy stays request-local")
			})
		}
	}
}

func TestExcelBPSChatCompletionsCanonicalInput(t *testing.T) {
	for _, stream := range []bool{false, true} {
		for _, responsesShape := range []bool{false, true} {
			t.Run(fmt.Sprintf("stream=%t/responsesShape=%t", stream, responsesShape), func(t *testing.T) {
				account := excelAccount()
				account.Credentials["model_mapping"] = map[string]any{"legacy-alias": "gpt-6-astra", "gpt-6-astra": "must-not-map-twice"}
				body := fmt.Sprintf("{\"model\":\"legacy-alias\",\"stream\":%t,\"messages\":[{\"role\":\"system\",\"content\":\"system policy\"},{\"role\":\"user\",\"content\":\"user hello\"}]}", stream)
				if responsesShape {
					body = fmt.Sprintf("{\"model\":\"legacy-alias\",\"stream\":%t,\"input\":\"user hello\"}", stream)
				}
				rec, result, upstream, err := runExcelBPSChatCase(t, context.Background(), account, body, excelBPSChatSuccessWire())
				require.NoError(t, err)
				require.Equal(t, "/basispoints/api/responses", upstream.lastReq.URL.Path)
				require.Equal(t, "gpt-6-astra", gjson.GetBytes(upstream.lastBody, "model").String())
				require.Contains(t, string(upstream.lastBody), "user hello")
				if !responsesShape {
					require.Contains(t, string(upstream.lastBody), "system policy")
				}
				require.Equal(t, "legacy-alias", result.Model)
				require.Equal(t, "gpt-6-astra", result.BillingModel)
				require.Equal(t, stream, result.Stream)
				require.Contains(t, rec.Body.String(), "legacy-alias")
				require.Contains(t, rec.Body.String(), "hello")
			})
		}
	}
}

func TestExcelBPSChatCompletionsModelSelector(t *testing.T) {
	account := excelAccount()
	account.Extra["openai_excel_bps_models"] = []any{"gpt-6-sol"}
	_, _, upstream, err := runExcelBPSChatCase(t, context.Background(), account, "{\"model\":\"gpt-6-astra\",\"messages\":[{\"role\":\"user\",\"content\":\"hello\"}]}", excelBPSChatSuccessWire())
	require.NoError(t, err)
	require.Equal(t, "/backend-api/codex/responses", upstream.lastReq.URL.Path)
}

type excelBPSChatSequenceUpstream struct {
	HTTPUpstream
	responses []*http.Response
	paths     []string
}

func (u *excelBPSChatSequenceUpstream) Do(req *http.Request, _ string, _ int64, _ int) (*http.Response, error) {
	u.paths = append(u.paths, req.URL.Path)
	if len(u.responses) == 0 {
		return nil, fmt.Errorf("unexpected upstream attempt")
	}
	response := u.responses[0]
	u.responses = u.responses[1:]
	return response, nil
}

func TestExcelBPSChatCompletionsNativeFallback(t *testing.T) {
	for _, stream := range []bool{false, true} {
		t.Run(fmt.Sprint(stream), func(t *testing.T) {
			upstream := &excelBPSChatSequenceUpstream{responses: []*http.Response{
				excelBPSJSONResponse(400, "{\"error\":{\"code\":\"model_not_found\",\"message\":\"The requested model is not available\"}}"),
				excelBPSSSEResponse(excelBPSChatSuccessWire()),
			}}
			svc := openAIClientToolsTestService(&httpUpstreamRecorder{})
			svc.httpUpstream = upstream
			rec := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(rec)
			body := fmt.Sprintf("{\"model\":\"gpt-6-astra\",\"stream\":%t,\"messages\":[{\"role\":\"user\",\"content\":\"hello\"}]}", stream)
			c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(body))
			result, err := svc.ForwardAsChatCompletions(context.Background(), c, excelAccount(), []byte(body), "", "")
			require.NoError(t, err)
			require.NotNil(t, result)
			require.Equal(t, []string{"/basispoints/api/responses", "/backend-api/codex/responses"}, upstream.paths)
			require.Contains(t, rec.Body.String(), "hello")
			require.Contains(t, rec.Body.String(), "chat.completion")
			require.NotContains(t, rec.Body.String(), "event: response.")
		})
	}
}

func TestExcelBPSChatWriterContentAndFraming(t *testing.T) {
	for _, stream := range []bool{false, true} {
		for _, terminalOnly := range []bool{false, true} {
			for _, fragmented := range []bool{false, true} {
				t.Run(fmt.Sprintf("stream=%t/terminalOnly=%t/fragmented=%t", stream, terminalOnly, fragmented), func(t *testing.T) {
					wire := excelBPSChatSuccessWire()
					if terminalOnly {
						wire = "data: {\"type\":\"response.completed\",\"response\":{\"id\":\"terminal\",\"status\":\"completed\",\"output\":[{\"type\":\"message\",\"content\":[{\"type\":\"output_text\",\"text\":\"hello\"}]}],\"usage\":{\"input_tokens\":10,\"output_tokens\":2}}}\n\n"
					}
					rec := httptest.NewRecorder()
					c, _ := gin.CreateTestContext(rec)
					w := newExcelBPSChatWriter(c.Writer, "client-model", stream)
					_, err := w.WriteString(": keepalive\n\n")
					require.NoError(t, err)
					w.Flush()
					if !stream {
						require.False(t, c.Writer.Written())
						require.Empty(t, rec.Body.String())
					}
					if fragmented {
						for _, b := range []byte(wire) {
							_, err = w.Write([]byte{b})
							require.NoError(t, err)
						}
					} else {
						_, err = w.WriteString(wire)
						require.NoError(t, err)
					}
					require.Contains(t, rec.Body.String(), "hello")
					require.NotContains(t, rec.Body.String(), "event: response.")
					if stream {
						require.Equal(t, 1, strings.Count(rec.Body.String(), "data: [DONE]"))
					} else {
						require.Equal(t, "application/json", rec.Header().Get("Content-Type"))
						require.Equal(t, "hello", gjson.Get(rec.Body.String(), "choices.0.message.content").String())
						require.Equal(t, int64(10), gjson.Get(rec.Body.String(), "usage.prompt_tokens").Int())
					}
				})
			}
		}
	}
}

func TestExcelBPSChatWriterToolCompletion(t *testing.T) {
	for _, stream := range []bool{false, true} {
		t.Run(fmt.Sprint(stream), func(t *testing.T) {
			wire := "data: {\"type\":\"response.completed\",\"response\":{\"id\":\"tool\",\"status\":\"completed\",\"output\":[{\"type\":\"function_call\",\"call_id\":\"call_tool\",\"name\":\"weather\",\"arguments\":\"{\\\"city\\\":\\\"Paris\\\"}\"}],\"usage\":{\"input_tokens\":10,\"output_tokens\":2}}}\n\n"
			rec := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(rec)
			w := newExcelBPSChatWriter(c.Writer, "client-model", stream)
			_, err := w.WriteString(wire)
			require.NoError(t, err)
			require.Contains(t, rec.Body.String(), "weather")
			require.Contains(t, rec.Body.String(), "Paris")
			require.Contains(t, rec.Body.String(), "tool_calls")
		})
	}
}

func TestExcelBPSChatWriterLateError(t *testing.T) {
	for _, stream := range []bool{false, true} {
		t.Run(fmt.Sprint(stream), func(t *testing.T) {
			rec := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(rec)
			w := newExcelBPSChatWriter(c.Writer, "client-model", stream)
			_, err := w.WriteString("data: {\"type\":\"response.output_text.delta\",\"delta\":\"partial\"}\n\ndata: {\"type\":\"error\",\"error\":{\"code\":\"rate_limit_exceeded\",\"message\":\"Rate limit reached\"}}\n\n")
			require.NoError(t, err)
			require.Contains(t, rec.Body.String(), "rate_limit_exceeded")
			require.NotContains(t, rec.Body.String(), "event:")
			if !stream {
				require.Equal(t, 502, rec.Code)
				require.True(t, gjson.Valid(rec.Body.String()))
				require.NotContains(t, rec.Body.String(), "partial")
			}
		})
	}
}

func TestExcelBPSChatWriterNamedCRLFAndJSON(t *testing.T) {
	response := "{\"id\":\"terminal\",\"object\":\"response\",\"status\":\"completed\",\"output\":[{\"type\":\"message\",\"content\":[{\"type\":\"output_text\",\"text\":\"hello\"}]}],\"usage\":{\"input_tokens\":10,\"output_tokens\":2}}"
	for _, stream := range []bool{false, true} {
		for _, jsonBody := range []bool{false, true} {
			t.Run(fmt.Sprintf("stream=%t/json=%t", stream, jsonBody), func(t *testing.T) {
				rec := httptest.NewRecorder()
				c, _ := gin.CreateTestContext(rec)
				writer := newExcelBPSChatWriter(c.Writer, "client-model", stream)
				wire := "event: response.completed\r\ndata: {\"response\":" + response + "}\r\n\r\n"
				if jsonBody {
					wire = response
				}
				_, err := writer.WriteString(wire)
				require.NoError(t, err)
				require.Contains(t, rec.Body.String(), "hello")
				require.Contains(t, rec.Body.String(), "chat.completion")
				if stream {
					require.Equal(t, "text/event-stream", rec.Header().Get("Content-Type"))
				} else {
					require.True(t, gjson.Valid(rec.Body.String()))
				}
			})
		}
	}
}

func TestExcelBPSChatWriterJSONErrorAfterHeartbeat(t *testing.T) {
	for _, stream := range []bool{false, true} {
		for _, heartbeat := range []bool{false, true} {
			t.Run(fmt.Sprintf("stream=%t/heartbeat=%t", stream, heartbeat), func(t *testing.T) {
				rec := httptest.NewRecorder()
				c, _ := gin.CreateTestContext(rec)
				writer := newExcelBPSChatWriter(c.Writer, "client-model", stream)
				if heartbeat {
					_, err := writer.WriteString(": keepalive\n\n")
					require.NoError(t, err)
					writer.Flush()
				}
				writer.WriteHeader(http.StatusBadGateway)
				_, err := writer.WriteString("{\"error\":{\"code\":\"basispoints_usage_invalid\",\"message\":\"Upstream error\"}}")
				require.NoError(t, err)
				require.Contains(t, rec.Body.String(), "basispoints_usage_invalid")
				if stream && heartbeat {
					require.Contains(t, rec.Body.String(), "data: {\"error\"")
					require.Equal(t, 1, strings.Count(rec.Body.String(), "data: [DONE]"))
				} else {
					require.Equal(t, http.StatusBadGateway, rec.Code)
					require.True(t, gjson.Valid(rec.Body.String()))
				}
			})
		}
	}
}
