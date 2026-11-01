//go:build integration

package repository

import (
	"context"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
)

// Two OS processes, one real Redis/PostgreSQL and a loopback SSE upstream.
// The account IDs are observed at that upstream, not inferred from scores.
func TestGPTQoSAcceptanceConversationAffinity(t *testing.T) {
	h := newAcceptanceHarness(t)
	ctx := context.Background()
	cache := NewConcurrencyCache(integrationRedis, 1, 60).(*concurrencyCache)
	for _, queue := range []bool{false, true} {
		t.Run(fmt.Sprintf("queue=%t", queue), func(t *testing.T) {
			policy := h.policy
			policy.Enabled, policy.QualityRoutingEnabled = queue, true
			h.save(policy)
			session := fmt.Sprintf("same-conversation-%t", queue)
			request := func(turn int) int64 {
				t.Helper()
				id := fmt.Sprintf("cache-affinity-%t-%d", queue, turn)
				status, body, _ := h.requestBody(ctx, turn%2, h.keys["premium"][0], map[string]any{
					"model": "gpt-5.2", "input": id, "stream": true, "prompt_cache_key": session,
				})
				require.Equal(t, 200, status, string(body))
				h.upstream.mu.Lock()
				defer h.upstream.mu.Unlock()
				var observed []int64
				for _, event := range h.upstream.events {
					if event.ID == id {
						observed = append(observed, event.Account)
						require.Equal(t, "strong", event.Quality)
					}
				}
				require.Len(t, observed, 1, "one user request must not be replayed for affinity")
				return observed[0]
			}
			owner := request(0)
			release := func() {
				for i := 0; i < 10; i++ {
					_ = cache.ReleaseAccountSlot(ctx, owner, fmt.Sprintf("affinity-held-%t-%d", queue, i))
				}
			}
			t.Cleanup(release)
			occupy := func(from, to int) {
				for i := from; i < to; i++ {
					ok, err := cache.AcquireAccountSlot(ctx, owner, 10, fmt.Sprintf("affinity-held-%t-%d", queue, i))
					require.NoError(t, err)
					require.True(t, ok)
				}
			}
			occupy(0, 5)
			for turn := 1; turn <= 12; turn++ {
				require.Equal(t, owner, request(turn), "a half-loaded owner should retain its conversation despite an idle peer")
			}
			occupy(5, 10)
			peer := request(13)
			require.NotEqual(t, owner, peer, "full soft owner must yield to qualified capacity")
			require.EqualValues(t, 10, integrationRedis.ZCard(ctx, fmt.Sprintf("concurrency:account:%d", owner)).Val())
			release()
			for turn := 14; turn <= 21; turn++ {
				require.Equal(t, peer, request(turn), "after a safe move, cache locality follows the newly bound account")
			}
			h.drained(t)
			h.artifact(fmt.Sprintf("conversation-affinity-%t", queue), map[string]any{
				"instances": 2, "fair_queue": queue, "observed_upstream_requests": 22,
				"initial_owner": owner, "same_owner_turns_with_50_percent_load": 12,
				"overflow_owner": peer, "rebound_owner_turns": 8, "resources_drained": true,
			})
		})
	}
}
