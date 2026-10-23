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

func TestOpenAICodexRateLimitsBeforeBareOverloadSwitchesAccounts(t *testing.T) {
	for _, endpoint := range requestAccountRetryEndpoints {
		for _, passthrough := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/%s/passthrough=%t", endpoint.path, endpoint.body, passthrough), func(t *testing.T) {
				upstream := &requestAccountRetryUpstream{
					eventType:  "error",
					errorBody:  `{"error":{"code":"server_is_overloaded","message":"Our servers are currently overloaded. Please try again later.","param":null,"type":"service_unavailable_error"},"sequence_number":3,"status":503}`,
					preamble:   "data: {\"type\":\"codex.rate_limits\",\"rate_limits\":{\"primary\":{\"used_percent\":25}},\"metered_limit_name\":\"failed-quota\"}\n\n",
					failureIDs: map[int64]bool{802: true},
				}
				var accounts []service.Account
				for _, id := range []int64{801, 802, 803} {
					account := service.Account{
						ID: id, Platform: service.PlatformOpenAI, Type: service.AccountTypeOAuth,
						Status: service.StatusActive, Schedulable: true, Concurrency: 1, Priority: int(id),
						Credentials: map[string]any{"access_token": "test-access"},
						Extra:       map[string]any{"openai_passthrough": passthrough},
					}
					if id == 803 {
						account.Type = service.AccountTypeAPIKey
						account.Credentials = map[string]any{"api_key": "test-key", "base_url": "https://api.example.test/v1"}
					}
					accounts = append(accounts, account)
				}
				router := newOpenAI502503RouterWithAccounts(t, upstream, accounts)
				rec := httptest.NewRecorder()
				req := httptest.NewRequest(http.MethodPost, endpoint.path, strings.NewReader(endpoint.body))
				router.ServeHTTP(rec, req)
				require.Equal(t, []int64{801, 802, 803}, upstream.hits, rec.Body.String())
				require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
				require.Contains(t, rec.Body.String(), "healthy")
				require.NotContains(t, rec.Body.String(), "failed-quota")
				require.NotContains(t, rec.Body.String(), "resp_failed_attempt")
				require.NotContains(t, rec.Body.String(), "overloaded")
			})
		}
	}
}
