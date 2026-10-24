package service

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/ctxkey"
	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
)

const MaxAPIKeyGroups = 32

var ErrAPIKeyGroupsUnavailable = infraerrors.ServiceUnavailable("API_KEY_GROUPS_UNAVAILABLE", "No available bound group can serve this request")

type orderedGroupAccountSource interface {
	ListSchedulableAccounts(context.Context, *int64, string, bool) ([]Account, bool, error)
}

// APIKeyGroupSelection describes the request before protocol dispatch.
type APIKeyGroupSelection struct {
	Models        []string
	GoogleOnly    bool
	CheckCapacity bool
	CheckBilling  bool
	AcceptGroup   func(*Group) bool
}

func firstAPIKeyGroupID(ids []int64) *int64 {
	if len(ids) == 0 {
		return nil
	}
	id := ids[0]
	return &id
}

func (s *APIKeyService) validateOrderedGroups(ctx context.Context, user *User, legacyID *int64, ids []int64) ([]int64, error) {
	if ids == nil && legacyID != nil {
		ids = []int64{*legacyID}
	}
	if len(ids) > MaxAPIKeyGroups || (legacyID != nil && (len(ids) == 0 || ids[0] != *legacyID)) {
		return nil, infraerrors.BadRequest("INVALID_API_KEY_GROUPS", "group_ids must contain at most 32 groups and group_id must match the first group")
	}
	seen := make(map[int64]bool, len(ids))
	for _, id := range ids {
		if id <= 0 || seen[id] {
			return nil, infraerrors.BadRequest("INVALID_API_KEY_GROUPS", "group_ids must contain unique positive group IDs")
		}
		seen[id] = true
		group, err := s.groupRepo.GetByID(ctx, id)
		if err != nil {
			return nil, fmt.Errorf("get group: %w", err)
		}
		if !s.canUserBindGroup(ctx, user, group) {
			return nil, ErrGroupNotAllowed
		}
	}
	return append([]int64{}, ids...), nil
}

// APIKeyInBoundGroup returns a request-local copy, keeping the cached key's
// binding and user/group permission snapshot immutable. Queue revalidation also
// uses this to recheck the group actually serving the request.
func (s *APIKeyService) APIKeyInBoundGroup(ctx context.Context, key *APIKey, groupID int64) (*APIKey, error) {
	if key == nil {
		return nil, ErrAPIKeyNotFound
	}
	if !slices.Contains(key.OrderedGroupIDs(), groupID) {
		return nil, ErrGroupNotAllowed
	}
	group, err := s.groupRepo.GetByID(ctx, groupID)
	if err != nil {
		return nil, err
	}
	if group == nil {
		return nil, ErrGroupNotFound
	}
	copyKey := *key
	copyKey.GroupID = firstAPIKeyGroupID([]int64{groupID})
	copyKey.Group = group
	if key.User != nil {
		user := *key.User
		copyKey.User = &user
		user.UserGroupDeniedModels = nil
		user.UserGroupRPMOverride = nil
		if err := s.loadUserGroupDeniedModels(ctx, &copyKey); err != nil {
			return nil, err
		}
		if s.userGroupRateRepo != nil {
			override, err := s.userGroupRateRepo.GetRPMOverrideByUserAndGroup(ctx, key.UserID, groupID)
			if err != nil {
				return nil, err
			}
			user.UserGroupRPMOverride = override
		}
	}
	return &copyKey, nil
}

// SelectOrderedGroup selects before protocol dispatch and billing, so both use
// the same authorized group. Key-wide quota, expiry and concurrency remain
// shared by every bound group and are enforced by the existing auth path.
func (s *APIKeyService) SelectOrderedGroup(ctx context.Context, key *APIKey, selection APIKeyGroupSelection, subscriptions *SubscriptionService) (*APIKey, error) {
	if len(key.OrderedGroupIDs()) <= 1 || key.User == nil {
		return key, nil
	}
	forcedPlatform, forced := ctx.Value(ctxkey.ForcePlatform).(string)
	for _, groupID := range key.OrderedGroupIDs() {
		candidate, err := s.APIKeyInBoundGroup(ctx, key, groupID)
		if errors.Is(err, ErrGroupNotFound) {
			continue
		}
		if err != nil {
			return nil, err
		}
		group, user := candidate.Group, candidate.User
		if !group.IsActive() || user.IsGroupBanned(groupID) {
			continue
		}
		if selection.AcceptGroup != nil && !selection.AcceptGroup(group) {
			continue
		}
		if selection.GoogleOnly && !forced && group.Platform != PlatformGemini && group.Platform != PlatformAntigravity && group.Platform != PlatformComposite {
			continue
		}
		if !group.IsSubscriptionType() && !user.CanBindGroup(groupID, group.IsExclusive) {
			continue
		}
		allowed := FirstUserGroupDeniedModel(candidate.DeniedModelsInGroup(), selection.Models) == ""
		for _, model := range selection.Models {
			allowed = allowed && group.ModelAllowlist.Allows(model)
			if IsGPTAutoVirtualModel(model) {
				allowed = allowed && group.GPTAutoEnabled && group.Platform == PlatformOpenAI
			}
		}
		if !allowed {
			continue
		}
		if selection.CheckBilling && (s.cfg == nil || s.cfg.RunMode != config.RunModeSimple) {
			if group.IsSubscriptionType() {
				if subscriptions == nil {
					continue
				}
				sub, err := subscriptions.GetActiveSubscription(ctx, user.ID, groupID)
				if err != nil {
					continue
				}
				maintenance, limitErr := subscriptions.ValidateAndCheckLimits(sub, group)
				if maintenance {
					sub, err = subscriptions.EnsureWindowMaintenance(ctx, sub)
					if err != nil {
						return nil, err
					}
					_, limitErr = subscriptions.ValidateAndCheckLimits(sub, group)
				}
				if limitErr != nil {
					continue
				}
				// The billing precheck rejects an already exhausted window even
				// when the auth validator's zero-cost check permits equality.
				if (group.HasDailyLimit() && sub.DailyUsageUSD >= *group.DailyLimitUSD) ||
					(group.HasWeeklyLimit() && sub.WeeklyUsageUSD >= *group.WeeklyLimitUSD) ||
					(group.HasMonthlyLimit() && sub.MonthlyUsageUSD >= *group.MonthlyLimitUSD) {
					continue
				}
			} else if user.Balance <= 0 {
				continue
			}
		}
		if selection.CheckCapacity && s.groupAccounts != nil && group.Platform != PlatformComposite {
			platform := group.Platform
			if forced {
				platform = forcedPlatform
			}
			accounts, mixed, err := s.groupAccounts.ListSchedulableAccounts(ctx, &groupID, platform, forced)
			if err != nil {
				return nil, err
			}
			available := false
			for i := range accounts {
				account := &accounts[i]
				if !account.IsSchedulable() || (mixed && account.Platform == PlatformAntigravity && !account.IsMixedSchedulingEnabled()) {
					continue
				}
				supported := true
				for _, model := range selection.Models {
					if strings.TrimSpace(model) == "" {
						continue
					}
					if IsGPTAutoVirtualModel(model) {
						eligible := false
						for _, candidate := range GPTAutoModelPool() {
							if group.ModelAllowlist.Allows(candidate) && FirstUserGroupDeniedModel(key.DeniedModelsInGroup(), []string{candidate}) == "" && account.IsModelAllowedInGroup(&groupID, candidate) && accountSupportsGatewayModel(account, candidate) {
								eligible = true
								break
							}
						}
						supported = supported && eligible
						continue
					}
					supported = supported && account.IsModelAllowedInGroup(&groupID, model) && !account.modelCapabilityRejectedForRequest(ctx, model)
					supported = supported && accountSupportsGatewayModel(account, model)
				}
				if supported {
					available = true
					break
				}
			}
			if !available {
				continue
			}
		}
		return candidate, nil
	}
	return nil, ErrAPIKeyGroupsUnavailable
}
