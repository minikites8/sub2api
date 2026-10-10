package service

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

const overloadFixture = "Our servers are currently overloaded. Please try again later."

func TestOpenAIOverloadHTTPAutoFailover(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, format := range []string{"error", "response_error", "message", "plain_text"} {
		for _, accountType := range []string{AccountTypeOAuth, AccountTypeAPIKey} {
			for _, passthrough := range []bool{false, true} {
				for _, stream := range []bool{false, true} {
					for _, status := range []int{400, 422, 500, 502, 503} {
						t.Run(fmt.Sprintf("%s/%s/passthrough_%t/stream_%t/%d", format, accountType, passthrough, stream, status), func(t *testing.T) {
							body := []byte(fmt.Sprintf(`{"model":"gpt-5.2","instructions":"test","input":"hello","stream":%t}`, stream))
							rec := httptest.NewRecorder()
							c, _ := gin.CreateTestContext(rec)
							c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", bytes.NewReader(body))
							c.Request.Header.Set("User-Agent", "codex_cli_rs/0.1.0")
							account := &Account{ID: 1, Platform: PlatformOpenAI, Type: accountType, Concurrency: 1, Credentials: map[string]any{"access_token": "oauth-test", "api_key": "sk-test"}, Extra: map[string]any{"openai_passthrough": passthrough}}
							responseBody := fmt.Sprintf(`{"error":{"type":"invalid_request_error","message":%q}}`, overloadFixture)
							switch format {
							case "response_error":
								responseBody = fmt.Sprintf(`{"response":{"error":{"type":"server_error","message":%q}}}`, overloadFixture)
							case "message":
								responseBody = fmt.Sprintf(`{"message":%q}`, overloadFixture)
							case "plain_text":
								responseBody = overloadFixture
							}
							svc := &OpenAIGatewayService{cfg: &config.Config{}, httpUpstream: &httpUpstreamRecorder{resp: &http.Response{StatusCode: status, Header: http.Header{"Content-Type": {"application/json"}}, Body: io.NopCloser(strings.NewReader(responseBody))}}}
							_, err := svc.Forward(context.Background(), c, account, body)
							var failover *UpstreamFailoverError
							require.ErrorAs(t, err, &failover)
							require.True(t, failover.ShouldRetryNextAccount())
							require.Contains(t, string(failover.ResponseBody), overloadFixture)
							require.False(t, c.Writer.Written())
							require.Empty(t, rec.Body.String())
							t.Logf("processing status=%d passthrough=%t failover=true output_bytes=%d", status, passthrough, rec.Body.Len())
						})
					}
				}
			}
		}
	}
}

func TestOpenAIOverloadClassificationSafety(t *testing.T) {
	account := &Account{Platform: PlatformOpenAI, Type: AccountTypeOAuth}
	for _, tc := range []struct {
		name   string
		status int
		body   string
		want   bool
	}{
		{"http200", 200, fmt.Sprintf(`{"error":{"message":%q}}`, overloadFixture), false},
		{"invalid_parameter", 400, `{"error":{"type":"invalid_request_error","message":"Unknown parameter"}}`, false},
		{"echoed_input", 400, fmt.Sprintf(`{"error":{"message":"Invalid input"},"input":{"message":%q}}`, overloadFixture), false},
		{"context_limit", 400, fmt.Sprintf(`{"error":{"code":"context_length_exceeded","message":%q}}`, overloadFixture), false},
		{"cyber_policy", 400, fmt.Sprintf(`{"error":{"code":"cyber_policy","message":%q}}`, overloadFixture), false},
		{"nested_error", 400, fmt.Sprintf(`{"response":{"error":{"message":%q}}}`, overloadFixture), true},
		{"plain_text", 500, "**" + overloadFixture + "**", true},
		{"top_level_400", 400, fmt.Sprintf(`{"message":%q}`, overloadFixture), true},
		{"top_level_422", 422, fmt.Sprintf(`{"message":%q}`, overloadFixture), true},
		{"case_insensitive", 400, fmt.Sprintf(`{"error":{"message":%q}}`, strings.ToUpper(overloadFixture)), true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			require.Equal(t, tc.want, shouldFailoverOpenAIPassthroughResponse(account, tc.status, []byte(tc.body)))
		})
	}
}
