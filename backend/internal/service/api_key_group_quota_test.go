//go:build unit

package service

import (
	"context"
	"errors"
	"math"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

type quotaRateRepo struct {
	UserGroupRateRepository
	rate *float64
	err  error
}

func (r *quotaRateRepo) GetByUserAndGroup(context.Context, int64, int64) (*float64, error) {
	return r.rate, r.err
}

func TestFirstAvailableGroupQuota(t *testing.T) {
	for _, tc := range []struct {
		name       string
		configure  func(*APIKeyService, *APIKey, *User, map[int64]*Group, map[int64]*UserSubscription)
		wantGroup  int64
		wantAmount float64
		wantError  string
	}{
		{name: "first subscribed group before larger wallet", wantGroup: 1, wantAmount: 3},
		{name: "exhausted subscription falls back", wantGroup: 2, wantAmount: 20, configure: func(_ *APIKeyService, _ *APIKey, _ *User, _ map[int64]*Group, subs map[int64]*UserSubscription) {
			subs[1].DailyUsageUSD = 10
		}},
		{name: "missing subscription falls back", wantGroup: 2, wantAmount: 20, configure: func(_ *APIKeyService, _ *APIKey, _ *User, _ map[int64]*Group, subs map[int64]*UserSubscription) {
			delete(subs, 1)
		}},
		{name: "expired subscription falls back", wantGroup: 2, wantAmount: 20, configure: func(_ *APIKeyService, _ *APIKey, _ *User, _ map[int64]*Group, subs map[int64]*UserSubscription) {
			subs[1].ExpiresAt = time.Now().Add(-time.Hour)
		}},
		{name: "inactive subscription falls back", wantGroup: 2, wantAmount: 20, configure: func(_ *APIKeyService, _ *APIKey, _ *User, _ map[int64]*Group, subs map[int64]*UserSubscription) {
			subs[1].Status = SubscriptionStatusExpired
		}},
		{name: "disabled group falls back", wantGroup: 2, wantAmount: 20, configure: func(_ *APIKeyService, _ *APIKey, _ *User, groups map[int64]*Group, _ map[int64]*UserSubscription) {
			groups[1].Status = StatusDisabled
		}},
		{name: "deleted group falls back", wantGroup: 2, wantAmount: 20, configure: func(_ *APIKeyService, _ *APIKey, _ *User, groups map[int64]*Group, _ map[int64]*UserSubscription) {
			delete(groups, 1)
		}},
		{name: "fresh user ban falls back", wantGroup: 2, wantAmount: 20, configure: func(_ *APIKeyService, _ *APIKey, user *User, _ map[int64]*Group, _ map[int64]*UserSubscription) {
			user.BannedGroupIDs = []int64{1}
		}},
		{name: "zero wallet still allows subscription", wantGroup: 1, wantAmount: 3, configure: func(_ *APIKeyService, _ *APIKey, user *User, _ map[int64]*Group, _ map[int64]*UserSubscription) {
			user.Balance = 0
		}},
		{name: "binding order is authoritative", wantGroup: 2, wantAmount: 20, configure: func(_ *APIKeyService, key *APIKey, _ *User, _ map[int64]*Group, _ map[int64]*UserSubscription) {
			key.GroupIDs = []int64{2, 1}
		}},
		{name: "request-selected group cannot reorder bindings", wantGroup: 1, wantAmount: 3, configure: func(_ *APIKeyService, key *APIKey, _ *User, _ map[int64]*Group, _ map[int64]*UserSubscription) {
			key.GroupID = firstAPIKeyGroupID([]int64{2})
		}},
		{name: "independent key cap is not group quota", wantGroup: 1, wantAmount: 3, configure: func(_ *APIKeyService, key *APIKey, _ *User, _ map[int64]*Group, _ map[int64]*UserSubscription) {
			key.Quota, key.QuotaUsed = 1, 0.9
		}},
		{name: "expired key is invalid", wantError: "API_KEY_EXPIRED", configure: func(_ *APIKeyService, key *APIKey, _ *User, _ map[int64]*Group, _ map[int64]*UserSubscription) {
			expired := time.Now().Add(-time.Hour)
			key.ExpiresAt = &expired
		}},
		{name: "exhausted key is invalid", wantError: "API_KEY_QUOTA_EXHAUSTED", configure: func(_ *APIKeyService, key *APIKey, _ *User, _ map[int64]*Group, _ map[int64]*UserSubscription) {
			key.Quota, key.QuotaUsed = 1, 1
		}},
		{name: "inactive fresh user is invalid", wantError: "USER_INACTIVE", configure: func(_ *APIKeyService, _ *APIKey, user *User, _ map[int64]*Group, _ map[int64]*UserSubscription) {
			user.Status = StatusDisabled
		}},
		{name: "all quota exhausted", wantError: "NO_AVAILABLE_GROUP", configure: func(_ *APIKeyService, _ *APIKey, user *User, _ map[int64]*Group, subs map[int64]*UserSubscription) {
			user.Balance = 0
			subs[1].DailyUsageUSD = 10
		}},
		{name: "exclusive wallet permission revoked", wantError: "NO_AVAILABLE_GROUP", configure: func(_ *APIKeyService, key *APIKey, _ *User, groups map[int64]*Group, _ map[int64]*UserSubscription) {
			key.GroupIDs = []int64{2}
			groups[2].IsExclusive = true
		}},
		{name: "legacy single binding", wantGroup: 1, wantAmount: 3, configure: func(_ *APIKeyService, key *APIKey, _ *User, _ map[int64]*Group, _ map[int64]*UserSubscription) {
			key.GroupIDs = nil
		}},
		{name: "unbound key", wantError: "NO_AVAILABLE_GROUP", configure: func(_ *APIKeyService, key *APIKey, _ *User, _ map[int64]*Group, _ map[int64]*UserSubscription) {
			key.GroupID, key.GroupIDs = nil, nil
		}},
		{name: "user rate override", wantGroup: 1, wantAmount: 12, configure: func(s *APIKeyService, _ *APIKey, _ *User, _ map[int64]*Group, _ map[int64]*UserSubscription) {
			rate := 0.5
			s.userGroupRateRepo = &quotaRateRepo{rate: &rate}
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			now := time.Now()
			daily, weekly := 10.0, 100.0
			groups := map[int64]*Group{
				1: {ID: 1, Name: "Subscription", Status: StatusActive, SubscriptionType: SubscriptionTypeSubscription, RateMultiplier: 2, DailyLimitUSD: &daily, WeeklyLimitUSD: &weekly},
				2: {ID: 2, Name: "Wallet", Status: StatusActive, RateMultiplier: 0.5},
			}
			subs := map[int64]*UserSubscription{1: {UserID: 7, GroupID: 1, Status: SubscriptionStatusActive, ExpiresAt: now.Add(time.Hour), DailyWindowStart: &now, WeeklyWindowStart: &now, DailyUsageUSD: 4, WeeklyUsageUSD: 20}}
			user := &User{ID: 7, Status: StatusActive, Balance: 10}
			svc := &APIKeyService{userRepo: &visibilityUserRepo{user: user}, groupRepo: &orderedGroupsRepo{groups: groups}, userSubRepo: &orderedSubscriptions{subs: subs}}
			key := &APIKey{UserID: 7, Status: StatusActive, GroupID: firstAPIKeyGroupID([]int64{1}), GroupIDs: []int64{1, 2}, User: &User{ID: 7, Balance: 999}}
			if tc.configure != nil {
				tc.configure(svc, key, user, groups, subs)
			}
			got, err := svc.GetFirstAvailableGroupQuota(context.Background(), key)
			require.NoError(t, err)
			require.Equal(t, "USD", got.Unit)
			if tc.wantError != "" {
				require.False(t, got.IsValid)
				require.NotNil(t, got.Error)
				require.Equal(t, tc.wantError, got.Error.Code)
				require.NotEmpty(t, got.InvalidMessage)
				require.Zero(t, got.Balance)
				return
			}
			require.True(t, got.IsValid)
			require.Nil(t, got.Error)
			require.Equal(t, tc.wantGroup, got.GroupID)
			require.InDelta(t, tc.wantAmount, got.Balance, 1e-9)
			require.Equal(t, got.Balance, got.Remaining)
			require.Equal(t, groups[tc.wantGroup].Name, got.PlanName)
			if tc.wantGroup == 1 {
				require.Equal(t, "daily", got.Window)
				require.NotNil(t, got.Total)
				require.NotNil(t, got.Used)
				require.InDelta(t, got.Balance, *got.Total-*got.Used, 1e-9)
			}
		})
	}
}

func TestFirstAvailableGroupQuotaWindowRolloverAndRecovery(t *testing.T) {
	now, old := time.Now(), time.Now().Add(-48*time.Hour)
	limit, monthly := 10.0, 30.0
	group := &Group{ID: 1, Status: StatusActive, SubscriptionType: SubscriptionTypeSubscription, RateMultiplier: 1, DailyLimitUSD: &limit, MonthlyLimitUSD: &monthly}
	sub := &UserSubscription{UserID: 7, GroupID: 1, Status: SubscriptionStatusActive, ExpiresAt: now.Add(time.Hour), DailyWindowStart: &old, MonthlyWindowStart: &now, DailyUsageUSD: 10, MonthlyUsageUSD: 26}
	svc := &APIKeyService{userRepo: &visibilityUserRepo{user: &User{ID: 7, Status: StatusActive}}, groupRepo: &orderedGroupsRepo{groups: map[int64]*Group{1: group}}, userSubRepo: &orderedSubscriptions{subs: map[int64]*UserSubscription{1: sub}}}
	key := &APIKey{UserID: 7, GroupIDs: []int64{1}}
	got, err := svc.GetFirstAvailableGroupQuota(context.Background(), key)
	require.NoError(t, err)
	require.Equal(t, 4.0, got.Balance)
	require.Equal(t, "monthly", got.Window)
	require.Equal(t, 10.0, sub.DailyUsageUSD, "query must not mutate repository or cached subscriptions")
	require.Equal(t, &old, sub.DailyWindowStart)
	require.Nil(t, sub.Group)
	sub.MonthlyUsageUSD = 30
	got, err = svc.GetFirstAvailableGroupQuota(context.Background(), key)
	require.NoError(t, err)
	require.False(t, got.IsValid)
	sub.MonthlyUsageUSD = 0
	got, err = svc.GetFirstAvailableGroupQuota(context.Background(), key)
	require.NoError(t, err)
	require.True(t, got.IsValid, "earlier group regains priority as quota recovers")
	require.Equal(t, 10.0, got.Balance)
}

func TestFirstAvailableGroupQuotaUnlimitedAndErrors(t *testing.T) {
	for _, tc := range []struct {
		name         string
		rate         float64
		subscription bool
		failure      error
		wantError    bool
	}{
		{name: "free wallet group", rate: 0},
		{name: "unlimited subscription", rate: 1, subscription: true},
		{name: "negative rate", rate: -1, wantError: true},
		{name: "invalid rate", rate: math.NaN(), wantError: true},
		{name: "rate lookup unavailable", rate: 1, failure: errors.New("database unavailable"), wantError: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			group := &Group{ID: 1, Status: StatusActive, RateMultiplier: tc.rate}
			if tc.subscription {
				group.SubscriptionType = SubscriptionTypeSubscription
			}
			svc := &APIKeyService{
				userRepo:          &visibilityUserRepo{user: &User{ID: 7, Status: StatusActive, Balance: 10}},
				groupRepo:         &orderedGroupsRepo{groups: map[int64]*Group{1: group}},
				userSubRepo:       &orderedSubscriptions{subs: map[int64]*UserSubscription{1: {UserID: 7, GroupID: 1, Status: SubscriptionStatusActive, ExpiresAt: time.Now().Add(time.Hour)}}},
				userGroupRateRepo: &quotaRateRepo{err: tc.failure},
			}
			got, err := svc.GetFirstAvailableGroupQuota(context.Background(), &APIKey{UserID: 7, GroupIDs: []int64{1}})
			if tc.wantError {
				require.Error(t, err)
				require.Nil(t, got)
				return
			}
			require.NoError(t, err)
			require.True(t, got.IsValid)
			require.True(t, got.Unlimited)
			require.Equal(t, -1.0, got.Balance)
			require.Nil(t, got.Total)
			require.Nil(t, got.Used)
		})
	}
}
