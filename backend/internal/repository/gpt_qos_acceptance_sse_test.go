//go:build integration

package repository

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestGPTQoSAcceptanceSSEAfterContentNoReplay(t *testing.T) {
	h := newAcceptanceHarness(t)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	s, body, _ := h.request(ctx, 0, h.keys["premium"][0], "partial-drop", true)
	require.Equal(t, 200, s, string(body))
	require.Contains(t, string(body), "irreversible-fixture-token")
	h.drained(t)
	h.upstream.mu.Lock()
	attempts := 0
	for _, e := range h.upstream.events {
		if e.ID == "partial-drop" {
			attempts++
		}
	}
	h.upstream.mu.Unlock()
	require.Equal(t, 1, attempts, "cannot replay after semantic SSE output")
	// Let the existing asynchronous settlement worker flush; incomplete usage
	// may be estimated or absent according to the existing billing semantics.
	time.Sleep(time.Second) // bounded grace for the real asynchronous usage worker
	var rows, duplicates int
	require.Eventually(t, func() bool {
		err := integrationDB.QueryRow(`SELECT count(*),count(*)-count(DISTINCT request_id) FROM usage_logs`).Scan(&rows, &duplicates)
		return err == nil
	}, 2*time.Second, 20*time.Millisecond)
	require.LessOrEqual(t, rows, 1)
	require.Zero(t, duplicates)
	h.artifact("partial-sse", map[string]any{"http_status": s, "actual_upstream_attempts": attempts, "usage_rows_observed": rows, "duplicates": duplicates, "has_partial_content": true, "settlement_scope": "observed no duplicates; exact partial-token pricing remains baseline semantics"})
}
