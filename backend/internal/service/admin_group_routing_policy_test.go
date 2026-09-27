package service

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

type groupRoutingPolicyRepo struct {
	GroupRepository
	group  *Group
	writes int
}

func (r *groupRoutingPolicyRepo) Create(_ context.Context, g *Group) error {
	g.ID = 7
	r.group = g
	r.writes++
	return nil
}
func (r *groupRoutingPolicyRepo) GetByID(_ context.Context, _ int64) (*Group, error) {
	g := *r.group
	return &g, nil
}
func (r *groupRoutingPolicyRepo) Update(_ context.Context, g *Group) error {
	r.group = g
	r.writes++
	return nil
}

func TestGroupRoutingPolicyCreateUpdateAndDuplicate(t *testing.T) {
	repo := &groupRoutingPolicyRepo{}
	svc := &adminServiceImpl{groupRepo: repo}
	group, err := svc.CreateGroup(context.Background(), &CreateGroupInput{Name: "routing", Platform: PlatformOpenAI, RateMultiplier: 1})
	require.NoError(t, err)
	require.False(t, group.EnableBPS)
	require.Equal(t, GroupSchedulingBalanced, group.SchedulingStrategy)
	enabled, strategy := true, GroupSchedulingPriorityWeekly
	group, err = svc.UpdateGroup(context.Background(), group.ID, &UpdateGroupInput{EnableBPS: &enabled, SchedulingStrategy: &strategy})
	require.NoError(t, err)
	require.True(t, group.EnableBPS)
	require.Equal(t, strategy, group.SchedulingStrategy)
	copy := cloneGroupForDuplicate(group, "routing-copy")
	require.True(t, copy.EnableBPS)
	require.Equal(t, strategy, copy.SchedulingStrategy)
	group, err = svc.UpdateGroup(context.Background(), group.ID, &UpdateGroupInput{Name: "renamed"})
	require.NoError(t, err)
	require.True(t, group.EnableBPS)
	require.Equal(t, strategy, group.SchedulingStrategy)
	enabled, strategy = false, GroupSchedulingPriority5h
	group, err = svc.UpdateGroup(context.Background(), group.ID, &UpdateGroupInput{EnableBPS: &enabled, SchedulingStrategy: &strategy})
	require.NoError(t, err)
	require.False(t, group.EnableBPS)
	require.Equal(t, strategy, group.SchedulingStrategy)
	writes := repo.writes
	strategy = "invalid"
	_, err = svc.UpdateGroup(context.Background(), group.ID, &UpdateGroupInput{SchedulingStrategy: &strategy})
	require.Error(t, err)
	_, err = svc.CreateGroup(context.Background(), &CreateGroupInput{Name: "bad", Platform: PlatformOpenAI, RateMultiplier: 1, SchedulingStrategy: strategy})
	require.Error(t, err)
	require.Equal(t, writes, repo.writes)
}

func TestGroupRoutingPolicyAuthCacheRoundtrip(t *testing.T) {
	groupID := int64(7)
	apiKey := &APIKey{ID: 2, UserID: 1, GroupID: &groupID, Key: "sk-group-policy", Status: StatusActive, User: &User{ID: 1, Status: StatusActive}, Group: &Group{ID: groupID, Name: "policy", Platform: PlatformOpenAI, Status: StatusActive, Hydrated: true, EnableBPS: true, SchedulingStrategy: GroupSchedulingPriorityWeekly}}
	svc := &APIKeyService{}
	payload, err := json.Marshal(&APIKeyAuthCacheEntry{Snapshot: svc.snapshotFromAPIKey(context.Background(), apiKey)})
	require.NoError(t, err)
	var entry APIKeyAuthCacheEntry
	require.NoError(t, json.Unmarshal(payload, &entry))
	restored, used, err := svc.applyAuthCacheEntry(apiKey.Key, &entry)
	require.NoError(t, err)
	require.True(t, used)
	require.True(t, restored.Group.EnableBPS)
	require.Equal(t, GroupSchedulingPriorityWeekly, restored.Group.SchedulingStrategy)
}
