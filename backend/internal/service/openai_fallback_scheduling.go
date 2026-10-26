package service

import (
	"context"
	"errors"
	"sort"
)

func partitionOpenAIFallbackAccounts(accounts []*Account) (regular, standby []*Account) {
	for _, account := range accounts {
		if account == nil {
			continue
		}
		if account.IsFallback {
			standby = append(standby, account)
		} else {
			regular = append(regular, account)
		}
	}
	return
}

func hasOpenAIFallbackAccount(accounts []Account) bool {
	for _, account := range accounts {
		if account.IsFallback {
			return true
		}
	}
	return false
}

func regularOpenAIAccountsFirst(accounts []*Account) {
	sort.SliceStable(accounts, func(i, j int) bool { return !accounts[i].IsFallback && accounts[j].IsFallback })
}

// A cached full primary must not keep sending new requests to standby after
// capacity returns. Refresh only on this saturation boundary, not on every pick.
func (s *OpenAIGatewayService) refreshSaturatedOpenAIPrimaryLoad(ctx context.Context, accounts []*Account, load map[int64]*AccountLoadInfo) map[int64]*AccountLoadInfo {
	regular, standby := partitionOpenAIFallbackAccounts(accounts)
	if s.concurrencyService == nil || len(regular) == 0 || len(standby) == 0 {
		return load
	}
	for _, account := range regular {
		if info := load[account.ID]; info == nil || info.LoadRate < 100 {
			return load
		}
	}
	if fresh, err := s.concurrencyService.GetAccountsLoadBatchFresh(ctx, buildOpenAIAccountLoadRequest(accounts)); err == nil {
		return fresh
	}
	return load
}

// Standby is overflow capacity, not a health-only failover pool. Do not commit
// to a primary wait plan before trying eligible standby slots. BPS, subscription,
// quota and Top-K preferences remain inside each role's existing scheduler.
func (s *defaultOpenAIAccountScheduler) selectByOpenAIFallbackPools(ctx context.Context, req OpenAIAccountScheduleRequest, accounts []*Account, load map[int64]*AccountLoadInfo, budget *openAISelectionProbeBudget, stats openAISelectionFilterStats) (*AccountSelectionResult, int, int, float64, error) {
	regular, standby := partitionOpenAIFallbackAccounts(accounts)
	if len(regular) == 0 || len(standby) == 0 {
		return s.selectByLoadBalancePoolPreferences(ctx, req, accounts, load, budget, stats)
	}
	req.checkPrimaryCapacity = true
	primary, count, topK, skew, err := s.selectByLoadBalancePoolPreferences(ctx, req, regular, load, budget, stats)
	if primary != nil && primary.Acquired {
		return primary, count, topK, skew, err
	}
	if err != nil && !errors.Is(err, ErrNoAvailableAccounts) && !errors.Is(err, ErrNoAvailableCompactAccounts) {
		return nil, count, topK, skew, err
	}
	// Preserve the per-pool probe cap: a saturated large primary pool must not
	// consume the standby pool's entire opportunity to perform atomic admission.
	backup, backupCount, backupTopK, backupSkew, backupErr := s.selectByLoadBalancePoolPreferences(ctx, req, standby, load, newOpenAISelectionProbeBudget(), stats)
	if backup != nil && backup.Acquired {
		return backup, backupCount, backupTopK, backupSkew, backupErr
	}
	if backupErr != nil && !errors.Is(backupErr, ErrNoAvailableAccounts) && !errors.Is(backupErr, ErrNoAvailableCompactAccounts) {
		return nil, backupCount, backupTopK, backupSkew, backupErr
	}
	if primary != nil {
		return primary, count, topK, skew, err
	}
	return backup, backupCount, backupTopK, backupSkew, backupErr
}
