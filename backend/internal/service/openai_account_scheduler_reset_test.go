package service

import (
	"context"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
)

func openAIResetTestScheduler(reset float64) *defaultOpenAIAccountScheduler {
	cfg := &config.Config{}
	cfg.Gateway.OpenAIWS.SchedulerScoreWeights = config.GatewayOpenAIWSSchedulerScoreWeights{
		Priority:      1.0,
		Load:          1.0,
		Queue:         0.7,
		ErrorRate:     0.8,
		TTFT:          0.5,
		Reset:         reset,
		QuotaHeadroom: 0,
	}
	return &defaultOpenAIAccountScheduler{service: &OpenAIGatewayService{cfg: cfg}}
}

func openAIQuotaHeadroomTestScheduler(quotaHeadroom float64) *defaultOpenAIAccountScheduler {
	cfg := &config.Config{}
	cfg.Gateway.OpenAIWS.SchedulerScoreWeights = config.GatewayOpenAIWSSchedulerScoreWeights{
		QuotaHeadroom: quotaHeadroom,
	}
	return &defaultOpenAIAccountScheduler{service: &OpenAIGatewayService{cfg: cfg}}
}

func openAIPlanScores(plan openAIAccountLoadPlan) map[int64]float64 {
	scores := make(map[int64]float64, len(plan.candidates))
	for _, c := range plan.candidates {
		scores[c.account.ID] = c.score
	}
	return scores
}

func TestOpenAIOAuthQuotaScheduleTier_Prefers5hAndRecognizesWeeklyOnly(t *testing.T) {
	fiveHour := &Account{
		Platform: PlatformOpenAI,
		Type:     AccountTypeOAuth,
		Extra:    map[string]any{"codex_has_5h_limit": true},
	}
	weeklyOnly := &Account{
		Platform: PlatformOpenAI,
		Type:     AccountTypeOAuth,
		Extra: map[string]any{
			"codex_has_5h_limit":      false,
			"codex_5h_used_percent":   1.0, // stale data from an earlier snapshot
			"codex_7d_window_minutes": 10080,
		},
	}
	apiKey := &Account{Platform: PlatformOpenAI, Type: AccountTypeAPIKey}

	require.Equal(t, openAIOAuthQuotaScheduleTierFiveHour, openAIOAuthQuotaScheduleTierFor(fiveHour))
	require.Equal(t, openAIOAuthQuotaScheduleTierWeeklyOnly, openAIOAuthQuotaScheduleTierFor(weeklyOnly))
	require.Equal(t, openAIOAuthQuotaScheduleTierNeutral, openAIOAuthQuotaScheduleTierFor(apiKey))
	require.Less(t, compareOpenAIOAuthQuotaScheduleTier(fiveHour, weeklyOnly), 0)
}

func TestBuildOpenAISelectionOrder_QuotaTierPrecedesSchedulerScore(t *testing.T) {
	scheduler := &defaultOpenAIAccountScheduler{}
	weeklyOnly := &Account{
		ID:       1,
		Platform: PlatformOpenAI,
		Type:     AccountTypeOAuth,
		Extra: map[string]any{
			"codex_has_5h_limit":      false,
			"codex_7d_window_minutes": 10080,
		},
	}
	fiveHour := &Account{
		ID:       2,
		Platform: PlatformOpenAI,
		Type:     AccountTypeOAuth,
		Extra:    map[string]any{"codex_has_5h_limit": true},
	}
	plan := openAIAccountLoadPlan{
		candidates: []openAIAccountCandidateScore{
			{account: weeklyOnly, loadInfo: &AccountLoadInfo{}, score: 100},
			{account: fiveHour, loadInfo: &AccountLoadInfo{}, score: 1},
		},
		topK: 1,
	}

	order := scheduler.buildOpenAISelectionOrder(OpenAIAccountScheduleRequest{Platform: PlatformOpenAI}, plan)
	require.Len(t, order, 2)
	require.Equal(t, int64(2), order[0].account.ID)
	require.Equal(t, int64(1), order[1].account.ID)
}

func TestOpenAIGatewayService_SelectBestAccount_QuotaTierPrecedesPriority(t *testing.T) {
	accounts := []Account{
		{
			ID: 1, Platform: PlatformOpenAI, Type: AccountTypeOAuth,
			Status: StatusActive, Schedulable: true, Priority: 10,
			Extra: map[string]any{"codex_has_5h_limit": true},
		},
		{
			ID: 2, Platform: PlatformOpenAI, Type: AccountTypeOAuth,
			Status: StatusActive, Schedulable: true, Priority: 0,
			Extra: map[string]any{
				"codex_has_5h_limit":      false,
				"codex_7d_window_minutes": 10080,
			},
		},
	}

	selected, _, _ := (&OpenAIGatewayService{}).selectBestAccount(
		context.Background(), nil, PlatformOpenAI, accounts, "gpt-5.1", nil, false, "", false,
	)
	require.NotNil(t, selected)
	require.Equal(t, int64(1), selected.ID)
}

func TestOpenAIGatewayService_SelectAccountWithLoadAwareness_QuotaTierPrecedesPriority(t *testing.T) {
	accounts := []Account{
		{
			ID: 11, Platform: PlatformOpenAI, Type: AccountTypeOAuth,
			Status: StatusActive, Schedulable: true, Priority: 10,
			Extra: map[string]any{"codex_has_5h_limit": true},
		},
		{
			ID: 12, Platform: PlatformOpenAI, Type: AccountTypeOAuth,
			Status: StatusActive, Schedulable: true, Priority: 0,
			Extra: map[string]any{
				"codex_has_5h_limit":      false,
				"codex_7d_window_minutes": 10080,
			},
		},
	}
	cfg := &config.Config{}
	cfg.Gateway.Scheduling.LoadBatchEnabled = true
	svc := &OpenAIGatewayService{
		accountRepo:        schedulerTestOpenAIAccountRepo{accounts: accounts},
		cfg:                cfg,
		concurrencyService: NewConcurrencyService(schedulerTestConcurrencyCache{}),
	}

	selection, err := svc.selectAccountWithLoadAwareness(
		context.Background(), nil, PlatformOpenAI, "", "gpt-5.1", nil, false, "", false,
	)
	require.NoError(t, err)
	require.NotNil(t, selection)
	require.NotNil(t, selection.Account)
	require.Equal(t, int64(11), selection.Account.ID)
	if selection.ReleaseFunc != nil {
		selection.ReleaseFunc()
	}
}

// Reset 权重 > 0 时，会话窗口最早重置的账号应获得更高分。
func TestBuildOpenAIAccountLoadPlan_ResetWeightPrefersSoonestReset(t *testing.T) {
	now := time.Now()
	soon := now.Add(1 * time.Hour)
	later := now.Add(20 * time.Hour)
	filtered := []*Account{
		{ID: 1, Priority: 0, SessionWindowEnd: &later},
		{ID: 2, Priority: 0, SessionWindowEnd: &soon},
	}
	sched := openAIResetTestScheduler(5.0)

	plan := sched.buildOpenAIAccountLoadPlan(context.Background(), OpenAIAccountScheduleRequest{}, filtered, map[int64]*AccountLoadInfo{})
	scores := openAIPlanScores(plan)
	require.Greater(t, scores[2], scores[1], "重置时间最早的账号（ID=2）得分更高")
}

// Reset 权重为 0（默认）时，窗口重置时间不应影响打分，保持原有行为。
func TestBuildOpenAIAccountLoadPlan_ResetWeightZeroNoEffect(t *testing.T) {
	now := time.Now()
	soon := now.Add(1 * time.Hour)
	later := now.Add(20 * time.Hour)
	filtered := []*Account{
		{ID: 1, Priority: 0, SessionWindowEnd: &later},
		{ID: 2, Priority: 0, SessionWindowEnd: &soon},
	}
	sched := openAIResetTestScheduler(0.0)

	plan := sched.buildOpenAIAccountLoadPlan(context.Background(), OpenAIAccountScheduleRequest{}, filtered, map[int64]*AccountLoadInfo{})
	scores := openAIPlanScores(plan)
	require.Equal(t, scores[1], scores[2], "Reset 权重为 0 时两账号得分相同")
}

// 无活跃窗口的账号 reset 因子为 0，应低于拥有未来窗口的账号。
func TestBuildOpenAIAccountLoadPlan_ResetWeightIgnoresNilWindow(t *testing.T) {
	now := time.Now()
	soon := now.Add(2 * time.Hour)
	filtered := []*Account{
		{ID: 1, Priority: 0, SessionWindowEnd: nil},
		{ID: 2, Priority: 0, SessionWindowEnd: &soon},
	}
	sched := openAIResetTestScheduler(5.0)

	plan := sched.buildOpenAIAccountLoadPlan(context.Background(), OpenAIAccountScheduleRequest{}, filtered, map[int64]*AccountLoadInfo{})
	scores := openAIPlanScores(plan)
	require.Greater(t, scores[2], scores[1], "拥有活跃窗口的账号得分高于无窗口账号")
}

func TestOpenAIQuotaHeadroomFactor_PrimaryUsedPercent(t *testing.T) {
	now := time.Date(2026, 3, 11, 10, 0, 0, 0, time.UTC)
	account := &Account{
		Extra: map[string]any{
			"codex_primary_used_percent": 20.0,
			"codex_primary_reset_at":     now.Add(24 * time.Hour).Format(time.RFC3339),
			"codex_usage_updated_at":     now.Add(-time.Minute).Format(time.RFC3339),
		},
	}

	require.InDelta(t, 0.8, openAIQuotaHeadroomFactor(account, now), 0.0001)
}

func TestOpenAIQuotaHeadroomFactor_PrimaryMissingIsNeutral(t *testing.T) {
	now := time.Date(2026, 3, 11, 10, 0, 0, 0, time.UTC)
	account := &Account{
		Extra: map[string]any{
			"codex_usage_updated_at": now.Add(-time.Minute).Format(time.RFC3339),
		},
	}

	require.Equal(t, openAIQuotaHeadroomNeutralFactor, openAIQuotaHeadroomFactor(account, now))
}

func TestOpenAIQuotaHeadroomFactor_PrimaryResetExpiredIsNeutral(t *testing.T) {
	now := time.Date(2026, 3, 11, 10, 0, 0, 0, time.UTC)
	account := &Account{
		Extra: map[string]any{
			"codex_primary_used_percent": 20.0,
			"codex_primary_reset_at":     now.Add(-time.Minute).Format(time.RFC3339),
			"codex_usage_updated_at":     now.Add(-time.Minute).Format(time.RFC3339),
		},
	}

	require.Equal(t, openAIQuotaHeadroomNeutralFactor, openAIQuotaHeadroomFactor(account, now))
}

func TestOpenAIQuotaHeadroomFactor_SecondaryLowHeadroomDiscountsPrimary(t *testing.T) {
	now := time.Date(2026, 3, 11, 10, 0, 0, 0, time.UTC)
	account := &Account{
		Extra: map[string]any{
			"codex_primary_used_percent":   20.0,
			"codex_primary_reset_at":       now.Add(24 * time.Hour).Format(time.RFC3339),
			"codex_secondary_used_percent": 95.0,
			"codex_secondary_reset_at":     now.Add(time.Hour).Format(time.RFC3339),
			"codex_usage_updated_at":       now.Add(-time.Minute).Format(time.RFC3339),
		},
	}

	require.InDelta(t, 0.4, openAIQuotaHeadroomFactor(account, now), 0.0001)
}

func TestBuildOpenAIAccountLoadPlan_QuotaHeadroomPrefersHigher7dRemaining(t *testing.T) {
	now := time.Now()
	filtered := []*Account{
		{
			ID:       1,
			Priority: 0,
			Extra: map[string]any{
				"codex_primary_used_percent": 80.0,
				"codex_primary_reset_at":     now.Add(24 * time.Hour).Format(time.RFC3339),
				"codex_usage_updated_at":     now.Add(-time.Minute).Format(time.RFC3339),
			},
		},
		{
			ID:       2,
			Priority: 0,
			Extra: map[string]any{
				"codex_primary_used_percent": 20.0,
				"codex_primary_reset_at":     now.Add(24 * time.Hour).Format(time.RFC3339),
				"codex_usage_updated_at":     now.Add(-time.Minute).Format(time.RFC3339),
			},
		},
	}
	sched := openAIQuotaHeadroomTestScheduler(1.0)

	plan := sched.buildOpenAIAccountLoadPlan(context.Background(), OpenAIAccountScheduleRequest{}, filtered, map[int64]*AccountLoadInfo{})
	scores := openAIPlanScores(plan)
	require.Greater(t, scores[2], scores[1], "7d 剩余额度更高的账号得分应更高")
}

func TestBuildOpenAIAccountLoadPlan_QuotaHeadroomZeroNoEffect(t *testing.T) {
	now := time.Now()
	filtered := []*Account{
		{
			ID:       1,
			Priority: 0,
			Extra: map[string]any{
				"codex_primary_used_percent": 80.0,
				"codex_primary_reset_at":     now.Add(24 * time.Hour).Format(time.RFC3339),
				"codex_usage_updated_at":     now.Add(-time.Minute).Format(time.RFC3339),
			},
		},
		{
			ID:       2,
			Priority: 0,
			Extra: map[string]any{
				"codex_primary_used_percent": 20.0,
				"codex_primary_reset_at":     now.Add(24 * time.Hour).Format(time.RFC3339),
				"codex_usage_updated_at":     now.Add(-time.Minute).Format(time.RFC3339),
			},
		},
	}
	sched := openAIResetTestScheduler(0)

	plan := sched.buildOpenAIAccountLoadPlan(context.Background(), OpenAIAccountScheduleRequest{}, filtered, map[int64]*AccountLoadInfo{})
	scores := openAIPlanScores(plan)
	require.Equal(t, scores[1], scores[2], "quota_headroom 权重为 0 时不应影响打分")
}
