//go:build unit

package service

import (
	"context"
	"encoding/json"
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
	require.Equal(t, string(BillingModeVideo), task.BillingMode)
}

func TestBaiduVODNewSilentVeoTaskUsesBaseModelPrice(t *testing.T) {
	resolver := newResolverWithChannel(t, []ChannelModelPricing{{
		Platform:        "anthropic",
		Models:          []string{"veo-3.1-fast"},
		BillingMode:     BillingModeVideo,
		PerRequestPrice: testPtrFloat64(0.5),
	}})
	videoService := &BaiduVODVideoService{billing: NewBillingService(nil, nil), pricing: resolver}
	groupID := groupIDPtr()
	apiKey := &APIKey{
		ID: 13, UserID: 23, User: &User{ID: 23}, GroupID: groupID,
		Group: &Group{ID: *groupID, RateMultiplier: 2},
	}
	account := &Account{ID: 33, Platform: PlatformBaiduVOD}
	spec, ok := BaiduVODModel("veo-3.1-fast-silent")
	require.True(t, ok)

	task, err := videoService.NewTask(
		context.Background(), "video_veo_silent", apiKey, account,
		BaiduVODVideoRequest{Model: "veo-3.1-fast-silent", Prompt: "video", Resolution: "720P", Ratio: "16:9", Duration: 4},
		spec, &BaiduVODSubmitResult{TaskID: "veo-silent", TaskStatus: "queued"}, "request-hash",
	)
	require.NoError(t, err)
	require.Equal(t, "veo-3.1-fast-silent", task.Model)
	require.Equal(t, "VE3.1F", task.UpstreamModel)
	require.InDelta(t, 4, task.EstimatedCost, 1e-12)
	require.Equal(t, string(BillingModeVideo), task.BillingMode)
}

func TestBaiduVODNewSeedanceTaskUsesChannelTokenPrice(t *testing.T) {
	resolver := newResolverWithChannel(t, []ChannelModelPricing{{
		Platform:    "anthropic",
		Models:      []string{"doubao-seedance-2-0-260128"},
		BillingMode: BillingModeToken,
		OutputPrice: testPtrFloat64(2e-6),
	}})
	service := &BaiduVODVideoService{billing: NewBillingService(nil, nil), pricing: resolver}
	groupID := groupIDPtr()
	apiKey := &APIKey{
		ID: 11, UserID: 21, User: &User{ID: 21}, GroupID: groupID,
		Group: &Group{ID: *groupID, RateMultiplier: 2},
	}
	account := &Account{ID: 31, Platform: PlatformBaiduVOD}
	spec, ok := BaiduVODModel("doubao-seedance-2-0-260128")
	require.True(t, ok)
	task, err := service.NewTask(
		context.Background(), "video_seedance", apiKey, account,
		BaiduVODVideoRequest{Model: spec.Model, Prompt: "video", Resolution: "720P", Ratio: "16:9", Duration: 5},
		spec, &BaiduVODSubmitResult{TaskID: "cgt-seedance", TaskStatus: "queued"}, "request-hash",
	)
	require.NoError(t, err)
	// 1280 * 720 * 24 * 5 / 1024 = 108000 completion tokens.
	require.InDelta(t, 0.432, task.EstimatedCost, 1e-12)
	require.InDelta(t, 0.432, task.HoldAmount, 1e-12)
	require.Equal(t, string(BillingModeToken), task.BillingMode)
}

func TestBaiduVODNewSeedanceTaskUsesConditionalVideoTokenPrice(t *testing.T) {
	resolver := newResolverWithChannel(t, []ChannelModelPricing{{
		Platform: "anthropic", Models: []string{"doubao-seedance-2-0-260128"}, BillingMode: BillingModeVideoToken,
		Intervals: []PricingInterval{
			{TierLabel: "720p:text", OutputPrice: testPtrFloat64(46e-6)},
			{TierLabel: "720p:video", OutputPrice: testPtrFloat64(28e-6)},
		},
	}})
	videoService := &BaiduVODVideoService{billing: NewBillingService(nil, nil), pricing: resolver}
	groupID := groupIDPtr()
	apiKey := &APIKey{ID: 12, UserID: 22, User: &User{ID: 22}, GroupID: groupID, Group: &Group{ID: *groupID, RateMultiplier: 2}}
	account := &Account{ID: 32, Platform: PlatformBaiduVOD}
	spec, ok := BaiduVODModel("doubao-seedance-2-0-260128")
	require.True(t, ok)

	textRequest := BaiduVODVideoRequest{Model: spec.Model, Prompt: "video", Resolution: "720P", Ratio: "16:9", Duration: 5}
	textTask, err := videoService.NewTask(context.Background(), "video_seedance_text", apiKey, account, textRequest, spec,
		&BaiduVODSubmitResult{TaskID: "cgt-seedance-text", TaskStatus: "queued"}, "request-hash-text")
	require.NoError(t, err)
	require.False(t, textTask.InputContainsVideo)
	require.Equal(t, string(BillingModeVideoToken), textTask.BillingMode)
	require.InDelta(t, float64(estimateSeedanceCompletionTokens(textRequest, spec))*46e-6*2, textTask.EstimatedCost, 1e-12)

	videoRequest := textRequest
	videoRequest.Video = json.RawMessage(`"https://example.com/input.mp4"`)
	videoTask, err := videoService.NewTask(context.Background(), "video_seedance_video", apiKey, account, videoRequest, spec,
		&BaiduVODSubmitResult{TaskID: "cgt-seedance-video", TaskStatus: "queued"}, "request-hash-video")
	require.NoError(t, err)
	require.True(t, videoTask.InputContainsVideo)
	require.Equal(t, string(BillingModeVideoToken), videoTask.BillingMode)
	require.InDelta(t, float64(estimateSeedanceCompletionTokens(videoRequest, spec))*28e-6*2, videoTask.EstimatedCost, 1e-12)
}

func TestBaiduVODVideoWorkerSettlesSeedanceTokenPricing(t *testing.T) {
	worker, tasks, billingRepo := newBaiduVODWorkerHarness(baiduVODWorkerResponse(200,
		`{"id":"cgt-seedance-token","status":"succeeded","content":{"video_url":"https://example.com/seedance-token.mp4"},"usage":{"completion_tokens":120000,"total_tokens":120000},"duration":5,"resolution":"720p","ratio":"16:9"}`), nil)
	worker.service.pricing = newResolverWithChannel(t, []ChannelModelPricing{{
		Platform: "anthropic", Models: []string{"doubao-seedance-2-0-260128"}, BillingMode: BillingModeToken,
		OutputPrice: testPtrFloat64(2e-6),
	}})
	task := newBaiduVODWorkerTask()
	task.Provider = BaiduVODProviderSeedance
	task.Model = "doubao-seedance-2-0-260128"
	task.UpstreamModel = task.Model
	task.UpstreamTaskID = "cgt-seedance-token"
	task.GroupID = groupIDPtr()
	task.BillingMode = string(BillingModeToken)
	task.VideoRateMultiplier = 2
	task.EstimatedCost = 0.432
	task.HoldAmount = 1

	worker.processOne(context.Background(), task)

	require.Len(t, billingRepo.captures, 1)
	require.InDelta(t, 0.48, billingRepo.captures[0].ActualAmount, 1e-12)
	require.Len(t, tasks.updates, 1)
	require.Equal(t, BaiduVODTaskStatusCompleted, tasks.updates[0].Status)
}

func TestBaiduVODVideoWorkerSettlesConditionalVideoTokenPricing(t *testing.T) {
	worker, tasks, billingRepo := newBaiduVODWorkerHarness(baiduVODWorkerResponse(200,
		`{"id":"cgt-seedance-video-token","status":"succeeded","content":{"video_url":"https://example.com/seedance-token.mp4"},"usage":{"completion_tokens":120000,"total_tokens":120000},"duration":5,"resolution":"1080p","ratio":"16:9"}`), nil)
	worker.service.pricing = newResolverWithChannel(t, []ChannelModelPricing{{
		Platform: "anthropic", Models: []string{"doubao-seedance-2-0-260128"}, BillingMode: BillingModeVideoToken,
		Intervals: []PricingInterval{
			{TierLabel: "720p:video", OutputPrice: testPtrFloat64(28e-6)},
			{TierLabel: "1080p:video", OutputPrice: testPtrFloat64(31e-6)},
		},
	}})
	task := newBaiduVODWorkerTask()
	task.Provider = BaiduVODProviderSeedance
	task.Model = "doubao-seedance-2-0-260128"
	task.UpstreamModel = task.Model
	task.UpstreamTaskID = "cgt-seedance-video-token"
	task.GroupID = groupIDPtr()
	task.BillingMode = string(BillingModeVideoToken)
	task.InputContainsVideo = true
	task.VideoRateMultiplier = 2
	task.EstimatedCost = 6
	task.HoldAmount = 10

	worker.processOne(context.Background(), task)

	require.Len(t, billingRepo.captures, 1)
	require.InDelta(t, 7.44, billingRepo.captures[0].ActualAmount, 1e-12)
	require.Len(t, tasks.updates, 1)
	require.Equal(t, BaiduVODTaskStatusCompleted, tasks.updates[0].Status)
}
