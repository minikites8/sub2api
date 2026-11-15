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
	"github.com/tidwall/gjson"
)

func TestSIWCUnsupportedHistorySwitchesToNativeAccount(t *testing.T) {
	for _, stream := range []bool{false, true} {
		for _, native := range []bool{false, true} {
			t.Run(fmt.Sprintf("stream=%t/native=%t", stream, native), func(t *testing.T) {
				accounts := []service.Account{}
				// More SIWC accounts than the switch budget: protocol rejection
				// must exclude all of them, rather than retrying each identical route.
				for id := int64(801); id <= 808; id++ {
					accounts = append(accounts, service.Account{ID: id, Platform: service.PlatformOpenAI, Type: service.AccountTypeOAuth,
						Status: service.StatusActive, Schedulable: true, Concurrency: 1, Priority: int(id),
						Credentials: map[string]any{"access_token": "synthetic-token", "chatgpt_account_id": fmt.Sprint("synthetic-", id)},
						Extra:       map[string]any{service.OpenAISIWCEnabledKey: true}})
				}
				if native {
					account := accounts[0]
					account.ID, account.Priority = 809, 809
					account.Extra = map[string]any{"openai_passthrough": true}
					accounts = append(accounts, account)
				}
				upstream := &excelBPSFailoverUpstream{answer: func(int) *http.Response { return excelBPSCompleted() }}
				router := newBPSLocalCapabilityRouter(t, upstream, accounts)
				body := fmt.Sprintf(`{"model":"gpt-6-astra","stream":%t,"input":[{"type":"agent_message","content":[{"type":"input_text","text":"ORIGINAL assignment"}],"encrypted_content":"SECRET_HISTORY"}]}`, stream)
				rec := httptest.NewRecorder()
				router.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/v1/responses", strings.NewReader(body)))
				if native {
					require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
					require.Equal(t, []int64{809}, upstream.calls())
					require.NotContains(t, upstream.urls[0], "siwc")
					require.Contains(t, string(upstream.bodies[0]), "ORIGINAL assignment")
					require.Contains(t, string(upstream.bodies[0]), "SECRET_HISTORY")
					require.NotContains(t, rec.Body.String(), "OPENAI_SIWC_UNSUPPORTED_INPUT")
				} else {
					require.Equal(t, http.StatusBadRequest, rec.Code, rec.Body.String())
					require.Equal(t, "OPENAI_SIWC_UNSUPPORTED_INPUT", gjson.GetBytes(rec.Body.Bytes(), "error.code").String())
					require.Empty(t, upstream.calls())
					require.NotContains(t, rec.Body.String(), "SECRET_HISTORY")
				}
				for _, account := range accounts[:8] {
					require.True(t, account.IsSIWCEnabled())
					require.True(t, account.Schedulable)
				}
			})
		}
	}
}
