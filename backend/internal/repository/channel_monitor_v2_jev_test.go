package repository

import (
	"context"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/lib/pq"
	"github.com/stretchr/testify/require"
)

func TestChannelMonitorV2JevDimensionsWithoutTraffic(t *testing.T) {
	tests := []struct {
		name            string
		filter          service.ChannelMonitorV2Filter
		configured      []int64
		typesafeEnabled bool
		groupIDs        []int64
		wantJev         bool
	}{
		{name: "all active groups", typesafeEnabled: true, groupIDs: []int64{7, 9}, wantJev: true},
		{name: "picker selection keeps full catalog", filter: service.ChannelMonitorV2Filter{Platforms: []string{"openai"}, GroupIDs: []int64{9}}, typesafeEnabled: true, groupIDs: []int64{7, 9}, wantJev: true},
		{name: "explicit monitored groups", configured: []int64{7, 9}, typesafeEnabled: true, groupIDs: []int64{7, 9}, wantJev: true},
		{name: "disabled typesafe stays excluded", configured: []int64{7, 9}, groupIDs: []int64{7, 9}},
		{name: "viewer scope excludes Jev", filter: service.ChannelMonitorV2Filter{RestrictGroups: true, AllowedGroupIDs: []int64{9}}, typesafeEnabled: true, groupIDs: []int64{9}},
		{name: "configured scope excludes Jev", configured: []int64{9}, typesafeEnabled: true, groupIDs: []int64{9}},
		{name: "viewer scope includes Jev", filter: service.ChannelMonitorV2Filter{RestrictGroups: true, AllowedGroupIDs: []int64{7}}, typesafeEnabled: true, groupIDs: []int64{7}, wantJev: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db, mock, err := sqlmock.New()
			require.NoError(t, err)
			t.Cleanup(func() { _ = db.Close() })
			filter := tt.filter
			filter.Start = time.Date(2026, 10, 11, 0, 0, 0, 0, time.UTC)
			filter.End = filter.Start.Add(time.Hour)
			filter.Bucket = time.Minute
			cfg := service.ChannelMonitorV2Config{
				Enabled: true, GroupIDs: tt.configured,
				Platforms: []service.ChannelMonitorV2PlatformConfig{
					{Platform: "openai", Enabled: true},
					{Platform: "typesafe", Enabled: tt.typesafeEnabled},
				},
			}
			mock.ExpectQuery("SELECT usage_coverage_start").WillReturnRows(sqlmock.NewRows([]string{
				"usage_coverage_start", "error_coverage_start", "data_through", "last_successful_at", "backfill_cursor",
			}))
			mock.ExpectQuery("SELECT m.platform").WillReturnRows(sqlmock.NewRows([]string{
				"platform", "group_name", "group_platform", "group_id", "model", "request_count",
			}))
			if len(tt.configured) == 0 && !filter.RestrictGroups {
				mock.ExpectQuery("SELECT id FROM groups WHERE deleted_at IS NULL AND status = 'active' ORDER BY id").
					WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(7).AddRow(9))
			}
			info := sqlmock.NewRows([]string{"id", "name", "platform", "rate_multiplier"})
			for _, id := range tt.groupIDs {
				if id == 7 {
					info.AddRow(7, "Jev", "typesafe", 1.0)
				} else {
					info.AddRow(9, "OpenAI", "openai", 0.5)
				}
			}
			mock.ExpectQuery("SELECT id, COALESCE").WithArgs(pq.Array(tt.groupIDs)).WillReturnRows(info)
			dims, err := (&channelMonitorV2Repository{db: db}).GetDimensions(context.Background(), filter, cfg)
			require.NoError(t, err)
			found := false
			for _, group := range dims.Groups {
				if group.ID == 7 {
					found = true
					require.Equal(t, "typesafe", group.Platform)
					require.Equal(t, int64(0), group.RequestCount)
				}
			}
			t.Logf("Jev visible=%t; request_count=0; groups=%v", found, dims.Groups)
			require.Equal(t, tt.wantJev, found)
			require.NoError(t, mock.ExpectationsWereMet())
		})
	}
}

func TestChannelMonitorV2JevMatrixKeepsRealRequestOutcomes(t *testing.T) {
	cfg := service.ChannelMonitorV2Config{Platforms: []service.ChannelMonitorV2PlatformConfig{{Platform: "typesafe", Enabled: true}}}
	accs := seedChannelMonitorV2MatrixAccumulators(service.ChannelMonitorV2Filter{}, cfg, service.ChannelMonitorV2GroupByPlatformGroup,
		map[int64]channelMonitorV2GroupInfo{7: {name: "Jev", platform: "typesafe", rate: 1}})
	key := channelMonitorV2MatrixKey{platform: "typesafe", groupID: 7}
	require.Contains(t, accs, key)
	require.Equal(t, "Jev", accs[key].groupName)
	before := accs[key].total.metric(1, true)
	require.False(t, before.HasSamples)
	require.Equal(t, "unknown", service.ChannelMonitorV2HealthForWithThresholds(before, service.DefaultChannelMonitorV2HealthThresholds()).Overall)
	accs[key].total.addFact(channelMonitorV2Fact{Platform: "typesafe", GroupID: 7, Model: "jev-latest", Success: 9, Errors: 1})
	after := accs[key].total.metric(1, true)
	require.Equal(t, int64(10), after.RequestCount)
	require.Equal(t, int64(9), after.SuccessRequests)
	require.Equal(t, int64(1), after.ErrorRequests)
	require.Equal(t, 0.9, after.SuccessRate)
	t.Logf("Jev matrix: no requests => unknown; real requests => success=%d errors=%d rate=%.1f", after.SuccessRequests, after.ErrorRequests, after.SuccessRate)
}
