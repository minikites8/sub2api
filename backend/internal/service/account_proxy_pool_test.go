package service

import (
	"context"
	"testing"
)

type proxyPoolSchedulingCacheStub struct {
	ConcurrencyCache
	counts       map[int64]int
	acquireCalls []int64
}

func (s *proxyPoolSchedulingCacheStub) GetAccountConcurrencyBatch(_ context.Context, accountIDs []int64) (map[int64]int, error) {
	result := make(map[int64]int, len(accountIDs))
	for _, accountID := range accountIDs {
		result[accountID] = s.counts[accountID]
	}
	return result, nil
}

func (s *proxyPoolSchedulingCacheStub) AcquireAccountSlot(_ context.Context, accountID int64, _ int, _ string) (bool, error) {
	s.acquireCalls = append(s.acquireCalls, accountID)
	return true, nil
}

func TestGatewayProxyPoolSchedulingPrefersLeastLoadedProxy(t *testing.T) {
	accountID := int64(42)
	bindings := []AccountProxyBinding{
		{ProxyID: 1, Concurrency: 10},
		{ProxyID: 2, Concurrency: 5},
	}
	cache := &proxyPoolSchedulingCacheStub{counts: map[int64]int{
		AccountProxySlotID(accountID, 1): 2,
		AccountProxySlotID(accountID, 2): 0,
	}}
	account := &Account{ID: accountID, Concurrency: 15, ProxyPool: bindings}
	service := &GatewayService{concurrencyService: NewConcurrencyService(cache)}

	result, err := service.tryAcquireAccountSlot(context.Background(), account)

	if err != nil {
		t.Fatal(err)
	}
	if result == nil || !result.Acquired {
		t.Fatalf("expected a proxy slot to be acquired: %#v", result)
	}
	if len(cache.acquireCalls) == 0 || cache.acquireCalls[0] != AccountProxySlotID(accountID, 2) {
		t.Fatalf("first proxy slot=%v, want proxy 2 slot %v", cache.acquireCalls, AccountProxySlotID(accountID, 2))
	}
	if account.ProxyID == nil || *account.ProxyID != 2 {
		t.Fatalf("selected proxy=%v, want 2", account.ProxyID)
	}
}

func TestParseAccountProxyPool(t *testing.T) {
	extra := map[string]any{
		AccountProxyPoolExtraKey: []any{
			map[string]any{"proxy_id": float64(10), "concurrency": float64(10)},
			map[string]any{"proxy_id": float64(11), "concurrency": float64(7)},
		},
	}
	bindings := ParseAccountProxyPool(extra)
	if len(bindings) != 2 || bindings[0].ProxyID != 10 || bindings[0].Concurrency != 10 || bindings[1].ProxyID != 11 || bindings[1].Concurrency != 7 {
		t.Fatalf("unexpected bindings: %#v", bindings)
	}
}

func TestAccountProxySlotIDStableAndDistinct(t *testing.T) {
	first := AccountProxySlotID(42, 10)
	if first < (1<<62) || first != AccountProxySlotID(42, 10) {
		t.Fatalf("slot ID must be stable and use the derived range: %d", first)
	}
	if first == AccountProxySlotID(42, 11) || first == AccountProxySlotID(43, 10) {
		t.Fatal("different account/proxy pairs must have different slot IDs")
	}
}

func TestAccountProxyPoolValidation(t *testing.T) {
	if err := ValidateAccountProxyPool([]AccountProxyBindingInput{{ProxyID: 1, Concurrency: 10}, {ProxyID: 2, Concurrency: 10}}); err != nil {
		t.Fatal(err)
	}
	if err := ValidateAccountProxyPool([]AccountProxyBindingInput{{ProxyID: 1, Concurrency: 0}}); err == nil {
		t.Fatal("zero concurrency must be rejected")
	}
	if err := ValidateAccountProxyPool([]AccountProxyBindingInput{{ProxyID: 1, Concurrency: 1}, {ProxyID: 1, Concurrency: 2}}); err == nil {
		t.Fatal("duplicate proxies must be rejected")
	}
}

func TestAccountProxyPoolEffectiveCapacity(t *testing.T) {
	account := &Account{ID: 7, Concurrency: 3, ProxyPool: []AccountProxyBinding{{ProxyID: 1, Concurrency: 10}, {ProxyID: 2, Concurrency: 10}}}
	if got := account.EffectiveLoadFactor(); got != 20 {
		t.Fatalf("effective capacity=%d, want 20", got)
	}
	account.setSelectedProxy(account.ProxyPool[1])
	if got := account.EffectiveProxyConcurrency(); got != 10 {
		t.Fatalf("selected proxy capacity=%d, want 10", got)
	}
}
