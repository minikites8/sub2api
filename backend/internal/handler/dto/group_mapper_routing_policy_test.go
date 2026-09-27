package dto

import (
	"encoding/json"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestGroupMapperRoutingPolicy(t *testing.T) {
	group := &service.Group{ID: 7, EnableBPS: true, SchedulingStrategy: service.GroupSchedulingPriorityWeekly}
	admin := GroupFromServiceAdmin(group)
	require.True(t, admin.EnableBPS)
	require.Equal(t, service.GroupSchedulingPriorityWeekly, admin.SchedulingStrategy)
	encoded, err := json.Marshal(GroupFromService(group))
	require.NoError(t, err)
	require.NotContains(t, string(encoded), "enable_bps")
	require.NotContains(t, string(encoded), "scheduling_strategy")
	require.Equal(t, service.GroupSchedulingBalanced, GroupFromServiceAdmin(&service.Group{}).SchedulingStrategy)
}
