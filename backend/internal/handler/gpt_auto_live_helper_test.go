package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/openai"
	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
	middleware2 "github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type gptAutoUserSettingRepo struct {
	mu     sync.Mutex
	values map[string]string
}

func (s *gptAutoUserSettingRepo) Get(ctx context.Context, key string) (*service.Setting, error) {
	value, err := s.GetValue(ctx, key)
	if err != nil {
		return nil, err
	}
	return &service.Setting{Key: key, Value: value}, nil
}
func (s *gptAutoUserSettingRepo) GetValue(_ context.Context, key string) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	value, ok := s.values[key]
	if !ok {
		return "", service.ErrSettingNotFound
	}
	return value, nil
}
func (s *gptAutoUserSettingRepo) Set(_ context.Context, key, value string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.values[key] = value
	return nil
}
func (s *gptAutoUserSettingRepo) GetMultiple(context.Context, []string) (map[string]string, error) {
	return map[string]string{}, nil
}
func (s *gptAutoUserSettingRepo) SetMultiple(context.Context, map[string]string) error { return nil }
func (s *gptAutoUserSettingRepo) GetAll(context.Context) (map[string]string, error) {
	return map[string]string{}, nil
}
func (s *gptAutoUserSettingRepo) Delete(context.Context, string) error { return nil }

// gptAutoUserRepoStub records the filters the customer handler sends.
type gptAutoUserRepoStub struct {
	service.GPTAutoShadowRepository
	mu            sync.Mutex
	listFilters   []service.GPTAutoDecisionFilter
	exportFilters []service.GPTAutoExportFilter
}

func (r *gptAutoUserRepoStub) List(_ context.Context, filter service.GPTAutoDecisionFilter) ([]service.GPTAutoShadowDecision, *pagination.PaginationResult, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.listFilters = append(r.listFilters, filter)
	upstream := int64(42)
	return []service.GPTAutoShadowDecision{{RequestID: "own", Mode: service.GPTAutoModeLive, ActualModel: service.GPTAutoVirtualModel,
		FinalModel: service.GPTAutoModelGPT6Luna, RoutingReason: service.GPTAutoRouteFallback, FallbackReason: service.GPTAutoErrAuth,
		UpstreamAccountID: &upstream, ClassifierCost: 0.5}}, &pagination.PaginationResult{Total: 1, Page: 1, PageSize: 20}, nil
}
func (r *gptAutoUserRepoStub) ExportDecisions(_ context.Context, filter service.GPTAutoExportFilter, _ *time.Time, _ int64, _ int) ([]service.GPTAutoShadowDecision, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.exportFilters = append(r.exportFilters, filter)
	return nil, nil
}
func (r *gptAutoUserRepoStub) CountDecisions(context.Context, service.GPTAutoExportFilter, int64) (int64, error) {
	return 0, nil
}
func (r *gptAutoUserRepoStub) ExportUsage(context.Context, []service.GPTAutoExportUsageKey, time.Time, time.Time) (map[service.GPTAutoExportUsageKey]*service.GPTAutoExportUsage, error) {
	return nil, nil
}
func (r *gptAutoUserRepoStub) InsertExportAudit(context.Context, service.GPTAutoExportAudit) error {
	return nil
}

func newGPTAutoUserRouter(t *testing.T, userID int64) (*gin.Engine, *gptAutoUserRepoStub) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	settings := service.NewSettingService(&gptAutoUserSettingRepo{values: map[string]string{}}, nil)
	repo := &gptAutoUserRepoStub{}
	svc := service.NewGPTAutoShadowService(settings, repo, nil, nil)
	t.Cleanup(svc.Stop)
	h := NewGPTAutoUserHandler(svc)
	router := gin.New()
	router.Use(func(c *gin.Context) {
		if userID > 0 {
			c.Set(string(middleware2.ContextKeyUser), middleware2.AuthSubject{UserID: userID})
		}
		c.Next()
	})
	router.GET("/logs", h.List)
	router.GET("/export", h.Export)
	router.GET("/status", h.Status)
	return router, repo
}

func TestGPTAutoCustomerEndpointsScopeToAuthenticatedUser(t *testing.T) {
	router, repo := newGPTAutoUserRouter(t, 11)
	// Every attempt to name another owner is ignored.
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/logs?user_id=99&api_key_id=5&mode=shadow", nil))
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	require.Len(t, repo.listFilters, 1)
	require.Equal(t, int64(11), *repo.listFilters[0].UserID)
	require.Equal(t, service.GPTAutoModeLive, repo.listFilters[0].Mode)
	body := rec.Body.String()
	for _, forbidden := range []string{"upstream_account", "classifier_cost", "fallback_reason", `"auth"`, "user_id"} {
		require.NotContains(t, body, forbidden)
	}
	require.Contains(t, body, `"fallback":true`)

	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/export?format=jsonl&user_id=99&mode=shadow", nil))
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	require.Len(t, repo.exportFilters, 1)
	require.Equal(t, int64(11), *repo.exportFilters[0].UserID)
	require.Equal(t, service.GPTAutoModeLive, repo.exportFilters[0].Mode)
	require.Contains(t, rec.Header().Get("Content-Disposition"), "gpt-auto-routing-customer-")

	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/logs?start_time=2026-01-01T00:00:00Z&end_time=2026-10-01T00:00:00Z", nil))
	require.Equal(t, http.StatusBadRequest, rec.Code, "customer windows are bounded")
}

func TestGPTAutoCustomerEndpointsRequireAuthentication(t *testing.T) {
	router, repo := newGPTAutoUserRouter(t, 0)
	for _, path := range []string{"/logs", "/export", "/status"} {
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
		require.Equal(t, http.StatusUnauthorized, rec.Code, path)
	}
	require.Empty(t, repo.listFilters)
	require.Empty(t, repo.exportFilters)
}

func TestGPTAutoModelPermitted(t *testing.T) {
	key := &service.APIKey{ID: 1}
	require.True(t, gptAutoModelPermitted(key)(service.GPTAutoModelGPT61Sol), "no allowlist, no denials")
}

func TestGPTAutoVirtualModelListing(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	models := []openai.Model{{ID: "gpt-6-luna"}}
	require.Equal(t, models, withGPTAutoVirtualModel(c, models), "not listed unless admitted")
	c.Set(gptAutoListKey, true)
	listed := withGPTAutoVirtualModel(c, models).([]openai.Model)
	require.Len(t, listed, 2)
	require.Equal(t, service.GPTAutoVirtualModel, listed[1].ID)
	require.Len(t, models, 1, "the caller's slice is not mutated")
	require.Len(t, withGPTAutoVirtualModel(c, listed).([]openai.Model), 2, "never listed twice")
}

func TestResolveGPTAutoLivePassesThroughOtherModels(t *testing.T) {
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	body := []byte(`{"model":"gpt-6-luna"}`)
	out, model, route, ok := resolveGPTAutoLive(c, nil, &service.APIKey{ID: 1}, service.GPTAutoEndpointChatCompletions, body, "gpt-6-luna")
	require.True(t, ok)
	require.Nil(t, route)
	require.Equal(t, "gpt-6-luna", model)
	require.Equal(t, body, out)
	require.False(t, c.Writer.Written())

	// gpt-auto with live routing unavailable answers like an unknown model.
	_, _, _, ok = resolveGPTAutoLive(c, nil, &service.APIKey{ID: 1}, service.GPTAutoEndpointChatCompletions, []byte(`{"model":"gpt-auto"}`), "gpt-auto")
	require.False(t, ok)
	require.Equal(t, http.StatusNotFound, rec.Code)
	var payload map[string]map[string]string
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &payload))
	require.Equal(t, "model_not_found", payload["error"]["code"])
	require.Equal(t, "invalid_request_error", payload["error"]["type"])
}
