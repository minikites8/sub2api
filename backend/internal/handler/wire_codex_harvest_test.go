//go:build unit

package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/handler/admin"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type harvestWiringSettingsRepo struct {
	service.SettingRepository
	values map[string]string
}

func (r *harvestWiringSettingsRepo) GetValue(_ context.Context, key string) (string, error) {
	value, ok := r.values[key]
	if !ok {
		return "", service.ErrSettingNotFound
	}
	return value, nil
}

func (r *harvestWiringSettingsRepo) Set(_ context.Context, key, value string) error {
	r.values[key] = value
	return nil
}

type harvestWiringNodesRepo struct {
	service.CodexHarvestNodeRepository
	resets []int64
}

func (r *harvestWiringNodesRepo) List(context.Context, int, int) (service.CodexHarvestNodePage, error) {
	return service.CodexHarvestNodePage{Items: []service.CodexHarvestNodeRecord{}}, nil
}

func (r *harvestWiringNodesRepo) Reset(_ context.Context, id int64) error {
	r.resets = append(r.resets, id)
	return nil
}

func TestProvideAdminHandlers_CodexHarvestRuntime(t *testing.T) {
	gin.SetMode(gin.TestMode)
	repo := &harvestWiringSettingsRepo{values: map[string]string{
		service.SettingKeyOpenAICodexRelayURL:              "https://relay.example",
		service.SettingKeyOpenAICodexRelayKey:              "relay-secret",
		service.SettingKeyOpenAICodexTicketHarvestProxyURL: "socks5h://harvest.example:1080",
	}}
	cfg := &config.Config{}
	settings := service.NewSettingService(repo, cfg)
	nodes := &harvestWiringNodesRepo{}
	harvest := service.NewCodexHarvestService(nodes, repo, cfg)
	gateway := &service.OpenAIGatewayService{}
	account := &admin.AccountHandler{}
	handlers := ProvideAdminHandlers(
		nil,      // requestCaptureHandler
		nil,      // dashboardHandler
		nil,      // userHandler
		nil,      // groupHandler
		account,  // accountHandler
		nil,      // announcementHandler
		nil,      // dataManagementHandler
		nil,      // backupHandler
		nil,      // oauthHandler
		nil,      // openaiOAuthHandler
		nil,      // openaiOAuthReauthHandler
		nil,      // geminiOAuthHandler
		nil,      // antigravityOAuthHandler
		nil,      // kiroOAuthHandler
		nil,      // grokOAuthHandler
		nil,      // cnProviderHandler
		nil,      // proxyHandler
		nil,      // redeemHandler
		nil,      // promoHandler
		nil,      // settingHandler
		nil,      // opsHandler
		nil,      // systemHandler
		nil,      // subscriptionHandler
		nil,      // usageHandler
		nil,      // dailyCheckinHandler
		nil,      // userAttributeHandler
		nil,      // errorPassthroughHandler
		nil,      // promptRuleHandler
		nil,      // tlsFingerprintProfileHandler
		nil,      // pluginHandler
		nil,      // apiKeyHandler
		nil,      // scheduledTestHandler
		nil,      // pelicanGroupTestHandler
		nil,      // controlledExperimentHandler
		nil,      // accountOpsHandler
		nil,      // accountTokenGuardHandler
		nil,      // accountTokenGuardV2Handler
		nil,      // channelHandler
		nil,      // channelMonitorHandler
		nil,      // channelMonitorTemplateHandler
		nil,      // contentModerationHandler
		nil,      // promptAuditHandler
		nil,      // paymentHandler
		nil,      // affiliateHandler
		nil,      // complianceHandler
		nil,      // auditLogHandler
		nil,      // upstreamBillingProbe
		nil,      // ollamaCloudUsage
		nil,      // opencodeGoUsage
		nil,      // yeTeamClient
		gateway,  // openAIGateway
		harvest,  // codexHarvest
		settings, // settingService
	)
	require.Same(t, account, handlers.Account)
	router := gin.New()
	router.GET("/controls", handlers.Account.GetCodexHarvestControls)
	router.PUT("/controls", handlers.Account.UpdateCodexHarvestControls)
	router.GET("/nodes", handlers.Account.GetCodexHarvestNodes)
	router.POST("/reset", handlers.Account.ResetCodexHarvestNodes)
	router.GET("/flow", handlers.Account.GetCodexHarvestFlow)
	router.POST("/:id/manual-harvest", handlers.Account.ManualCodexHarvest)
	request := func(method, path, body string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(method, path, strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		out := httptest.NewRecorder()
		router.ServeHTTP(out, req)
		return out
	}

	// Use the production factory to exercise all three runtime dependencies.
	out := request(http.MethodGet, "/controls?timezone=Asia%2FShanghai", "")
	require.Equal(t, http.StatusOK, out.Code, out.Body.String())
	var snapshot struct {
		Data service.CodexHarvestControlSnapshot `json:"data"`
	}
	require.NoError(t, json.Unmarshal(out.Body.Bytes(), &snapshot))
	require.True(t, snapshot.Data.Available, "saved Relay settings reach the control snapshot")

	controls := snapshot.Data.Settings
	controls.MintMode = "local"
	controls.NodeMemoryEnabled = false
	payload, err := json.Marshal(controls)
	require.NoError(t, err)
	out = request(http.MethodPut, "/controls", string(payload))
	require.Equal(t, http.StatusOK, out.Code, out.Body.String())
	out = request(http.MethodGet, "/controls", "")
	require.Equal(t, http.StatusOK, out.Code, out.Body.String())
	require.NoError(t, json.Unmarshal(out.Body.Bytes(), &snapshot))
	require.Equal(t, "local", snapshot.Data.Settings.MintMode)
	require.True(t, snapshot.Data.Available, "saved local proxy reaches the control snapshot")

	require.Equal(t, http.StatusOK, request(http.MethodGet, "/nodes", "").Code)
	require.Equal(t, http.StatusOK, request(http.MethodPost, "/reset", `{"record_id":7}`).Code)
	require.Equal(t, []int64{7}, nodes.resets)
	out = request(http.MethodGet, "/flow", "")
	require.Equal(t, http.StatusOK, out.Code, out.Body.String())
	var flow struct {
		Data service.CodexHarvestFlowSnapshot `json:"data"`
	}
	require.NoError(t, json.Unmarshal(out.Body.Bytes(), &flow))
	require.Equal(t, "socks5h://harvest.example:1080", flow.Data.Harvest.HarvestProxy)
	require.NotNil(t, flow.Data.Runtime)

	// Reaching validation confirms the manual endpoint received its gateway.
	out = request(http.MethodPost, "/invalid/manual-harvest", `{}`)
	require.Equal(t, http.StatusBadRequest, out.Code, out.Body.String())
}
