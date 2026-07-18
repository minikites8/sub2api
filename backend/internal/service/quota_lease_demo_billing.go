package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
)

const quotaLeaseDemoUsageBillingRequestPrefix = "quota_lease_usage:"

func (s *QuotaLeaseDemoService) applyLeaseUsageBilling(ctx context.Context, event QuotaLeaseDemoUsageEvent) (bool, error) {
	if s == nil || s.remoteMode() || event.Amount <= 0 {
		return true, nil
	}
	repo := s.usageBillingRepository()
	if repo == nil {
		return true, nil
	}
	result, err := repo.Apply(ctx, &UsageBillingCommand{
		RequestID:          quotaLeaseDemoUsageBillingRequestID(event.NodeID, event.APIKeyID, event.RequestID),
		APIKeyID:           event.APIKeyID,
		UserID:             event.UserID,
		BalanceCost:        event.Amount,
		StrictBalance:      true,
		RequestPayloadHash: quotaLeaseDemoUsageBillingPayloadHash(event.NodeID, event.UserID, event.APIKeyID, event.RequestID, event.Amount, event.EventType),
	})
	if err != nil {
		return false, err
	}
	if result != nil && !result.Applied {
		return false, nil
	}
	return true, nil
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
