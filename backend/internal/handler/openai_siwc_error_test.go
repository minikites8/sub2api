//go:build unit

package handler

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func siwcErrorTestAccount() *service.Account {
	return &service.Account{
		ID: 801, Platform: service.PlatformOpenAI, Type: service.AccountTypeOAuth,
		Status: service.StatusActive, Schedulable: true, Concurrency: 1,
		Credentials: map[string]any{"access_token": "must-not-send"},
		Extra:       map[string]any{service.OpenAISIWCEnabledKey: true},
	}
}

func TestSIWCUnsupportedHistoryReturnsOneHTTPError(t *testing.T) {
	for _, stream := range []string{"false", "true"} {
		t.Run("stream_"+stream, func(t *testing.T) {
			upstream := &excelBPSErrorUpstream{}
			handler := newOpenAIResponsesFailoverTestHandler(t, upstream, *siwcErrorTestAccount())
			body := `{"model":"gpt-6-astra","stream":` + stream + `,"input":[{"type":"agent_message","encrypted_content":"SECRET_HISTORY"}]}`
			c, recorder := newAstraProFailoverContext(t, body)
			// Opaque history must pass the independent long-context policy so
			// this test reaches SIWC capability validation.
			key, _ := c.Get(string(middleware.ContextKeyAPIKey))
			key.(*service.APIKey).AllowLongContext = true

			handler.Responses(c)

			require.Equal(t, http.StatusBadRequest, recorder.Code, recorder.Body.String())
			require.True(t, json.Valid(recorder.Body.Bytes()), "must return one JSON error: %s", recorder.Body.String())
			require.Equal(t, "OPENAI_SIWC_UNSUPPORTED_INPUT", gjson.GetBytes(recorder.Body.Bytes(), "error.code").String())
			require.Contains(t, gjson.GetBytes(recorder.Body.Bytes(), "error.message").String(), "agent_message")
			require.NotContains(t, recorder.Body.String(), "SECRET_HISTORY")
			require.Empty(t, upstream.paths)
		})
	}
}

func TestSIWCLocalErrorDoesNotAppendFallback(t *testing.T) {
	for _, tt := range []struct {
		name, path, input, tools, code string
	}{
		{"agent_history", "/v1/responses", `[{"type":"agent_message","encrypted_content":"SECRET_HISTORY"}]`, `[]`, "OPENAI_SIWC_UNSUPPORTED_INPUT"},
		{"hosted_tool", "/v1/responses", `[]`, `[{"type":"computer"}]`, "OPENAI_SIWC_UNSUPPORTED_TOOL"},
		{"invalid_tools", "/v1/responses", `[{"type":"additional_tools","tools":"invalid"}]`, `[]`, "OPENAI_SIWC_INVALID_REQUEST"},
		{"unsupported_endpoint", "/v1/responses/compact", `[]`, `[]`, "OPENAI_SIWC_UNSUPPORTED_ENDPOINT"},
	} {
		for _, mode := range []string{"json", "stream", "headers_prepared", "stream_started"} {
			t.Run(tt.name+"/"+mode, func(t *testing.T) {
				upstream := &excelBPSErrorUpstream{}
				account := siwcErrorTestAccount()
				gateway := service.NewOpenAIGatewayService(excelBPSErrorAccountRepo{account: account}, nil, nil, nil, nil, nil, nil, nil,
					&config.Config{}, nil, nil, nil, nil, nil, upstream, nil, nil, nil, nil, nil, nil, nil, nil, nil)
				body, err := json.Marshal(map[string]any{
					"model": "gpt-6-astra", "stream": mode != "json",
					"input": json.RawMessage(tt.input), "tools": json.RawMessage(tt.tools),
				})
				require.NoError(t, err)
				recorder := httptest.NewRecorder()
				c, _ := gin.CreateTestContext(recorder)
				c.Request = httptest.NewRequest(http.MethodPost, tt.path, nil)
				if mode == "headers_prepared" || mode == "stream_started" {
					c.Header("Content-Type", "text/event-stream")
				}
				if mode == "stream_started" {
					_, err = c.Writer.WriteString(": keepalive\n\n")
					require.NoError(t, err)
					c.Writer.Flush()
				}
				before := c.Writer.Size()

				_, err = gateway.Forward(context.Background(), c, account, body)

				require.Error(t, err)
				var failover *service.UpstreamFailoverError
				if errors.As(err, &failover) && failover.IsSIWCLocalCapabilityRejection() {
					require.False(t, service.IsResponseCommitted(c))
					require.Equal(t, before, c.Writer.Size(), "capability rejection must leave the account-switch window open")
					(&OpenAIGatewayHandler{}).handleFailoverExhausted(c, failover, false)
				}
				response := recorder.Body.String()
				if !openAIForwardErrorAlreadyCommunicated(c, before, err) {
					require.False(t, (&OpenAIGatewayHandler{}).ensureForwardErrorResponse(c, false), "the specific error already terminated the response")
				}
				require.Equal(t, response, recorder.Body.String())
				require.True(t, service.IsResponseCommitted(c))
				require.Empty(t, upstream.paths)
				require.NotContains(t, response, "SECRET_HISTORY")
				require.NotContains(t, response, "Upstream request failed")
				if mode == "stream_started" {
					require.Equal(t, http.StatusOK, recorder.Code)
					require.Equal(t, "text/event-stream", recorder.Header().Get("Content-Type"))
					require.Equal(t, 1, strings.Count(response, "event: response.failed\n"))
					data := strings.TrimSpace(strings.TrimPrefix(strings.TrimPrefix(response, ": keepalive\n\n"), "event: response.failed\ndata: "))
					require.True(t, json.Valid([]byte(data)), data)
					require.Equal(t, tt.code, gjson.Get(data, "response.error.code").String())
					require.Equal(t, "failed", gjson.Get(data, "response.status").String())
				} else {
					require.Equal(t, http.StatusBadRequest, recorder.Code)
					require.Equal(t, "application/json; charset=utf-8", recorder.Header().Get("Content-Type"))
					require.True(t, json.Valid([]byte(response)), response)
					require.Equal(t, tt.code, gjson.Get(response, "error.code").String())
				}
			})
		}
	}
}
