package service

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/pkg/openai"
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
			account := excelAccount()
			account.Credentials["client_id"] = openai.ExcelClientID
			original := c.Writer
			result, err := svc.ForwardAsChatCompletions(context.Background(), c, account, body, "", "")
			require.Same(t, original, c.Writer)
			require.True(t, account.IsExcelOAuth())
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

// A retryable account rejection must leave the real writer untouched. The
// handler can then select another account using the same client request.
func TestExcelBPSChatCompletionsRejectedAccountPreservesReplay(t *testing.T) {
	for _, stream := range []bool{false, true} {
		t.Run(fmt.Sprint(stream), func(t *testing.T) {
			upstream := &httpUpstreamRecorder{resp: &http.Response{StatusCode: http.StatusForbidden, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(`{"error":{"message":"Account credential not found"}}`))}}
			svc := openAIClientToolsTestService(upstream)
			account := excelAccount()
			account.Credentials["client_id"] = openai.ExcelClientID
			body := []byte(fmt.Sprintf(`{"model":"gpt-6-astra","stream":%t,"messages":[{"role":"user","content":"hello"}]}`, stream))
			rec := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(rec)
			c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(string(body)))
			original := c.Writer
			result, err := svc.ForwardAsChatCompletions(context.Background(), c, account, body, "", "")
			var failover *UpstreamFailoverError
			require.ErrorAs(t, err, &failover)
			require.Nil(t, result)
			require.Same(t, original, c.Writer)
			require.False(t, c.Writer.Written())
			require.Empty(t, rec.Body.String())
			require.Equal(t, "/basispoints/api/responses", upstream.lastReq.URL.Path)
		})
	}
}

func TestExcelBPSChatWriterControlFramesDoNotCommit(t *testing.T) {
	for _, stream := range []bool{false, true} {
		t.Run(fmt.Sprint(stream), func(t *testing.T) {
			rec := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(rec)
			writer := newExcelBPSChatWriter(c.Writer, "gpt-6-astra", stream)
			_, err := writer.WriteString(": keepalive\n\ndata: {\"type\":\"response.created\",\"response\":{\"id\":\"resp_control\"}}\n\ndata: {\"type\":\"response.in_progress\"}\n\n")
			require.NoError(t, err)
			require.NoError(t, FlushGatewayResponse(writer))
			require.False(t, c.Writer.Written())
			require.Empty(t, rec.Body.String())
			_, err = writer.WriteString("data: {\"type\":\"response.output_text.delta\",\"delta\":\"hello\"}\n\n")
			require.NoError(t, err)
			require.Equal(t, stream, c.Writer.Written())
			if stream {
				require.Contains(t, rec.Body.String(), "hello")
			}
		})
	}
}

func TestManagedExcelBPSChatCompletionsRoutesThroughBridge(t *testing.T) {
	for _, stream := range []bool{false, true} {
		t.Run(fmt.Sprint(stream), func(t *testing.T) {
			reauth, _, connection := managedBPSConnectionFailureFixture(t)
			wire := "data: {\"type\":\"response.completed\",\"response\":{\"id\":\"resp_managed_chat\",\"status\":\"completed\",\"model\":\"gpt-6-astra\",\"output\":[{\"type\":\"message\",\"role\":\"assistant\",\"content\":[{\"type\":\"output_text\",\"text\":\"managed hello\"}]}],\"usage\":{\"input_tokens\":10,\"output_tokens\":2}}}\n\n"
			upstream := &httpUpstreamRecorder{resp: &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": {"text/event-stream"}}, Body: io.NopCloser(strings.NewReader(wire))}}
			gateway := openAIClientToolsTestService(upstream)
			gateway.excelOAuthReauth = reauth
			parent := managedBPSBillingParent(nil)
			parent.Extra["openai_auth_connections"] = []*OpenAIAuthConnection{connection}
			body := []byte(fmt.Sprintf(`{"model":"gpt-6-astra","stream":%t,"messages":[{"role":"user","content":"hello"}]}`, stream))
			rec := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(rec)
			c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(string(body)))
			result, err := gateway.ForwardAsChatCompletions(context.Background(), c, parent, body, "", "")
			require.NoError(t, err)
			require.NotNil(t, result)
			require.Equal(t, connection.ID, result.AuthConnectionID)
			require.Equal(t, "/basispoints/api/responses", upstream.lastReq.URL.Path)
			require.Equal(t, "Bearer synthetic-connection-token", upstream.lastReq.Header.Get("Authorization"))
			require.Contains(t, rec.Body.String(), "managed hello")
			if stream {
				require.Contains(t, rec.Body.String(), "chat.completion.chunk")
			} else {
				require.Equal(t, "chat.completion", gjson.Get(rec.Body.String(), "object").String())
			}
		})
	}
}

func TestExcelBPSChatCompletionsGlobalDisableDoesNotBypass(t *testing.T) {
	for _, managed := range []bool{false, true} {
		t.Run(fmt.Sprint(managed), func(t *testing.T) {
			upstream := &httpUpstreamRecorder{}
			gateway := openAIClientToolsTestService(upstream)
			gateway.settingService = NewSettingService(&excelBPSImageSettingsRepo{values: map[string]string{SettingKeyExcelBPSEnabled: "false"}}, gateway.cfg)
			account := excelAccount()
			account.Credentials["client_id"] = openai.ExcelClientID
			if managed {
				reauth, _, connection := managedBPSConnectionFailureFixture(t)
				gateway.excelOAuthReauth = reauth
				account = managedBPSBillingParent(nil)
				account.Extra["openai_auth_connections"] = []*OpenAIAuthConnection{connection}
			}
			body := []byte(`{"model":"gpt-6-astra","messages":[{"role":"user","content":"hello"}]}`)
			c, _ := gin.CreateTestContext(httptest.NewRecorder())
			c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(string(body)))
			_, err := gateway.ForwardAsChatCompletions(context.Background(), c, account, body, "", "")
			require.Error(t, err)
			require.Empty(t, upstream.requests, "global BPS off must not send via BPS or ordinary OAuth")
		})
	}
}
