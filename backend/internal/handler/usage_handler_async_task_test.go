package handler

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	middleware2 "github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type usageDetailLogRepo struct {
	service.UsageLogRepository
	log *service.UsageLog
}

func (r *usageDetailLogRepo) GetByID(_ context.Context, _ int64) (*service.UsageLog, error) {
	return r.log, nil
}

type usageDetailBaiduRepo struct {
	service.BaiduVODVideoTaskRepository
	task *service.BaiduVODVideoTask
}

func (r *usageDetailBaiduRepo) GetForOwner(_ context.Context, _, _ int64, _ string) (*service.BaiduVODVideoTask, error) {
	return r.task, nil
}

func TestUsageGetByIDIncludesAsyncTaskDetails(t *testing.T) {
	gin.SetMode(gin.TestMode)
	resultURL := "https://cdn.example.com/video.mp4"
	usageRepo := &usageDetailLogRepo{log: &service.UsageLog{
		ID:          1,
		UserID:      42,
		APIKeyID:    7,
		RequestID:   "baidu_vod_capture:video-task-1",
		Model:       "veo-3",
		RequestType: service.RequestTypeAsync,
	}}
	usageService := service.NewUsageService(usageRepo, nil, nil, nil)
	asyncTasks := service.NewUsageAsyncTaskService(&usageDetailBaiduRepo{task: &service.BaiduVODVideoTask{
		TaskID:    "video-task-1",
		Status:    service.BaiduVODTaskStatusCompleted,
		ResultURL: &resultURL,
	}}, nil)
	handler := NewUsageHandler(usageService, nil, nil, nil)
	handler.asyncTaskService = asyncTasks

	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set(string(middleware2.ContextKeyUser), middleware2.AuthSubject{UserID: 42})
		c.Next()
	})
	router.GET("/usage/:id", handler.GetByID)

	req := httptest.NewRequest(http.MethodGet, "/usage/1", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	require.Contains(t, rec.Body.String(), "\"request_type\":\"async\"")
	require.Contains(t, rec.Body.String(), "\"async_task\"")
	require.Contains(t, rec.Body.String(), "\"task_id\":\"video-task-1\"")
	require.Contains(t, rec.Body.String(), "\"result_urls\":[\"https://cdn.example.com/video.mp4\"]")
}
