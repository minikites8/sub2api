package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"
)

const quotaLeaseDemoClientAuthCacheTTL = 30 * time.Second

type QuotaLeaseDemoClientAuthRequest struct {
	NodeID    string  `json:"node_id"`
	APIKey    string  `json:"api_key"`
	Amount    float64 `json:"amount,omitempty"`
	RequestID string  `json:"request_id,omitempty"`
	TraceID   string  `json:"trace_id,omitempty"`
}

type QuotaLeaseDemoClientAuthResult struct {
	Snapshot  *APIKeyAuthSnapshot  `json:"snapshot"`
	Lease     *QuotaLeaseDemoLease `json:"lease,omitempty"`
	TraceID   string               `json:"trace_id,omitempty"`
	ExpiresAt time.Time            `json:"expires_at"`
}

type quotaLeaseDemoClientAuthCacheEntry struct {
	Result    QuotaLeaseDemoClientAuthResult
	ExpiresAt time.Time
}

func (s *QuotaLeaseDemoService) AuthorizeClientKeyViaControlPlane(ctx context.Context, apiKey string, amount float64) (*QuotaLeaseDemoClientAuthResult, error) {
	if s == nil || !s.remoteMode() {
		return nil, ErrQuotaLeaseDemoDisabled
	}
	apiKey = strings.TrimSpace(apiKey)
	if apiKey == "" {
		return nil, ErrAPIKeyNotFound
	}
	requestAmount := amount
	if requestAmount <= 0 {
		requestAmount = s.DefaultGrantAmount()
	}
	requestID := quotaLeaseDemoContextRequestID(ctx)
	if cached := s.getClientAuthCache(apiKey); cached != nil {
		if err := s.ensureClientAuthCapacity(ctx, cached, requestAmount); err != nil {
			s.deleteClientAuthCache(apiKey)
			return nil, err
		}
		return cached, nil
	}

	nodeID, secret, err := s.remoteNodeAuth(ctx)
	if err != nil {
		return nil, err
	}
	traceID := quotaLeaseDemoTraceID("", nodeID, 0, 0, requestID)
	req := QuotaLeaseDemoClientAuthRequest{
		NodeID:    nodeID,
		APIKey:    apiKey,
		Amount:    requestAmount,
		RequestID: requestID,
		TraceID:   traceID,
	}
	var result QuotaLeaseDemoClientAuthResult
	if err := s.doRemoteJSON(ctx, http.MethodPost, "/auth/client-key", nodeID, secret, req, &result); err != nil {
		var httpErr *quotaLeaseDemoRemoteHTTPError
		if errors.As(err, &httpErr) && httpErr.StatusCode == http.StatusUnauthorized && strings.Contains(httpErr.Body, "invalid_api_key") {
			return nil, ErrAPIKeyNotFound
		}
		if quotaLeaseDemoRemoteNoCapacity(err) {
			probe := s.inspectCapacitySnapshot(nodeID, 0, 0, requestAmount, time.Now().UTC())
			s.logCapacityDenied("client_auth", "remote_client_auth_no_capacity", nodeID, 0, 0, requestAmount, probe, err)
			return nil, ErrQuotaLeaseDemoNoCapacity
		}
		return nil, err
	}
	if result.Snapshot == nil || result.Snapshot.APIKeyID <= 0 || result.Snapshot.UserID <= 0 {
		return nil, fmt.Errorf("%w: client auth response missing snapshot", ErrQuotaLeaseDemoInvalidInput)
	}
	result.TraceID = quotaLeaseDemoTraceID(result.TraceID, nodeID, result.Snapshot.UserID, result.Snapshot.APIKeyID, requestID)
	if result.Lease != nil {
		s.cacheRemoteLease(result.Lease)
	}
	if err := s.ensureClientAuthCapacity(ctx, &result, requestAmount); err != nil {
		return nil, err
	}
	if result.ExpiresAt.IsZero() {
		result.ExpiresAt = quotaLeaseDemoClientAuthExpiresAt(result.Lease)
	}
	s.setClientAuthCache(apiKey, &result)
	return &result, nil
}

func (s *QuotaLeaseDemoService) ensureClientAuthCapacity(ctx context.Context, result *QuotaLeaseDemoClientAuthResult, amount float64) error {
	if s == nil || result == nil || result.Snapshot == nil {
		return ErrQuotaLeaseDemoNoCapacity
	}
	amount = s.clientAuthCapacityAmount(result.Snapshot, amount)
	if amount <= 0 {
		return ErrQuotaLeaseDemoNoCapacity
	}
	if !s.ensureCapacityWithMinimum(ctx, "client_auth", s.activeNodeID(), result.Snapshot.UserID, result.Snapshot.APIKeyID, amount, s.preflightCapacityCheckAmount(amount)) {
		return ErrQuotaLeaseDemoNoCapacity
	}
	return nil
}

func (s *QuotaLeaseDemoService) clientAuthCapacityAmount(snapshot *APIKeyAuthSnapshot, requested float64) float64 {
	if s == nil {
		return 0
	}
	amount := requested
	if amount <= 0 {
		amount = s.DefaultGrantAmount()
	}
	if snapshot == nil {
		return amount
	}
	balance := snapshot.User.Balance
	if balance <= 0 {
		return 0
	}
	if balance < amount {
		return balance
	}
	return amount
}

func quotaLeaseDemoRemoteNoCapacity(err error) bool {
	var httpErr *quotaLeaseDemoRemoteHTTPError
	if !errors.As(err, &httpErr) {
		return false
	}
	if httpErr.StatusCode != http.StatusForbidden {
		return false
	}
	body := strings.ToLower(strings.TrimSpace(httpErr.Body))
	return strings.Contains(body, "no_capacity") || strings.Contains(body, "quota_lease_demo_no_capacity")
}

func (s *QuotaLeaseDemoService) getClientAuthCache(apiKey string) *QuotaLeaseDemoClientAuthResult {
	if s == nil {
		return nil
	}
	cacheKey := quotaLeaseDemoClientAuthCacheKey(apiKey)
	now := time.Now().UTC()
	s.mu.Lock()
	defer s.mu.Unlock()
	entry := s.clientAuthCache[cacheKey]
	if entry == nil {
		return nil
	}
	if !entry.ExpiresAt.After(now) {
		delete(s.clientAuthCache, cacheKey)
		return nil
	}
	result := entry.Result
	return &result
}

func (s *QuotaLeaseDemoService) deleteClientAuthCache(apiKey string) {
	if s == nil {
		return
	}
	cacheKey := quotaLeaseDemoClientAuthCacheKey(apiKey)
	s.mu.Lock()
	delete(s.clientAuthCache, cacheKey)
	s.mu.Unlock()
}

func (s *QuotaLeaseDemoService) setClientAuthCache(apiKey string, result *QuotaLeaseDemoClientAuthResult) {
	if s == nil || result == nil || result.Snapshot == nil {
		return
	}
	expiresAt := result.ExpiresAt
	if expiresAt.IsZero() {
		expiresAt = quotaLeaseDemoClientAuthExpiresAt(result.Lease)
	}
	if !expiresAt.After(time.Now().UTC()) {
		return
	}
	cacheKey := quotaLeaseDemoClientAuthCacheKey(apiKey)
	s.mu.Lock()
	if s.clientAuthCache == nil {
		s.clientAuthCache = make(map[string]*quotaLeaseDemoClientAuthCacheEntry)
	}
	s.clientAuthCache[cacheKey] = &quotaLeaseDemoClientAuthCacheEntry{
		Result:    *result,
		ExpiresAt: expiresAt,
	}
	s.mu.Unlock()
}

func quotaLeaseDemoClientAuthExpiresAt(lease *QuotaLeaseDemoLease) time.Time {
	expiresAt := time.Now().UTC().Add(quotaLeaseDemoClientAuthCacheTTL)
	if lease != nil && !lease.ExpiresAt.IsZero() && lease.ExpiresAt.Before(expiresAt) {
		expiresAt = lease.ExpiresAt
	}
	return expiresAt
}

func quotaLeaseDemoClientAuthCacheKey(apiKey string) string {
	sum := sha256.Sum256([]byte(strings.TrimSpace(apiKey)))
	return hex.EncodeToString(sum[:])
}
