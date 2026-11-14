//go:build unit

package service

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/openai"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func accountTestManagedConnection(t *testing.T, kind string) (*OpenAIOAuthReauthService, *connectionTypeRepo, *Account) {
	t.Helper()
	reauth, repo := connectionTypeService(AccountTypeOAuth)
	repo.managed = true
	connection := &OpenAIAuthConnection{
		ID: "6a0f1f1e-6c9f-4d55-9c3c-3f0a1b2c3d4e", AccountID: 42,
		Kind: kind, Status: "active", RefreshState: "idle", Revision: 1, Preferred: true,
		Capabilities: connectionCapabilities(kind, openai.SIWCDirectScope), ClientID: "synthetic-issued",
	}
	if kind == "bps" {
		connection.ClientID = openai.ExcelClientID
	}
	require.NoError(t, reauth.encryptConnectionCredentials(connection, map[string]any{
		"access_token": "synthetic-managed-token", "client_id": connection.ClientID,
		"expires_at": strconv.FormatInt(time.Now().Add(time.Hour).Unix(), 10), "chatgpt_account_id": "synthetic-workspace",
	}))
	repo.rows = []*OpenAIAuthConnection{connection}
	account, err := reauth.accounts.GetAccount(context.Background(), 42)
	require.NoError(t, err)
	account.Status, account.Schedulable, account.Concurrency = StatusActive, true, 1
	account.Extra = map[string]any{"openai_connections_managed": true, OpenAISIWCEnabledKey: kind == "siwc", "openai_excel_bps": kind == "bps"}
	return reauth, repo, account
}

func TestAccountTestServiceManagedSIWCConnection(t *testing.T) {
	for _, tc := range []struct {
		name       string
		background bool
		legacySIWC bool
	}{
		{name: "admin test endpoint", legacySIWC: true},
		{name: "background without URL", background: true, legacySIWC: true},
		{name: "legacy protocol remains Codex"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			reauth, _, account := accountTestManagedConnection(t, "siwc")
			account.Extra[OpenAISIWCEnabledKey] = tc.legacySIWC
			wire := "data: {\"type\":\"response.output_text.delta\",\"delta\":\"OK\"}\n\n" +
				"data: {\"type\":\"response.completed\",\"response\":{\"id\":\"resp_probe\",\"status\":\"completed\",\"model\":\"gpt-6-astra\",\"usage\":{\"input_tokens\":1,\"output_tokens\":1}}}\n\n"
			upstream := &httpUpstreamRecorder{resp: &http.Response{StatusCode: http.StatusOK,
				Header: http.Header{"Content-Type": {"text/event-stream"}}, Body: io.NopCloser(strings.NewReader(wire))}}
			gateway := openAIClientToolsTestService(upstream)
			gateway.excelOAuthReauth = reauth
			svc := &AccountTestService{openaiGatewayService: gateway, accountRepo: &accountTestLoggingRepo{account: account}}
			recorder := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(recorder)
			c.Request = httptest.NewRequest(http.MethodPost, "/api/v1/admin/accounts/42/test", nil)
			if tc.background {
				c.Request = &http.Request{}
			}
			originalURL := c.Request.URL

			err := svc.TestAccountConnection(c, account.ID, "gpt-6-astra", "Reply OK", AccountTestModeDefault)

			require.NoError(t, err, recorder.Body.String())
			require.Contains(t, recorder.Body.String(), `"type":"test_complete","success":true`)
			require.Len(t, upstream.requests, 1)
			require.Equal(t, "https://api.openai.com/v1/responses", upstream.lastReq.URL.String())
			require.Equal(t, "Bearer synthetic-managed-token", upstream.lastReq.Header.Get("Authorization"))
			require.Equal(t, "synthetic-misleading-oauth", account.GetCredential("access_token"))
			require.Equal(t, originalURL, c.Request.URL, "the admin request URL must remain unchanged")
			require.Empty(t, c.Request.Header.Get("Session-Id"))
		})
	}
}

func TestAccountTestServiceManagedSIWCRejectsUnavailableConnection(t *testing.T) {
	for _, state := range []string{"disabled", "identity only"} {
		t.Run(state, func(t *testing.T) {
			reauth, repo, account := accountTestManagedConnection(t, "siwc")
			if state == "disabled" {
				repo.rows[0].Status = "disabled"
			} else {
				repo.rows[0].Capabilities = nil
			}
			upstream := &httpUpstreamRecorder{}
			gateway := openAIClientToolsTestService(upstream)
			gateway.excelOAuthReauth = reauth
			svc := &AccountTestService{openaiGatewayService: gateway}
			recorder := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(recorder)
			c.Request = httptest.NewRequest(http.MethodPost, "/api/v1/admin/accounts/42/test", nil)

			err := svc.testOpenAIAccountConnection(c, account, "gpt-6-astra", "Reply OK", AccountTestModeDefault)

			require.ErrorContains(t, err, "OPENAI_CONNECTION_NOT_CAPABLE")
			require.Empty(t, upstream.requests, "unavailable connections must not fall back to legacy credentials")
			require.NotContains(t, recorder.Body.String(), `"type":"test_complete"`)
		})
	}
}

func TestExcelBPSToolProbeManagedConnection(t *testing.T) {
	reauth, _, account := accountTestManagedConnection(t, "bps")
	upstream := &bpsProbeUpstream{}
	svc := bpsProbeTestService(upstream)
	svc.openaiGatewayService.excelOAuthReauth = reauth
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/api/v1/admin/accounts/42/test", nil)

	err := svc.testExcelBPSToolRoundtrip(c, account, "gpt-6-astra")

	require.NoError(t, err, recorder.Body.String())
	require.Len(t, upstream.bodies, 3)
	require.Contains(t, recorder.Body.String(), `"type":"test_complete"`)
	require.Equal(t, "/api/v1/admin/accounts/42/test", c.Request.URL.Path)
	require.Empty(t, c.Request.Header.Get("Session-Id"))
}

func TestSIWCForwardRepairsCodexToolDescriptions(t *testing.T) {
	reauth, _, account := accountTestManagedConnection(t, "siwc")
	wire := "data: {\"type\":\"response.output_text.delta\",\"delta\":\"OK\"}\n\n" +
		"data: {\"type\":\"response.completed\",\"response\":{\"id\":\"resp_siwc\",\"status\":\"completed\",\"model\":\"gpt-6-astra\",\"output\":[{\"type\":\"message\",\"role\":\"assistant\",\"content\":[{\"type\":\"output_text\",\"text\":\"OK\"}]}],\"usage\":{\"input_tokens\":1,\"output_tokens\":1}}}\n\n"
	upstream := &httpUpstreamRecorder{resp: &http.Response{StatusCode: http.StatusOK,
		Header: http.Header{"Content-Type": {"text/event-stream"}}, Body: io.NopCloser(strings.NewReader(wire))}}
	gateway := openAIClientToolsTestService(upstream)
	gateway.excelOAuthReauth = reauth
	body := []byte(`{"model":"gpt-6-astra","stream":true,"tools":[
		{"type":"namespace","name":"collaboration","tools":[{"type":"function","name":"wait_agent","parameters":{"type":"object"}}]},
		{"type":"function","name":"exec","parameters":{"type":"object"}}
	],"input":[{"type":"function_call","namespace":"collaboration","name":"wait_agent","call_id":"call_1","arguments":"{}"},{"type":"function_call_output","call_id":"call_1","output":"done"}]}`)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", strings.NewReader(string(body)))

	_, err := gateway.Forward(context.Background(), c, account, body)

	require.NoError(t, err)
	require.Len(t, upstream.requests, 1)
	require.Equal(t, "https://api.openai.com/v1/responses", upstream.lastReq.URL.String())
	require.Equal(t, "Bearer synthetic-managed-token", upstream.lastReq.Header.Get("Authorization"))
	for _, tool := range gjson.GetBytes(upstream.lastBody, "tools").Array() {
		require.NotEmpty(t, tool.Get("description").String())
	}
	require.Equal(t, "collaboration", gjson.GetBytes(upstream.lastBody, "input.0.namespace").String())
	require.Equal(t, "wait_agent", gjson.GetBytes(upstream.lastBody, "input.0.name").String())
	require.Equal(t, "done", gjson.GetBytes(upstream.lastBody, "input.1.output").String())
}
