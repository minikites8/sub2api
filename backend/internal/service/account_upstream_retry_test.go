//go:build unit

package service

import (
	"context"
	"encoding/json"
	"math"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestNormalizeOpenAIUpstreamRetryCredentials(t *testing.T) {
	require.NoError(t, NormalizeOpenAIUpstreamRetryCredentials(nil))
	for _, value := range []any{0, int64(2), float64(3), json.Number("4"), nil, maxOpenAIUpstreamRetries} {
		credentials := map[string]any{credKeyOpenAIUpstreamRetryAccount: true, credKeyOpenAIUpstreamMaxRetries: value}
		require.NoError(t, NormalizeOpenAIUpstreamRetryCredentials(credentials), "%v", value)
		if value == nil {
			require.Contains(t, credentials, credKeyOpenAIUpstreamMaxRetries, "null must survive a JSONB merge patch")
		}
	}
	for _, value := range []any{-1, 1.5, float64(maxOpenAIUpstreamRetries) + 1, "2", true, math.NaN(), math.Inf(1), json.Number("invalid")} {
		require.Error(t, NormalizeOpenAIUpstreamRetryCredentials(map[string]any{credKeyOpenAIUpstreamMaxRetries: value}), "%v", value)
	}
	require.Error(t, NormalizeOpenAIUpstreamRetryCredentials(map[string]any{credKeyOpenAIUpstreamRetryAccount: "true"}))
}

func TestOpenAIUpstreamRetryHeaders(t *testing.T) {
	for _, tc := range []struct {
		name, platform, accountType string
		enabled                     any
		limit                       any
		wantEnabled, wantLimit      string
	}{
		{"enabled", PlatformOpenAI, AccountTypeAPIKey, true, float64(2), "True", "2"},
		{"zero", PlatformOpenAI, AccountTypeAPIKey, true, 0, "True", "0"},
		{"default", PlatformOpenAI, AccountTypeAPIKey, true, nil, "True", ""},
		{"disabled", PlatformOpenAI, AccountTypeAPIKey, false, 2, "", ""},
		{"unset", PlatformOpenAI, AccountTypeAPIKey, nil, nil, "", ""},
		{"malformed flag", PlatformOpenAI, AccountTypeAPIKey, "true", 2, "", ""},
		{"malformed limit", PlatformOpenAI, AccountTypeAPIKey, true, -1, "True", ""},
		{"oauth excluded", PlatformOpenAI, AccountTypeOAuth, true, 2, "", ""},
		{"other platform", PlatformAnthropic, AccountTypeAPIKey, true, 2, "", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			account := &Account{Platform: tc.platform, Type: tc.accountType, Credentials: map[string]any{
				credKeyOpenAIUpstreamRetryAccount: tc.enabled, credKeyOpenAIUpstreamMaxRetries: tc.limit,
			}}
			headers := http.Header{"Authorization": {"Bearer test-key"}}
			account.ApplyHeaderOverrides(headers)
			require.Equal(t, tc.wantEnabled, headers.Get(openAIUpstreamRetryAccountHeader))
			require.Equal(t, tc.wantLimit, headers.Get(openAIUpstreamMaxRetriesHeader))
			require.Equal(t, "Bearer test-key", headers.Get("Authorization"))
			account.ApplyHeaderOverrides(nil)
		})
	}
}

func TestOpenAIUpstreamRetryOverridesCustomHeadersWithoutDuplicates(t *testing.T) {
	account := &Account{Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Credentials: map[string]any{
		credKeyOpenAIUpstreamRetryAccount: true, credKeyOpenAIUpstreamMaxRetries: 0,
		credKeyHeaderOverrideEnabled: true,
		credKeyHeaderOverrides: map[string]any{
			"x-sub2api-retry-account": "False", "x-sub2api-max-retries": "9", "x-other": "preserved",
		},
	}}
	headers := http.Header{"x-SUB2API-MAX-retries": {"7"}, "X-SUB2API-RETRY-ACCOUNT": {"False"}}
	account.ApplyHeaderOverrides(headers)
	require.Len(t, headers, 3)
	require.Equal(t, "True", headers.Get(openAIUpstreamRetryAccountHeader))
	require.Equal(t, "0", headers.Get(openAIUpstreamMaxRetriesHeader))
	require.Equal(t, "preserved", getHeaderRaw(headers, "x-other"))
	account.Credentials[credKeyOpenAIUpstreamMaxRetries] = nil
	account.ApplyHeaderOverrides(headers)
	require.Empty(t, headers.Values(openAIUpstreamMaxRetriesHeader), "cleared limit must use upstream default")
	account.Credentials[credKeyOpenAIUpstreamRetryAccount] = false
	account.ApplyHeaderOverrides(headers)
	require.Equal(t, "False", getHeaderRaw(headers, "x-sub2api-retry-account"), "disabled panel retains generic overrides")
}

func TestOpenAIUpstreamRetryRequestBuilders(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
	account := &Account{Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Credentials: map[string]any{
		"base_url": "https://relay.example.com", "api_key": "test-key",
		credKeyOpenAIUpstreamRetryAccount: true, credKeyOpenAIUpstreamMaxRetries: 2,
	}}
	svc := &OpenAIGatewayService{cfg: &config.Config{}}
	body := []byte(`{"model":"gpt-test","input":"hello","stream":true}`)
	req, err := svc.buildUpstreamRequest(context.Background(), c, account, body, "test-key", true, "", false)
	require.NoError(t, err)
	passthrough, err := svc.buildUpstreamRequestOpenAIPassthrough(context.Background(), c, account, body, "test-key")
	require.NoError(t, err)
	wsHeaders, _, err := svc.buildOpenAIWSHeaders(context.Background(), c, account, "test-key", OpenAIWSProtocolDecision{}, false, "", "", "")
	require.NoError(t, err)
	for _, headers := range []http.Header{req.Header, passthrough.Header, wsHeaders} {
		require.Equal(t, "True", headers.Get(openAIUpstreamRetryAccountHeader))
		require.Equal(t, "2", headers.Get(openAIUpstreamMaxRetriesHeader))
	}
}
