package service

import (
	"context"
	"strings"
)

const quotaLeaseDemoUsageBillingRequestPrefix = "quota_lease_usage:"

func (s *QuotaLeaseDemoService) applyLeaseUsageBilling(ctx context.Context, event QuotaLeaseDemoUsageEvent) error {
	if s == nil || s.remoteMode() || event.Amount <= 0 {
		return nil
	}
	repo := s.usageBillingRepository()
	if repo == nil {
		return nil
	}
	_, err := repo.Apply(ctx, &UsageBillingCommand{
		RequestID:          quotaLeaseDemoUsageBillingRequestID(event.EventID),
		APIKeyID:           event.APIKeyID,
		UserID:             event.UserID,
		BalanceCost:        event.Amount,
		RequestPayloadHash: quotaLeaseDemoPayloadHash(event.LeaseID, event.NodeID, event.UserID, event.APIKeyID, event.RequestID, event.Amount, event.EventType),
	})
	return err
}

func quotaLeaseDemoUsageBillingRequestID(eventID string) string {
	return quotaLeaseDemoUsageBillingRequestPrefix + strings.TrimSpace(eventID)
}
