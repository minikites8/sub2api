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

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/ctxkey"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

type gptQoSSnapshotAccountRepo struct {
	service.AccountRepository
	account service.Account
}

func (r gptQoSSnapshotAccountRepo) GetByID(_ context.Context, id int64) (*service.Account, error) {
	if id != r.account.ID {
		return nil, fmt.Errorf("fixture account not found")
	}
	a := r.account
	return &a, nil
}

func TestGPTQoSRealRedisCachedProxySelectionSharesLegacySlot(t *testing.T) {
	ctx := context.WithValue(context.Background(), ctxkey.UserID, int64(42))
	rdb := testRedis(t)
	cache := NewConcurrencyCache(rdb, 15, 60)
	// Separate service/cache owners contend on the same Redis semaphore.
	legacy := NewConcurrencyCache(rdb, 15, 60)
	cfg := &config.Config{RunMode: config.RunModeSimple}
	settings := service.NewSettingService(NewSettingRepository(testEntTx(t).Client()), cfg)
	policy := service.DefaultGPTQoSConfig()
	policy.Enabled, policy.QualityRoutingEnabled = true, true
	policy.LogSampleRate = 0
	require.NoError(t, settings.SaveGPTQoSConfig(ctx, policy))
	proxyID := int64(12)
	account := service.Account{ID: 802, Platform: service.PlatformOpenAI, Type: service.AccountTypeAPIKey, Status: service.StatusActive, Schedulable: true, Concurrency: 1, ProxyID: &proxyID, Credentials: map[string]any{"model_mapping": map[string]any{"public-model": "upstream-model"}}, Extra: map[string]any{service.AccountQualityTierKey: "strong"}}
	repo := gptQoSSnapshotAccountRepo{account: account}
	snapshotCache := NewSchedulerCache(rdb)
	bucket := service.SchedulerBucket{Platform: service.PlatformOpenAI, Mode: service.SchedulerModeSingle}
	token, err := snapshotCache.CaptureBucketWriteToken(ctx, bucket)
	require.NoError(t, err)
	require.NoError(t, snapshotCache.SetSnapshot(ctx, bucket, token, []service.Account{account}))
	meta, hit, err := snapshotCache.GetSnapshot(ctx, bucket)
	require.NoError(t, err)
	require.True(t, hit)
	require.Len(t, meta, 1)
	require.Nil(t, meta[0].ProxyID, "the candidate projection must remain stripped; QoS hydrates before admission")
	snapshot := service.NewSchedulerSnapshotService(snapshotCache, nil, repo, nil, cfg)
	gateway := service.NewOpenAIGatewayService(repo, nil, nil, nil, nil, nil, nil, nil, cfg, snapshot, service.NewConcurrencyService(cache), nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, settings, nil, nil)
	selection, _, err := gateway.SelectAccountWithScheduler(ctx, nil, "", "", "public-model", nil, service.OpenAIUpstreamTransportAny, false)
	require.NoError(t, err)
	require.True(t, selection.Acquired)
	t.Cleanup(selection.ReleaseFunc)
	require.Equal(t, account.ID, selection.Account.ID)
	slot := service.AccountProxySlotID(account.ID, proxyID)
	require.Equal(t, int64(1), rdb.ZCard(ctx, fmt.Sprintf("concurrency:account:%d", slot)).Val())
	require.Zero(t, rdb.ZCard(ctx, "concurrency:account:802").Val())
	ok, err := legacy.AcquireAccountSlot(ctx, slot, 1, "other-instance")
	require.NoError(t, err)
	require.False(t, ok, "the other instance cannot oversell the hydrated proxy slot")
	selection.ReleaseFunc()
	selection.ReleaseFunc() // Public release remains idempotent.
	require.Zero(t, rdb.ZCard(ctx, "gptqos:{v1}:leases").Val())
	require.Zero(t, rdb.ZCard(ctx, "gptqos:{v1}:waiting").Val())
	ok, err = legacy.AcquireAccountSlot(ctx, slot, 1, "after-release")
	require.NoError(t, err)
	require.True(t, ok)
	require.NoError(t, legacy.ReleaseAccountSlot(ctx, slot, "after-release"))
}

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

func TestGPTQoSPoolLabelProjectionUsesCurrentNamesAndSharedMembers(t *testing.T) {
	tx := testEntTx(t)
	ctx := context.Background()
	client := tx.Client()
	a := mustCreateAccount(t, client, &service.Account{Name: "正常 OAuth", Platform: service.PlatformOpenAI, Type: service.AccountTypeOAuth, Credentials: map[string]any{"access_token": "fixture-secret-not-for-display"}, Extra: map[string]any{service.AccountQualityTierKey: "strong"}})
	b := mustCreateAccount(t, client, &service.Account{Name: "共享 A", Platform: service.PlatformOpenAI, Extra: map[string]any{service.AccountQualityTierKey: "basic", service.AccountResourcePoolKey: "fixture"}})
	c := mustCreateAccount(t, client, &service.Account{Name: "共享 B", Platform: service.PlatformOpenAI, Extra: map[string]any{service.AccountResourcePoolKey: "fixture"}})
	mustCreateAccount(t, client, &service.Account{Name: "其他平台", Platform: service.PlatformAnthropic, Extra: map[string]any{service.AccountResourcePoolKey: "fixture"}})
	repo := newAccountRepositoryWithSQL(client, tx, nil)
	rows, err := repo.GetGPTQoSPoolAccounts(ctx, []string{fmt.Sprintf("account:%d", a.ID), "shared:fixture", fmt.Sprintf("account:%d", b.ID)})
	require.NoError(t, err)
	require.Equal(t, []service.GPTQoSPoolAccount{
		{ID: a.ID, Name: "正常 OAuth", Quality: "strong", PoolID: fmt.Sprintf("account:%d", a.ID)},
		{ID: b.ID, Name: "共享 A", Quality: "basic", PoolID: "shared:fixture"},
		{ID: c.ID, Name: "共享 B", Quality: "unknown", PoolID: "shared:fixture"},
	}, rows)
	_, err = client.Account.UpdateOneID(a.ID).SetName("已改名").Save(ctx)
	require.NoError(t, err)
	rows, err = repo.GetGPTQoSPoolAccounts(ctx, []string{fmt.Sprintf("account:%d", a.ID)})
	require.NoError(t, err)
	require.Equal(t, "已改名", rows[0].Name)
}

func TestGPTQoSRealRedisLoadRouting(t *testing.T) {
	rdb := testRedis(t)
	c := NewConcurrencyCache(rdb, 15, 60).(*concurrencyCache)
	ctx := context.Background()
	counts := map[int64]int{}
	for i := 0; i < 60; i++ {
		e := qosEntry(fmt.Sprintf("load-serial-%d", i), 42, "premium")
		e.Candidates = []service.GPTQoSCandidate{
			{AccountID: 1, SlotID: 1, Concurrency: 10, PoolID: "account:1", PoolCapacity: 10, Quality: "strong"},
			{AccountID: 2, SlotID: 2, Concurrency: 3, PoolID: "account:2", PoolCapacity: 3, Quality: "strong"},
			{AccountID: 3, SlotID: 3, Concurrency: 3, PoolID: "account:3", PoolCapacity: 3, Quality: "strong"},
		}
		g, err := c.GPTQoSPoll(ctx, e)
		require.NoError(t, err)
		require.Equal(t, "granted", g.Status)
		counts[g.Candidate.AccountID]++
		require.NoError(t, c.GPTQoSClose(ctx, e.ID))
	}
	require.Equal(t, map[int64]int{1: 20, 2: 20, 3: 20}, counts)
	t.Logf("60 completed idle-pool turns: %v", counts)
	for i := 0; i < 6; i++ {
		ok, err := c.AcquireAccountSlot(ctx, 1, 10, fmt.Sprintf("load-in-use-%d", i))
		require.NoError(t, err)
		require.True(t, ok)
	}
	e := qosEntry("load-new-request", 43, "standard")
	e.Candidates = []service.GPTQoSCandidate{
		{AccountID: 1, SlotID: 1, Concurrency: 10, PoolID: "account:1", PoolCapacity: 10, Quality: "strong"},
		{AccountID: 2, SlotID: 2, Concurrency: 3, PoolID: "account:2", PoolCapacity: 3, Quality: "strong"},
	}
	g, err := c.GPTQoSPoll(ctx, e)
	require.NoError(t, err)
	require.Equal(t, int64(2), g.Candidate.AccountID, "the busy first pool must yield to the idle peer")
	require.NoError(t, c.GPTQoSClose(ctx, e.ID))
	for i := 0; i < 6; i++ {
		require.NoError(t, c.ReleaseAccountSlot(ctx, 1, fmt.Sprintf("load-in-use-%d", i)))
	}
	require.Zero(t, rdb.ZCard(ctx, "gptqos:{v1}:leases").Val())
	require.Zero(t, rdb.ZCard(ctx, "gptqos:{v1}:waiting").Val())
}

func TestGPTQoSRealRedisCandidateScanLatencySample(t *testing.T) {
	for _, n := range []int{1, 16, 64, 256} {
		t.Run(fmt.Sprintf("candidates=%d", n), func(t *testing.T) {
			c := NewConcurrencyCache(testRedis(t), 15, 60).(*concurrencyCache)
			ctx := context.Background()
			var elapsed []time.Duration
			for i := 0; i < 25; i++ {
				e := qosEntry(fmt.Sprintf("scan-%d", i), 42, "standard")
				e.Candidates = nil
				for j := 1; j <= n; j++ {
					id := int64(j)
					e.Candidates = append(e.Candidates, service.GPTQoSCandidate{AccountID: id, SlotID: id, Concurrency: 10, PoolID: fmt.Sprintf("account:%d", j), PoolCapacity: 10, Quality: "strong"})
				}
				started := time.Now()
				g, err := c.GPTQoSPoll(ctx, e)
				require.NoError(t, err)
				require.Equal(t, "granted", g.Status)
				require.NoError(t, c.GPTQoSClose(ctx, e.ID))
				if i >= 5 {
					elapsed = append(elapsed, time.Since(started))
				}
			}
			sort.Slice(elapsed, func(i, j int) bool { return elapsed[i] < elapsed[j] })
			t.Logf("20 local Redis acquire+release samples after 5 warmups: P50=%s P95=%s", elapsed[9], elapsed[18])
		})
	}
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

func TestGPTQoSGroupConfigPersistsAcrossServiceInstances(t *testing.T) {
	ctx := context.Background()
	repo := NewSettingRepository(testEntTx(t).Client())
	writer := service.NewSettingService(repo, nil)
	cfg := service.DefaultGPTQoSConfig()
	cfg.Groups["42"] = service.DefaultGPTQoSGroupConfig("premium")
	require.NoError(t, writer.SaveGPTQoSConfig(ctx, cfg))

	// A reopened editor reads persistent configuration, not the writer's
	// process-local snapshot or the ordinary group's subscription_type.
	reader := service.NewSettingService(repo, nil)
	reopened, err := reader.GetGPTQoSConfig(ctx)
	require.NoError(t, err)
	id := int64(42)
	require.Equal(t, service.DefaultGPTQoSGroupConfig("premium"), reopened.Group(&id))
	require.False(t, reopened.Enabled, "saving a group must not enable scheduling")
	reopened.Groups["43"] = service.DefaultGPTQoSGroupConfig("economy")
	require.NoError(t, reader.SaveGPTQoSConfig(ctx, reopened))
	final, err := service.NewSettingService(repo, nil).GetGPTQoSConfig(ctx)
	require.NoError(t, err)
	require.Equal(t, service.DefaultGPTQoSGroupConfig("premium"), final.Groups["42"])
	require.Equal(t, service.DefaultGPTQoSGroupConfig("economy"), final.Groups["43"])
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

func TestGPTQoSRealRedisCapacitySameRevisionAccountMutation(t *testing.T) {
	ctx := context.Background()
	rdb := testRedis(t)
	c := NewConcurrencyCache(rdb, 15, 60).(*concurrencyCache)
	cfg := service.DefaultGPTQoSConfig()
	cfg.Version = 1793031000000087654
	cfg.CapacityEnabled = true
	cfg.Peak.Enabled = true
	cfg.ResourcePools["fixture"] = service.GPTQoSResourcePool{Concurrency: 10}
	accounts := []service.Account{}
	for id := int64(1); id <= 2; id++ {
		accounts = append(accounts, service.Account{ID: id, Platform: service.PlatformOpenAI, Type: service.AccountTypeAPIKey, Status: service.StatusActive, Schedulable: true, Concurrency: 10, Extra: map[string]any{service.AccountQualityTierKey: "strong"}})
	}
	require.NoError(t, c.GPTQoSConfigureCapacity(ctx, cfg, accounts))
	require.Equal(t, "20", rdb.HGet(ctx, "gptqos:{v1}:capacity_policy", "strong_capacity").Val())
	for i := range accounts {
		accounts[i].Extra[service.AccountResourcePoolKey] = "fixture"
	}
	require.NoError(t, c.GPTQoSConfigureCapacity(ctx, cfg, accounts))
	require.Equal(t, "10", rdb.HGet(ctx, "gptqos:{v1}:capacity_policy", "strong_capacity").Val(), "same policy revision must still apply account/pool mutations")
	require.Equal(t, "shared:fixture", rdb.HGet(ctx, "gptqos:{v1}:account_pools", "1").Val())
	require.Equal(t, "1793031000000087654", rdb.HGet(ctx, "gptqos:{v1}:capacity_policy", "version").Val())
	// A genuinely older revision must not overwrite the current registry.
	cfg.Version--
	for i := range accounts {
		delete(accounts[i].Extra, service.AccountResourcePoolKey)
	}
	require.NoError(t, c.GPTQoSConfigureCapacity(ctx, cfg, accounts))
	require.Equal(t, "10", rdb.HGet(ctx, "gptqos:{v1}:capacity_policy", "strong_capacity").Val())
	// Migrate a registry written in Redis Lua's former scientific notation.
	cfg.Version++
	require.NoError(t, rdb.HSet(ctx, "gptqos:{v1}:capacity_policy", "version", "1.7930310000001e+18").Err())
	require.NoError(t, c.GPTQoSConfigureCapacity(ctx, cfg, accounts))
	require.Equal(t, "20", rdb.HGet(ctx, "gptqos:{v1}:capacity_policy", "strong_capacity").Val())
}

func TestGPTQoSRealRedisWaitExpirationRemainsTimeout(t *testing.T) {
	ctx := context.Background()
	rdb := testRedis(t)
	owner := NewConcurrencyCache(rdb, 15, 60).(*concurrencyCache)
	reaper := NewConcurrencyCache(rdb, 15, 60).(*concurrencyCache)
	ok, err := owner.AcquireAccountSlot(ctx, 1, 1, "running-fixture")
	require.NoError(t, err)
	require.True(t, ok)
	e := qosEntry("expired-owner", 42, "standard")
	e.WaitMs = 80
	g, err := owner.GPTQoSPoll(ctx, e)
	require.NoError(t, err)
	require.Equal(t, "waiting", g.Status)
	time.Sleep(100 * time.Millisecond)
	state, err := reaper.GPTQoSState(ctx)
	require.NoError(t, err)
	require.EqualValues(t, 0, state["queued"])
	g, err = owner.GPTQoSPoll(ctx, e)
	require.NoError(t, err)
	require.Equal(t, "timeout", g.Status)
	require.NoError(t, owner.GPTQoSClose(ctx, e.ID))
	require.NoError(t, owner.GPTQoSClose(ctx, e.ID))
	require.NoError(t, owner.ReleaseAccountSlot(ctx, 1, "running-fixture"))
	next := qosEntry("next-owner", 43, "premium")
	g, err = reaper.GPTQoSPoll(ctx, next)
	require.NoError(t, err)
	require.Equal(t, "granted", g.Status)
	g, err = owner.GPTQoSPoll(ctx, e)
	require.NoError(t, err)
	require.Equal(t, "timeout", g.Status)
	require.Equal(t, int64(1), rdb.ZCard(ctx, "concurrency:account:1").Val(), "expired request may not consume the new owner's slot")
	require.NoError(t, reaper.GPTQoSClose(ctx, next.ID))
	state, err = owner.GPTQoSState(ctx)
	require.NoError(t, err)
	require.EqualValues(t, 0, state["active"])
	require.EqualValues(t, 0, state["queued"])
}
