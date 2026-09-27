package repository

import (
	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestGroupRoutingPolicyEntityMapping(t *testing.T) {
	for _, strategy := range []string{service.GroupSchedulingBalanced, service.GroupSchedulingPriority5h, service.GroupSchedulingPriorityWeekly} {
		group := groupEntityToService(&dbent.Group{ID: 7, EnableBps: true, SchedulingStrategy: strategy})
		require.True(t, group.EnableBPS)
		require.Equal(t, strategy, group.SchedulingStrategy)
	}
}
