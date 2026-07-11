package service

import (
	"context"
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
		shouldDisable := idx >= threshold-1
		if disablePreviousAccounts && idx >= keepPreviousAccounts {
			shouldDisable = true
		}
		if !shouldDisable {
			continue
		}
		target, err := s.userRepo.GetByID(ctx, item.UserID)
		if err != nil {
			logger.LegacyPrintf("service.gateway", "api usage ip+ua risk control get user failed: user=%d err=%v", item.UserID, err)
			continue
		}
		if target == nil || target.Status == StatusDisabled {
			continue
		}
		target.Status = StatusDisabled
		if err := s.userRepo.Update(ctx, target); err != nil {
			logger.LegacyPrintf("service.gateway", "api usage ip+ua risk control disable failed: user=%d err=%v", item.UserID, err)
			continue
		}
		if s.authCacheInvalidator != nil {
			s.authCacheInvalidator.InvalidateAuthCacheByUserID(ctx, item.UserID)
		}
		if currentIndex == idx {
			return
		}
	}
}

func (s *OpenAIGatewayService) gatewayRiskController() *GatewayService {
	if s == nil {
		return nil
	}
	return &GatewayService{
		usageLogRepo:         s.usageLogRepo,
		userRepo:             s.userRepo,
		settingService:       s.settingService,
		authCacheInvalidator: s.authCacheInvalidator,
	}
}
