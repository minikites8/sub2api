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
	NodeID string  `json:"node_id"`
	APIKey string  `json:"api_key"`
	Amount float64 `json:"amount,omitempty"`
}

type QuotaLeaseDemoClientAuthResult struct {
	Snapshot  *APIKeyAuthSnapshot  `json:"snapshot"`
	Lease     *QuotaLeaseDemoLease `json:"lease,omitempty"`
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
	if cached := s.getClientAuthCache(apiKey); cached != nil {
		if err := s.ensureClientAuthCapacity(ctx, cached, amount); err != nil {
			s.deleteClientAuthCache(apiKey)
			return nil, err
		}
		return cached, nil
	}

	nodeID, secret, err := s.remoteNodeAuth(ctx)
	if err != nil {
		return nil, err
	}
	req := QuotaLeaseDemoClientAuthRequest{
		NodeID: nodeID,
		APIKey: apiKey,
		Amount: amount,
	}
	var result QuotaLeaseDemoClientAuthResult
	if err := s.doRemoteJSON(ctx, http.MethodPost, "/auth/client-key", nodeID, secret, req, &result); err != nil {
		var httpErr *quotaLeaseDemoRemoteHTTPError
		if errors.As(err, &httpErr) && httpErr.StatusCode == http.StatusUnauthorized && strings.Contains(httpErr.Body, "invalid_api_key") {
			return nil, ErrAPIKeyNotFound
		}
		return nil, err
	}
	if result.Snapshot == nil || result.Snapshot.APIKeyID <= 0 || result.Snapshot.UserID <= 0 {
		return nil, fmt.Errorf("%w: client auth response missing snapshot", ErrQuotaLeaseDemoInvalidInput)
	}
	if result.Lease != nil {
		s.cacheRemoteLease(result.Lease)
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
	if amount <= 0 {
		amount = s.PreflightReserveAmount()
	}
	if !s.ensureCapacity(ctx, s.activeNodeID(), result.Snapshot.UserID, result.Snapshot.APIKeyID, amount) {
		return ErrQuotaLeaseDemoNoCapacity
	}
	return nil
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
