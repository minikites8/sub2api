//go:build unit

package handler

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/synctest"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestBPSEncryptedAssignmentFallsBackToNative(t *testing.T) {
	for _, stream := range []bool{false, true} {
		for _, heartbeat := range []bool{false, true} {
			if heartbeat && !stream {
				continue
			}
			for _, nativeAvailable := range []bool{false, true} {
				t.Run(fmt.Sprintf("stream=%t/heartbeat=%t/native=%t", stream, heartbeat, nativeAvailable), func(t *testing.T) {
					var accounts []service.Account
					for id := int64(801); id <= 808; id++ {
						if id == 808 && !nativeAvailable {
							continue
						}
						accounts = append(accounts, service.Account{ID: id, Platform: service.PlatformOpenAI, Type: service.AccountTypeOAuth,
							Status: service.StatusActive, Schedulable: true, Concurrency: 1, Priority: int(id),
							Credentials: map[string]any{"access_token": "test-access", "chatgpt_account_id": fmt.Sprint(id)},
							Extra:       map[string]any{"openai_excel_bps": id != 808, service.ExcelBPSIgnoreEncryptedContentKey: true}})
					}
					upstream := &excelBPSFailoverUpstream{answer: func(int) *http.Response { return excelBPSCompleted() }}
					rec := httptest.NewRecorder()
					var before []gin.HandlerFunc
					if heartbeat {
						before = append(before, waitForQueueHeartbeat(t, "account", rec))
					}
					router := newBPSLocalCapabilityRouter(t, upstream, accounts, before...)
					body := fmt.Sprintf(`{"model":"gpt-6-astra","stream":%t,"input":[{"role":"user","content":"keep original history"},{"type":"agent_message","author":"parent","recipient":"child","content":[{"type":"input_text","text":"task header"},{"type":"encrypted_content","data":"PRIVATE_ASSIGNMENT"}]}]}`, stream)
					synctest.Test(t, func(t *testing.T) {
						router.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/v1/responses", strings.NewReader(body)))
					})
					if nativeAvailable {
						require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
						require.Equal(t, []int64{808}, upstream.calls(), "skip all BPS routes, without submitting or retrying on them")
						require.NotContains(t, upstream.urls[0], "basispoints")
						require.JSONEq(t, gjson.Get(body, "input").Raw, gjson.GetBytes(upstream.bodies[0], "input").Raw)
						require.Contains(t, string(upstream.bodies[0]), "PRIVATE_ASSIGNMENT")
						require.Contains(t, rec.Body.String(), "done")
					} else {
						require.Empty(t, upstream.calls(), "never send an unreadable task to another BPS route")
						require.Contains(t, rec.Body.String(), "400: Invalid request body.")
						require.NotContains(t, strings.ToLower(rec.Body.String()), "bps")
						require.NotContains(t, strings.ToLower(rec.Body.String()), "basispoints")
					}
					require.NotContains(t, rec.Body.String(), "PRIVATE_ASSIGNMENT")
				})
			}
		}
	}
}
