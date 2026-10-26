//go:build integration

package repository

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestGPTQoSAcceptanceStandbyOverflow(t *testing.T) {
	h := newAcceptanceHarness(t)
	status, raw := h.admin(0, http.MethodPut, fmt.Sprintf("/accounts/%d", h.strong[1]), map[string]any{"is_fallback": true})
	require.Equal(t, 200, status, string(raw))
	for _, id := range h.basic {
		status, raw = h.admin(0, http.MethodPost, fmt.Sprintf("/accounts/%d/schedulable", id), map[string]any{"schedulable": false})
		require.Equal(t, 200, status, string(raw))
	}
	snapshotCache := NewSchedulerCache(integrationRedis)
	require.Eventually(t, func() bool {
		account, err := snapshotCache.GetAccount(context.Background(), h.strong[1])
		if err != nil || account == nil || !account.IsFallback {
			return false
		}
		for _, id := range h.basic {
			account, err = snapshotCache.GetAccount(context.Background(), id)
			if err != nil || account != nil && account.Schedulable {
				return false
			}
		}
		return true
	}, 8*time.Second, 50*time.Millisecond, "scheduler metadata did not publish fixture account roles")
	cache := NewConcurrencyCache(integrationRedis, 1, 60).(*concurrencyCache)
	for _, qos := range []bool{false, true} {
		t.Run(fmt.Sprintf("qos=%t", qos), func(t *testing.T) {
			policy := h.policy
			policy.Enabled, policy.QualityRoutingEnabled = qos, qos
			h.save(policy)
			check := func(instance int, id string, want int64) {
				t.Helper()
				status, body, _ := h.request(context.Background(), instance, h.keys["premium"][0], id, true)
				require.Equal(t, 200, status, string(body))
				require.Contains(t, string(body), fmt.Sprintf("resp_%d_", want))
			}
			check(0, fmt.Sprintf("standby-idle-%t", qos), h.strong[0])
			release := func() {
				for i := 0; i < 10; i++ {
					_ = cache.ReleaseAccountSlot(context.Background(), h.strong[0], fmt.Sprintf("fixture-full-primary-%d", i))
				}
			}
			t.Cleanup(release)
			for i := 0; i < 10; i++ {
				ok, err := cache.AcquireAccountSlot(context.Background(), h.strong[0], 10, fmt.Sprintf("fixture-full-primary-%d", i))
				require.NoError(t, err)
				require.True(t, ok)
			}
			for instance := 0; instance < 2; instance++ {
				check(instance, fmt.Sprintf("standby-overflow-%t-%d", qos, instance), h.strong[1])
			}
			holdID := fmt.Sprintf("hold-standby-running-%t", qos)
			type result struct {
				status int
				body   []byte
			}
			done := make(chan result, 1)
			go func() {
				s, b, _ := h.request(context.Background(), 1, h.keys["premium"][1], holdID, true)
				done <- result{s, b}
			}()
			require.Eventually(t, func() bool {
				h.upstream.mu.Lock()
				defer h.upstream.mu.Unlock()
				for _, event := range h.upstream.events {
					if event.ID == holdID && event.Account == h.strong[1] && event.Ended.IsZero() {
						return true
					}
				}
				return false
			}, 3*time.Second, 10*time.Millisecond)
			release()
			for instance := 0; instance < 2; instance++ {
				check(instance, fmt.Sprintf("standby-primary-recovered-%t-%d", qos, instance), h.strong[0])
			}
			require.Len(t, done, 0, "a standby stream must still be running after primary recovery")
			r := <-done
			require.Equal(t, 200, r.status, string(r.body))
			require.Contains(t, string(r.body), fmt.Sprintf("resp_%d_", h.strong[1]))
			// Required response continuity remains on standby even with free primary slots.
			status, body, _ := h.requestBody(context.Background(), 0, h.keys["premium"][1], map[string]any{
				"model": "gpt-5.2", "input": fmt.Sprintf("standby-continuation-%t", qos), "stream": true,
				"previous_response_id": fmt.Sprintf("resp_%d_%s", h.strong[1], holdID),
			})
			require.Equal(t, 200, status, string(body))
			require.True(t, strings.Contains(string(body), fmt.Sprintf("resp_%d_", h.strong[1])))
			h.drained(t)
			h.artifact(fmt.Sprintf("standby-overflow-qos-%t", qos), map[string]any{
				"instances": 2, "qos": qos, "primary_account": h.strong[0], "standby_account": h.strong[1],
				"healthy_primary_slots_held": 10, "overflow_requests": 2, "recovered_primary_requests": 2,
				"running_standby_not_preempted": true, "required_continuation_retained": true,
			})
		})
	}
}
