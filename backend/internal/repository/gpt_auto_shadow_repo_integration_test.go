//go:build integration

package repository

import (
	"context"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func TestGPTAutoShadowRepositoryRoundTrip(t *testing.T) {
	ctx := context.Background()
	repo := NewGPTAutoShadowRepository(integrationDB)
	groupID := time.Now().UnixNano()
	now := time.Now().UTC().Truncate(time.Second)

	snapshot := service.NewGPTAutoPolicySnapshot(*service.DefaultGPTAutoShadowConfig())
	hash, raw, err := snapshot.Hash()
	require.NoError(t, err)
	require.NoError(t, repo.UpsertPolicySnapshot(ctx, hash, snapshot.PolicyVersion, raw))
	require.NoError(t, repo.UpsertPolicySnapshot(ctx, hash, snapshot.PolicyVersion, raw), "upsert is idempotent")
	loaded, err := repo.GetPolicySnapshot(ctx, hash)
	require.NoError(t, err)
	require.Len(t, loaded.ModelProfiles, 4)

	margin := 0.42
	complexity := 2.5
	success := &service.GPTAutoShadowDecision{
		CreatedAt: now, RequestID: "req-ok", GroupID: &groupID, Endpoint: service.GPTAutoEndpointChatCompletions,
		ActualModel: "gpt-5.6-sol", ActualEffort: "high", EvaluationStatus: service.GPTAutoEvaluationSuccess,
		DecisionStatus: service.GPTAutoDecisionAdjusted, JevModel: "gpt-6.1-sol", JevEffort: "none",
		FinalModel: "gpt-6.1-sol", FinalEffort: "low", Complexity: &complexity, Category: service.GPTAutoCategoryCoding,
		ClassifierModel: "jev-1", ClassifierInputTokens: 300, ClassifierOutputTokens: 4, ClassifierCost: 0.0000126,
		ClassifierLatencyMS: 420, Attempts: 1, EstGrossMargin: &margin, PolicyVersion: "shadow-v1", PolicySnapshotHash: hash,
		Classification: &service.GPTAutoClassification{Model: "gpt-6.1-sol", Effort: "none", ModelProbabilities: map[string]float64{"gpt-6.1-sol": 0.7}},
		Decision:       &service.GPTAutoPolicyDecision{Status: service.GPTAutoDecisionAdjusted, FinalModel: "gpt-6.1-sol", FinalEffort: "low"},
	}
	require.NoError(t, repo.Insert(ctx, success))
	require.Positive(t, success.ID)
	failed := &service.GPTAutoShadowDecision{
		CreatedAt: now, RequestID: "req-fail", GroupID: &groupID, Endpoint: service.GPTAutoEndpointResponses,
		ActualModel: "gpt-6-luna", EvaluationStatus: service.GPTAutoEvaluationFailed, ErrorClass: service.GPTAutoErrTimeout,
		ClassifierLatencyMS: 8000, Attempts: 2, PolicyVersion: "shadow-v1",
	}
	require.NoError(t, repo.Insert(ctx, failed))

	got, err := repo.GetByID(ctx, success.ID)
	require.NoError(t, err)
	require.Equal(t, "gpt-6.1-sol", got.FinalModel)
	require.Equal(t, 0.7, got.Classification.ModelProbabilities["gpt-6.1-sol"])
	require.Equal(t, "low", got.Decision.FinalEffort)
	require.InDelta(t, margin, *got.EstGrossMargin, 1e-9)
	require.Nil(t, got.EstRevenue)

	_, err = repo.GetByID(ctx, -1)
	require.ErrorIs(t, err, service.ErrGPTAutoDecisionNotFound)

	items, page, err := repo.List(ctx, service.GPTAutoDecisionFilter{
		Pagination: pagination.PaginationParams{Page: 1, PageSize: 10}, GroupID: &groupID,
		EvaluationStatus: service.GPTAutoEvaluationSuccess, FinalEffort: "low",
	})
	require.NoError(t, err)
	require.EqualValues(t, 1, page.Total)
	require.Equal(t, "req-ok", items[0].RequestID)

	stats, err := repo.Stats(ctx, service.GPTAutoStatsFilter{StartTime: now.Add(-time.Hour), EndTime: now.Add(time.Hour), GroupID: &groupID})
	require.NoError(t, err)
	require.EqualValues(t, 2, stats.Total)
	require.EqualValues(t, 1, stats.Success)
	require.EqualValues(t, 1, stats.Failed)
	require.EqualValues(t, 1, stats.Adjusted)
	require.EqualValues(t, 300, stats.InputTokens)
	require.Equal(t, []service.GPTAutoCount{{Key: "gpt-6.1-sol", Count: 1}}, stats.FinalModels)
	require.Equal(t, []service.GPTAutoCount{{Key: "3", Count: 1}}, stats.Complexity)
	require.Equal(t, []service.GPTAutoCount{{Key: service.GPTAutoErrTimeout, Count: 1}}, stats.ErrorClasses)
	require.Equal(t, []service.GPTAutoPairCount{{Actual: "gpt-5.6-sol", Recommended: "gpt-6.1-sol", Count: 1}}, stats.ActualVsFinal)
	require.Len(t, stats.Trend, 1)
	require.InDelta(t, 420, *stats.P50LatencyMS, 1e-9)

	usage, err := repo.UsageSince(ctx, now.Add(-time.Minute))
	require.NoError(t, err)
	require.GreaterOrEqual(t, usage.Calls, int64(2))

	deleted, err := repo.DeleteOlderThan(ctx, now.Add(time.Second), 1)
	require.NoError(t, err)
	require.EqualValues(t, 1, deleted, "deletion is batched")

	_, err = repo.OutputHistory(ctx, service.GPTAutoModelPool(), now.Add(-24*time.Hour))
	require.NoError(t, err, "usage_logs columns used by the estimator must exist")
}

func TestGPTAutoShadowRepositoryContextDiagnosticsRoundTrip(t *testing.T) {
	ctx := context.Background()
	repo := NewGPTAutoShadowRepository(integrationDB)
	now := time.Now().UTC().Truncate(time.Second)

	// Synthetic request; only its extraction metadata is stored.
	body := []byte(`{"model":"gpt-5.6-sol","messages":[{"role":"user","content":"Implement an LRU cache"},` +
		`{"role":"assistant","content":"Done."},{"role":"user","content":"continue"}]}`)
	snapshot := service.BuildGPTAutoSnapshot(service.GPTAutoEndpointChatCompletions, body, service.DefaultGPTAutoShadowConfig().ContextOptions())
	stateChars := 321
	snapshot.Context.Diagnostics.StateChars = &stateChars
	context := snapshot.Context
	withDiagnostics := &service.GPTAutoShadowDecision{
		CreatedAt: now, RequestID: "req-diag", Endpoint: service.GPTAutoEndpointChatCompletions, EvaluationStatus: service.GPTAutoEvaluationFailed,
		ErrorClass: service.GPTAutoErrTimeout, PolicyVersion: "shadow-v1",
		Features: service.GPTAutoRequestFeatures{Endpoint: service.GPTAutoEndpointChatCompletions, Context: &context},
	}
	require.NoError(t, repo.Insert(ctx, withDiagnostics))
	legacy := &service.GPTAutoShadowDecision{
		CreatedAt: now, RequestID: "req-legacy", Endpoint: service.GPTAutoEndpointChatCompletions, EvaluationStatus: service.GPTAutoEvaluationFailed,
		ErrorClass: service.GPTAutoErrTimeout, PolicyVersion: "shadow-v1",
		Features: service.GPTAutoRequestFeatures{Endpoint: service.GPTAutoEndpointChatCompletions},
	}
	require.NoError(t, repo.Insert(ctx, legacy))

	got, err := repo.GetByID(ctx, withDiagnostics.ID)
	require.NoError(t, err)
	require.NotNil(t, got.Features.Context)
	require.Equal(t, snapshot.Context.Diagnostics, got.Features.Context.Diagnostics)
	require.Equal(t, service.GPTAutoFollowUpLinked, got.Features.Context.Diagnostics.FollowUp.Resolution)

	var raw string
	require.NoError(t, integrationDB.QueryRowContext(ctx, `SELECT features::text FROM gpt_auto_shadow_decisions WHERE id = $1`, withDiagnostics.ID).Scan(&raw))
	require.NotContains(t, raw, "LRU cache", "no request text is persisted")
	require.Contains(t, raw, service.GPTAutoContextDiagnosticsVersion)

	old, err := repo.GetByID(ctx, legacy.ID)
	require.NoError(t, err)
	require.Nil(t, old.Features.Context, "rows without diagnostics stay readable and nothing is reconstructed")
}
