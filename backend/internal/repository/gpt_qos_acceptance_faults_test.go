//go:build integration

package repository

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

// Faults deliberately affect real cached account state; restore through the
// production admin API before independent scenarios (never weaken their checks).
func (h *acceptanceHarness) recoverFixtureAccounts(t *testing.T) {
	t.Helper()
	for _, id := range append(append([]int64{}, h.strong...), h.basic...) {
		status, raw := h.admin(0, http.MethodPost, fmt.Sprintf("/accounts/%d/recover-state", id), nil)
		require.Equal(t, 200, status, string(raw))
	}
	h.save(h.policy)
	require.Eventually(t, func() bool {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		s, _, _ := h.request(ctx, 0, h.keys["premium"][0], "fixture-recovered", false)
		return s == 200
	}, 10*time.Second, 100*time.Millisecond)
}

func (h *acceptanceHarness) additionalScenarios(t *testing.T) {
	t.Run("RealRedisDisconnectFailsClosedAndRecovers", func(t *testing.T) {
		h.drained(t)
		h.redisProxy.block(true)
		var wg sync.WaitGroup
		var mu sync.Mutex
		statuses := map[int]int{}
		for i := 0; i < 20; i++ {
			wg.Add(1)
			go func(i int) {
				defer wg.Done()
				ctx, cancel := context.WithTimeout(context.Background(), 600*time.Millisecond)
				defer cancel()
				status, _, _ := h.request(ctx, i%2, h.keys["premium"][i%10], fmt.Sprintf("redis-disconnect-%d", i), true)
				mu.Lock()
				statuses[status]++
				mu.Unlock()
			}(i)
		}
		wg.Wait()
		h.redisProxy.block(false)
		h.artifact("redis-disconnect", statuses)
		h.upstream.mu.Lock()
		for _, e := range h.upstream.events {
			require.False(t, strings.HasPrefix(e.ID, "redis-disconnect-"), "Redis failure bypassed atomic slot")
		}
		h.upstream.mu.Unlock()
		require.Eventually(t, func() bool {
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer cancel()
			s, _, _ := h.request(ctx, 0, h.keys["premium"][0], "redis-recovered", false)
			return s == 200
		}, 8*time.Second, 200*time.Millisecond)
		h.drained(t)
	})
	t.Run("SharedKeysUseOnePhysicalCapacity", func(t *testing.T) {
		h.drained(t)
		policy := h.policy
		policy.Peak.Enabled = false
		policy.CapacityEnabled = true
		policy.ResourcePools["audit-shared"] = service.GPTQoSResourcePool{Concurrency: 10}
		h.save(policy)
		for _, id := range h.strong {
			s, raw := h.admin(0, http.MethodPut, fmt.Sprintf("/accounts/%d", id), map[string]any{"qos_resource_pool": "audit-shared"})
			require.Equal(t, 200, s, string(raw))
		}
		// Account cache updates are delivered through the production outbox.
		require.Eventually(t, func() bool {
			return integrationRedis.HGet(context.Background(), "gptqos:{v1}:capacity_policy", "strong_capacity").Val() == "10"
		}, 8*time.Second, 100*time.Millisecond)
		h.upstream.mu.Lock()
		h.upstream.maximum = map[string]int{}
		h.upstream.mu.Unlock()
		results := h.runLoad(t, "shared", 3, false)
		h.summarize("shared", results)
		h.upstream.mu.Lock()
		maximum := h.upstream.maximum["strong"]
		h.upstream.mu.Unlock()
		require.Greater(t, maximum, 0)
		require.LessOrEqual(t, maximum, 10, "two API keys must not duplicate physical capacity")
		for _, id := range h.strong {
			s, raw := h.admin(0, http.MethodPut, fmt.Sprintf("/accounts/%d", id), map[string]any{"qos_resource_pool": ""})
			require.Equal(t, 200, s, string(raw))
		}
		h.drained(t)
	})
	t.Run("RealPreviousResponseBindingAcrossInstances", func(t *testing.T) {
		status, body, _ := h.request(context.Background(), 0, h.keys["premium"][0], "continuation-parent", false)
		require.Equal(t, 200, status, string(body))
		var previous struct {
			ID string `json:"id"`
		}
		require.NoError(t, json.Unmarshal(body, &previous))
		require.NotEmpty(t, previous.ID)
		reqBody := map[string]any{"model": "gpt-5.2", "input": "continuation-child", "previous_response_id": previous.ID, "stream": false}
		status, body, _ = h.requestBody(context.Background(), 1, h.keys["premium"][0], reqBody)
		require.Equal(t, 200, status, string(body))
		h.upstream.mu.Lock()
		var owner, child int64
		for _, e := range h.upstream.events {
			if e.ID == "continuation-parent" {
				owner = e.Account
			}
			if e.ID == "continuation-child" {
				child = e.Account
			}
		}
		h.upstream.mu.Unlock()
		require.NotZero(t, owner)
		require.Equal(t, owner, child)
		h.drained(t)
	})
}
func (h *acceptanceHarness) baselineScenario(t *testing.T) {
	t.Run("BaselineBinaryFlagsOffDifferential", func(t *testing.T) {
		baseline := os.Getenv("SUB2API_ACCEPTANCE_BASELINE_BINARY")
		if baseline == "" {
			t.Skip("baseline binary not supplied; differential NOT VERIFIED")
		}
		require.FileExists(t, baseline)
		h.recoverFixtureAccounts(t)
		h.save(service.DefaultGPTQoSConfig())
		// Deterministic old-scheduler preference: the idle Basic account must win
		// with all quality and QoS switches off, in both actual binaries.
		for _, id := range append(append([]int64{}, h.strong...), h.basic...) {
			priority := 100
			if id == h.basic[0] {
				priority = -100
			}
			status, raw := h.admin(0, http.MethodPut, fmt.Sprintf("/accounts/%d", id), map[string]any{"priority": priority})
			require.Equal(t, 200, status, string(raw))
		}
		// Wait for the actual distributed snapshot outbox, not only the SQL write.
		require.Eventually(t, func() bool {
			status, body, _ := h.request(context.Background(), 0, h.keys["premium"][0], "baseline-warm", false)
			return status == 200 && strings.Contains(string(body), fmt.Sprintf("resp_%d_", h.basic[0]))
		}, 10*time.Second, 100*time.Millisecond)
		h.drained(t)
		run := func(label string) map[string]any {
			statuses := map[int]int{}
			counts := map[int64]int{}
			latencies := []float64{}
			var mu sync.Mutex
			var wg sync.WaitGroup
			for worker := 0; worker < 10; worker++ {
				wg.Add(1)
				go func(worker int) {
					defer wg.Done()
					for n := 0; n < 20; n++ {
						id := fmt.Sprintf("%s-%d-%d", label, worker, n)
						s, _, d := h.request(context.Background(), n%2, h.keys["premium"][worker], id, n%2 == 0)
						mu.Lock()
						statuses[s]++
						latencies = append(latencies, float64(d.Microseconds())/1000)
						mu.Unlock()
					}
				}(worker)
			}
			wg.Wait()
			h.upstream.mu.Lock()
			for _, e := range h.upstream.events {
				if strings.HasPrefix(e.ID, label+"-") {
					counts[e.Account]++
				}
			}
			h.upstream.mu.Unlock()
			require.Equal(t, map[int]int{200: 200}, statuses)
			require.Equal(t, map[int64]int{h.basic[0]: 200}, counts, "flags off must preserve unique legacy Basic priority")
			return map[string]any{"statuses": statuses, "account_selections": counts, "latency_ms": quantiles(latencies)}
		}
		current := run("current-off")
		h.billingSnapshot("billing-before-restart")
		time.Sleep(time.Second) // drain asynchronous settlement before intentionally replacing processes
		h.billingSnapshot("billing-before-restart-drained")
		for _, p := range h.processes {
			_ = p.cmd.Process.Kill()
			_ = p.cmd.Wait()
		}
		h.processes = nil
		old := h.binary
		h.binary = baseline
		for i := 0; i < 2; i++ {
			h.processes = append(h.processes, h.start(2+i))
		}
		h.binary = old
		base := run("baseline-off")
		h.artifact("baseline-differential", map[string]any{"baseline_commit": "21f675138", "current": current, "baseline": base, "scope": "200 requests per actual binary; fixed unique Basic priority; 10 workers; Responses stream/non-stream"})
		h.billingSnapshot("billing-after-baseline")
		var residual float64
		require.Eventually(t, func() bool {
			err := integrationDB.QueryRow(`SELECT coalesce(max(abs(balance-(1000-coalesce(charged,0)))),0) FROM users u LEFT JOIN (SELECT user_id,sum(actual_cost) charged FROM usage_logs GROUP BY user_id) l ON l.user_id=u.id`).Scan(&residual)
			return err == nil && residual < 0.000001
		}, 5*time.Second, 50*time.Millisecond, "balance and persisted usage charges diverged")
		rows, err := integrationDB.Query(`SELECT a.extra->>'quality_tier',count(*),min(l.actual_cost),max(l.actual_cost),sum(l.actual_cost) FROM usage_logs l JOIN accounts a ON a.id=l.account_id GROUP BY 1`)
		require.NoError(t, err)
		defer rows.Close()
		costs := []map[string]any{}
		for rows.Next() {
			var quality string
			var count int
			var low, high, total float64
			require.NoError(t, rows.Scan(&quality, &count, &low, &high, &total))
			require.InDelta(t, low, high, .000000001, "identical tokens and sales multiplier must not acquire a quality surcharge")
			costs = append(costs, map[string]any{"quality": quality, "rows": count, "min_cost": low, "max_cost": high, "sum_cost": total})
		}
		require.NoError(t, rows.Err())
		h.artifact("billing-reconciliation", map[string]any{"max_balance_residual": residual, "by_quality": costs})
	})
}

func (h *acceptanceHarness) billingSnapshot(name string) {
	rows, err := integrationDB.Query(`SELECT u.id,u.role,u.balance,u.gift_balance,u.frozen_balance,coalesce(l.charged,0),coalesce(l.n,0),1000-u.balance-coalesce(l.charged,0) FROM users u LEFT JOIN (SELECT user_id,sum(actual_cost) charged,count(*) n FROM usage_logs GROUP BY user_id) l ON l.user_id=u.id ORDER BY u.id`)
	require.NoError(h.t, err)
	defer rows.Close()
	out := []map[string]any{}
	for rows.Next() {
		var id, n int64
		var role string
		var balance, gift, frozen, cost, residual float64
		require.NoError(h.t, rows.Scan(&id, &role, &balance, &gift, &frozen, &cost, &n, &residual))
		out = append(out, map[string]any{"id": id, "role": role, "balance": balance, "gift": gift, "frozen": frozen, "usage_cost": cost, "usage_rows": n, "residual": residual})
	}
	require.NoError(h.t, rows.Err())
	h.artifact(name, out)
}
