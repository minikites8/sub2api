package admin

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

// gptAutoExportRepoStub implements only what export needs; any other call
// panics through the nil embedded interface, which would fail the test.
type gptAutoExportRepoStub struct {
	service.GPTAutoShadowRepository
	mu      sync.Mutex
	rows    []service.GPTAutoShadowDecision
	filters []service.GPTAutoExportFilter
	audits  []service.GPTAutoExportAudit
}

func (r *gptAutoExportRepoStub) ExportDecisions(_ context.Context, filter service.GPTAutoExportFilter, cursor *time.Time, _ int64, limit int) ([]service.GPTAutoShadowDecision, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.filters = append(r.filters, filter)
	if cursor != nil {
		return nil, nil
	}
	if len(r.rows) > limit {
		return r.rows[:limit], nil
	}
	return r.rows, nil
}

func (r *gptAutoExportRepoStub) CountDecisions(context.Context, service.GPTAutoExportFilter, int64) (int64, error) {
	return int64(len(r.rows)), nil
}

func (r *gptAutoExportRepoStub) ExportUsage(context.Context, []service.GPTAutoExportUsageKey, time.Time, time.Time) (map[service.GPTAutoExportUsageKey]*service.GPTAutoExportUsage, error) {
	return nil, nil
}

func (r *gptAutoExportRepoStub) InsertExportAudit(_ context.Context, audit service.GPTAutoExportAudit) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.audits = append(r.audits, audit)
	return nil
}

func newGPTAutoExportRouter(t *testing.T) (*gin.Engine, *gptAutoExportRepoStub) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	settings := service.NewSettingService(&gptAutoSettingRepoStub{values: map[string]string{}}, nil)
	repo := &gptAutoExportRepoStub{rows: []service.GPTAutoShadowDecision{{ID: 1, CreatedAt: time.Now().UTC().Add(-time.Minute), Mode: service.GPTAutoModeLive,
		RequestID: "@cmd", ActualModel: service.GPTAutoVirtualModel, FinalModel: service.GPTAutoModelGPT6Luna}}}
	shadow := service.NewGPTAutoShadowService(settings, repo, nil, nil)
	t.Cleanup(shadow.Stop)
	h := NewGPTAutoHandler(settings, shadow, nil)
	router := gin.New()
	router.GET("/export", h.ExportDecisions)
	router.GET("/export/count", h.CountExport)
	return router, repo
}

func TestGPTAutoAdminExportEndpoint(t *testing.T) {
	router, repo := newGPTAutoExportRouter(t)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/export?format=csv&mode=live&final_model=gpt-6-luna&complexity_min=1.5&fallback_only=true", nil))
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	require.Equal(t, "text/csv; charset=utf-8", rec.Header().Get("Content-Type"))
	disposition := rec.Header().Get("Content-Disposition")
	require.True(t, strings.HasPrefix(disposition, `attachment; filename="gpt-auto-routing-admin-`))
	require.True(t, strings.HasSuffix(disposition, `.csv"`))
	require.Equal(t, service.GPTAutoExportSchemaVersion, rec.Header().Get("X-GPT-Auto-Export-Schema"))
	require.Contains(t, rec.Body.String(), "'@cmd", "formula-like values are neutralised")
	require.Len(t, repo.filters, 1)
	filter := repo.filters[0]
	require.Equal(t, service.GPTAutoModeLive, filter.Mode)
	require.Equal(t, service.GPTAutoModelGPT6Luna, filter.FinalModel)
	require.True(t, filter.FallbackOnly)
	require.InDelta(t, 1.5, *filter.ComplexityMin, 1e-9)
	require.Nil(t, filter.UserID, "admin scope is not narrowed to a user")
	require.Len(t, repo.audits, 1)

	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/export?format=jsonl", nil))
	require.Equal(t, http.StatusOK, rec.Code)
	require.Equal(t, "application/x-ndjson; charset=utf-8", rec.Header().Get("Content-Type"))
	require.Contains(t, rec.Body.String(), `"request_id":"@cmd"`)

	for _, query := range []string{"format=xlsx", "mode=both", "complexity_min=abc", "start_time=yesterday", "upstream_account_id=-1"} {
		rec = httptest.NewRecorder()
		router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/export?"+query, nil))
		require.Equal(t, http.StatusBadRequest, rec.Code, query)
		require.Contains(t, rec.Header().Get("Content-Type"), "application/json", "validation errors are JSON, not a broken file")
	}

	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/export/count?mode=shadow", nil))
	require.Equal(t, http.StatusOK, rec.Code)
	require.Contains(t, rec.Body.String(), `"rows":1`)
}
