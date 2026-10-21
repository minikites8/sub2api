package admin

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type gptAutoSettingRepoStub struct {
	mu     sync.Mutex
	values map[string]string
}

func (s *gptAutoSettingRepoStub) Get(_ context.Context, key string) (*service.Setting, error) {
	value, err := s.GetValue(context.Background(), key)
	if err != nil {
		return nil, err
	}
	return &service.Setting{Key: key, Value: value}, nil
}

func (s *gptAutoSettingRepoStub) GetValue(_ context.Context, key string) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	value, ok := s.values[key]
	if !ok {
		return "", service.ErrSettingNotFound
	}
	return value, nil
}

func (s *gptAutoSettingRepoStub) Set(_ context.Context, key, value string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.values[key] = value
	return nil
}

func (s *gptAutoSettingRepoStub) GetMultiple(_ context.Context, keys []string) (map[string]string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := map[string]string{}
	for _, key := range keys {
		if value, ok := s.values[key]; ok {
			out[key] = value
		}
	}
	return out, nil
}
func (s *gptAutoSettingRepoStub) SetMultiple(context.Context, map[string]string) error { return nil }
func (s *gptAutoSettingRepoStub) GetAll(context.Context) (map[string]string, error) {
	return map[string]string{}, nil
}
func (s *gptAutoSettingRepoStub) Delete(context.Context, string) error { return nil }

func newGPTAutoTestRouter(t *testing.T, authorized bool) (*gin.Engine, *gptAutoSettingRepoStub) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	repo := &gptAutoSettingRepoStub{values: map[string]string{}}
	settings := service.NewSettingService(repo, nil)
	h := NewGPTAutoHandler(settings, nil)
	router := gin.New()
	adminGroup := router.Group("/api/v1/admin")
	// Stand-in for the real admin auth middleware mounted by RegisterAdminRoutes.
	adminGroup.Use(func(c *gin.Context) {
		if !authorized {
			c.AbortWithStatus(http.StatusUnauthorized)
			return
		}
		c.Next()
	})
	adminGroup.GET("/gpt-auto/config", h.GetConfig)
	adminGroup.PUT("/gpt-auto/config", h.UpdateConfig)
	adminGroup.GET("/gpt-auto/meta", h.Meta)
	adminGroup.GET("/gpt-auto/decisions/:id", h.GetDecision)
	return router, repo
}

func gptAutoDo(router *gin.Engine, method, path string, body any) *httptest.ResponseRecorder {
	var reader *bytes.Reader
	if body != nil {
		raw, _ := json.Marshal(body)
		reader = bytes.NewReader(raw)
	} else {
		reader = bytes.NewReader(nil)
	}
	req := httptest.NewRequest(method, path, reader)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	return rec
}

func TestGPTAutoAdminRoutesRequireAdmin(t *testing.T) {
	router, _ := newGPTAutoTestRouter(t, false)
	for _, path := range []string{"/api/v1/admin/gpt-auto/config", "/api/v1/admin/gpt-auto/meta"} {
		require.Equal(t, http.StatusUnauthorized, gptAutoDo(router, http.MethodGet, path, nil).Code)
	}
	require.Equal(t, http.StatusUnauthorized, gptAutoDo(router, http.MethodPut, "/api/v1/admin/gpt-auto/config", map[string]any{}).Code)
}

func TestGPTAutoConfigRoundTripMasksKey(t *testing.T) {
	router, repo := newGPTAutoTestRouter(t, true)
	cfg := service.DefaultGPTAutoShadowConfig()
	cfg.ShadowEnabled = true
	cfg.SampleRate = 5
	cfg.ClassifierAPIKey = "ts-secret-key-123456"

	rec := gptAutoDo(router, http.MethodPut, "/api/v1/admin/gpt-auto/config", cfg)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	require.NotContains(t, rec.Body.String(), "ts-secret-key-123456")
	require.Contains(t, rec.Body.String(), `"classifier_api_key_configured":true`)
	require.Contains(t, repo.values[service.SettingKeyGPTAutoShadowConfig], "ts-secret-key-123456")

	// An update without a key keeps the stored key.
	cfg.ClassifierAPIKey = ""
	cfg.SampleRate = 9
	rec = gptAutoDo(router, http.MethodPut, "/api/v1/admin/gpt-auto/config", cfg)
	require.Equal(t, http.StatusOK, rec.Code)
	require.Contains(t, repo.values[service.SettingKeyGPTAutoShadowConfig], "ts-secret-key-123456")

	rec = gptAutoDo(router, http.MethodGet, "/api/v1/admin/gpt-auto/config", nil)
	require.Equal(t, http.StatusOK, rec.Code)
	require.NotContains(t, rec.Body.String(), "ts-secret-key-123456")
	require.Contains(t, rec.Body.String(), `"sample_rate":9`)

	// Explicit clear removes it.
	rec = gptAutoDo(router, http.MethodPut, "/api/v1/admin/gpt-auto/config", map[string]any{
		"shadow_enabled": false, "sample_rate": 0, "all_groups": true,
		"classifier_timeout_ms": 8000, "classifier_max_concurrency": 4, "worker_count": 2, "queue_size": 100,
		"classification_max_context_chars": 4000, "decision_retention_days": 30, "classifier_model": "jev-latest",
		"clear_classifier_api_key": true,
	})
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	require.Contains(t, rec.Body.String(), `"classifier_api_key_configured":false`)
}

func TestGPTAutoConfigRejectsInvalidInput(t *testing.T) {
	router, repo := newGPTAutoTestRouter(t, true)
	for name, mutate := range map[string]func(*service.GPTAutoShadowConfig){
		"sample rate":   func(c *service.GPTAutoShadowConfig) { c.SampleRate = 150 },
		"multiplier":    func(c *service.GPTAutoShadowConfig) { c.ModelProfiles[0].BillingMultiplier = -2 },
		"unknown model": func(c *service.GPTAutoShadowConfig) { c.ModelProfiles[0].Model = "gpt-auto" },
		"bad effort":    func(c *service.GPTAutoShadowConfig) { c.ModelProfiles[3].SupportedEfforts = []string{"none"} },
	} {
		t.Run(name, func(t *testing.T) {
			cfg := service.DefaultGPTAutoShadowConfig()
			mutate(cfg)
			rec := gptAutoDo(router, http.MethodPut, "/api/v1/admin/gpt-auto/config", cfg)
			require.Equal(t, http.StatusBadRequest, rec.Code, rec.Body.String())
			require.Contains(t, rec.Body.String(), "GPT_AUTO_CONFIG_INVALID")
			require.Empty(t, repo.values[service.SettingKeyGPTAutoShadowConfig], "invalid config must not be stored")
		})
	}
	require.Equal(t, http.StatusBadRequest, gptAutoDo(router, http.MethodGet, "/api/v1/admin/gpt-auto/decisions/abc", nil).Code)
}

func TestGPTAutoConfigSecretHandling(t *testing.T) {
	const secret = "ts-secret-key-123456"
	stored := func(repo *gptAutoSettingRepoStub) service.GPTAutoShadowConfig {
		var cfg service.GPTAutoShadowConfig
		require.NoError(t, json.Unmarshal([]byte(repo.values[service.SettingKeyGPTAutoShadowConfig]), &cfg))
		return cfg
	}
	router, repo := newGPTAutoTestRouter(t, true)
	cfg := service.DefaultGPTAutoShadowConfig()
	cfg.ClassifierAPIKey = secret
	require.Equal(t, http.StatusOK, gptAutoDo(router, http.MethodPut, "/api/v1/admin/gpt-auto/config", cfg).Code)

	// A client echoing the masked hint back must not overwrite the key.
	cfg.ClassifierAPIKey = "ts-s****3456"
	cfg.SampleRate = 3
	rec := gptAutoDo(router, http.MethodPut, "/api/v1/admin/gpt-auto/config", cfg)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	require.Equal(t, secret, stored(repo).ClassifierAPIKey)
	require.Equal(t, 3, stored(repo).SampleRate, "unrelated fields still update")
	require.NotContains(t, rec.Body.String(), secret)

	// Any other masked-looking value is rejected rather than stored.
	cfg.ClassifierAPIKey = "abcd****wxyz"
	rec = gptAutoDo(router, http.MethodPut, "/api/v1/admin/gpt-auto/config", cfg)
	require.Equal(t, http.StatusBadRequest, rec.Code)
	require.Equal(t, secret, stored(repo).ClassifierAPIKey)

	// The stored key must not silently follow a new classifier host.
	cfg.ClassifierAPIKey = ""
	cfg.ClassifierBaseURL = "https://classifier.example.org"
	rec = gptAutoDo(router, http.MethodPut, "/api/v1/admin/gpt-auto/config", cfg)
	require.Equal(t, http.StatusBadRequest, rec.Code, rec.Body.String())
	require.Equal(t, "https://api.typesafe.ai", stored(repo).ClassifierBaseURL)

	// Changing the host together with a freshly entered key is allowed.
	cfg.ClassifierAPIKey = "ts-new-key-abcdefgh"
	rec = gptAutoDo(router, http.MethodPut, "/api/v1/admin/gpt-auto/config", cfg)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	require.Equal(t, "ts-new-key-abcdefgh", stored(repo).ClassifierAPIKey)
	require.NotContains(t, rec.Body.String(), "ts-new-key-abcdefgh")
}

func TestGPTAutoConfigRejectsGatewayAsClassifier(t *testing.T) {
	router, repo := newGPTAutoTestRouter(t, true)
	repo.values[service.SettingKeyAPIBaseURL] = "https://api.gateway.example.com/v1"
	cfg := service.DefaultGPTAutoShadowConfig()
	cfg.ClassifierAPIKey = "sk-customer-billable-key"
	cfg.ClassifierBaseURL = "http://API.gateway.example.com:443"
	rec := gptAutoDo(router, http.MethodPut, "/api/v1/admin/gpt-auto/config", cfg)
	require.Equal(t, http.StatusBadRequest, rec.Code, rec.Body.String())
	require.Empty(t, repo.values[service.SettingKeyGPTAutoShadowConfig])
}

func TestGPTAutoMetaReportsShadowPhase(t *testing.T) {
	router, _ := newGPTAutoTestRouter(t, true)
	rec := gptAutoDo(router, http.MethodGet, "/api/v1/admin/gpt-auto/meta", nil)
	require.Equal(t, http.StatusOK, rec.Code)
	require.Contains(t, rec.Body.String(), `"live_routing_active":false`)
	require.Contains(t, rec.Body.String(), "gpt-6.1-sol")
}
