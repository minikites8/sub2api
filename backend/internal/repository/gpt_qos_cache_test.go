package repository

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
)

func qosTestCache(t testing.TB) (*concurrencyCache, *miniredis.Miniredis) {
	t.Helper()
	m := miniredis.RunT(t)
	r := redis.NewClient(&redis.Options{Addr: m.Addr()})
	t.Cleanup(func() { _ = r.Close() })
	return NewConcurrencyCache(r, 15, 60).(*concurrencyCache), m
}
func qosEntry(id string, uid int64, tier string) service.GPTQoSEntry {
	g := service.DefaultGPTQoSGroupConfig(tier)
	cfg := service.DefaultGPTQoSConfig()
	return service.GPTQoSEntry{ID: id, UserID: uid, GroupID: 1, Tier: tier, Weight: g.Weight, MaxWaiting: 2000, GroupMaxWaiting: 2000, UserMaxWaiting: 100, WaitMs: 120000, StrongWaitMs: 0, Protected: g.CapacityProtection, Weights: [3]int{6, 3, 1}, Peak: cfg.Peak, Candidates: []service.GPTQoSCandidate{{AccountID: 1, SlotID: 1, Concurrency: 1, PoolID: "account:1", PoolCapacity: 1, Quality: "strong"}}}
}

func TestGPTQoSChoosesLowerLiveLoadWithinQualityPool(t *testing.T) {
	for _, tc := range []struct {
		name   string
		busyID int64
		wantID int64
	}{
		{"first candidate busy second idle", 1, 2},
		{"second candidate busy first idle", 2, 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c, _ := qosTestCache(t)
			ctx := context.Background()
			ok, err := c.AcquireAccountSlot(ctx, tc.busyID, 10, "existing")
			require.NoError(t, err)
			require.True(t, ok)
			e := qosEntry("load-choice", 42, "standard")
			e.Candidates = []service.GPTQoSCandidate{
				{AccountID: 1, SlotID: 1, Concurrency: 10, PoolID: "account:1", PoolCapacity: 10, Quality: "strong"},
				{AccountID: 2, SlotID: 2, Concurrency: 10, PoolID: "account:2", PoolCapacity: 10, Quality: "strong"},
			}
			g, err := c.GPTQoSPoll(ctx, e)
			require.NoError(t, err)
			require.Equal(t, "granted", g.Status)
			require.Equal(t, tc.wantID, g.Candidate.AccountID, "an available first candidate must not hide a less-loaded peer")
			require.NoError(t, c.GPTQoSClose(ctx, e.ID))
			require.NoError(t, c.ReleaseAccountSlot(ctx, tc.busyID, "existing"))
		})
	}
}

func TestGPTQoSEqualLoadRotatesPhysicalPoolsAcrossInstances(t *testing.T) {
	c, m := qosTestCache(t)
	r2 := redis.NewClient(&redis.Options{Addr: m.Addr()})
	defer r2.Close()
	c2 := NewConcurrencyCache(r2, 15, 60).(*concurrencyCache)
	ctx := context.Background()
	counts := map[string]int{}
	for i := 0; i < 60; i++ {
		e := qosEntry(fmt.Sprintf("serial-%d", i), 42, "standard")
		// Two API keys can share one physical root. They must not earn two
		// idle-pool turns against one independent OAuth account.
		e.Candidates = []service.GPTQoSCandidate{
			{AccountID: 1, SlotID: 1, Concurrency: 10, PoolID: "shared:fixture", PoolCapacity: 10, Quality: "strong"},
			{AccountID: 2, SlotID: 2, Concurrency: 10, PoolID: "shared:fixture", PoolCapacity: 10, Quality: "strong"},
			{AccountID: 3, SlotID: 3, Concurrency: 3, PoolID: "account:3", PoolCapacity: 3, Quality: "strong"},
		}
		owner := c
		if i%2 != 0 {
			owner = c2
		}
		g, err := owner.GPTQoSPoll(ctx, e)
		require.NoError(t, err)
		require.Equal(t, "granted", g.Status)
		counts[g.Candidate.PoolID]++
		require.NoError(t, owner.GPTQoSClose(ctx, e.ID))
	}
	require.Equal(t, 30, counts["shared:fixture"])
	require.Equal(t, 30, counts["account:3"], "serial short requests must not all return to the first pool")
}

func TestGPTQoSLoadDoesNotOverrideQuality(t *testing.T) {
	c, _ := qosTestCache(t)
	ctx := context.Background()
	ok, err := c.AcquireAccountSlot(ctx, 1, 10, "strong-in-use")
	require.NoError(t, err)
	require.True(t, ok)
	e := qosEntry("quality-before-load", 42, "economy")
	e.Candidates = []service.GPTQoSCandidate{
		{AccountID: 2, SlotID: 2, Concurrency: 10, PoolID: "account:2", PoolCapacity: 10, Quality: "basic"},
		{AccountID: 1, SlotID: 1, Concurrency: 10, PoolID: "account:1", PoolCapacity: 10, Quality: "strong"},
	}
	g, err := c.GPTQoSPoll(ctx, e)
	require.NoError(t, err)
	require.Equal(t, int64(1), g.Candidate.AccountID)
	require.NoError(t, c.GPTQoSClose(ctx, e.ID))
	require.NoError(t, c.ReleaseAccountSlot(ctx, 1, "strong-in-use"))
}

func TestGPTQoSComparesNormalizedAccountAndSharedPoolLoad(t *testing.T) {
	c, _ := qosTestCache(t)
	ctx := context.Background()
	cfg := service.DefaultGPTQoSConfig()
	cfg.Version, cfg.CapacityEnabled = 1, true
	cfg.ResourcePools["fixture"] = service.GPTQoSResourcePool{Concurrency: 10}
	accounts := []service.Account{}
	for _, id := range []int64{1, 2} {
		accounts = append(accounts, service.Account{ID: id, Concurrency: 10, Platform: service.PlatformOpenAI, Status: service.StatusActive, Schedulable: true, Extra: map[string]any{service.AccountQualityTierKey: "strong", service.AccountResourcePoolKey: "fixture"}})
	}
	require.NoError(t, c.GPTQoSConfigureCapacity(ctx, cfg, accounts))
	for i := 0; i < 8; i++ {
		ok, err := c.AcquireAccountSlot(ctx, 2, 10, fmt.Sprintf("shared-busy-%d", i))
		require.NoError(t, err)
		require.True(t, ok)
	}
	ok, err := c.AcquireAccountSlot(ctx, 3, 3, "independent-busy")
	require.NoError(t, err)
	require.True(t, ok)
	e := qosEntry("shared-load", 42, "premium")
	e.Candidates = []service.GPTQoSCandidate{
		{AccountID: 1, SlotID: 1, Concurrency: 10, PoolID: "shared:fixture", PoolCapacity: 10, Quality: "strong"},
		{AccountID: 3, SlotID: 3, Concurrency: 3, PoolID: "account:3", PoolCapacity: 3, Quality: "strong"},
	}
	g, err := c.GPTQoSPoll(ctx, e)
	require.NoError(t, err)
	require.Equal(t, int64(3), g.Candidate.AccountID, "an idle API key must still reflect its busy shared physical root")
	require.NoError(t, c.GPTQoSClose(ctx, e.ID))
	for i := 0; i < 8; i++ {
		require.NoError(t, c.ReleaseAccountSlot(ctx, 2, fmt.Sprintf("shared-busy-%d", i)))
	}
	require.NoError(t, c.ReleaseAccountSlot(ctx, 3, "independent-busy"))

	for _, id := range []string{"wide-1", "wide-2"} {
		ok, err := c.AcquireAccountSlot(ctx, 4, 10, id)
		require.NoError(t, err)
		require.True(t, ok)
	}
	ok, err = c.AcquireAccountSlot(ctx, 5, 2, "narrow-1")
	require.NoError(t, err)
	require.True(t, ok)
	e = qosEntry("normalized-load", 42, "standard")
	e.Candidates = []service.GPTQoSCandidate{
		{AccountID: 5, SlotID: 5, Concurrency: 2, PoolID: "account:5", PoolCapacity: 2, Quality: "strong"},
		{AccountID: 4, SlotID: 4, Concurrency: 10, PoolID: "account:4", PoolCapacity: 10, Quality: "strong"},
	}
	g, err = c.GPTQoSPoll(ctx, e)
	require.NoError(t, err)
	require.Equal(t, int64(4), g.Candidate.AccountID, "2/10 load is lower than 1/2; raw active counts do not measure saturation")
	require.NoError(t, c.GPTQoSClose(ctx, e.ID))
	for _, id := range []string{"wide-1", "wide-2"} {
		require.NoError(t, c.ReleaseAccountSlot(ctx, 4, id))
	}
	require.NoError(t, c.ReleaseAccountSlot(ctx, 5, "narrow-1"))
}
func TestGPTQoSAtomicAdmissionNoOversellAcrossInstances(t *testing.T) {
	c, m := qosTestCache(t)
	r2 := redis.NewClient(&redis.Options{Addr: m.Addr()})
	defer r2.Close()
	c2 := NewConcurrencyCache(r2, 15, 60).(*concurrencyCache)
	var wg sync.WaitGroup
	var mu sync.Mutex
	granted := map[string]bool{}
	for i := 0; i < 40; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			cache := c
			if i%2 == 1 {
				cache = c2
			}
			e := qosEntry(fmt.Sprint(i), int64(i+1), "standard")
			g, err := cache.GPTQoSPoll(context.Background(), e)
			mu.Lock()
			defer mu.Unlock()
			require.NoError(t, err)
			if g.Status == "granted" {
				granted[e.ID] = true
			}
		}(i)
	}
	wg.Wait()
	require.Len(t, granted, 1)
	require.Equal(t, int64(1), c.rdb.ZCard(context.Background(), "concurrency:account:1").Val())
	for id := range granted {
		require.NoError(t, c2.GPTQoSClose(context.Background(), id))
		require.NoError(t, c2.GPTQoSClose(context.Background(), id))
	}
	require.Zero(t, c.rdb.ZCard(context.Background(), "concurrency:account:1").Val())
}
func TestGPTQoSHierarchicalWeightedSharesAndUserFairness(t *testing.T) {
	c, _ := qosTestCache(t)
	ctx := context.Background()
	ok, err := c.AcquireAccountSlot(ctx, 1, 1, "busy")
	require.NoError(t, err)
	require.True(t, ok)
	entries := []service.GPTQoSEntry{}
	for _, tier := range []string{"premium", "standard", "economy"} {
		for user := 0; user < 2; user++ {
			for i := 0; i < 80; i++ {
				e := qosEntry(fmt.Sprintf("%s-%d-%d", tier, user, i), int64(1+user+len(entries)/160*10), tier)
				entries = append(entries, e)
				g, err := c.GPTQoSPoll(ctx, e)
				require.NoError(t, err)
				require.Equal(t, "waiting", g.Status)
			}
		}
	}
	require.NoError(t, c.ReleaseAccountSlot(ctx, 1, "busy"))
	counts := map[string]int{}
	users := map[int64]int{}
	for grants := 0; grants < 100; {
		progress := false
		for _, e := range entries {
			g, err := c.GPTQoSPoll(ctx, e)
			require.NoError(t, err)
			if g.Status == "granted" {
				counts[e.Tier]++
				users[e.UserID]++
				grants++
				require.NoError(t, c.GPTQoSClose(ctx, e.ID))
				progress = true
				break
			}
		}
		require.True(t, progress, "queue must make progress")
	}
	t.Logf("100 grants: tiers=%v users=%v", counts, users)
	require.InDelta(t, 60, counts["premium"], 2)
	require.InDelta(t, 30, counts["standard"], 2)
	require.InDelta(t, 10, counts["economy"], 2)
	require.Equal(t, users[1], users[2])
	require.Equal(t, users[11], users[12])
	require.Equal(t, users[21], users[22])
}

func TestGPTQoSPeakReserveHysteresisAndSmallPools(t *testing.T) {
	c, m := qosTestCache(t)
	ctx := context.Background()
	now := time.Now()
	m.SetTime(now)
	entry := func(id string, uid int64, tier string) service.GPTQoSEntry {
		e := qosEntry(id, uid, tier)
		e.Candidates[0].Concurrency = 20
		e.Candidates[0].PoolCapacity = 20
		e.Peak.Enabled = true
		e.Peak.EnterHoldMs = 100
		e.Peak.ExitHoldMs = 200
		return e
	}
	for i := 0; i < 17; i++ {
		g, err := c.GPTQoSPoll(ctx, entry(fmt.Sprintf("load-%d", i), int64(i+1), "standard"))
		require.NoError(t, err)
		require.Equal(t, "granted", g.Status)
	}
	// The 17th arrival starts the entry window; no running SSE is preempted.
	m.SetTime(now.Add(150 * time.Millisecond))
	g, err := c.GPTQoSPoll(ctx, entry("protected-low", 100, "economy"))
	require.NoError(t, err)
	require.Equal(t, "waiting", g.Status)
	g, err = c.GPTQoSPoll(ctx, entry("premium", 101, "premium"))
	require.NoError(t, err)
	require.Equal(t, "granted", g.Status)
	require.True(t, g.Peak)
	require.Equal(t, 6, g.Reserved)
	// Enough releases start the lower exit threshold's stable window.
	for i := 0; i < 8; i++ {
		require.NoError(t, c.GPTQoSClose(ctx, fmt.Sprintf("load-%d", i)))
	}
	_, err = c.GPTQoSPoll(ctx, entry("exit-observe", 102, "premium"))
	require.NoError(t, err)
	m.SetTime(now.Add(400 * time.Millisecond))
	g, err = c.GPTQoSPoll(ctx, entry("low-after-exit", 103, "economy"))
	require.NoError(t, err)
	// Another pending user can be granted first; poll until this request owns its grant.
	for i := 0; i < 5 && g.Status != "granted"; i++ {
		g, err = c.GPTQoSPoll(ctx, entry("low-after-exit", 103, "economy"))
		require.NoError(t, err)
	}
	require.Equal(t, "granted", g.Status)
	require.False(t, g.Peak)
	// A one-slot pool reserves zero slots: proportional rounding cannot strand it.
	small := qosEntry("small", 500, "economy")
	small.Candidates[0].AccountID = 9
	small.Candidates[0].SlotID = 9
	small.Candidates[0].PoolID = "account:9"
	small.Peak.Enabled = true
	g, err = c.GPTQoSPoll(ctx, small)
	require.NoError(t, err)
	require.Equal(t, "granted", g.Status)
	require.Zero(t, g.Reserved)
}

func TestGPTQoSLegacyOccupancyAndLeaseRenewal(t *testing.T) {
	c, _ := qosTestCache(t)
	ctx := context.Background()
	ok, err := c.AcquireAccountSlot(ctx, 1, 1, "legacy")
	require.NoError(t, err)
	require.True(t, ok)
	e := qosEntry("new", 1, "premium")
	g, err := c.GPTQoSPoll(ctx, e)
	require.NoError(t, err)
	require.Equal(t, "waiting", g.Status)
	require.NoError(t, c.ReleaseAccountSlot(ctx, 1, "legacy"))
	g, err = c.GPTQoSPoll(ctx, e)
	require.NoError(t, err)
	require.Equal(t, "granted", g.Status)
	ok, err = c.GPTQoSRefresh(ctx, e.ID)
	require.NoError(t, err)
	require.True(t, ok)
	require.NoError(t, c.GPTQoSClose(ctx, e.ID))
	ok, err = c.GPTQoSRefresh(ctx, e.ID)
	require.NoError(t, err)
	require.False(t, ok)
	require.Zero(t, c.rdb.ZCard(ctx, "concurrency:account:1").Val())
}
func TestGPTQoSCancelTimeoutAndLatePollFence(t *testing.T) {
	c, m := qosTestCache(t)
	ctx := context.Background()
	_, err := c.AcquireAccountSlot(ctx, 1, 1, "busy")
	require.NoError(t, err)
	e := qosEntry("cancel", 1, "standard")
	g, err := c.GPTQoSPoll(ctx, e)
	require.NoError(t, err)
	require.Equal(t, "waiting", g.Status)
	require.NoError(t, c.GPTQoSClose(ctx, e.ID))
	g, err = c.GPTQoSPoll(ctx, e)
	require.NoError(t, err)
	require.Equal(t, "closed", g.Status)
	e.ID = "expire"
	e.WaitMs = 100
	g, err = c.GPTQoSPoll(ctx, e)
	require.NoError(t, err)
	require.Equal(t, "waiting", g.Status)
	m.SetTime(time.Now().Add(time.Second))
	g, err = c.GPTQoSPoll(ctx, e)
	require.NoError(t, err)
	require.Equal(t, "closed", g.Status)
	require.Zero(t, c.rdb.ZCard(ctx, "gptqos:{v1}:waiting").Val())
	require.Equal(t, int64(1), c.rdb.ZCard(ctx, "concurrency:account:1").Val())
}
func TestGPTQoSUserBackpressureAcrossAPIKeys(t *testing.T) {
	c, _ := qosTestCache(t)
	ctx := context.Background()
	_, err := c.AcquireAccountSlot(ctx, 1, 1, "busy")
	require.NoError(t, err)
	e := qosEntry("key-one", 7, "standard")
	e.UserMaxWaiting = 1
	g, err := c.GPTQoSPoll(ctx, e)
	require.NoError(t, err)
	require.Equal(t, "waiting", g.Status)
	e.ID = "key-two"
	g, err = c.GPTQoSPoll(ctx, e)
	require.NoError(t, err)
	require.Equal(t, "full", g.Status)
	e.ID = "other-user"
	e.UserID = 8
	g, err = c.GPTQoSPoll(ctx, e)
	require.NoError(t, err)
	require.Equal(t, "waiting", g.Status)
}
func TestGPTQoSSharedPoolCapacityNotSummed(t *testing.T) {
	c, _ := qosTestCache(t)
	ctx := context.Background()
	for i := 1; i <= 4; i++ {
		e := qosEntry(fmt.Sprint(i), int64(i), "standard")
		e.Candidates[0].AccountID = int64(i)
		e.Candidates[0].SlotID = int64(i)
		e.Candidates[0].PoolID = "shared:vm"
		e.Candidates[0].PoolCapacity = 2
		g, err := c.GPTQoSPoll(ctx, e)
		require.NoError(t, err)
		if i <= 2 {
			require.Equal(t, "granted", g.Status)
		} else {
			require.Equal(t, "waiting", g.Status)
		}
	}
	require.Equal(t, int64(2), c.rdb.ZCard(ctx, "gptqos:{v1}:pool:shared:vm").Val())
}
func TestGPTQoSRedisFailureFailsClosed(t *testing.T) {
	c, m := qosTestCache(t)
	m.SetError("simulated outage")
	_, err := c.GPTQoSPoll(context.Background(), qosEntry("outage", 1, "premium"))
	require.Error(t, err)
}

func TestGPTQoSSharedCapacityIndependentOfQualityRouting(t *testing.T) {
	c, _ := qosTestCache(t)
	ctx := context.Background()
	cfg := service.DefaultGPTQoSConfig()
	cfg.CapacityEnabled = true
	cfg.Version = 1
	cfg.ResourcePools["vm"] = service.GPTQoSResourcePool{Concurrency: 2}
	accounts := []service.Account{{ID: 1, Status: service.StatusActive, Schedulable: true, Concurrency: 3, Extra: map[string]any{service.AccountResourcePoolKey: "vm"}}, {ID: 2, Status: service.StatusActive, Schedulable: true, Concurrency: 3, Extra: map[string]any{service.AccountResourcePoolKey: "vm"}}}
	// An existing request must be counted when the new feature is enabled.
	ok, err := c.AcquireAccountSlot(ctx, 1, 3, "old")
	require.NoError(t, err)
	require.True(t, ok)
	require.NoError(t, c.GPTQoSConfigureCapacity(ctx, cfg, accounts))
	ok, err = c.AcquireAccountSlot(ctx, 2, 3, "new")
	require.NoError(t, err)
	require.True(t, ok)
	ok, err = c.AcquireAccountSlot(ctx, 2, 3, "overflow")
	require.NoError(t, err)
	require.False(t, ok, "two API keys share one two-slot resource")
	require.NoError(t, c.ReleaseAccountSlot(ctx, 1, "old"))
	ok, err = c.AcquireAccountSlot(ctx, 2, 3, "overflow")
	require.NoError(t, err)
	require.True(t, ok)
	cfg.CapacityEnabled = false
	cfg.Version = 2
	require.NoError(t, c.GPTQoSConfigureCapacity(ctx, cfg, nil))
	ok, err = c.AcquireAccountSlot(ctx, 1, 3, "rollback")
	require.NoError(t, err)
	require.True(t, ok, "flag off restores account-local admission")
	for _, slot := range []struct {
		id  int64
		rid string
	}{{2, "new"}, {2, "overflow"}, {1, "rollback"}} {
		require.NoError(t, c.ReleaseAccountSlot(ctx, slot.id, slot.rid))
	}
	require.Zero(t, c.rdb.ZCard(ctx, "gptqos:{v1}:pool:shared:vm").Val())
}

func TestGPTQoSMixedSharedPoolCannotConsumePremiumReserve(t *testing.T) {
	c, m := qosTestCache(t)
	ctx := context.Background()
	now := time.Now()
	m.SetTime(now)
	cfg := service.DefaultGPTQoSConfig()
	cfg.Version = 1
	cfg.CapacityEnabled, cfg.Peak.Enabled = true, true
	cfg.ResourcePools["vm"] = service.GPTQoSResourcePool{Concurrency: 10}
	accounts := []service.Account{
		{ID: 1, Status: service.StatusActive, Schedulable: true, Concurrency: 10, Extra: map[string]any{service.AccountResourcePoolKey: "vm", service.AccountQualityTierKey: "strong"}},
		{ID: 2, Status: service.StatusActive, Schedulable: true, Concurrency: 10, Extra: map[string]any{service.AccountResourcePoolKey: "vm", service.AccountQualityTierKey: "basic"}},
	}
	require.NoError(t, c.GPTQoSConfigureCapacity(ctx, cfg, accounts))
	entry := func(id string, uid int64, tier, quality string) service.GPTQoSEntry {
		e := qosEntry(id, uid, tier)
		e.Peak = cfg.Peak
		e.Peak.EnterHoldMs = 100
		e.Candidates[0].PoolID, e.Candidates[0].PoolCapacity, e.Candidates[0].Concurrency = "shared:vm", 10, 10
		e.Candidates[0].Quality = quality
		if quality == "basic" {
			e.Candidates[0].AccountID, e.Candidates[0].SlotID = 2, 2
		}
		return e
	}
	for i := 0; i < 9; i++ {
		g, err := c.GPTQoSPoll(ctx, entry(fmt.Sprint(i), int64(i+1), "standard", "strong"))
		require.NoError(t, err)
		require.Equal(t, "granted", g.Status)
	}
	m.SetTime(now.Add(150 * time.Millisecond))
	g, err := c.GPTQoSPoll(ctx, entry("basic-overflow", 100, "economy", "basic"))
	require.NoError(t, err)
	require.Equal(t, "waiting", g.Status, "a Basic API key shares physical Premium reserve")
	// Legacy non-QoS admission must obey the same physical reserve.
	ok, err := c.AcquireAccountSlot(ctx, 2, 10, "legacy-basic")
	require.NoError(t, err)
	require.False(t, ok)
	g, err = c.GPTQoSPoll(ctx, entry("protected-premium", 101, "premium", "strong"))
	require.NoError(t, err)
	require.Equal(t, "granted", g.Status)
	require.Equal(t, 3, g.Reserved)
	require.Equal(t, int64(10), c.rdb.ZCard(ctx, "gptqos:{v1}:pool:shared:vm").Val())
}

func TestGPTQoSSingleProxyAndLegacyShareSemaphore(t *testing.T) {
	c, _ := qosTestCache(t)
	ctx := context.Background()
	proxy := int64(9)
	a := service.Account{ID: 31, ProxyID: &proxy, Status: service.StatusActive, Schedulable: true, Concurrency: 1, Extra: map[string]any{service.AccountResourcePoolKey: "vm", service.AccountQualityTierKey: "strong"}}
	cfg := service.DefaultGPTQoSConfig()
	cfg.Version, cfg.CapacityEnabled = 1, true
	cfg.ResourcePools["vm"] = service.GPTQoSResourcePool{Concurrency: 1}
	require.NoError(t, c.GPTQoSConfigureCapacity(ctx, cfg, []service.Account{a}))
	slot := service.GPTQoSAccountSlotID(&a)
	ok, err := c.AcquireAccountSlot(ctx, slot, 1, "legacy")
	require.NoError(t, err)
	require.True(t, ok)
	e := qosEntry("proxy-qos", 42, "premium")
	e.Candidates[0].AccountID, e.Candidates[0].SlotID, e.Candidates[0].PoolID = a.ID, slot, "shared:vm"
	g, err := c.GPTQoSPoll(ctx, e)
	require.NoError(t, err)
	require.Equal(t, "waiting", g.Status)
	require.NoError(t, c.ReleaseAccountSlot(ctx, slot, "legacy"))
	g, err = c.GPTQoSPoll(ctx, e)
	require.NoError(t, err)
	require.Equal(t, "granted", g.Status)
	require.NoError(t, c.GPTQoSClose(ctx, e.ID))
	require.Zero(t, c.rdb.ZCard(ctx, fmt.Sprintf("concurrency:account:%d", slot)).Val())
	require.Zero(t, c.rdb.ZCard(ctx, "gptqos:{v1}:pool:shared:vm").Val())
}

func TestGPTQoSAggregateReserveAcrossSingleSlotAccounts(t *testing.T) {
	c, m := qosTestCache(t)
	ctx := context.Background()
	now := time.Now()
	m.SetTime(now)
	cfg := service.DefaultGPTQoSConfig()
	cfg.CapacityEnabled, cfg.Peak.Enabled, cfg.Version = true, true, 1
	var accounts []service.Account
	for i := int64(1); i <= 20; i++ {
		accounts = append(accounts, service.Account{ID: i, Status: service.StatusActive, Schedulable: true, Concurrency: 1, Extra: map[string]any{service.AccountQualityTierKey: "strong"}})
	}
	require.NoError(t, c.GPTQoSConfigureCapacity(ctx, cfg, accounts))
	entry := func(id string, uid, account int64, tier string) service.GPTQoSEntry {
		e := qosEntry(id, uid, tier)
		e.Peak = cfg.Peak
		e.Peak.EnterHoldMs = 100
		e.Candidates[0].AccountID, e.Candidates[0].SlotID, e.Candidates[0].PoolID = account, account, fmt.Sprintf("account:%d", account)
		return e
	}
	for i := int64(1); i <= 17; i++ {
		g, err := c.GPTQoSPoll(ctx, entry(fmt.Sprint(i), i, i, "standard"))
		require.NoError(t, err)
		require.Equal(t, "granted", g.Status)
	}
	m.SetTime(now.Add(150 * time.Millisecond))
	low := entry("low", 100, 18, "economy")
	g, err := c.GPTQoSPoll(ctx, low)
	require.NoError(t, err)
	require.Equal(t, "waiting", g.Status)
	for i := 1; i <= 3; i++ {
		require.NoError(t, c.GPTQoSClose(ctx, fmt.Sprint(i)))
	}
	g, err = c.GPTQoSPoll(ctx, low)
	require.NoError(t, err)
	require.Equal(t, "waiting", g.Status, "14/20 shared slots are occupied; six aggregate slots stay protected")
	ok, err := c.AcquireAccountSlot(ctx, 18, 1, "legacy-low")
	require.NoError(t, err)
	require.False(t, ok)
	g, err = c.GPTQoSPoll(ctx, entry("premium", 101, 18, "premium"))
	require.NoError(t, err)
	require.Equal(t, "granted", g.Status)
	state, err := c.GPTQoSState(ctx)
	require.NoError(t, err)
	global := state["strong_pool"].(map[string]any)
	require.EqualValues(t, 20, global["capacity"])
	require.EqualValues(t, 6, global["reserved"])
	require.EqualValues(t, 1, global["premium_reserved_used"])
}
