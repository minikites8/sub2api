package service

import (
	"context"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/pkg/ctxkey"
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
