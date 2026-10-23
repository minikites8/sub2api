package handler

import (
	"errors"
	"net/http"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
)

func applyUsageKeyValidity(resp gin.H, key *service.APIKey) {
	resp["status"] = key.Status
	if key.ExpiresAt != nil {
		resp["expires_at"] = key.ExpiresAt
	}
	switch {
	case key.Status == service.StatusAPIKeyExpired || key.IsExpired():
		resp["isValid"], resp["invalidMessage"] = false, "API key has expired"
	case key.Status == service.StatusAPIKeyQuotaExhausted || key.IsQuotaExhausted():
		resp["isValid"], resp["invalidMessage"] = false, "API key total quota is exhausted"
	case key.Status != service.StatusAPIKeyActive:
		resp["isValid"], resp["invalidMessage"] = false, "API key is not active"
	}
}

func (h *GatewayHandler) appendUsageSubscription(c *gin.Context, key *service.APIKey, resp gin.H) bool {
	if key.Group == nil || !key.Group.IsSubscriptionType() {
		return true
	}
	resp["planName"] = key.Group.Name
	var snapshot *service.SubscriptionUsageSnapshot
	var err error
	if h.apiKeyService != nil {
		snapshot, err = h.apiKeyService.GetUsageSubscriptionSnapshot(c.Request.Context(), key)
	} else if sub, ok := middleware.GetSubscriptionFromContext(c); ok && sub.UserID == key.UserID && sub.GroupID == key.Group.ID {
		snapshot, err = service.SnapshotSubscriptionUsage(key.Group, sub, time.Now())
	} else {
		err = service.ErrSubscriptionNotFound
	}
	if errors.Is(err, service.ErrSubscriptionNotFound) {
		resp["isValid"], resp["invalidMessage"] = false, "No active subscription found for this group"
		return true
	}
	if err != nil {
		h.errorResponse(c, http.StatusServiceUnavailable, "api_error", "Subscription usage is temporarily unavailable")
		return false
	}
	resp["subscription"] = snapshot
	if key.Quota <= 0 {
		resp["remaining"] = h.calculateSubscriptionRemaining(key.Group, &service.UserSubscription{
			DailyUsageUSD: snapshot.DailyUsageUSD, WeeklyUsageUSD: snapshot.WeeklyUsageUSD, MonthlyUsageUSD: snapshot.MonthlyUsageUSD,
		})
		resp["unlimited"] = snapshot.Unlimited
	}
	return true
}
