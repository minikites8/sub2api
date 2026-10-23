package service

import (
	"context"
	"errors"
	"math"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestSubscriptionUsageSnapshotPeriods(t *testing.T) {
	now := time.Date(2026, 10, 23, 12, 0, 0, 0, time.UTC)
	start := now.Add(-time.Hour)
	daily, weekly, monthly := 90.0, 500.0, 1300.0
	group := &Group{DailyLimitUSD: &daily, WeeklyLimitUSD: &weekly, MonthlyLimitUSD: &monthly}
	sub := &UserSubscription{Status: SubscriptionStatusActive, StartsAt: start, ExpiresAt: now.AddDate(0, 3, 0), DailyWindowStart: &start, WeeklyWindowStart: &start, MonthlyWindowStart: &start, DailyUsageUSD: 32, WeeklyUsageUSD: 285, MonthlyUsageUSD: 720}
	got, err := SnapshotSubscriptionUsage(group, sub, now)
	require.NoError(t, err)
	require.Equal(t, 58.0, *got.DailyRemainingUSD)
	require.Equal(t, 215.0, *got.WeeklyRemainingUSD)
	require.Equal(t, 580.0, *got.MonthlyRemainingUSD)
	require.InDelta(t, 32.0/90*100, *got.DailyPercentage, 0.0001)
	require.NotNil(t, got.DailyResetAt)
	require.Equal(t, sub.WeeklyResetTime(), got.WeeklyResetAt)
	require.Equal(t, sub.MonthlyResetTime(), got.MonthlyResetAt)
	sub.DailyUsageUSD = 100
	got, err = SnapshotSubscriptionUsage(group, sub, now)
	require.NoError(t, err)
	require.Zero(t, *got.DailyRemainingUSD)
	require.Equal(t, 100.0, *got.DailyPercentage)
	require.Equal(t, 215.0, *got.WeeklyRemainingUSD)
	require.False(t, got.Unlimited)
}

func TestSubscriptionUsageSnapshotRolloverIsReadOnly(t *testing.T) {
	now := time.Date(2026, 10, 23, 12, 0, 0, 0, time.UTC)
	old := now.AddDate(0, 0, -32)
	limit := 90.0
	group := &Group{DailyLimitUSD: &limit, WeeklyLimitUSD: &limit, MonthlyLimitUSD: &limit}
	sub := &UserSubscription{Status: SubscriptionStatusActive, StartsAt: old, ExpiresAt: now.AddDate(0, 2, 0), DailyWindowStart: &old, WeeklyWindowStart: &old, MonthlyWindowStart: &old, DailyUsageUSD: 90, WeeklyUsageUSD: 90, MonthlyUsageUSD: 90}
	got, err := SnapshotSubscriptionUsage(group, sub, now)
	require.NoError(t, err)
	require.Zero(t, got.DailyUsageUSD)
	require.Zero(t, got.WeeklyUsageUSD)
	require.Zero(t, got.MonthlyUsageUSD)
	require.Nil(t, got.DailyResetAt, "do not invent an anchor before persistence/activation")
	require.Nil(t, got.WeeklyResetAt)
	require.Nil(t, got.MonthlyResetAt)
	require.Equal(t, 90.0, sub.DailyUsageUSD)
	require.Equal(t, &old, sub.DailyWindowStart)
}

func TestSubscriptionUsageSnapshotUnlimitedAndDailyCard(t *testing.T) {
	now := time.Now()
	start := now.Add(-12 * time.Hour)
	sub := &UserSubscription{Status: SubscriptionStatusActive, StartsAt: start, ExpiresAt: start.Add(24 * time.Hour), DailyWindowStart: &start, DailyUsageUSD: 3}
	group := &Group{}
	got, err := SnapshotSubscriptionUsage(group, sub, now)
	require.NoError(t, err)
	require.True(t, got.Unlimited)
	require.Nil(t, got.DailyRemainingUSD)
	limit := 10.0
	group.DailyLimitUSD = &limit
	got, err = SnapshotSubscriptionUsage(group, sub, now)
	require.NoError(t, err)
	require.Equal(t, 3.0, got.DailyUsageUSD, "one-time daily cards do not renew at midnight")
	require.Nil(t, got.DailyResetAt, "expiry is not a quota renewal")
	sub.ExpiresAt = now.Add(-time.Second)
	_, err = SnapshotSubscriptionUsage(group, sub, now)
	require.ErrorIs(t, err, ErrSubscriptionNotFound)
}

type usageSnapshotSubRepo struct {
	UserSubscriptionRepository
	sub             *UserSubscription
	err             error
	userID, groupID int64
}

func (r *usageSnapshotSubRepo) GetActiveByUserIDAndGroupID(_ context.Context, userID, groupID int64) (*UserSubscription, error) {
	r.userID, r.groupID = userID, groupID
	return r.sub, r.err
}

func TestAPIKeyUsageSubscriptionScope(t *testing.T) {
	group := &Group{ID: 11, SubscriptionType: SubscriptionTypeSubscription}
	key := &APIKey{UserID: 7, Group: group}
	repo := &usageSnapshotSubRepo{sub: &UserSubscription{UserID: 7, GroupID: 11, Status: SubscriptionStatusActive, ExpiresAt: time.Now().Add(time.Hour)}}
	svc := &APIKeyService{userSubRepo: repo}
	_, err := svc.GetUsageSubscriptionSnapshot(context.Background(), key)
	require.NoError(t, err)
	require.Equal(t, int64(7), repo.userID)
	require.Equal(t, int64(11), repo.groupID)
	repo.sub.UserID = 8
	_, err = svc.GetUsageSubscriptionSnapshot(context.Background(), key)
	require.ErrorIs(t, err, ErrSubscriptionNotFound)
	repo.err = errors.New("database unavailable")
	_, err = svc.GetUsageSubscriptionSnapshot(context.Background(), key)
	require.ErrorContains(t, err, "database unavailable")
}

type usageSnapshotKeyRepo struct {
	APIKeyRepository
	key *APIKey
}

func (r *usageSnapshotKeyRepo) GetByID(context.Context, int64) (*APIKey, error) { return r.key, nil }

func TestRefreshUsageKeyUsesPersistedCountersAndOwner(t *testing.T) {
	key := &APIKey{ID: 99, UserID: 7, Status: StatusAPIKeyActive, Quota: 100, QuotaUsed: 1}
	repo := &usageSnapshotKeyRepo{key: &APIKey{ID: 99, UserID: 7, Status: StatusAPIKeyActive, Quota: 100, QuotaUsed: 20, RateLimit5h: 10}}
	svc := &APIKeyService{apiKeyRepo: repo}
	got, err := svc.RefreshUsageKey(context.Background(), key)
	require.NoError(t, err)
	require.Equal(t, 20.0, got.QuotaUsed)
	require.Equal(t, 10.0, got.RateLimit5h)
	require.Equal(t, 1.0, key.QuotaUsed, "do not mutate auth cache snapshot")
	repo.key.UserID = 8
	_, err = svc.RefreshUsageKey(context.Background(), key)
	require.ErrorIs(t, err, ErrAPIKeyNotFound)
	repo.key = nil
	_, err = svc.RefreshUsageKey(context.Background(), key)
	require.ErrorIs(t, err, ErrAPIKeyNotFound)
}

func TestSubscriptionUsageSnapshotRejectsInvalidNumbers(t *testing.T) {
	for _, value := range []float64{math.NaN(), math.Inf(1), -1} {
		_, _, err := subscriptionUsageAmounts(&value, value)
		require.Error(t, err)
	}
	limit := 90.0
	remaining, percent, err := subscriptionUsageAmounts(&limit, 0)
	require.NoError(t, err)
	require.Equal(t, limit, *remaining)
	require.Zero(t, *percent)
}
