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

func TestOpenAIWebSocketRestartSSEBeforeOutput(t *testing.T) {
	for _, passthrough := range []bool{false, true} {
		for _, terminal := range []string{"error", "response.failed"} {
			for _, output := range []string{"", `{"type":"response.output_text.delta","delta":"generated"}`, `{"type":"response.output_item.done","item":{"type":"reasoning","encrypted_content":"fixture-cipher"}}`} {
				t.Run(fmt.Sprintf("passthrough=%t/%s/output=%t", passthrough, terminal, output != ""), func(t *testing.T) {
					rejection := `{"type":"error","error":{"type":"websocket_connection_error","code":1012,"message":"The upstream WebSocket closed before the response completed (code 1012)."}}`
					if terminal == "response.failed" {
						rejection = `{"type":"response.failed","response":{"error":{"type":"websocket_connection_error","code":"1012","message":"The upstream WebSocket closed before the response completed (code 1012)."}}}`
					}
					payload := "data: {\"type\":\"response.created\"}\n\n"
					if output != "" {
						payload += "data: " + output + "\n\n"
					}
					payload += "data: " + rejection + "\n\n"
					c, _ := gin.CreateTestContext(httptest.NewRecorder())
					c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
					svc := &OpenAIGatewayService{cfg: &config.Config{Gateway: config.GatewayConfig{MaxLineSize: defaultMaxLineSize}}}
					resp := &http.Response{StatusCode: 200, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(payload))}
					account := &Account{ID: 42, Platform: PlatformOpenAI, Type: AccountTypeAPIKey}
					var err error
					if passthrough {
						_, err = svc.handleStreamingResponsePassthrough(c.Request.Context(), resp, c, account, time.Now(), "model", "model")
					} else {
						_, err = svc.handleStreamingResponse(c.Request.Context(), resp, c, account, time.Now(), "model", "model")
					}
					var failure *UpstreamFailoverError
					if output == "" {
						require.ErrorAs(t, err, &failure)
						require.Equal(t, 502, failure.StatusCode)
						require.True(t, OpenAIDefaultFailureCanRetry(account, failure))
						require.False(t, c.Writer.Written())
					} else {
						require.Error(t, err)
						require.NotErrorAs(t, err, &failure)
						boundary := openAIStreamFirstOutputBoundary(c)
						require.Equal(t, strings.Contains(output, "encrypted_content"), boundary.HasEncryptedContent)
						require.Equal(t, !strings.Contains(output, "encrypted_content"), boundary.HasVisibleContent)
					}
				})
			}
		}
	}
}

func TestOpenAIWebSocketRestartRequiresTypedSignal(t *testing.T) {
	for _, payload := range []string{
		`{"type":"error","error":{"code":1012,"type":"invalid_request_error","message":"WebSocket closed"}}`,
		`{"type":"error","error":{"code":1008,"type":"websocket_connection_error","message":"WebSocket closed"}}`,
		`{"type":"error","error":{"message":"The upstream WebSocket closed before the response completed (code 1012)."}}`,
	} {
		require.False(t, openAIStreamErrorEventShouldFailover([]byte(payload), extractOpenAISSEErrorMessage([]byte(payload))))
	}
}
