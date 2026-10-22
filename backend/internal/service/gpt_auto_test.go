package service

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
	"github.com/Wei-Shaw/sub2api/internal/pkg/typesafe"
	"github.com/stretchr/testify/require"
)

// ---- fakes ----------------------------------------------------------------

type gptAutoTestSettingRepo struct {
	mu     sync.Mutex
	values map[string]string
}

func newGPTAutoTestSettingRepo() *gptAutoTestSettingRepo {
	return &gptAutoTestSettingRepo{values: map[string]string{}}
}

func (r *gptAutoTestSettingRepo) Get(_ context.Context, key string) (*Setting, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	value, ok := r.values[key]
	if !ok {
		return nil, ErrSettingNotFound
	}
	return &Setting{Key: key, Value: value}, nil
}

func (r *gptAutoTestSettingRepo) GetValue(_ context.Context, key string) (string, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	value, ok := r.values[key]
	if !ok {
		return "", ErrSettingNotFound
	}
	return value, nil
}

func (r *gptAutoTestSettingRepo) Set(_ context.Context, key, value string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.values[key] = value
	return nil
}

func (r *gptAutoTestSettingRepo) GetMultiple(_ context.Context, keys []string) (map[string]string, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := map[string]string{}
	for _, key := range keys {
		if value, ok := r.values[key]; ok {
			out[key] = value
		}
	}
	return out, nil
}

func (r *gptAutoTestSettingRepo) SetMultiple(ctx context.Context, settings map[string]string) error {
	for key, value := range settings {
		_ = r.Set(ctx, key, value)
	}
	return nil
}

func (r *gptAutoTestSettingRepo) GetAll(_ context.Context) (map[string]string, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := map[string]string{}
	for key, value := range r.values {
		out[key] = value
	}
	return out, nil
}

func (r *gptAutoTestSettingRepo) Delete(_ context.Context, key string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.values, key)
	return nil
}

type gptAutoTestRepo struct {
	mu        sync.Mutex
	rows      []GPTAutoShadowDecision
	snapshots map[string][]byte
	usage     GPTAutoUsageToday
	history   []GPTAutoOutputHistory
	actual    map[string]*GPTAutoActualUsage
	inserted  chan struct{}
	// classifierConfigs records every UpsertClassifierConfig call in order.
	classifierConfigs []GPTAutoClassifierConfigRecord
}

func newGPTAutoTestRepo() *gptAutoTestRepo {
	return &gptAutoTestRepo{snapshots: map[string][]byte{}, actual: map[string]*GPTAutoActualUsage{}, inserted: make(chan struct{}, 1000)}
}

func (r *gptAutoTestRepo) Insert(_ context.Context, d *GPTAutoShadowDecision) error {
	r.mu.Lock()
	d.ID = int64(len(r.rows) + 1)
	r.rows = append(r.rows, *d)
	r.mu.Unlock()
	r.inserted <- struct{}{}
	return nil
}

func (r *gptAutoTestRepo) UpsertPolicySnapshot(_ context.Context, hash, _ string, snapshot []byte) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.snapshots[hash] = append([]byte(nil), snapshot...)
	return nil
}

func (r *gptAutoTestRepo) GetPolicySnapshot(_ context.Context, hash string) (*GPTAutoPolicySnapshot, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	raw, ok := r.snapshots[hash]
	if !ok {
		return nil, ErrGPTAutoDecisionNotFound
	}
	var snapshot GPTAutoPolicySnapshot
	return &snapshot, json.Unmarshal(raw, &snapshot)
}

func (r *gptAutoTestRepo) UpsertClassifierConfig(_ context.Context, record GPTAutoClassifierConfigRecord) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	for i := range r.classifierConfigs {
		if r.classifierConfigs[i].Hash == record.Hash {
			if r.classifierConfigs[i].CreatedBy == nil {
				r.classifierConfigs[i].CreatedBy = record.CreatedBy
			}
			return nil
		}
	}
	r.classifierConfigs = append(r.classifierConfigs, record)
	return nil
}

func (r *gptAutoTestRepo) GetClassifierConfig(_ context.Context, hash string) (*GPTAutoClassifierConfigRecord, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for i := range r.classifierConfigs {
		if r.classifierConfigs[i].Hash == hash {
			record := r.classifierConfigs[i]
			return &record, nil
		}
	}
	return nil, ErrGPTAutoDecisionNotFound
}

func (r *gptAutoTestRepo) ListClassifierConfigs(context.Context, int) ([]GPTAutoClassifierConfigRecord, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]GPTAutoClassifierConfigRecord, 0, len(r.classifierConfigs))
	for i := len(r.classifierConfigs) - 1; i >= 0; i-- {
		out = append(out, r.classifierConfigs[i])
	}
	return out, nil
}

func (r *gptAutoTestRepo) GetByID(_ context.Context, id int64) (*GPTAutoShadowDecision, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for i := range r.rows {
		if r.rows[i].ID == id {
			row := r.rows[i]
			return &row, nil
		}
	}
	return nil, ErrGPTAutoDecisionNotFound
}

func (r *gptAutoTestRepo) List(context.Context, GPTAutoDecisionFilter) ([]GPTAutoShadowDecision, *pagination.PaginationResult, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]GPTAutoShadowDecision(nil), r.rows...), &pagination.PaginationResult{Total: int64(len(r.rows)), Page: 1, PageSize: 20}, nil
}

func (r *gptAutoTestRepo) Stats(context.Context, GPTAutoStatsFilter) (*GPTAutoStoredStats, error) {
	return &GPTAutoStoredStats{}, nil
}

func (r *gptAutoTestRepo) UsageSince(context.Context, time.Time) (GPTAutoUsageToday, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.usage, nil
}

func (r *gptAutoTestRepo) DeleteOlderThan(context.Context, time.Time, int) (int64, error) {
	return 0, nil
}

func (r *gptAutoTestRepo) OutputHistory(context.Context, []string, time.Time) ([]GPTAutoOutputHistory, error) {
	return r.history, nil
}

func (r *gptAutoTestRepo) ActualUsage(_ context.Context, requestID string, _ time.Time) (*GPTAutoActualUsage, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.actual[requestID], nil
}

func (r *gptAutoTestRepo) count() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.rows)
}

func (r *gptAutoTestRepo) waitRows(t *testing.T, n int) {
	t.Helper()
	deadline := time.After(5 * time.Second)
	for r.count() < n {
		select {
		case <-r.inserted:
		case <-deadline:
			t.Fatalf("timed out waiting for %d rows, have %d", n, r.count())
		}
	}
}

type gptAutoTestClassifier struct {
	calls  atomic.Int64
	result func() (*GPTAutoClassification, error)
	block  chan struct{}
}

func (c *gptAutoTestClassifier) Classify(ctx context.Context, _ GPTAutoShadowConfig, _ GPTAutoRequestSnapshot) (*GPTAutoClassification, error) {
	c.calls.Add(1)
	if c.block != nil {
		select {
		case <-c.block:
		case <-ctx.Done():
			return nil, &GPTAutoClassifierError{Class: GPTAutoErrTimeout, Err: ctx.Err()}
		}
	}
	if c.result != nil {
		return c.result()
	}
	return gptAutoSampleClassification(GPTAutoModelGPT6Luna, GPTAutoEffortMedium), nil
}

func gptAutoSampleClassification(model, effort string) *GPTAutoClassification {
	return &GPTAutoClassification{
		ClassifierModel: "jev-1.13.0",
		Model:           model, ModelConfidence: 0.7,
		ModelProbabilities: map[string]float64{model: 0.7},
		Effort:             effort, EffortConfidence: 0.6,
		Complexity: 2.1, Category: GPTAutoCategoryCoding,
		InputTokens: 300, OutputTokens: 4,
	}
}

func gptAutoTestPricing(model string) (*ModelPricing, error) {
	switch model {
	case typesafe.JevLatestModel:
		return &ModelPricing{InputPricePerToken: 0.042 / 1e6}, nil
	case GPTAutoModelGPT56Luna, GPTAutoModelGPT56Sol, GPTAutoModelGPT6Luna, GPTAutoModelGPT61Sol:
		// $2/M input, $10/M output.
		return &ModelPricing{InputPricePerToken: 2e-6, OutputPricePerToken: 10e-6}, nil
	}
	return nil, errors.New("unknown model")
}

func newGPTAutoTestService(t *testing.T, mutate func(*GPTAutoShadowConfig)) (*GPTAutoShadowService, *gptAutoTestRepo, *gptAutoTestClassifier, *SettingService) {
	t.Helper()
	settings := NewSettingService(newGPTAutoTestSettingRepo(), nil)
	cfg := DefaultGPTAutoShadowConfig()
	cfg.ShadowEnabled = true
	cfg.SampleRate = 100
	cfg.ClassifierAPIKey = "ts-test-key"
	cfg.ClassifierRetryCount = 0
	if mutate != nil {
		mutate(cfg)
	}
	require.NoError(t, settings.SetGPTAutoShadowConfig(context.Background(), cfg))
	repo := newGPTAutoTestRepo()
	classifier := &gptAutoTestClassifier{}
	svc := NewGPTAutoShadowService(settings, repo, classifier, gptAutoTestPricing)
	t.Cleanup(svc.Stop)
	return svc, repo, classifier, settings
}

var gptAutoTestBody = []byte(`{"model":"gpt-5.6-sol","stream":true,"reasoning_effort":"high","messages":[` +
	`{"role":"system","content":"You are helpful."},` +
	`{"role":"user","content":"Fix this Go race condition, my key is sk-proj-abcdefghijklmnop1234 and mail me at dev@example.com"}]}`)

func observeTestRequest(svc *GPTAutoShadowService, body []byte) {
	group := int64(7)
	svc.Observe(context.Background(), GPTAutoObserveInput{
		Endpoint: GPTAutoEndpointChatCompletions, Body: body, RequestID: "req-1",
		UserID: 11, APIKeyID: 22, GroupID: &group,
	})
}

// ---- 1-3: enable switch and sampling ---------------------------------------

func TestGPTAutoDefaultConfigIsSafe(t *testing.T) {
	cfg := DefaultGPTAutoShadowConfig()
	require.False(t, cfg.ShadowEnabled)
	require.Zero(t, cfg.SampleRate)
	require.Equal(t, typesafe.JevLatestModel, cfg.ClassifierModel)
	require.Len(t, cfg.ModelProfiles, 4)
}

func TestGPTAutoShadowDisabledMakesNoClassifierCalls(t *testing.T) {
	svc, repo, classifier, _ := newGPTAutoTestService(t, func(c *GPTAutoShadowConfig) { c.ShadowEnabled = false })
	svc.Start()
	for i := 0; i < 50; i++ {
		observeTestRequest(svc, gptAutoTestBody)
	}
	time.Sleep(100 * time.Millisecond)
	require.Zero(t, classifier.calls.Load())
	require.Zero(t, repo.count())
	require.Zero(t, len(svc.queue))
	require.Zero(t, svc.counters[gptAutoCounterObserved].Load())
}

func TestGPTAutoZeroSampleRateMakesNoClassifierCalls(t *testing.T) {
	svc, repo, classifier, _ := newGPTAutoTestService(t, func(c *GPTAutoShadowConfig) { c.SampleRate = 0 })
	svc.Start()
	for i := 0; i < 50; i++ {
		observeTestRequest(svc, gptAutoTestBody)
	}
	time.Sleep(100 * time.Millisecond)
	require.Zero(t, classifier.calls.Load())
	require.Zero(t, repo.count())
}

func TestGPTAutoFullSamplingEvaluatesAndPersists(t *testing.T) {
	svc, repo, classifier, _ := newGPTAutoTestService(t, nil)
	svc.Start()
	observeTestRequest(svc, gptAutoTestBody)
	repo.waitRows(t, 1)

	require.EqualValues(t, 1, classifier.calls.Load())
	row := repo.rows[0]
	require.Equal(t, GPTAutoEvaluationSuccess, row.EvaluationStatus)
	require.Equal(t, "gpt-5.6-sol", row.ActualModel, "actual production model is recorded, not rewritten")
	require.Equal(t, "high", row.ActualEffort)
	require.Equal(t, GPTAutoModelGPT6Luna, row.FinalModel)
	require.Equal(t, "req-1", row.RequestID)
	require.NotEmpty(t, row.PolicySnapshotHash)
	require.NotNil(t, row.Classification)
	require.InDelta(t, 300*0.042/1e6, row.ClassifierCost, 1e-15)
}

func TestGPTAutoSamplingIsDeterministicAndProportional(t *testing.T) {
	body := []byte(`{"model":"x","messages":[{"role":"user","content":"hello"}]}`)
	first := gptAutoSampled(42, body, 37)
	for i := 0; i < 10; i++ {
		require.Equal(t, first, gptAutoSampled(42, body, 37), "same key and body must sample consistently")
	}
	require.True(t, gptAutoSampled(1, body, 100))
	require.False(t, gptAutoSampled(1, body, 0))

	hits := 0
	for key := int64(0); key < 20000; key++ {
		if gptAutoSampled(key, body, 25) {
			hits++
		}
	}
	require.InDelta(t, 5000, hits, 400, "25%% sampling should hit roughly a quarter of keys")
}

func TestGPTAutoFiltersSkipOutOfScopeRequests(t *testing.T) {
	svc, repo, classifier, _ := newGPTAutoTestService(t, func(c *GPTAutoShadowConfig) {
		c.AllGroups = false
		c.GroupIDs = []int64{999}
	})
	svc.Start()
	observeTestRequest(svc, gptAutoTestBody)
	time.Sleep(100 * time.Millisecond)
	require.Zero(t, classifier.calls.Load())
	require.Zero(t, repo.count())
	require.EqualValues(t, 1, svc.counters[gptAutoCounterSkippedFilter].Load())
}

// ---- 4-5: failure isolation and saturation ---------------------------------

func TestGPTAutoClassifierFailureNeverBlocksObserve(t *testing.T) {
	svc, repo, classifier, _ := newGPTAutoTestService(t, func(c *GPTAutoShadowConfig) { c.ClassifierTimeoutMS = 200 })
	classifier.block = make(chan struct{}) // never released: every call times out
	svc.Start()

	started := time.Now()
	observeTestRequest(svc, gptAutoTestBody)
	require.Less(t, time.Since(started), 50*time.Millisecond, "Observe must return without waiting for Jev")

	repo.waitRows(t, 1)
	require.Equal(t, GPTAutoEvaluationFailed, repo.rows[0].EvaluationStatus)
	require.Equal(t, GPTAutoErrTimeout, repo.rows[0].ErrorClass)
	require.Zero(t, repo.rows[0].ClassifierCost)
}

func TestGPTAutoQueueSaturationDropsWithoutBlocking(t *testing.T) {
	svc, _, classifier, _ := newGPTAutoTestService(t, func(c *GPTAutoShadowConfig) { c.QueueSize = 3 })
	classifier.block = make(chan struct{})
	// Workers not started: the queue can only fill.
	started := time.Now()
	for i := 0; i < 100; i++ {
		observeTestRequest(svc, gptAutoTestBody)
	}
	require.Less(t, time.Since(started), time.Second)
	require.Equal(t, 3, len(svc.queue))
	require.EqualValues(t, 97, svc.counters[gptAutoCounterDropped].Load())
	require.Zero(t, classifier.calls.Load())
}

func TestGPTAutoCircuitBreakerOpensAfterConsecutiveFailures(t *testing.T) {
	var breaker gptAutoBreaker
	now := time.Now()
	for i := 0; i < gptAutoBreakerThreshold; i++ {
		require.True(t, breaker.allow(now))
		breaker.record(false, now)
	}
	require.Equal(t, "open", breaker.state(now))
	require.False(t, breaker.allow(now))

	later := now.Add(gptAutoBreakerCooldown + time.Second)
	require.True(t, breaker.allow(later), "one half-open probe after cool-down")
	require.False(t, breaker.allow(later), "only one probe at a time")
	breaker.record(true, later)
	require.Equal(t, "closed", breaker.state(later))
}

func TestGPTAutoDailyBudgetStopsSampling(t *testing.T) {
	svc, repo, classifier, _ := newGPTAutoTestService(t, func(c *GPTAutoShadowConfig) { c.ClassifierDailyCallLimit = 5 })
	// Persisted decisions seed an empty shared budget (e.g. after a Redis flush).
	repo.usage = GPTAutoUsageToday{Calls: 5}
	svc.refreshBudget(context.Background(), true)
	svc.Start()
	observeTestRequest(svc, gptAutoTestBody)
	time.Sleep(100 * time.Millisecond)
	require.Zero(t, classifier.calls.Load())
	require.EqualValues(t, 1, svc.counters[gptAutoCounterSkippedBudget].Load())
}

// ---- 6-8: policy -------------------------------------------------------------

func gptAutoPolicyInput(cls GPTAutoClassification, mutate func(*GPTAutoShadowConfig)) GPTAutoPolicyInput {
	cfg := DefaultGPTAutoShadowConfig()
	if mutate != nil {
		mutate(cfg)
	}
	return GPTAutoPolicyInput{
		Classification: cls,
		Features:       GPTAutoRequestFeatures{Endpoint: GPTAutoEndpointChatCompletions, EstimatedInputTokens: 1000},
		Config:         *cfg,
		Pricing:        gptAutoTestPricing,
		OutputEstimate: func(string, string) GPTAutoOutputEstimate {
			return GPTAutoOutputEstimate{Tokens: 500, Source: GPTAutoOutputSourceModelEffort, Samples: 50}
		},
	}
}

func TestGPTAutoCapabilityMatrixMatchesGateway(t *testing.T) {
	require.NotContains(t, gptAutoNativeSupportedEfforts(GPTAutoModelGPT61Sol), GPTAutoEffortNone)
	require.Contains(t, gptAutoNativeSupportedEfforts(GPTAutoModelGPT61Sol), GPTAutoEffortMax)
	require.Contains(t, gptAutoNativeSupportedEfforts(GPTAutoModelGPT6Luna), GPTAutoEffortNone)
	for _, model := range GPTAutoModelPool() {
		require.Contains(t, gptAutoNativeSupportedEfforts(model), GPTAutoEffortMax, model)
	}
}

func TestGPTAutoPolicyRecordsUnsupportedEffortCorrection(t *testing.T) {
	cls := *gptAutoSampleClassification(GPTAutoModelGPT61Sol, GPTAutoEffortNone)
	decision := EvaluateGPTAutoPolicy(gptAutoPolicyInput(cls, nil))
	require.Equal(t, GPTAutoDecisionAdjusted, decision.Status)
	require.Equal(t, GPTAutoEffortNone, decision.JevEffort, "raw Jev effort is preserved")
	require.Equal(t, GPTAutoEffortLow, decision.FinalEffort, "raised to nearest supported level")
	require.Equal(t, GPTAutoModelGPT61Sol, decision.FinalModel)
	require.Equal(t, GPTAutoRuleEffortUnsupported, decision.Adjustments[0].Rule)
	require.NotEmpty(t, decision.CapabilityWarnings)
}

func TestGPTAutoPolicyNeverPicksIncompatibleModel(t *testing.T) {
	cls := *gptAutoSampleClassification(GPTAutoModelGPT61Sol, GPTAutoEffortHigh)
	cls.ModelProbabilities = map[string]float64{GPTAutoModelGPT61Sol: 0.6, GPTAutoModelGPT6Luna: 0.3, GPTAutoModelGPT56Sol: 0.1}
	input := gptAutoPolicyInput(cls, func(c *GPTAutoShadowConfig) {
		for i := range c.ModelProfiles {
			if c.ModelProfiles[i].Model == GPTAutoModelGPT61Sol {
				c.ModelProfiles[i].SupportsTools = false
				c.ModelProfiles[i].RoutingWeight = 1000
				c.ModelProfiles[i].BillingMultiplier = 99
			}
		}
	})
	input.Features.HasTools = true
	decision := EvaluateGPTAutoPolicy(input)
	require.Equal(t, GPTAutoModelGPT6Luna, decision.FinalModel, "highest-probability eligible model")
	require.Contains(t, decision.CapabilityWarnings, GPTAutoRuleModelTools)

	for i := range input.Config.ModelProfiles {
		input.Config.ModelProfiles[i].Enabled = false
	}
	decision = EvaluateGPTAutoPolicy(input)
	require.Equal(t, GPTAutoDecisionNoEligible, decision.Status)
	require.Empty(t, decision.FinalModel, "no route is fabricated")
}

func TestGPTAutoBillingMultiplierDoesNotAffectRouting(t *testing.T) {
	cls := *gptAutoSampleClassification(GPTAutoModelGPT56Luna, GPTAutoEffortMedium)
	cls.ModelProbabilities = map[string]float64{GPTAutoModelGPT56Luna: 0.5, GPTAutoModelGPT6Luna: 0.48}

	base := EvaluateGPTAutoPolicy(gptAutoPolicyInput(cls, func(c *GPTAutoShadowConfig) {
		for i := range c.ModelProfiles {
			c.ModelProfiles[i].RoutingWeight = 50
		}
	}))
	require.Equal(t, GPTAutoModelGPT56Luna, base.FinalModel)

	richer := EvaluateGPTAutoPolicy(gptAutoPolicyInput(cls, func(c *GPTAutoShadowConfig) {
		for i := range c.ModelProfiles {
			c.ModelProfiles[i].RoutingWeight = 50
			if c.ModelProfiles[i].Model == GPTAutoModelGPT6Luna {
				c.ModelProfiles[i].BillingMultiplier = 50
			}
		}
	}))
	require.Equal(t, base.FinalModel, richer.FinalModel, "billing multiplier must not change the route")
	require.NotEqual(t, base.Economics.EstimatedRevenue, 0.0)
}

func TestGPTAutoRoutingWeightOnlyBreaksNearTies(t *testing.T) {
	cls := *gptAutoSampleClassification(GPTAutoModelGPT56Luna, GPTAutoEffortMedium)
	weighted := func(c *GPTAutoShadowConfig) {
		for i := range c.ModelProfiles {
			c.ModelProfiles[i].RoutingWeight = 10
			if c.ModelProfiles[i].Model == GPTAutoModelGPT6Luna {
				c.ModelProfiles[i].RoutingWeight = 500
			}
		}
	}
	cls.ModelProbabilities = map[string]float64{GPTAutoModelGPT56Luna: 0.50, GPTAutoModelGPT6Luna: 0.47}
	decision := EvaluateGPTAutoPolicy(gptAutoPolicyInput(cls, weighted))
	require.Equal(t, GPTAutoModelGPT6Luna, decision.FinalModel)
	require.Equal(t, GPTAutoRuleRoutingWeightTie, decision.Adjustments[0].Rule)

	cls.ModelProbabilities = map[string]float64{GPTAutoModelGPT56Luna: 0.70, GPTAutoModelGPT6Luna: 0.20}
	decision = EvaluateGPTAutoPolicy(gptAutoPolicyInput(cls, weighted))
	require.Equal(t, GPTAutoModelGPT56Luna, decision.FinalModel, "weight cannot override a clear quality signal")
}

func TestGPTAutoProfitCalculation(t *testing.T) {
	cls := *gptAutoSampleClassification(GPTAutoModelGPT6Luna, GPTAutoEffortMedium)
	input := gptAutoPolicyInput(cls, func(c *GPTAutoShadowConfig) {
		c.GlobalMinMargin, c.ProfitSafetyBuffer = 0.30, 0.05
		for i := range c.ModelProfiles {
			if c.ModelProfiles[i].Model == GPTAutoModelGPT6Luna {
				c.ModelProfiles[i].BillingMultiplier = 2
				c.ModelProfiles[i].UpstreamCosts = []GPTAutoUpstreamCost{
					{Channel: "expensive", Enabled: true, InputPerM: 3, OutputPerM: 12},
					{Channel: "cheap", Enabled: true, InputPerM: 1, OutputPerM: 5},
					{Channel: "disabled", Enabled: false, InputPerM: 0.1, OutputPerM: 0.1},
				}
			}
		}
	})
	input.ClassifierCostUSD = 0.001
	economics := EstimateGPTAutoEconomics(input, GPTAutoModelGPT6Luna, GPTAutoEffortMedium)
	// list = 1000*2/M + 500*10/M = 0.007; revenue = 0.014
	// upstream (cheap) = 1000*1/M + 500*5/M = 0.0035
	require.True(t, economics.Hypothetical)
	require.Equal(t, "cheap", economics.UpstreamChannel)
	require.InDelta(t, 0.014, economics.EstimatedRevenue, 1e-12)
	require.InDelta(t, 0.0035, economics.EstimatedUpstreamCost, 1e-12)
	require.InDelta(t, 0.0095, economics.EstimatedGrossProfit, 1e-12)
	require.InDelta(t, 0.0095/0.014, *economics.EstimatedGrossMargin, 1e-6, "margins are rounded to 6 decimals")
	require.InDelta(t, 0.35, economics.TargetMargin, 1e-12)
	require.False(t, economics.BelowTarget)
	// Model-only: margin (0.014-0.0035)/0.014 = 75%.
	require.InDelta(t, 0.75, *economics.ModelGrossMargin, 1e-9)
	require.Equal(t, GPTAutoMarginMeetsTarget, economics.ModelMarginStatus)
	require.Equal(t, GPTAutoMarginMeetsTarget, economics.FullyLoadedMarginStatus)
	// required multiplier: model-only 0.0035/((1-0.35)*0.007); fully loaded
	// (0.0035+0.001)/((1-0.35)*0.007)
	require.InDelta(t, 0.7692, *economics.MinRequiredMultiplier, 0.0001)
	require.InDelta(t, 0.989, *economics.MinRequiredMultiplierFullyLoaded, 0.001)
}

func TestGPTAutoProfitNegativeMargin(t *testing.T) {
	cls := *gptAutoSampleClassification(GPTAutoModelGPT6Luna, GPTAutoEffortMedium)
	input := gptAutoPolicyInput(cls, func(c *GPTAutoShadowConfig) {
		for i := range c.ModelProfiles {
			if c.ModelProfiles[i].Model == GPTAutoModelGPT6Luna {
				c.ModelProfiles[i].BillingMultiplier = 1.0
				c.ModelProfiles[i].UpstreamCosts = []GPTAutoUpstreamCost{{Channel: "pricey", Enabled: true, InputPerM: 4, OutputPerM: 20}}
			}
		}
	})
	decision := EvaluateGPTAutoPolicy(input)
	require.Less(t, *decision.Economics.EstimatedGrossMargin, 0.0)
	require.True(t, decision.Economics.BelowTarget)
	require.Equal(t, GPTAutoModelGPT6Luna, decision.FinalModel, "a low margin never changes the recommendation")
	var rules []string
	for _, warning := range decision.Warnings {
		rules = append(rules, warning.Rule)
	}
	require.Contains(t, rules, GPTAutoRuleModelMarginLoss)
	require.Equal(t, GPTAutoDecisionAccepted, decision.Status, "a margin warning is not a route modification")
	for _, adjustment := range decision.Adjustments {
		require.NotContains(t, []string{GPTAutoRuleModelMarginLoss, GPTAutoRuleMarginBelowTarget}, adjustment.Rule)
	}
}

func TestGPTAutoProfitWithoutPricingOrHistory(t *testing.T) {
	cls := *gptAutoSampleClassification(GPTAutoModelGPT6Luna, GPTAutoEffortMedium)
	input := gptAutoPolicyInput(cls, nil)
	input.Pricing = func(string) (*ModelPricing, error) { return nil, errors.New("missing") }
	input.OutputEstimate = nil
	economics := EstimateGPTAutoEconomics(input, GPTAutoModelGPT6Luna, GPTAutoEffortMedium)
	require.False(t, economics.PricingAvailable)
	require.Nil(t, economics.EstimatedGrossMargin)
	require.Equal(t, GPTAutoOutputSourceNone, economics.OutputEstimateSource)
}

// ---- 9: settings persistence and validation ----------------------------------

func TestGPTAutoConfigPersistsAndReloads(t *testing.T) {
	repo := newGPTAutoTestSettingRepo()
	settings := NewSettingService(repo, nil)
	cfg := DefaultGPTAutoShadowConfig()
	cfg.ShadowEnabled = true
	cfg.SampleRate = 12
	cfg.ModelProfiles[0].BillingMultiplier = 1.33
	cfg.ModelProfiles[0].RoutingWeight = 77
	require.NoError(t, settings.SetGPTAutoShadowConfig(context.Background(), cfg))

	reloaded, err := NewSettingService(repo, nil).GetGPTAutoShadowConfig(context.Background())
	require.NoError(t, err)
	require.True(t, reloaded.ShadowEnabled)
	require.Equal(t, 12, reloaded.SampleRate)
	require.Equal(t, 1.33, reloaded.ModelProfiles[0].BillingMultiplier)
	require.Equal(t, 77, reloaded.ModelProfiles[0].RoutingWeight)
	require.Equal(t, 12, settings.GetGPTAutoShadowConfigCached(context.Background()).SampleRate)
}

func TestGPTAutoConfigCorruptValueFallsBackToSafeDefaults(t *testing.T) {
	repo := newGPTAutoTestSettingRepo()
	require.NoError(t, repo.Set(context.Background(), SettingKeyGPTAutoShadowConfig, "{not json"))
	cfg, err := NewSettingService(repo, nil).GetGPTAutoShadowConfig(context.Background())
	require.NoError(t, err)
	require.False(t, cfg.ShadowEnabled)
}

func TestGPTAutoConfigValidation(t *testing.T) {
	for name, mutate := range map[string]func(*GPTAutoShadowConfig){
		"sample rate over 100":   func(c *GPTAutoShadowConfig) { c.SampleRate = 101 },
		"negative sample rate":   func(c *GPTAutoShadowConfig) { c.SampleRate = -1 },
		"negative multiplier":    func(c *GPTAutoShadowConfig) { c.ModelProfiles[0].BillingMultiplier = -1 },
		"zero multiplier":        func(c *GPTAutoShadowConfig) { c.ModelProfiles[0].BillingMultiplier = 0 },
		"unsupported model":      func(c *GPTAutoShadowConfig) { c.ModelProfiles[0].Model = "gpt-9" },
		"duplicate model":        func(c *GPTAutoShadowConfig) { c.ModelProfiles[1].Model = c.ModelProfiles[0].Model },
		"unknown effort":         func(c *GPTAutoShadowConfig) { c.ModelProfiles[0].SupportedEfforts = []string{"ultra"} },
		"effort model can't run": func(c *GPTAutoShadowConfig) { c.ModelProfiles[3].SupportedEfforts = []string{"none"} },
		"margin of 100%":         func(c *GPTAutoShadowConfig) { c.GlobalMinMargin = 1 },
		"negative upstream cost": func(c *GPTAutoShadowConfig) {
			c.ModelProfiles[0].UpstreamCosts = []GPTAutoUpstreamCost{{Channel: "a", InputPerM: -1}}
		},
		"timeout too small":       func(c *GPTAutoShadowConfig) { c.ClassifierTimeoutMS = 10 },
		"no groups":               func(c *GPTAutoShadowConfig) { c.AllGroups = false; c.GroupIDs = nil },
		"private classifier host": func(c *GPTAutoShadowConfig) { c.ClassifierBaseURL = "https://127.0.0.1" },
		"plain http classifier":   func(c *GPTAutoShadowConfig) { c.ClassifierBaseURL = "http://api.typesafe.ai" },
		"unsupported endpoint":    func(c *GPTAutoShadowConfig) { c.Endpoints = []string{"messages"} },
	} {
		t.Run(name, func(t *testing.T) {
			cfg := DefaultGPTAutoShadowConfig()
			mutate(cfg)
			require.Error(t, ValidateGPTAutoShadowConfig(cfg))
		})
	}
	require.NoError(t, ValidateGPTAutoShadowConfig(DefaultGPTAutoShadowConfig()))
}

// ---- 10: replay -------------------------------------------------------------

func TestGPTAutoReplayDoesNotCallClassifier(t *testing.T) {
	svc, repo, classifier, settings := newGPTAutoTestService(t, nil)
	svc.Start()
	observeTestRequest(svc, gptAutoTestBody)
	repo.waitRows(t, 1)
	require.EqualValues(t, 1, classifier.calls.Load())

	historical, err := svc.Replay(context.Background(), 1, GPTAutoReplayInput{Mode: "historical"})
	require.NoError(t, err)
	require.False(t, historical.Changed)
	require.Equal(t, GPTAutoModelGPT6Luna, historical.Replayed.FinalModel)

	cfg, err := settings.GetGPTAutoShadowConfig(context.Background())
	require.NoError(t, err)
	profiles := append([]GPTAutoModelProfile(nil), cfg.ModelProfiles...)
	for i := range profiles {
		if profiles[i].Model == GPTAutoModelGPT6Luna {
			profiles[i].Enabled = false
		}
	}
	custom, err := svc.Replay(context.Background(), 1, GPTAutoReplayInput{Mode: "custom", ModelProfiles: profiles})
	require.NoError(t, err)
	require.True(t, custom.Changed)
	require.NotEqual(t, GPTAutoModelGPT6Luna, custom.Replayed.FinalModel)

	require.EqualValues(t, 1, classifier.calls.Load(), "replay must not call Jev")
}

// ---- 12-13: no customer impact -------------------------------------------------

func TestGPTAutoObserveDoesNotMutateRequestBody(t *testing.T) {
	svc, repo, _, _ := newGPTAutoTestService(t, nil)
	svc.Start()
	body := append([]byte(nil), gptAutoTestBody...)
	observeTestRequest(svc, body)
	repo.waitRows(t, 1)
	require.True(t, bytes.Equal(gptAutoTestBody, body))
}

func TestGPTAutoClassifierCostIsInternalOnly(t *testing.T) {
	// The service has no dependency able to charge a customer: its only sink
	// for classifier cost is the shadow decision row.
	svc, repo, _, _ := newGPTAutoTestService(t, nil)
	svc.Start()
	observeTestRequest(svc, gptAutoTestBody)
	repo.waitRows(t, 1)
	require.Greater(t, repo.rows[0].ClassifierCost, 0.0)
	require.True(t, repo.rows[0].Decision.Economics.Hypothetical)
}

// ---- snapshot + redaction -------------------------------------------------------

func TestGPTAutoSnapshotIsBoundedAndRedacted(t *testing.T) {
	snapshot := BuildGPTAutoSnapshot(GPTAutoEndpointChatCompletions, gptAutoTestBody, GPTAutoContextOptions{MaxChars: 4000, ContextTurns: 4})
	require.Equal(t, "gpt-5.6-sol", snapshot.RequestedModel)
	require.Equal(t, "high", snapshot.RequestedEffort)
	require.True(t, snapshot.Stream)
	require.Contains(t, snapshot.LatestUser, "race condition")
	require.NotContains(t, snapshot.LatestUser, "sk-proj-abcdefghijklmnop1234")
	require.NotContains(t, snapshot.LatestUser, "dev@example.com")
	// System prompts describe the client application, not the task: only
	// their presence is forwarded (state-v2).
	require.Empty(t, snapshot.RecentContext)
	require.True(t, snapshot.HasSystemPrompt)
	state, err := buildGPTAutoClassifierState(snapshot)
	require.NoError(t, err)
	require.NotContains(t, state, "You are helpful.", "system message text is never forwarded")

	huge := []byte(`{"model":"m","messages":[{"role":"user","content":"` + string(bytes.Repeat([]byte("a"), 50000)) + `"}]}`)
	snapshot = BuildGPTAutoSnapshot(GPTAutoEndpointChatCompletions, huge, GPTAutoContextOptions{MaxChars: 1000, ContextTurns: 4})
	require.LessOrEqual(t, len([]rune(snapshot.LatestUser)), 750)
	require.Equal(t, 12500, snapshot.EstimatedInputTokens)

	responses := []byte(`{"model":"m","tools":[{"type":"function"}],"input":[{"role":"user","content":[{"type":"input_text","text":"plan a trip"},{"type":"input_image","image_url":"x"}]}]}`)
	snapshot = BuildGPTAutoSnapshot(GPTAutoEndpointResponses, responses, GPTAutoContextOptions{MaxChars: 1000, ContextTurns: 4})
	require.True(t, snapshot.HasTools)
	require.True(t, snapshot.HasImages)
	require.Equal(t, "plan a trip", snapshot.LatestUser)
}

func TestGPTAutoQuestionsPassSystemOneValidation(t *testing.T) {
	cfg := DefaultGPTAutoShadowConfig()
	state, err := buildGPTAutoClassifierState(BuildGPTAutoSnapshot(GPTAutoEndpointChatCompletions, gptAutoTestBody, GPTAutoContextOptions{MaxChars: 4000, ContextTurns: 4}))
	require.NoError(t, err)
	require.NotContains(t, state, "gpt-5.6-sol", "requested model is withheld from Jev")
	raw, err := json.Marshal(typesafe.Request{Model: typesafe.JevLatestModel, State: state, Questions: buildGPTAutoQuestions(*cfg)})
	require.NoError(t, err)
	_, err = typesafe.ValidateSystemOneRequest(raw)
	require.NoError(t, err)
}
