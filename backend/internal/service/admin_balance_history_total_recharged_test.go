//go:build unit

package service

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestAdminServiceUserTotalRechargedUsesUserAggregate(t *testing.T) {
	t.Parallel()

	svc := &adminServiceImpl{
		userRepo:       &userRepoStub{user: &User{ID: 7, TotalRecharged: 123.45}},
		redeemCodeRepo: &redeemRepoStub{},
	}

	total, err := svc.userTotalRecharged(context.Background(), 7)
	require.NoError(t, err)
	require.Equal(t, 123.45, total)
}
