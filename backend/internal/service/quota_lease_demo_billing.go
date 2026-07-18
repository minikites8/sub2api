package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
)

const (
	quotaLeaseDemoReserveRequestPrefix = "quota_lease_reserve:"
	quotaLeaseDemoCaptureRequestPrefix = "quota_lease_capture:"
	quotaLeaseDemoReleaseRequestPrefix = "quota_lease_release:"
)

func (s *QuotaLeaseDemoService) reserveLeaseBalance(ctx context.Context, lease *QuotaLeaseDemoLease, amount float64, requestID string) error {
	if s == nil || s.remoteMode() || lease == nil || amount <= 0 {
		return nil
	}
	repo := s.usageBillingRepository()
	if repo == nil {
		return nil
	}
	_, err := repo.ReserveBalanceHold(ctx, &BalanceHoldCommand{
		RequestID:          requestID,
		APIKeyID:           lease.APIKeyID,
		UserID:             lease.UserID,
		HoldID:             lease.ID,
		HoldAmount:         amount,
		RequestPayloadHash: quotaLeaseDemoBalanceHoldPayloadHash(lease.ID, lease.NodeID, lease.UserID, lease.APIKeyID, "", amount, "reserve"),
	})
	if errors.Is(err, ErrBalanceHoldInsufficientBalance) {
		return ErrQuotaLeaseDemoNoCapacity
	}
	return err
}

func (s *QuotaLeaseDemoService) captureLeaseBalance(ctx context.Context, event QuotaLeaseDemoUsageEvent) error {
	if s == nil || s.remoteMode() || event.Amount <= 0 {
		return nil
	}
	repo := s.usageBillingRepository()
	if repo == nil {
		return nil
	}
	_, err := repo.CaptureBalanceHold(ctx, &BalanceHoldCommand{
		RequestID:          quotaLeaseDemoCaptureRequestID(event.EventID),
		APIKeyID:           event.APIKeyID,
		UserID:             event.UserID,
		HoldID:             event.LeaseID,
		HoldAmount:         event.Amount,
		ActualAmount:       event.Amount,
		RequestPayloadHash: quotaLeaseDemoBalanceHoldPayloadHash(event.LeaseID, event.NodeID, event.UserID, event.APIKeyID, event.RequestID, event.Amount, "capture"),
	})
	if errors.Is(err, ErrBalanceHoldFrozenBalanceInsufficient) {
		return ErrQuotaLeaseDemoNoCapacity
	}
	return err
}

func (s *QuotaLeaseDemoService) releaseLeaseBalance(ctx context.Context, lease *QuotaLeaseDemoLease, amount float64) error {
	if s == nil || s.remoteMode() || lease == nil || amount <= 0 {
		return nil
	}
	repo := s.usageBillingRepository()
	if repo == nil {
		return nil
	}
	_, err := repo.ReleaseBalanceHold(ctx, &BalanceHoldCommand{
		RequestID:          quotaLeaseDemoReleaseRequestID(lease.ID),
		APIKeyID:           lease.APIKeyID,
		UserID:             lease.UserID,
		HoldID:             lease.ID,
		ReserveRequestID:   quotaLeaseDemoInitialReserveRequestID(lease.ID),
		HoldAmount:         amount,
		RequestPayloadHash: quotaLeaseDemoBalanceHoldPayloadHash(lease.ID, lease.NodeID, lease.UserID, lease.APIKeyID, "", amount, "release"),
	})
	if errors.Is(err, ErrBalanceHoldFrozenBalanceInsufficient) {
		return ErrQuotaLeaseDemoNoCapacity
	}
	return err
}

func quotaLeaseDemoInitialReserveRequestID(leaseID string) string {
	return quotaLeaseDemoReserveRequestPrefix + strings.TrimSpace(leaseID)
}

func quotaLeaseDemoTopUpReserveRequestID(leaseID string, targetGranted float64) string {
	raw := fmt.Sprintf("%s|%0.10f", strings.TrimSpace(leaseID), targetGranted)
	sum := sha256.Sum256([]byte(raw))
	return quotaLeaseDemoReserveRequestPrefix + strings.TrimSpace(leaseID) + ":topup:" + hex.EncodeToString(sum[:8])
}

func quotaLeaseDemoCaptureRequestID(eventID string) string {
	return quotaLeaseDemoCaptureRequestPrefix + strings.TrimSpace(eventID)
}

func quotaLeaseDemoReleaseRequestID(leaseID string) string {
	return quotaLeaseDemoReleaseRequestPrefix + strings.TrimSpace(leaseID)
}

func quotaLeaseDemoBalanceHoldPayloadHash(leaseID, nodeID string, userID, apiKeyID int64, requestID string, amount float64, operation string) string {
	return quotaLeaseDemoPayloadHash(leaseID, nodeID, userID, apiKeyID, requestID, amount, "balance_"+strings.TrimSpace(operation))
}
