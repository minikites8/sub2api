package service

import (
	"context"
	"encoding/json"
	"math"
	"sync"
	"time"
)

// GPTAutoBudgetLimits are the daily classifier limits. Zero means unlimited.
type GPTAutoBudgetLimits struct {
	Calls       int64
	CostNanoUSD int64
}

// GPTAutoBudgetUsage is one UTC day's classifier usage across all nodes.
type GPTAutoBudgetUsage struct {
	Calls       int64 `json:"calls"`
	CostNanoUSD int64 `json:"cost_nano_usd"`
}

// CostUSD converts the stored integer cost back to dollars.
func (u GPTAutoBudgetUsage) CostUSD() float64 {
	return float64(u.CostNanoUSD) / gptAutoNanoPerUSD
}

const gptAutoNanoPerUSD = 1e9

func gptAutoUSDToNano(usd float64) int64 {
	if usd <= 0 || math.IsNaN(usd) {
		return 0
	}
	return int64(math.Ceil(usd * gptAutoNanoPerUSD))
}

// GPTAutoBudgetStore enforces the daily Jev budgets for the whole cluster and
// aggregates per-node counters. The production implementation is Redis-backed
// (repository.NewGPTAutoBudgetStore) so limits do not multiply with the number
// of gateway instances. Days are UTC and formatted 2006-01-02.
type GPTAutoBudgetStore interface {
	// Reserve atomically admits one classifier call, booking costNanoUSD, only
	// if the day's totals stay within limits. It returns the totals after the
	// attempt and whether the call was admitted.
	Reserve(ctx context.Context, day string, limits GPTAutoBudgetLimits, costNanoUSD int64) (GPTAutoBudgetUsage, bool, error)
	// Adjust corrects a reservation: the cost delta settles the estimate to
	// the billed amount, and a call delta of -1 refunds a call never made.
	Adjust(ctx context.Context, day string, callDelta, costDeltaNanoUSD int64) error
	// Seed initialises a day from persisted decisions unless it already exists.
	Seed(ctx context.Context, day string, usage GPTAutoBudgetUsage) error
	// Usage returns the day's totals and whether the day exists.
	Usage(ctx context.Context, day string) (GPTAutoBudgetUsage, bool, error)
	AddCounters(ctx context.Context, day string, deltas map[string]int64) error
	Counters(ctx context.Context, day string) (map[string]int64, error)
}

// localGPTAutoBudgetStore is the single-process fallback used when no shared
// store is wired (tests, tooling). It is not safe for multi-instance budgets.
type localGPTAutoBudgetStore struct {
	mu       sync.Mutex
	usage    map[string]GPTAutoBudgetUsage
	counters map[string]map[string]int64
}

func newLocalGPTAutoBudgetStore() *localGPTAutoBudgetStore {
	return &localGPTAutoBudgetStore{usage: map[string]GPTAutoBudgetUsage{}, counters: map[string]map[string]int64{}}
}

func (s *localGPTAutoBudgetStore) Reserve(_ context.Context, day string, limits GPTAutoBudgetLimits, cost int64) (GPTAutoBudgetUsage, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	usage := s.usage[day]
	if limits.Calls > 0 && usage.Calls+1 > limits.Calls {
		return usage, false, nil
	}
	if limits.CostNanoUSD > 0 && usage.CostNanoUSD+cost > limits.CostNanoUSD {
		return usage, false, nil
	}
	usage.Calls++
	usage.CostNanoUSD += cost
	s.usage[day] = usage
	return usage, true, nil
}

func (s *localGPTAutoBudgetStore) Adjust(_ context.Context, day string, callDelta, costDelta int64) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	usage := s.usage[day]
	usage.Calls += callDelta
	usage.CostNanoUSD += costDelta
	s.usage[day] = usage
	return nil
}

func (s *localGPTAutoBudgetStore) Seed(_ context.Context, day string, usage GPTAutoBudgetUsage) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.usage[day]; !ok {
		s.usage[day] = usage
	}
	return nil
}

func (s *localGPTAutoBudgetStore) Usage(_ context.Context, day string) (GPTAutoBudgetUsage, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	usage, ok := s.usage[day]
	return usage, ok, nil
}

func (s *localGPTAutoBudgetStore) AddCounters(_ context.Context, day string, deltas map[string]int64) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.counters[day] == nil {
		s.counters[day] = map[string]int64{}
	}
	for name, delta := range deltas {
		s.counters[day][name] += delta
	}
	return nil
}

func (s *localGPTAutoBudgetStore) Counters(_ context.Context, day string) (map[string]int64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make(map[string]int64, len(s.counters[day]))
	for name, value := range s.counters[day] {
		out[name] = value
	}
	return out, nil
}

func gptAutoBudgetLimitsFor(cfg GPTAutoShadowConfig) GPTAutoBudgetLimits {
	return GPTAutoBudgetLimits{
		Calls:       int64(cfg.ClassifierDailyCallLimit),
		CostNanoUSD: gptAutoUSDToNano(cfg.ClassifierDailyCostLimit),
	}
}

func (l GPTAutoBudgetLimits) exhaustedBy(usage GPTAutoBudgetUsage) bool {
	return (l.Calls > 0 && usage.Calls >= l.Calls) || (l.CostNanoUSD > 0 && usage.CostNanoUSD >= l.CostNanoUSD)
}

// gptAutoBudgetView is the last observed cluster usage. The gateway hook only
// reads it (no I/O); workers and the maintenance loop refresh it.
type gptAutoBudgetView struct {
	day   string
	usage GPTAutoBudgetUsage
}

// gptAutoReservation is one admitted classifier call.
type gptAutoReservation struct {
	day      string
	costNano int64
}

// Errors from budget admission. They are never persisted as decision rows when
// they happen before the first Jev call.
var (
	errGPTAutoBudgetExhausted   = &GPTAutoClassifierError{Class: GPTAutoErrBudgetExhausted}
	errGPTAutoBudgetUnavailable = &GPTAutoClassifierError{Class: GPTAutoErrBudgetUnavailable}
)

const gptAutoBudgetOpTimeout = 2 * time.Second

// reserveBudget admits one Jev call against the cluster-wide daily limits.
// When a limit is configured and the shared store cannot be reached it fails
// closed: shadow work is disposable, an unbounded bill is not.
func (s *GPTAutoShadowService) reserveBudget(ctx context.Context, cfg GPTAutoShadowConfig, estimateNano int64) (*gptAutoReservation, error) {
	day := gptAutoDayStart(s.now()).Format(time.DateOnly)
	opCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), gptAutoBudgetOpTimeout)
	defer cancel()
	usage, allowed, err := s.budget.Reserve(opCtx, day, gptAutoBudgetLimitsFor(cfg), estimateNano)
	if err != nil {
		limits := gptAutoBudgetLimitsFor(cfg)
		if limits.Calls > 0 || limits.CostNanoUSD > 0 {
			return nil, errGPTAutoBudgetUnavailable
		}
		// Unlimited budget: the store only feeds the dashboard, so proceed.
		return &gptAutoReservation{day: day}, nil
	}
	s.budgetView.Store(&gptAutoBudgetView{day: day, usage: usage})
	if !allowed {
		return nil, errGPTAutoBudgetExhausted
	}
	return &gptAutoReservation{day: day, costNano: estimateNano}, nil
}

// settleBudget replaces the reserved estimate with the billed cost. A call
// that was never sent is refunded entirely.
func (s *GPTAutoShadowService) settleBudget(reservation *gptAutoReservation, billedUSD float64, sent bool) {
	if reservation == nil {
		return
	}
	callDelta, costDelta := int64(0), gptAutoUSDToNano(billedUSD)-reservation.costNano
	if !sent {
		callDelta, costDelta = -1, -reservation.costNano
	}
	ctx, cancel := context.WithTimeout(context.Background(), gptAutoBudgetOpTimeout)
	defer cancel()
	if err := s.budget.Adjust(ctx, reservation.day, callDelta, costDelta); err != nil {
		s.incr(gptAutoCounterBudgetError)
	}
}

// budgetLikelyExhausted is the hot-path pre-check. It never performs I/O; the
// authoritative check is the atomic reservation on the worker.
func (s *GPTAutoShadowService) budgetLikelyExhausted(cfg GPTAutoShadowConfig, now time.Time) bool {
	limits := gptAutoBudgetLimitsFor(cfg)
	if limits.Calls <= 0 && limits.CostNanoUSD <= 0 {
		return false
	}
	view := s.budgetView.Load()
	if view == nil || view.day != gptAutoDayStart(now).Format(time.DateOnly) {
		return false
	}
	return limits.exhaustedBy(view.usage)
}

// estimateClassifierCostNano is an upper-bound guess of a Jev call's cost,
// used for the reservation before the real usage is known. JSON bytes / 3 is
// deliberately pessimistic relative to typical tokenizers.
func (s *GPTAutoShadowService) estimateClassifierCostNano(cfg GPTAutoShadowConfig, snapshot GPTAutoRequestSnapshot) int64 {
	state, _ := buildGPTAutoClassifierState(snapshot)
	questions, _ := json.Marshal(buildGPTAutoQuestions(cfg))
	tokens := (len(state) + len(questions) + len(cfg.ClassifierModel) + 64 + 2) / 3
	return gptAutoUSDToNano(gptAutoClassifierCostUSD(s.pricing, cfg.ClassifierModel, tokens, gptAutoEstimatedOutputTokens))
}

// gptAutoEstimatedOutputTokens covers four short answers; output is currently
// free on jev-latest but is reserved anyway in case pricing changes.
const gptAutoEstimatedOutputTokens = 64

// refreshBudget runs off the hot path: it seeds today's counter from persisted
// decisions when the shared store has no entry, then refreshes the view.
func (s *GPTAutoShadowService) refreshBudget(ctx context.Context, seed bool) {
	now := s.now()
	day := gptAutoDayStart(now).Format(time.DateOnly)
	opCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	usage, exists, err := s.budget.Usage(opCtx, day)
	if err != nil {
		return
	}
	if !exists && seed && s.repo != nil {
		persisted, err := s.repo.UsageSince(opCtx, gptAutoDayStart(now))
		if err == nil {
			_ = s.budget.Seed(opCtx, day, GPTAutoBudgetUsage{Calls: persisted.Calls, CostNanoUSD: gptAutoUSDToNano(persisted.Cost)})
			usage, _, _ = s.budget.Usage(opCtx, day)
		}
	}
	s.budgetView.Store(&gptAutoBudgetView{day: day, usage: usage})
}

// flushCounters pushes this node's counter deltas into the shared store so the
// dashboard can show cluster-wide totals for the day.
func (s *GPTAutoShadowService) flushCounters(ctx context.Context) {
	deltas := make(map[string]int64, len(s.counters))
	for name, counter := range s.counters {
		current := counter.Load()
		if delta := current - s.flushed[name]; delta != 0 {
			deltas[name] = delta
		}
	}
	if len(deltas) == 0 {
		return
	}
	day := gptAutoDayStart(s.now()).Format(time.DateOnly)
	opCtx, cancel := context.WithTimeout(ctx, gptAutoBudgetOpTimeout)
	defer cancel()
	if err := s.budget.AddCounters(opCtx, day, deltas); err != nil {
		return
	}
	for name, delta := range deltas {
		s.flushed[name] += delta
	}
}
