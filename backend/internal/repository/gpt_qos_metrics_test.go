package repository

import (
	"context"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func TestGPTQoSMetricsProtocolRoundTripAndFilters(t *testing.T) {
	c, _ := qosTestCache(t)
	ctx := context.Background()
	now := time.Now().UnixMilli()
	events := []service.GPTQoSMetric{
		{At: now, GroupID: 7, Tier: "premium", Model: "model-a", Effort: "high", AccountType: service.AccountTypeAPIKey, Attempts: 2},
		{At: now, GroupID: 8, Tier: "standard", Model: "model-a", Effort: "low", AccountType: service.AccountTypeOAuth, Attempts: 1},
		{At: now, GroupID: 7, Tier: "economy", Model: "model-b", Effort: "high", UpstreamKind: "vm2api", Attempts: 1},
		{At: time.Now().Add(-25 * time.Hour).UnixMilli(), GroupID: 7, Tier: "premium", AccountType: service.AccountTypeAPIKey, Attempts: 1},
	}
	for _, event := range events {
		require.NoError(t, c.GPTQoSRecord(ctx, event))
	}
	for _, tc := range []struct {
		name   string
		filter service.GPTQoSMetricFilter
		want   []service.GPTQoSMetric
	}{
		{"window", service.GPTQoSMetricFilter{}, []service.GPTQoSMetric{events[2], events[1], events[0]}},
		{"group", service.GPTQoSMetricFilter{GroupID: 7}, []service.GPTQoSMetric{events[2], events[0]}},
		{"model", service.GPTQoSMetricFilter{Model: "model-a"}, []service.GPTQoSMetric{events[1], events[0]}},
		{"effort", service.GPTQoSMetricFilter{Effort: "low"}, []service.GPTQoSMetric{events[1]}},
		{"combined", service.GPTQoSMetricFilter{GroupID: 7, Model: "model-a", Effort: "high"}, []service.GPTQoSMetric{events[0]}},
		{"empty", service.GPTQoSMetricFilter{GroupID: 999}, []service.GPTQoSMetric{}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := c.GPTQoSMetrics(ctx, tc.filter)
			require.NoError(t, err)
			require.Equal(t, tc.want, got, "account type survives storage; old events remain unclassified")
		})
	}
}
