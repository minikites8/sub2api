package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
)

const quotaLeaseDemoUsageBillingRequestPrefix = "quota_lease_usage:"

func (s *QuotaLeaseDemoService) applyLeaseUsageBilling(ctx context.Context, event QuotaLeaseDemoUsageEvent, lease *QuotaLeaseDemoLease, ledgerEvent *QuotaLeaseDemoLedgerEvent) (applied bool, persisted bool, err error) {
	if s == nil || s.remoteMode() || event.Amount <= 0 {
		return true, false, nil
	}
	repo := s.usageBillingRepository()
	if repo == nil {
		return true, false, nil
	}
	cmd := &UsageBillingCommand{
		RequestID:          quotaLeaseDemoUsageBillingRequestID(event.NodeID, event.APIKeyID, event.RequestID),
		APIKeyID:           event.APIKeyID,
		UserID:             event.UserID,
		BalanceCost:        event.Amount,
		StrictBalance:      false,
		RequestPayloadHash: quotaLeaseDemoUsageBillingPayloadHash(event.NodeID, event.UserID, event.APIKeyID, event.RequestID, event.Amount, event.EventType),
	}
	if atomicRepo, ok := repo.(QuotaLeaseDemoUsageBillingRepository); ok && lease != nil && ledgerEvent != nil {
		result, err := atomicRepo.ApplyQuotaLeaseUsage(ctx, &QuotaLeaseDemoUsageBillingCommand{
			Billing: cmd,
			Lease:   *lease,
			Event:   *ledgerEvent,
		})
		if err != nil {
			return false, false, err
		}
		if result != nil && !result.Applied {
			return false, true, nil
		}
		return true, true, nil
	}
	result, err := repo.Apply(ctx, cmd)
	if err != nil {
		return false, false, err
	}
	if result != nil && !result.Applied {
		return false, false, nil
	}
	return true, false, nil
}

func quotaLeaseDemoUsageBillingRequestID(nodeID string, apiKeyID int64, requestID string) string {
	raw := fmt.Sprintf("%s|%d|%s", strings.TrimSpace(nodeID), apiKeyID, strings.TrimSpace(requestID))
	sum := sha256.Sum256([]byte(raw))
	return quotaLeaseDemoUsageBillingRequestPrefix + hex.EncodeToString(sum[:8])
}

func quotaLeaseDemoUsageBillingPayloadHash(nodeID string, userID, apiKeyID int64, requestID string, amount float64, eventType string) string {
	raw := fmt.Sprintf("%s|%d|%d|%s|%0.10f|%s",
		strings.TrimSpace(nodeID),
		userID,
		apiKeyID,
		strings.TrimSpace(requestID),
		amount,
		strings.TrimSpace(eventType),
	)
	sum := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(sum[:])
}
