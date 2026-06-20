//go:build unit

package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
)

type fakeDailyCheckinRepo struct {
	checkin       *DailyCheckinRecord
	latest        *DailyCheckinRecord
	total         float64
	claimResult   *DailyCheckinClaimResult
	claimErr      error
	claimInputs   []DailyCheckinClaimInput
	sumCallCount  int
	userCallCount int
}

func (r *fakeDailyCheckinRepo) GetUserCheckin(context.Context, int64, string) (*DailyCheckinRecord, error) {
	r.userCallCount++
	return r.checkin, nil
}

func (r *fakeDailyCheckinRepo) GetUserLatestCheckin(context.Context, int64) (*DailyCheckinRecord, error) {
	return r.latest, nil
}

func (r *fakeDailyCheckinRepo) SumRewardsByDate(context.Context, string) (float64, error) {
	r.sumCallCount++
	return r.total, nil
}

func (r *fakeDailyCheckinRepo) Claim(_ context.Context, input DailyCheckinClaimInput) (*DailyCheckinClaimResult, error) {
	r.claimInputs = append(r.claimInputs, input)
	if r.claimErr != nil {
		return nil, r.claimErr
	}
	if r.claimResult != nil {
		return r.claimResult, nil
	}
	return &DailyCheckinClaimResult{
		Record: DailyCheckinRecord{
			UserID:    input.UserID,
			Date:      input.Date,
			Reward:    input.Reward,
			CreatedAt: time.Now(),
		},
		TodayTotalGranted: r.total + input.Reward,
		Balance:           10 + input.Reward,
	}, nil
}

func TestDailyCheckinGetStatusDisabledWhenConfigNotClaimable(t *testing.T) {
	repo := &fakeDailyCheckinRepo{}
	svc := NewDailyCheckinService(repo, &config.Config{
		DailyCheckin: config.DailyCheckinConfig{
			Enabled:         true,
			DailyTotalLimit: 0,
			MinReward:       0,
			MaxReward:       1,
		},
	}, nil)

	status, err := svc.GetStatus(context.Background(), 1)
	require.NoError(t, err)
	require.False(t, status.Enabled)
	require.Equal(t, 0.0, status.DailyTotalLimit)
	require.Equal(t, 1.0, status.MaxReward)
	require.Equal(t, 1, repo.userCallCount)
	require.Equal(t, 1, repo.sumCallCount)
}

func TestDailyCheckinClaimDisabledWhenConfigNotClaimable(t *testing.T) {
	repo := &fakeDailyCheckinRepo{}
	svc := NewDailyCheckinService(repo, &config.Config{
		DailyCheckin: config.DailyCheckinConfig{
			Enabled:         true,
			DailyTotalLimit: 1,
			MinReward:       2,
			MaxReward:       2,
		},
	}, nil)

	result, err := svc.Claim(context.Background(), 1)
	require.Nil(t, result)
	require.ErrorIs(t, err, ErrDailyCheckinDisabled)
	require.Empty(t, repo.claimInputs)
	require.Equal(t, 0, repo.sumCallCount)
}

func TestDailyCheckinClaimExhaustedWhenRemainingBelowMinimum(t *testing.T) {
	repo := &fakeDailyCheckinRepo{total: 0.95}
	svc := NewDailyCheckinService(repo, &config.Config{
		DailyCheckin: config.DailyCheckinConfig{
			Enabled:         true,
			DailyTotalLimit: 1,
			MinReward:       0.1,
			MaxReward:       0.2,
		},
	}, nil)

	result, err := svc.Claim(context.Background(), 1)
	require.Nil(t, result)
	require.ErrorIs(t, err, ErrDailyCheckinExhausted)
	require.Empty(t, repo.claimInputs)
	require.Equal(t, 1, repo.sumCallCount)
}

func TestDailyCheckinClaimPropagatesAlreadyCheckedIn(t *testing.T) {
	repo := &fakeDailyCheckinRepo{
		total:    0.2,
		claimErr: ErrDailyCheckinAlready,
	}
	svc := NewDailyCheckinService(repo, &config.Config{
		DailyCheckin: config.DailyCheckinConfig{
			Enabled:         true,
			DailyTotalLimit: 1,
			MinReward:       0.1,
			MaxReward:       0.2,
		},
	}, nil)

	result, err := svc.Claim(context.Background(), 1)
	require.Nil(t, result)
	require.ErrorIs(t, err, ErrDailyCheckinAlready)
	require.Len(t, repo.claimInputs, 1)
}

func TestDailyCheckinClaimRewardWithinRangeAndUpdatesStatus(t *testing.T) {
	repo := &fakeDailyCheckinRepo{total: 0.2}
	svc := NewDailyCheckinService(repo, &config.Config{
		Timezone: "Asia/Shanghai",
		DailyCheckin: config.DailyCheckinConfig{
			Enabled:         true,
			DailyTotalLimit: 1,
			MinReward:       0.1,
			MaxReward:       0.2,
		},
	}, nil)

	result, err := svc.Claim(context.Background(), 42)
	require.NoError(t, err)
	require.NotNil(t, result)
	require.True(t, result.Enabled)
	require.True(t, result.CheckedInToday)
	require.GreaterOrEqual(t, result.Reward, 0.1)
	require.LessOrEqual(t, result.Reward, 0.2)
	require.Equal(t, result.Reward, result.TodayReward)
	require.Equal(t, roundCheckinReward(0.2+result.Reward), result.TodayTotalGranted)
	require.Equal(t, roundCheckinReward(10+result.Reward), result.Balance)
	require.Len(t, repo.claimInputs, 1)
	require.Equal(t, int64(42), repo.claimInputs[0].UserID)
	require.Equal(t, 0.2, repo.claimInputs[0].GrantedSoFar)
}

func TestDailyCheckinClaimWrapsUnexpectedRepositoryError(t *testing.T) {
	repoErr := errors.New("repository unavailable")
	repo := &fakeDailyCheckinRepo{claimErr: repoErr}
	svc := NewDailyCheckinService(repo, &config.Config{
		DailyCheckin: config.DailyCheckinConfig{
			Enabled:         true,
			DailyTotalLimit: 1,
			MinReward:       0.1,
			MaxReward:       0.2,
		},
	}, nil)

	result, err := svc.Claim(context.Background(), 1)
	require.Nil(t, result)
	require.ErrorIs(t, err, repoErr)
}
