package service

import "testing"

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
