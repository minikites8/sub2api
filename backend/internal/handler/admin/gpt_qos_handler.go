package admin

import (
	"context"
	"net/http"
	"strconv"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
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
	snapshot := h.openAIGatewayService.GPTQoSSnapshotFiltered(c.Request.Context(), service.GPTQoSMetricFilter{GroupID: groupID, Model: c.Query("model"), Effort: c.Query("effort")})
	h.labelGPTQoSGroups(c.Request.Context(), snapshot)
	response.Success(c, snapshot)
}

func (h *AccountHandler) labelGPTQoSGroups(ctx context.Context, snapshot map[string]any) {
	rows, _ := snapshot["groups"].(map[string][]service.GPTQoSGroupMetrics)
	if len(rows) == 0 || h.adminService == nil {
		return
	}
	// One metadata read per admin snapshot, never per model request or group.
	// Disabled groups still have historical requests; deleted groups keep IDs.
	readCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	groups, err := h.adminService.GetAllGroupsIncludingInactive(readCtx)
	if err != nil {
		return
	}
	names := make(map[int64]string, len(groups))
	for _, group := range groups {
		names[group.ID] = group.Name
	}
	for _, tierRows := range rows {
		for i := range tierRows {
			tierRows[i].GroupName = names[tierRows[i].GroupID]
		}
	}
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
