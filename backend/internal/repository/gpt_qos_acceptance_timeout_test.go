//go:build integration

package repository

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestGPTQoSAcceptanceTimeoutDiagnostics(t *testing.T) {
	h := newAcceptanceHarness(t)
	policy := h.policy
	for id, g := range policy.Groups {
		g.StrongWaitMs = 80
		g.BasicWaitMs = 1000
		policy.Groups[id] = g
	}
	h.save(policy)
	cache := NewConcurrencyCache(integrationRedis, 1, 60).(*concurrencyCache)
	release := func() {
		for _, id := range h.strong {
			for slot := 0; slot < 10; slot++ {
				_ = cache.ReleaseAccountSlot(context.Background(), id, fmt.Sprintf("busy-%d-%d", id, slot))
			}
		}
	}
	t.Cleanup(release)
	for _, id := range h.strong {
		for slot := 0; slot < 10; slot++ {
			ok, err := cache.AcquireAccountSlot(context.Background(), id, 10, fmt.Sprintf("busy-%d-%d", id, slot))
			require.NoError(t, err)
			require.True(t, ok)
		}
	}
	// The second real service continuously reads state, reproducing the other
	// instance/dashboard cleanup that used to replace expiry with "closed".
	stop := make(chan struct{})
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		ticker := time.NewTicker(5 * time.Millisecond)
		defer ticker.Stop()
		for {
			select {
			case <-stop:
				return
			case <-ticker.C:
				_, _ = h.admin(1, http.MethodGet, "/gpt-qos/snapshot", nil)
			}
		}
	}()
	status, body, elapsed := h.request(context.Background(), 0, h.keys["premium"][0], "timeout-premium-denied", true)
	close(stop)
	wg.Wait()
	require.Equal(t, 503, status, string(body))
	require.Contains(t, string(body), "gpt_qos_queue_timeout")
	require.NotContains(t, string(body), "gpt_qos_queue_closed")
	require.NotContains(t, string(body), "expired or was closed")
	h.upstream.mu.Lock()
	require.Empty(t, h.upstream.events, "quality guarantee rejection must not forward or charge")
	h.upstream.mu.Unlock()
	var billed int
	require.NoError(t, integrationDB.QueryRow(`SELECT count(*) FROM usage_logs`).Scan(&billed))
	require.Zero(t, billed)
	// Explicitly allowed Basic fallback still works; the timeout fix must not
	// turn a Premium guarantee failure into a downgrade or a reset/requeue loop.
	econStatus, econBody, _ := h.request(context.Background(), 1, h.keys["economy"][0], "timeout-economy-fallback", false)
	require.Equal(t, 200, econStatus, string(econBody))
	h.upstream.mu.Lock()
	require.Len(t, h.upstream.events, 1)
	require.Equal(t, "basic", h.upstream.events[0].Quality)
	h.upstream.mu.Unlock()
	release()
	restoredStatus, restoredBody, _ := h.request(context.Background(), 0, h.keys["premium"][0], "timeout-premium-restored", true)
	require.Equal(t, 200, restoredStatus, string(restoredBody))
	require.True(t, strings.Contains(string(restoredBody), "fixture-ok"))
	h.drained(t)
	require.Eventually(t, func() bool {
		err := integrationDB.QueryRow(`SELECT count(*) FROM usage_logs`).Scan(&billed)
		return err == nil && billed == 2
	}, 5*time.Second, 50*time.Millisecond)
	h.artifact("timeout-diagnostics", map[string]any{"instances": 2, "strong_wait_ms": 80, "premium_full_status": status, "premium_full_error_code": "gpt_qos_queue_timeout", "premium_full_elapsed_ms": elapsed.Milliseconds(), "economy_fallback_status": econStatus, "premium_restored_status": restoredStatus, "usage_rows": billed})
}
