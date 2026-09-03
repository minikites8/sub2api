package service

import (
	"context"
	"fmt"
	"time"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
)

const MaxManualBanDurationHours = 24 * 365

// manualBanExpiry converts the admin API duration convention into a timestamp.
// Zero means permanent; positive values are measured in whole hours.
func manualBanExpiry(now time.Time, durationHours int) (*time.Time, error) {
	if durationHours < 0 || durationHours > MaxManualBanDurationHours {
		return nil, infraerrors.BadRequest(
			"INVALID_BAN_DURATION",
			fmt.Sprintf("duration_hours must be between 0 and %d", MaxManualBanDurationHours),
		)
	}
	if durationHours == 0 {
		return nil, nil
	}
	expiresAt := now.UTC().Add(time.Duration(durationHours) * time.Hour)
	return &expiresAt, nil
}

func validateManualBanTarget(user *User) error {
	if user == nil {
		return ErrUserNotFound
	}
	if user.IsAdmin() {
		return infraerrors.BadRequest("ADMIN_BAN_FORBIDDEN", "admin users cannot be banned")
	}
	return nil
}

// AdminBanUser applies a permanent or timed account-level ban.
func (s *UserService) AdminBanUser(ctx context.Context, userID int64, durationHours int) (*User, error) {
	if userID <= 0 {
		return nil, infraerrors.BadRequest("INVALID_USER_ID", "user ID is invalid")
	}
	expiresAt, err := manualBanExpiry(time.Now(), durationHours)
	if err != nil {
		return nil, err
	}
	user, err := s.userRepo.GetByID(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("get user for manual ban: %w", err)
	}
	if err := validateManualBanTarget(user); err != nil {
		return nil, err
	}

	user.Status = StatusDisabled
	user.DisabledUntil = expiresAt
	if err := s.userRepo.Update(ctx, user, UserUpdateFields{Status: true, DisabledUntil: true}); err != nil {
		return nil, fmt.Errorf("apply manual user ban: %w", err)
	}
	s.invalidateManualBanAuthCache(ctx, userID)
	return user, nil
}

// AdminUnbanUser restores account-level access and clears any timed expiry.
func (s *UserService) AdminUnbanUser(ctx context.Context, userID int64) (*User, error) {
	if userID <= 0 {
		return nil, infraerrors.BadRequest("INVALID_USER_ID", "user ID is invalid")
	}
	user, err := s.userRepo.GetByID(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("get user for manual unban: %w", err)
	}
	if user.Status != StatusActive || user.DisabledUntil != nil {
		user.Status = StatusActive
		user.DisabledUntil = nil
		if err := s.userRepo.Update(ctx, user, UserUpdateFields{Status: true, DisabledUntil: true}); err != nil {
			return nil, fmt.Errorf("remove manual user ban: %w", err)
		}
	}
	s.invalidateManualBanAuthCache(ctx, userID)
	return user, nil
}

// AdminBanGroupForUser blocks one user from one group while preserving access
// to every other group.
func (s *UserService) AdminBanGroupForUser(ctx context.Context, userID, groupID int64, durationHours int) (*User, error) {
	if userID <= 0 {
		return nil, infraerrors.BadRequest("INVALID_USER_ID", "user ID is invalid")
	}
	expiresAt, err := manualBanExpiry(time.Now(), durationHours)
	if err != nil {
		return nil, err
	}
	if groupID <= 0 {
		return nil, infraerrors.BadRequest("INVALID_GROUP_ID", "group ID is invalid")
	}
	user, err := s.userRepo.GetByID(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("get user for manual group ban: %w", err)
	}
	if err := validateManualBanTarget(user); err != nil {
		return nil, err
	}
	banRepo, ok := s.userRepo.(UserGroupBanRepository)
	if !ok {
		return nil, infraerrors.InternalServer("GROUP_BAN_REPOSITORY_UNAVAILABLE", "group ban repository is unavailable")
	}
	if err := banRepo.BanGroupForUser(ctx, userID, groupID, expiresAt); err != nil {
		return nil, fmt.Errorf("apply manual group ban: %w", err)
	}
	setUserGroupBanState(user, groupID, expiresAt)
	s.invalidateManualBanAuthCache(ctx, userID)
	return user, nil
}

// AdminUnbanGroupForUser restores one user-group pair and keeps other bans.
func (s *UserService) AdminUnbanGroupForUser(ctx context.Context, userID, groupID int64) (*User, error) {
	if userID <= 0 {
		return nil, infraerrors.BadRequest("INVALID_USER_ID", "user ID is invalid")
	}
	if groupID <= 0 {
		return nil, infraerrors.BadRequest("INVALID_GROUP_ID", "group ID is invalid")
	}
	user, err := s.userRepo.GetByID(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("get user for manual group unban: %w", err)
	}
	banRepo, ok := s.userRepo.(UserGroupBanRepository)
	if !ok {
		return nil, infraerrors.InternalServer("GROUP_BAN_REPOSITORY_UNAVAILABLE", "group ban repository is unavailable")
	}
	if err := banRepo.UnbanGroupForUser(ctx, userID, groupID); err != nil {
		return nil, fmt.Errorf("remove manual group ban: %w", err)
	}
	clearUserGroupBanState(user, groupID)
	s.invalidateManualBanAuthCache(ctx, userID)
	return user, nil
}

func setUserGroupBanState(user *User, groupID int64, expiresAt *time.Time) {
	for _, existingID := range user.BannedGroupIDs {
		if existingID == groupID {
			setUserGroupBanExpiry(user, groupID, expiresAt)
			return
		}
	}
	user.BannedGroupIDs = append(user.BannedGroupIDs, groupID)
	setUserGroupBanExpiry(user, groupID, expiresAt)
}

func setUserGroupBanExpiry(user *User, groupID int64, expiresAt *time.Time) {
	if expiresAt == nil {
		delete(user.BannedGroupExpirations, groupID)
		return
	}
	if user.BannedGroupExpirations == nil {
		user.BannedGroupExpirations = make(map[int64]time.Time)
	}
	user.BannedGroupExpirations[groupID] = *expiresAt
}

func clearUserGroupBanState(user *User, groupID int64) {
	filtered := user.BannedGroupIDs[:0]
	for _, existingID := range user.BannedGroupIDs {
		if existingID != groupID {
			filtered = append(filtered, existingID)
		}
	}
	user.BannedGroupIDs = filtered
	delete(user.BannedGroupExpirations, groupID)
}

func (s *UserService) invalidateManualBanAuthCache(ctx context.Context, userID int64) {
	if s.authCacheInvalidator != nil {
		s.authCacheInvalidator.InvalidateAuthCacheByUserID(ctx, userID)
	}
}
