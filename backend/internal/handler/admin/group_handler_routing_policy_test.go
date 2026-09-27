package admin

import (
	"encoding/json"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestGroupRequestsDecodeRoutingPolicy(t *testing.T) {
	var create CreateGroupRequest
	require.NoError(t, json.Unmarshal([]byte(`{"enable_bps":true,"scheduling_strategy":"priority_5h"}`), &create))
	require.True(t, create.EnableBPS)
	require.Equal(t, "priority_5h", create.SchedulingStrategy)
	var update UpdateGroupRequest
	require.NoError(t, json.Unmarshal([]byte(`{"enable_bps":false,"scheduling_strategy":"balanced"}`), &update))
	require.NotNil(t, update.EnableBPS)
	require.False(t, *update.EnableBPS)
	require.Equal(t, "balanced", *update.SchedulingStrategy)
	var omitted UpdateGroupRequest
	require.NoError(t, json.Unmarshal([]byte(`{}`), &omitted))
	require.Nil(t, omitted.EnableBPS)
	require.Nil(t, omitted.SchedulingStrategy)
}
