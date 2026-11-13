//go:build integration

package repository

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
)

func TestGPTQoSRealRedisStrictReserveDoesNotProtectIndependentBasic(t *testing.T) {
	for _, legacyPeak := range []bool{false, true} {
		t.Run(fmt.Sprintf("cachedPeak=%t", legacyPeak), func(t *testing.T) {
			ctx := context.Background()
			rdb := testRedis(t)
			cache, ok := NewConcurrencyCache(rdb, 15, 60).(*concurrencyCache)
			require.True(t, ok)
			now, err := rdb.Time(ctx).Result()
			require.NoError(t, err)
			require.NoError(t, rdb.HSet(ctx, "gptqos:{v1}:capacity_policy", "strong_capacity", 100).Err())
			peakRaw, err := json.Marshal(map[string]any{"peak": true, "since": 0, "updated": now.UnixMilli()})
			require.NoError(t, err)
			require.NoError(t, rdb.Set(ctx, "gptqos:{v1}:global_peak", peakRaw, 0).Err())
			for i := 0; i < 80; i++ {
				require.NoError(t, rdb.ZAdd(ctx, "gptqos:{v1}:all_strong", redis.Z{Score: float64(now.Add(time.Minute).UnixMilli()), Member: fmt.Sprintf("global-%d", i)}).Err())
			}
			if legacyPeak {
				require.NoError(t, rdb.HSet(ctx, "gptqos:{v1}:pools", "account:1461", peakRaw).Err())
				require.NoError(t, rdb.HSet(ctx, "gptqos:{v1}:strong_limits", "account:1461", 12).Err())
			}
			for i := 0; i < 9; i++ {
				acquired, err := cache.AcquireAccountSlot(ctx, 1461, 12, fmt.Sprintf("basic-active-%d", i))
				require.NoError(t, err)
				require.True(t, acquired)
			}
			e := qosEntry("configured-basic-fallback", 42, "economy")
			e.Peak.Enabled, e.Peak.StrictReserve = true, true
			e.StrongWaitMs, e.WaitMs = 100, 1100
			e.Candidates = []service.GPTQoSCandidate{{AccountID: 1, SlotID: 1, Concurrency: 12, PoolID: "account:1", PoolCapacity: 12, Quality: "strong"}, {AccountID: 1461, SlotID: 1461, Concurrency: 12, PoolID: "account:1461", PoolCapacity: 12, Quality: "basic"}}
			grant, err := cache.GPTQoSPoll(ctx, e)
			require.NoError(t, err)
			require.Equal(t, "waiting", grant.Status)
			time.Sleep(120 * time.Millisecond)
			grant, err = cache.GPTQoSPoll(ctx, e)
			require.NoError(t, err)
			require.Equal(t, "granted", grant.Status, "independent Basic can use its own idle slots despite global Strong reservation")
			require.Equal(t, int64(1461), grant.Candidate.AccountID)
			require.Equal(t, "basic", grant.Candidate.Quality)
			require.NoError(t, cache.GPTQoSClose(ctx, e.ID))
			// A genuinely shared physical pool must still preserve its Strong
			// reserve even when the selected binding is marked Basic.
			require.NoError(t, rdb.HSet(ctx, "gptqos:{v1}:pools", "shared:physical", peakRaw).Err())
			require.NoError(t, rdb.HSet(ctx, "gptqos:{v1}:strong_limits", "shared:physical", 12).Err())
			for i := 0; i < 9; i++ {
				require.NoError(t, rdb.ZAdd(ctx, "gptqos:{v1}:pool:shared:physical", redis.Z{Score: float64(now.Add(time.Minute).UnixMilli()), Member: fmt.Sprintf("physical-active-%d", i)}).Err())
			}
			e.ID = "shared-basic-fallback"
			e.StrongWaitMs = 0
			e.Candidates = []service.GPTQoSCandidate{{AccountID: 1461, SlotID: 1461, Concurrency: 12, PoolID: "shared:physical", PoolCapacity: 12, Quality: "basic"}}
			grant, err = cache.GPTQoSPoll(ctx, e)
			require.NoError(t, err)
			require.Equal(t, "waiting", grant.Status, "Basic quality cannot bypass the same upstream's physical Strong reserve")
		})
	}
}
