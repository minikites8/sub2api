package service

import (
	"bufio"
	"context"
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

func TestOpenAIMessagesReadErrorFailoverBoundaries(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, stream := range []bool{false, true} {
		for _, async := range []bool{false, true} {
			for _, mode := range []string{"before output", "canceled", "oversized", "after output"} {
				if !stream && mode == "after output" {
					continue
				}
				t.Run(fmt.Sprintf("stream=%t/async=%t/%s", stream, async, mode), func(t *testing.T) {
					c, _ := gin.CreateTestContext(httptest.NewRecorder())
					ctx, cancel := context.WithCancel(context.Background())
					defer cancel()
					if mode == "canceled" {
						cancel()
					}
					c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", nil).WithContext(ctx)
					readErr := error(io.ErrUnexpectedEOF)
					if mode == "oversized" {
						readErr = bufio.ErrTooLong
					}
					prefix := "data: {\"type\":\"response.created\",\"response\":{\"id\":\"failed-attempt\",\"output\":[]}}\n\n"
					if mode == "after output" {
						prefix += "data: {\"type\":\"response.output_text.delta\",\"output_index\":0,\"content_index\":0,\"delta\":\"partial\"}\n\n"
					}
					resp := &http.Response{StatusCode: 200, Header: http.Header{"X-Request-Id": {"failed-attempt"}}, Body: io.NopCloser(io.MultiReader(strings.NewReader(prefix), &openAICompatBufferedReadErrorCloser{err: readErr}))}
					svc := &OpenAIGatewayService{cfg: &config.Config{}}
					if async {
						svc.cfg.Gateway.StreamKeepaliveInterval = 1
					}
					account := &Account{ID: 1, Platform: PlatformOpenAI, Type: AccountTypeOAuth}
					var result *OpenAIForwardResult
					var err error
					if stream {
						result, err = svc.handleAnthropicStreamingResponse(resp, c, account, "gpt-6-luna", "gpt-6-luna", "gpt-6-luna", time.Now())
					} else {
						result, err = svc.handleAnthropicBufferedStreamingResponse(resp, c, account, "gpt-6-luna", "gpt-6-luna", "gpt-6-luna", time.Now())
					}
					require.Error(t, err)
					var failover *UpstreamFailoverError
					if mode == "before output" {
						require.ErrorAs(t, err, &failover)
						require.Nil(t, result)
						require.False(t, c.Writer.Written())
						require.False(t, failover.RetryableOnSameAccount)
						require.Equal(t, 502, failover.StatusCode)
					} else {
						require.NotErrorAs(t, err, &failover)
					}
				})
			}
		}
	}
}
