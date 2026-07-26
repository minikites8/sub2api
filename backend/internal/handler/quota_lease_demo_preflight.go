package handler

import (
	"context"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/service"
)

// A granted quota lease stands in for the balance check only. Every other
// admission control — the admin-configured user × platform spend caps, API key
// rate limits, RPM — has no other enforcement point before settlement, so it
// must still run or enabling quota leases silently disables it.
func checkQuotaLeaseBillingEligibility(
	ctx context.Context,
	cfg *config.Config,
	billingCacheService *service.BillingCacheService,
	user *service.User,
	apiKey *service.APIKey,
	group *service.Group,
	subscription *service.UserSubscription,
	platform string,
) error {
	leaseDemo := service.GetQuotaLeaseDemoService(cfg)
	if leaseDemo.Enabled() && subscription == nil {
		if !leaseDemo.CanAuthorizeRequest(ctx, apiKey, subscription) {
			return service.ErrQuotaLeaseDemoNoCapacity
		}
		return billingCacheService.CheckNonBalanceEligibility(ctx, user, apiKey, group, subscription, platform)
	}
	return billingCacheService.CheckBillingEligibility(ctx, user, apiKey, group, subscription, platform)
}

func (h *GatewayHandler) checkBillingEligibility(ctx context.Context, user *service.User, apiKey *service.APIKey, group *service.Group, subscription *service.UserSubscription, platform string) error {
	return checkQuotaLeaseBillingEligibility(ctx, h.cfg, h.billingCacheService, user, apiKey, group, subscription, platform)
}

func (h *OpenAIGatewayHandler) checkBillingEligibility(ctx context.Context, user *service.User, apiKey *service.APIKey, group *service.Group, subscription *service.UserSubscription, platform string) error {
	return checkQuotaLeaseBillingEligibility(ctx, h.cfg, h.billingCacheService, user, apiKey, group, subscription, platform)
}
