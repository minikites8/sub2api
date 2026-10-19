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
)

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
