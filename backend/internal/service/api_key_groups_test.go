//go:build unit

package service

import (
	"context"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/ctxkey"
	"github.com/stretchr/testify/require"
)

type orderedGroupsRepo struct {
	GroupRepository
	groups map[int64]*Group
}

func (r *orderedGroupsRepo) GetByID(_ context.Context, id int64) (*Group, error) {
	if group := r.groups[id]; group != nil {
		return group, nil
	}
	return nil, ErrGroupNotFound
}

type orderedGroupAccounts struct {
	accounts map[int64][]Account
	visited  []int64
}

func (s *orderedGroupAccounts) ListSchedulableAccounts(_ context.Context, groupID *int64, _ string, _ bool) ([]Account, bool, error) {
	s.visited = append(s.visited, *groupID)
	return s.accounts[*groupID], false, nil
}

func orderedGroupsService() *APIKeyService {
	return &APIKeyService{
		cfg: &config.Config{},
		groupRepo: &orderedGroupsRepo{groups: map[int64]*Group{
			1: {ID: 1, Status: StatusActive, Platform: PlatformOpenAI},
			2: {ID: 2, Status: StatusActive, Platform: PlatformOpenAI},
			3: {ID: 3, Status: StatusActive, Platform: PlatformOpenAI, IsExclusive: true},
		}},
	}
}

func TestAPIKeyOrderedGroupsValidation(t *testing.T) {
	svc := orderedGroupsService()
	user := &User{ID: 7}
	ids, err := svc.validateOrderedGroups(context.Background(), user, nil, []int64{2, 1})
	require.NoError(t, err)
	require.Equal(t, []int64{2, 1}, ids)
	for _, invalid := range [][]int64{{1, 1}, {0}, {-1}, {1, 3}, make([]int64, MaxAPIKeyGroups+1)} {
		_, err := svc.validateOrderedGroups(context.Background(), user, nil, invalid)
		require.Error(t, err)
	}
	legacyID := int64(1)
	_, err = svc.validateOrderedGroups(context.Background(), user, &legacyID, []int64{2, 1})
	require.Error(t, err)
	ids, err = svc.validateOrderedGroups(context.Background(), user, &legacyID, nil)
	require.NoError(t, err)
	require.Equal(t, []int64{1}, ids)
}

func TestAPIKeyOrderedGroupsCreateUpdateAndClear(t *testing.T) {
	svc := orderedGroupsService()
	repo := &securityAPIKeyRepo{key: &APIKey{ID: 10, UserID: 7, Key: "sk-test", Status: StatusActive, GroupID: firstAPIKeyGroupID([]int64{1}), GroupIDs: []int64{1}}}
	svc.apiKeyRepo = repo
	svc.userRepo = &visibilityUserRepo{user: &User{ID: 7}}
	created, err := svc.Create(context.Background(), 7, CreateAPIKeyRequest{Name: "ordered", GroupIDs: []int64{2, 1}})
	require.NoError(t, err)
	require.Equal(t, []int64{2, 1}, created.GroupIDs)
	require.EqualValues(t, 2, *created.GroupID)
	ids := []int64{2, 1}
	updated, err := svc.Update(context.Background(), 10, 7, UpdateAPIKeyRequest{GroupIDs: &ids})
	require.NoError(t, err)
	require.Equal(t, ids, updated.GroupIDs)
	require.EqualValues(t, 2, *updated.GroupID)
	ids = []int64{}
	updated, err = svc.Update(context.Background(), 10, 7, UpdateAPIKeyRequest{GroupIDs: &ids})
	require.NoError(t, err)
	require.Empty(t, updated.GroupIDs)
	require.Nil(t, updated.GroupID)
	updated, err = svc.Update(context.Background(), 10, 7, UpdateAPIKeyRequest{})
	require.NoError(t, err)
	require.Equal(t, []int64{1}, updated.GroupIDs)
	require.Equal(t, 4, repo.writes)
	ids = []int64{1, 3}
	_, err = svc.Update(context.Background(), 10, 7, UpdateAPIKeyRequest{GroupIDs: &ids})
	require.ErrorIs(t, err, ErrGroupNotAllowed)
	require.Equal(t, 4, repo.writes)
}

func TestAPIKeyOrderedGroupsRouting(t *testing.T) {
	for _, tc := range []struct {
		name      string
		configure func(*APIKeyService, *APIKey, *orderedGroupAccounts)
		want      int64
	}{
		{name: "priority", want: 1},
		{name: "no accounts", want: 2, configure: func(_ *APIKeyService, _ *APIKey, source *orderedGroupAccounts) { delete(source.accounts, 1) }},
		{name: "rate limited", want: 2, configure: func(_ *APIKeyService, _ *APIKey, source *orderedGroupAccounts) {
			future := time.Now().Add(time.Hour)
			source.accounts[1][0].RateLimitResetAt = &future
		}},
		{name: "disabled group", want: 2, configure: func(s *APIKeyService, _ *APIKey, _ *orderedGroupAccounts) {
			s.groupRepo.(*orderedGroupsRepo).groups[1].Status = StatusDisabled
		}},
		{name: "deleted group", want: 2, configure: func(s *APIKeyService, _ *APIKey, _ *orderedGroupAccounts) {
			delete(s.groupRepo.(*orderedGroupsRepo).groups, 1)
		}},
		{name: "banned", want: 2, configure: func(_ *APIKeyService, key *APIKey, _ *orderedGroupAccounts) { key.User.BannedGroupIDs = []int64{1} }},
		{name: "revoked access", want: 2, configure: func(s *APIKeyService, _ *APIKey, _ *orderedGroupAccounts) {
			s.groupRepo.(*orderedGroupsRepo).groups[1].IsExclusive = true
		}},
		{name: "model unavailable", want: 2, configure: func(s *APIKeyService, _ *APIKey, _ *orderedGroupAccounts) {
			s.groupRepo.(*orderedGroupsRepo).groups[1].ModelAllowlist = GroupModelAllowlist{Enabled: true, Models: []string{"other-model"}}
		}},
		{name: "all unavailable", want: 0, configure: func(_ *APIKeyService, _ *APIKey, source *orderedGroupAccounts) { source.accounts = nil }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			svc := orderedGroupsService()
			key := &APIKey{GroupID: firstAPIKeyGroupID([]int64{1}), GroupIDs: []int64{1, 2}, User: &User{ID: 7, Balance: 10}}
			source := &orderedGroupAccounts{accounts: map[int64][]Account{
				1: {{ID: 11, Status: StatusActive, Schedulable: true, Platform: PlatformOpenAI, Type: AccountTypeAPIKey}},
				2: {{ID: 22, Status: StatusActive, Schedulable: true, Platform: PlatformOpenAI, Type: AccountTypeAPIKey}},
			}}
			svc.groupAccounts = source
			if tc.configure != nil {
				tc.configure(svc, key, source)
			}
			got, err := svc.SelectOrderedGroup(context.Background(), key, APIKeyGroupSelection{Models: []string{"gpt-test"}, CheckCapacity: true, CheckBilling: true}, nil)
			if tc.want == 0 {
				require.ErrorIs(t, err, ErrAPIKeyGroupsUnavailable)
				return
			}
			require.NoError(t, err)
			require.Equal(t, tc.want, *got.GroupID)
			require.Equal(t, tc.want, got.Group.ID)
			require.EqualValues(t, 1, *key.GroupID, "cached binding must not change")
			require.Nil(t, key.Group)
			if tc.want == 1 {
				require.Equal(t, []int64{1}, source.visited)
			}
		})
	}
}

func TestAPIKeyOrderedGroupsAuthCache(t *testing.T) {
	svc := &APIKeyService{}
	key := &APIKey{ID: 9, GroupID: firstAPIKeyGroupID([]int64{2}), GroupIDs: []int64{2, 1}, User: &User{ID: 7}}
	snapshot := svc.snapshotFromAPIKey(context.Background(), key)
	restored := svc.snapshotToAPIKey("test", snapshot)
	require.Equal(t, []int64{2, 1}, restored.GroupIDs)
	restored.GroupIDs[0] = 99
	require.Equal(t, []int64{2, 1}, snapshot.GroupIDs)
}

type orderedSubscriptions struct {
	UserSubscriptionRepository
	subs map[int64]*UserSubscription
}

func (r *orderedSubscriptions) GetActiveByUserIDAndGroupID(_ context.Context, _, groupID int64) (*UserSubscription, error) {
	if sub := r.subs[groupID]; sub != nil {
		return sub, nil
	}
	return nil, ErrSubscriptionNotFound
}

func TestAPIKeyOrderedGroupsSubscriptionFallback(t *testing.T) {
	svc := orderedGroupsService()
	groups := svc.groupRepo.(*orderedGroupsRepo).groups
	limit := 1.0
	groups[1].SubscriptionType, groups[2].SubscriptionType = SubscriptionTypeSubscription, SubscriptionTypeSubscription
	groups[1].DailyLimitUSD, groups[2].DailyLimitUSD = &limit, &limit
	now := time.Now()
	subs := &orderedSubscriptions{subs: map[int64]*UserSubscription{
		1: {ID: 11, UserID: 7, GroupID: 1, Status: SubscriptionStatusActive, ExpiresAt: now.Add(time.Hour), DailyWindowStart: &now, WeeklyWindowStart: &now, MonthlyWindowStart: &now, DailyUsageUSD: 1},
		2: {ID: 22, UserID: 7, GroupID: 2, Status: SubscriptionStatusActive, ExpiresAt: now.Add(time.Hour), DailyWindowStart: &now, WeeklyWindowStart: &now, MonthlyWindowStart: &now},
	}}
	subscriptions := &SubscriptionService{userSubRepo: subs, now: func() time.Time { return now }}
	key := &APIKey{GroupID: firstAPIKeyGroupID([]int64{1}), GroupIDs: []int64{1, 2}, User: &User{ID: 7}}
	selection := APIKeyGroupSelection{CheckBilling: true}
	got, err := svc.SelectOrderedGroup(context.Background(), key, selection, subscriptions)
	require.NoError(t, err)
	require.EqualValues(t, 2, *got.GroupID, "subscription limits must be checked before choosing a group")
	subs.subs[1].DailyUsageUSD = 0
	got, err = svc.SelectOrderedGroup(context.Background(), key, selection, subscriptions)
	require.NoError(t, err)
	require.EqualValues(t, 1, *got.GroupID, "earlier groups regain priority when available")
	groups[1].SubscriptionType = SubscriptionTypeStandard
	got, err = svc.SelectOrderedGroup(context.Background(), key, selection, subscriptions)
	require.NoError(t, err)
	require.EqualValues(t, 2, *got.GroupID, "zero balance must still allow an active subscribed group")
}

type orderedGroupPermissions struct{ UserGroupRateRepository }

func (r *orderedGroupPermissions) GetDeniedModelsByUserAndGroup(_ context.Context, _, groupID int64) ([]string, error) {
	if groupID == 1 {
		return []string{"gpt-test"}, nil
	}
	return []string{"other-model"}, nil
}

func (r *orderedGroupPermissions) GetRPMOverrideByUserAndGroup(_ context.Context, _, groupID int64) (*int, error) {
	limit := int(groupID) * 10
	return &limit, nil
}

func TestAPIKeyOrderedGroupsLoadsSelectedPermissions(t *testing.T) {
	svc := orderedGroupsService()
	svc.userGroupRateRepo = &orderedGroupPermissions{}
	primaryLimit := 99
	key := &APIKey{UserID: 7, GroupID: firstAPIKeyGroupID([]int64{1}), GroupIDs: []int64{1, 2},
		User: &User{ID: 7, UserGroupRPMOverride: &primaryLimit, UserGroupDeniedModels: []string{"original-model"}}}
	got, err := svc.SelectOrderedGroup(context.Background(), key, APIKeyGroupSelection{Models: []string{"gpt-test"}}, nil)
	require.NoError(t, err)
	require.EqualValues(t, 2, *got.GroupID)
	require.Equal(t, 20, *got.User.UserGroupRPMOverride)
	require.Equal(t, []string{"other-model"}, got.DeniedModelsInGroup())
	require.Equal(t, 99, *key.User.UserGroupRPMOverride)
	require.Equal(t, []string{"original-model"}, key.User.UserGroupDeniedModels)
}

func TestAPIKeyOrderedGroupsPreservesForcedPlatformScope(t *testing.T) {
	svc := orderedGroupsService()
	key := &APIKey{GroupID: firstAPIKeyGroupID([]int64{1}), GroupIDs: []int64{1, 2}, User: &User{ID: 7}}
	ctx := context.WithValue(context.Background(), ctxkey.ForcePlatform, PlatformAntigravity)
	got, err := svc.SelectOrderedGroup(ctx, key, APIKeyGroupSelection{GoogleOnly: true}, nil)
	require.NoError(t, err)
	require.EqualValues(t, 1, *got.GroupID)
}
