//go:build integration

package repository

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestGPTQoSAcceptanceProcessCrashLeaseRecovery(t *testing.T) {
	h := newAcceptanceHarness(t)
	var wg sync.WaitGroup
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	for n := 0; n < 20; n++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			h.request(ctx, 0, h.keys["premium"][n%10], fmt.Sprintf("crash-hold-%d", n), true)
		}(n)
	}
	require.Eventually(t, func() bool {
		h.upstream.mu.Lock()
		defer h.upstream.mu.Unlock()
		return h.upstream.active["strong"] == 20
	}, 5*time.Second, 10*time.Millisecond)
	died := time.Now()
	require.NoError(t, h.processes[0].cmd.Process.Kill())
	_ = h.processes[0].cmd.Wait()
	wg.Wait()
	// The other independent instance must conservatively retain unexpired leases.
	s, _, _ := h.request(context.Background(), 1, h.keys["premium"][0], "crash-before-expiry", false)
	require.Equal(t, 503, s, "do not recycle leases before their owner TTL expires")
	require.Eventually(t, func() bool {
		c, cancel := context.WithTimeout(context.Background(), 1500*time.Millisecond)
		defer cancel()
		s, _, _ := h.request(c, 1, h.keys["premium"][0], "crash-after-expiry", true)
		return s == 200
	}, 70*time.Second, 200*time.Millisecond)
	deadPID := h.processes[0].cmd.Process.Pid
	h.processes = []*acceptanceProcess{h.processes[1]}
	h.drained(t)
	state := integrationRedis.ZCard(context.Background(), "gptqos:{v1}:leases").Val()
	require.Zero(t, state)
	h.artifact("process-crash", map[string]any{"ttl_seconds": 60, "recovery_ms": time.Since(died).Milliseconds(), "post_recovery_leases": state, "process_killed": deadPID, "surviving_process": h.processes[0].cmd.Process.Pid})
}
