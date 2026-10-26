package service

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"sync"
	"sync/atomic"
	"time"

	"github.com/tidwall/gjson"
)

var ErrGPTQoSUnavailable = errors.New("GPT QoS admission unavailable")
var ErrGPTQualityUnavailable = errors.New("no eligible quality pool capacity within request budget")

type gptQoSRequestKey struct{}
type gptQualityConstraintKey struct{}
type gptQoSRequestState struct {
	mu                                                                       sync.Mutex
	config                                                                   GPTQoSConfig
	group                                                                    GPTQoSGroupConfig
	started                                                                  time.Time
	strongDeadline                                                           time.Time
	attempts                                                                 atomic.Int32
	selectionAttempts                                                        atomic.Int32
	outcomeKnown, lastSucceeded                                              bool
	upstreamKind                                                             string
	accountType, submittedAccountType                                        string
	queueWait                                                                time.Duration
	selectionDuration                                                        time.Duration
	accountID                                                                int64
	quality, pool, fallback, sticky, strategy, model, effectiveModel, effort string
	peak, downgraded                                                         bool
}

func (s *OpenAIGatewayService) gptQoSConfigForContext(ctx context.Context) GPTQoSConfig {
	if state, _ := ctx.Value(gptQoSRequestKey{}).(*gptQoSRequestState); state != nil {
		return state.config
	}
	if s == nil {
		return DefaultGPTQoSConfig()
	}
	return s.settingService.gptQoSRuntimeConfig()
}
func (s *OpenAIGatewayService) qualityRoutingEnabled(ctx context.Context, req OpenAIAccountScheduleRequest) bool {
	return req.Platform == PlatformOpenAI && req.RequiredImageCapability == "" && req.UseUpstreamTokenCost && s.gptQoSConfigForContext(ctx).QualityRoutingEnabled && s.gptQoSConfigForContext(ctx).Applies(req.GroupID)
}

// PrepareGPTQoSRequest freezes policy and deadline across every failover attempt.
// The body is parsed once for non-sensitive labels and is never retained.
func (s *OpenAIGatewayService) PrepareGPTQoSRequest(ctx context.Context, groupID *int64, body []byte) (context.Context, func(bool)) {
	cfg := s.gptQoSConfigForContext(ctx)
	if !cfg.QualityRoutingEnabled || !cfg.Applies(groupID) {
		return ctx, func(bool) {}
	}
	group := cfg.Group(groupID)
	now := time.Now()
	state := &gptQoSRequestState{config: cfg, group: group, started: now, strongDeadline: time.Time{}, model: truncateQoSLabel(gjson.GetBytes(body, "model").String(), 128), effort: gjson.GetBytes(body, "reasoning.effort").String()}
	if state.effort == "" {
		state.effort = gjson.GetBytes(body, "reasoning_effort").String()
	}
	ctx = context.WithValue(ctx, gptQoSRequestKey{}, state)
	ctx = context.WithValue(ctx, gptQoSGroupIDKey{}, derefGroupID(groupID))
	cancel := func() {}
	if cfg.Enabled {
		budget := cfg.RequestBudgetMs
		if state.effort == "high" || state.effort == "xhigh" || state.effort == "max" {
			budget = cfg.HighEffortBudgetMs
		}
		ctx, cancel = context.WithTimeoutCause(ctx, time.Duration(budget)*time.Millisecond, ErrGPTQoSRequestDeadline)
	}
	if cfg.Enabled {
		timedCancel := cancel
		controlled, cancelCause := context.WithCancelCause(ctx)
		ctx = controlled
		cancel = func() { cancelCause(context.Canceled); timedCancel() }
		ctx = context.WithValue(ctx, gptQoSCancelKey{}, context.CancelFunc(func() { cancelCause(ErrGPTQoSLeaseLost) }))
	}
	return ctx, func(success bool) { defer cancel(); s.finishGPTQoSRequest(ctx, groupID, state, success) }
}

func (s *defaultOpenAIAccountScheduler) qualityPoolOrder(ctx context.Context, req OpenAIAccountScheduleRequest, pool []*Account, load map[int64]*AccountLoadInfo) []openAIAccountCandidateScore {
	req.StickyWeighted = false
	build := func(accounts []*Account) []openAIAccountCandidateScore {
		plan := s.buildOpenAIAccountLoadPlan(ctx, req, accounts, load)
		plan.includeOverflowFallback = true
		return s.buildOpenAISelectionOrder(req, plan)
	}
	var order []openAIAccountCandidateScore
	if req.SubscriptionPriority {
		sub, regular := partitionOpenAIChatGPTSubscriptionAccounts(pool)
		order = append(build(sub), build(regular)...)
	} else {
		order = build(pool)
	}
	// Quality is an explicit account label. Within that pool, load and queue
	// pressure precede cost, protocol, subscription and account-score tastes.
	// Idle peers use last-used order; the distributed queue rechecks live slots
	// and rotates physical roots atomically, independent of this snapshot.
	sort.SliceStable(order, func(i, j int) bool {
		a, b := order[i], order[j]
		if a.account.IsFallback != b.account.IsFallback {
			return !a.account.IsFallback
		}
		if a.loadInfo.LoadRate != b.loadInfo.LoadRate {
			return a.loadInfo.LoadRate < b.loadInfo.LoadRate
		}
		if a.loadInfo.WaitingCount != b.loadInfo.WaitingCount {
			return a.loadInfo.WaitingCount < b.loadInfo.WaitingCount
		}
		if a.account.LastUsedAt == nil || b.account.LastUsedAt == nil {
			return a.account.LastUsedAt == nil && b.account.LastUsedAt != nil
		}
		return a.account.LastUsedAt.Before(*b.account.LastUsedAt)
	})
	return order
}

func splitGPTQualityPools(accounts []*Account) (strong, basic []*Account) {
	for _, a := range accounts {
		switch a.QualityTier() {
		case "strong":
			strong = append(strong, a)
		case "basic":
			basic = append(basic, a)
		}
	}
	return
}

func (s *defaultOpenAIAccountScheduler) selectQualityPools(ctx context.Context, req OpenAIAccountScheduleRequest, accounts []*Account, load map[int64]*AccountLoadInfo) (*AccountSelectionResult, int, int, float64, error) {
	started := time.Now()
	cfg := s.service.gptQoSConfigForContext(ctx)
	group := cfg.Group(req.GroupID)
	state, _ := ctx.Value(gptQoSRequestKey{}).(*gptQoSRequestState)
	deadline := started.Add(time.Duration(group.StrongWaitMs) * time.Millisecond)
	if state != nil {
		state.mu.Lock()
		if state.strongDeadline.IsZero() {
			state.strongDeadline = deadline
		}
		deadline = state.strongDeadline
		state.mu.Unlock()
	}
	strong, basic := splitGPTQualityPools(accounts)
	strongOrder := s.qualityPoolOrder(ctx, req, strong, load)
	var basicOrder []openAIAccountCandidateScore
	if group.AllowBasicFallback {
		basicOrder = s.qualityPoolOrder(ctx, req, basic, load)
	}
	if cfg.Enabled {
		return s.selectGPTQoSQueue(ctx, req, cfg, group, strongOrder, basicOrder, deadline)
	}
	budget := newOpenAISelectionProbeBudget()
	budget.enableLimit()
	for {
		result, _, err := s.tryAcquireOpenAISelectionOrderWithBudget(context.WithValue(ctx, gptQualityConstraintKey{}, "strong"), req, strongOrder, budget)
		if err != nil {
			return nil, len(accounts), len(strongOrder), 0, err
		}
		if result != nil {
			s.recordGPTQualitySelection(ctx, req, result, "strong", false, started)
			return result, len(accounts), len(strongOrder), 0, nil
		}
		if len(strongOrder) == 0 || !time.Now().Before(deadline) {
			break
		}
		if !waitGPTQoS(ctx, 20*time.Millisecond) {
			return nil, 0, 0, 0, ctx.Err()
		}
		// Acquisition checks the authoritative slot store on each pass. Rebuild
		// load so a once-full snapshot cannot suppress newly available slots.
		if s.service.concurrencyService != nil {
			if fresh, err := s.service.concurrencyService.GetAccountsLoadBatchFresh(ctx, buildOpenAIAccountLoadRequest(strong)); err == nil {
				strongOrder = s.qualityPoolOrder(ctx, req, strong, fresh)
			}
		}
		budget = newOpenAISelectionProbeBudget()
		budget.enableLimit()
	}
	if group.AllowBasicFallback {
		result, _, err := s.tryAcquireOpenAISelectionOrderWithBudget(context.WithValue(ctx, gptQualityConstraintKey{}, "basic"), req, basicOrder, budget)
		if err != nil {
			return nil, len(accounts), len(basicOrder), 0, err
		}
		if result != nil {
			s.recordGPTQualitySelection(ctx, req, result, "basic", true, started)
			return result, len(accounts), len(basicOrder), 0, nil
		}
	}
	return nil, len(accounts), len(strongOrder), 0, fmt.Errorf("%w: %w", ErrNoAvailableAccounts, ErrGPTQualityUnavailable)
}
func waitGPTQoS(ctx context.Context, d time.Duration) bool {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-t.C:
		return true
	}
}
func (s *defaultOpenAIAccountScheduler) recordGPTQualitySelection(ctx context.Context, req OpenAIAccountScheduleRequest, result *AccountSelectionResult, quality string, downgraded bool, started time.Time) {
	if state, _ := ctx.Value(gptQoSRequestKey{}).(*gptQoSRequestState); state != nil {
		state.mu.Lock()
		defer state.mu.Unlock()
		state.accountID = result.Account.ID
		state.accountType = result.Account.Type
		state.upstreamKind, _ = result.Account.Extra["qos_upstream_kind"].(string)
		if state.upstreamKind == "" && result.Account.IsOpenAIOAuth() {
			state.upstreamKind = "direct"
		}
		state.strategy = "quality_pool_existing_scheduler"
		state.quality = quality
		state.pool = result.Account.QoSResourcePool()
		state.downgraded = state.downgraded || downgraded
		state.selectionDuration += time.Since(started)
		state.effectiveModel = resolveOpenAIAccountUpstreamModelForRequest(result.Account, req.RequestedModel, req.RequireCompact)
		if downgraded {
			state.fallback = "strong_wait_budget_exhausted"
		}
	}
}
func (s *OpenAIGatewayService) finishGPTQoSRequest(ctx context.Context, groupID *int64, state *gptQoSRequestState, success bool) {
	state.mu.Lock()
	defer state.mu.Unlock()
	if state.outcomeKnown {
		success = success && state.lastSucceeded
	}
	s.recordGPTQoSMetrics(ctx, groupID, state, success)
}
