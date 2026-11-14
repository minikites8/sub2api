//go:build unit

package handler

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func TestOpenAIWebSocketRestartRetriesBeforeOutput(t *testing.T) {
	for _, terminal := range []string{"error", "response.failed"} {
		for _, content := range []string{"none", "empty_reasoning", "encrypted_reasoning"} {
			t.Run(terminal+"/"+content, func(t *testing.T) {
				accounts := make([]service.Account, 0, 2)
				for _, id := range []int64{801, 802} {
					accounts = append(accounts, service.Account{ID: id, Name: fmt.Sprint(id), Platform: service.PlatformOpenAI, Type: service.AccountTypeAPIKey, Status: service.StatusActive, Schedulable: true, Concurrency: 1, Priority: int(id), Credentials: map[string]any{"api_key": "fixture-key", "base_url": "https://api.openai.com"}, Extra: map[string]any{"openai_passthrough": true}})
				}
				upstream := &excelBPSFailoverUpstream{answer: func(call int) *http.Response {
					if call >= 3 {
						return excelBPSCompleted()
					}
					wire := "data: {\"type\":\"response.created\"}\n\n"
					if content != "none" {
						item := `{"type":"reasoning","summary":[]}`
						if content == "encrypted_reasoning" {
							item = `{"type":"reasoning","summary":[],"encrypted_content":"fixture-cipher"}`
						}
						wire += "data: {\"type\":\"response.output_item.done\",\"item\":" + item + "}\n\n"
					}
					errBody := `{"type":"websocket_connection_error","code":"1012","message":"The upstream WebSocket closed before the response completed (code 1012)."}`
					if terminal == "error" {
						wire += "data: {\"type\":\"error\",\"error\":" + errBody + "}\n\n"
					} else {
						wire += "data: {\"type\":\"response.failed\",\"response\":{\"error\":" + errBody + "}}\n\n"
					}
					return &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": {"text/event-stream"}}, Body: io.NopCloser(strings.NewReader(wire))}
				}}
				router := newOpenAI502503RouterWithAccounts(t, upstream, accounts)
				rec := httptest.NewRecorder()
				req := httptest.NewRequest(http.MethodPost, "/v1/responses", strings.NewReader(`{"model":"gpt-5.2","input":"hello","stream":true}`))
				req.Header.Set("Content-Type", "application/json")
				router.ServeHTTP(rec, req)
				if content == "encrypted_reasoning" {
					require.Equal(t, []int64{801}, upstream.calls())
					require.Contains(t, rec.Body.String(), "fixture-cipher")
					return
				}
				require.Equal(t, []int64{801, 801, 802, 802}, upstream.calls(), rec.Body.String())
				require.Equal(t, http.StatusOK, rec.Code)
				require.Contains(t, rec.Body.String(), "done")
				require.NotContains(t, rec.Body.String(), "1012")
			})
		}
	}
}
