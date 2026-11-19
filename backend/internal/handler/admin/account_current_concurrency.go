package admin

import (
	"context"
	"sort"
	"time"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
	"github.com/Wei-Shaw/sub2api/internal/service"
)

type accountCapacitySnapshot struct {
	slots  map[int64]int
	totals map[int64]int
}

// Some admission paths reserve the base account slot and others reserve an
// account/proxy slot, including accounts with only a legacy proxy_id.
func (h *AccountHandler) readAccountCapacity(ctx context.Context, accounts []service.Account) (*accountCapacitySnapshot, error) {
	owners := make(map[int64]int64, len(accounts))
	ids := make([]int64, 0, len(accounts))
	add := func(slot, owner int64) {
		if _, ok := owners[slot]; !ok {
			owners[slot] = owner
			ids = append(ids, slot)
		}
	}
	for i := range accounts {
		a := &accounts[i]
		add(a.ID, a.ID)
		for _, binding := range a.ProxyPool {
			add(service.AccountProxySlotID(a.ID, binding.ProxyID), a.ID)
		}
		if len(a.ProxyPool) == 0 && !a.ProxyPoolConfigured && a.ProxyID != nil {
			add(service.AccountProxySlotID(a.ID, *a.ProxyID), a.ID)
		}
	}
	slots, err := h.concurrencyService.GetAccountConcurrencySnapshot(ctx, ids)
	if err != nil {
		return nil, err
	}
	totals := make(map[int64]int, len(accounts))
	for slot, owner := range owners {
		totals[owner] += slots[slot]
	}
	return &accountCapacitySnapshot{slots: slots, totals: totals}, nil
}

func (s *accountCapacitySnapshot) apply(item *AccountWithConcurrency) {
	if s == nil || item.Account == nil {
		return
	}
	total := s.totals[item.ID]
	item.CurrentConcurrency = &total
	for i := range item.ProxyPool {
		binding := &item.ProxyPool[i]
		count := s.slots[service.AccountProxySlotID(item.ID, binding.ProxyID)]
		binding.CurrentConcurrency = &count
	}
}

// listAccountsByCurrentConcurrency applies the existing filtered/observer-scoped
// listing before sorting and pagination. Reuse one snapshot for order and display
// so a second live read cannot make the returned page contradict its own order.
func (h *AccountHandler) listAccountsByCurrentConcurrency(
	ctx context.Context, page, pageSize int,
	platform, accountType, status, search string, groupID int64, privacyMode, sortOrder string,
) ([]service.Account, int64, *accountCapacitySnapshot, error) {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	accounts, err := h.listAccountsFiltered(ctx, platform, accountType, status, search, groupID, privacyMode, "id", "asc")
	if err != nil {
		return nil, 0, nil, err
	}
	counts, err := h.readAccountCapacity(ctx, accounts)
	if err != nil {
		return nil, 0, nil, infraerrors.ServiceUnavailable("ACCOUNT_CONCURRENCY_UNAVAILABLE", "Current account concurrency is unavailable; retry or choose another sort order").WithCause(err)
	}
	desc := pagination.NormalizeSortOrder(sortOrder, pagination.SortOrderAsc) == pagination.SortOrderDesc
	sort.Slice(accounts, func(i, j int) bool {
		left, right := counts.totals[accounts[i].ID], counts.totals[accounts[j].ID]
		if left == right {
			if desc {
				return accounts[i].ID > accounts[j].ID
			}
			return accounts[i].ID < accounts[j].ID
		}
		if desc {
			return left > right
		}
		return left < right
	})
	total := int64(len(accounts))
	params := pagination.PaginationParams{Page: page, PageSize: pageSize}
	start := params.Offset()
	if start >= len(accounts) {
		return []service.Account{}, total, counts, nil
	}
	return accounts[start:min(start+params.Limit(), len(accounts))], total, counts, nil
}
