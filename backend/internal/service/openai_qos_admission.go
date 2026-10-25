package service

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/ctxkey"
)

// UpstreamCapacityProvider is intentionally transport-neutral. No VM2API HTTP
// management API is assumed and no model probes are sent. A future provider
// must identify shared physical resources, source time and freshness explicitly.
type UpstreamCapacityProvider interface {
	Capacity(context.Context, string) (UpstreamCapacitySnapshot, error)
}
type UpstreamCapacitySnapshot struct {
	PoolID                                                                               string
	EffectiveConcurrency, ActiveConcurrency, AvailableSlots, QueuedRequests, RPMCapacity int
	QuotaStatus, CooldownStatus                                                          string
	UpdatedAt                                                                            time.Time
}

func ReliableUpstreamCapacity(v UpstreamCapacitySnapshot, now time.Time) bool {
	return v.PoolID != "" && v.EffectiveConcurrency > 0 && v.ActiveConcurrency >= 0 && v.AvailableSlots >= 0 && v.AvailableSlots <= v.EffectiveConcurrency && !v.UpdatedAt.After(now) && now.Sub(v.UpdatedAt) <= 30*time.Second
}

type GPTQoSCandidate struct {
	AccountID    int64  `json:"account_id,string"`
	SlotID       int64  `json:"slot_id,string"`
	Concurrency  int    `json:"concurrency"`
	PoolID       string `json:"pool_id"`
	PoolCapacity int    `json:"pool_capacity"`
	Quality      string `json:"quality"`
}
type GPTQoSEntry struct {
	ID                                          string `json:"id"`
	UserID                                      int64  `json:"user_id,string"`
	GroupID                                     int64  `json:"group_id,string"`
	Tier                                        string `json:"tier"`
	Weight                                      int    `json:"weight"`
	GroupLimit                                  int    `json:"group_limit"`
	MaxWaiting, GroupMaxWaiting, UserMaxWaiting int
	WaitMs, StrongWaitMs                        int
	Candidates                                  []GPTQoSCandidate `json:"candidates"`
	Peak                                        GPTQoSPeakConfig  `json:"peak"`
	Protected                                   bool              `json:"protected"`
	Weights                                     [3]int            `json:"weights"`
}
type GPTQoSGrant struct {
	Status    string          `json:"status"`
	Candidate GPTQoSCandidate `json:"candidate"`
	Peak      bool            `json:"peak"`
	Reserved  int             `json:"reserved"`
	WaitMs    int64           `json:"wait_ms"`
}

// Admission and the existing Redis account slot are acquired in one atomic
// operation. There is no second semaphore or unowned capacity reservation.
type GPTQoSAdmissionStore interface {
	GPTQoSPoll(context.Context, GPTQoSEntry) (GPTQoSGrant, error)
	GPTQoSClose(context.Context, string) error
	GPTQoSRefresh(context.Context, string) (bool, error)
	GPTQoSState(context.Context) (map[string]any, error)
	GPTQoSRecord(context.Context, GPTQoSMetric) error
	GPTQoSMetrics(context.Context, GPTQoSMetricFilter) ([]GPTQoSMetric, error)
}

func (s *OpenAIGatewayService) gptQoSStore() GPTQoSAdmissionStore {
	if s == nil || s.concurrencyService == nil {
		return nil
	}
	store, _ := s.concurrencyService.cache.(GPTQoSAdmissionStore)
	return store
}
func qosCandidate(account *Account, cfg GPTQoSConfig) (GPTQoSCandidate, bool) {
	if account == nil || account.Concurrency <= 0 {
		return GPTQoSCandidate{}, false
	}
	// Proxy pools have per-binding slots and cannot be flattened to account.ID.
	// Until a binding is selected, leave them out of guaranteed capacity.
	if len(account.ProxyPool) > 0 || account.ProxyPoolConfigured {
		return GPTQoSCandidate{}, false
	}
	cap := account.Concurrency
	pool := account.QoSResourcePool()
	if strings.HasPrefix(pool, "shared:") {
		p, ok := cfg.ResourcePools[strings.TrimPrefix(pool, "shared:")]
		if !ok || !cfg.CapacityEnabled {
			return GPTQoSCandidate{}, false
		}
		cap = p.Concurrency
	}
	return GPTQoSCandidate{AccountID: account.ID, SlotID: GPTQoSAccountSlotID(account), Concurrency: account.Concurrency, PoolID: pool, PoolCapacity: cap, Quality: account.QualityTier()}, true
}
func (s *defaultOpenAIAccountScheduler) selectGPTQoSQueue(ctx context.Context, req OpenAIAccountScheduleRequest, cfg GPTQoSConfig, group GPTQoSGroupConfig, strong, basic []openAIAccountCandidateScore, deadline time.Time) (*AccountSelectionResult, int, int, float64, error) {
	store := s.service.gptQoSStore()
	if store == nil {
		return nil, 0, 0, 0, fmt.Errorf("%w: %w", ErrNoAvailableAccounts, ErrGPTQoSUnavailable)
	}
	uid, _ := ctx.Value(ctxkey.UserID).(int64)
	if uid <= 0 {
		return nil, 0, 0, 0, fmt.Errorf("%w: QoS requires authenticated user", ErrNoAvailableAccounts)
	}
	entry := GPTQoSEntry{ID: generateRequestID(), UserID: uid, GroupID: derefGroupID(req.GroupID), Tier: group.Tier, Weight: group.Weight, GroupLimit: group.Concurrency, MaxWaiting: cfg.MaxWaiting, GroupMaxWaiting: group.MaxWaiting, UserMaxWaiting: group.UserMaxWaiting, WaitMs: max(1, int(time.Until(deadline).Milliseconds())), StrongWaitMs: max(0, int(time.Until(deadline).Milliseconds())), Peak: cfg.Peak, Protected: group.CapacityProtection, Weights: [3]int{cfg.PremiumWeight, cfg.StandardWeight, cfg.EconomyWeight}}
	accounts := map[int64]*Account{}
	for _, pool := range [][]openAIAccountCandidateScore{strong, basic} {
		for _, item := range pool {
			if c, ok := qosCandidate(item.account, cfg); ok {
				entry.Candidates = append(entry.Candidates, c)
				accounts[item.account.ID] = item.account
			}
		}
	}
	if len(strong) == 0 {
		entry.StrongWaitMs = 0
	}
	if group.AllowBasicFallback && !req.PreserveStickyBinding {
		entry.WaitMs += group.BasicWaitMs
	}
	if req.PreserveStickyBinding {
		entry.StrongWaitMs = 0
	}
	if len(entry.Candidates) == 0 {
		return nil, 0, 0, 0, fmt.Errorf("%w: no bounded eligible quality capacity", ErrNoAvailableAccounts)
	}
	if len(entry.Candidates) > 256 {
		return nil, 0, 0, 0, fmt.Errorf("%w: QoS candidate limit exceeded", ErrNoAvailableAccounts)
	}
	if deadline, ok := ctx.Deadline(); ok {
		entry.WaitMs = min(entry.WaitMs, max(1, int(time.Until(deadline).Milliseconds())))
	}
	started := time.Now()
	defer func() {
		if state, _ := ctx.Value(gptQoSRequestKey{}).(*gptQoSRequestState); state != nil {
			state.mu.Lock()
			state.queueWait += time.Since(started)
			state.mu.Unlock()
		}
	}()
	initialWaitMs := entry.WaitMs
	rechecks := 0
	owned := false
	defer func() {
		if !owned {
			closeGPTQoSEntry(store, entry.ID)
		}
	}()
	for {
		grant, err := store.GPTQoSPoll(ctx, entry)
		if err != nil {
			slog.Error("gpt_qos_redis_unavailable", "operation", "admission")
			return nil, 0, 0, 0, fmt.Errorf("%w: %w", ErrNoAvailableAccounts, ErrGPTQoSUnavailable)
		}
		switch grant.Status {
		case "granted":
			account := accounts[grant.Candidate.AccountID]
			qctx := context.WithValue(ctx, gptQualityConstraintKey{}, grant.Candidate.Quality)
			fresh := s.service.resolveFreshSchedulableOpenAIAccount(qctx, account, req.GroupID, req.Platform, req.RequestedModel, false, req.RequiredCapability, req.RequireCompact)
			fresh = s.service.recheckSelectedOpenAIAccountFromDB(qctx, fresh, req.GroupID, req.Platform, req.RequestedModel, false, req.RequiredCapability, req.RequireCompact)
			if fresh == nil || fresh.Concurrency != grant.Candidate.Concurrency || GPTQoSAccountSlotID(fresh) != grant.Candidate.SlotID || len(fresh.ProxyPool) > 0 || fresh.ProxyPoolConfigured || fresh.QoSResourcePool() != grant.Candidate.PoolID || !s.isAccountRequestCompatible(qctx, fresh, req) || !s.isAccountTransportCompatible(fresh, req.RequiredTransport, req.RequestedModel) || (req.RequireCompact && openAICompactSupportTier(fresh) == 0) {
				closeGPTQoSEntry(store, entry.ID)
				rechecks++
				next := make([]GPTQoSCandidate, 0, len(entry.Candidates)-1)
				for _, candidate := range entry.Candidates {
					if candidate.AccountID != grant.Candidate.AccountID {
						next = append(next, candidate)
					}
				}
				entry.Candidates = next
				entry.WaitMs = initialWaitMs - int(time.Since(started).Milliseconds())
				entry.StrongWaitMs = max(0, int(time.Until(deadline).Milliseconds()))
				if len(next) == 0 || rechecks >= cfg.MaxAttempts || entry.WaitMs <= 0 {
					return nil, 0, 0, 0, fmt.Errorf("%w: QoS selected account changed", ErrNoAvailableAccounts)
				}
				entry.ID = generateRequestID()
				continue
			}
			if bindings := fresh.ProxyBindings(); len(bindings) == 1 {
				fresh.setSelectedProxy(bindings[0])
			}
			result := &AccountSelectionResult{Account: fresh, Acquired: true, AccountRequestID: entry.ID}
			result.ReleaseFunc = keepGPTQoSGrant(ctx, store, entry.ID)
			owned = true
			if req.SessionHash != "" && !req.PreserveStickyBinding {
				_ = s.service.bindOpenAIStickySessionDuringSelection(ctx, req.GroupID, req.SessionHash, fresh.ID)
			}
			s.recordGPTQualitySelection(ctx, req, result, fresh.QualityTier(), fresh.QualityTier() != "strong" && !req.PreserveStickyBinding, started)
			if state, _ := ctx.Value(gptQoSRequestKey{}).(*gptQoSRequestState); state != nil {
				state.mu.Lock()
				state.strategy = "quality_pool_global_weighted_fair_queue"
				if req.PreserveStickyBinding {
					state.sticky = "hard_continuation_owner"
				}
				state.peak = grant.Peak
				state.mu.Unlock()
			}
			return attachSelectionProfitGate(ctx, result), len(entry.Candidates), len(entry.Candidates), 0, nil
		case "full", "timeout", "closed":
			return nil, 0, 0, 0, fmt.Errorf("%w: QoS queue %s", ErrNoAvailableAccounts, grant.Status)
		}
		if !group.AllowBasicFallback && !time.Now().Before(deadline) {
			return nil, 0, 0, 0, fmt.Errorf("%w: strong wait budget exhausted", ErrNoAvailableAccounts)
		}
		if !waitGPTQoS(ctx, min(25*time.Millisecond, time.Duration(max(1, entry.WaitMs))*time.Millisecond)) {
			return nil, 0, 0, 0, ctx.Err()
		}
	}
}
func closeGPTQoSEntry(store GPTQoSAdmissionStore, id string) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := store.GPTQoSClose(ctx, id); err != nil {
		slog.Error("gpt_qos_release_failed", "operation", "close")
	}
}
func keepGPTQoSGrant(ctx context.Context, store GPTQoSAdmissionStore, id string) func() {
	stop := make(chan struct{})
	var once sync.Once
	release := func() { once.Do(func() { close(stop); closeGPTQoSEntry(store, id) }) }
	go func() {
		ticker := time.NewTicker(10 * time.Second)
		defer ticker.Stop()
		done := ctx.Done()
		for {
			select {
			case <-stop:
				return
			case <-done:
				// The forwarding owner releases only after upstream work stops.
				// Continue renewal while cancellation drains an SSE/WS turn.
				done = nil
			case <-ticker.C:
				check, cancel := context.WithTimeout(context.Background(), 2*time.Second)
				ok, err := store.GPTQoSRefresh(check, id)
				cancel()
				if err != nil || !ok {
					slog.Error("gpt_qos_lease_lost", "operation", "refresh")
					if state, _ := ctx.Value(gptQoSCancelKey{}).(context.CancelFunc); state != nil {
						state()
					}
					return
				}
			}
		}
	}()
	return release
}

// AcquireGPTQoSBoundAccount keeps subsequent stateful WS turns on their owner,
// but subjects them to the same distributed queue and account semaphore.
func (s *OpenAIGatewayService) AcquireGPTQoSBoundAccount(ctx context.Context, account *Account) (*AcquireResult, bool, error) {
	state, _ := ctx.Value(gptQoSRequestKey{}).(*gptQoSRequestState)
	if state == nil || !state.config.Enabled {
		return nil, false, nil
	}
	state.selectionAttempts.Add(1)
	r, err := s.acquireGPTQoSOwner(ctx, account)
	return r, true, err
}

type gptQoSCancelKey struct{}

// Called for stateful continuations, which may use basic/unknown owners but
// must still enter the same global fairness and capacity admission.
func (s *OpenAIGatewayService) acquireGPTQoSOwner(ctx context.Context, account *Account) (*AcquireResult, error) {
	state, _ := ctx.Value(gptQoSRequestKey{}).(*gptQoSRequestState)
	if state == nil || !state.config.Enabled {
		return nil, nil
	}
	groupID := stateGroupID(ctx)
	scheduler := &defaultOpenAIAccountScheduler{service: s, stats: s.openaiAccountStats}
	req := OpenAIAccountScheduleRequest{GroupID: &groupID, Platform: PlatformOpenAI, RequestedModel: state.model, UseUpstreamTokenCost: true, PreserveStickyBinding: true}
	if forward, ok := openAIForwardModelFromContext(ctx); ok {
		req.RequestedModel, req.RequireCompact = forward.model, forward.useCompactModelMapping
	}
	item := []openAIAccountCandidateScore{{account: account}}
	// Owners are not downgraded: the one compatible account is the only option.
	selection, _, _, _, err := scheduler.selectGPTQoSQueue(ctx, req, state.config, state.group, item, nil, time.Now().Add(time.Duration(state.group.StrongWaitMs)*time.Millisecond))
	if err != nil {
		return nil, err
	}
	if bindings := account.ProxyBindings(); len(bindings) == 1 {
		account.setSelectedProxy(bindings[0])
	}
	return &AcquireResult{Acquired: true, SlotID: GPTQoSAccountSlotID(account), RequestID: selection.AccountRequestID, ReleaseFunc: selection.ReleaseFunc}, nil
}

type gptQoSGroupIDKey struct{}

func stateGroupID(ctx context.Context) int64 {
	id, _ := ctx.Value(gptQoSGroupIDKey{}).(int64)
	return id
}
