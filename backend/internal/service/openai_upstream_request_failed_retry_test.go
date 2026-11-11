package service

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestOpenAIUpstreamRequestFailedSkipsSameAccountRetry(t *testing.T) {
	for _, accountType := range []string{AccountTypeOAuth, AccountTypeAPIKey} {
		for _, pool := range []bool{false, true} {
			for _, tc := range []struct {
				name, message, body string
				status              int
				wantSameAccount     bool
			}{
				{"http 502", "Upstream request failed", `{"error":{"message":"Upstream request failed"}}`, 502, false},
				{"nested 503", "", `{"response":{"error":{"message":"Upstream request failed"}}}`, 503, false},
				{"plain text 500", "", " Upstream request failed ", 500, false},
				{"case insensitive", " UPSTREAM REQUEST FAILED ", "", 502, false},
				{"other upstream error", "Temporary internal error", `{"error":{"message":"Temporary internal error"}}`, 502, true},
				{"echoed request data", "Temporary internal error", `{"error":{"message":"Temporary internal error"},"input":{"message":"Upstream request failed"}}`, 503, true},
				{"client error", "Upstream request failed", "", 400, true},
			} {
				t.Run(fmt.Sprintf("%s/pool=%t/%s", accountType, pool, tc.name), func(t *testing.T) {
					account := &Account{Platform: PlatformOpenAI, Type: accountType, Credentials: map[string]any{"pool_mode": pool, "pool_mode_retry_count": 10}}
					failover := (&OpenAIGatewayService{}).newOpenAIAccountFailoverError(account, tc.status, nil, []byte(tc.body), tc.message, false, pool)
					wantSameAccount := tc.wantSameAccount && (pool || accountType == AccountTypeOAuth && (tc.status == 502 || tc.status == 503))
					require.Equal(t, wantSameAccount, failover.RetryableOnSameAccount)
					require.True(t, failover.ShouldRetryNextAccount())
					if !tc.wantSameAccount {
						require.Zero(t, failover.SameAccountRetryMax)
					}
				})
			}
		}
	}
}
