//go:build unit

package handler

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func TestOpenAIOverloadAfterEmptyProgressSwitchesWithoutOptIn(t *testing.T) {
	for _, endpoint := range requestAccountRetryEndpoints {
		streaming := strings.Contains(endpoint.body, `"stream":true`)
		for _, passthrough := range []bool{false, true} {
			for _, progress := range []struct {
				name, event string
				output      bool
			}{
				{"metadata_only", "", false},
				{"response_metadata", `{"type":"response.metadata","sequence_number":2,"metadata":{"request_id":"failed-attempt-metadata"}}`, false},
				{"codex_response_metadata", `{"type":"codex.response.metadata","headers":{"x-codex-turn-state":"failed-attempt-metadata"}}`, false},
				{"codex_rate_limits", `{"type":"codex.rate_limits","rate_limits":{"primary":{"used_percent":25}},"metered_limit_name":"failed-attempt-metadata"}`, false},
				{"empty_text", `{"type":"response.output_text.delta","delta":""}`, false},
				{"empty_reasoning", `{"type":"response.reasoning_summary_text.delta","delta":""}`, false},
				{"empty_item", `{"type":"response.output_item.done","item":{"type":"reasoning","summary":[]}}`, false},
				{"encrypted_reasoning", `{"type":"response.output_item.added","sequence_number":2,"item":{"id":"rs_failed","type":"reasoning","summary":[]}}` + "\n\ndata: " + `{"type":"response.output_item.done","sequence_number":3,"item":{"id":"rs_failed","type":"reasoning","summary":[],"encrypted_content":"failed-attempt-ciphertext"}}`, false},
				{"empty_content", `{"type":"response.content_part.done","part":{"type":"output_text","text":""}}`, false},
				{"real_output", `{"type":"response.output_text.delta","delta":"partial"}`, true},
			} {
				if progress.output && !streaming {
					continue
				}
				t.Run(fmt.Sprintf("%s/stream=%t/passthrough=%t/%s", strings.TrimPrefix(endpoint.path, "/"), streaming, passthrough, progress.name), func(t *testing.T) {
					upstream := &requestAccountRetryUpstream{
						eventType:  "response.failed",
						errorBody:  `{"error":{"code":"server_is_overloaded","message":"Our servers are currently overloaded. Please try again later."}}`,
						preamble:   "data: {\"type\":\"response.in_progress\"}\n\n",
						failureIDs: map[int64]bool{802: true},
					}
					if progress.event != "" {
						upstream.preamble += "data: " + progress.event + "\n\n"
					}
					var accounts []service.Account
					for _, id := range []int64{801, 802, 803} {
						accounts = append(accounts, service.Account{
							ID: id, Platform: service.PlatformOpenAI, Type: service.AccountTypeOAuth,
							Status: service.StatusActive, Schedulable: true, Concurrency: 1, Priority: int(id),
							Credentials: map[string]any{"access_token": "test-access"},
							Extra:       map[string]any{"openai_passthrough": passthrough},
						})
					}
					router := newOpenAI502503RouterWithAccounts(t, upstream, accounts)
					req := httptest.NewRequest(http.MethodPost, endpoint.path, strings.NewReader(endpoint.body))
					rec := httptest.NewRecorder()
					router.ServeHTTP(rec, req)
					if progress.output {
						require.Equal(t, []int64{801}, upstream.hits, rec.Body.String())
						require.Contains(t, rec.Body.String(), "partial")
						require.NotContains(t, rec.Body.String(), "healthy")
						return
					}
					require.Equal(t, []int64{801, 802, 803}, upstream.hits, rec.Body.String())
					require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
					require.Contains(t, rec.Body.String(), "healthy")
					require.NotContains(t, rec.Body.String(), "resp_failed_attempt")
					require.NotContains(t, rec.Body.String(), "failed-attempt-ciphertext")
					require.NotContains(t, rec.Body.String(), "failed-attempt-metadata")
					require.NotContains(t, rec.Body.String(), "overloaded")
				})
			}
		}
	}
}
