package admin

import (
	"context"
	"errors"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

type qosGroupNameStub struct {
	service.AdminService
	groups []service.Group
	err    error
	calls  int
}

func (s *qosGroupNameStub) GetAllGroupsIncludingInactive(ctx context.Context) ([]service.Group, error) {
	s.calls++
	_, bounded := ctx.Deadline()
	if !bounded {
		return nil, errors.New("unbounded metadata lookup")
	}
	return s.groups, s.err
}

func TestGPTQoSSnapshotLabelsGroupsWithoutChangingHistoricalTier(t *testing.T) {
	svc := &qosGroupNameStub{groups: []service.Group{
		{ID: 7, Name: "月卡分组", Status: service.StatusActive},
		{ID: 8, Name: "已停用分组", Status: service.StatusDisabled},
	}}
	rows := map[string][]service.GPTQoSGroupMetrics{
		"premium":  {{GroupID: 7, GPTQoSTierMetrics: service.GPTQoSTierMetrics{Requests: 10}}, {GroupID: 9}},
		"standard": {{GroupID: 7}, {GroupID: 8}},
	}
	h := &AccountHandler{adminService: svc}
	h.labelGPTQoSGroups(context.Background(), map[string]any{"groups": rows})
	require.Equal(t, 1, svc.calls, "one admin metadata query covers all groups and tiers")
	require.Equal(t, "月卡分组", rows["premium"][0].GroupName)
	require.Equal(t, "月卡分组", rows["standard"][0].GroupName)
	require.Equal(t, "已停用分组", rows["standard"][1].GroupName)
	require.Empty(t, rows["premium"][1].GroupName, "deleted group retains its ID as the UI fallback")
	require.Equal(t, int64(9), rows["premium"][1].GroupID)
	require.Equal(t, 10, rows["premium"][0].Requests)
}

func TestGPTQoSSnapshotGroupNameFailurePreservesMetrics(t *testing.T) {
	svc := &qosGroupNameStub{err: errors.New("metadata unavailable")}
	h := &AccountHandler{adminService: svc}
	rows := map[string][]service.GPTQoSGroupMetrics{"premium": {{GroupID: 7, GPTQoSTierMetrics: service.GPTQoSTierMetrics{Requests: 3}}}}
	h.labelGPTQoSGroups(context.Background(), map[string]any{"groups": rows})
	require.Equal(t, 3, rows["premium"][0].Requests)
	require.Equal(t, int64(7), rows["premium"][0].GroupID)
	require.Empty(t, rows["premium"][0].GroupName)
	h.labelGPTQoSGroups(context.Background(), map[string]any{"groups": map[string][]service.GPTQoSGroupMetrics{}})
	require.Equal(t, 1, svc.calls, "empty observations do not query group metadata")
}
