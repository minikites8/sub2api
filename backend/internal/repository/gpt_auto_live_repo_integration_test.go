//go:build integration

package repository

import (
	"context"
	"fmt"
	"io/fs"
	"testing"
	"testing/fstest"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/Wei-Shaw/sub2api/migrations"
	"github.com/stretchr/testify/require"
	tcpostgres "github.com/testcontainers/testcontainers-go/modules/postgres"
)

func TestGPTAutoLiveDecisionRoundTripAndExport(t *testing.T) {
	ctx := context.Background()
	repo := NewGPTAutoShadowRepository(integrationDB).(*gptAutoShadowRepository)
	suffix := time.Now().UnixNano()
	owner := mustCreateAPIKeyRepoUser(t, ctx, integrationEntClient, fmt.Sprintf("gptauto-live-%d@test.com", suffix))
	stranger := mustCreateAPIKeyRepoUser(t, ctx, integrationEntClient, fmt.Sprintf("gptauto-other-%d@test.com", suffix))
	accountID := mustCreateAPIKeyRepoAccount(t, ctx, integrationEntClient, fmt.Sprintf("gptauto-live-acc-%d", suffix))
	ownerKey := mustCreateApiKey(t, integrationEntClient, &service.APIKey{UserID: owner.ID, Key: fmt.Sprintf("sk-gptauto-live-%d", suffix), Name: "live", Status: service.StatusActive})
	strangerKey := mustCreateApiKey(t, integrationEntClient, &service.APIKey{UserID: stranger.ID, Key: fmt.Sprintf("sk-gptauto-oth-%d", suffix), Name: "other", Status: service.StatusActive})

	base := time.Now().UTC().Truncate(time.Second).Add(-time.Hour)
	group := suffix
	status, ttft, total, gateway, weight := 200, 321, 1500, 12, 90
	multiplier := 1.5
	insert := func(row *service.GPTAutoShadowDecision) *service.GPTAutoShadowDecision {
		t.Helper()
		row.GroupID = &group
		require.NoError(t, repo.Insert(ctx, row))
		return row
	}
	shadow := insert(&service.GPTAutoShadowDecision{CreatedAt: base, RequestID: fmt.Sprintf("shadow-%d", suffix),
		UserID: &owner.ID, APIKeyID: &ownerKey.ID, Endpoint: service.GPTAutoEndpointChatCompletions,
		ActualModel: service.GPTAutoModelGPT56Sol, JevModel: service.GPTAutoModelGPT56Sol, FinalModel: service.GPTAutoModelGPT56Sol,
		EvaluationStatus: service.GPTAutoEvaluationSuccess, PolicyVersion: "shadow-v1"})
	liveRequest := fmt.Sprintf("live-%d", suffix)
	accountRef := accountID
	live := insert(&service.GPTAutoShadowDecision{CreatedAt: base.Add(time.Minute), Mode: service.GPTAutoModeLive, RequestID: liveRequest,
		UserID: &owner.ID, APIKeyID: &ownerKey.ID, Endpoint: service.GPTAutoEndpointResponses,
		ActualModel: service.GPTAutoVirtualModel, JevModel: service.GPTAutoModelGPT61Sol, FinalModel: service.GPTAutoModelGPT61Sol,
		FinalEffort: service.GPTAutoEffortHigh, EvaluationStatus: service.GPTAutoEvaluationSuccess, Category: service.GPTAutoCategoryCoding,
		RoutingReason: service.GPTAutoRouteClassified, AffinityOutcome: service.GPTAutoAffinityNone, ServedModel: "gpt-6.1-sol-2026-09",
		UpstreamAccountID: &accountRef, RequestStatus: service.GPTAutoRequestSucceeded, HTTPStatus: &status, RetryCount: 1,
		GatewayLatencyMS: &gateway, UpstreamTTFTMS: &ttft, TotalLatencyMS: &total, BillingMultiplier: &multiplier, RoutingWeight: &weight,
		StateVersion: service.GPTAutoClassifierStateVersion, ClassifierSchema: service.GPTAutoClassifierSchemaVersion,
		ClassifierCost: 0.0000126, PolicyVersion: service.GPTAutoPolicyVersionDefault,
		Classification: &service.GPTAutoClassification{Model: service.GPTAutoModelGPT61Sol, ModelProbabilities: map[string]float64{service.GPTAutoModelGPT61Sol: 0.8}}})
	insert(&service.GPTAutoShadowDecision{CreatedAt: base.Add(2 * time.Minute), Mode: service.GPTAutoModeLive, RequestID: fmt.Sprintf("stranger-%d", suffix),
		UserID: &stranger.ID, APIKeyID: &strangerKey.ID, Endpoint: service.GPTAutoEndpointChatCompletions,
		ActualModel: service.GPTAutoVirtualModel, FinalModel: service.GPTAutoModelGPT6Luna, EvaluationStatus: service.GPTAutoEvaluationFailed,
		RoutingReason: service.GPTAutoRouteFallback, FallbackReason: service.GPTAutoErrTimeout, RequestStatus: service.GPTAutoRequestSucceeded})

	got, err := repo.GetByID(ctx, live.ID)
	require.NoError(t, err)
	require.Equal(t, service.GPTAutoModeLive, got.Mode)
	require.Equal(t, "gpt-6.1-sol-2026-09", got.ServedModel)
	require.Equal(t, accountID, *got.UpstreamAccountID)
	require.Equal(t, 321, *got.UpstreamTTFTMS)
	require.InDelta(t, 1.5, *got.BillingMultiplier, 1e-9)
	require.Equal(t, 1, got.RetryCount)
	historical, err := repo.GetByID(ctx, shadow.ID)
	require.NoError(t, err)
	require.Equal(t, service.GPTAutoModeShadow, historical.Mode, "rows without a mode read as shadow")
	require.Equal(t, service.GPTAutoModelGPT56Sol, historical.FinalModel, "historical retired-model choices are kept")

	// Charged usage for the live request, matched on (request_id, api_key_id).
	_, err = integrationEntClient.UsageLog.Create().
		SetUserID(owner.ID).SetAPIKeyID(ownerKey.ID).SetAccountID(accountID).
		SetRequestID(liveRequest).SetModel(service.GPTAutoModelGPT61Sol).SetRequestedModel(service.GPTAutoVirtualModel).
		SetInputTokens(1000).SetOutputTokens(500).SetCacheReadTokens(200).SetTotalCost(0.01).SetActualCost(0.0150000001).
		SetRateMultiplier(1.5).SetCreatedAt(base.Add(time.Minute + 3*time.Second)).Save(ctx)
	require.NoError(t, err)

	window := service.GPTAutoExportFilter{StartTime: base.Add(-time.Minute), EndTime: base.Add(time.Hour), GroupID: &group}

	all, err := repo.ExportDecisions(ctx, window, nil, 0, 10)
	require.NoError(t, err)
	require.Len(t, all, 3)
	require.Equal(t, live.ID+1, all[0].ID, "newest first")
	require.NotNil(t, all[1].Classification, "export rows carry the classification JSON")

	// Keyset pagination: one row per page, no duplicates, no gaps.
	var ids []int64
	var cursorTime *time.Time
	var cursorID int64
	for {
		page, err := repo.ExportDecisions(ctx, window, cursorTime, cursorID, 1)
		require.NoError(t, err)
		if len(page) == 0 {
			break
		}
		ids = append(ids, page[0].ID)
		created := page[0].CreatedAt
		cursorTime, cursorID = &created, page[0].ID
	}
	require.Equal(t, []int64{all[0].ID, all[1].ID, all[2].ID}, ids)

	liveOnly := window
	liveOnly.Mode = service.GPTAutoModeLive
	rows, err := repo.ExportDecisions(ctx, liveOnly, nil, 0, 10)
	require.NoError(t, err)
	require.Len(t, rows, 2)

	// Customer scoping: the owner can never see the stranger's row.
	ownerScope := liveOnly
	ownerScope.UserID = &owner.ID
	rows, err = repo.ExportDecisions(ctx, ownerScope, nil, 0, 10)
	require.NoError(t, err)
	require.Len(t, rows, 1)
	require.Equal(t, liveRequest, rows[0].RequestID)

	for name, mutate := range map[string]func(*service.GPTAutoExportFilter){
		"final model": func(f *service.GPTAutoExportFilter) { f.FinalModel = service.GPTAutoModelGPT6Luna },
		"evaluation":  func(f *service.GPTAutoExportFilter) { f.EvaluationStatus = service.GPTAutoEvaluationFailed },
		"requested model": func(f *service.GPTAutoExportFilter) {
			f.RequestedModel = service.GPTAutoVirtualModel
			f.UserID = &stranger.ID
		},
	} {
		t.Run(name, func(t *testing.T) {
			filter := window
			mutate(&filter)
			rows, err := repo.ExportDecisions(ctx, filter, nil, 0, 10)
			require.NoError(t, err)
			require.Len(t, rows, 1)
			require.Equal(t, stranger.ID, *rows[0].UserID)
		})
	}
	fallbackOnly := window
	fallbackOnly.FallbackOnly = true
	rows, err = repo.ExportDecisions(ctx, fallbackOnly, nil, 0, 10)
	require.NoError(t, err)
	require.Len(t, rows, 2, "fallback rows and rerouted rows")
	account := accountID
	byAccount := window
	byAccount.UpstreamAccount = &account
	rows, err = repo.ExportDecisions(ctx, byAccount, nil, 0, 10)
	require.NoError(t, err)
	require.Len(t, rows, 1)
	lo, hi := 0.5, 0.9
	byConfidence := window
	byConfidence.ConfidenceMin, byConfidence.ConfidenceMax = &lo, &hi
	rows, err = repo.ExportDecisions(ctx, byConfidence, nil, 0, 10)
	require.NoError(t, err)
	require.Empty(t, rows, "rows without a confidence are excluded by a confidence range")

	count, err := repo.CountDecisions(ctx, window, 2)
	require.NoError(t, err)
	require.Equal(t, int64(2), count, "the count stops at its limit")

	usage, err := repo.ExportUsage(ctx, []service.GPTAutoExportUsageKey{
		{RequestID: liveRequest, APIKeyID: ownerKey.ID},
		{RequestID: liveRequest, APIKeyID: strangerKey.ID}, // same id, other key: no match
	}, base.Add(-time.Minute), base.Add(time.Hour))
	require.NoError(t, err)
	require.Len(t, usage, 1)
	matched := usage[service.GPTAutoExportUsageKey{RequestID: liveRequest, APIKeyID: ownerKey.ID}]
	require.EqualValues(t, 200, matched.CacheReadTokens)
	require.InDelta(t, 0.0150000001, matched.ChargedCost, 1e-12, "NUMERIC(20,10) precision survives")
	require.Equal(t, service.GPTAutoVirtualModel, matched.RequestedModel)

	require.NoError(t, repo.InsertExportAudit(ctx, service.GPTAutoExportAudit{ActorID: owner.ID, Scope: service.GPTAutoExportScopeCustomer,
		Format: service.GPTAutoExportFormatCSV, Filters: ownerScope, RowCount: 1, Status: "completed", DurationMS: 4}))
	var audited int
	require.NoError(t, integrationDB.QueryRowContext(ctx, `SELECT COUNT(*) FROM gpt_auto_export_audit WHERE actor_id = $1 AND scope = 'customer' AND row_count = 1`, owner.ID).Scan(&audited))
	require.Equal(t, 1, audited)

	listed, page, err := repo.List(ctx, service.GPTAutoDecisionFilter{Mode: service.GPTAutoModeLive, UserID: &owner.ID, GroupID: &group,
		Pagination: pagination.PaginationParams{Page: 1, PageSize: 20}})
	require.NoError(t, err)
	require.EqualValues(t, 1, page.Total)
	require.Equal(t, liveRequest, listed[0].RequestID)

	stats, err := repo.Stats(ctx, service.GPTAutoStatsFilter{StartTime: base.Add(-time.Minute), EndTime: base.Add(time.Hour), GroupID: &group, Mode: service.GPTAutoModeShadow})
	require.NoError(t, err)
	require.EqualValues(t, 1, stats.Total)
}

func TestGPTAutoLiveMigrationIndexes(t *testing.T) {
	ctx := context.Background()
	for _, index := range []string{gptAutoDecisionsModeIndex, gptAutoDecisionsUserLiveIndex} {
		var valid bool
		err := integrationDB.QueryRowContext(ctx, `SELECT i.indisvalid FROM pg_class c JOIN pg_index i ON i.indexrelid = c.oid WHERE c.relname = $1`, index).Scan(&valid)
		require.NoError(t, err, index)
		require.True(t, valid, index)
	}
	var mode string
	require.NoError(t, integrationDB.QueryRowContext(ctx, `SELECT column_default FROM information_schema.columns
WHERE table_name = 'gpt_auto_shadow_decisions' AND column_name = 'mode'`).Scan(&mode))
	require.Contains(t, mode, "shadow")
}

// TestGPTAutoPhase2UpgradeKeepsPhase1Rows applies everything up to the Phase
// 1 schema, writes a Phase 1 shadow decision recommending the now-retired
// gpt-5.6-sol, then upgrades. The row must survive unchanged and read as
// shadow; the new columns must take their neutral defaults.
func TestGPTAutoPhase2UpgradeKeepsPhase1Rows(t *testing.T) {
	ctx := context.Background()
	container, err := tcpostgres.Run(ctx, selectDockerImage(ctx, postgresImageTag),
		tcpostgres.WithDatabase("gptauto_phase2"), tcpostgres.WithUsername("postgres"), tcpostgres.WithPassword("postgres"),
		tcpostgres.BasicWaitStrategies())
	require.NoError(t, err)
	t.Cleanup(func() { _ = container.Terminate(ctx) })
	dsn, err := container.ConnectionString(ctx, "sslmode=disable", "TimeZone=UTC")
	require.NoError(t, err)
	db, err := openSQLWithRetry(ctx, dsn, 30*time.Second)
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })

	phase1 := fstest.MapFS{}
	names, err := fs.Glob(migrations.FS, "*.sql")
	require.NoError(t, err)
	for _, name := range names {
		if name == "280_gpt_auto_live_routing.sql" || name == gptAutoLiveIndexesMigration || name == "282_gpt_auto_independent_pricing.sql" {
			continue
		}
		raw, err := fs.ReadFile(migrations.FS, name)
		require.NoError(t, err)
		phase1[name] = &fstest.MapFile{Data: raw}
	}
	require.NoError(t, applyMigrationsFS(ctx, db, phase1))
	_, err = db.ExecContext(ctx, `INSERT INTO gpt_auto_shadow_decisions
  (request_id, evaluation_status, actual_model, jev_model, final_model, final_effort, policy_version, est_revenue)
VALUES ('phase1-row', 'success', 'gpt-5.6-sol', 'gpt-5.6-sol', 'gpt-5.6-sol', 'high', 'shadow-v1', 0.0123456789)`)
	require.NoError(t, err)

	require.NoError(t, applyMigrationsFS(ctx, db, migrations.FS))
	require.NoError(t, applyMigrationsFS(ctx, db, migrations.FS), "re-running migrations is a no-op")

	repo := NewGPTAutoShadowRepository(db).(*gptAutoShadowRepository)
	rows, err := repo.ExportDecisions(ctx, service.GPTAutoExportFilter{StartTime: time.Now().Add(-time.Hour), EndTime: time.Now().Add(time.Hour)}, nil, 0, 10)
	require.NoError(t, err)
	require.Len(t, rows, 1)
	row := rows[0]
	require.Equal(t, service.GPTAutoModeShadow, row.Mode)
	require.Equal(t, service.GPTAutoModelGPT56Sol, row.FinalModel)
	require.Equal(t, "shadow-v1", row.PolicyVersion)
	require.InDelta(t, 0.0123456789, *row.EstRevenue, 1e-12)
	require.Empty(t, row.RequestStatus)
	require.Nil(t, row.BillingMultiplier)
	require.Zero(t, row.RetryCount)
	var valid bool
	require.NoError(t, db.QueryRowContext(ctx, `SELECT i.indisvalid FROM pg_class c JOIN pg_index i ON i.indexrelid = c.oid WHERE c.relname = $1`, gptAutoDecisionsUserLiveIndex).Scan(&valid))
	require.True(t, valid)
}
