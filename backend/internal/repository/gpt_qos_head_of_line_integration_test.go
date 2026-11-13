//go:build integration

package repository

import (
	"context"
	"fmt"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func TestGPTQoSRealRedisSameUserReadyCapacityBehindBlockedModel(t *testing.T) {
	for _, groupFull := range []bool{false, true} {
		t.Run(fmt.Sprintf("groupFull=%t", groupFull), func(t *testing.T) {
			ctx := context.Background()
			rdb := testRedis(t)
			cache, ok := NewConcurrencyCache(rdb, 15, 60).(*concurrencyCache)
			require.True(t, ok)
			acquired, err := cache.AcquireAccountSlot(ctx, 1, 1, "busy-model-a")
			require.NoError(t, err)
			require.True(t, acquired)
			for i := 0; i < 3; i++ {
				acquired, err = cache.AcquireAccountSlot(ctx, 1461, 12, fmt.Sprintf("existing-b-%d", i))
				require.NoError(t, err)
				require.True(t, acquired)
			}
			first := qosEntry("first-model-a", 42, "economy")
			second := qosEntry("second-model-b", 42, "economy")
			second.Candidates = []service.GPTQoSCandidate{{AccountID: 1461, SlotID: 1461, Concurrency: 12, PoolID: "account:1461", PoolCapacity: 12, Quality: "strong"}}
			if groupFull {
				holder := qosEntry("group-holder", 99, "economy")
				holder.GroupLimit = 1
				holder.Candidates = []service.GPTQoSCandidate{{AccountID: 3, SlotID: 3, Concurrency: 1, PoolID: "account:3", PoolCapacity: 1, Quality: "strong"}}
				grant, err := cache.GPTQoSPoll(ctx, holder)
				require.NoError(t, err)
				require.Equal(t, "granted", grant.Status)
				first.GroupLimit, second.GroupLimit = 1, 1
			}
			grant, err := cache.GPTQoSPoll(ctx, first)
			require.NoError(t, err)
			require.Equal(t, "waiting", grant.Status)
			grant, err = cache.GPTQoSPoll(ctx, second)
			require.NoError(t, err)
			if groupFull {
				require.Equal(t, "waiting", grant.Status, "lookahead cannot bypass the group's real capacity limit")
				require.NoError(t, cache.GPTQoSClose(ctx, "group-holder"))
				grant, err = cache.GPTQoSPoll(ctx, second)
				require.NoError(t, err)
			}
			require.Equal(t, "granted", grant.Status, "model A cannot hide model B's unused account slots for the same user")
			require.Equal(t, int64(1461), grant.Candidate.AccountID)
			require.EqualValues(t, 4, rdb.ZCard(ctx, "concurrency:account:1461").Val())
			require.EqualValues(t, 1, rdb.ZCard(ctx, "gptqos:{v1}:waiting").Val())
		})
	}
}

func TestGPTQoSRealRedisSameUserLookaheadBoundAndReadyFIFO(t *testing.T) {
	ctx := context.Background()
	rdb := testRedis(t)
	cache, ok := NewConcurrencyCache(rdb, 15, 60).(*concurrencyCache)
	require.True(t, ok)
	acquired, err := cache.AcquireAccountSlot(ctx, 1, 1, "busy-a")
	require.NoError(t, err)
	require.True(t, acquired)
	for i := 0; i < 8; i++ {
		grant, err := cache.GPTQoSPoll(ctx, qosEntry(fmt.Sprintf("blocked-%d", i), 42, "economy"))
		require.NoError(t, err)
		require.Equal(t, "waiting", grant.Status)
	}
	ready := qosEntry("ready-outside-prefix", 42, "economy")
	ready.Candidates = []service.GPTQoSCandidate{{AccountID: 2, SlotID: 2, Concurrency: 1, PoolID: "account:2", PoolCapacity: 1, Quality: "strong"}}
	grant, err := cache.GPTQoSPoll(ctx, ready)
	require.NoError(t, err)
	require.Equal(t, "waiting", grant.Status, "only the first eight requests are examined")
	require.NoError(t, cache.GPTQoSClose(ctx, "blocked-0"))
	grant, err = cache.GPTQoSPoll(ctx, ready)
	require.NoError(t, err)
	require.Equal(t, "granted", grant.Status)
	require.NoError(t, cache.GPTQoSClose(ctx, ready.ID))
	// Two waiting requests become ready together. Polling the younger one must
	// grant the older ready request, without charging a second user fair turn.
	acquired, err = cache.AcquireAccountSlot(ctx, 2, 1, "busy-b")
	require.NoError(t, err)
	require.True(t, acquired)
	ready.ID = "older-ready"
	grant, err = cache.GPTQoSPoll(ctx, ready)
	require.NoError(t, err)
	require.Equal(t, "waiting", grant.Status)
	require.NoError(t, cache.GPTQoSClose(ctx, "blocked-1"))
	younger := ready
	younger.ID = "younger-ready"
	grant, err = cache.GPTQoSPoll(ctx, younger)
	require.NoError(t, err)
	require.Equal(t, "waiting", grant.Status)
	require.NoError(t, cache.ReleaseAccountSlot(ctx, 2, "busy-b"))
	grant, err = cache.GPTQoSPoll(ctx, younger)
	require.NoError(t, err)
	require.Equal(t, "waiting", grant.Status)
	grant, err = cache.GPTQoSPoll(ctx, ready)
	require.NoError(t, err)
	require.Equal(t, "granted", grant.Status)
	require.NoError(t, cache.GPTQoSClose(ctx, ready.ID))
	grant, err = cache.GPTQoSPoll(ctx, younger)
	require.NoError(t, err)
	require.Equal(t, "granted", grant.Status)
}
