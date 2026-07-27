//go:build unit

package service

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestBaiduVODNewTaskUsesChannelVideoSecondPrice(t *testing.T) {
	resolver := newResolverWithChannel(t, []ChannelModelPricing{{
		Platform:        "anthropic",
		Models:          []string{"happyhorse-1.1-t2v"},
		BillingMode:     BillingModeVideo,
		PerRequestPrice: testPtrFloat64(0.4),
		Intervals: []PricingInterval{
			{TierLabel: "1080P", PerRequestPrice: testPtrFloat64(0.75)},
		},
	}})
	service := &BaiduVODVideoService{
		billing: NewBillingService(nil, nil),
		pricing: resolver,
	}
	groupID := groupIDPtr()
	apiKey := &APIKey{
		ID:      10,
		UserID:  20,
		User:    &User{ID: 20},
		GroupID: groupID,
		Group:   &Group{ID: *groupID, RateMultiplier: 2},
	}
	account := &Account{ID: 30, Platform: PlatformBaiduVOD}
	task, err := service.NewTask(
		context.Background(),
		"video_test",
		apiKey,
		account,
		BaiduVODVideoRequest{Model: "happyhorse-1.1-t2v", Resolution: "1080P", Duration: 5},
		BaiduVODModelSpec{UpstreamModel: "happyhorse-1.1-t2v", Capability: BaiduVODCapabilityT2V},
		&BaiduVODSubmitResult{TaskID: "upstream_test", TaskStatus: "PENDING"},
		"request-hash",
	)
	require.NoError(t, err)
	require.InDelta(t, 7.5, task.EstimatedCost, 1e-12)
	require.InDelta(t, 7.5, task.HoldAmount, 1e-12)
}
