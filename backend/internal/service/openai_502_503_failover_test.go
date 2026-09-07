package service

import (
	"bytes"
	"context"
	"errors"
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

func TestOpenAI502503HTTPFailover(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, accountType := range []string{AccountTypeOAuth, AccountTypeAPIKey} {
		for _, passthrough := range []bool{false, true} {
			for _, stream := range []bool{false, true} {
				for _, status := range []int{502, 503} {
					t.Run(fmt.Sprintf("%s/passthrough_%t/stream_%t/%d", accountType, passthrough, stream, status), func(t *testing.T) {
						body := []byte(fmt.Sprintf(`{"model":"gpt-5.2","instructions":"test","input":"hello","stream":%t}`, stream))
						rec := httptest.NewRecorder()
						c, _ := gin.CreateTestContext(rec)
						c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", bytes.NewReader(body))
						c.Request.Header.Set("User-Agent", "codex_cli_rs/0.1.0")
						account := &Account{ID: 1, Platform: PlatformOpenAI, Type: accountType, Concurrency: 1,
							Credentials: map[string]any{"access_token": "oauth-test", "api_key": "sk-test"},
							Extra:       map[string]any{"openai_passthrough": passthrough}}
						svc := &OpenAIGatewayService{cfg: &config.Config{}, httpUpstream: &httpUpstreamRecorder{resp: &http.Response{
							StatusCode: status, Header: http.Header{"Content-Type": {"application/json"}},
							Body: io.NopCloser(strings.NewReader(`{"error":{"type":"server_error","message":"Upstream request failed"}}`)),
						}}}
						_, err := svc.Forward(context.Background(), c, account, body)
						var failover *UpstreamFailoverError
						require.ErrorAs(t, err, &failover)
						require.Equal(t, status, failover.StatusCode)
						require.True(t, failover.ShouldRetryNextAccount())
						require.False(t, c.Writer.Written())
						require.Empty(t, rec.Body.String())
						if accountType == AccountTypeOAuth {
							require.True(t, failover.RetryableOnSameAccount)
							require.Equal(t, 2, failover.SameAccountRetryMax)
						}
					})
				}
			}
		}
	}
}

func TestOpenAI502503StreamFailover(t *testing.T) {
	gin.SetMode(gin.TestMode)
	payloads := []struct {
		name, payload string
		status        int
	}{
		{"status_502", `{"type":"error","error":{"status_code":502,"message":"Upstream request failed"}}`, 502},
		{"status_503", `{"type":"error","error":{"status_code":503,"message":"Upstream request failed"}}`, 503},
		{"server_error", `{"type":"error","error":{"type":"server_error","message":"Upstream request failed"}}`, 502},
		{"upstream_error", `{"type":"error","error":{"code":"upstream_error","message":"Upstream request failed"}}`, 502},
		{"unavailable", `{"type":"error","error":{"type":"service_unavailable_error","message":"Service unavailable"}}`, 503},
		{"processing", `{"type":"error","error":{"type":"server_error","message":"An error occurred while processing your request."}}`, 502},
		{"overload", `{"type":"error","error":{"type":"service_unavailable_error","message":"Our servers are currently overloaded. Please try again later."}}`, 503},
		{"failed_503", `{"type":"response.failed","response":{"error":{"status_code":503,"message":"Service unavailable"}}}`, 503},
	}
	readers := []struct {
		name string
		run  func(*OpenAIGatewayService, *gin.Context, *Account, *http.Response, []byte) error
	}{
		{"responses_stream", func(s *OpenAIGatewayService, c *gin.Context, a *Account, r *http.Response, _ []byte) error {
			_, err := s.handleStreamingResponse(c.Request.Context(), r, c, a, time.Now(), "model", "model")
			return err
		}},
		{"passthrough_stream", func(s *OpenAIGatewayService, c *gin.Context, a *Account, r *http.Response, _ []byte) error {
			_, err := s.handleStreamingResponsePassthrough(c.Request.Context(), r, c, a, time.Now(), "model", "model")
			return err
		}},
		{"responses_buffered", func(s *OpenAIGatewayService, c *gin.Context, a *Account, r *http.Response, body []byte) error {
			_, err := s.handleSSEToJSON(r, c, a, body, "model", "model")
			return err
		}},
		{"passthrough_buffered", func(s *OpenAIGatewayService, c *gin.Context, a *Account, r *http.Response, body []byte) error {
			_, err := s.handlePassthroughSSEToJSON(r, c, a, body, "model", "model")
			return err
		}},
		{"messages", func(s *OpenAIGatewayService, c *gin.Context, a *Account, r *http.Response, _ []byte) error {
			_, err := s.handleAnthropicStreamingResponse(r, c, a, "model", "model", "model", time.Now())
			return err
		}},
		{"chat", func(s *OpenAIGatewayService, c *gin.Context, a *Account, r *http.Response, _ []byte) error {
			_, err := s.handleChatStreamingResponse(r, c, a, "model", "model", "model", time.Now(), 128)
			return err
		}},
	}
	for _, p := range payloads {
		for _, reader := range readers {
			t.Run(p.name+"/"+reader.name, func(t *testing.T) {
				body := []byte("event: response.created\ndata: {\"type\":\"response.created\",\"response\":{\"id\":\"resp_test\"}}\n\ndata: " + p.payload + "\n\n")
				c, rec := newNonStreamingFailoverContext(t)
				a := &Account{ID: 1, Platform: PlatformOpenAI, Type: AccountTypeOAuth}
				r := newNonStreamingSSEResponse()
				r.Body = io.NopCloser(bytes.NewReader(body))
				err := reader.run(newNonStreamingFailoverService(), c, a, r, body)
				var failover *UpstreamFailoverError
				require.ErrorAs(t, err, &failover)
				require.Equal(t, p.status, failover.StatusCode)
				require.True(t, failover.ShouldRetryNextAccount())
				require.True(t, failover.RetryableOnSameAccount)
				require.False(t, c.Writer.Written())
				require.Empty(t, rec.Body.String())
			})
		}
	}
}

func TestOpenAI502503StreamRequestErrorsRemainTerminal(t *testing.T) {
	for _, payload := range []string{
		`{"type":"error","error":{"status_code":503,"type":"invalid_request_error","message":"Unknown parameter"}}`,
		`{"type":"error","error":{"status_code":502,"code":"context_length_exceeded","message":"Maximum context length exceeded"}}`,
		`{"type":"error","error":{"status_code":503,"code":"content_policy_violation","message":"Blocked by content policy"}}`,
		`{"type":"error","error":{"message":"Unknown parameter"},"input":{"status_code":503,"code":"server_error"}}`,
	} {
		require.False(t, openAIStreamErrorEventShouldFailover([]byte(payload), extractOpenAISSEErrorMessage([]byte(payload))), payload)
	}
}

func TestOpenAI502503StreamAfterOutputRemainsTerminal(t *testing.T) {
	for _, passthrough := range []bool{false, true} {
		c, rec := newNonStreamingFailoverContext(t)
		body := "data: {\"type\":\"response.output_text.delta\",\"delta\":\"hello\"}\n\ndata: {\"type\":\"error\",\"error\":{\"type\":\"server_error\",\"message\":\"Upstream request failed\"}}\n\n"
		r := newNonStreamingSSEResponse()
		r.Body = io.NopCloser(strings.NewReader(body))
		s := newNonStreamingFailoverService()
		a := &Account{ID: 1, Platform: PlatformOpenAI, Type: AccountTypeOAuth}
		var err error
		if passthrough {
			_, err = s.handleStreamingResponsePassthrough(c.Request.Context(), r, c, a, time.Now(), "model", "model")
		} else {
			_, err = s.handleStreamingResponse(c.Request.Context(), r, c, a, time.Now(), "model", "model")
		}
		var failover *UpstreamFailoverError
		require.Error(t, err)
		require.False(t, errors.As(err, &failover))
		require.Contains(t, rec.Body.String(), "hello")
	}
}
