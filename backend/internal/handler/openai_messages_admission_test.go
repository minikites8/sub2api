package handler

import (
	"context"
	"errors"
	"fmt"
	"github.com/Wei-Shaw/sub2api/internal/config"
	middleware2 "github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestMessagesAdmissionRecoveryAndSpecificErrors(t *testing.T) {
	for _, stream := range []bool{false, true} {
		for _, tc := range []struct {
			name                     string
			allDenied, dbFailure     bool
			limit, wantCalls, status int
			reason                   string
		}{
			{name: "reselect healthy account", limit: 3, wantCalls: 1, status: 400},
			{name: "all accounts unavailable", allDenied: true, limit: 3, status: 503, reason: "account_ineligible"},
			{name: "database unavailable", dbFailure: true, limit: 3, status: 503, reason: "latest_state_unavailable"},
			{name: "bounded retries", limit: 0, status: 503, reason: "account_ineligible"},
		} {
			t.Run(fmt.Sprintf("%s/stream=%t", tc.name, stream), func(t *testing.T) {
				accounts := []service.Account{{ID: 1, Platform: service.PlatformOpenAI, Type: service.AccountTypeOAuth, Status: service.StatusActive, Schedulable: true, Priority: 0, Credentials: map[string]any{"access_token": "token-1"}, Extra: map[string]any{"openai_excel_bps": false, "openai_passthrough": true}}, {ID: 2, Platform: service.PlatformOpenAI, Type: service.AccountTypeOAuth, Status: service.StatusActive, Schedulable: true, Priority: 1, Credentials: map[string]any{"access_token": "token-2"}, Extra: map[string]any{"openai_excel_bps": false, "openai_passthrough": true}}}
				repo := &httpAdmissionRecoveryRepo{httpAdmissionSelectedRepo: httpAdmissionSelectedRepo{accounts: accounts}, allDenied: tc.allDenied}
				if tc.dbFailure {
					repo.readErr = errors.New("fixture database outage")
				}
				u := &httpAdmissionRecoveryUpstream{}
				cfg := &config.Config{RunMode: config.RunModeSimple}
				gateway := service.NewOpenAIGatewayService(repo, nil, nil, nil, nil, nil, nil, nil, cfg, nil, nil, nil, nil, nil, u, nil, nil, nil, nil, nil, nil, nil, nil, nil)
				billing := service.NewBillingCacheService(nil, nil, nil, nil, nil, nil, cfg, nil)
				t.Cleanup(billing.Stop)
				h := NewOpenAIGatewayHandler(gateway, service.NewConcurrencyService(nil), billing, service.NewAPIKeyService(nil, nil, nil, nil, nil, nil, cfg), nil, nil, nil, nil, nil, cfg)
				h.maxAccountSwitches = tc.limit
				c, rec := newHTTPAdmissionRecoveryContext(t, context.Background())
				key, _ := middleware2.GetAPIKeyFromContext(c)
				key.Group.AllowMessagesDispatch = true
				c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", strings.NewReader(fmt.Sprintf(`{"model":"gpt-5.1","max_tokens":32,"messages":[{"role":"user","content":"hello"}],"stream":%t}`, stream)))
				h.Messages(c)
				require.Equal(t, tc.status, rec.Code, rec.Body.String())
				require.Len(t, u.accounts, tc.wantCalls)
				if tc.wantCalls > 0 {
					require.Equal(t, int64(2), u.accounts[0])
				}
				if tc.reason != "" {
					require.Contains(t, rec.Body.String(), tc.reason)
					require.Contains(t, c.GetString(service.OpsUpstreamErrorMessageKey), tc.reason)
				}
				require.NotContains(t, rec.Body.String(), "Upstream request failed")
				if tc.dbFailure {
					require.Equal(t, 1, repo.reads)
				}
			})
		}
	}
}
