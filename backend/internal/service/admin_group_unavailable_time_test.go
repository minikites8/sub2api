//go:build unit

package service

import (
	"context"
	"github.com/Wei-Shaw/sub2api/internal/pkg/timezone"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestAdminGroupUnavailableTimePersistenceAndPartialUpdate(t *testing.T) {
	repo := &groupRepoStubForAdmin{}
	invalidator := &authCacheInvalidatorStub{}
	svc := &adminServiceImpl{groupRepo: repo, authCacheInvalidator: invalidator}
	created, err := svc.CreateGroup(context.Background(), &CreateGroupInput{Name: "Night", Platform: PlatformOpenAI, RateMultiplier: 1, SubscriptionType: "subscription", UnavailableTimeEnabled: true, UnavailableStart: "08:00", UnavailableEnd: "22:00"})
	require.NoError(t, err)
	require.True(t, repo.created.UnavailableTimeEnabled)
	require.Equal(t, "08:00", created.UnavailableStart)
	require.Equal(t, "22:00", created.UnavailableEnd)
	repo.getByID = created
	end := "23:00"
	updated, err := svc.UpdateGroup(context.Background(), created.ID, &UpdateGroupInput{UnavailableEnd: &end})
	require.NoError(t, err)
	require.Equal(t, "08:00", updated.UnavailableStart)
	require.Equal(t, end, updated.UnavailableEnd)
	require.NotEmpty(t, invalidator.groupIDs)
	end = "08:00"
	_, err = svc.UpdateGroup(context.Background(), created.ID, &UpdateGroupInput{UnavailableEnd: &end})
	require.Error(t, err)
	kind := "standard"
	updated, err = svc.UpdateGroup(context.Background(), created.ID, &UpdateGroupInput{SubscriptionType: kind})
	require.NoError(t, err)
	require.False(t, updated.UnavailableTimeEnabled)
	require.Empty(t, updated.UnavailableStart)
	require.Empty(t, updated.UnavailableEnd)
}

func TestOrderedGroupsSkipUnavailableTime(t *testing.T) {
	svc := orderedGroupsService()
	blocked := svc.groupRepo.(*orderedGroupsRepo).groups[1]
	blocked.SubscriptionType = "subscription"
	blocked.UnavailableTimeEnabled = true
	now := time.Now().In(timezone.Location())
	blocked.UnavailableStart = now.Add(-time.Hour).Format("15:04")
	blocked.UnavailableEnd = now.Add(time.Hour).Format("15:04")
	key := &APIKey{GroupIDs: []int64{1, 2}, User: &User{ID: 7}}
	selected, err := svc.SelectOrderedGroup(context.Background(), key, APIKeyGroupSelection{}, nil)
	require.NoError(t, err)
	require.EqualValues(t, 2, *selected.GroupID)
	second := svc.groupRepo.(*orderedGroupsRepo).groups[2]
	second.SubscriptionType = "subscription"
	second.UnavailableTimeEnabled = true
	second.UnavailableStart = blocked.UnavailableStart
	second.UnavailableEnd = blocked.UnavailableEnd
	_, err = svc.SelectOrderedGroup(context.Background(), key, APIKeyGroupSelection{}, nil)
	require.ErrorIs(t, err, ErrAPIKeyGroupsUnavailable)
}
