//go:build unit

package handler

import (
	"context"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

type publicInfoGroupRepo struct {
	service.GroupRepository
	groups []service.Group
}

func (r *publicInfoGroupRepo) ListActive(context.Context) ([]service.Group, error) {
	return r.groups, nil
}

func TestPublicInfoLoadPublicGroupRatesUsesUserVisibleGroups(t *testing.T) {
	h := NewPublicInfoHandler(&publicInfoGroupRepo{groups: []service.Group{
		{
			ID:                   1,
			Name:                 "public",
			Platform:             service.PlatformOpenAI,
			Status:               service.StatusActive,
			SubscriptionType:     service.SubscriptionTypeStandard,
			RateMultiplier:       0.5,
			AllowImageGeneration: true,
			ImageRateMultiplier:  1,
			ActiveAccountCount:   2,
		},
		{
			ID:                 2,
			Name:               "exclusive",
			Platform:           service.PlatformOpenAI,
			Status:             service.StatusActive,
			SubscriptionType:   service.SubscriptionTypeStandard,
			IsExclusive:        true,
			ActiveAccountCount: 2,
		},
		{
			ID:                 3,
			Name:               "subscription",
			Platform:           service.PlatformOpenAI,
			Status:             service.StatusActive,
			SubscriptionType:   service.SubscriptionTypeSubscription,
			ActiveAccountCount: 2,
		},
		{
			ID:                 4,
			Name:               "no-active-account",
			Platform:           service.PlatformKiro,
			Status:             service.StatusActive,
			SubscriptionType:   service.SubscriptionTypeStandard,
			ActiveAccountCount: 0,
		},
	}}, nil, nil, nil)

	groups, err := h.loadPublicGroupRates(context.Background())
	require.NoError(t, err)
	require.Equal(t, []publicGroupRate{
		{
			ID:                   1,
			Name:                 "public",
			Platform:             service.PlatformOpenAI,
			RateMultiplier:       0.5,
			AllowImageGeneration: true,
			ImageRateMultiplier:  1,
		},
	}, groups)
}
