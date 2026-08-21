package service

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/logger"
)

type apiUsageIPUARiskQueryRepo interface {
	ListDistinctUsersByIPAndUserAgentSince(ctx context.Context, ipAddress, userAgent string, startTime time.Time) ([]UsageLogUserFirstSeen, error)
}

func (s *GatewayService) applyAPIUsageIPUARiskControl(ctx context.Context, userID int64, ipAddress, userAgent string) {
	if s == nil || s.userRepo == nil || s.usageLogRepo == nil || userID <= 0 {
		return
	}
	ipAddress = strings.TrimSpace(ipAddress)
	userAgent = strings.TrimSpace(userAgent)
	if ipAddress == "" || userAgent == "" {
		return
	}

	repo, ok := s.usageLogRepo.(apiUsageIPUARiskQueryRepo)
	if !ok {
		return
	}

	threshold := defaultAPIUsageIPUARiskControlThreshold
	disablePreviousAccounts := defaultAPIUsageIPUADisablePreviousAccounts
	keepPreviousAccounts := defaultAPIUsageIPUAKeepPreviousAccounts
	if s.settingService != nil {
		threshold = s.settingService.GetAPIUsageIPUARiskControlThreshold(ctx)
		disablePreviousAccounts = s.settingService.GetAPIUsageIPUADisablePreviousAccounts(ctx)
		keepPreviousAccounts = s.settingService.GetAPIUsageIPUAKeepPreviousAccounts(ctx)
	}
	if threshold < 1 {
		threshold = 1
	}
	if keepPreviousAccounts < 0 {
		keepPreviousAccounts = 0
	}

	items, err := repo.ListDistinctUsersByIPAndUserAgentSince(ctx, ipAddress, userAgent, time.Now().Add(-24*time.Hour))
	if err != nil {
		logger.LegacyPrintf("service.gateway", "api usage ip+ua risk control query failed: user=%d ip=%s ua=%s err=%v", userID, ipAddress, userAgent, err)
		return
	}
	if len(items) < threshold {
		return
	}

	currentIndex := -1
	for idx, item := range items {
		if item.UserID == userID {
			currentIndex = idx
			break
		}
	}
	if currentIndex == -1 {
		return
	}

	for idx, item := range items {
		shouldDeduct := idx >= threshold-1
		if disablePreviousAccounts && idx >= keepPreviousAccounts {
			shouldDeduct = true
		}
		if !shouldDeduct {
			continue
		}
		target, err := s.userRepo.GetByID(ctx, item.UserID)
		if err != nil {
			logger.LegacyPrintf("service.gateway", "api usage ip+ua risk control get user failed: user=%d err=%v", item.UserID, err)
			if currentIndex == idx {
				return
			}
			continue
		}
		if target == nil {
			if currentIndex == idx {
				return
			}
			continue
		}
		if target.TotalRecharged <= 0 && target.Balance > 0 {
			deducted, err := deductIPRiskGiftBalance(ctx, s.userRepo, target)
			if err != nil {
				logger.LegacyPrintf("service.gateway", "api usage ip+ua risk control gift balance deduction failed: user=%d err=%v", item.UserID, err)
			} else if deducted > 0 {
				if recorder, ok := s.userRepo.(RiskControlBalanceRecorder); ok {
					if err := recorder.RecordRiskControlBalanceDeduction(ctx, item.UserID, deducted, fmt.Sprintf("API IP+UA 风控扣除赠金（IP: %s）", ipAddress)); err != nil {
						logger.LegacyPrintf("service.gateway", "api usage ip+ua risk control history record failed: user=%d ip=%s ua=%s err=%v", item.UserID, ipAddress, userAgent, err)
					}
				}
				if s.authCacheInvalidator != nil {
					s.authCacheInvalidator.InvalidateAuthCacheByUserID(ctx, item.UserID)
				}
				if s.billingCacheService != nil {
					_ = s.billingCacheService.InvalidateUserBalance(ctx, item.UserID)
				}
			}
		}
		if currentIndex == idx {
			return
		}
	}
}

func deductIPRiskGiftBalance(ctx context.Context, repo UserRepository, target *User) (float64, error) {
	if repo == nil || target == nil || target.ID <= 0 || target.TotalRecharged > 0 || target.Balance <= 0 {
		return 0, nil
	}
	if deductor, ok := repo.(IPRiskGiftBalanceDeductor); ok {
		return deductor.DeductAvailableGiftBalance(ctx, target.ID, target.Balance)
	}
	if deductor, ok := repo.(availableBalanceDeductor); ok {
		return deductor.DeductAvailableBalance(ctx, target.ID, target.Balance)
	}
	if adjuster, ok := repo.(RedeemUserAdjustmentRepository); ok {
		if err := adjuster.ApplyRedeemBalanceAdjustment(ctx, target.ID, -target.Balance); err != nil {
			return 0, err
		}
		return target.Balance, nil
	}
	return 0, errors.New("user repository does not support IP risk gift balance deduction")
}

func (s *OpenAIGatewayService) gatewayRiskController() *GatewayService {
	if s == nil {
		return nil
	}
	return &GatewayService{
		usageLogRepo:         s.usageLogRepo,
		userRepo:             s.userRepo,
		settingService:       s.settingService,
		billingCacheService:  s.billingCacheService,
		authCacheInvalidator: s.authCacheInvalidator,
	}
}
