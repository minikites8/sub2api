package handler

import (
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
	"testing"
)

func TestOpenAIFastAliasWSAdmission(t *testing.T) {
	for _, mode := range []string{service.OpenAIWSIngressModeDedicated, service.OpenAIWSIngressModePassthrough} {
		t.Run(mode, func(t *testing.T) {
			got := runOpenAIResponsesWebSocketUsageLogCase(t, openAIResponsesWSUsageLogCase{firstPayload: `{"type":"response.create","model":"gpt-5.4-fast","stream":false}`, secondPayload: `{"type":"response.create","model":"gpt-5.4-fast","stream":false}`, group: wsAllowlistGroup(true, "gpt-5.4"), ingressMode: mode})
			require.Len(t, got.upstreamPayloads, 2)
			for _, b := range got.upstreamPayloads {
				require.Equal(t, "gpt-5.4", gjson.GetBytes(b, "model").String())
				require.Equal(t, "priority", gjson.GetBytes(b, "service_tier").String())
			}
		})
	}
}
