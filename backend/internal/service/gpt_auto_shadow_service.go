package service

import (
	"context"
	"encoding/binary"
	"errors"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"time"
	"unicode/utf8"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/Wei-Shaw/sub2api/internal/pkg/httpclient"
	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
	"github.com/cespare/xxhash/v2"
	"github.com/tidwall/gjson"
)

// GPTAutoObserveInput is what a gateway handler hands to the shadow observer.
// Body is read synchronously and never retained. An empty RequestID is derived
// from the request context the same way usage logging derives it.
type GPTAutoObserveInput struct {
	Endpoint  string
	Body      []byte
	RequestID string
	UserID    int64
	APIKeyID  int64
	GroupID   *int64
}

// Runtime counter names (this node, since process start). The maintenance
// loop also folds them into cluster-wide per-day totals in the budget store.
const (
	gptAutoCounterObserved          = "observed"
	gptAutoCounterSampled           = "sampled"
	gptAutoCounterNotSampled        = "not_sampled"
	gptAutoCounterSkippedFilter     = "skipped_filter"
	gptAutoCounterSkippedBudget     = "skipped_budget"
	gptAutoCounterSkippedBudgetDown = "skipped_budget_unavailable"
	gptAutoCounterSkippedCircit     = "skipped_circuit_open"
	gptAutoCounterSkippedConfig     = "skipped_not_configured"
	gptAutoCounterSkippedEmpty      = "skipped_no_text"
	gptAutoCounterDropped           = "dropped_queue_full"
	gptAutoCounterSuccess           = "success"
	gptAutoCounterFailed            = "failed"
	gptAutoCounterPersistFailed     = "persist_failed"
	gptAutoCounterBudgetError       = "budget_settle_failed"
)

func gptAutoCounterNames() []string {
	return []string{
		gptAutoCounterObserved, gptAutoCounterSampled, gptAutoCounterNotSampled,
		gptAutoCounterSkippedFilter, gptAutoCounterSkippedBudget, gptAutoCounterSkippedBudgetDown,
		gptAutoCounterSkippedCircit, gptAutoCounterSkippedConfig, gptAutoCounterSkippedEmpty,
		gptAutoCounterDropped, gptAutoCounterSuccess, gptAutoCounterFailed,
		gptAutoCounterPersistFailed, gptAutoCounterBudgetError,
	}
}

// Circuit breaker tuning. A run of consecutive classifier failures opens the
// circuit so a degraded provider costs nothing until the cool-down elapses.
const (
	gptAutoBreakerThreshold = 5
	gptAutoBreakerCooldown  = 30 * time.Second
)

const (
	gptAutoMaintenanceInterval = 10 * time.Second
	gptAutoBudgetSeedInterval  = 5 * time.Minute
	gptAutoOutputHistoryTTL    = 15 * time.Minute
	gptAutoOutputHistoryWindow = 7 * 24 * time.Hour
	gptAutoCleanupInterval     = time.Hour
	gptAutoCleanupBatchSize    = 5000
	gptAutoCleanupMaxBatches   = 200
	gptAutoPersistTimeout      = 5 * time.Second
	gptAutoSampleHashPrefix    = 4096
	gptAutoSampleBuckets       = 10000
	// gptAutoMaxQueuedBytes caps the memory held by queued snapshots no matter
	// how queue_size and classification_max_context_chars are configured.
	gptAutoMaxQueuedBytes = 64 << 20
)

type gptAutoTask struct {
	snapshot   GPTAutoRequestSnapshot
	enqueuedAt time.Time
	bytes      int64
}

// gptAutoSnapshotBytes approximates the memory a queued snapshot pins.
func gptAutoSnapshotBytes(snapshot GPTAutoRequestSnapshot) int64 {
	size := 256 + len(snapshot.LatestUser) + len(snapshot.RequestedModel) + len(snapshot.RequestID)
	for _, turn := range snapshot.RecentContext {
		size += 32 + len(turn.Text)
	}
	return int64(size)
}

type gptAutoBreaker struct {
	mu          sync.Mutex
	failures    int
	openUntil   time.Time
	halfOpenUse bool
}

// allow reports whether a call may proceed. After the cool-down exactly one
// probe call is let through (half-open). A caller that got true must either
// record an outcome or call releaseProbe.
func (b *gptAutoBreaker) allow(now time.Time) bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.failures < gptAutoBreakerThreshold {
		return true
	}
	if now.Before(b.openUntil) {
		return false
	}
	if b.halfOpenUse {
		return false
	}
	b.halfOpenUse = true
	return true
}

// releaseProbe returns an unused half-open probe slot without recording an
// outcome, so a skipped call neither closes nor wedges the breaker.
func (b *gptAutoBreaker) releaseProbe() {
	b.mu.Lock()
	b.halfOpenUse = false
	b.mu.Unlock()
}

func (b *gptAutoBreaker) record(success bool, now time.Time) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.halfOpenUse = false
	if success {
		b.failures = 0
		return
	}
	b.failures++
	if b.failures >= gptAutoBreakerThreshold {
		b.openUntil = now.Add(gptAutoBreakerCooldown)
	}
}

func (b *gptAutoBreaker) state(now time.Time) string {
	b.mu.Lock()
	defer b.mu.Unlock()
	switch {
	case b.failures < gptAutoBreakerThreshold:
		return "closed"
	case now.Before(b.openUntil):
		return "open"
	default:
		return "half_open"
	}
}

type gptAutoOutputCache struct {
	mu       sync.Mutex
	loadedAt time.Time
	byPair   map[string]GPTAutoOutputEstimate
	byModel  map[string]GPTAutoOutputEstimate
}

// GPTAutoShadowService runs Phase 1 shadow classification. Nothing it does can
// alter a customer request, its routing, its response or its billing: the
// gateway hands it a read-only body, and everything after the bounded
// snapshot runs on its own workers with its own timeouts.
//
// Scope of limits: the daily call/cost budgets and the per-day counters are
// cluster-wide (budget store, Redis in production). Classifier concurrency,
// the queue and the circuit breaker are per node.
type GPTAutoShadowService struct {
	settings   *SettingService
	repo       GPTAutoShadowRepository
	classifier GPTAutoClassifier
	pricing    GPTAutoPricingLookup
	budget     GPTAutoBudgetStore
	now        func() time.Time

	queue       chan *gptAutoTask
	queuedBytes atomic.Int64
	semaphore   chan struct{}
	active      atomic.Int64
	counters    map[string]*atomic.Int64
	// flushed is owned by the maintenance goroutine.
	flushed    map[string]int64
	breaker    gptAutoBreaker
	budgetView atomic.Pointer[gptAutoBudgetView]
	output     gptAutoOutputCache
	snapshots  sync.Map // policy snapshot hash -> time last upserted

	latencyMu      sync.Mutex
	recentLatency  []int
	recentLatencyN int

	startOnce sync.Once
	stopOnce  sync.Once
	stopCh    chan struct{}
	wg        sync.WaitGroup
	// baseCtx parents every background classifier call and is cancelled by
	// Stop, so shutdown never waits on a hung provider.
	baseCtx    context.Context
	baseCancel context.CancelFunc
}

// gptAutoRecentLatencyWindow sizes the in-memory latency ring used for the
// "this node" runtime panel; persisted percentiles come from the database.
const gptAutoRecentLatencyWindow = 512

// NewGPTAutoShadowService builds the service. Call Start to launch workers.
func NewGPTAutoShadowService(settings *SettingService, repo GPTAutoShadowRepository, classifier GPTAutoClassifier, pricing GPTAutoPricingLookup) *GPTAutoShadowService {
	s := &GPTAutoShadowService{
		settings:      settings,
		repo:          repo,
		classifier:    classifier,
		pricing:       pricing,
		budget:        newLocalGPTAutoBudgetStore(),
		now:           time.Now,
		queue:         make(chan *gptAutoTask, gptAutoMaxQueueSize),
		semaphore:     make(chan struct{}, gptAutoMaxConcurrency),
		counters:      make(map[string]*atomic.Int64, len(gptAutoCounterNames())),
		flushed:       make(map[string]int64, len(gptAutoCounterNames())),
		stopCh:        make(chan struct{}),
		recentLatency: make([]int, gptAutoRecentLatencyWindow),
	}
	for _, name := range gptAutoCounterNames() {
		s.counters[name] = &atomic.Int64{}
	}
	s.baseCtx, s.baseCancel = context.WithCancel(context.Background())
	return s
}

// Start launches the worker pool, the maintenance loop (budget view refresh,
// counter flush) and the retention cleaner. Idempotent.
func (s *GPTAutoShadowService) Start() {
	if s == nil {
		return
	}
	s.startOnce.Do(func() {
		for id := 0; id < gptAutoMaxWorkerCount; id++ {
			s.wg.Add(1)
			go s.worker(id)
		}
		s.wg.Add(2)
		go s.cleanupLoop()
		go s.maintenanceLoop()
	})
}

// Stop drains nothing: queued shadow work is disposable by design. In-flight
// classifier calls are cancelled, then the workers are awaited.
func (s *GPTAutoShadowService) Stop() {
	if s == nil {
		return
	}
	s.stopOnce.Do(func() {
		close(s.stopCh)
		s.baseCancel()
		s.wg.Wait()
	})
}

func (s *GPTAutoShadowService) maintenanceLoop() {
	defer s.wg.Done()
	ticker := time.NewTicker(gptAutoMaintenanceInterval)
	defer ticker.Stop()
	s.refreshBudget(s.baseCtx, true)
	lastSeed := s.now()
	for {
		select {
		case <-s.stopCh:
			// Final flush so this node's counters are not lost on shutdown.
			flushCtx, cancel := context.WithTimeout(context.Background(), gptAutoBudgetOpTimeout)
			s.flushCounters(flushCtx)
			cancel()
			return
		case <-ticker.C:
			seed := s.now().Sub(lastSeed) >= gptAutoBudgetSeedInterval
			if seed {
				lastSeed = s.now()
			}
			s.refreshBudget(s.baseCtx, seed)
			s.flushCounters(s.baseCtx)
		}
	}
}

func (s *GPTAutoShadowService) incr(name string) {
	if counter, ok := s.counters[name]; ok {
		counter.Add(1)
	}
}

// Observe is the gateway hook. It is safe to call on every request: when
// shadow mode is off it returns after one cached config read. It never
// blocks, never returns an error and recovers from its own panics.
func (s *GPTAutoShadowService) Observe(ctx context.Context, input GPTAutoObserveInput) {
	if s == nil || s.settings == nil {
		return
	}
	defer func() {
		if r := recover(); r != nil {
			slog.Error("gpt_auto.observe_panic", "recover", r)
		}
	}()
	cfg := s.settings.GetGPTAutoShadowConfigCached(ctx)
	if !cfg.ShadowEnabled || cfg.SampleRate <= 0 {
		return
	}
	s.incr(gptAutoCounterObserved)

	model := strings.TrimSpace(peekGPTAutoModel(input.Body))
	if !cfg.IncludesEndpoint(input.Endpoint) || !cfg.IncludesGroup(input.GroupID) || !cfg.IncludesModel(model) {
		s.incr(gptAutoCounterSkippedFilter)
		return
	}
	if !gptAutoSampled(input.APIKeyID, input.Body, cfg.SampleRate) {
		s.incr(gptAutoCounterNotSampled)
		return
	}
	s.incr(gptAutoCounterSampled)
	if strings.TrimSpace(cfg.ClassifierAPIKey) == "" {
		s.incr(gptAutoCounterSkippedConfig)
		return
	}
	now := s.now()
	if s.breaker.state(now) == "open" {
		s.incr(gptAutoCounterSkippedCircit)
		return
	}
	if s.budgetLikelyExhausted(cfg, now) {
		s.incr(gptAutoCounterSkippedBudget)
		return
	}
	if len(s.queue) >= cfg.QueueSize || s.queuedBytes.Load() >= gptAutoMaxQueuedBytes {
		s.incr(gptAutoCounterDropped)
		return
	}

	snapshot := BuildGPTAutoSnapshot(input.Endpoint, input.Body, cfg.ClassificationMaxContextChars, cfg.ClassificationContextMessages)
	if strings.TrimSpace(snapshot.LatestUser) == "" && len(snapshot.RecentContext) == 0 {
		s.incr(gptAutoCounterSkippedEmpty)
		return
	}
	snapshot.RequestID = input.RequestID
	if snapshot.RequestID == "" {
		// The id usage_logs will carry, so the decision can be joined to the
		// request's real model, tokens and charge.
		snapshot.RequestID = GPTAutoUsageRequestID(ctx)
	}
	snapshot.UserID = input.UserID
	snapshot.APIKeyID = input.APIKeyID
	snapshot.GroupID = cloneInt64Ptr(input.GroupID)

	task := &gptAutoTask{snapshot: snapshot, enqueuedAt: now, bytes: gptAutoSnapshotBytes(snapshot)}
	if s.queuedBytes.Add(task.bytes) > gptAutoMaxQueuedBytes {
		s.queuedBytes.Add(-task.bytes)
		s.incr(gptAutoCounterDropped)
		return
	}
	select {
	case s.queue <- task:
	default:
		s.queuedBytes.Add(-task.bytes)
		s.incr(gptAutoCounterDropped)
	}
}

func peekGPTAutoModel(body []byte) string {
	// gjson stops at the first match; cheap even for large bodies.
	return gjson.GetBytes(body, "model").String()
}

// gptAutoSampled is deterministic: the same API key sending the same request
// prefix always lands in the same bucket, so repeated requests are analysed
// consistently. Only a bounded prefix is hashed.
func gptAutoSampled(apiKeyID int64, body []byte, sampleRate int) bool {
	if sampleRate >= gptAutoMaxSampleRate {
		return true
	}
	if sampleRate <= 0 {
		return false
	}
	var prefix [8]byte
	binary.LittleEndian.PutUint64(prefix[:], uint64(apiKeyID))
	digest := xxhash.New()
	_, _ = digest.Write(prefix[:])
	if len(body) > gptAutoSampleHashPrefix {
		body = body[:gptAutoSampleHashPrefix]
	}
	_, _ = digest.Write(body)
	return digest.Sum64()%gptAutoSampleBuckets < uint64(sampleRate)*(gptAutoSampleBuckets/gptAutoMaxSampleRate)
}

func gptAutoDayStart(now time.Time) time.Time {
	utc := now.UTC()
	return time.Date(utc.Year(), utc.Month(), utc.Day(), 0, 0, 0, 0, time.UTC)
}

func (s *GPTAutoShadowService) worker(id int) {
	defer s.wg.Done()
	for {
		select {
		case <-s.stopCh:
			return
		default:
		}
		cfg := s.settings.GetGPTAutoShadowConfigCached(context.Background())
		if id >= cfg.WorkerCount {
			select {
			case <-s.stopCh:
				return
			case <-time.After(time.Second):
			}
			continue
		}
		select {
		case <-s.stopCh:
			return
		case task := <-s.queue:
			s.queuedBytes.Add(-task.bytes)
			s.process(task)
		case <-time.After(time.Second):
		}
	}
}

// acquire takes a classifier slot, honouring the configured concurrency limit.
func (s *GPTAutoShadowService) acquire(ctx context.Context, limit int) bool {
	for {
		select {
		case s.semaphore <- struct{}{}:
			if s.active.Add(1) <= int64(limit) {
				return true
			}
			s.active.Add(-1)
			<-s.semaphore
		case <-ctx.Done():
			return false
		case <-s.stopCh:
			return false
		}
		select {
		case <-time.After(50 * time.Millisecond):
		case <-ctx.Done():
			return false
		case <-s.stopCh:
			return false
		}
	}
}

func (s *GPTAutoShadowService) release() {
	s.active.Add(-1)
	<-s.semaphore
}

func (s *GPTAutoShadowService) process(task *gptAutoTask) {
	probeHeld := false
	defer func() {
		if probeHeld {
			s.breaker.releaseProbe()
		}
		if r := recover(); r != nil {
			slog.Error("gpt_auto.worker_panic", "recover", r)
		}
	}()
	ctx := s.baseCtx
	cfg := s.settings.GetGPTAutoShadowConfigCached(ctx)
	if !cfg.ShadowEnabled {
		return
	}
	now := s.now()
	if !s.breaker.allow(now) {
		s.incr(gptAutoCounterSkippedCircit)
		return
	}
	probeHeld = true
	waitCtx, cancel := context.WithTimeout(ctx, time.Duration(cfg.ClassifierTimeoutMS)*time.Millisecond)
	acquired := s.acquire(waitCtx, cfg.ClassifierMaxConcurrency)
	cancel()
	if !acquired {
		s.incr(gptAutoCounterDropped)
		return
	}
	queueDelay := int(s.now().Sub(task.enqueuedAt).Milliseconds())
	started := s.now()
	result := s.classifyWithRetry(ctx, cfg, task.snapshot)
	latency := int(s.now().Sub(started).Milliseconds())
	s.release()

	if result.attempts == 0 {
		// Budget denied before any Jev call: nothing was spent, nothing is
		// stored, and the provider's health is unknown.
		if errors.Is(result.err, errGPTAutoBudgetUnavailable) {
			s.incr(gptAutoCounterSkippedBudgetDown)
		} else {
			s.incr(gptAutoCounterSkippedBudget)
		}
		return
	}
	s.breaker.record(result.err == nil, s.now())
	probeHeld = false
	decision := s.buildDecisionRow(cfg, task.snapshot, result, latency, queueDelay)
	if result.err == nil {
		s.incr(gptAutoCounterSuccess)
		s.observeLatency(latency)
	} else {
		s.incr(gptAutoCounterFailed)
		slog.Warn("gpt_auto.classify_failed", "error_class", decision.ErrorClass, "attempts", result.attempts, "latency_ms", latency)
	}
	// Record the outcome even if Stop cancelled the classifier call.
	s.persist(context.WithoutCancel(ctx), cfg, decision)
}

// gptAutoAttemptResult aggregates every Jev attempt made for one decision.
type gptAutoAttemptResult struct {
	classification  *GPTAutoClassification
	attempts        int
	classifierModel string
	inputTokens     int
	outputTokens    int
	costUSD         float64
	err             error
}

// classifyWithRetry reserves budget before every attempt (each attempt is a
// billable HTTP call) and settles it to the billed usage afterwards.
func (s *GPTAutoShadowService) classifyWithRetry(ctx context.Context, cfg GPTAutoShadowConfig, snapshot GPTAutoRequestSnapshot) gptAutoAttemptResult {
	var result gptAutoAttemptResult
	if s.classifier == nil {
		result.attempts = 1
		result.err = &GPTAutoClassifierError{Class: GPTAutoErrNotConfigured, Err: ErrGPTAutoClassifierNotConfigured}
		return result
	}
	estimate := s.estimateClassifierCostNano(cfg, snapshot)
	for attempt := 1; attempt <= 1+cfg.ClassifierRetryCount; attempt++ {
		reservation, err := s.reserveBudget(ctx, cfg, estimate)
		if err != nil {
			if result.attempts == 0 {
				result.err = err
			}
			return result
		}
		result.attempts = attempt
		// Bound each attempt here rather than trusting the classifier
		// implementation to honour the configured timeout.
		attemptCtx, cancel := context.WithTimeout(ctx, time.Duration(cfg.ClassifierTimeoutMS)*time.Millisecond)
		classification, err := s.classifier.Classify(attemptCtx, cfg, snapshot)
		if err != nil && errors.Is(attemptCtx.Err(), context.DeadlineExceeded) {
			err = &GPTAutoClassifierError{Class: GPTAutoErrTimeout, Err: context.DeadlineExceeded}
		}
		cancel()
		model, input, output := gptAutoBilledUsage(classification, err)
		cost := 0.0
		if input > 0 || output > 0 {
			cost = gptAutoClassifierCostUSD(s.pricing, cfg.ClassifierModel, input, output)
			result.inputTokens += input
			result.outputTokens += output
			result.costUSD += cost
		}
		if model != "" {
			result.classifierModel = model
		}
		s.settleBudget(reservation, cost, true)
		if err == nil {
			result.classification = classification
			result.err = nil
			return result
		}
		result.err = err
		if !isRetryableGPTAutoError(err) || attempt > cfg.ClassifierRetryCount {
			return result
		}
		select {
		case <-time.After(time.Duration(attempt) * 200 * time.Millisecond):
		case <-s.stopCh:
			return result
		}
	}
	return result
}

// gptAutoBilledUsage extracts the usage a Jev attempt was billed for, from a
// successful classification or from a 2xx response with unusable answers.
func gptAutoBilledUsage(classification *GPTAutoClassification, err error) (string, int, int) {
	if classification != nil {
		return classification.ClassifierModel, classification.InputTokens, classification.OutputTokens
	}
	var classified *GPTAutoClassifierError
	if errors.As(err, &classified) {
		return classified.ClassifierModel, classified.InputTokens, classified.OutputTokens
	}
	return "", 0, 0
}

func (s *GPTAutoShadowService) buildDecisionRow(cfg GPTAutoShadowConfig, snapshot GPTAutoRequestSnapshot, result gptAutoAttemptResult, latency, queueDelay int) *GPTAutoShadowDecision {
	features := GPTAutoRequestFeatures{
		Endpoint:             snapshot.Endpoint,
		HasTools:             snapshot.HasTools,
		EstimatedInputTokens: snapshot.EstimatedInputTokens,
	}
	row := &GPTAutoShadowDecision{
		CreatedAt:              s.now().UTC(),
		RequestID:              snapshot.RequestID,
		UserID:                 nonZeroInt64(snapshot.UserID),
		APIKeyID:               nonZeroInt64(snapshot.APIKeyID),
		GroupID:                cloneInt64Ptr(snapshot.GroupID),
		Endpoint:               snapshot.Endpoint,
		ActualModel:            snapshot.RequestedModel,
		ActualEffort:           snapshot.RequestedEffort,
		Stream:                 snapshot.Stream,
		HasTools:               snapshot.HasTools,
		ClassifierModel:        cfg.ClassifierModel,
		ClassifierInputTokens:  result.inputTokens,
		ClassifierOutputTokens: result.outputTokens,
		ClassifierCost:         result.costUSD,
		ClassifierLatencyMS:    latency,
		QueueDelayMS:           queueDelay,
		Attempts:               result.attempts,
		PolicyVersion:          cfg.PolicyVersion,
		Features:               features,
	}
	if result.classifierModel != "" {
		row.ClassifierModel = result.classifierModel
	}
	if result.err != nil {
		row.EvaluationStatus = GPTAutoEvaluationFailed
		row.ErrorClass = GPTAutoErrTransport
		var classified *GPTAutoClassifierError
		if errors.As(result.err, &classified) {
			row.ErrorClass = classified.Class
		}
		return row
	}

	classification := result.classification
	row.EvaluationStatus = GPTAutoEvaluationSuccess
	row.Classification = classification

	decision := EvaluateGPTAutoPolicy(GPTAutoPolicyInput{
		Classification:    *classification,
		Features:          features,
		Config:            cfg,
		Pricing:           s.pricing,
		OutputEstimate:    s.outputEstimator(),
		ClassifierCostUSD: row.ClassifierCost,
	})
	s.applyDecision(row, classification, &decision)
	return row
}

func (s *GPTAutoShadowService) applyDecision(row *GPTAutoShadowDecision, classification *GPTAutoClassification, decision *GPTAutoPolicyDecision) {
	row.Decision = decision
	row.DecisionStatus = decision.Status
	row.JevModel = decision.JevModel
	row.JevEffort = decision.JevEffort
	row.FinalModel = decision.FinalModel
	row.FinalEffort = decision.FinalEffort
	row.Category = classification.Category
	row.Complexity = float64Ptr(classification.Complexity)
	row.ModelConfidence = float64Ptr(classification.ModelConfidence)
	row.EffortConfidence = float64Ptr(classification.EffortConfidence)
	row.EstRevenue, row.EstUpstreamCost, row.EstGrossProfit, row.EstGrossMargin = nil, nil, nil, nil
	row.MarginBelowTarget = false
	if economics := decision.Economics; economics != nil && economics.PricingAvailable {
		row.EstRevenue = float64Ptr(economics.EstimatedRevenue)
		row.EstUpstreamCost = float64Ptr(economics.EstimatedUpstreamCost)
		row.EstGrossProfit = float64Ptr(economics.EstimatedGrossProfit)
		row.EstGrossMargin = economics.EstimatedGrossMargin
		row.MarginBelowTarget = economics.BelowTarget
	}
}

func (s *GPTAutoShadowService) persist(ctx context.Context, cfg GPTAutoShadowConfig, row *GPTAutoShadowDecision) {
	if s.repo == nil {
		return
	}
	persistCtx, cancel := context.WithTimeout(ctx, gptAutoPersistTimeout)
	defer cancel()
	if hash, err := s.ensurePolicySnapshot(persistCtx, cfg); err == nil {
		row.PolicySnapshotHash = hash
	} else {
		slog.Warn("gpt_auto.snapshot_persist_failed", "error", err)
	}
	if err := s.repo.Insert(persistCtx, row); err != nil {
		s.incr(gptAutoCounterPersistFailed)
		slog.Warn("gpt_auto.persist_failed", "error", err)
	}
}

func (s *GPTAutoShadowService) ensurePolicySnapshot(ctx context.Context, cfg GPTAutoShadowConfig) (string, error) {
	hash, raw, err := NewGPTAutoPolicySnapshot(cfg).Hash()
	if err != nil {
		return "", err
	}
	// The upsert is idempotent; the cache only avoids repeating it on every
	// decision. Entries expire so a restored or pruned table heals itself.
	if value, ok := s.snapshots.Load(hash); ok {
		if seenAt, isTime := value.(time.Time); isTime && s.now().Sub(seenAt) < gptAutoSnapshotRecheck {
			return hash, nil
		}
	}
	if err := s.repo.UpsertPolicySnapshot(ctx, hash, cfg.PolicyVersion, raw); err != nil {
		return "", err
	}
	s.snapshots.Store(hash, s.now())
	return hash, nil
}

const gptAutoSnapshotRecheck = time.Hour

func (s *GPTAutoShadowService) observeLatency(ms int) {
	s.latencyMu.Lock()
	s.recentLatency[s.recentLatencyN%gptAutoRecentLatencyWindow] = ms
	s.recentLatencyN++
	s.latencyMu.Unlock()
}

// outputEstimator returns historical output-token usage from usage_logs,
// refreshed every gptAutoOutputHistoryTTL. Effort-specific history is
// preferred; model-level history is the fallback.
func (s *GPTAutoShadowService) outputEstimator() GPTAutoOutputEstimator {
	s.refreshOutputHistory()
	s.output.mu.Lock()
	byPair, byModel := s.output.byPair, s.output.byModel
	s.output.mu.Unlock()
	return func(model, effort string) GPTAutoOutputEstimate {
		if estimate, ok := byPair[model+"|"+effort]; ok {
			return estimate
		}
		if estimate, ok := byModel[model]; ok {
			return estimate
		}
		return GPTAutoOutputEstimate{Source: GPTAutoOutputSourceNone}
	}
}

func (s *GPTAutoShadowService) refreshOutputHistory() {
	s.output.mu.Lock()
	defer s.output.mu.Unlock()
	now := s.now()
	if s.repo == nil || (s.output.byPair != nil && now.Sub(s.output.loadedAt) < gptAutoOutputHistoryTTL) {
		return
	}
	s.output.loadedAt = now
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	rows, err := s.repo.OutputHistory(ctx, GPTAutoModelPool(), now.Add(-gptAutoOutputHistoryWindow))
	if err != nil {
		slog.Warn("gpt_auto.output_history_failed", "error", err)
		if s.output.byPair == nil {
			s.output.byPair, s.output.byModel = map[string]GPTAutoOutputEstimate{}, map[string]GPTAutoOutputEstimate{}
		}
		return
	}
	byPair := make(map[string]GPTAutoOutputEstimate, len(rows))
	totals := make(map[string][2]float64)
	for _, row := range rows {
		effort := strings.ToLower(strings.TrimSpace(row.Effort))
		if row.Samples > 0 && effort != "" {
			byPair[row.Model+"|"+effort] = GPTAutoOutputEstimate{Tokens: row.AvgOutput, Source: GPTAutoOutputSourceModelEffort, Samples: row.Samples}
		}
		total := totals[row.Model]
		total[0] += row.AvgOutput * float64(row.Samples)
		total[1] += float64(row.Samples)
		totals[row.Model] = total
	}
	byModel := make(map[string]GPTAutoOutputEstimate, len(totals))
	for model, total := range totals {
		if total[1] > 0 {
			byModel[model] = GPTAutoOutputEstimate{Tokens: total[0] / total[1], Source: GPTAutoOutputSourceModel, Samples: int64(total[1])}
		}
	}
	s.output.byPair, s.output.byModel = byPair, byModel
}

func (s *GPTAutoShadowService) cleanupLoop() {
	defer s.wg.Done()
	timer := time.NewTimer(time.Minute)
	defer timer.Stop()
	for {
		select {
		case <-s.stopCh:
			return
		case <-timer.C:
			s.cleanupOnce()
			timer.Reset(gptAutoCleanupInterval)
		}
	}
}

func (s *GPTAutoShadowService) cleanupOnce() {
	if s.repo == nil {
		return
	}
	cfg := s.settings.GetGPTAutoShadowConfigCached(context.Background())
	cutoff := s.now().Add(-time.Duration(cfg.DecisionRetentionDays) * 24 * time.Hour)
	var total int64
	for i := 0; i < gptAutoCleanupMaxBatches; i++ {
		select {
		case <-s.stopCh:
			return
		default:
		}
		ctx, cancel := context.WithTimeout(s.baseCtx, 30*time.Second)
		deleted, err := s.repo.DeleteOlderThan(ctx, cutoff, gptAutoCleanupBatchSize)
		cancel()
		if err != nil {
			slog.Warn("gpt_auto.cleanup_failed", "error", err)
			return
		}
		total += deleted
		if deleted < gptAutoCleanupBatchSize {
			break
		}
	}
	if total > 0 {
		slog.Info("gpt_auto.cleanup_done", "deleted", total, "retention_days", cfg.DecisionRetentionDays)
	}
}

// ---------------------------------------------------------------------------
// Admin operations
// ---------------------------------------------------------------------------

// GPTAutoRuntimeStatus combines this node's in-memory view with the
// cluster-wide daily budget and counters from the budget store.
type GPTAutoRuntimeStatus struct {
	// Counters are this node's totals since process start.
	Counters      map[string]int64 `json:"counters"`
	QueueLength   int              `json:"queue_length"`
	QueueCapacity int              `json:"queue_capacity"`
	QueueBytes    int64            `json:"queue_bytes"`
	ActiveCalls   int64            `json:"active_calls"`
	// CircuitState is this node's breaker.
	CircuitState string `json:"circuit_state"`
	// TodayCalls/TodayCost are the cluster-wide UTC-day budget totals.
	TodayCalls int64   `json:"today_calls"`
	TodayCost  float64 `json:"today_cost"`
	// BudgetScope is "cluster" when the shared store answered, otherwise
	// "unavailable" (budgeted classification is then paused).
	BudgetScope string `json:"budget_scope"`
	// ClusterCountersToday are every node's counters for the UTC day, as of
	// each node's last flush (about every 10 seconds).
	ClusterCountersToday map[string]int64 `json:"cluster_counters_today,omitempty"`
	RecentP50MS          *float64         `json:"recent_p50_ms,omitempty"`
	RecentP95MS          *float64         `json:"recent_p95_ms,omitempty"`
	RecentLatencyN       int              `json:"recent_latency_samples"`
	ExcludedReasons      []string         `json:"excluded_endpoints"`
}

// GPTAutoOverview combines persisted statistics with runtime state.
type GPTAutoOverview struct {
	ShadowEnabled bool                 `json:"shadow_enabled"`
	SampleRate    int                  `json:"sample_rate"`
	Configured    bool                 `json:"classifier_configured"`
	PolicyVersion string               `json:"policy_version"`
	StartTime     time.Time            `json:"start_time"`
	EndTime       time.Time            `json:"end_time"`
	Stored        *GPTAutoStoredStats  `json:"stored"`
	Runtime       GPTAutoRuntimeStatus `json:"runtime"`
	ErrorRate     *float64             `json:"error_rate,omitempty"`
}

// gptAutoMinPercentileSamples is the smallest sample for which P50/P95 are
// shown; below it the numbers are noise.
const gptAutoMinPercentileSamples = 20

// Overview returns dashboard statistics for a time range.
func (s *GPTAutoShadowService) Overview(ctx context.Context, filter GPTAutoStatsFilter) (*GPTAutoOverview, error) {
	cfg, err := s.settings.GetGPTAutoShadowConfig(ctx)
	if err != nil {
		return nil, err
	}
	stored, err := s.repo.Stats(ctx, filter)
	if err != nil {
		return nil, err
	}
	if stored.Success < gptAutoMinPercentileSamples {
		stored.P50LatencyMS, stored.P95LatencyMS = nil, nil
	}
	overview := &GPTAutoOverview{
		ShadowEnabled: cfg.ShadowEnabled,
		SampleRate:    cfg.SampleRate,
		Configured:    strings.TrimSpace(cfg.ClassifierAPIKey) != "",
		PolicyVersion: cfg.PolicyVersion,
		StartTime:     filter.StartTime,
		EndTime:       filter.EndTime,
		Stored:        stored,
		Runtime:       s.RuntimeStatus(ctx),
	}
	if stored.Total > 0 {
		rate := float64(stored.Failed) / float64(stored.Total)
		overview.ErrorRate = &rate
	}
	return overview, nil
}

// RuntimeStatus reports this node's counters and queue.
func (s *GPTAutoShadowService) RuntimeStatus(ctx context.Context) GPTAutoRuntimeStatus {
	status := GPTAutoRuntimeStatus{
		Counters:        make(map[string]int64, len(s.counters)),
		QueueLength:     len(s.queue),
		QueueBytes:      s.queuedBytes.Load(),
		ActiveCalls:     s.active.Load(),
		CircuitState:    s.breaker.state(s.now()),
		BudgetScope:     "cluster",
		ExcludedReasons: []string{"messages", "gemini", "systemone", "responses_websocket", "count_tokens", "images", "videos"},
	}
	cfg := s.settings.GetGPTAutoShadowConfigCached(ctx)
	status.QueueCapacity = cfg.QueueSize
	for name, counter := range s.counters {
		status.Counters[name] = counter.Load()
	}
	day := gptAutoDayStart(s.now()).Format(time.DateOnly)
	opCtx, cancel := context.WithTimeout(ctx, gptAutoBudgetOpTimeout)
	defer cancel()
	if usage, _, err := s.budget.Usage(opCtx, day); err == nil {
		status.TodayCalls, status.TodayCost = usage.Calls, usage.CostUSD()
	} else {
		status.BudgetScope = "unavailable"
	}
	if counters, err := s.budget.Counters(opCtx, day); err == nil && len(counters) > 0 {
		status.ClusterCountersToday = counters
	}
	s.latencyMu.Lock()
	n := s.recentLatencyN
	if n > gptAutoRecentLatencyWindow {
		n = gptAutoRecentLatencyWindow
	}
	samples := append([]int(nil), s.recentLatency[:n]...)
	s.latencyMu.Unlock()
	status.RecentLatencyN = n
	if n >= gptAutoMinPercentileSamples {
		p50, p95 := gptAutoPercentile(samples, 0.5), gptAutoPercentile(samples, 0.95)
		status.RecentP50MS, status.RecentP95MS = &p50, &p95
	}
	return status
}

func gptAutoPercentile(samples []int, q float64) float64 {
	sorted := append([]int(nil), samples...)
	for i := 1; i < len(sorted); i++ {
		for j := i; j > 0 && sorted[j] < sorted[j-1]; j-- {
			sorted[j], sorted[j-1] = sorted[j-1], sorted[j]
		}
	}
	index := int(q * float64(len(sorted)-1))
	return float64(sorted[index])
}

// ListDecisions pages the decision log.
func (s *GPTAutoShadowService) ListDecisions(ctx context.Context, filter GPTAutoDecisionFilter) ([]GPTAutoShadowDecision, *pagination.PaginationResult, error) {
	if filter.Pagination.Page <= 0 {
		filter.Pagination.Page = 1
	}
	if filter.Pagination.PageSize <= 0 {
		filter.Pagination.PageSize = 20
	}
	if filter.Pagination.PageSize > 200 {
		filter.Pagination.PageSize = 200
	}
	return s.repo.List(ctx, filter)
}

// ErrGPTAutoDecisionNotFound is returned for unknown decision ids.
var ErrGPTAutoDecisionNotFound = infraerrors.NotFound("GPT_AUTO_DECISION_NOT_FOUND", "shadow decision not found")

// GetDecision returns one decision with its full JSON payloads and, when the
// request has been billed, its actual production usage from usage_logs.
func (s *GPTAutoShadowService) GetDecision(ctx context.Context, id int64) (*GPTAutoShadowDecision, error) {
	row, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	actual, err := s.repo.ActualUsage(ctx, row.RequestID, row.CreatedAt)
	if err != nil {
		slog.Warn("gpt_auto.actual_usage_lookup_failed", "error", err)
	}
	row.ActualUsage = actual
	return row, nil
}

// GPTAutoUsageRequestID returns the request id usage_logs records for the
// current request, so a shadow decision can be joined to the real outcome.
func GPTAutoUsageRequestID(ctx context.Context) string {
	return resolveUsageBillingRequestID(ctx, "")
}

// GPTAutoTestInput drives the interactive test lab.
type GPTAutoTestInput struct {
	Prompt   string               `json:"prompt"`
	Context  []GPTAutoContextTurn `json:"context"`
	Endpoint string               `json:"endpoint"`
	HasTools bool                 `json:"has_tools"`
	// ModelProfiles optionally replaces the saved profiles for this test only
	// (capability constraints, weights, multipliers).
	ModelProfiles []GPTAutoModelProfile `json:"model_profiles,omitempty"`
	// EstimatedInputTokens overrides the character-based estimate.
	EstimatedInputTokens *int `json:"estimated_input_tokens,omitempty"`
}

// GPTAutoTestResult is returned by the test lab. Nothing is persisted.
type GPTAutoTestResult struct {
	Classification    *GPTAutoClassification `json:"classification"`
	Decision          GPTAutoPolicyDecision  `json:"decision"`
	ClassifierCost    float64                `json:"classifier_cost"`
	LatencyMS         int                    `json:"latency_ms"`
	SnapshotPreview   string                 `json:"snapshot_preview"`
	ProfilesOverriden bool                   `json:"profiles_overridden"`
}

const gptAutoMaxTestPromptRunes = 50000

// RunTest classifies an administrator-supplied sample with Jev and evaluates
// the policy. It calls only the classifier, never a GPT model, and is not
// stored. It shares the classifier concurrency limit with shadow traffic.
func (s *GPTAutoShadowService) RunTest(ctx context.Context, input GPTAutoTestInput) (*GPTAutoTestResult, error) {
	cfg, err := s.settings.GetGPTAutoShadowConfig(ctx)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(input.Prompt) == "" {
		return nil, infraerrors.BadRequest("GPT_AUTO_TEST_INVALID", "prompt is required")
	}
	if len([]rune(input.Prompt)) > gptAutoMaxTestPromptRunes || len(input.Context) > gptAutoMaxContextMessages {
		return nil, infraerrors.BadRequest("GPT_AUTO_TEST_INVALID", "test input is too large")
	}
	if input.Endpoint == "" {
		input.Endpoint = GPTAutoEndpointChatCompletions
	}
	if !isGPTAutoSupportedEndpoint(input.Endpoint) {
		return nil, infraerrors.BadRequest("GPT_AUTO_TEST_INVALID", "unsupported endpoint")
	}
	overridden, err := applyGPTAutoProfileOverride(cfg, input.ModelProfiles)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(cfg.ClassifierAPIKey) == "" {
		return nil, infraerrors.BadRequest("GPT_AUTO_CLASSIFIER_NOT_CONFIGURED", "configure the Jev API key before running a test")
	}

	snapshot := buildGPTAutoTestSnapshot(*cfg, input)
	waitCtx, cancel := context.WithTimeout(ctx, time.Duration(cfg.ClassifierTimeoutMS)*time.Millisecond)
	acquired := s.acquire(waitCtx, cfg.ClassifierMaxConcurrency)
	cancel()
	if !acquired {
		return nil, infraerrors.New(429, "GPT_AUTO_CLASSIFIER_BUSY", "classifier concurrency limit reached; retry shortly")
	}
	defer s.release()
	// Test-lab calls count against the same cluster-wide daily budget as
	// shadow traffic, so the lab cannot be used to bypass it.
	reservation, err := s.reserveBudget(ctx, *cfg, s.estimateClassifierCostNano(*cfg, snapshot))
	if err != nil {
		if errors.Is(err, errGPTAutoBudgetUnavailable) {
			return nil, infraerrors.New(503, "GPT_AUTO_BUDGET_UNAVAILABLE", "budget store unavailable; daily limits cannot be enforced")
		}
		return nil, infraerrors.New(429, "GPT_AUTO_BUDGET_EXHAUSTED", "daily Jev call or cost limit reached")
	}
	started := s.now()
	callCtx, callCancel := context.WithTimeout(ctx, time.Duration(cfg.ClassifierTimeoutMS)*time.Millisecond)
	classification, err := s.classifier.Classify(callCtx, *cfg, snapshot)
	callCancel()
	latency := int(s.now().Sub(started).Milliseconds())
	_, billedIn, billedOut := gptAutoBilledUsage(classification, err)
	cost := gptAutoClassifierCostUSD(s.pricing, cfg.ClassifierModel, billedIn, billedOut)
	s.settleBudget(reservation, cost, true)
	if err != nil {
		class := GPTAutoErrTransport
		var classified *GPTAutoClassifierError
		if errors.As(err, &classified) {
			class = classified.Class
		}
		return nil, infraerrors.New(502, "GPT_AUTO_CLASSIFIER_FAILED", "Jev classification failed: "+class)
	}
	features := GPTAutoRequestFeatures{Endpoint: snapshot.Endpoint, HasTools: snapshot.HasTools, EstimatedInputTokens: snapshot.EstimatedInputTokens}
	decision := EvaluateGPTAutoPolicy(GPTAutoPolicyInput{
		Classification: *classification, Features: features, Config: *cfg,
		Pricing: s.pricing, OutputEstimate: s.outputEstimator(), ClassifierCostUSD: cost,
	})
	preview, _ := buildGPTAutoClassifierState(snapshot)
	return &GPTAutoTestResult{
		Classification: classification, Decision: decision, ClassifierCost: cost,
		LatencyMS: latency, SnapshotPreview: preview, ProfilesOverriden: overridden,
	}, nil
}

func buildGPTAutoTestSnapshot(cfg GPTAutoShadowConfig, input GPTAutoTestInput) GPTAutoRequestSnapshot {
	turns := make([]GPTAutoContextTurn, 0, len(input.Context))
	totalChars := utf8.RuneCountInString(input.Prompt)
	budget := cfg.ClassificationMaxContextChars
	capture := budget * gptAutoTurnCaptureFactor
	latest := trimRunesKeepTail(redactGPTAutoText(gptAutoCaptureText(input.Prompt, capture)), budget*3/4)
	budget -= utf8.RuneCountInString(latest)
	for _, turn := range input.Context {
		totalChars += utf8.RuneCountInString(turn.Text)
		text := strings.TrimSpace(turn.Text)
		if text == "" || budget <= 0 {
			continue
		}
		text = trimRunesKeepTail(redactGPTAutoText(gptAutoCaptureText(text, capture)), budget/maxInt(len(input.Context), 1))
		budget -= utf8.RuneCountInString(text)
		turns = append(turns, GPTAutoContextTurn{Role: gptAutoNormalizeRole(turn.Role), Text: text})
	}
	snapshot := GPTAutoRequestSnapshot{
		Endpoint:             input.Endpoint,
		HasTools:             input.HasTools,
		MessageCount:         len(input.Context) + 1,
		LatestUser:           latest,
		RecentContext:        turns,
		EstimatedInputTokens: (totalChars + gptAutoCharsPerToken - 1) / gptAutoCharsPerToken,
	}
	if input.HasTools {
		snapshot.ToolCount = 1
	}
	if input.EstimatedInputTokens != nil && *input.EstimatedInputTokens >= 0 {
		snapshot.EstimatedInputTokens = *input.EstimatedInputTokens
	}
	return snapshot
}

// applyGPTAutoProfileOverride validates and applies a per-call profile
// override. Returns whether an override was applied.
func applyGPTAutoProfileOverride(cfg *GPTAutoShadowConfig, profiles []GPTAutoModelProfile) (bool, error) {
	if len(profiles) == 0 {
		return false, nil
	}
	candidate := *cfg
	candidate.ModelProfiles = profiles
	if err := ValidateGPTAutoShadowConfig(&candidate); err != nil {
		return false, err
	}
	normalizeGPTAutoShadowConfig(&candidate)
	cfg.ModelProfiles = candidate.ModelProfiles
	return true, nil
}

// GPTAutoReplayInput selects the policy to replay a stored decision against.
type GPTAutoReplayInput struct {
	// Mode is "historical" (the stored snapshot), "current" (saved config) or
	// "custom" (ModelProfiles below).
	Mode               string                `json:"mode"`
	ModelProfiles      []GPTAutoModelProfile `json:"model_profiles,omitempty"`
	GlobalMinMargin    *float64              `json:"global_min_margin,omitempty"`
	ProfitSafetyBuffer *float64              `json:"profit_safety_buffer,omitempty"`
}

// GPTAutoReplayResult compares the stored decision with a replay.
type GPTAutoReplayResult struct {
	DecisionID int64                  `json:"decision_id"`
	Mode       string                 `json:"mode"`
	Original   *GPTAutoPolicyDecision `json:"original"`
	Replayed   GPTAutoPolicyDecision  `json:"replayed"`
	Changed    bool                   `json:"changed"`
}

// Replay re-runs only the deterministic policy against a stored Jev result.
// It never calls the classifier.
func (s *GPTAutoShadowService) Replay(ctx context.Context, id int64, input GPTAutoReplayInput) (*GPTAutoReplayResult, error) {
	row, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if row.EvaluationStatus != GPTAutoEvaluationSuccess || row.Classification == nil {
		return nil, infraerrors.BadRequest("GPT_AUTO_REPLAY_UNAVAILABLE", "only successful classifications can be replayed")
	}
	cfg, err := s.settings.GetGPTAutoShadowConfig(ctx)
	if err != nil {
		return nil, err
	}
	mode := strings.TrimSpace(input.Mode)
	if mode == "" {
		mode = "historical"
	}
	switch mode {
	case "historical":
		if row.PolicySnapshotHash == "" {
			return nil, infraerrors.BadRequest("GPT_AUTO_REPLAY_UNAVAILABLE", "decision has no stored policy snapshot")
		}
		snapshot, err := s.repo.GetPolicySnapshot(ctx, row.PolicySnapshotHash)
		if err != nil {
			return nil, err
		}
		*cfg = snapshot.ApplyTo(*cfg)
	case "current":
	case "custom":
		if _, err := applyGPTAutoProfileOverride(cfg, input.ModelProfiles); err != nil {
			return nil, err
		}
		if input.GlobalMinMargin != nil {
			if *input.GlobalMinMargin < 0 || *input.GlobalMinMargin >= 1 {
				return nil, infraerrors.BadRequest("GPT_AUTO_REPLAY_INVALID", "global_min_margin must be between 0 and 1")
			}
			cfg.GlobalMinMargin = *input.GlobalMinMargin
		}
		if input.ProfitSafetyBuffer != nil {
			if *input.ProfitSafetyBuffer < 0 || *input.ProfitSafetyBuffer >= 1 {
				return nil, infraerrors.BadRequest("GPT_AUTO_REPLAY_INVALID", "profit_safety_buffer must be between 0 and 1")
			}
			cfg.ProfitSafetyBuffer = *input.ProfitSafetyBuffer
		}
		cfg.PolicyVersion = cfg.PolicyVersion + "+custom"
	default:
		return nil, infraerrors.BadRequest("GPT_AUTO_REPLAY_INVALID", "mode must be historical, current or custom")
	}
	replayed := EvaluateGPTAutoPolicy(GPTAutoPolicyInput{
		Classification:    *row.Classification,
		Features:          row.Features,
		Config:            *cfg,
		Pricing:           s.pricing,
		OutputEstimate:    s.outputEstimator(),
		ClassifierCostUSD: row.ClassifierCost,
	})
	result := &GPTAutoReplayResult{DecisionID: row.ID, Mode: mode, Original: row.Decision, Replayed: replayed}
	if row.Decision != nil {
		result.Changed = row.Decision.FinalModel != replayed.FinalModel || row.Decision.FinalEffort != replayed.FinalEffort
	}
	return result, nil
}

// GPTAutoSimulationInput drives the profitability simulator with unsaved
// profile edits.
type GPTAutoSimulationInput struct {
	ModelProfiles        []GPTAutoModelProfile `json:"model_profiles"`
	GlobalMinMargin      *float64              `json:"global_min_margin,omitempty"`
	ProfitSafetyBuffer   *float64              `json:"profit_safety_buffer,omitempty"`
	Effort               string                `json:"effort"`
	InputTokens          int                   `json:"input_tokens"`
	OutputTokens         *int                  `json:"output_tokens,omitempty"`
	IncludeClassifier    bool                  `json:"include_classifier_cost"`
	ClassifierInputToken int                   `json:"classifier_input_tokens"`
}

// GPTAutoSimulationResult has one economics row per pool model.
type GPTAutoSimulationResult struct {
	Rows           []GPTAutoEconomics `json:"rows"`
	ClassifierCost float64            `json:"classifier_cost"`
}

const gptAutoMaxSimulationTokens = 10_000_000

// Simulate computes hypothetical economics for every model. It changes
// nothing and calls nothing external.
func (s *GPTAutoShadowService) Simulate(ctx context.Context, input GPTAutoSimulationInput) (*GPTAutoSimulationResult, error) {
	cfg, err := s.settings.GetGPTAutoShadowConfig(ctx)
	if err != nil {
		return nil, err
	}
	if _, err := applyGPTAutoProfileOverride(cfg, input.ModelProfiles); err != nil {
		return nil, err
	}
	if input.GlobalMinMargin != nil {
		cfg.GlobalMinMargin = *input.GlobalMinMargin
	}
	if input.ProfitSafetyBuffer != nil {
		cfg.ProfitSafetyBuffer = *input.ProfitSafetyBuffer
	}
	if err := ValidateGPTAutoShadowConfig(cfg); err != nil {
		return nil, err
	}
	if input.InputTokens < 0 || input.InputTokens > gptAutoMaxSimulationTokens ||
		input.ClassifierInputToken < 0 || input.ClassifierInputToken > gptAutoMaxSimulationTokens ||
		(input.OutputTokens != nil && (*input.OutputTokens < 0 || *input.OutputTokens > gptAutoMaxSimulationTokens)) {
		return nil, infraerrors.BadRequest("GPT_AUTO_SIMULATION_INVALID", "token counts must be between 0 and 10,000,000")
	}
	effort := strings.ToLower(strings.TrimSpace(input.Effort))
	if effort == "" {
		effort = GPTAutoEffortMedium
	}
	if !IsGPTAutoEffort(effort) {
		return nil, infraerrors.BadRequest("GPT_AUTO_SIMULATION_INVALID", "unknown reasoning effort")
	}
	classifierCost := 0.0
	if input.IncludeClassifier {
		classifierCost = gptAutoClassifierCostUSD(s.pricing, cfg.ClassifierModel, input.ClassifierInputToken, 0)
	}
	estimator := s.outputEstimator()
	if input.OutputTokens != nil {
		fixed := float64(*input.OutputTokens)
		estimator = func(string, string) GPTAutoOutputEstimate {
			return GPTAutoOutputEstimate{Tokens: fixed, Source: "manual_input"}
		}
	}
	result := &GPTAutoSimulationResult{ClassifierCost: classifierCost}
	for _, model := range GPTAutoModelPool() {
		result.Rows = append(result.Rows, EstimateGPTAutoEconomics(GPTAutoPolicyInput{
			Config:            *cfg,
			Features:          GPTAutoRequestFeatures{EstimatedInputTokens: input.InputTokens},
			Pricing:           s.pricing,
			OutputEstimate:    estimator,
			ClassifierCostUSD: classifierCost,
		}, model, effort))
	}
	return result, nil
}

func nonZeroInt64(value int64) *int64 {
	if value == 0 {
		return nil
	}
	return &value
}

// ProvideGPTAutoShadowService wires the production classifier (TypeSafe
// System One, optional proxy), pricing table and cluster-wide budget store,
// then starts the workers. Stop is registered in cmd/server provideCleanup.
func ProvideGPTAutoShadowService(settings *SettingService, repo GPTAutoShadowRepository, budget GPTAutoBudgetStore, billing *BillingService, proxyRepo ProxyRepository) *GPTAutoShadowService {
	var pricing GPTAutoPricingLookup
	if billing != nil {
		pricing = billing.GetModelPricing
	}
	classifier := NewTypeSafeGPTAutoClassifier(newGPTAutoHTTPClientFactory(proxyRepo))
	svc := NewGPTAutoShadowService(settings, repo, classifier, pricing)
	if budget != nil {
		svc.budget = budget
	}
	svc.Start()
	return svc
}

func newGPTAutoHTTPClientFactory(proxyRepo ProxyRepository) GPTAutoHTTPClientFactory {
	return func(ctx context.Context, cfg GPTAutoShadowConfig) (*http.Client, error) {
		// ValidateResolvedIP blocks DNS-rebinding to internal addresses; the
		// base URL itself is validated when the config is saved.
		if cfg.ClassifierProxyID == nil {
			return httpclient.GetClient(httpclient.Options{ValidateResolvedIP: true})
		}
		if proxyRepo == nil {
			return nil, errors.New("proxy repository unavailable")
		}
		proxy, err := proxyRepo.GetByID(ctx, *cfg.ClassifierProxyID)
		if err != nil {
			return nil, err
		}
		return httpclient.GetClient(httpclient.Options{ProxyURL: proxy.URL()})
	}
}
