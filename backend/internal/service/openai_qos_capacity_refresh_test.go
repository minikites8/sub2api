package service

import (
	"context"
	"encoding/json"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
	"github.com/stretchr/testify/require"
)

type qosCapacityRefreshAccounts struct{ AccountRepository }

func (qosCapacityRefreshAccounts) ListWithFilters(context.Context, pagination.PaginationParams, string, string, string, string, int64, string) ([]Account, *pagination.PaginationResult, error) {
	return []Account{}, nil, nil
}

type qosCapacityRefreshStore struct{ calls atomic.Int64 }

func (s *qosCapacityRefreshStore) GPTQoSConfigureCapacity(context.Context, GPTQoSConfig, []Account) error {
	s.calls.Add(1)
	return nil
}
func (s *qosCapacityRefreshStore) GPTQoSValidatePool(context.Context, int64, string) (bool, error) {
	return true, nil
}

func TestGPTQoSPeakCapacityRefreshesWithoutPolicyChange(t *testing.T) {
	cfg := DefaultGPTQoSConfig()
	cfg.Version = 123
	cfg.Enabled = true
	cfg.QualityRoutingEnabled = true
	cfg.CapacityEnabled = true
	cfg.Peak.Enabled = true
	raw, _ := json.Marshal(cfg)
	store := &qosCapacityRefreshStore{}
	s := &SettingService{settingRepo: &openAIAdvancedSchedulerSettingRepoStub{values: map[string]string{GPTQoSSettingKey: string(raw)}}, gptQoSCapacityStore: store, gptQoSAccountRepo: qosCapacityRefreshAccounts{}}
	s.gptQoSConfig.config = cfg
	s.gptQoSConfig.expires = time.Now().Add(-time.Second)
	_ = s.gptQoSRuntimeConfig()
	require.Eventually(t, func() bool {
		s.gptQoSConfig.mu.Lock()
		defer s.gptQoSConfig.mu.Unlock()
		return !s.gptQoSConfig.refreshing
	}, time.Second, time.Millisecond)
	require.Equal(t, int64(1), store.calls.Load(), "cooldown/expiry changes effective capacity without changing policy version")
	s.gptQoSConfig.mu.Lock()
	s.gptQoSConfig.expires = time.Now().Add(-time.Second)
	s.gptQoSConfig.mu.Unlock()
	_ = s.gptQoSRuntimeConfig()
	require.Eventually(t, func() bool {
		s.gptQoSConfig.mu.Lock()
		defer s.gptQoSConfig.mu.Unlock()
		return !s.gptQoSConfig.refreshing
	}, time.Second, time.Millisecond)
	require.Equal(t, int64(1), store.calls.Load(), "do not scan account registry on every config poll")
}
