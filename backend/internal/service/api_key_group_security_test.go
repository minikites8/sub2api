//go:build unit

package service

import (
	"context"
	"errors"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
)

type securityGroupRepo struct {
	GroupRepository
	group *Group
}

func (r *securityGroupRepo) GetByID(context.Context, int64) (*Group, error) { return r.group, nil }

type securitySubscriptionRepo struct {
	UserSubscriptionRepository
	active bool
}

func (r *securitySubscriptionRepo) GetActiveByUserIDAndGroupID(context.Context, int64, int64) (*UserSubscription, error) {
	if r.active {
		return &UserSubscription{}, nil
	}
	return nil, errors.New("subscription unavailable")
}

type securityAPIKeyRepo struct {
	APIKeyRepository
	key    *APIKey
	writes int
}

func (r *securityAPIKeyRepo) GetByID(context.Context, int64) (*APIKey, error) {
	copy := *r.key
	return &copy, nil
}

func (r *securityAPIKeyRepo) Create(_ context.Context, key *APIKey) error {
	r.writes++
	return nil
}

func (r *securityAPIKeyRepo) Update(context.Context, *APIKey, APIKeyUpdateFields) error {
	r.writes++
	return nil
}

func TestAPIKeyGroupAuthorizationCreateAndUpdate(t *testing.T) {
	for _, tc := range []struct {
		name                string
		user                User
		group               Group
		subscribed, allowed bool
	}{
		{name: "public", allowed: true},
		{name: "exclusive", group: Group{IsExclusive: true}},
		{name: "exclusive authorized", user: User{AllowedGroups: []int64{29}}, group: Group{IsExclusive: true}, allowed: true},
		{name: "public restricted", user: User{RestrictPublicGroups: true}},
		{name: "banned public", user: User{BannedGroupIDs: []int64{29}}},
		{name: "subscription required", group: Group{SubscriptionType: "subscription"}},
		{name: "active subscription", group: Group{SubscriptionType: "subscription"}, subscribed: true, allowed: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			tc.user.ID = 7
			tc.user.Balance = 12
			tc.group.ID = 29
			repo := &securityAPIKeyRepo{key: &APIKey{ID: 10, UserID: 7, Key: "sk-test", Status: StatusActive}}
			svc := &APIKeyService{
				apiKeyRepo: repo, userRepo: &visibilityUserRepo{user: &tc.user}, cfg: &config.Config{},
				groupRepo: &securityGroupRepo{group: &tc.group}, userSubRepo: &securitySubscriptionRepo{active: tc.subscribed},
			}
			_, createErr := svc.Create(context.Background(), 7, CreateAPIKeyRequest{Name: "probe", GroupID: &tc.group.ID, Quota: 650})
			quota := 650.0
			_, updateErr := svc.Update(context.Background(), 10, 7, UpdateAPIKeyRequest{GroupID: &tc.group.ID, Quota: &quota})
			if tc.allowed {
				require.NoError(t, createErr)
				require.NoError(t, updateErr)
				require.Equal(t, 2, repo.writes)
			} else {
				require.ErrorIs(t, createErr, ErrGroupNotAllowed)
				require.ErrorIs(t, updateErr, ErrGroupNotAllowed)
				require.Zero(t, repo.writes)
			}
			require.Equal(t, 12.0, tc.user.Balance)
			_, err := svc.Update(context.Background(), 10, 8, UpdateAPIKeyRequest{Quota: &quota})
			require.ErrorIs(t, err, ErrInsufficientPerms)
		})
	}
}
