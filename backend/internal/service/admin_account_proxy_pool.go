package service

import (
	"context"
	"fmt"
	"time"
)

func (s *adminServiceImpl) normalizeProxyPool(ctx context.Context, pool []AccountProxyBindingInput) ([]AccountProxyBindingInput, error) {
	if err := ValidateAccountProxyPool(pool); err != nil {
		return nil, err
	}
	if len(pool) == 0 {
		return []AccountProxyBindingInput{}, nil
	}
	if s == nil || s.proxyRepo == nil {
		return nil, fmt.Errorf("proxy repository is unavailable")
	}
	normalized := make([]AccountProxyBindingInput, 0, len(pool))
	now := time.Now()
	for _, binding := range pool {
		proxy, err := s.proxyRepo.GetByID(ctx, binding.ProxyID)
		if err != nil {
			return nil, fmt.Errorf("load proxy %d: %w", binding.ProxyID, err)
		}
		if proxy == nil || !proxy.IsActive() || proxy.IsExpired(now) {
			return nil, fmt.Errorf("proxy %d is unavailable", binding.ProxyID)
		}
		normalized = append(normalized, AccountProxyBindingInput{
			ProxyID:     binding.ProxyID,
			Concurrency: binding.Concurrency,
		})
	}
	return normalized, nil
}

func setAccountProxyPoolExtra(extra map[string]any, pool []AccountProxyBindingInput) map[string]any {
	if extra == nil {
		extra = make(map[string]any)
	}
	if len(pool) == 0 {
		delete(extra, AccountProxyPoolExtraKey)
	} else {
		extra[AccountProxyPoolExtraKey] = AccountProxyPoolExtra(pool)
	}
	return extra
}
