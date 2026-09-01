package service

import (
	"encoding/json"
	"errors"
	"fmt"
	"hash/fnv"
	"strconv"
	"time"
)

// AccountProxyPoolExtraKey stores the optional per-account proxy pool in the
// account extra JSON document. Each entry is an object with proxy_id and
// concurrency fields.
const AccountProxyPoolExtraKey = "proxy_pool"

// AccountProxyBinding is one proxy assigned to an account.
type AccountProxyBinding struct {
	ProxyID     int64
	Concurrency int
	Proxy       *Proxy
}

// AccountProxyBindingInput is the API/admin representation used when saving a
// proxy pool.
type AccountProxyBindingInput struct {
	ProxyID     int64 `json:"proxy_id"`
	Concurrency int   `json:"concurrency"`
}

// ParseAccountProxyPool reads the permissive JSON representation used by
// accounts.extra. Invalid entries are ignored so old account rows remain
// schedulable; write paths validate entries before persisting them.
func ParseAccountProxyPool(extra map[string]any) []AccountProxyBinding {
	if extra == nil {
		return nil
	}
	raw, ok := extra[AccountProxyPoolExtraKey]
	if !ok || raw == nil {
		return nil
	}
	if typed, ok := raw.([]AccountProxyBinding); ok {
		return cloneAccountProxyBindings(typed)
	}
	bytes, err := json.Marshal(raw)
	if err != nil {
		return nil
	}
	var entries []AccountProxyBindingInput
	if err := json.Unmarshal(bytes, &entries); err != nil {
		return nil
	}
	bindings := make([]AccountProxyBinding, 0, len(entries))
	seen := make(map[int64]struct{}, len(entries))
	for _, entry := range entries {
		if entry.ProxyID <= 0 || entry.Concurrency <= 0 {
			continue
		}
		if _, exists := seen[entry.ProxyID]; exists {
			continue
		}
		seen[entry.ProxyID] = struct{}{}
		bindings = append(bindings, AccountProxyBinding{ProxyID: entry.ProxyID, Concurrency: entry.Concurrency})
	}
	return bindings
}

func accountProxyPoolInputsFromExtra(extra map[string]any) ([]AccountProxyBindingInput, bool, error) {
	if extra == nil {
		return nil, false, nil
	}
	raw, exists := extra[AccountProxyPoolExtraKey]
	if !exists {
		return nil, false, nil
	}
	bytes, err := json.Marshal(raw)
	if err != nil {
		return nil, true, err
	}
	var inputs []AccountProxyBindingInput
	if err := json.Unmarshal(bytes, &inputs); err != nil {
		return nil, true, fmt.Errorf("proxy_pool must be an array: %w", err)
	}
	return inputs, true, nil
}

func cloneAccountProxyBindings(bindings []AccountProxyBinding) []AccountProxyBinding {
	if len(bindings) == 0 {
		return nil
	}
	cloned := make([]AccountProxyBinding, len(bindings))
	copy(cloned, bindings)
	return cloned
}

// AccountProxyPoolExtra returns a JSON-safe value for accounts.extra.
func AccountProxyPoolExtra(bindings []AccountProxyBindingInput) []map[string]any {
	entries := make([]map[string]any, 0, len(bindings))
	for _, binding := range bindings {
		entries = append(entries, map[string]any{
			"proxy_id":    binding.ProxyID,
			"concurrency": binding.Concurrency,
		})
	}
	return entries
}

func accountProxyBindingsFromInputs(inputs []AccountProxyBindingInput) []AccountProxyBinding {
	if len(inputs) == 0 {
		return nil
	}
	out := make([]AccountProxyBinding, 0, len(inputs))
	for _, input := range inputs {
		out = append(out, AccountProxyBinding{ProxyID: input.ProxyID, Concurrency: input.Concurrency})
	}
	return out
}

func (a *Account) ProxyBindings() []AccountProxyBinding {
	if a == nil {
		return nil
	}
	if len(a.ProxyPool) > 0 {
		bindings := make([]AccountProxyBinding, 0, len(a.ProxyPool))
		now := time.Now()
		for _, binding := range a.ProxyPool {
			if binding.Proxy != nil && (!binding.Proxy.IsActive() || binding.Proxy.IsExpired(now)) {
				continue
			}
			bindings = append(bindings, binding)
		}
		return bindings
	}
	if a.ProxyPoolConfigured {
		return nil
	}
	if a.ProxyID != nil && *a.ProxyID > 0 {
		return []AccountProxyBinding{{ProxyID: *a.ProxyID, Concurrency: a.Concurrency, Proxy: a.Proxy}}
	}
	return nil
}

func (a *Account) setSelectedProxy(binding AccountProxyBinding) {
	if a == nil || binding.ProxyID <= 0 {
		return
	}
	proxyID := binding.ProxyID
	a.ProxyID = &proxyID
	a.Proxy = binding.Proxy
}

func (a *Account) ensureSelectedProxy() {
	if a == nil || len(a.ProxyPool) == 0 {
		return
	}
	bindings := a.ProxyBindings()
	if a.ProxyID != nil {
		for _, binding := range bindings {
			if binding.ProxyID == *a.ProxyID {
				return
			}
		}
	}
	if len(bindings) > 0 {
		a.setSelectedProxy(bindings[0])
	} else {
		a.ProxyID = nil
		a.Proxy = nil
	}
}

// PreserveSelectedProxyFrom copies the proxy choice made during scheduling to
// a freshly loaded account object used by admission checks.
func (a *Account) PreserveSelectedProxyFrom(source *Account) {
	if a == nil || source == nil || source.ProxyID == nil {
		return
	}
	selectedID := *source.ProxyID
	for _, binding := range a.ProxyPool {
		if binding.ProxyID == selectedID {
			a.setSelectedProxy(binding)
			return
		}
	}
}

// EffectiveProxyConcurrency returns the configured cap for a selected proxy.
func (a *Account) EffectiveProxyConcurrency() int {
	if a == nil {
		return 0
	}
	for _, binding := range a.ProxyBindings() {
		if a.ProxyID != nil && binding.ProxyID == *a.ProxyID && binding.Concurrency > 0 {
			return binding.Concurrency
		}
	}
	return a.Concurrency
}

// EffectiveProxySlotID returns the Redis slot namespace for the selected
// proxy. A single legacy proxy keeps the account ID for compatibility.
func (a *Account) EffectiveProxySlotID() int64 {
	if a == nil || a.ProxyID == nil || *a.ProxyID <= 0 || len(a.ProxyPool) == 0 {
		if a == nil {
			return 0
		}
		return a.ID
	}
	return AccountProxySlotID(a.ID, *a.ProxyID)
}

// AccountProxySlotID creates a stable high-range ID for an account/proxy pair.
// The reserved high range keeps derived slots distinct from normal account IDs
// while allowing the existing active-index cleanup to track them.
func AccountProxySlotID(accountID, proxyID int64) int64 {
	if accountID <= 0 || proxyID <= 0 {
		return accountID
	}
	h := fnv.New64a()
	_, _ = h.Write([]byte(strconv.FormatInt(accountID, 10)))
	_, _ = h.Write([]byte{0})
	_, _ = h.Write([]byte(strconv.FormatInt(proxyID, 10)))
	value := int64(h.Sum64() & 0x3fffffffffffffff)
	return value | (1 << 62)
}

func ValidateAccountProxyPool(bindings []AccountProxyBindingInput) error {
	seen := make(map[int64]struct{}, len(bindings))
	for _, binding := range bindings {
		if binding.ProxyID <= 0 {
			return errors.New("proxy_pool.proxy_id must be positive")
		}
		if binding.Concurrency <= 0 {
			return fmt.Errorf("proxy_pool concurrency for proxy %d must be positive", binding.ProxyID)
		}
		if binding.Concurrency > 10000 {
			return fmt.Errorf("proxy_pool concurrency for proxy %d must be <= 10000", binding.ProxyID)
		}
		if _, exists := seen[binding.ProxyID]; exists {
			return fmt.Errorf("proxy_pool contains duplicate proxy %d", binding.ProxyID)
		}
		seen[binding.ProxyID] = struct{}{}
	}
	return nil
}
