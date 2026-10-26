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
	IsFallback   bool   `json:"is_fallback,omitempty"`
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
	if gptQoSCapacityIneligibleReason(account, cfg) != "" {
		return GPTQoSCandidate{}, false
	}
	cap := account.Concurrency
	pool := account.QoSResourcePool()
	if strings.HasPrefix(pool, "shared:") {
		p := cfg.ResourcePools[strings.TrimPrefix(pool, "shared:")]
		cap = p.Concurrency
	}
	return GPTQoSCandidate{IsFallback: account.IsFallback, AccountID: account.ID, SlotID: GPTQoSAccountSlotID(account), Concurrency: account.Concurrency, PoolID: pool, PoolCapacity: cap, Quality: account.QualityTier()}, true
}

func gptQoSCapacityIneligibleReason(a *Account, cfg GPTQoSConfig) string {
	if a == nil || a.Concurrency <= 0 {
		return "unbounded_account_capacity"
	}
	// Explicit proxy pools need a per-binding lifecycle; a single legacy proxy
	// remains supported through its existing account/proxy semaphore.
	if len(a.ProxyPool) > 0 || a.ProxyPoolConfigured {
		return "proxy_pool_unsupported"
	}
	if pool := a.QoSResourcePool(); strings.HasPrefix(pool, "shared:") {
		if !cfg.CapacityEnabled {
			return "shared_capacity_disabled"
		}
		if p, ok := cfg.ResourcePools[strings.TrimPrefix(pool, "shared:")]; !ok || p.Concurrency <= 0 {
			return "shared_capacity_not_configured"
		}
	}
	return ""
}
func (s *defaultOpenAIAccountScheduler) selectGPTQoSQueue(ctx context.Context, req OpenAIAccountScheduleRequest, cfg GPTQoSConfig, group GPTQoSGroupConfig, strong, basic []openAIAccountCandidateScore, deadline time.Time) (*AccountSelectionResult, int, int, float64, error) {
	store := s.service.gptQoSStore()
	if store == nil {
		return nil, 0, 0, 0, newGPTQoSSelectionError("unavailable", "", ErrGPTQoSUnavailable)
	}
	uid, _ := ctx.Value(ctxkey.UserID).(int64)
	if uid <= 0 {
		return nil, 0, 0, 0, fmt.Errorf("%w: QoS requires authenticated user", ErrNoAvailableAccounts)
	}
	entry := GPTQoSEntry{ID: generateRequestID(), UserID: uid, GroupID: derefGroupID(req.GroupID), Tier: group.Tier, Weight: group.Weight, GroupLimit: group.Concurrency, MaxWaiting: cfg.MaxWaiting, GroupMaxWaiting: group.MaxWaiting, UserMaxWaiting: group.UserMaxWaiting, WaitMs: max(1, int(time.Until(deadline).Milliseconds())), StrongWaitMs: max(0, int(time.Until(deadline).Milliseconds())), Peak: cfg.Peak, Protected: group.CapacityProtection, Weights: [3]int{cfg.PremiumWeight, cfg.StandardWeight, cfg.EconomyWeight}}
	accounts := map[int64]*Account{}
	// Candidate lists contain sched:meta projections, which deliberately omit
	// proxy bindings. Read complete snapshots in bounded batches before queueing:
	// the queue must acquire the same semaphore as the final hydrated account.
	// GetAccounts uses Redis MGET and the existing bounded DB fallback policy.
	var full map[int64]*Account
	if s.service.schedulerSnapshot != nil {
		ids := make([]int64, 0, len(strong)+len(basic))
		for _, pool := range [][]openAIAccountCandidateScore{strong, basic} {
			for _, item := range pool {
				if item.account != nil {
					ids = append(ids, item.account.ID)
				}
			}
		}
		var err error
		full, err = s.service.schedulerSnapshot.GetAccounts(ctx, ids)
		if err != nil {
			if ctx.Err() != nil {
				return nil, 0, 0, 0, ctx.Err()
			}
			return nil, 0, 0, 0, newGPTQoSSelectionError("snapshot_unavailable", "", ErrGPTQoSUnavailable)
		}
	}
	excluded := make(map[string]int)
	for _, pool := range [][]openAIAccountCandidateScore{strong, basic} {
		for _, item := range pool {
			account := item.account
			if account == nil {
				continue
			}
			if full != nil {
				account = full[account.ID]
			}
			account = s.service.openAIAccountForGroup(ctx, req.GroupID, account)
			reason := s.gptQoSCandidateIneligibleReason(ctx, req, account, item.account.QualityTier())
			if reason != "" {
				excluded[reason]++
				continue
			}
			if c, ok := qosCandidate(account, cfg); ok {
				entry.Candidates = append(entry.Candidates, c)
				accounts[account.ID] = account
			} else {
				excluded[gptQoSCapacityIneligibleReason(account, cfg)]++
			}
		}
	}
	entry.WaitMs = max(1, int(time.Until(deadline).Milliseconds()))
	entry.StrongWaitMs = max(0, int(time.Until(deadline).Milliseconds()))
	hasStrong := false
	for _, candidate := range entry.Candidates {
		hasStrong = hasStrong || candidate.Quality == "strong"
	}
	if !hasStrong {
		entry.StrongWaitMs = 0
	}
	if group.AllowBasicFallback && !req.PreserveStickyBinding {
		entry.WaitMs += group.BasicWaitMs
	}
	if req.PreserveStickyBinding {
		entry.StrongWaitMs = 0
	}
	if len(entry.Candidates) == 0 {
		slog.WarnContext(ctx, "gpt_qos_no_eligible_capacity", "request_id", ctx.Value(ctxkey.RequestID), "group_id", entry.GroupID, "excluded", excluded)
		return nil, 0, 0, 0, newGPTQoSSelectionError("no_eligible_capacity", "", nil)
	}
	if len(entry.Candidates) > 256 {
		return nil, 0, 0, 0, newGPTQoSSelectionError("candidate_limit", "", nil)
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
	waitDeadline := started.Add(time.Duration(initialWaitMs) * time.Millisecond)
	rechecks := 0
	owned := false
	defer func() {
		if !owned {
			closeGPTQoSEntry(store, entry.ID)
		}
	}()
	for {
		if err := ctx.Err(); err != nil {
			return nil, 0, 0, 0, err
		}
		grant, err := store.GPTQoSPoll(ctx, entry)
		if err != nil {
			if ctx.Err() != nil {
				return nil, 0, 0, 0, ctx.Err()
			}
			slog.Error("gpt_qos_redis_unavailable", "operation", "admission")
			return nil, 0, 0, 0, newGPTQoSSelectionError("unavailable", "", ErrGPTQoSUnavailable)
		}
		if err := ctx.Err(); err != nil {
			return nil, 0, 0, 0, err
		}
		switch grant.Status {
		case "granted":
			account := accounts[grant.Candidate.AccountID]
			qctx := context.WithValue(ctx, gptQualityConstraintKey{}, grant.Candidate.Quality)
			fresh := s.service.resolveFreshSchedulableOpenAIAccount(qctx, account, req.GroupID, req.Platform, req.RequestedModel, false, req.RequiredCapability, req.RequireCompact)
			fresh = s.service.recheckSelectedOpenAIAccountFromDB(qctx, fresh, req.GroupID, req.Platform, req.RequestedModel, false, req.RequiredCapability, req.RequireCompact)
			reason := s.gptQoSGrantChangedReason(qctx, req, fresh, grant.Candidate)
			if reason != "" {
				slog.WarnContext(ctx, "gpt_qos_selected_account_changed", "request_id", ctx.Value(ctxkey.RequestID), "group_id", entry.GroupID, "account_id", grant.Candidate.AccountID, "reason", reason)
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
					return nil, 0, 0, 0, newGPTQoSSelectionError("account_changed", reason, nil)
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
		case "closed":
			// Older instances may have reaped this entry using the former shared
			// close marker. Never re-enqueue it or reset its fairness/budget.
			if !time.Now().Before(waitDeadline) {
				return nil, 0, 0, 0, newGPTQoSSelectionError("queue_timeout", "expired_close_marker", nil)
			}
			return nil, 0, 0, 0, newGPTQoSSelectionError("queue_closed", "closed_before_wait_deadline", nil)
		case "full", "timeout":
			return nil, 0, 0, 0, newGPTQoSSelectionError("queue_"+grant.Status, "", nil)
		}
		if !time.Now().Before(waitDeadline) {
			return nil, 0, 0, 0, newGPTQoSSelectionError("queue_timeout", "", nil)
		}
		if !waitGPTQoS(ctx, min(25*time.Millisecond, time.Until(waitDeadline))) {
			return nil, 0, 0, 0, ctx.Err()
		}
	}
}

// Model mapping, group authorization and endpoint compatibility are checked
// before an account enters the fair queue. Recheck after hydration too: cached
// metadata can be older than the complete snapshot used for slot identity.
func (s *defaultOpenAIAccountScheduler) gptQoSCandidateIneligibleReason(ctx context.Context, req OpenAIAccountScheduleRequest, a *Account, quality string) string {
	if a == nil || !a.IsSchedulable() {
		return "account_unavailable"
	}
	if a.QualityTier() != quality {
		return "quality_tier_changed"
	}
	if a.Platform != NormalizeOpenAICompatiblePlatform(req.Platform) || !s.service.openAIAccountMatchesSchedulingGroup(a, req.GroupID) {
		return "group_or_platform_changed"
	}
	if s.service.openAIGroupRequiresPrivacySet(ctx, req.GroupID) && !a.IsPrivacySet() {
		return "privacy_not_set"
	}
	if ok, reason := s.isAccountRequestCompatibleReason(ctx, a, req); !ok {
		return reason
	}
	if !s.isAccountTransportCompatible(a, req.RequiredTransport, req.RequestedModel) {
		return "transport_incompatible"
	}
	if req.RequireCompact && openAICompactSupportTier(a) == 0 {
		return "compact_unsupported"
	}
	return ""
}

func (s *defaultOpenAIAccountScheduler) gptQoSGrantChangedReason(ctx context.Context, req OpenAIAccountScheduleRequest, a *Account, c GPTQoSCandidate) string {
	if reason := s.gptQoSCandidateIneligibleReason(ctx, req, a, c.Quality); reason != "" {
		return reason
	}
	if a.Concurrency != c.Concurrency {
		return "concurrency_changed"
	}
	if len(a.ProxyPool) > 0 || a.ProxyPoolConfigured {
		return "proxy_pool_unsupported"
	}
	if GPTQoSAccountSlotID(a) != c.SlotID {
		return "slot_binding_changed"
	}
	if a.QoSResourcePool() != c.PoolID {
		return "resource_pool_changed"
	}
	return ""
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
