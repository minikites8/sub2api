//go:build unit

package service

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestChannel_IsMarketplaceProviderVisible(t *testing.T) {
	tests := []struct {
		name     string
		channel  *Channel
		platform string
		want     bool
	}{
		{
			name: "enabled platform",
			channel: &Channel{FeaturesConfig: map[string]any{
				featureKeyMarketplaceProviderVisible: map[string]any{"baidu_vod": true},
			}},
			platform: "baidu_vod",
			want:     true,
		},
		{
			name: "disabled platform",
			channel: &Channel{FeaturesConfig: map[string]any{
				featureKeyMarketplaceProviderVisible: map[string]any{"baidu_vod": false},
			}},
			platform: "baidu_vod",
		},
		{
			name: "other platform",
			channel: &Channel{FeaturesConfig: map[string]any{
				featureKeyMarketplaceProviderVisible: map[string]any{"openai": true},
			}},
			platform: "anthropic",
		},
		{name: "missing config", channel: &Channel{}, platform: "openai"},
		{name: "nil channel", platform: "openai"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.want, tt.channel.IsMarketplaceProviderVisible(tt.platform))
		})
	}
}
