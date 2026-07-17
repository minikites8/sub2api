package handler

import (
	"context"

	"github.com/Wei-Shaw/sub2api/internal/service"
)

func (h *GatewayHandler) checkBillingEligibility(ctx context.Context, user *service.User, apiKey *service.APIKey, group *service.Group, subscription *service.UserSubscription, platform string) error {
	leaseDemo := service.GetQuotaLeaseDemoService(h.cfg)
	if leaseDemo.Enabled() && subscription == nil {
		if leaseDemo.CanAuthorizeRequest(ctx, apiKey, subscription) {
			return nil
		}
		return service.ErrQuotaLeaseDemoNoCapacity
	}
	return h.billingCacheService.CheckBillingEligibility(ctx, user, apiKey, group, subscription, platform)
}

func (h *OpenAIGatewayHandler) checkBillingEligibility(ctx context.Context, user *service.User, apiKey *service.APIKey, group *service.Group, subscription *service.UserSubscription, platform string) error {
	leaseDemo := service.GetQuotaLeaseDemoService(h.cfg)
	if leaseDemo.Enabled() && subscription == nil {
		if leaseDemo.CanAuthorizeRequest(ctx, apiKey, subscription) {
			return nil
		}
		return service.ErrQuotaLeaseDemoNoCapacity
	}
	return h.billingCacheService.CheckBillingEligibility(ctx, user, apiKey, group, subscription, platform)
}
