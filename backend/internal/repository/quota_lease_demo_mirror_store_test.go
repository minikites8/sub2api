package repository

import (
	"context"
	"database/sql"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func TestQuotaLeaseDemoMirrorStoreReconcileAccountGroupsUsesDensePlaceholders(t *testing.T) {
	store := &quotaLeaseDemoMirrorStore{}
	exec := &quotaLeaseDemoMirrorSQLRecorder{}
	now := time.Now().UTC()

	err := store.reconcileMirrorAccountGroups(
		context.Background(),
		exec,
		[]service.Account{{ID: 101}},
		[]service.AccountGroup{
			{AccountID: 0, GroupID: 7, Priority: 1, CreatedAt: now},
			{AccountID: 101, GroupID: 8, Priority: 2, CreatedAt: now},
		},
		"node-1",
	)
	require.NoError(t, err)
	require.Len(t, exec.calls, 2)

	insert := exec.calls[1]
	require.Contains(t, insert.query, "($1, $2, $3, $4)")
	require.NotContains(t, insert.query, "$5")
	require.Len(t, insert.args, 4)
	require.Equal(t, []any{int64(101), int64(8), 2, now}, insert.args)
}

func TestQuotaLeaseDemoMirrorAccountGroupsFromIDsDefaultCreatedAt(t *testing.T) {
	groups := cloneQuotaLeaseDemoMirrorAccountGroups(nil, []service.Account{{
		ID:        202,
		GroupIDs:  []int64{9},
		Priority:  3,
		CreatedAt: time.Time{},
	}})

	require.Len(t, groups, 1)
	require.Equal(t, int64(202), groups[0].AccountID)
	require.Equal(t, int64(9), groups[0].GroupID)
	require.Equal(t, 3, groups[0].Priority)
	require.False(t, groups[0].CreatedAt.IsZero())
}

func TestQuotaLeaseDemoMirrorStoreUpsertChannelsWritesPricingSnapshot(t *testing.T) {
	store := &quotaLeaseDemoMirrorStore{}
	exec := &quotaLeaseDemoMirrorSQLRecorder{}
	now := time.Now().UTC()
	inputPrice := 0.000001
	outputPrice := 0.000002
	imageInputPrice := 0.000003
	imageOutputPrice := 0.04
	priorityMultiplier := 1.25
	maxTokens := 8192

	err := store.upsertChannels(context.Background(), exec, []service.Channel{{
		ID:                         77,
		Name:                       "premium",
		Status:                     service.StatusActive,
		BillingModelSource:         service.BillingModelSourceChannelMapped,
		RestrictModels:             true,
		FeaturesConfig:             map[string]any{"web_search_emulation": map[string]any{"openai": true}},
		ApplyPricingToAccountStats: true,
		GroupIDs:                   []int64{11, 12},
		ModelMapping:               map[string]map[string]string{service.PlatformOpenAI: {"gpt-5.5": "gpt-5.5-chat-latest"}},
		ModelPricing: []service.ChannelModelPricing{{
			ID:                 88,
			Platform:           service.PlatformOpenAI,
			Models:             []string{"gpt-5.5"},
			BillingMode:        service.BillingModeToken,
			InputPrice:         &inputPrice,
			OutputPrice:        &outputPrice,
			ImageInputPrice:    &imageInputPrice,
			ImageOutputPrice:   &imageOutputPrice,
			PriorityMultiplier: &priorityMultiplier,
			Intervals: []service.PricingInterval{{
				ID:              99,
				MinTokens:       1024,
				MaxTokens:       &maxTokens,
				InputPrice:      &inputPrice,
				OutputPrice:     &outputPrice,
				PerRequestPrice: &imageOutputPrice,
				SortOrder:       1,
				CreatedAt:       now,
				UpdatedAt:       now,
			}},
			CreatedAt: now,
			UpdatedAt: now,
		}},
		CreatedAt: now,
		UpdatedAt: now,
	}})
	require.NoError(t, err)

	require.Len(t, exec.calls, 6)
	require.Contains(t, exec.calls[2].query, "INSERT INTO channels")
	require.Contains(t, exec.calls[2].query, "ON CONFLICT (id) DO UPDATE")
	require.Equal(t, int64(77), exec.calls[2].args[0])
	require.Contains(t, exec.calls[3].query, "INSERT INTO channel_groups")
	require.Contains(t, exec.calls[4].query, "INSERT INTO channel_model_pricing")
	require.Equal(t, int64(88), exec.calls[4].args[0])
	require.Equal(t, int64(77), exec.calls[4].args[1])
	require.Equal(t, service.PlatformOpenAI, exec.calls[4].args[2])
	require.Contains(t, exec.calls[5].query, "INSERT INTO channel_pricing_intervals")
	require.Equal(t, int64(99), exec.calls[5].args[0])
	require.Equal(t, int64(88), exec.calls[5].args[1])
}

type quotaLeaseDemoMirrorSQLRecorder struct {
	calls []quotaLeaseDemoMirrorSQLCall
}

type quotaLeaseDemoMirrorSQLCall struct {
	query string
	args  []any
}

func (r *quotaLeaseDemoMirrorSQLRecorder) ExecContext(_ context.Context, query string, args ...any) (sql.Result, error) {
	r.calls = append(r.calls, quotaLeaseDemoMirrorSQLCall{
		query: strings.TrimSpace(query),
		args:  append([]any(nil), args...),
	})
	return quotaLeaseDemoMirrorSQLResult(0), nil
}

func (r *quotaLeaseDemoMirrorSQLRecorder) QueryContext(context.Context, string, ...any) (*sql.Rows, error) {
	return nil, nil
}

type quotaLeaseDemoMirrorSQLResult int64

func (r quotaLeaseDemoMirrorSQLResult) LastInsertId() (int64, error) {
	return 0, nil
}

func (r quotaLeaseDemoMirrorSQLResult) RowsAffected() (int64, error) {
	return int64(r), nil
}
