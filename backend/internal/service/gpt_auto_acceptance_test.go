package service

import (
	"bytes"
	"context"
	"errors"
	"net/http"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/ctxkey"
	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/stretchr/testify/require"
)

// Regression tests for defects found in the Phase 1 acceptance audit.

// blockingBudgetStore never answers; any call from the request path would hang.
type blockingBudgetStore struct{ release chan struct{} }

func (s blockingBudgetStore) wait(ctx context.Context) error {
	select {
	case <-s.release:
		return errors.New("released")
	case <-ctx.Done():
		return ctx.Err()
	}
}
func (s blockingBudgetStore) Reserve(ctx context.Context, _ string, _ GPTAutoBudgetLimits, _ int64) (GPTAutoBudgetUsage, bool, error) {
	return GPTAutoBudgetUsage{}, false, s.wait(ctx)
}
func (s blockingBudgetStore) Adjust(ctx context.Context, _ string, _, _ int64) error {
	return s.wait(ctx)
}
func (s blockingBudgetStore) Seed(ctx context.Context, _ string, _ GPTAutoBudgetUsage) error {
	return s.wait(ctx)
}
func (s blockingBudgetStore) Usage(ctx context.Context, _ string) (GPTAutoBudgetUsage, bool, error) {
	return GPTAutoBudgetUsage{}, false, s.wait(ctx)
}
func (s blockingBudgetStore) AddCounters(ctx context.Context, _ string, _ map[string]int64) error {
	return s.wait(ctx)
}
func (s blockingBudgetStore) Counters(ctx context.Context, _ string) (map[string]int64, error) {
	return nil, s.wait(ctx)
}

// failingBudgetStore models Redis being down.
type failingBudgetStore struct{}

var errBudgetDown = errors.New("redis down")

func (failingBudgetStore) Reserve(context.Context, string, GPTAutoBudgetLimits, int64) (GPTAutoBudgetUsage, bool, error) {
	return GPTAutoBudgetUsage{}, false, errBudgetDown
}
func (failingBudgetStore) Adjust(context.Context, string, int64, int64) error { return errBudgetDown }
func (failingBudgetStore) Seed(context.Context, string, GPTAutoBudgetUsage) error {
	return errBudgetDown
}
func (failingBudgetStore) Usage(context.Context, string) (GPTAutoBudgetUsage, bool, error) {
	return GPTAutoBudgetUsage{}, false, errBudgetDown
}
func (failingBudgetStore) AddCounters(context.Context, string, map[string]int64) error {
	return errBudgetDown
}
func (failingBudgetStore) Counters(context.Context, string) (map[string]int64, error) {
	return nil, errBudgetDown
}

func gptAutoTestTask(svc *GPTAutoShadowService) *gptAutoTask {
	cfg := svc.settings.GetGPTAutoShadowConfigCached(context.Background())
	snapshot := BuildGPTAutoSnapshot(GPTAutoEndpointChatCompletions, gptAutoTestBody, cfg.ContextOptions())
	snapshot.RequestID = "req-task"
	return &gptAutoTask{snapshot: snapshot, enqueuedAt: svc.now()}
}

func TestGPTAutoObservePerformsNoIOOnHotPath(t *testing.T) {
	svc, _, classifier, _ := newGPTAutoTestService(t, func(c *GPTAutoShadowConfig) {
		c.ClassifierDailyCallLimit = 10
		c.ClassifierDailyCostLimit = 1
	})
	store := blockingBudgetStore{release: make(chan struct{})}
	defer close(store.release)
	svc.budget = store
	classifier.block = make(chan struct{})

	started := time.Now()
	for i := 0; i < 200; i++ {
		observeTestRequest(svc, gptAutoTestBody)
	}
	require.Less(t, time.Since(started), time.Second, "Observe must never wait on the budget store or database")
	require.EqualValues(t, 200, svc.counters[gptAutoCounterSampled].Load())
}

func TestGPTAutoDailyBudgetIsGlobalAcrossNodes(t *testing.T) {
	const limit = 4
	shared := newLocalGPTAutoBudgetStore()
	var nodes []*GPTAutoShadowService
	var repos []*gptAutoTestRepo
	var classifiers []*gptAutoTestClassifier
	for i := 0; i < 3; i++ {
		svc, repo, classifier, _ := newGPTAutoTestService(t, func(c *GPTAutoShadowConfig) {
			c.ClassifierDailyCallLimit = limit
			c.WorkerCount = 4
		})
		svc.budget = shared
		nodes, repos, classifiers = append(nodes, svc), append(repos, repo), append(classifiers, classifier)
	}
	var wg sync.WaitGroup
	for _, svc := range nodes {
		wg.Add(1)
		go func(svc *GPTAutoShadowService) {
			defer wg.Done()
			for i := 0; i < 10; i++ {
				svc.process(gptAutoTestTask(svc))
			}
		}(svc)
	}
	wg.Wait()

	calls, rows, skipped := int64(0), 0, int64(0)
	for i := range nodes {
		calls += classifiers[i].calls.Load()
		rows += repos[i].count()
		skipped += nodes[i].counters[gptAutoCounterSkippedBudget].Load()
	}
	require.EqualValues(t, limit, calls, "three nodes must share one daily call budget")
	require.Equal(t, limit, rows)
	require.EqualValues(t, 30-limit, skipped)
	usage, _, _ := shared.Usage(context.Background(), gptAutoDayStart(time.Now()).Format(time.DateOnly))
	require.EqualValues(t, limit, usage.Calls)
	require.Greater(t, usage.CostNanoUSD, int64(0), "reservations are settled to the billed cost")
}

func TestGPTAutoCostBudgetSettlesToBilledCost(t *testing.T) {
	svc, _, _, _ := newGPTAutoTestService(t, func(c *GPTAutoShadowConfig) { c.ClassifierDailyCostLimit = 1 })
	svc.process(gptAutoTestTask(svc))
	usage, _, _ := svc.budget.Usage(context.Background(), gptAutoDayStart(time.Now()).Format(time.DateOnly))
	require.EqualValues(t, 1, usage.Calls)
	// 300 input tokens at $0.042/M; the larger reservation estimate was refunded.
	require.Equal(t, gptAutoUSDToNano(300*0.042/1e6), usage.CostNanoUSD)
}

func TestGPTAutoBudgetFailsClosedWhenStoreUnavailable(t *testing.T) {
	svc, repo, classifier, _ := newGPTAutoTestService(t, func(c *GPTAutoShadowConfig) { c.ClassifierDailyCallLimit = 100 })
	svc.budget = failingBudgetStore{}
	svc.process(gptAutoTestTask(svc))
	require.Zero(t, classifier.calls.Load(), "a configured limit that cannot be checked must not be bypassed")
	require.Zero(t, repo.count())
	require.EqualValues(t, 1, svc.counters[gptAutoCounterSkippedBudgetDown].Load())

	unlimited, _, unlimitedClassifier, _ := newGPTAutoTestService(t, func(c *GPTAutoShadowConfig) {
		c.ClassifierDailyCallLimit, c.ClassifierDailyCostLimit = 0, 0
	})
	unlimited.budget = failingBudgetStore{}
	unlimited.process(gptAutoTestTask(unlimited))
	require.EqualValues(t, 1, unlimitedClassifier.calls.Load(), "with no limit the store only feeds the dashboard")
}

func TestGPTAutoBreakerProbeIsReleasedWhenCallIsSkipped(t *testing.T) {
	svc, _, classifier, _ := newGPTAutoTestService(t, func(c *GPTAutoShadowConfig) { c.ClassifierDailyCallLimit = 1 })
	opened := time.Now().Add(-time.Hour)
	for i := 0; i < gptAutoBreakerThreshold; i++ {
		svc.breaker.record(false, opened)
	}
	require.NoError(t, svc.budget.Seed(context.Background(), gptAutoDayStart(time.Now()).Format(time.DateOnly), GPTAutoBudgetUsage{Calls: 1}))

	svc.process(gptAutoTestTask(svc)) // half-open probe admitted by the breaker, denied by the budget
	require.Zero(t, classifier.calls.Load())
	require.Equal(t, "half_open", svc.breaker.state(time.Now()), "a skipped probe must not close the breaker")
	require.True(t, svc.breaker.allow(time.Now()), "the unused probe slot must be returned")
}

func TestGPTAutoBilledInvalidAnswerIsCosted(t *testing.T) {
	svc, repo, classifier, _ := newGPTAutoTestService(t, func(c *GPTAutoShadowConfig) { c.ClassifierDailyCostLimit = 1 })
	classifier.result = func() (*GPTAutoClassification, error) {
		return nil, &GPTAutoClassifierError{Class: GPTAutoErrInvalidAnswer, StatusCode: http.StatusOK,
			ClassifierModel: "jev-1.13.0", InputTokens: 500, OutputTokens: 6}
	}
	svc.process(gptAutoTestTask(svc))
	require.Equal(t, 1, repo.count())
	row := repo.rows[0]
	require.Equal(t, GPTAutoEvaluationFailed, row.EvaluationStatus)
	require.Equal(t, GPTAutoErrInvalidAnswer, row.ErrorClass)
	require.Equal(t, 500, row.ClassifierInputTokens)
	require.InDelta(t, 500*0.042/1e6, row.ClassifierCost, 1e-15)
	usage, _, _ := svc.budget.Usage(context.Background(), gptAutoDayStart(time.Now()).Format(time.DateOnly))
	require.Equal(t, gptAutoUSDToNano(500*0.042/1e6), usage.CostNanoUSD, "a billed failure counts against the budget")
}

func TestGPTAutoRetriesEachReserveBudget(t *testing.T) {
	svc, repo, classifier, _ := newGPTAutoTestService(t, func(c *GPTAutoShadowConfig) {
		c.ClassifierRetryCount = 2
		c.ClassifierDailyCallLimit = 2
	})
	classifier.result = func() (*GPTAutoClassification, error) {
		return nil, &GPTAutoClassifierError{Class: GPTAutoErrUpstream, StatusCode: http.StatusBadGateway}
	}
	svc.process(gptAutoTestTask(svc))
	require.EqualValues(t, 2, classifier.calls.Load(), "the third attempt exceeds the daily call limit")
	require.Equal(t, 2, repo.rows[0].Attempts)
	require.Equal(t, GPTAutoErrUpstream, repo.rows[0].ErrorClass)
}

func TestGPTAutoRunTestHonoursBudget(t *testing.T) {
	svc, _, classifier, _ := newGPTAutoTestService(t, func(c *GPTAutoShadowConfig) { c.ClassifierDailyCallLimit = 1 })
	_, err := svc.RunTest(context.Background(), GPTAutoTestInput{Prompt: "Prove that sqrt(2) is irrational."})
	require.NoError(t, err)
	_, err = svc.RunTest(context.Background(), GPTAutoTestInput{Prompt: "Again"})
	require.Error(t, err)
	require.Equal(t, http.StatusTooManyRequests, infraerrors.Code(err), "the test lab cannot bypass the daily limit")
	require.EqualValues(t, 1, classifier.calls.Load())

	svc.budget = failingBudgetStore{}
	_, err = svc.RunTest(context.Background(), GPTAutoTestInput{Prompt: "x"})
	require.Equal(t, http.StatusServiceUnavailable, infraerrors.Code(err))
}

func TestGPTAutoQueueMemoryIsBounded(t *testing.T) {
	svc, _, _, _ := newGPTAutoTestService(t, func(c *GPTAutoShadowConfig) { c.QueueSize = gptAutoMaxQueueSize })
	svc.queuedBytes.Store(gptAutoMaxQueuedBytes - 10)
	observeTestRequest(svc, gptAutoTestBody)
	require.Zero(t, len(svc.queue))
	require.EqualValues(t, 1, svc.counters[gptAutoCounterDropped].Load())
	require.EqualValues(t, gptAutoMaxQueuedBytes-10, svc.queuedBytes.Load(), "a rejected task releases its reservation")

	svc.queuedBytes.Store(0)
	observeTestRequest(svc, gptAutoTestBody)
	require.Equal(t, 1, len(svc.queue))
	require.Positive(t, svc.queuedBytes.Load())
}

func TestGPTAutoSnapshotOfHugeBodyIsBoundedAndDetached(t *testing.T) {
	secret := "sk-proj-" + strings.Repeat("Z", 40)
	filler := strings.Repeat("lorem ipsum ", 400_000) // ~4.8 MB
	content := filler + " " + secret + " " + filler + " please fix my code " + secret
	body := []byte(`{"model":"gpt-6-luna","messages":[{"role":"system","content":"` + filler + `"},{"role":"user","content":"` + content + `"}]}`)

	started := time.Now()
	snapshot := BuildGPTAutoSnapshot(GPTAutoEndpointChatCompletions, body, GPTAutoContextOptions{MaxChars: 4000, ContextTurns: 4})
	elapsed := time.Since(started)
	require.Less(t, elapsed, 2*time.Second, "snapshot cost must not scale with regexes over megabytes")
	require.LessOrEqual(t, len([]rune(snapshot.LatestUser)), 3000)
	require.Contains(t, snapshot.LatestUser, "please fix my code")
	require.NotContains(t, snapshot.LatestUser, "ZZZZZZZZZZ", "secrets at the kept tail are redacted")
	for _, turn := range snapshot.RecentContext {
		require.LessOrEqual(t, len([]rune(turn.Text)), 4000)
	}
	require.Greater(t, snapshot.EstimatedInputTokens, 2_000_000, "token estimate still reflects the full prompt")

	before := snapshot.LatestUser
	for i := range body {
		body[i] = 'x'
	}
	require.Equal(t, before, snapshot.LatestUser, "snapshot shares no memory with the request body")
}

func TestGPTAutoTrimNeverSplitsRunes(t *testing.T) {
	text := strings.Repeat("中文测试", 1000)
	trimmed := trimRunesKeepTail(text, 101)
	require.Equal(t, 101, len([]rune(trimmed)))
	require.True(t, strings.HasPrefix(trimmed, "中文"))
	require.True(t, strings.HasSuffix(trimmed, "测试"))
	require.Equal(t, "abc", trimRunesKeepTail("abc", 3))
	require.Equal(t, "ab", trimRunesKeepTail("abcdef", 2))
}

func TestGPTAutoTieBreakRequiresReportedProbabilities(t *testing.T) {
	weighted := func(c *GPTAutoShadowConfig) {
		for i := range c.ModelProfiles {
			c.ModelProfiles[i].RoutingWeight = 10
			if c.ModelProfiles[i].Model == GPTAutoModelGPT6Luna {
				c.ModelProfiles[i].RoutingWeight = 900
			}
		}
	}
	cls := *gptAutoSampleClassification(GPTAutoModelGPT56Sol, GPTAutoEffortMedium)
	cls.ModelProbabilities = nil
	decision := EvaluateGPTAutoPolicy(gptAutoPolicyInput(cls, weighted))
	require.Equal(t, GPTAutoModelGPT56Sol, decision.FinalModel, "missing probabilities are not a near-tie")
	require.Equal(t, GPTAutoDecisionAccepted, decision.Status)

	cls.ModelProbabilities = map[string]float64{GPTAutoModelGPT56Sol: 0.41, GPTAutoModelGPT6Luna: 0.40}
	decision = EvaluateGPTAutoPolicy(gptAutoPolicyInput(cls, weighted))
	require.Equal(t, GPTAutoModelGPT6Luna, decision.FinalModel)
}

func TestGPTAutoPolicyAllModelsAndEfforts(t *testing.T) {
	for _, model := range GPTAutoModelPool() {
		for _, effort := range GPTAutoEffortLadder() {
			cls := *gptAutoSampleClassification(model, effort)
			decision := EvaluateGPTAutoPolicy(gptAutoPolicyInput(cls, nil))
			require.Equal(t, model, decision.FinalModel, "%s/%s", model, effort)
			require.Contains(t, gptAutoNativeSupportedEfforts(model), decision.FinalEffort, "%s/%s", model, effort)
			require.Equal(t, effort, decision.JevEffort, "raw Jev effort is kept")
			if model == GPTAutoModelGPT61Sol && effort == GPTAutoEffortNone {
				require.Equal(t, GPTAutoEffortLow, decision.FinalEffort)
				require.Equal(t, GPTAutoDecisionAdjusted, decision.Status)
			} else {
				require.Equal(t, effort, decision.FinalEffort, "%s/%s", model, effort)
				require.Equal(t, GPTAutoDecisionAccepted, decision.Status)
			}
		}
	}

	// Endpoint restriction is a hard constraint that weight cannot override.
	cls := *gptAutoSampleClassification(GPTAutoModelGPT61Sol, GPTAutoEffortHigh)
	cls.ModelProbabilities = map[string]float64{GPTAutoModelGPT61Sol: 0.9, GPTAutoModelGPT56Luna: 0.1}
	input := gptAutoPolicyInput(cls, func(c *GPTAutoShadowConfig) {
		for i := range c.ModelProfiles {
			if c.ModelProfiles[i].Model == GPTAutoModelGPT61Sol {
				c.ModelProfiles[i].Endpoints = []string{GPTAutoEndpointResponses}
				c.ModelProfiles[i].RoutingWeight = 1000
			}
		}
	})
	input.Features.Endpoint = GPTAutoEndpointChatCompletions
	decision := EvaluateGPTAutoPolicy(input)
	require.Equal(t, GPTAutoModelGPT56Luna, decision.FinalModel)
	require.Contains(t, decision.CapabilityWarnings, GPTAutoRuleModelEndpoint)

	// Admin-narrowed efforts are honoured.
	input = gptAutoPolicyInput(*gptAutoSampleClassification(GPTAutoModelGPT6Luna, GPTAutoEffortMax), func(c *GPTAutoShadowConfig) {
		for i := range c.ModelProfiles {
			if c.ModelProfiles[i].Model == GPTAutoModelGPT6Luna {
				c.ModelProfiles[i].SupportedEfforts = []string{GPTAutoEffortLow, GPTAutoEffortHigh}
			}
		}
	})
	decision = EvaluateGPTAutoPolicy(input)
	require.Equal(t, GPTAutoEffortHigh, decision.FinalEffort, "lowered to the nearest supported level when nothing higher exists")
}

func TestGPTAutoHistoricalReplayKeepsOldPolicy(t *testing.T) {
	svc, repo, classifier, settings := newGPTAutoTestService(t, nil)
	svc.process(gptAutoTestTask(svc))
	require.Equal(t, 1, repo.count())

	cfg, err := settings.GetGPTAutoShadowConfig(context.Background())
	require.NoError(t, err)
	for i := range cfg.ModelProfiles {
		if cfg.ModelProfiles[i].Model == GPTAutoModelGPT6Luna {
			cfg.ModelProfiles[i].Enabled = false
		}
	}
	require.NoError(t, settings.SetGPTAutoShadowConfig(context.Background(), cfg))

	historical, err := svc.Replay(context.Background(), 1, GPTAutoReplayInput{Mode: "historical"})
	require.NoError(t, err)
	require.False(t, historical.Changed)
	current, err := svc.Replay(context.Background(), 1, GPTAutoReplayInput{Mode: "current"})
	require.NoError(t, err)
	require.True(t, current.Changed)
	again, err := svc.Replay(context.Background(), 1, GPTAutoReplayInput{Mode: "current"})
	require.NoError(t, err)
	require.Equal(t, current.Replayed.FinalModel, again.Replayed.FinalModel, "replay is deterministic")
	require.EqualValues(t, 1, classifier.calls.Load())
}

func TestGPTAutoDecisionCarriesActualUsage(t *testing.T) {
	svc, repo, _, _ := newGPTAutoTestService(t, nil)
	ctx := context.WithValue(context.Background(), ctxkey.ClientRequestID, "c0ffee")
	group := int64(7)
	svc.Observe(ctx, GPTAutoObserveInput{Endpoint: GPTAutoEndpointChatCompletions, Body: gptAutoTestBody, APIKeyID: 22, GroupID: &group})
	task := <-svc.queue
	require.Equal(t, "client:c0ffee", task.snapshot.RequestID, "decision id matches the usage_logs request id")
	svc.process(task)

	repo.actual["client:c0ffee"] = &GPTAutoActualUsage{Model: "gpt-5.6-sol", OutputTokens: 900, ChargedCost: 0.02, AccountCost: 0.012}
	row, err := svc.GetDecision(context.Background(), 1)
	require.NoError(t, err)
	require.NotNil(t, row.ActualUsage)
	require.Equal(t, 0.02, row.ActualUsage.ChargedCost)
	require.True(t, row.Decision.Economics.Hypothetical, "actual and hypothetical figures stay separate")
}

func TestGPTAutoStopCancelsInflightCallsWithoutLeaks(t *testing.T) {
	baseline := runtime.NumGoroutine()
	svc, repo, classifier, _ := newGPTAutoTestService(t, func(c *GPTAutoShadowConfig) { c.ClassifierTimeoutMS = 60000 })
	classifier.block = make(chan struct{})
	svc.Start()
	observeTestRequest(svc, gptAutoTestBody)
	require.Eventually(t, func() bool { return classifier.calls.Load() == 1 }, 5*time.Second, 10*time.Millisecond)

	started := time.Now()
	svc.Stop()
	require.Less(t, time.Since(started), 7*time.Second, "Stop must not wait for a hung provider")
	require.Equal(t, 1, repo.count(), "the cancelled call is still recorded")
	require.Eventually(t, func() bool { return runtime.NumGoroutine() <= baseline+2 }, 5*time.Second, 50*time.Millisecond,
		"workers and loops exit on Stop")
}

func TestGPTAutoCountersFlushToClusterTotals(t *testing.T) {
	shared := newLocalGPTAutoBudgetStore()
	a, _, _, _ := newGPTAutoTestService(t, func(c *GPTAutoShadowConfig) { c.SampleRate = 0 })
	b, _, _, _ := newGPTAutoTestService(t, func(c *GPTAutoShadowConfig) { c.SampleRate = 0 })
	a.budget, b.budget = shared, shared
	a.incr(gptAutoCounterDropped)
	a.incr(gptAutoCounterDropped)
	b.incr(gptAutoCounterDropped)
	a.flushCounters(context.Background())
	b.flushCounters(context.Background())
	a.flushCounters(context.Background()) // idempotent: only deltas are pushed

	status := a.RuntimeStatus(context.Background())
	require.EqualValues(t, 3, status.ClusterCountersToday[gptAutoCounterDropped])
	require.EqualValues(t, 2, status.Counters[gptAutoCounterDropped])
	require.Equal(t, "cluster", status.BudgetScope)
}

func TestGPTAutoObserveBodyIsNotRetained(t *testing.T) {
	svc, _, _, _ := newGPTAutoTestService(t, nil)
	body := append([]byte(nil), gptAutoTestBody...)
	observeTestRequest(svc, body)
	task := <-svc.queue
	latest := task.snapshot.LatestUser
	copy(body, bytes.Repeat([]byte("!"), len(body)))
	require.Equal(t, latest, task.snapshot.LatestUser)
}

func TestGPTAutoPolicySnapshotSelfHealsAfterTablePrune(t *testing.T) {
	svc, repo, _, _ := newGPTAutoTestService(t, nil)
	clock := time.Now()
	svc.now = func() time.Time { return clock }
	svc.process(gptAutoTestTask(svc))
	require.Len(t, repo.snapshots, 1)

	repo.mu.Lock()
	repo.snapshots = map[string][]byte{} // e.g. restored from an older backup
	repo.mu.Unlock()
	clock = clock.Add(gptAutoSnapshotRecheck + time.Minute)
	svc.process(gptAutoTestTask(svc))
	require.Len(t, repo.snapshots, 1, "the snapshot referenced by new decisions is written again")
	_, err := svc.Replay(context.Background(), 2, GPTAutoReplayInput{Mode: "historical"})
	require.NoError(t, err)
}
