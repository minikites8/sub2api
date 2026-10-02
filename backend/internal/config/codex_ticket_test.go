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
	require.False(t, cfg.Gateway.OpenAICodexTicket.CloudMint.Enabled)
	require.Equal(t, "SUB2API_CODEX_CLOUD_MINT_KEY", cfg.Gateway.OpenAICodexTicket.CloudMint.KeyEnv)
	require.Equal(t, "sse", cfg.Gateway.OpenAICodexTicket.CloudMint.Transport)
	require.Equal(t, "any", cfg.Gateway.OpenAICodexTicket.CloudMint.Gateway)
	require.Equal(t, 25, cfg.Gateway.OpenAICodexTicket.CloudMint.TimeoutSeconds)
}
func TestCodexTicketConfigEnvironment(t *testing.T) {
	resetViperWithJWTSecret(t)
	t.Setenv("GATEWAY_OPENAI_CODEX_TICKET_ENABLED", "true")
	t.Setenv("GATEWAY_OPENAI_CODEX_TICKET_HARVEST_PROXY_URL", "socks5://proxy.example:1080")
	t.Setenv("GATEWAY_OPENAI_CODEX_TICKET_MODELS", "model-a,model-b")
	t.Setenv("GATEWAY_OPENAI_CODEX_TICKET_CLOUD_MINT_ENABLED", "true")
	t.Setenv("GATEWAY_OPENAI_CODEX_TICKET_CLOUD_MINT_URL", "https://relay.example/mint")
	t.Setenv("GATEWAY_OPENAI_CODEX_TICKET_CLOUD_MINT_KEY_ENV", "CUSTOM_RELAY_KEY")
	t.Setenv("GATEWAY_OPENAI_CODEX_TICKET_CLOUD_MINT_PROXY_URL", "socks5://proxy.example:1080")
	t.Setenv("GATEWAY_OPENAI_CODEX_TICKET_CLOUD_MINT_TRANSPORT", "websocket")
	t.Setenv("GATEWAY_OPENAI_CODEX_TICKET_CLOUD_MINT_GATEWAY", "unified-88")
	t.Setenv("GATEWAY_OPENAI_CODEX_TICKET_CLOUD_MINT_TIMEOUT_SECONDS", "31")
	cfg, err := Load()
	require.NoError(t, err)
	require.True(t, cfg.Gateway.OpenAICodexTicket.Enabled)
	require.Equal(t, "socks5://proxy.example:1080", cfg.Gateway.OpenAICodexTicket.HarvestProxyURL)
	require.Equal(t, []string{"model-a", "model-b"}, cfg.Gateway.OpenAICodexTicket.Models)
	require.True(t, cfg.Gateway.OpenAICodexTicket.CloudMint.Enabled)
	require.Equal(t, "https://relay.example/mint", cfg.Gateway.OpenAICodexTicket.CloudMint.URL)
	require.Equal(t, "CUSTOM_RELAY_KEY", cfg.Gateway.OpenAICodexTicket.CloudMint.KeyEnv)
	require.Equal(t, "socks5://proxy.example:1080", cfg.Gateway.OpenAICodexTicket.CloudMint.ProxyURL)
	require.Equal(t, "websocket", cfg.Gateway.OpenAICodexTicket.CloudMint.Transport)
	require.Equal(t, "unified-88", cfg.Gateway.OpenAICodexTicket.CloudMint.Gateway)
	require.Equal(t, 31, cfg.Gateway.OpenAICodexTicket.CloudMint.TimeoutSeconds)
}
