package admin

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
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
	shadow := service.NewGPTAutoShadowService(settings, nil, nil, nil)
	t.Cleanup(shadow.Stop)
	h := NewGPTAutoHandler(settings, shadow)
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
	adminGroup.GET("/gpt-auto/classifier", h.GetClassifier)
	adminGroup.PUT("/gpt-auto/classifier", h.UpdateClassifier)
	adminGroup.POST("/gpt-auto/classifier/preview", h.PreviewClassifier)
	adminGroup.POST("/gpt-auto/test/compare", h.CompareClassifier)
	adminGroup.POST("/gpt-auto/debug/state", h.DebugState)
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
	require.Equal(t, http.StatusUnauthorized, gptAutoDo(router, http.MethodGet, "/api/v1/admin/gpt-auto/classifier", nil).Code)
	for _, route := range []struct{ method, path string }{
		{http.MethodPut, "/api/v1/admin/gpt-auto/classifier"},
		{http.MethodPost, "/api/v1/admin/gpt-auto/classifier/preview"},
		{http.MethodPost, "/api/v1/admin/gpt-auto/test/compare"},
		{http.MethodPost, "/api/v1/admin/gpt-auto/debug/state"},
	} {
		require.Equal(t, http.StatusUnauthorized, gptAutoDo(router, route.method, route.path, map[string]any{}).Code, route.path)
	}
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

func gptAutoTestInstructions(prefix string) map[string]any {
	return map[string]any{
		"model_selection":         prefix + " model selection instruction text",
		"effort_selection":        prefix + " effort selection instruction text",
		"complexity_scoring":      prefix + " complexity scoring instruction text",
		"category_classification": prefix + " category classification instruction text",
	}
}

func TestGPTAutoClassifierInstructionsEndpoints(t *testing.T) {
	router, repo := newGPTAutoTestRouter(t, true)

	rec := gptAutoDo(router, http.MethodGet, "/api/v1/admin/gpt-auto/classifier", nil)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	var view struct {
		Data service.GPTAutoClassifierView `json:"data"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &view))
	require.True(t, view.Data.IsDefault)
	require.Len(t, view.Data.Effective.Questions, 4)
	require.Equal(t, service.DefaultGPTAutoClassifierInstructions().ModelSelection, view.Data.Defaults.ModelSelection)

	// Validation is server-side.
	bad := gptAutoTestInstructions("A")
	bad["effort_selection"] = "short"
	require.Equal(t, http.StatusBadRequest, gptAutoDo(router, http.MethodPut, "/api/v1/admin/gpt-auto/classifier", bad).Code)
	tooLong := gptAutoTestInstructions("A")
	tooLong["model_selection"] = strings.Repeat("x", 4001)
	require.Equal(t, http.StatusBadRequest, gptAutoDo(router, http.MethodPut, "/api/v1/admin/gpt-auto/classifier", tooLong).Code)

	// Preview never saves.
	rec = gptAutoDo(router, http.MethodPost, "/api/v1/admin/gpt-auto/classifier/preview", gptAutoTestInstructions("PREVIEW"))
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	require.Contains(t, rec.Body.String(), "PREVIEW model selection")
	require.NotContains(t, repo.values[service.SettingKeyGPTAutoShadowConfig], "PREVIEW")

	rec = gptAutoDo(router, http.MethodPut, "/api/v1/admin/gpt-auto/classifier", gptAutoTestInstructions("SAVED"))
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	require.Contains(t, repo.values[service.SettingKeyGPTAutoShadowConfig], "SAVED model selection")
	require.Contains(t, rec.Body.String(), `"revision":1`)

	// A general config save, even with stale or forged instructions, keeps the
	// saved ones: instructions change only through their own endpoint.
	cfg := service.DefaultGPTAutoShadowConfig()
	cfg.ClassifierInstructions.ModelSelection = "STALE model selection instruction from an old form"
	rec = gptAutoDo(router, http.MethodPut, "/api/v1/admin/gpt-auto/config", cfg)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	stored := repo.values[service.SettingKeyGPTAutoShadowConfig]
	require.Contains(t, stored, "SAVED model selection")
	require.NotContains(t, stored, "STALE")
}

func TestGPTAutoCompareRejectsInvalidDraftWithoutCallingJev(t *testing.T) {
	router, _ := newGPTAutoTestRouter(t, true)
	draft := gptAutoTestInstructions("D")
	draft["category_classification"] = ""
	rec := gptAutoDo(router, http.MethodPost, "/api/v1/admin/gpt-auto/test/compare", map[string]any{"prompt": "hi", "draft_instructions": draft})
	require.Equal(t, http.StatusBadRequest, rec.Code, rec.Body.String())
	// No key configured: refused before any classifier call.
	rec = gptAutoDo(router, http.MethodPost, "/api/v1/admin/gpt-auto/test/compare", map[string]any{"prompt": "hi", "draft_instructions": gptAutoTestInstructions("D")})
	require.Equal(t, http.StatusBadRequest, rec.Code, rec.Body.String())
	require.Contains(t, rec.Body.String(), "GPT_AUTO_CLASSIFIER_NOT_CONFIGURED")
}

func TestGPTAutoDebugStateEndpoint(t *testing.T) {
	router, repo := newGPTAutoTestRouter(t, true)
	request := map[string]any{"model": "gpt-5.6-sol", "messages": []any{
		map[string]any{"role": "system", "content": "PRIVATE operator prompt"},
		map[string]any{"role": "user", "content": "Write a haiku about rain"},
	}}
	rec := gptAutoDo(router, http.MethodPost, "/api/v1/admin/gpt-auto/debug/state", map[string]any{"endpoint": "chat_completions", "request": request})
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	var out struct {
		Data service.GPTAutoDebugResult `json:"data"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &out))
	require.Contains(t, out.Data.JevRequest.State, "Write a haiku about rain")
	require.NotContains(t, out.Data.JevRequest.State, "PRIVATE operator prompt")
	require.Len(t, out.Data.JevRequest.Questions, 4)
	require.Equal(t, "state-v3", out.Data.Context.StateVersion)
	require.Empty(t, repo.values[service.SettingKeyGPTAutoShadowConfig], "nothing is saved")

	rec = gptAutoDo(router, http.MethodPost, "/api/v1/admin/gpt-auto/debug/state", map[string]any{"endpoint": "messages", "request": request})
	require.Equal(t, http.StatusBadRequest, rec.Code)
	rec = gptAutoDo(router, http.MethodPost, "/api/v1/admin/gpt-auto/debug/state", map[string]any{"request": "not an object"})
	require.Equal(t, http.StatusBadRequest, rec.Code)
}
