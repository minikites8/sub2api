package config

import (
	"github.com/stretchr/testify/require"
	"testing"
)

func TestCodexTicketConfigDefaults(t *testing.T) {
	resetViperWithJWTSecret(t)
	cfg, err := Load()
	require.NoError(t, err)
	require.False(t, cfg.Gateway.OpenAICodexTicket.Enabled)
	require.True(t, cfg.Gateway.OpenAICodexTicket.FailClosed)
	require.Equal(t, 3600, cfg.Gateway.OpenAICodexTicket.TTLSeconds)
	require.Equal(t, []string{"gpt-6-astra", "gpt-5.6-sol"}, cfg.Gateway.OpenAICodexTicket.Models)
}
func TestCodexTicketConfigEnvironment(t *testing.T) {
	resetViperWithJWTSecret(t)
	t.Setenv("GATEWAY_OPENAI_CODEX_TICKET_ENABLED", "true")
	t.Setenv("GATEWAY_OPENAI_CODEX_TICKET_HARVEST_PROXY_URL", "socks5://proxy.example:1080")
	t.Setenv("GATEWAY_OPENAI_CODEX_TICKET_MODELS", "model-a,model-b")
	cfg, err := Load()
	require.NoError(t, err)
	require.True(t, cfg.Gateway.OpenAICodexTicket.Enabled)
	require.Equal(t, "socks5://proxy.example:1080", cfg.Gateway.OpenAICodexTicket.HarvestProxyURL)
	require.Equal(t, []string{"model-a", "model-b"}, cfg.Gateway.OpenAICodexTicket.Models)
}
