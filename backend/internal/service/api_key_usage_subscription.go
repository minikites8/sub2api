package service

import (
	"context"
	"fmt"
	"math"
	"time"
)

// SubscriptionUsageSnapshot keeps subscription spending windows separate from
// the API key's independent quota and rate-limit windows. All amounts use the
// existing subscription USD counters, without changing billing multipliers.
type SubscriptionUsageSnapshot struct {
	DailyLimitUSD       *float64   `json:"daily_limit_usd"`
	DailyUsageUSD       float64    `json:"daily_usage_usd"`
	DailyRemainingUSD   *float64   `json:"daily_remaining_usd,omitempty"`
	DailyPercentage     *float64   `json:"daily_percentage,omitempty"`
	DailyWindowStart    *time.Time `json:"daily_window_start"`
	DailyResetAt        *time.Time `json:"daily_reset_at,omitempty"`
	WeeklyLimitUSD      *float64   `json:"weekly_limit_usd"`
	WeeklyUsageUSD      float64    `json:"weekly_usage_usd"`
	WeeklyRemainingUSD  *float64   `json:"weekly_remaining_usd,omitempty"`
	WeeklyPercentage    *float64   `json:"weekly_percentage,omitempty"`
	WeeklyWindowStart   *time.Time `json:"weekly_window_start"`
	WeeklyResetAt       *time.Time `json:"weekly_reset_at,omitempty"`
	MonthlyLimitUSD     *float64   `json:"monthly_limit_usd"`
	MonthlyUsageUSD     float64    `json:"monthly_usage_usd"`
	MonthlyRemainingUSD *float64   `json:"monthly_remaining_usd,omitempty"`
	MonthlyPercentage   *float64   `json:"monthly_percentage,omitempty"`
	MonthlyWindowStart  *time.Time `json:"monthly_window_start"`
	MonthlyResetAt      *time.Time `json:"monthly_reset_at,omitempty"`
	ExpiresAt           time.Time  `json:"expires_at"`
	Status              string     `json:"status"`
	Unlimited           bool       `json:"unlimited"`
}

// RefreshUsageKey reads the persisted quota/status without reusing cached
// spending counters. The selected group is retained only if it is still bound.
func (s *APIKeyService) RefreshUsageKey(ctx context.Context, authenticated *APIKey) (*APIKey, error) {
	if s == nil || authenticated == nil || s.apiKeyRepo == nil {
		return nil, fmt.Errorf("key usage unavailable")
	}
	fresh, err := s.apiKeyRepo.GetByID(ctx, authenticated.ID)
	if err != nil {
		return nil, err
	}
	if fresh == nil || fresh.ID != authenticated.ID || fresh.UserID != authenticated.UserID {
		return nil, ErrAPIKeyNotFound
	}
	copyKey := *authenticated
	copyKey.Status, copyKey.Quota, copyKey.QuotaUsed, copyKey.ExpiresAt = fresh.Status, fresh.Quota, fresh.QuotaUsed, fresh.ExpiresAt
	copyKey.RateLimit5h, copyKey.RateLimit1d, copyKey.RateLimit7d = fresh.RateLimit5h, fresh.RateLimit1d, fresh.RateLimit7d
	if authenticated.Group != nil {
		groupID := authenticated.Group.ID
		bound := false
		for _, id := range fresh.OrderedGroupIDs() {
			if id == groupID {
				bound = true
				break
			}
		}
		if !bound {
			return nil, ErrGroupNotAllowed
		}
		if s.groupRepo == nil {
			return nil, fmt.Errorf("group usage unavailable")
		}
		group, err := s.groupRepo.GetByID(ctx, groupID)
		if err != nil {
			return nil, err
		}
		if group == nil || !group.IsActive() {
			return nil, ErrGroupNotAllowed
		}
		copyKey.Group = group
	}
	return &copyKey, nil
}

// GetUsageSubscriptionSnapshot reloads only the authenticated user's selected
// group. It does not sum groups, skip exhausted periods, or mutate usage windows.
func (s *APIKeyService) GetUsageSubscriptionSnapshot(ctx context.Context, key *APIKey) (*SubscriptionUsageSnapshot, error) {
	if s == nil || key == nil {
		return nil, fmt.Errorf("subscription usage unavailable")
	}
	if key.Group == nil || !key.Group.IsSubscriptionType() {
		return nil, nil
	}
	if s.userSubRepo == nil {
		return nil, fmt.Errorf("subscription usage unavailable")
	}
	sub, err := s.userSubRepo.GetActiveByUserIDAndGroupID(ctx, key.UserID, key.Group.ID)
	if err != nil {
		return nil, err
	}
	if sub == nil || sub.UserID != key.UserID || sub.GroupID != key.Group.ID {
		return nil, ErrSubscriptionNotFound
	}
	return SnapshotSubscriptionUsage(key.Group, sub, time.Now())
}

// SnapshotSubscriptionUsage reuses the same calendar-day / rolling-window and
// one-time daily-card rules as subscription admission and maintenance. A window
// cleared for display has no persisted reset anchor, so no reset time is invented.
func SnapshotSubscriptionUsage(group *Group, sub *UserSubscription, now time.Time) (*SubscriptionUsageSnapshot, error) {
	if group == nil {
		return nil, fmt.Errorf("subscription group unavailable")
	}
	if sub == nil || sub.Status != SubscriptionStatusActive || !sub.ExpiresAt.After(now) {
		return nil, ErrSubscriptionNotFound
	}
	copySub := *sub
	copySub.Group = group
	normalized := []UserSubscription{copySub}
	normalizeExpiredWindowsAt(normalized, now)
	copySub = normalized[0]
	daily, dailyPct, err := subscriptionUsageAmounts(group.DailyLimitUSD, copySub.DailyUsageUSD)
	if err != nil {
		return nil, err
	}
	weekly, weeklyPct, err := subscriptionUsageAmounts(group.WeeklyLimitUSD, copySub.WeeklyUsageUSD)
	if err != nil {
		return nil, err
	}
	monthly, monthlyPct, err := subscriptionUsageAmounts(group.MonthlyLimitUSD, copySub.MonthlyUsageUSD)
	if err != nil {
		return nil, err
	}
	snapshot := &SubscriptionUsageSnapshot{
		DailyLimitUSD: group.DailyLimitUSD, DailyUsageUSD: copySub.DailyUsageUSD, DailyRemainingUSD: daily, DailyPercentage: dailyPct, DailyWindowStart: copySub.DailyWindowStart,
		WeeklyLimitUSD: group.WeeklyLimitUSD, WeeklyUsageUSD: copySub.WeeklyUsageUSD, WeeklyRemainingUSD: weekly, WeeklyPercentage: weeklyPct, WeeklyWindowStart: copySub.WeeklyWindowStart,
		MonthlyLimitUSD: group.MonthlyLimitUSD, MonthlyUsageUSD: copySub.MonthlyUsageUSD, MonthlyRemainingUSD: monthly, MonthlyPercentage: monthlyPct, MonthlyWindowStart: copySub.MonthlyWindowStart,
		ExpiresAt: sub.ExpiresAt, Status: sub.Status, Unlimited: daily == nil && weekly == nil && monthly == nil,
	}
	// A final period can end at subscription expiry without renewing. Its
	// nominal weekly/monthly boundary must not be advertised as a quota reset.
	actualReset := func(reset *time.Time) *time.Time {
		if reset != nil && reset.After(now) && reset.Before(sub.ExpiresAt) {
			return reset
		}
		return nil
	}
	if daily != nil && !copySub.HasOneTimeDailyQuota() {
		snapshot.DailyResetAt = actualReset(copySub.DailyResetTime())
	}
	if weekly != nil {
		snapshot.WeeklyResetAt = actualReset(copySub.WeeklyResetTime())
	}
	if monthly != nil {
		snapshot.MonthlyResetAt = actualReset(copySub.MonthlyResetTime())
	}
	return snapshot, nil
}

func subscriptionUsageAmounts(limit *float64, used float64) (*float64, *float64, error) {
	if math.IsNaN(used) || math.IsInf(used, 0) || used < 0 || (limit != nil && (math.IsNaN(*limit) || math.IsInf(*limit, 0))) {
		return nil, nil, fmt.Errorf("invalid subscription usage")
	}
	if limit == nil || *limit <= 0 {
		return nil, nil, nil
	}
	remaining := math.Max(0, *limit-used)
	percentage := math.Min(100, used / *limit * 100)
	return &remaining, &percentage, nil
}
