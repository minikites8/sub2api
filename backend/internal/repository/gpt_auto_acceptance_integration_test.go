//go:build integration

package repository

import (
	"context"
	"fmt"
	"io/fs"
	"sync"
	"testing"
	"testing/fstest"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/Wei-Shaw/sub2api/migrations"
	"github.com/stretchr/testify/require"
	tcpostgres "github.com/testcontainers/testcontainers-go/modules/postgres"
)

// TestGPTAutoMigrationUpgradeFromPreviousSchema applies every migration up to
// 277 on an isolated database, then upgrades to 278 and re-runs the full set.
// The shared harness database already covers the fresh-install path.
func TestGPTAutoMigrationUpgradeFromPreviousSchema(t *testing.T) {
	ctx := context.Background()
	container, err := tcpostgres.Run(ctx, selectDockerImage(ctx, postgresImageTag),
		tcpostgres.WithDatabase("gptauto_upgrade"), tcpostgres.WithUsername("postgres"), tcpostgres.WithPassword("postgres"),
		tcpostgres.BasicWaitStrategies())
	require.NoError(t, err)
	t.Cleanup(func() { _ = container.Terminate(ctx) })
	dsn, err := container.ConnectionString(ctx, "sslmode=disable", "TimeZone=UTC")
	require.NoError(t, err)
	db, err := openSQLWithRetry(ctx, dsn, 30*time.Second)
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })

	const target = "278_gpt_auto_shadow_decisions.sql"
	previous := fstest.MapFS{}
	names, err := fs.Glob(migrations.FS, "*.sql")
	require.NoError(t, err)
	for _, name := range names {
		if name == target {
			continue
		}
		raw, err := fs.ReadFile(migrations.FS, name)
		require.NoError(t, err)
		previous[name] = &fstest.MapFile{Data: raw}
	}
	require.NoError(t, applyMigrationsFS(ctx, db, previous))

	var exists bool
	require.NoError(t, db.QueryRowContext(ctx, `SELECT to_regclass('public.gpt_auto_shadow_decisions') IS NOT NULL`).Scan(&exists))
	require.False(t, exists, "pre-upgrade schema has no GPT-Auto tables")
	// Pre-existing data must survive the upgrade untouched.
	_, err = db.ExecContext(ctx, `INSERT INTO settings (key, value) VALUES ('gptauto_upgrade_probe', 'kept')`)
	require.NoError(t, err)

	require.NoError(t, applyMigrationsFS(ctx, db, migrations.FS))
	require.NoError(t, applyMigrationsFS(ctx, db, migrations.FS), "re-running migrations is a no-op")

	var applied int
	require.NoError(t, db.QueryRowContext(ctx, `SELECT COUNT(*) FROM schema_migrations WHERE filename = $1`, target).Scan(&applied))
	require.Equal(t, 1, applied)
	var probe string
	require.NoError(t, db.QueryRowContext(ctx, `SELECT value FROM settings WHERE key = 'gptauto_upgrade_probe'`).Scan(&probe))
	require.Equal(t, "kept", probe)

	for _, index := range []string{
		"idx_gpt_auto_decisions_created", "idx_gpt_auto_decisions_status_created", "idx_gpt_auto_decisions_actual_created",
		"idx_gpt_auto_decisions_final_created", "idx_gpt_auto_decisions_group_created", "idx_gpt_auto_decisions_request_id",
	} {
		var found bool
		require.NoError(t, db.QueryRowContext(ctx, `SELECT to_regclass('public.'||$1) IS NOT NULL`, index).Scan(&found))
		require.True(t, found, index)
	}
	var costType string
	require.NoError(t, db.QueryRowContext(ctx, `SELECT format_type(atttypid, atttypmod) FROM pg_attribute
WHERE attrelid = 'gpt_auto_shadow_decisions'::regclass AND attname = 'classifier_cost'`).Scan(&costType))
	require.Equal(t, "numeric(20,10)", costType)

	repo := NewGPTAutoShadowRepository(db)
	row := &service.GPTAutoShadowDecision{RequestID: "upgrade", Endpoint: service.GPTAutoEndpointResponses,
		EvaluationStatus: service.GPTAutoEvaluationSuccess, ClassifierCost: 0.0000000001, PolicyVersion: "shadow-v1"}
	require.NoError(t, repo.Insert(ctx, row))
	got, err := repo.GetByID(ctx, row.ID)
	require.NoError(t, err)
	require.InDelta(t, 1e-10, got.ClassifierCost, 1e-15, "numeric(20,10) keeps sub-cent Jev costs")
}

func TestGPTAutoShadowRepositoryActualUsageAndPagination(t *testing.T) {
	ctx := context.Background()
	repo := NewGPTAutoShadowRepository(integrationDB)
	suffix := time.Now().UnixNano()
	user := mustCreateAPIKeyRepoUser(t, ctx, integrationEntClient, fmt.Sprintf("gptauto-%d@test.com", suffix))
	accountID := mustCreateAPIKeyRepoAccount(t, ctx, integrationEntClient, fmt.Sprintf("gptauto-acc-%d", suffix))
	apiKey := mustCreateApiKey(t, integrationEntClient, &service.APIKey{UserID: user.ID, Key: fmt.Sprintf("sk-gptauto-%d", suffix), Name: "gptauto", Status: service.StatusActive})

	now := time.Now().UTC().Truncate(time.Second)
	requestID := fmt.Sprintf("client:gptauto-%d", suffix)
	_, err := integrationEntClient.UsageLog.Create().
		SetUserID(user.ID).SetAPIKeyID(apiKey.ID).SetAccountID(accountID).
		SetRequestID(requestID).SetModel("gpt-5.6-sol").
		SetInputTokens(1200).SetOutputTokens(800).SetTotalCost(0.01).SetActualCost(0.015).
		SetCreatedAt(now.Add(2 * time.Minute)).Save(ctx)
	require.NoError(t, err)
	_, err = integrationDB.ExecContext(ctx, `UPDATE usage_logs SET reasoning_effort = 'high', account_rate_multiplier = 0.6 WHERE request_id = $1`, requestID)
	require.NoError(t, err)

	actual, err := repo.ActualUsage(ctx, requestID, now)
	require.NoError(t, err)
	require.NotNil(t, actual)
	require.Equal(t, "gpt-5.6-sol", actual.Model)
	require.Equal(t, "high", actual.ReasoningEffort)
	require.EqualValues(t, 800, actual.OutputTokens)
	require.InDelta(t, 0.015, actual.ChargedCost, 1e-12)
	require.InDelta(t, 0.006, actual.AccountCost, 1e-12, "account cost = total_cost x account_rate_multiplier")

	missing, err := repo.ActualUsage(ctx, requestID, now.Add(-48*time.Hour))
	require.NoError(t, err)
	require.Nil(t, missing, "rows outside the time window are not matched")

	groupID := suffix
	for i := 0; i < 5; i++ {
		require.NoError(t, repo.Insert(ctx, &service.GPTAutoShadowDecision{
			CreatedAt: now.Add(time.Duration(i) * time.Second), RequestID: fmt.Sprintf("page-%d", i), GroupID: &groupID,
			Endpoint: service.GPTAutoEndpointChatCompletions, EvaluationStatus: service.GPTAutoEvaluationSuccess,
			FinalModel: service.GPTAutoModelGPT6Luna, PolicyVersion: "shadow-v1",
		}))
	}
	var seen []string
	for page := 1; page <= 3; page++ {
		items, result, err := repo.List(ctx, service.GPTAutoDecisionFilter{GroupID: &groupID,
			Pagination: pagination.PaginationParams{Page: page, PageSize: 2}})
		require.NoError(t, err)
		require.EqualValues(t, 5, result.Total)
		require.Equal(t, 3, result.Pages)
		for _, item := range items {
			seen = append(seen, item.RequestID)
		}
	}
	require.Equal(t, []string{"page-4", "page-3", "page-2", "page-1", "page-0"}, seen, "newest first, no duplicates or gaps")
}

func TestGPTAutoBudgetStoreIsAtomicAcrossNodes(t *testing.T) {
	ctx := context.Background()
	// The unprefixed client is used because the harness prefix hook does not
	// rewrite HINCRBY; keys are unique per run and removed afterwards.
	store := NewGPTAutoBudgetStore(integrationRedis)
	day := fmt.Sprintf("test-%d", time.Now().UnixNano())
	t.Cleanup(func() {
		keys, _ := integrationRedis.Keys(ctx, "gpt_auto:*:"+day+"*").Result()
		if len(keys) > 0 {
			_ = integrationRedis.Del(ctx, keys...).Err()
		}
	})
	limits := service.GPTAutoBudgetLimits{Calls: 25, CostNanoUSD: 1_000_000}

	var wg sync.WaitGroup
	var mu sync.Mutex
	admitted := 0
	for node := 0; node < 8; node++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < 20; i++ {
				_, ok, err := store.Reserve(ctx, day, limits, 1_000)
				if err != nil {
					t.Error(err)
					return
				}
				if ok {
					mu.Lock()
					admitted++
					mu.Unlock()
				}
			}
		}()
	}
	wg.Wait()
	require.Equal(t, 25, admitted, "160 concurrent attempts from 8 nodes admit exactly the call limit")
	usage, exists, err := store.Usage(ctx, day)
	require.NoError(t, err)
	require.True(t, exists)
	require.EqualValues(t, 25, usage.Calls)
	require.EqualValues(t, 25_000, usage.CostNanoUSD)

	// Settling to billed cost and refunding an unsent call.
	require.NoError(t, store.Adjust(ctx, day, 0, -500))
	require.NoError(t, store.Adjust(ctx, day, -1, -1_000))
	usage, _, err = store.Usage(ctx, day)
	require.NoError(t, err)
	require.EqualValues(t, 24, usage.Calls)
	require.EqualValues(t, 23_500, usage.CostNanoUSD)

	// Cost limit: a reservation that would cross it is refused.
	costDay := day + "-cost"
	_, ok, err := store.Reserve(ctx, costDay, service.GPTAutoBudgetLimits{CostNanoUSD: 1_500}, 1_000)
	require.NoError(t, err)
	require.True(t, ok)
	_, ok, err = store.Reserve(ctx, costDay, service.GPTAutoBudgetLimits{CostNanoUSD: 1_500}, 1_000)
	require.NoError(t, err)
	require.False(t, ok)

	// Seeding never overwrites a day that already exists.
	require.NoError(t, store.Seed(ctx, day, service.GPTAutoBudgetUsage{Calls: 0}))
	usage, _, _ = store.Usage(ctx, day)
	require.EqualValues(t, 24, usage.Calls)
	seedDay := day + "-seed"
	require.NoError(t, store.Seed(ctx, seedDay, service.GPTAutoBudgetUsage{Calls: 7, CostNanoUSD: 70}))
	usage, _, _ = store.Usage(ctx, seedDay)
	require.EqualValues(t, 7, usage.Calls)

	require.NoError(t, store.AddCounters(ctx, day, map[string]int64{"sampled": 3, "dropped_queue_full": 1}))
	require.NoError(t, store.AddCounters(ctx, day, map[string]int64{"sampled": 2}))
	counters, err := store.Counters(ctx, day)
	require.NoError(t, err)
	require.EqualValues(t, 5, counters["sampled"])
	require.EqualValues(t, 1, counters["dropped_queue_full"])
}
