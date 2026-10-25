package service

import (
	"context"
	"errors"
	"log/slog"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
)

// This adapter also guards ordinary account-slot users, so sharing a VM pool
// cannot evade its cap through a legacy scheduler or a non-QoS API key.
type GPTQoSCapacityStore interface {
	GPTQoSConfigureCapacity(context.Context, GPTQoSConfig, []Account) error
	GPTQoSValidatePool(context.Context, int64, string) (bool, error)
}

func (s *SettingService) synchronizeGPTQoSCapacity(ctx context.Context, c GPTQoSConfig) error {
	if s.gptQoSCapacityStore == nil {
		if c.CapacityEnabled {
			return errors.New("shared capacity store unavailable")
		}
		return nil
	}
	var accounts []Account
	if c.CapacityEnabled {
		if s.gptQoSAccountRepo == nil {
			return errors.New("account registry unavailable")
		}
		for page := 1; page <= 20; page++ {
			rows, result, err := s.gptQoSAccountRepo.ListWithFilters(ctx, pagination.PaginationParams{Page: page, PageSize: 500}, PlatformOpenAI, "", "", "", 0, "")
			if err != nil {
				return err
			}
			for i := range rows {
				if err := ValidateGPTQoSAccountCapacity(&rows[i]); err != nil {
					return err
				}
			}
			accounts = append(accounts, rows...)
			if result == nil || len(accounts) >= int(result.Total) {
				return s.gptQoSCapacityStore.GPTQoSConfigureCapacity(ctx, c, accounts)
			}
		}
		return errors.New("shared capacity registry exceeds 10000 accounts")
	}
	return s.gptQoSCapacityStore.GPTQoSConfigureCapacity(ctx, c, accounts)
}

func concurrencyServiceCapacityStore(s *ConcurrencyService) (GPTQoSCapacityStore, bool) {
	if s == nil {
		return nil, false
	}
	store, ok := s.cache.(GPTQoSCapacityStore)
	return store, ok
}

func ValidateGPTQoSAccountCapacity(a *Account) error {
	if a != nil && strings.HasPrefix(a.QoSResourcePool(), "shared:") && (a.Concurrency <= 0 || len(a.ProxyPool) > 0 || a.ProxyPoolConfigured) {
		return errors.New("shared resource accounts require a positive concurrency cap and no proxy pool")
	}
	return nil
}
func (s *adminServiceImpl) refreshGPTQoSRegistryAfterMutation() {
	if s.settingService == nil {
		return
	}
	s.settingService.gptQoSConfig.applyMu.Lock()
	defer s.settingService.gptQoSConfig.applyMu.Unlock()
	s.settingService.gptQoSConfig.mu.Lock()
	cfg := s.settingService.gptQoSConfig.config
	s.settingService.gptQoSConfig.mu.Unlock()
	if !cfg.CapacityEnabled {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if err := s.settingService.synchronizeGPTQoSCapacity(ctx, cfg); err != nil {
		slog.Error("gpt_qos_capacity_registry_refresh_failed")
	}
}
func (s *OpenAIGatewayService) checkGPTQoSFinalAdmission(ctx context.Context, selected, latest *Account) error {
	cfg := s.gptQoSConfigForContext(ctx)
	if state, _ := ctx.Value(gptQoSRequestKey{}).(*gptQoSRequestState); cfg.QualityRoutingEnabled && state != nil && selected.QualityTier() != latest.QualityTier() {
		return denyOpenAITurn("account_binding_changed")
	}
	if !cfg.CapacityEnabled {
		return nil
	}
	if selected.QoSResourcePool() != latest.QoSResourcePool() {
		return denyOpenAITurn("account_binding_changed")
	}
	if strings.HasPrefix(latest.QoSResourcePool(), "shared:") {
		store, ok := concurrencyServiceCapacityStore(s.concurrencyService)
		if !ok {
			return denyOpenAITurn("qos_capacity_unavailable")
		}
		valid, err := store.GPTQoSValidatePool(ctx, latest.ID, latest.QoSResourcePool())
		if err != nil || !valid {
			return denyOpenAITurn("qos_capacity_unavailable")
		}
	}
	return nil
}

func GPTQoSAccountSlotID(a *Account) int64 {
	if a == nil {
		return 0
	}
	bindings := a.ProxyBindings()
	if len(bindings) == 1 {
		return AccountProxySlotID(a.ID, bindings[0].ProxyID)
	}
	return a.ID
}
