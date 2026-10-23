package handler

import (
	"net/http"

	"github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
)

type CCSwitchUsageHandler struct{ keys *service.APIKeyService }

func NewCCSwitchUsageHandler(keys *service.APIKeyService) *CCSwitchUsageHandler {
	return &CCSwitchUsageHandler{keys: keys}
}

// Usage serves CC Switch's API-key-authenticated balance query without a JWT.
func (h *CCSwitchUsageHandler) Usage(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	key, ok := middleware.GetAPIKeyFromContext(c)
	if !ok {
		ccSwitchUsageError(c, http.StatusUnauthorized, "INVALID_API_KEY", "Invalid API key")
		return
	}
	quota, err := h.keys.GetFirstAvailableGroupQuota(c.Request.Context(), key)
	if err != nil {
		ccSwitchUsageError(c, http.StatusServiceUnavailable, "QUOTA_UNAVAILABLE", "Failed to query group quota")
		return
	}
	c.JSON(http.StatusOK, quota)
}

func ccSwitchUsageError(c *gin.Context, status int, code, message string) {
	c.JSON(status, service.APIKeyGroupQuota{
		Unit: "USD", InvalidMessage: message,
		Error: &service.APIKeyQuotaError{Code: code, Message: message},
	})
}
