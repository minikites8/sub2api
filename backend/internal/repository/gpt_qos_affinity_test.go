package repository

import (
	"context"
	"fmt"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
)

func TestGPTQoSCacheAffinityAcrossInstances(t *testing.T) {
	c, m := qosTestCache(t)
	r2 := redis.NewClient(&redis.Options{Addr: m.Addr()})
	t.Cleanup(func() { _ = r2.Close() })
	c2 := NewConcurrencyCache(r2, 15, 60).(*concurrencyCache)
	ctx := context.Background()
	e := qosEntry("", 42, "standard")
	e.Candidates = []service.GPTQoSCandidate{
		{AccountID: 1, SlotID: 1, Concurrency: 2, PoolID: "account:1", PoolCapacity: 2, Quality: "strong"},
		{AccountID: 2, SlotID: 2, Concurrency: 2, PoolID: "account:2", PoolCapacity: 2, Quality: "strong", CacheAffinity: 1},
	}
	ok, err := c.AcquireAccountSlot(ctx, 2, 2, "busy-owner")
	require.NoError(t, err)
	require.True(t, ok)
	for turn := 0; turn < 24; turn++ {
		e.ID = fmt.Sprintf("cache-turn-%d", turn)
		owner := c
		if turn%2 != 0 {
			owner = c2
		}
		grant, err := owner.GPTQoSPoll(ctx, e)
		require.NoError(t, err)
		require.Equal(t, "granted", grant.Status)
		require.Equal(t, int64(2), grant.Candidate.AccountID, "live load must not rotate an available conversation owner")
		require.NoError(t, owner.GPTQoSClose(ctx, e.ID))
	}
	// A second live slot fills the owner; affinity is not a reservation and
	// must not block an idle eligible peer or oversell the owner.
	ok, err = c.AcquireAccountSlot(ctx, 2, 2, "full-owner")
	require.NoError(t, err)
	require.True(t, ok)
	e.ID = "cache-overflow"
	g, err := c2.GPTQoSPoll(ctx, e)
	require.NoError(t, err)
	require.Equal(t, "granted", g.Status)
	require.Equal(t, int64(1), g.Candidate.AccountID)
	require.EqualValues(t, 2, c.rdb.ZCard(ctx, "concurrency:account:2").Val())
	require.NoError(t, c2.GPTQoSClose(ctx, e.ID))
	for _, id := range []string{"busy-owner", "full-owner"} {
		require.NoError(t, c.ReleaseAccountSlot(ctx, 2, id))
	}
	state, err := c.GPTQoSState(ctx)
	require.NoError(t, err)
	require.EqualValues(t, 0, state["active"])
	require.EqualValues(t, 0, state["queued"])
}

func TestGPTQoSCacheAffinityKeepsQualityRoleAndCapacityConstraints(t *testing.T) {
	for _, tc := range []string{"basic", "standby", "shared-full", "protected"} {
		t.Run(tc, func(t *testing.T) {
			c, _ := qosTestCache(t)
			ctx := context.Background()
			e := qosEntry("cache-constraint", 42, "standard")
			a := service.GPTQoSCandidate{AccountID: 1, SlotID: 1, Concurrency: 10, PoolID: "account:1", PoolCapacity: 10, Quality: "strong", CacheAffinity: 1}
			b := service.GPTQoSCandidate{AccountID: 2, SlotID: 2, Concurrency: 10, PoolID: "account:2", PoolCapacity: 10, Quality: "strong"}
			switch tc {
			case "basic":
				a.Quality = "basic"
			case "standby":
				a.IsFallback = true
			case "shared-full":
				a.PoolID, a.PoolCapacity = "shared:fixture", 1
				holder := qosEntry("shared-holder", 99, "premium")
				holder.Candidates = []service.GPTQoSCandidate{{AccountID: 3, SlotID: 3, Concurrency: 10, PoolID: a.PoolID, PoolCapacity: 1, Quality: "strong"}}
				g, err := c.GPTQoSPoll(ctx, holder)
				require.NoError(t, err)
				require.Equal(t, "granted", g.Status)
				t.Cleanup(func() { require.NoError(t, c.GPTQoSClose(ctx, holder.ID)) })
			case "protected":
				e.Peak.Enabled, e.Peak.StrictReserve = true, true
				e.Peak.EnterHoldMs = 0
				e.Peak.EnterUtilization = 0.7
				for i := 0; i < 7; i++ {
					ok, err := c.AcquireAccountSlot(ctx, 1, 10, fmt.Sprintf("occupancy-%d", i))
					require.NoError(t, err)
					require.True(t, ok)
				}
			}
			e.Candidates = []service.GPTQoSCandidate{a, b}
			g, err := c.GPTQoSPoll(ctx, e)
			require.NoError(t, err)
			require.Equal(t, "granted", g.Status)
			require.Equal(t, int64(2), g.Candidate.AccountID)
			require.NoError(t, c.GPTQoSClose(ctx, e.ID))
		})
	}
}
