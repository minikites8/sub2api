package admin

import (
	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"net/http"
	"strconv"
)

func (h *SettingHandler) GetGPTQoSConfig(c *gin.Context) {
	cfg, err := h.settingService.GetGPTQoSConfig(c.Request.Context())
	if err != nil {
		response.Error(c, http.StatusServiceUnavailable, "QoS configuration unavailable")
		return
	}
	response.Success(c, cfg)
}
func (h *SettingHandler) SaveGPTQoSConfig(c *gin.Context) {
	cfg := service.DefaultGPTQoSConfig()
	if c.ShouldBindJSON(&cfg) != nil {
		response.BadRequest(c, "Invalid QoS configuration")
		return
	}
	if err := service.ValidateGPTQoSConfig(cfg); err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	if err := h.settingService.SaveGPTQoSConfig(c.Request.Context(), cfg); err != nil {
		response.Error(c, 503, "Could not save QoS configuration")
		return
	}
	stored, err := h.settingService.GetGPTQoSConfig(c.Request.Context())
	if err != nil {
		response.Error(c, 503, "Could not read QoS configuration")
		return
	}
	response.Success(c, stored)
}
func (h *AccountHandler) GPTQoSSnapshot(c *gin.Context) {
	groupID, _ := strconv.ParseInt(c.Query("group_id"), 10, 64)
	response.Success(c, h.openAIGatewayService.GPTQoSSnapshotFiltered(c.Request.Context(), service.GPTQoSMetricFilter{GroupID: groupID, Model: c.Query("model"), Effort: c.Query("effort")}))
}

func mergeQualityRequestExtra(extra map[string]any, tier, pool *string) map[string]any {
	if tier == nil && pool == nil {
		return extra
	}
	out := map[string]any{}
	for k, v := range extra {
		out[k] = v
	}
	if tier != nil {
		out[service.AccountQualityTierKey] = *tier
	}
	if pool != nil {
		out[service.AccountResourcePoolKey] = *pool
	}
	return out
}
