//go:build unit

package admin

import (
	"context"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestSettingsCodexTicketProxyWriteReadAndHotReload(t *testing.T) {
	key := service.SettingKeyOpenAICodexTicketHarvestProxyURL
	oldProxy := "http://user:old-secret@old.example.com:8080"
	newProxy := "socks5h://user:new-secret@new.example.com:1080"
	h, repo := newStepUpSwitchTestHandler(t, map[string]string{key: oldProxy})
	require.Equal(t, oldProxy, h.settingService.GetOpenAICodexTicketHarvestProxyURL(context.Background()))
	rec := doUpdateSettings(t, h, map[string]any{key: newProxy}, nil)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	require.Equal(t, newProxy, repo.values[key])
	require.Equal(t, newProxy, h.settingService.GetOpenAICodexTicketHarvestProxyURL(context.Background()))
	require.NotContains(t, rec.Body.String(), "new-secret")
	require.Contains(t, rec.Body.String(), `"openai_codex_ticket_harvest_proxy_configured":true`)
	// Omission, empty input and the masked GET value all preserve the real secret.
	for _, body := range []map[string]any{{"site_name": "updated"}, {key: ""}, {key: service.MaskProxyURL(newProxy)}} {
		rec = doUpdateSettings(t, h, body, nil)
		require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
		require.Equal(t, newProxy, repo.values[key])
	}
	get := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(get)
	c.Request = httptest.NewRequest(http.MethodGet, "/api/v1/admin/settings", nil)
	h.GetSettings(c)
	require.Equal(t, http.StatusOK, get.Code)
	require.NotContains(t, get.Body.String(), "new-secret")
	require.Contains(t, get.Body.String(), "new.example.com")
}

func TestSettingsCodexTicketRejectInvalidProxyWithoutLeakingPassword(t *testing.T) {
	key := service.SettingKeyOpenAICodexTicketHarvestProxyURL
	h, repo := newStepUpSwitchTestHandler(t, map[string]string{key: "http://previous.example.com:8080"})
	rec := doUpdateSettings(t, h, map[string]any{key: "ftp://user:invalid-secret@proxy.example.com:21"}, nil)
	require.Equal(t, http.StatusBadRequest, rec.Code, rec.Body.String())
	require.NotContains(t, rec.Body.String(), "invalid-secret")
	require.Equal(t, "http://previous.example.com:8080", repo.values[key])
}

func TestSettingsCodexTicketModelsPersistOmissionAndEmpty(t *testing.T) {
	key := service.SettingKeyOpenAICodexTicketModels
	h, repo := newStepUpSwitchTestHandler(t, map[string]string{key: `["gpt-6-astra","gpt-5.6-sol"]`})
	ctx := context.Background()
	require.Len(t, h.settingService.GetOpenAICodexTicketModels(ctx, nil), 2)
	for _, models := range [][]string{{"gpt-6-astra"}, {}} {
		rec := doUpdateSettings(t, h, map[string]any{key: models}, nil)
		require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
		require.Equal(t, models, h.settingService.GetOpenAICodexTicketModels(ctx, nil))
		saved := repo.values[key]
		rec = doUpdateSettings(t, h, map[string]any{"site_name": "updated"}, nil)
		require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
		require.Equal(t, saved, repo.values[key])
	}
}

func TestSettingsCodexTicketRestoresStaticProxyAfterKernel(t *testing.T) {
	key := service.SettingKeyOpenAICodexTicketHarvestProxyURL
	original := "http://user:secret@residential.example:8080"
	h, repo := newStepUpSwitchTestHandler(t, map[string]string{key: original})
	rec := doUpdateSettings(t, h, map[string]any{key: "http://127.0.0.1:3101"}, nil)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	require.Equal(t, original, repo.values[service.SettingKeyOpenAICodexTicketStaticProxyURL])
	require.NotContains(t, rec.Body.String(), ":secret@")
	rec = doUpdateSettings(t, h, map[string]any{key: service.MaskProxyURL(original), "openai_codex_ticket_use_saved_static_proxy": true}, nil)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	require.Equal(t, original, repo.values[key])
}

func TestSettingsCodexTicketIPPoolRoundTripKeepsStaticProxy(t *testing.T) {
	key := service.SettingKeyOpenAICodexTicketHarvestProxyURL
	original := "http://user:secret@residential.example:8080"
	h, repo := newStepUpSwitchTestHandler(t, map[string]string{key: original})
	rec := doUpdateSettings(t, h, map[string]any{key: service.OpenAICodexTicketHarvestIPPoolURL}, nil)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	require.Equal(t, service.OpenAICodexTicketHarvestIPPoolURL, repo.values[key])
	require.Equal(t, original, repo.values[service.SettingKeyOpenAICodexTicketStaticProxyURL])
	require.Contains(t, rec.Body.String(), `"openai_codex_ticket_harvest_proxy_url":"ippool://active"`)
	require.Equal(t, service.OpenAICodexTicketHarvestIPPoolURL, h.settingService.GetOpenAICodexTicketHarvestProxyURL(context.Background()))

	// Saving again in pool mode must not turn the sentinel into the static memory.
	rec = doUpdateSettings(t, h, map[string]any{key: service.OpenAICodexTicketHarvestIPPoolURL}, nil)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	require.Equal(t, original, repo.values[service.SettingKeyOpenAICodexTicketStaticProxyURL])

	get := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(get)
	c.Request = httptest.NewRequest(http.MethodGet, "/api/v1/admin/settings", nil)
	h.GetSettings(c)
	require.Equal(t, http.StatusOK, get.Code)
	require.Contains(t, get.Body.String(), `"openai_codex_ticket_harvest_proxy_url":"ippool://active"`)
	require.NotContains(t, get.Body.String(), ":secret@")

	rec = doUpdateSettings(t, h, map[string]any{key: service.MaskProxyURL(original), "openai_codex_ticket_use_saved_static_proxy": true}, nil)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	require.Equal(t, original, repo.values[key])
}

func TestSettingsCodexTicketStrategyRoundTrip(t *testing.T) {
	h, repo := newStepUpSwitchTestHandler(t, map[string]string{})
	rec := doUpdateSettings(t, h, map[string]any{"openai_codex_ticket_enabled": true, "openai_codex_ticket_harvest_proxy_url": "http://user:ticket-test-secret-123@proxy.example:8080", "openai_codex_ticket_models": []string{"a", "b", "a"}}, nil)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	require.Equal(t, "true", repo.values[service.SettingKeyOpenAICodexTicketEnabled])
	require.Equal(t, `["a","b"]`, repo.values[service.SettingKeyOpenAICodexTicketModels])
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
	require.Equal(t, original, repo.values[service.SettingKeyOpenAICodexTicketHarvestProxyURL])
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
