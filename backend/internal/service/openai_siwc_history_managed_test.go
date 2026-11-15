//go:build unit

package service

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestSIWCHistoryCompatibilityManagedConnectionWithLegacyCodex(t *testing.T) {
	reauth, _, account := accountTestManagedConnection(t, "siwc")
	account.Extra[OpenAISIWCEnabledKey] = false
	account.Extra[OpenAISIWCIgnoreEncryptedContentKey] = true
	upstream := &httpUpstreamRecorder{resp: &http.Response{
		StatusCode: http.StatusOK, Header: http.Header{"Content-Type": {"text/event-stream"}},
		Body: io.NopCloser(strings.NewReader("data: {\"type\":\"response.completed\",\"response\":{\"id\":\"resp_managed_history\",\"status\":\"completed\",\"output\":[]}}\n\n")),
	}}
	gateway := openAIClientToolsTestService(upstream)
	gateway.excelOAuthReauth = reauth
	body := []byte(`{"model":"gpt-5.4","stream":false,"input":` + siwcCompatibilityHistory + `,"tools":[{"type":"function","name":"read","parameters":{"type":"object"}}]}`)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)

	_, err := gateway.Forward(context.Background(), c, account, body)

	require.NoError(t, err)
	require.Len(t, upstream.requests, 1)
	require.Equal(t, "Bearer synthetic-managed-token", upstream.lastReq.Header.Get("Authorization"))
	require.NotContains(t, string(upstream.lastBody), "PRIVATE_")
	require.Equal(t, "message", gjson.GetBytes(upstream.lastBody, "input.1.type").String())
	require.Contains(t, string(upstream.lastBody), "audit passed")
	require.Equal(t, false, account.Extra[OpenAISIWCEnabledKey], "selecting a managed connection must not change the retained legacy protocol")
	require.Equal(t, "synthetic-misleading-oauth", account.GetCredential("access_token"))
}
