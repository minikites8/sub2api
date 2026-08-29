package service

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/ctxkey"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestNormalizeOpenAIServiceTierConfig(t *testing.T) {
	tests := []struct {
		name     string
		platform string
		mode     string
		tier     string
		wantMode string
		wantTier string
	}{
		{name: "empty mode defaults to passthrough", platform: PlatformOpenAI, wantMode: OpenAIServiceTierModePassthrough},
		{name: "set trims and normalizes fast", platform: PlatformOpenAI, mode: " SET ", tier: " Fast ", wantMode: OpenAIServiceTierModeSet, wantTier: OpenAIFastTierPriority},
		{name: "clear drops configured tier", platform: PlatformOpenAI, mode: "CLEAR", tier: "priority", wantMode: OpenAIServiceTierModeClear},
		{name: "non openai resets policy", platform: PlatformAnthropic, mode: OpenAIServiceTierModeSet, tier: OpenAIFastTierFlex, wantMode: OpenAIServiceTierModePassthrough},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mode, tier, err := NormalizeOpenAIServiceTierConfig(tt.platform, tt.mode, tt.tier)
			require.NoError(t, err)
			require.Equal(t, tt.wantMode, mode)
			require.Equal(t, tt.wantTier, tier)
		})
	}

	_, _, err := NormalizeOpenAIServiceTierConfig(PlatformOpenAI, OpenAIServiceTierModeSet, "unsupported")
	require.Error(t, err)
	_, _, err = NormalizeOpenAIServiceTierConfig(PlatformOpenAI, "override", OpenAIFastTierPriority)
	require.Error(t, err)
}

func TestApplyGroupOpenAIServiceTierPolicyToWSResponseCreate(t *testing.T) {
	account := &Account{Platform: PlatformOpenAI, Type: AccountTypeAPIKey}
	groupContext := func(mode, tier string) context.Context {
		return context.WithValue(context.Background(), ctxkey.Group, &Group{
			ID: 1, Platform: PlatformOpenAI, Status: StatusActive, Hydrated: true,
			OpenAIServiceTierMode: mode, OpenAIServiceTier: tier,
		})
	}
	svc := &OpenAIGatewayService{}

	setFrame, blocked, err := svc.applyOpenAIFastPolicyToWSResponseCreate(
		groupContext(OpenAIServiceTierModeSet, OpenAIFastTierPriority),
		account,
		"gpt-5.5",
		[]byte(`{"type":"response.create","service_tier":"flex"}`),
	)
	require.NoError(t, err)
	require.Nil(t, blocked)
	require.Equal(t, OpenAIFastTierPriority, gjson.GetBytes(setFrame, "service_tier").String())

	clearFrame, blocked, err := svc.applyOpenAIFastPolicyToWSResponseCreate(
		groupContext(OpenAIServiceTierModeClear, ""),
		account,
		"gpt-5.5",
		[]byte(`{"type":"response.create","service_tier":"priority"}`),
	)
	require.NoError(t, err)
	require.Nil(t, blocked)
	require.False(t, gjson.GetBytes(clearFrame, "service_tier").Exists())

	passFrame, blocked, err := svc.applyOpenAIFastPolicyToWSResponseCreate(
		groupContext(OpenAIServiceTierModePassthrough, ""),
		account,
		"gpt-5.5",
		[]byte(`{"type":"response.create","service_tier":"flex"}`),
	)
	require.NoError(t, err)
	require.Nil(t, blocked)
	require.Equal(t, OpenAIFastTierFlex, gjson.GetBytes(passFrame, "service_tier").String())
}

func TestOpenAIGatewayServiceForward_GroupServiceTierSetPropagatesToUsage(t *testing.T) {
	gin.SetMode(gin.TestMode)

	upstream := &httpUpstreamRecorder{resp: &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"text/event-stream"}, "x-request-id": []string{"rid_group_service_tier"}},
		Body: io.NopCloser(strings.NewReader(
			`data: {"type":"response.completed","response":{"id":"resp_group_service_tier","object":"response","model":"gpt-5.5","status":"completed","output":[],"usage":{"input_tokens":3,"output_tokens":1,"total_tokens":4}}}` + "\n\n",
		)),
	}}
	cfg := &config.Config{}
	cfg.Security.URLAllowlist.Enabled = false
	svc := &OpenAIGatewayService{cfg: cfg, httpUpstream: upstream}
	account := &Account{
		ID: 901, Name: "oauth-service-tier", Platform: PlatformOpenAI, Type: AccountTypeOAuth, Concurrency: 1,
		Credentials: map[string]any{"access_token": "oauth-token", "chatgpt_account_id": "chatgpt-account"},
		Status:      StatusActive, Schedulable: true,
	}
	group := &Group{
		ID: 902, Platform: PlatformOpenAI, Status: StatusActive, Hydrated: true,
		OpenAIServiceTierMode: OpenAIServiceTierModeSet, OpenAIServiceTier: OpenAIFastTierPriority,
	}
	ctx := context.WithValue(context.Background(), ctxkey.Group, group)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	body := []byte(`{"model":"gpt-5.5","stream":true,"input":"hello"}`)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", strings.NewReader(string(body)))
	SetOpenAIClientTransport(c, OpenAIClientTransportHTTP)

	result, err := svc.Forward(ctx, c, account, body)
	require.NoError(t, err)
	require.NotNil(t, result)
	require.Equal(t, OpenAIFastTierPriority, gjson.GetBytes(upstream.lastBody, "service_tier").String())
	require.NotNil(t, result.ServiceTier)
	require.Equal(t, OpenAIFastTierPriority, *result.ServiceTier)
}
