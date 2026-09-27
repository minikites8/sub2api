package service

import (
	"context"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/pkg/ctxkey"
	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
)

const (
	GroupSchedulingBalanced       = "balanced"
	GroupSchedulingPriority5h     = "priority_5h"
	GroupSchedulingPriorityWeekly = "priority_weekly"
)

func NormalizeGroupSchedulingStrategy(value string) (string, error) {
	value = strings.ToLower(strings.TrimSpace(value))
	switch value {
	case "", GroupSchedulingBalanced:
		return GroupSchedulingBalanced, nil
	case GroupSchedulingPriority5h, GroupSchedulingPriorityWeekly:
		return value, nil
	default:
		return "", infraerrors.BadRequest("INVALID_SCHEDULING_STRATEGY", "scheduling_strategy must be balanced, priority_5h, or priority_weekly")
	}
}

func (g *Group) EffectiveSchedulingStrategy() string {
	if g != nil {
		switch g.SchedulingStrategy {
		case GroupSchedulingPriority5h, GroupSchedulingPriorityWeekly:
			return g.SchedulingStrategy
		}
	}
	return GroupSchedulingBalanced
}

type openAIGroupRoutingPolicyKey struct{}
type openAIGroupRoutingPolicy struct {
	groupID int64
	group   *Group
}

func (s *OpenAIGatewayService) openAIRequestGroup(ctx context.Context, groupID *int64) *Group {
	if ctx != nil {
		if policy, ok := ctx.Value(openAIGroupRoutingPolicyKey{}).(openAIGroupRoutingPolicy); ok && policy.groupID == derefGroupID(groupID) {
			return policy.group
		}
		if group, ok := ctx.Value(ctxkey.Group).(*Group); ok && group != nil && (groupID == nil || group.ID == *groupID) {
			return group
		}
	}
	if groupID != nil {
		if s != nil && s.schedulerSnapshot != nil {
			if group, err := s.schedulerSnapshot.GetGroupByIDLite(ctx, *groupID); err == nil && group != nil {
				return group
			}
		}
		// Group-scoped requests require an explicit BPS opt-in. An unavailable
		// group snapshot retains the platform route and balanced scheduling.
		return &Group{ID: *groupID}
	}
	// Administrative account probes have no group and exercise account settings.
	return nil
}

func (s *OpenAIGatewayService) withOpenAIGroupRoutingPolicy(ctx context.Context, groupID *int64) context.Context {
	return context.WithValue(ctx, openAIGroupRoutingPolicyKey{}, openAIGroupRoutingPolicy{
		groupID: derefGroupID(groupID), group: s.openAIRequestGroup(ctx, groupID),
	})
}

func (s *OpenAIGatewayService) openAIAccountForGroup(ctx context.Context, groupID *int64, account *Account) *Account {
	group := s.openAIRequestGroup(ctx, groupID)
	if account == nil || group == nil || account.Platform != PlatformOpenAI {
		return account
	}
	disabled := !group.EnableBPS
	if account.bpsDisabledByGroup == disabled {
		return account
	}
	// Request-local metadata keeps account and scheduler cache settings intact.
	scoped := *account
	scoped.bpsDisabledByGroup = disabled
	return &scoped
}

func openAIGroupQuotaRank(account *Account, strategy string) int {
	tier := openAIOAuthQuotaScheduleTierFor(account)
	switch strategy {
	case GroupSchedulingPriority5h:
		if tier == openAIOAuthQuotaScheduleTierFiveHour {
			return 0
		}
	case GroupSchedulingPriorityWeekly:
		if tier == openAIOAuthQuotaScheduleTierWeeklyOnly {
			return 0
		}
	default:
		return 0
	}
	return 1
}

func compareOpenAIGroupQuotaRank(a, b *Account, strategy string) int {
	return openAIGroupQuotaRank(a, strategy) - openAIGroupQuotaRank(b, strategy)
}
