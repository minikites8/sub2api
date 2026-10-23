package service

import (
	"context"
	"errors"
	"fmt"
	"math"
	"time"
)

// APIKeyGroupQuota describes the first billable bound group, not the sum of
// groups or the API key's independent spending cap. Amounts match the API-key
// group's available-quota display: billing headroom / effective group rate.
type APIKeyGroupQuota struct {
	IsValid        bool              `json:"isValid"`
	InvalidMessage string            `json:"invalidMessage,omitempty"`
	Error          *APIKeyQuotaError `json:"error,omitempty"`
	Balance        float64           `json:"balance"`
	Remaining      float64           `json:"remaining"`
	Unit           string            `json:"unit"`
	PlanName       string            `json:"planName,omitempty"`
	GroupID        int64             `json:"group_id,omitempty"`
	Total          *float64          `json:"total,omitempty"`
	Used           *float64          `json:"used,omitempty"`
	Unlimited      bool              `json:"unlimited,omitempty"`
	Window         string            `json:"quota_window,omitempty"`
}

type APIKeyQuotaError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

func invalidAPIKeyGroupQuota(code, message string) *APIKeyGroupQuota {
	return &APIKeyGroupQuota{Unit: "USD", InvalidMessage: message, Error: &APIKeyQuotaError{Code: code, Message: message}}
}

// GetFirstAvailableGroupQuota is read-only. It reloads wallet, permissions,
// group and subscription usage rather than trusting the auth cache, and resets
// elapsed subscription windows only in the returned in-memory snapshot.
func (s *APIKeyService) GetFirstAvailableGroupQuota(ctx context.Context, key *APIKey) (*APIKeyGroupQuota, error) {
	if s == nil || key == nil || s.userRepo == nil || s.groupRepo == nil {
		return nil, fmt.Errorf("group quota dependencies unavailable")
	}
	if key.Status == StatusAPIKeyExpired || key.IsExpired() {
		return invalidAPIKeyGroupQuota("API_KEY_EXPIRED", "API key has expired"), nil
	}
	if key.Status == StatusAPIKeyQuotaExhausted || key.IsQuotaExhausted() {
		return invalidAPIKeyGroupQuota("API_KEY_QUOTA_EXHAUSTED", "API key quota is exhausted"), nil
	}
	user, err := s.userRepo.GetByID(ctx, key.UserID)
	if err != nil {
		return nil, err
	}
	if user == nil || user.ID != key.UserID || !user.IsActive() {
		return invalidAPIKeyGroupQuota("USER_INACTIVE", "User account is not active"), nil
	}
	now := time.Now()
	for _, groupID := range key.OrderedGroupIDs() {
		group, err := s.groupRepo.GetByID(ctx, groupID)
		if errors.Is(err, ErrGroupNotFound) {
			continue
		}
		if err != nil {
			return nil, err
		}
		if group == nil || !group.IsActive() || user.IsGroupBanned(groupID) {
			continue
		}
		quota := &APIKeyGroupQuota{IsValid: true, Unit: "USD", GroupID: groupID, PlanName: group.Name}
		headroom := user.Balance
		if !group.IsSubscriptionType() && (math.IsNaN(headroom) || math.IsInf(headroom, 0)) {
			return nil, fmt.Errorf("invalid wallet balance")
		}
		if group.IsSubscriptionType() {
			if s.userSubRepo == nil {
				return nil, fmt.Errorf("subscription quota unavailable")
			}
			sub, err := s.userSubRepo.GetActiveByUserIDAndGroupID(ctx, user.ID, groupID)
			if errors.Is(err, ErrSubscriptionNotFound) {
				continue
			}
			if err != nil {
				return nil, err
			}
			if sub == nil || sub.Status != SubscriptionStatusActive || !sub.ExpiresAt.After(now) {
				continue
			}
			copySub := *sub
			copySub.Group = group
			snapshots := []UserSubscription{copySub}
			normalizeExpiredWindowsAt(snapshots, now)
			copySub = snapshots[0]
			headroom = math.Inf(1)
			for _, window := range []struct {
				name  string
				limit *float64
				used  float64
			}{
				{"daily", group.DailyLimitUSD, copySub.DailyUsageUSD},
				{"weekly", group.WeeklyLimitUSD, copySub.WeeklyUsageUSD},
				{"monthly", group.MonthlyLimitUSD, copySub.MonthlyUsageUSD},
			} {
				if window.limit == nil || *window.limit <= 0 {
					continue
				}
				if math.IsNaN(*window.limit) || math.IsInf(*window.limit, 0) || math.IsNaN(window.used) || math.IsInf(window.used, 0) {
					return nil, fmt.Errorf("invalid subscription quota")
				}
				remaining := math.Max(0, *window.limit-window.used)
				if remaining < headroom {
					headroom = remaining
					limit, used := *window.limit, window.used
					quota.Total, quota.Used, quota.Window = &limit, &used, window.name
				}
			}
		} else if !user.CanBindGroup(groupID, group.IsExclusive) {
			continue
		}
		if headroom <= 0 {
			continue
		}
		rate := group.RateMultiplier
		if s.userGroupRateRepo != nil {
			override, err := s.userGroupRateRepo.GetByUserAndGroup(ctx, user.ID, groupID)
			if err != nil {
				return nil, err
			}
			if override != nil {
				rate = *override
			}
		}
		if rate < 0 || math.IsNaN(rate) || math.IsInf(rate, 0) || math.IsNaN(headroom) {
			return nil, fmt.Errorf("invalid group quota configuration")
		}
		if rate == 0 || math.IsInf(headroom, 1) {
			quota.Balance, quota.Remaining, quota.Unlimited = -1, -1, true
			quota.Total, quota.Used = nil, nil
		} else {
			quota.Balance, quota.Remaining = headroom/rate, headroom/rate
			if math.IsInf(quota.Balance, 0) {
				return nil, fmt.Errorf("group quota exceeds supported range")
			}
			if quota.Total != nil {
				*quota.Total /= rate
				*quota.Used /= rate
				if math.IsInf(*quota.Total, 0) || math.IsInf(*quota.Used, 0) {
					return nil, fmt.Errorf("subscription quota exceeds supported range")
				}
			}
		}
		return quota, nil
	}
	return invalidAPIKeyGroupQuota("NO_AVAILABLE_GROUP", "No available bound group has remaining quota"), nil
}
