package admin

import (
	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
)

// GetAutoSupplySettings returns the masked automatic account supply settings.
// GET /api/v1/admin/settings/auto-supply
func (h *SettingHandler) GetAutoSupplySettings(c *gin.Context) {
	if h.autoSupplyService == nil {
		response.InternalError(c, "Automatic supply service is unavailable")
		return
	}
	settings, err := h.autoSupplyService.GetSettings(c.Request.Context())
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, settings)
}

// GetAutoSupplyOrders returns the latest automatic replenishment orders.
// GET /api/v1/admin/settings/auto-supply/orders
func (h *SettingHandler) GetAutoSupplyOrders(c *gin.Context) {
	if h.autoSupplyService == nil {
		response.InternalError(c, "Automatic supply service is unavailable")
		return
	}
	orders, err := h.autoSupplyService.ListOrders(c.Request.Context())
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, orders)
}

// UpdateAutoSupplySettings persists and immediately applies automatic account supply settings.
// PUT /api/v1/admin/settings/auto-supply
func (h *SettingHandler) UpdateAutoSupplySettings(c *gin.Context) {
	if h.autoSupplyService == nil {
		response.InternalError(c, "Automatic supply service is unavailable")
		return
	}
	var input service.AutoSupplySettingsUpdate
	if err := c.ShouldBindJSON(&input); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}
	settings, err := h.autoSupplyService.UpdateSettings(c.Request.Context(), input)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, settings)
}
