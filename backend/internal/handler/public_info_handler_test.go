//go:build unit

package handler

import (
	"context"
	"testing"
	"time"

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

type publicInfoMonitorRepo struct {
	service.ChannelMonitorRepository
	monitor      *service.ChannelMonitor
	latest       []*service.ChannelMonitorLatest
	timeline     []*service.ChannelMonitorHistoryEntry
	availability map[int][]*service.ChannelMonitorAvailability
}

func (r *publicInfoMonitorRepo) ListEnabled(context.Context) ([]*service.ChannelMonitor, error) {
	return []*service.ChannelMonitor{r.monitor}, nil
}

func (r *publicInfoMonitorRepo) GetByID(_ context.Context, id int64) (*service.ChannelMonitor, error) {
	if r.monitor != nil && r.monitor.ID == id {
		return r.monitor, nil
	}
	return nil, service.ErrChannelMonitorNotFound
}

func (r *publicInfoMonitorRepo) ListLatestForMonitorIDs(context.Context, []int64) (map[int64][]*service.ChannelMonitorLatest, error) {
	return map[int64][]*service.ChannelMonitorLatest{r.monitor.ID: r.latest}, nil
}

func (r *publicInfoMonitorRepo) ComputeAvailabilityForMonitors(_ context.Context, _ []int64, windowDays int) (map[int64][]*service.ChannelMonitorAvailability, error) {
	return map[int64][]*service.ChannelMonitorAvailability{r.monitor.ID: r.availability[windowDays]}, nil
}

func (r *publicInfoMonitorRepo) ListRecentHistoryForMonitors(context.Context, []int64, map[int64]string, int) (map[int64][]*service.ChannelMonitorHistoryEntry, error) {
	return map[int64][]*service.ChannelMonitorHistoryEntry{r.monitor.ID: r.timeline}, nil
}

func (r *publicInfoMonitorRepo) ListLatestPerModel(context.Context, int64) ([]*service.ChannelMonitorLatest, error) {
	return r.latest, nil
}

func (r *publicInfoMonitorRepo) ComputeAvailability(_ context.Context, _ int64, windowDays int) ([]*service.ChannelMonitorAvailability, error) {
	return r.availability[windowDays], nil
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

func TestPublicInfoLoadPublicModelAvailabilityIncludesTimeline(t *testing.T) {
	latency := 123
	pingLatency := 45
	checkedAt := time.Date(2026, 7, 1, 8, 30, 0, 0, time.FixedZone("CST", 8*60*60))
	repo := &publicInfoMonitorRepo{
		monitor: &service.ChannelMonitor{
			ID:           7,
			Name:         "OpenAI",
			Provider:     service.MonitorProviderOpenAI,
			PrimaryModel: "gpt-5",
			GroupName:    "default",
			Enabled:      true,
		},
		latest: []*service.ChannelMonitorLatest{
			{
				Model:         "gpt-5",
				Status:        service.MonitorStatusOperational,
				LatencyMs:     &latency,
				PingLatencyMs: &pingLatency,
				CheckedAt:     checkedAt,
			},
		},
		timeline: []*service.ChannelMonitorHistoryEntry{
			{
				Model:         "gpt-5",
				Status:        service.MonitorStatusOperational,
				LatencyMs:     &latency,
				PingLatencyMs: &pingLatency,
				CheckedAt:     checkedAt,
			},
		},
		availability: map[int][]*service.ChannelMonitorAvailability{
			7: {
				{Model: "gpt-5", AvailabilityPct: 99.5},
			},
			15: {
				{Model: "gpt-5", AvailabilityPct: 98.5},
			},
			30: {
				{Model: "gpt-5", AvailabilityPct: 97.5},
			},
		},
	}
	h := NewPublicInfoHandler(nil, service.NewChannelMonitorService(repo, nil), nil, nil)

	availability, err := h.loadPublicModelAvailability(context.Background())

	require.NoError(t, err)
	require.Equal(t, []publicMonitorAvailability{
		{
			ID:        7,
			Name:      "OpenAI",
			Provider:  service.MonitorProviderOpenAI,
			GroupName: "default",
			Models: []publicModelAvailability{
				{
					Model:           "gpt-5",
					LatestStatus:    service.MonitorStatusOperational,
					Availability7d:  99.5,
					Availability15d: 98.5,
					Availability30d: 97.5,
				},
			},
			Timeline: []publicMonitorTimelinePoint{
				{
					Status:        service.MonitorStatusOperational,
					LatencyMs:     &latency,
					PingLatencyMs: &pingLatency,
					CheckedAt:     "2026-07-01T00:30:00Z",
				},
			},
		},
	}, availability)
}
