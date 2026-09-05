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

	policy := DefaultAntiAbusePolicy()
	if s.settingService != nil {
		policy = s.settingService.GetAntiAbusePolicy(ctx)
	}
	if !policy.Enabled {
		return
	}

	items, err := repo.ListDistinctUsersByIPAndUserAgentSince(ctx, ipAddress, userAgent, time.Now().Add(-24*time.Hour))
	if err != nil {
		logger.LegacyPrintf("service.gateway", "api usage ip+ua risk control query failed: user=%d ip=%s ua=%s err=%v", userID, ipAddress, userAgent, err)
		return
	}
	target, targetErr := s.userRepo.GetByID(ctx, userID)
	if targetErr != nil || target == nil {
		return
	}
	if target.IsAdmin() {
		return
	}
	signals := RiskSignals{IPAddress: ipAddress, Email: target.Email, UserAgent: userAgent}
	contextSignals := RiskSignalsFromContext(ctx)
	signals.BrowserFingerprints = contextSignals.BrowserFingerprints
	signals.AccountAttempts = NormalizeAccountAttempts(append(append([]string{}, contextSignals.AccountAttempts...), target.Email))
	signals.JA3 = contextSignals.JA3
	signals.JA4 = contextSignals.JA4
	fingerprintVelocity := 0
	tlsVelocity := 0
	fingerprintLinkedUserIDs := make(map[int64]struct{})
	if s.settingService != nil {
		signals.IPReputationScore = s.settingService.LookupConfiguredIPReputation(ctx, ipAddress)
	}
	windowStart := time.Now().Add(-24 * time.Hour)
	browserHashes := HashBrowserFingerprints(signals.BrowserFingerprints)
	signalStore, hasSignalStore := s.userRepo.(AntiAbuseSignalStore)
	if browserStore, ok := s.userRepo.(AntiAbuseBrowserLinkStore); ok {
		if len(browserHashes) == 0 {
			if storedHashes, readErr := browserStore.GetUserBrowserFingerprints(ctx, userID); readErr == nil {
				browserHashes = storedHashes
			}
		}
		if len(browserHashes) > 0 {
			if hasSignalStore {
				if count, countErr := signalStore.CountUsersByFingerprintHashes(ctx, browserHashes, windowStart); countErr == nil {
					fingerprintVelocity = maxInt(0, count-1)
				}
			}
			if linkedIDs, queryErr := browserStore.ListUsersByFingerprintHashes(ctx, browserHashes, windowStart); queryErr == nil {
				for _, linkedID := range linkedIDs {
					if linkedID > 0 && linkedID != userID {
						fingerprintLinkedUserIDs[linkedID] = struct{}{}
					}
				}
				if linkedCount := maxInt(len(linkedIDs)-1, 0); linkedCount > fingerprintVelocity {
					fingerprintVelocity = linkedCount
				}
			}
		}
	}
	if hasSignalStore {
		if count, countErr := signalStore.CountUsersByTransportFingerprints(ctx, signals.JA3, signals.JA4, windowStart); countErr == nil {
			tlsVelocity = maxInt(0, count-1)
		}
	}
	assessment := EvaluateGatewayAntiAbuse(signals, maxInt(0, len(items)-1), fingerprintVelocity, 0, tlsVelocity, policy)
	antiAbuseEvents, _ := s.userRepo.(AntiAbuseEventStore)
	RecordAntiAbuseAssessment(ctx, antiAbuseEvents, "gateway", &userID, target.Email, signals, assessment)
	if hasSignalStore && (signals.JA3 != "" || signals.JA4 != "") {
		_ = signalStore.StoreTransportFingerprints(ctx, userID, signals.JA3, signals.JA4, assessment)
	}
	threshold := policy.APIUsageIPUARiskControlThreshold
	disablePreviousAccounts := policy.APIUsageIPUADisablePreviousAccounts
	keepPreviousAccounts := policy.APIUsageIPUAKeepPreviousAccounts
	if threshold < 1 {
		threshold = defaultAPIUsageIPUARiskControlThreshold
	}
	if keepPreviousAccounts < 0 {
		keepPreviousAccounts = 0
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
		shouldDeduct := assessment.Action == AntiAbuseActionRestrict && item.UserID == userID
		if _, linked := fingerprintLinkedUserIDs[item.UserID]; linked {
			shouldDeduct = true
		}
		if len(items) >= threshold && idx >= threshold-1 {
			shouldDeduct = true
		}
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
				RecordAntiAbuseDeduction(ctx, antiAbuseEvents, "gateway_gift_deduction", &item.UserID, target.Email, signals, assessment, deducted)
				if recorder, ok := s.userRepo.(RiskControlBalanceRecorder); ok {
					note := fmt.Sprintf("API 多维反滥用风控扣除赠金（score=%d factors=%v）", assessment.Score, assessment.Factors)
					if err := recorder.RecordRiskControlBalanceDeduction(ctx, item.UserID, deducted, note); err != nil {
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
			break
		}
	}

	// Fingerprint-linked accounts can use different IPs and User-Agents, so
	// process linked free-credit accounts after the IP+UA ordered set.
	for linkedID := range fingerprintLinkedUserIDs {
		foundInIPUASet := false
		for _, item := range items {
			if item.UserID == linkedID {
				foundInIPUASet = true
				break
			}
		}
		if foundInIPUASet {
			continue
		}
		target, err := s.userRepo.GetByID(ctx, linkedID)
		if err != nil || target == nil || target.TotalRecharged > 0 || target.Balance <= 0 {
			continue
		}
		deducted, err := deductIPRiskGiftBalance(ctx, s.userRepo, target)
		if err != nil {
			logger.LegacyPrintf("service.gateway", "fingerprint-linked gift balance deduction failed: user=%d linked_user=%d err=%v", userID, linkedID, err)
			continue
		}
		if deducted <= 0 {
			continue
		}
		RecordAntiAbuseDeduction(ctx, antiAbuseEvents, "gateway_gift_deduction", &linkedID, target.Email, signals, assessment, deducted)
		if recorder, ok := s.userRepo.(RiskControlBalanceRecorder); ok {
			note := fmt.Sprintf("多维反滥用浏览器指纹关联扣除赠金（score=%d factors=%v）", assessment.Score, assessment.Factors)
			if err := recorder.RecordRiskControlBalanceDeduction(ctx, linkedID, deducted, note); err != nil {
				logger.LegacyPrintf("service.gateway", "fingerprint-linked history record failed: user=%d linked_user=%d err=%v", userID, linkedID, err)
			}
		}
		if s.authCacheInvalidator != nil {
			s.authCacheInvalidator.InvalidateAuthCacheByUserID(ctx, linkedID)
		}
		if s.billingCacheService != nil {
			_ = s.billingCacheService.InvalidateUserBalance(ctx, linkedID)
		}
	}
}

func deductIPRiskGiftBalance(ctx context.Context, repo UserRepository, target *User) (float64, error) {
	if repo == nil || target == nil || target.ID <= 0 || target.Balance <= 0 {
		return 0, nil
	}
	if target.GiftBalance <= 0 {
		fresh, err := repo.GetByID(ctx, target.ID)
		if err != nil {
			return 0, err
		}
		target = fresh
	}
	if target.GiftBalance <= 0 {
		return 0, nil
	}
	if deductor, ok := repo.(IPRiskGiftBalanceDeductor); ok {
		return deductor.DeductAvailableGiftBalance(ctx, target.ID, target.GiftBalance)
	}
	if deductor, ok := repo.(availableBalanceDeductor); ok {
		return deductor.DeductAvailableBalance(ctx, target.ID, target.GiftBalance)
	}
	if adjuster, ok := repo.(RedeemUserAdjustmentRepository); ok {
		if err := adjuster.ApplyRedeemBalanceAdjustment(ctx, target.ID, -target.GiftBalance); err != nil {
			return 0, err
		}
		return target.GiftBalance, nil
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
