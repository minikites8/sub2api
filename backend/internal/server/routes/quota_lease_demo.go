package routes

import (
	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/handler"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
)

func RegisterQuotaLeaseDemoRoutes(v1 *gin.RouterGroup, cfg *config.Config, apiKeyService *service.APIKeyService, usageService *service.UsageService, adminSvc ...service.AdminService) {
	h := handler.NewQuotaLeaseDemoHandler(service.GetQuotaLeaseDemoService(cfg), adminSvc...)
	h.SetAPIKeyService(apiKeyService)
	h.SetUsageService(usageService)
	group := v1.Group("/node-leases/demo")
	{
		group.POST("/nodes/register", h.RegisterNode)
		group.POST("/nodes/heartbeat", h.HeartbeatNode)
		group.GET("/nodes", h.ListNodes)
		group.POST("/auth/client-key", h.AuthorizeClientKey)
		group.POST("/accounts/login-tasks", h.CreateAccountLoginTask)
		group.GET("/accounts/login-tasks", h.ListAccountLoginTasks)
		group.POST("/accounts/login-tasks/:task_id/complete", h.CompleteAccountLoginTask)
		group.POST("/accounts/login-tasks/:task_id/progress", h.ReportAccountLoginTaskProgress)
		group.POST("/accounts/login-tasks/:task_id/callback", h.SubmitAccountLoginTaskCallback)
		group.POST("/accounts/status", h.ReportAccountStatus)
		group.GET("/accounts/assignments", h.ListAssignedAccounts)
		group.POST("/leases/request", h.RequestLease)
		group.POST("/usage/batch", h.PostUsageBatch)
		group.POST("/usage-logs/batch", h.PostUsageLogBatch)
		group.POST("/reclaim", h.ReclaimExpired)
		group.GET("/status", h.Status)
	}
}
