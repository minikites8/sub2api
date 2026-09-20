//go:build unit

package admin

import (
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
	"net/http"
	"testing"
)

func TestCodexTicketSettingsRoundTrip(t *testing.T) {
	h, repo := newStepUpSwitchTestHandler(t, map[string]string{})
	rec := doUpdateSettings(t, h, map[string]any{"openai_codex_ticket_enabled": true, "openai_codex_ticket_harvest_proxy_url": "http://user:ticket-test-secret-123@proxy.example:8080", "openai_codex_ticket_models": "a, b,a"}, nil)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	require.Equal(t, "true", repo.values[service.SettingKeyOpenAICodexTicketEnabled])
	require.Equal(t, "a,b", repo.values[service.SettingKeyOpenAICodexTicketModels])
	require.NotContains(t, rec.Body.String(), "ticket-test-secret-123")
	original := repo.values[service.SettingKeyOpenAICodexTicketHarvestProxyURL]
	rec = doUpdateSettings(t, h, map[string]any{"openai_codex_ticket_harvest_proxy_url": service.MaskCodexTicketProxyURL(original)}, nil)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	require.Equal(t, original, repo.values[service.SettingKeyOpenAICodexTicketHarvestProxyURL])
	rec = doUpdateSettings(t, h, map[string]any{"site_name": "retained"}, nil)
	require.Equal(t, http.StatusOK, rec.Code)
	require.Equal(t, original, repo.values[service.SettingKeyOpenAICodexTicketHarvestProxyURL])
	require.Equal(t, "true", repo.values[service.SettingKeyOpenAICodexTicketEnabled])
	rec = doUpdateSettings(t, h, map[string]any{"openai_codex_ticket_enabled": false, "openai_codex_ticket_harvest_proxy_url": ""}, nil)
	require.Equal(t, http.StatusOK, rec.Code)
	require.Equal(t, "false", repo.values[service.SettingKeyOpenAICodexTicketEnabled])
	require.Empty(t, repo.values[service.SettingKeyOpenAICodexTicketHarvestProxyURL])
}
func TestCodexTicketSettingsInvalidProxy(t *testing.T) {
	h, repo := newStepUpSwitchTestHandler(t, map[string]string{})
	rec := doUpdateSettings(t, h, map[string]any{"openai_codex_ticket_harvest_proxy_url": "file:///secret"}, nil)
	require.Equal(t, http.StatusBadRequest, rec.Code)
	require.Empty(t, repo.values[service.SettingKeyOpenAICodexTicketHarvestProxyURL])
}
func TestCodexTicketSettingsAuditMasksPassword(t *testing.T) {
	proxy := "http://user:ticket-test-secret-123@proxy.example:8080"
	req := settingsAuditRequest(UpdateSettingsRequest{OpenAICodexTicketHarvestProxyURL: &proxy})
	require.NotContains(t, *req.OpenAICodexTicketHarvestProxyURL, "ticket-test-secret-123")
	require.Contains(t, proxy, "ticket-test-secret-123")
}
