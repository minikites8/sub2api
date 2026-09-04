package dto

import (
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func TestUserFromServiceShallowSplitsBalanceAndPreservesGiftExpiry(t *testing.T) {
	expiresAt := time.Date(2026, 9, 30, 23, 59, 59, 0, time.UTC)
	user := UserFromServiceShallow(&service.User{
		Balance:                 12.5,
		GiftBalance:             2.5,
		GiftBalanceExpiresAt:    &expiresAt,
		RegistrationGiftBalance: 1,
		DailyCheckinBalance:     1.5,
		DailyCheckinExpiresAt:   &expiresAt,
	})

	require.Equal(t, 12.5, user.Balance)
	require.Equal(t, 10.0, user.RechargeBalance)
	require.Equal(t, 2.5, user.GiftBalance)
	require.Equal(t, &expiresAt, user.GiftBalanceExpiresAt)
	require.Equal(t, 1.0, user.RegistrationGiftBalance)
	require.Equal(t, 1.5, user.DailyCheckinBalance)
	require.Equal(t, &expiresAt, user.DailyCheckinExpiresAt)
}

func TestUserFromServiceShallowClampsRechargeBalanceAtZero(t *testing.T) {
	user := UserFromServiceShallow(&service.User{Balance: 1, GiftBalance: 2})
	require.Zero(t, user.RechargeBalance)
}
