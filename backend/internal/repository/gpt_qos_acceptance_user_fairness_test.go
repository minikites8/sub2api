//go:build integration

package repository

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func (h *acceptanceHarness) sameUserFairness(t *testing.T) {
	t.Run("SameTierUsersWithOneTenThreeAPIKeys", func(t *testing.T) {
		keys := make([][]string, 3)
		for user, keyCount := range []int{1, 10, 3} {
			primary := h.keys["premium"][user]
			keys[user] = []string{primary}
			var uid int64
			require.NoError(t, integrationDB.QueryRow(`SELECT user_id FROM api_keys WHERE key=$1`, primary).Scan(&uid))
			for k := 1; k < keyCount; k++ {
				key := fmt.Sprintf("sk-audit-extra-u%d-key%d-local-only", user, k)
				_, err := integrationEntClient.APIKey.Create().SetUserID(uid).SetGroupID(h.groups["premium"]).SetKey(key).SetName("audit-multiple-keys").SetStatus(service.StatusActive).Save(context.Background())
				require.NoError(t, err)
				keys[user] = append(keys[user], key)
			}
		}
		started := time.Now()
		end := started.Add(6 * time.Second)
		var wg sync.WaitGroup
		var mu sync.Mutex
		statuses := map[int]int{}
		ids := map[string]int{}
		for user := 0; user < 3; user++ {
			for worker := 0; worker < 18; worker++ {
				wg.Add(1)
				go func(user, worker int) {
					defer wg.Done()
					for n := 0; time.Now().Before(end); n++ {
						id := fmt.Sprintf("sameuser-%d-%d-%d", user, worker, n)
						s, _, _ := h.request(context.Background(), (n+worker)%2, keys[user][(n+worker)%len(keys[user])], id, false)
						mu.Lock()
						statuses[s]++
						ids[id] = user
						mu.Unlock()
					}
				}(user, worker)
			}
		}
		wg.Wait()
		h.drained(t)
		counts := map[int]int{}
		total := 0
		h.upstream.mu.Lock()
		for _, e := range h.upstream.events {
			user, ok := ids[e.ID]
			if ok && e.Started.After(started.Add(time.Second)) && e.Started.Before(started.Add(5*time.Second)) {
				counts[user]++
				total++
			}
		}
		h.upstream.mu.Unlock()
		h.artifact("same-user-fairness", map[string]any{"keys_per_user": []int{1, 10, 3}, "steady_admissions": counts, "total": total, "statuses": statuses, "steady_window_seconds": 4, "workers_per_user": 18})
		require.Greater(t, total, 150)
		for user := 0; user < 3; user++ {
			require.InDelta(t, 1.0/3, float64(counts[user])/float64(total), .1/3, "API key count must not increase user %d share: %v", user, counts)
		}
	})
}
