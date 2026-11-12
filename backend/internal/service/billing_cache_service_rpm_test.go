//go:build unit

package service

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
)

// userRPMCacheStub 记录每种计数器被调用的次数，并可注入返回值与错误。
type userRPMCacheStub struct {
	userGroupCalls int32
	userCalls      int32

	userGroupCounts []int // 依次返回的计数值
	userGroupErr    error
	userCounts      []int
	userErr         error
}

func (s *userRPMCacheStub) IncrementUserGroupRPM(_ context.Context, _, _ int64) (int, error) {
	idx := int(atomic.AddInt32(&s.userGroupCalls, 1)) - 1
	if s.userGroupErr != nil {
		return 0, s.userGroupErr
	}
	if idx < len(s.userGroupCounts) {
		return s.userGroupCounts[idx], nil
	}
	return 1, nil
}

func (s *userRPMCacheStub) IncrementUserRPM(_ context.Context, _ int64) (int, error) {
	idx := int(atomic.AddInt32(&s.userCalls, 1)) - 1
	if s.userErr != nil {
		return 0, s.userErr
	}
	if idx < len(s.userCounts) {
		return s.userCounts[idx], nil
	}
	return 1, nil
}

func (s *userRPMCacheStub) GetUserGroupRPM(_ context.Context, _, _ int64) (int, error) {
	return 0, nil
}

func (s *userRPMCacheStub) GetUserRPM(_ context.Context, _ int64) (int, error) {
	return 0, nil
}

// rpmOverrideRepoStub 专用于 checkRPM 分支测试，只实现必要方法。
type rpmOverrideRepoStub struct {
	UserGroupRateRepository

	override *int
	err      error
	calls    int32
}

func (s *rpmOverrideRepoStub) GetRPMOverrideByUserAndGroup(_ context.Context, _, _ int64) (*int, error) {
	atomic.AddInt32(&s.calls, 1)
	if s.err != nil {
		return nil, s.err
	}
	return s.override, nil
}

func newBillingServiceForRPM(t *testing.T, cache UserRPMCache, rateRepo UserGroupRateRepository) *BillingCacheService {
	t.Helper()
	// 用 nil BillingCache 走 "无缓存" 分支，避免 CheckBillingEligibility 副作用。
	// 我们只直接测 checkRPM。
	svc := NewBillingCacheService(nil, nil, nil, nil, cache, rateRepo, &config.Config{}, nil)
	t.Cleanup(svc.Stop)
	return svc
}

func TestBillingCacheService_CheckRPM_OverrideTakesPrecedenceOverGroup(t *testing.T) {
	override := 2
	// 专属限额覆盖分组默认值和用户兜底值。
	cache := &userRPMCacheStub{userGroupCounts: []int{1, 2, 3}}
	repo := &rpmOverrideRepoStub{override: &override}
	svc := newBillingServiceForRPM(t, cache, repo)

	user := &User{ID: 1, RPMLimit: 100}
	group := &Group{ID: 10, RPMLimit: 100}

	require.NoError(t, svc.checkRPM(context.Background(), user, group))
	require.NoError(t, svc.checkRPM(context.Background(), user, group))
	require.ErrorIs(t, svc.checkRPM(context.Background(), user, group), ErrGroupRPMExceeded)

	require.EqualValues(t, 3, atomic.LoadInt32(&cache.userGroupCalls), "override 命中分支应走 user-group 计数")
	require.EqualValues(t, 0, atomic.LoadInt32(&cache.userCalls), "专属限额命中后不应消耗用户兜底额度")
	require.EqualValues(t, 3, atomic.LoadInt32(&repo.calls))
}

func TestBillingCacheService_CheckRPM_OverrideTakesPrecedenceOverUser(t *testing.T) {
	override := 3
	cache := &userRPMCacheStub{userGroupCounts: []int{1, 2, 3, 4}, userCounts: []int{3}}
	repo := &rpmOverrideRepoStub{override: &override}
	svc := newBillingServiceForRPM(t, cache, repo)

	user := &User{ID: 1, RPMLimit: 2}
	group := &Group{ID: 10, RPMLimit: 1}

	require.NoError(t, svc.checkRPM(context.Background(), user, group))
	require.NoError(t, svc.checkRPM(context.Background(), user, group))
	require.NoError(t, svc.checkRPM(context.Background(), user, group), "专属额度应允许请求超过用户和分组默认限额")
	require.ErrorIs(t, svc.checkRPM(context.Background(), user, group), ErrGroupRPMExceeded)
	require.EqualValues(t, 0, atomic.LoadInt32(&cache.userCalls))
}

func TestBillingCacheService_CheckRPM_OverrideZeroSkipsGroupAndUser(t *testing.T) {
	for _, tc := range []struct {
		name       string
		groupLimit int
	}{
		{name: "group limited", groupLimit: 100},
		{name: "group unlimited", groupLimit: 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			zero := 0
			cache := &userRPMCacheStub{userCounts: []int{6}}
			repo := &rpmOverrideRepoStub{override: &zero}
			svc := newBillingServiceForRPM(t, cache, repo)

			user := &User{ID: 1, RPMLimit: 5}
			group := &Group{ID: 10, RPMLimit: tc.groupLimit}

			for i := 0; i < 6; i++ {
				require.NoError(t, svc.checkRPM(context.Background(), user, group), "request %d should pass", i+1)
			}
			require.EqualValues(t, 0, atomic.LoadInt32(&cache.userGroupCalls), "override=0 不应触发分组计数器")
			require.EqualValues(t, 0, atomic.LoadInt32(&cache.userCalls), "override=0 不应回落到用户限额")
		})
	}
}

func TestBillingCacheService_CheckRPM_OverrideZeroAndUserZeroIsFullyUnlimited(t *testing.T) {
	zero := 0
	cache := &userRPMCacheStub{}
	repo := &rpmOverrideRepoStub{override: &zero}
	svc := newBillingServiceForRPM(t, cache, repo)

	user := &User{ID: 1, RPMLimit: 0} // user 也不限
	group := &Group{ID: 10, RPMLimit: 100}

	for i := 0; i < 50; i++ {
		require.NoError(t, svc.checkRPM(context.Background(), user, group))
	}
	require.EqualValues(t, 0, atomic.LoadInt32(&cache.userGroupCalls), "override=0 不触发分组计数")
	require.EqualValues(t, 0, atomic.LoadInt32(&cache.userCalls), "user.RPMLimit=0 也不触发用户计数")
}

func TestBillingCacheService_CheckRPM_CachedOverrideTakesPrecedence(t *testing.T) {
	for _, tc := range []struct {
		name     string
		override int
		wantErr  error
	}{
		{name: "limited", override: 3, wantErr: ErrGroupRPMExceeded},
		{name: "unlimited", override: 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cache := &userRPMCacheStub{userGroupCounts: []int{1, 2, 3, 4}, userCounts: []int{2}}
			repo := &rpmOverrideRepoStub{err: errors.New("must use cached override")}
			svc := newBillingServiceForRPM(t, cache, repo)

			user := &User{ID: 1, RPMLimit: 1, UserGroupRPMOverride: &tc.override}
			group := &Group{ID: 10, RPMLimit: 1}

			for i := 0; i < 3; i++ {
				require.NoError(t, svc.checkRPM(context.Background(), user, group))
			}
			err := svc.checkRPM(context.Background(), user, group)
			if tc.wantErr != nil {
				require.ErrorIs(t, err, tc.wantErr)
				require.EqualValues(t, 4, atomic.LoadInt32(&cache.userGroupCalls))
			} else {
				require.NoError(t, err)
				require.EqualValues(t, 0, atomic.LoadInt32(&cache.userGroupCalls))
			}
			require.EqualValues(t, 0, atomic.LoadInt32(&cache.userCalls))
			require.EqualValues(t, 0, atomic.LoadInt32(&repo.calls))
		})
	}
}

func TestBillingCacheService_CheckRPM_NilOverrideUsesGroupInsteadOfUser(t *testing.T) {
	cache := &userRPMCacheStub{userGroupCounts: []int{60, 61, 120, 121}, userCounts: []int{61}}
	repo := &rpmOverrideRepoStub{override: nil}
	svc := newBillingServiceForRPM(t, cache, repo)

	user := &User{ID: 1, RPMLimit: 60}
	group := &Group{ID: 10, RPMLimit: 120}

	require.NoError(t, svc.checkRPM(context.Background(), user, group))                      // 60
	require.NoError(t, svc.checkRPM(context.Background(), user, group))                      // 61，超过用户兜底值仍可用
	require.NoError(t, svc.checkRPM(context.Background(), user, group))                      // 120
	require.ErrorIs(t, svc.checkRPM(context.Background(), user, group), ErrGroupRPMExceeded) // 121

	require.EqualValues(t, 4, atomic.LoadInt32(&cache.userGroupCalls))
	require.EqualValues(t, 0, atomic.LoadInt32(&cache.userCalls), "分组限额已配置时不应消耗用户兜底额度")
}

func TestBillingCacheService_CheckRPM_OverrideLookupErrorFallsThroughToGroup(t *testing.T) {
	cache := &userRPMCacheStub{userGroupCounts: []int{3}}
	repo := &rpmOverrideRepoStub{err: errors.New("db down")}
	svc := newBillingServiceForRPM(t, cache, repo)

	user := &User{ID: 1, RPMLimit: 1}
	group := &Group{ID: 10, RPMLimit: 10}

	// override 查询失败后应继续尝试 group 分支（不直接拒绝）
	require.NoError(t, svc.checkRPM(context.Background(), user, group))
	require.EqualValues(t, 1, atomic.LoadInt32(&cache.userGroupCalls))
	require.EqualValues(t, 0, atomic.LoadInt32(&cache.userCalls))
	require.EqualValues(t, 1, atomic.LoadInt32(&repo.calls))
}

func TestBillingCacheService_CheckRPM_UserLevelFallbackWhenGroupUnlimited(t *testing.T) {
	cache := &userRPMCacheStub{userCounts: []int{1, 2, 3}}
	repo := &rpmOverrideRepoStub{override: nil}
	svc := newBillingServiceForRPM(t, cache, repo)

	user := &User{ID: 1, RPMLimit: 2}
	group := &Group{ID: 10, RPMLimit: 0} // 分组未设限

	require.NoError(t, svc.checkRPM(context.Background(), user, group))
	require.NoError(t, svc.checkRPM(context.Background(), user, group))
	require.ErrorIs(t, svc.checkRPM(context.Background(), user, group), ErrUserRPMExceeded)

	require.EqualValues(t, 0, atomic.LoadInt32(&cache.userGroupCalls), "group 未设限时不应 INCR user-group 键")
	require.EqualValues(t, 3, atomic.LoadInt32(&cache.userCalls))
}

func TestBillingCacheService_CheckRPM_NoLimitsConfiguredIsNoop(t *testing.T) {
	cache := &userRPMCacheStub{}
	repo := &rpmOverrideRepoStub{override: nil}
	svc := newBillingServiceForRPM(t, cache, repo)

	user := &User{ID: 1, RPMLimit: 0}
	group := &Group{ID: 10, RPMLimit: 0}

	for i := 0; i < 10; i++ {
		require.NoError(t, svc.checkRPM(context.Background(), user, group))
	}
	require.EqualValues(t, 0, atomic.LoadInt32(&cache.userGroupCalls))
	require.EqualValues(t, 0, atomic.LoadInt32(&cache.userCalls))
}

func TestBillingCacheService_CheckRPM_RedisErrorFailOpen(t *testing.T) {
	cache := &userRPMCacheStub{userGroupErr: errors.New("redis unavailable")}
	repo := &rpmOverrideRepoStub{override: nil}
	svc := newBillingServiceForRPM(t, cache, repo)

	user := &User{ID: 1, RPMLimit: 1}
	group := &Group{ID: 10, RPMLimit: 5}

	// Redis 故障时应 fail-open，不拒绝请求
	require.NoError(t, svc.checkRPM(context.Background(), user, group))
	require.EqualValues(t, 1, atomic.LoadInt32(&cache.userGroupCalls))
	require.EqualValues(t, 0, atomic.LoadInt32(&cache.userCalls), "分组计数失败应放行，不回落到用户限额")
}

func TestBillingCacheService_CheckRPM_NoGroupUsesUserOnly(t *testing.T) {
	cache := &userRPMCacheStub{userCounts: []int{1, 2, 3}}
	repo := &rpmOverrideRepoStub{}
	svc := newBillingServiceForRPM(t, cache, repo)

	user := &User{ID: 1, RPMLimit: 2}

	// 无 group（纯用户级限流场景），不应查询 rpm_override。
	require.NoError(t, svc.checkRPM(context.Background(), user, nil))
	require.NoError(t, svc.checkRPM(context.Background(), user, nil))
	require.ErrorIs(t, svc.checkRPM(context.Background(), user, nil), ErrUserRPMExceeded)

	require.EqualValues(t, 0, atomic.LoadInt32(&repo.calls), "无 group 时不应查询 rpm_override")
	require.EqualValues(t, 3, atomic.LoadInt32(&cache.userCalls))
}

func TestBillingCacheService_CheckRPM_NilUserIsNoop(t *testing.T) {
	cache := &userRPMCacheStub{}
	repo := &rpmOverrideRepoStub{}
	svc := newBillingServiceForRPM(t, cache, repo)

	require.NoError(t, svc.checkRPM(context.Background(), nil, &Group{ID: 1, RPMLimit: 10}))
	require.EqualValues(t, 0, atomic.LoadInt32(&cache.userGroupCalls))
	require.EqualValues(t, 0, atomic.LoadInt32(&cache.userCalls))
	require.EqualValues(t, 0, atomic.LoadInt32(&repo.calls))
}
