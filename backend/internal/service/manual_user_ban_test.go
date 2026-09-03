package service

import (
	"context"
	"testing"
	"time"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/stretchr/testify/require"
)

type manualBanUserRepo struct {
	UserRepository
	user          *User
	updateCalls   int
	updateFields  []UserUpdateFields
	bannedGroupID int64
	bannedUntil   *time.Time
	unbannedID    int64
}

func (r *manualBanUserRepo) GetByID(_ context.Context, _ int64) (*User, error) {
	clone := *r.user
	clone.BannedGroupIDs = append([]int64(nil), r.user.BannedGroupIDs...)
	if r.user.BannedGroupExpirations != nil {
		clone.BannedGroupExpirations = make(map[int64]time.Time, len(r.user.BannedGroupExpirations))
		for groupID, expiresAt := range r.user.BannedGroupExpirations {
			clone.BannedGroupExpirations[groupID] = expiresAt
		}
	}
	return &clone, nil
}

func (r *manualBanUserRepo) Update(_ context.Context, user *User, fields UserUpdateFields) error {
	r.updateCalls++
	r.updateFields = append(r.updateFields, fields)
	r.user.Status = user.Status
	r.user.DisabledUntil = user.DisabledUntil
	return nil
}

func (r *manualBanUserRepo) BanGroupForUser(_ context.Context, _, groupID int64, expiresAt *time.Time) error {
	r.bannedGroupID = groupID
	r.bannedUntil = expiresAt
	return nil
}

func (r *manualBanUserRepo) UnbanGroupForUser(_ context.Context, _, groupID int64) error {
	r.unbannedID = groupID
	return nil
}

func (r *manualBanUserRepo) ListBannedGroupIDs(context.Context, int64) ([]int64, map[int64]time.Time, error) {
	return nil, nil, nil
}

type manualBanAuthCache struct {
	APIKeyAuthCacheInvalidator
	invalidatedUserIDs []int64
}

func (c *manualBanAuthCache) InvalidateAuthCacheByUserID(_ context.Context, userID int64) {
	c.invalidatedUserIDs = append(c.invalidatedUserIDs, userID)
}

func TestAdminBanUserSupportsPermanentAndTemporaryModes(t *testing.T) {
	tests := []struct {
		name          string
		durationHours int
		wantExpiry    bool
	}{
		{name: "permanent", durationHours: 0, wantExpiry: false},
		{name: "temporary", durationHours: 48, wantExpiry: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := &manualBanUserRepo{user: &User{ID: 7, Role: RoleUser, Status: StatusActive}}
			cache := &manualBanAuthCache{}
			svc := NewUserService(repo, nil, cache, nil)
			startedAt := time.Now().UTC()

			user, err := svc.AdminBanUser(context.Background(), 7, tt.durationHours)

			require.NoError(t, err)
			require.Equal(t, StatusDisabled, user.Status)
			require.Equal(t, []UserUpdateFields{{Status: true, DisabledUntil: true}}, repo.updateFields)
			require.Equal(t, []int64{7}, cache.invalidatedUserIDs)
			if tt.wantExpiry {
				require.NotNil(t, user.DisabledUntil)
				require.WithinDuration(t, startedAt.Add(48*time.Hour), *user.DisabledUntil, 2*time.Second)
			} else {
				require.Nil(t, user.DisabledUntil)
			}
		})
	}
}

func TestAdminBanUserRejectsAdminAndInvalidDuration(t *testing.T) {
	t.Run("admin", func(t *testing.T) {
		repo := &manualBanUserRepo{user: &User{ID: 1, Role: RoleAdmin, Status: StatusActive}}
		_, err := NewUserService(repo, nil, nil, nil).AdminBanUser(context.Background(), 1, 0)
		require.Error(t, err)
		require.Equal(t, "ADMIN_BAN_FORBIDDEN", infraerrors.Reason(err))
		require.Zero(t, repo.updateCalls)
	})

	t.Run("duration", func(t *testing.T) {
		repo := &manualBanUserRepo{user: &User{ID: 7, Role: RoleUser, Status: StatusActive}}
		_, err := NewUserService(repo, nil, nil, nil).AdminBanUser(context.Background(), 7, MaxManualBanDurationHours+1)
		require.Error(t, err)
		require.Equal(t, "INVALID_BAN_DURATION", infraerrors.Reason(err))
		require.Zero(t, repo.updateCalls)
	})
}

func TestAdminUnbanUserClearsStatusAndExpiry(t *testing.T) {
	expiresAt := time.Now().Add(time.Hour)
	repo := &manualBanUserRepo{user: &User{ID: 7, Role: RoleUser, Status: StatusDisabled, DisabledUntil: &expiresAt}}

	user, err := NewUserService(repo, nil, nil, nil).AdminUnbanUser(context.Background(), 7)

	require.NoError(t, err)
	require.Equal(t, StatusActive, user.Status)
	require.Nil(t, user.DisabledUntil)
	require.Equal(t, []UserUpdateFields{{Status: true, DisabledUntil: true}}, repo.updateFields)
}

func TestAdminGroupBanSupportsExpiryAndScopedUnban(t *testing.T) {
	repo := &manualBanUserRepo{
		user: &User{
			ID:                     7,
			Role:                   RoleUser,
			Status:                 StatusActive,
			BannedGroupIDs:         []int64{3},
			BannedGroupExpirations: map[int64]time.Time{},
		},
	}
	cache := &manualBanAuthCache{}
	svc := NewUserService(repo, nil, cache, nil)

	user, err := svc.AdminBanGroupForUser(context.Background(), 7, 9, 24)
	require.NoError(t, err)
	require.Equal(t, int64(9), repo.bannedGroupID)
	require.NotNil(t, repo.bannedUntil)
	require.ElementsMatch(t, []int64{3, 9}, user.BannedGroupIDs)
	require.True(t, user.IsGroupBanned(9))

	user, err = svc.AdminUnbanGroupForUser(context.Background(), 7, 3)
	require.NoError(t, err)
	require.Equal(t, int64(3), repo.unbannedID)
	require.NotContains(t, user.BannedGroupIDs, int64(3))
	require.Equal(t, []int64{7, 7}, cache.invalidatedUserIDs)
}
