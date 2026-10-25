//go:build integration

package repository

import (
	"context"
	"fmt"
	"sort"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func TestGPTQoSRealRedisAtomicAdmission(t *testing.T) {
	rdb := testRedis(t)
	c := NewConcurrencyCache(rdb, 15, 60).(*concurrencyCache)
	ctx := context.Background()
	var granted atomic.Int32
	var wg sync.WaitGroup
	for i := 0; i < 32; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			g, err := c.GPTQoSPoll(ctx, qosEntry(fmt.Sprintf("real-%d", i), int64(i+1), "premium"))
			if err != nil {
				t.Errorf("admit: %v", err)
				return
			}
			if g.Status == "granted" {
				granted.Add(1)
			}
		}(i)
	}
	wg.Wait()
	require.Equal(t, int32(1), granted.Load())
	require.Equal(t, int64(1), rdb.ZCard(ctx, "concurrency:account:1").Val())
	for i := 0; i < 32; i++ {
		require.NoError(t, c.GPTQoSClose(ctx, fmt.Sprintf("real-%d", i)))
	}
	require.Zero(t, rdb.ZCard(ctx, "concurrency:account:1").Val())
}
func TestGPTQualityAuditMigrationPersistsActorAndOldNew(t *testing.T) {
	tx := testEntTx(t)
	client := tx.Client()
	ctx := context.Background()
	a := mustCreateAccount(t, client, &service.Account{Name: "quality-audit", Platform: service.PlatformOpenAI, Extra: map[string]any{service.AccountQualityTierKey: "basic", "quality_tier_actor_id": 42}})
	_, err := tx.ExecContext(ctx, `UPDATE accounts SET extra=extra || '{"quality_tier":"strong","quality_tier_actor_id":43}'::jsonb WHERE id=$1`, a.ID)
	require.NoError(t, err)
	rows, err := tx.QueryContext(ctx, `SELECT actor_user_id,old_tier,new_tier FROM account_quality_audit WHERE account_id=$1 ORDER BY id`, a.ID)
	require.NoError(t, err)
	defer rows.Close()
	var actor int64
	var before, after string
	require.True(t, rows.Next())
	require.NoError(t, rows.Scan(&actor, &before, &after))
	require.Equal(t, int64(42), actor)
	require.Equal(t, "unknown", before)
	require.Equal(t, "basic", after)
	require.True(t, rows.Next())
	require.NoError(t, rows.Scan(&actor, &before, &after))
	require.Equal(t, int64(43), actor)
	require.Equal(t, "basic", before)
	require.Equal(t, "strong", after)
	require.False(t, rows.Next())
	require.NoError(t, rows.Err())
}

// An isolated local Redis latency sample, including acquire + release. It is
// evidence of admission overhead only, never a production/model load result.
func TestGPTQoSRealRedisLatencySample(t *testing.T) {
	c := NewConcurrencyCache(testRedis(t), 15, 60).(*concurrencyCache)
	ctx := context.Background()
	for _, qos := range []bool{false, true} {
		var durations []time.Duration
		for i := 0; i < 220; i++ {
			id := fmt.Sprintf("latency-%t-%d", qos, i)
			start := time.Now()
			if qos {
				grant, err := c.GPTQoSPoll(ctx, qosEntry(id, 42, "premium"))
				require.NoError(t, err)
				require.Equal(t, "granted", grant.Status)
				require.NoError(t, c.GPTQoSClose(ctx, id))
			} else {
				ok, err := c.AcquireAccountSlot(ctx, 1, 1, id)
				require.NoError(t, err)
				require.True(t, ok)
				require.NoError(t, c.ReleaseAccountSlot(ctx, 1, id))
			}
			if i >= 20 {
				durations = append(durations, time.Since(start))
			}
		}
		var total time.Duration
		for _, d := range durations {
			total += d
		}
		sort.Slice(durations, func(i, j int) bool { return durations[i] < durations[j] })
		t.Logf("qos=%t n=%d mean=%s p50=%s p95=%s sequential_pairs_per_second=%.1f", qos, len(durations), total/time.Duration(len(durations)), durations[99], durations[189], float64(len(durations))/total.Seconds())
	}
}
