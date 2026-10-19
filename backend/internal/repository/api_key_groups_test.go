package repository

import (
	"context"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func TestAPIKeyOrderedGroupPersistenceAndFilters(t *testing.T) {
	repo, client := newAPIKeyRepoSQLite(t)
	ctx := context.Background()
	user := mustCreateAPIKeyRepoUser(t, ctx, client, "ordered-groups@test.com")
	first, err := client.Group.Create().SetName("ordered-first").Save(ctx)
	require.NoError(t, err)
	second, err := client.Group.Create().SetName("ordered-second").Save(ctx)
	require.NoError(t, err)
	key := &service.APIKey{UserID: user.ID, Key: "sk-ordered", Name: "ordered", Status: service.StatusActive,
		GroupID: &second.ID, GroupIDs: []int64{second.ID, first.ID}}
	require.NoError(t, repo.Create(ctx, key))
	got, err := repo.GetByKeyForAuth(ctx, key.Key)
	require.NoError(t, err)
	require.Equal(t, key.GroupIDs, got.GroupIDs)
	require.Equal(t, second.ID, *got.GroupID)
	keys, _, err := repo.ListByUserID(ctx, user.ID, pagination.PaginationParams{Page: 1, PageSize: 10}, service.APIKeyListFilters{GroupID: &first.ID})
	require.NoError(t, err)
	require.Len(t, keys, 1, "secondary bindings participate in filtering")
	cachedKeys, err := repo.ListKeysByGroupID(ctx, first.ID)
	require.NoError(t, err)
	require.Equal(t, []string{key.Key}, cachedKeys, "secondary group edits invalidate this key")
	count, err := repo.CountByGroupID(ctx, first.ID)
	require.NoError(t, err)
	require.EqualValues(t, 1, count)
	key.GroupID = &first.ID
	key.GroupIDs = []int64{first.ID, second.ID}
	require.NoError(t, repo.Update(ctx, key, service.APIKeyUpdateFields{GroupID: true}))
	got, err = repo.GetByID(ctx, key.ID)
	require.NoError(t, err)
	require.Equal(t, key.GroupIDs, got.GroupIDs)
	require.Equal(t, first.ID, *got.GroupID)
}
