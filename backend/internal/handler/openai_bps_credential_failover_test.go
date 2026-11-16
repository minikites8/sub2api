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
)

func TestBPSCredentialsUnavailableSwitchesBeforeSubmission(t *testing.T) {
	for _, nextRoute := range []string{"bps", "native"} {
		for _, heartbeat := range []bool{false, true} {
			for _, optIn := range []bool{false, true} {
				t.Run(fmt.Sprintf("next=%s/heartbeat=%t/optIn=%t", nextRoute, heartbeat, optIn), func(t *testing.T) {
					accounts := queueHeartbeatAccounts("bps")
					delete(accounts[0].Credentials, "access_token")
					accounts[1].Extra["openai_excel_bps"] = nextRoute == "bps"
					upstream := &excelBPSFailoverUpstream{answer: func(int) *http.Response { return excelBPSCompleted() }}
					rec := httptest.NewRecorder()
					before := []gin.HandlerFunc{func(c *gin.Context) {
						c.Next()
						value, exists := c.Get(service.OpsUpstreamErrorsKey)
						require.True(t, exists)
						events := value.([]*service.OpsUpstreamErrorEvent)
						require.NotEmpty(t, events)
						require.Equal(t, "bps_credentials_unavailable", events[0].Reason)
						require.Equal(t, "account_auth", events[0].Stage)
						require.Equal(t, int64(801), events[0].AccountID)
						require.Zero(t, events[0].UpstreamStatusCode)
						require.Nil(t, events[0].ProxyID)
					}}
					if heartbeat {
						before = append(before, waitForQueueHeartbeat(t, "account", rec))
					}
					router := newBPSLocalCapabilityRouter(t, upstream, accounts, before...)
					synctest.Test(t, func(t *testing.T) {
						req := httptest.NewRequest(http.MethodPost, "/v1/responses", strings.NewReader(`{"model":"gpt-6-astra","input":"hello","stream":true}`))
						if optIn {
							req.Header.Set("X-Sub2API-Retry-Account", "true")
						}
						router.ServeHTTP(rec, req)
					})
					require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
					require.Equal(t, []int64{802}, upstream.calls(), "the broken account must never send inference or retry credentials on the same account")
					require.Contains(t, rec.Body.String(), "done")
					require.NotContains(t, rec.Body.String(), "credentials")
					require.NotContains(t, rec.Body.String(), "Upstream request failed")
				})
			}
		}
	}
}

func TestBPSCredentialsUnavailableExhaustionIsGenericAndBounded(t *testing.T) {
	for _, switchLimit := range []string{"", "0"} {
		t.Run("switchLimit="+switchLimit, func(t *testing.T) {
			accounts := queueHeartbeatAccounts("bps")
			for i := range accounts {
				delete(accounts[i].Credentials, "access_token")
			}
			upstream := &excelBPSFailoverUpstream{answer: func(int) *http.Response {
				t.Fatal("credential acquisition failure must not submit inference")
				return nil
			}}
			var attempted []int64
			router := newBPSLocalCapabilityRouter(t, upstream, accounts, func(c *gin.Context) {
				c.Next()
				value, exists := c.Get(service.OpsUpstreamErrorsKey)
				require.True(t, exists)
				for _, event := range value.([]*service.OpsUpstreamErrorEvent) {
					if event.Reason == "bps_credentials_unavailable" {
						attempted = append(attempted, event.AccountID)
					}
				}
			})
			rec := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodPost, "/v1/responses", strings.NewReader(`{"model":"gpt-6-astra","input":"hello"}`))
			if switchLimit != "" {
				req.Header.Set("X-Sub2API-Retry-Account", "true")
				req.Header.Set("X-Sub2API-Max-Retries", switchLimit)
			}
			router.ServeHTTP(rec, req)
			require.Equal(t, http.StatusServiceUnavailable, rec.Code, rec.Body.String())
			require.Empty(t, upstream.calls())
			if switchLimit == "0" {
				require.Equal(t, []int64{801}, attempted)
			} else {
				require.Equal(t, []int64{801, 802}, attempted)
			}
			require.Contains(t, rec.Body.String(), "503: Service unavailable.")
			for _, internal := range []string{"bps", "basispoints", "Grok", "credentials", "access_token"} {
				require.NotContains(t, rec.Body.String(), internal)
			}
		})
	}
}

func TestOpenAILocalForwardFailureKeepsCurrentDiagnostic(t *testing.T) {
	accounts := queueHeartbeatAccounts("native")[:1]
	delete(accounts[0].Credentials, "access_token")
	upstream := &excelBPSFailoverUpstream{answer: func(int) *http.Response {
		t.Fatal("missing credentials must fail before inference")
		return nil
	}}
	router := newBPSLocalCapabilityRouter(t, upstream, accounts, func(c *gin.Context) {
		service.SetOpsUpstreamError(c, 429, "earlier account rate limit", "old detail")
		c.Next()
		require.Zero(t, c.GetInt(service.OpsUpstreamStatusCodeKey), "a local failure must not inherit an earlier upstream status")
		require.Equal(t, "", c.GetString(service.OpsUpstreamErrorDetailKey))
		require.Contains(t, c.GetString(service.OpsUpstreamErrorMessageKey), "access_token not found")
		value, exists := c.Get(service.OpsUpstreamErrorsKey)
		require.True(t, exists)
		events := value.([]*service.OpsUpstreamErrorEvent)
		require.Equal(t, "unclassified_forward_error", events[len(events)-1].Reason)
	})
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/v1/responses", strings.NewReader(`{"model":"gpt-6-astra","input":"hello"}`)))
	require.Equal(t, http.StatusBadGateway, rec.Code, rec.Body.String())
	require.JSONEq(t, `{"error":{"message":"Upstream request failed","type":"upstream_error"}}`, rec.Body.String())
}

func TestOpenAIPanicKeepsPrivateDiagnostic(t *testing.T) {
	for _, message := range []string{
		"token refresh failed: Bearer PRIVATE_TOKEN_123456789012345",
		`invalid encrypted content: {"encrypted_content":"PRIVATE_CIPHERTEXT"}`,
	} {
		t.Run(message[:12], func(t *testing.T) {
			rec := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(rec)
			c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
			h := &OpenAIGatewayHandler{}
			func() {
				defer h.recoverResponsesPanic(c, nil)
				panic(message)
			}()
			require.Equal(t, http.StatusBadGateway, rec.Code)
			require.Contains(t, rec.Body.String(), "Upstream request failed")
			require.NotContains(t, rec.Body.String(), "PRIVATE_")
			value, exists := c.Get(service.OpsUpstreamErrorsKey)
			require.True(t, exists)
			events := value.([]*service.OpsUpstreamErrorEvent)
			require.Equal(t, "gateway_panic", events[0].Reason)
			require.NotContains(t, events[0].Message, "PRIVATE_")
			require.Zero(t, events[0].UpstreamStatusCode)
		})
	}
}
