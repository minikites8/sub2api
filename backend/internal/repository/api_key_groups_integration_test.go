//go:build integration

package repository

import (
	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
	"github.com/Wei-Shaw/sub2api/internal/service"
)

func (s *APIKeyRepoSuite) TestOrderedGroupBindings() {
	user := s.mustCreateUser("ordered-groups@test.com")
	first := s.mustCreateGroup("ordered-first")
	second := s.mustCreateGroup("ordered-second")
	third := s.mustCreateGroup("ordered-third")
	key := &service.APIKey{UserID: user.ID, Key: "sk-ordered", Name: "ordered", Status: service.StatusActive,
		GroupID: &first.ID, GroupIDs: []int64{first.ID, second.ID, third.ID}}
	s.Require().NoError(s.repo.Create(s.ctx, key))
	auth, err := s.repo.GetByKeyForAuth(s.ctx, key.Key)
	s.Require().NoError(err)
	s.Require().Equal(key.GroupIDs, auth.GroupIDs)
	keys, _, err := s.repo.ListByUserID(s.ctx, user.ID, pagination.PaginationParams{Page: 1, PageSize: 10}, service.APIKeyListFilters{GroupID: &second.ID})
	s.Require().NoError(err)
	s.Require().Len(keys, 1)
	cachedKeys, err := s.repo.ListKeysByGroupID(s.ctx, second.ID)
	s.Require().NoError(err)
	s.Require().Equal([]string{key.Key}, cachedKeys)
	count, err := s.repo.CountByGroupID(s.ctx, second.ID)
	s.Require().NoError(err)
	s.Require().EqualValues(1, count)

	// Moving to an already bound group deduplicates while preserving priority.
	n, err := s.repo.UpdateGroupIDByUserAndGroup(s.ctx, user.ID, first.ID, second.ID)
	s.Require().NoError(err)
	s.Require().EqualValues(1, n)
	updated, err := s.repo.GetByID(s.ctx, key.ID)
	s.Require().NoError(err)
	s.Require().Equal([]int64{second.ID, third.ID}, updated.GroupIDs)
	s.Require().Equal(second.ID, *updated.GroupID)

	n, err = s.repo.ClearGroupIDByGroupID(s.ctx, second.ID)
	s.Require().NoError(err)
	s.Require().EqualValues(1, n)
	updated, err = s.repo.GetByID(s.ctx, key.ID)
	s.Require().NoError(err)
	s.Require().Equal([]int64{third.ID}, updated.GroupIDs)
	s.Require().Equal(third.ID, *updated.GroupID)

	updated.GroupID = nil
	updated.GroupIDs = []int64{}
	s.Require().NoError(s.repo.Update(s.ctx, updated, service.APIKeyUpdateFields{GroupID: true}))
	updated, err = s.repo.GetByID(s.ctx, key.ID)
	s.Require().NoError(err)
	s.Require().Empty(updated.OrderedGroupIDs())
	s.Require().Nil(updated.GroupID)
}
