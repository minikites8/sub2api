//go:build integration

package repository

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"math/rand"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

type acceptanceResult struct {
	ID         string    `json:"id"`
	Tier       string    `json:"tier"`
	User       int       `json:"user"`
	Status     int       `json:"status"`
	Sent       time.Time `json:"sent"`
	DurationMS float64   `json:"duration_ms"`
	Canceled   bool      `json:"canceled"`
}

func (h *acceptanceHarness) admin(index int, method, path string, value any) (int, json.RawMessage) {
	data, _ := json.Marshal(value)
	req, _ := http.NewRequest(method, h.processes[index%len(h.processes)].URL+"/api/v1/admin"+path, bytes.NewReader(data))
	req.Header.Set("Authorization", "Bearer "+h.adminToken)
	req.Header.Set("Content-Type", "application/json")
	client := &http.Client{Timeout: 5 * time.Second}
	res, err := client.Do(req)
	require.NoError(h.t, err)
	defer res.Body.Close()
	body, _ := io.ReadAll(res.Body)
	var envelope struct {
		Data json.RawMessage `json:"data"`
	}
	_ = json.Unmarshal(body, &envelope)
	if res.StatusCode != 200 {
		return res.StatusCode, body
	}
	return res.StatusCode, envelope.Data
}
func (h *acceptanceHarness) save(policy service.GPTQoSConfig) {
	h.t.Helper()
	status, raw := h.admin(0, http.MethodPut, "/gpt-qos/config", policy)
	require.Equal(h.t, 200, status, string(raw))
	require.NoError(h.t, json.Unmarshal(raw, &h.policy))
	for i := range h.processes {
		require.Eventually(h.t, func() bool {
			status, raw := h.admin(i, http.MethodGet, "/gpt-qos/snapshot", nil)
			if status != 200 {
				return false
			}
			var v struct {
				Config service.GPTQoSConfig `json:"config"`
			}
			_ = json.Unmarshal(raw, &v)
			return v.Config.Version == h.policy.Version
		}, 10*time.Second, 100*time.Millisecond, "cross-process config did not refresh")
	}
}
func (h *acceptanceHarness) artifact(name string, value any) {
	data, err := json.MarshalIndent(value, "", "  ")
	require.NoError(h.t, err)
	require.NoError(h.t, os.WriteFile(filepath.Join(h.dir, name+".json"), data, 0600))
}
func (h *acceptanceHarness) drained(t *testing.T) {
	t.Helper()
	require.Eventually(t, func() bool {
		status, raw := h.admin(0, http.MethodGet, "/gpt-qos/snapshot", nil)
		var s struct {
			Capacity struct{ Active, Queued int } `json:"capacity"`
		}
		_ = json.Unmarshal(raw, &s)
		h.upstream.mu.Lock()
		n := h.upstream.active["strong"] + h.upstream.active["basic"]
		h.upstream.mu.Unlock()
		return status == 200 && s.Capacity.Active == 0 && s.Capacity.Queued == 0 && n == 0
	}, 8*time.Second, 50*time.Millisecond, "leaked active or queued requests")
}
func (h *acceptanceHarness) runLoad(t *testing.T, name string, seconds int, mixed bool) []acceptanceResult {
	t.Helper()
	var mu sync.Mutex
	results := []acceptanceResult{}
	var wg sync.WaitGroup
	var seq atomic.Int64
	stop := time.Now().Add(time.Duration(seconds) * time.Second)
	resourceStop := make(chan struct{})
	resourceDone := make(chan struct{})
	samples := []string{}
	go func() {
		defer close(resourceDone)
		ticker := time.NewTicker(500 * time.Millisecond)
		defer ticker.Stop()
		for {
			select {
			case <-resourceStop:
				return
			case <-ticker.C:
				ids := []string{}
				for _, p := range h.processes {
					ids = append(ids, strconv.Itoa(p.cmd.Process.Pid))
				}
				data, _ := exec.Command("/bin/ps", "-o", "pid=,%cpu=,rss=", "-p", strings.Join(ids, ",")).Output()
				samples = append(samples, fmt.Sprintf("%s %s", time.Now().Format(time.RFC3339Nano), data))
			}
		}
	}()
	var redisBefore map[string]string
	redisBefore, _ = integrationRedis.ConfigGet(context.Background(), "maxmemory").Result()
	infoBefore := integrationRedis.Info(context.Background(), "stats", "commandstats").Val()
	for ti, tier := range []string{"premium", "standard", "economy"} {
		for user := 0; user < 10; user++ {
			for worker := 0; worker < 4; worker++ {
				wg.Add(1)
				go func(tier string, ti, user, worker int) {
					defer wg.Done()
					rng := rand.New(rand.NewSource(int64(1337 + ti*1000 + user*10 + worker)))
					for time.Now().Before(stop) {
						n := seq.Add(1)
						id := fmt.Sprintf("%s-%s-u%d-%d", name, tier, user, n)
						if mixed && rng.Intn(7) == 0 {
							id += "-slow"
						}
						if mixed && rng.Intn(149) == 0 {
							id += "-fail429"
						}
						timeout := 8 * time.Second
						cancelled := mixed && rng.Intn(11) == 0
						if cancelled {
							timeout = 15 * time.Millisecond
						}
						ctx, cancel := context.WithTimeout(context.Background(), timeout)
						sent := time.Now()
						status, _, d := h.request(ctx, int(n)%2, h.keys[tier][user], id, n%2 == 0)
						cancel()
						mu.Lock()
						results = append(results, acceptanceResult{ID: id, Tier: tier, User: user, Status: status, Sent: sent, DurationMS: float64(d.Microseconds()) / 1000, Canceled: cancelled && status == 0})
						mu.Unlock()
					}
				}(tier, ti, user, worker)
			}
		}
	}
	wg.Wait()
	close(resourceStop)
	<-resourceDone
	h.artifact(name+"-resources", map[string]any{"ps_pid_cpu_percent_rss_kb": samples, "redis_config": redisBefore, "redis_before": infoBefore, "redis_after": integrationRedis.Info(context.Background(), "stats", "commandstats").Val(), "database_query_count": "NOT VERIFIED: pg_stat_statements is not enabled"})
	h.artifact(name+"-requests", results)
	h.drained(t)
	return results
}
func quantiles(v []float64) map[string]float64 {
	sort.Float64s(v)
	out := map[string]float64{}
	if len(v) == 0 {
		return out
	}
	for k, p := range map[string]float64{"p50": .5, "p95": .95, "p99": .99} {
		out[k] = v[int(float64(len(v)-1)*p)]
	}
	return out
}
func (h *acceptanceHarness) summarize(name string, results []acceptanceResult) map[string]any {
	h.upstream.mu.Lock()
	events := append([]*acceptanceEvent(nil), h.upstream.events...)
	maxima := map[string]int{}
	for k, v := range h.upstream.maximum {
		maxima[k] = v
	}
	h.upstream.mu.Unlock()
	byID := map[string]acceptanceResult{}
	summary := map[string]any{}
	counts := map[string]map[string]int{}
	waits := map[string][]float64{}
	elapsed := map[string][]float64{}
	for _, r := range results {
		byID[r.ID] = r
		if counts[r.Tier] == nil {
			counts[r.Tier] = map[string]int{}
		}
		counts[r.Tier]["requests"]++
		counts[r.Tier][fmt.Sprintf("http_%d", r.Status)]++
		elapsed[r.Tier] = append(elapsed[r.Tier], r.DurationMS)
	}
	for _, e := range events {
		r, ok := byID[e.ID]
		if !ok {
			continue
		}
		counts[r.Tier][e.Quality+"_attempts"]++
		counts[r.Tier]["attempts"]++
		if e.Status == 429 {
			counts[r.Tier]["upstream_429"]++
		}
		waits[r.Tier] = append(waits[r.Tier], float64(e.Started.Sub(r.Sent).Microseconds())/1000)
	}
	for tier, c := range counts {
		summary[tier] = map[string]any{"counts": c, "admission_wait_ms": quantiles(waits[tier]), "http_duration_ms": quantiles(elapsed[tier])}
	}
	summary["maximum_actual_upstream_concurrency"] = maxima
	h.artifact(name+"-summary", summary)
	return summary
}
func (h *acceptanceHarness) runAcceptance(t *testing.T) {
	t.Run("QualityStrongAndConfigAuthorization", func(t *testing.T) {
		status, _ := h.admin(0, http.MethodGet, "/gpt-qos/config", nil)
		require.Equal(t, 200, status)
		for n := 0; n < 40; n++ {
			status, body, _ := h.request(context.Background(), n%2, h.keys["economy"][n%10], fmt.Sprintf("strong-priority-%d", n), n%2 == 0)
			require.Equal(t, 200, status, string(body))
		}
		h.upstream.mu.Lock()
		for _, e := range h.upstream.events {
			if strings.HasPrefix(e.ID, "strong-priority-") {
				require.Equal(t, "strong", e.Quality)
			}
		}
		h.upstream.mu.Unlock()
		req, _ := http.NewRequest(http.MethodPut, h.processes[0].URL+"/api/v1/admin/gpt-qos/config", strings.NewReader(`{"enabled":false}`))
		req.Header.Set("Authorization", "Bearer "+h.keys["economy"][0])
		res, err := http.DefaultClient.Do(req)
		require.NoError(t, err)
		res.Body.Close()
		require.NotEqual(t, 200, res.StatusCode)
		invalid := h.policy
		invalid.Peak.ProtectionRatio = 2
		status, _ = h.admin(0, http.MethodPut, "/gpt-qos/config", invalid)
		require.Equal(t, 400, status)
	})
	h.qualityScenarios(t)
	t.Run("WeightedSharesSaturated", func(t *testing.T) {
		policy := h.policy
		for id, g := range policy.Groups {
			g.AllowBasicFallback = false
			g.StrongWaitMs = 15000
			policy.Groups[id] = g
		}
		h.save(policy)
		results := h.runLoad(t, "weighted", 60, false)
		summary := h.summarize("weighted", results)
		t.Logf("actual two-process weighted summary: %v", summary)
		byID := map[string]acceptanceResult{}
		first := time.Now()
		for _, r := range results {
			byID[r.ID] = r
			if r.Sent.Before(first) {
				first = r.Sent
			}
		}
		start, end := first.Add(5*time.Second), first.Add(59*time.Second)
		counts := map[string]int{}
		total := 0
		h.upstream.mu.Lock()
		for _, e := range h.upstream.events {
			r, ok := byID[e.ID]
			if ok && e.Started.After(start) && e.Started.Before(end) {
				counts[r.Tier]++
				total++
				require.Equal(t, "strong", e.Quality)
			}
		}
		h.upstream.mu.Unlock()
		h.artifact("weighted-steady-shares", map[string]any{"counts": counts, "total": total, "window_seconds": 54, "method": "upstream admission starts, identical 40ms service; 5s warmup; deterministic workload"})
		require.Greater(t, total, 4000)
		for tier, w := range map[string]float64{"premium": .6, "standard": .3, "economy": .1} {
			require.InDelta(t, w, float64(counts[tier])/float64(total), w*.10, "tier %s actual grants %v", tier, counts)
		}
	})
	h.sameUserFairness(t)
	t.Run("IntegratedPeakFallbackCancellation429", func(t *testing.T) {
		policy := h.policy
		policy.CapacityEnabled = true
		policy.Peak.Enabled = true
		policy.Peak.EnterHoldMs = 100
		policy.Peak.ExitHoldMs = 200
		for id, g := range policy.Groups {
			g.StrongWaitMs = 3000
			g.AllowBasicFallback = g.Tier == "economy"
			if g.Tier == "economy" {
				g.StrongWaitMs = 100
				g.BasicWaitMs = 1000
			}
			policy.Groups[id] = g
		}
		h.save(policy)
		results := h.runLoad(t, "mixed", 8, true)
		summary := h.summarize("mixed", results)
		t.Logf("actual two-process mixed summary: %v", summary)
		// Runtime 429/cooldown must refresh the registry even with an unchanged
		// policy version. Compare against the authoritative disposable database.
		require.Eventually(t, func() bool {
			_, _ = h.admin(0, http.MethodGet, "/gpt-qos/snapshot", nil)
			rows, err := integrationEntClient.Account.Query().All(context.Background())
			if err != nil {
				return false
			}
			expected := 0
			for _, row := range rows {
				a := &service.Account{Status: row.Status, Schedulable: row.Schedulable, Concurrency: row.Concurrency, ExpiresAt: row.ExpiresAt, AutoPauseOnExpired: row.AutoPauseOnExpired, RateLimitResetAt: row.RateLimitResetAt, OverloadUntil: row.OverloadUntil, TempUnschedulableUntil: row.TempUnschedulableUntil, Extra: row.Extra}
				if a.QualityTier() == "strong" && a.IsSchedulable() {
					expected += a.Concurrency
				}
			}
			actual, err := integrationRedis.HGet(context.Background(), "gptqos:{v1}:capacity_policy", "strong_capacity").Int()
			return err == nil && actual == expected
		}, 9*time.Second, 100*time.Millisecond, "effective Strong capacity did not follow runtime cooldown")
		h.upstream.mu.Lock()
		defer h.upstream.mu.Unlock()
		require.LessOrEqual(t, h.upstream.maximum["strong"], 20)
		require.LessOrEqual(t, h.upstream.maximum["basic"], 30)
		for _, e := range h.upstream.events {
			if strings.HasPrefix(e.ID, "mixed-premium-") {
				require.Equal(t, "strong", e.Quality, "Premium quality violation")
			}
		}
	})
	t.Run("StrictReserveAndNoPreemption", func(t *testing.T) {
		h.recoverFixtureAccounts(t)
		// Observe aggregate reserve during actual overlapping, cancellable HTTP calls.
		var wg sync.WaitGroup
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		for n := 0; n < 40; n++ {
			wg.Add(1)
			go func(n int) {
				defer wg.Done()
				h.request(ctx, n%2, h.keys["standard"][n%10], fmt.Sprintf("reserve-hold-%d", n), true)
			}(n)
		}
		require.Eventually(t, func() bool {
			status, raw := h.admin(0, http.MethodGet, "/gpt-qos/snapshot", nil)
			var s struct {
				Capacity struct {
					StrongPool struct {
						Peak                       bool
						Capacity, Reserved, Active int
					} `json:"strong_pool"`
				} `json:"capacity"`
			}
			_ = json.Unmarshal(raw, &s)
			return status == 200 && s.Capacity.StrongPool.Peak && s.Capacity.StrongPool.Reserved == 6
		}, 5*time.Second, 50*time.Millisecond)
		status, raw := h.admin(1, http.MethodGet, "/gpt-qos/snapshot", nil)
		require.Equal(t, 200, status)
		h.artifact("peak-active-snapshot", json.RawMessage(raw))
		h.upstream.mu.Lock()
		existing := h.upstream.active["strong"]
		h.upstream.mu.Unlock()
		require.LessOrEqual(t, existing, 20)
		// Running requests are allowed to drain. Premium must use the protected headroom.
		status, body, _ := h.request(context.Background(), 1, h.keys["premium"][0], "protected-premium", true)
		require.Equal(t, 200, status, string(body))
		cancel()
		wg.Wait()
		h.drained(t)
	})
	h.additionalScenarios(t)
	t.Run("AllFlagsOffRollbackAndBilling", func(t *testing.T) {
		off := service.DefaultGPTQoSConfig()
		h.save(off)
		for n := 0; n < 20; n++ {
			status, body, _ := h.request(context.Background(), n%2, h.keys["premium"][0], fmt.Sprintf("rollback-%d", n), n%2 == 0)
			require.Equal(t, 200, status, string(body))
		}
		h.drained(t)
		status, raw := h.admin(0, http.MethodGet, "/gpt-qos/snapshot", nil)
		require.Equal(t, 200, status)
		h.artifact("flags-off-snapshot", json.RawMessage(raw))
		var count, duplicates, badTokens, badMultiplier int
		require.NoError(t, integrationDB.QueryRow(`SELECT count(*),count(*)-count(DISTINCT request_id),count(*) FILTER(WHERE input_tokens<>100 OR output_tokens<>50),count(*) FILTER(WHERE rate_multiplier<>1) FROM usage_logs`).Scan(&count, &duplicates, &badTokens, &badMultiplier))
		h.artifact("billing-invariants", map[string]int{"usage_rows": count, "duplicate_request_ids": duplicates, "invalid_tokens": badTokens, "changed_rate_multipliers": badMultiplier})
		require.Greater(t, count, 20)
		require.Zero(t, duplicates)
		require.Zero(t, badTokens)
		require.Zero(t, badMultiplier)
	})
	// Keep the report explicit: this records measured results, not a performance claim.
	h.artifact("fixture", map[string]any{"instances": 2, "strong_capacity": 20, "basic_capacity": 30, "weights": []int{6, 3, 1}, "seed": 1337, "peak_ratio": .3, "binary": h.binary, "groups": h.groups, "configured_pool_count": len(h.strong) + len(h.basic), "max_attempts": h.policy.MaxAttempts, "rounding_20_slots": int(math.Floor(20 * .3)), "group_id_format": strconv.FormatInt(h.groups["premium"], 10)})
	h.baselineScenario(t)
}
