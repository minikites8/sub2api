//go:build unit

package service_test

import (
	"context"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func TestAuthServiceRegister_DeductsGiftBalanceFromRiskyFreeAccounts(t *testing.T) {
	svc, _, client := newAuthServiceWithEnt(t, map[string]string{
		service.SettingKeyRegistrationEnabled:             "true",
		service.SettingKeySignupIPRiskControlThreshold:    "3",
		service.SettingKeySignupIPDisablePreviousAccounts: "true",
		service.SettingKeySignupIPKeepPreviousAccounts:    "1",
	}, nil)

	signupCtx := func() context.Context {
		return service.WithSignupIP(context.Background(), "1.2.3.4")
	}

	_, firstUser, err := svc.Register(signupCtx(), "signup-ip-first@example.com", "password")
	require.NoError(t, err)
	require.NotNil(t, firstUser)
	require.Equal(t, service.StatusActive, firstUser.Status)

	_, secondUser, err := svc.Register(signupCtx(), "signup-ip-second@example.com", "password")
	require.NoError(t, err)
	require.NotNil(t, secondUser)
	require.Equal(t, service.StatusActive, secondUser.Status)

	_, thirdUser, err := svc.Register(signupCtx(), "signup-ip-third@example.com", "password")
	require.NoError(t, err)
	require.NotNil(t, thirdUser)
	require.Equal(t, service.StatusActive, thirdUser.Status)
	require.Zero(t, thirdUser.Balance)

	storedFirstUser, err := client.User.Get(context.Background(), firstUser.ID)
	require.NoError(t, err)
	require.Equal(t, service.StatusActive, storedFirstUser.Status)
	require.NotNil(t, storedFirstUser.SignupIP)
	require.Equal(t, "1.2.3.4", *storedFirstUser.SignupIP)

	storedSecondUser, err := client.User.Get(context.Background(), secondUser.ID)
	require.NoError(t, err)
	require.Equal(t, service.StatusActive, storedSecondUser.Status)
	require.Zero(t, storedSecondUser.Balance)

	storedThirdUser, err := client.User.Get(context.Background(), thirdUser.ID)
	require.NoError(t, err)
	require.Equal(t, service.StatusActive, storedThirdUser.Status)
	require.Zero(t, storedThirdUser.Balance)
}

func TestAuthServiceRegister_DeductsOnlyCurrentGiftBalanceWhenPreviousProcessingDisabled(t *testing.T) {
	svc, _, client := newAuthServiceWithEnt(t, map[string]string{
		service.SettingKeyRegistrationEnabled:             "true",
		service.SettingKeySignupIPRiskControlThreshold:    "2",
		service.SettingKeySignupIPDisablePreviousAccounts: "false",
		service.SettingKeySignupIPKeepPreviousAccounts:    "0",
	}, nil)

	signupCtx := func() context.Context {
		return service.WithSignupIP(context.Background(), "5.6.7.8")
	}

	_, firstUser, err := svc.Register(signupCtx(), "signup-ip-keep-first@example.com", "password")
	require.NoError(t, err)
	require.NotNil(t, firstUser)
	require.Equal(t, service.StatusActive, firstUser.Status)

	_, secondUser, err := svc.Register(signupCtx(), "signup-ip-disable-current@example.com", "password")
	require.NoError(t, err)
	require.NotNil(t, secondUser)
	require.Equal(t, service.StatusActive, secondUser.Status)
	require.Zero(t, secondUser.Balance)

	storedFirstUser, err := client.User.Get(context.Background(), firstUser.ID)
	require.NoError(t, err)
	require.Equal(t, service.StatusActive, storedFirstUser.Status)

	storedSecondUser, err := client.User.Get(context.Background(), secondUser.ID)
	require.NoError(t, err)
	require.Equal(t, service.StatusActive, storedSecondUser.Status)
	require.Zero(t, storedSecondUser.Balance)
}

func TestAuthServiceRegister_DeductsEarlierGiftBalancesBeyondKeepCount(t *testing.T) {
	svc, _, client := newAuthServiceWithEnt(t, map[string]string{
		service.SettingKeyRegistrationEnabled:             "true",
		service.SettingKeySignupIPRiskControlThreshold:    "4",
		service.SettingKeySignupIPDisablePreviousAccounts: "true",
		service.SettingKeySignupIPKeepPreviousAccounts:    "2",
	}, nil)

	signupCtx := func() context.Context {
		return service.WithSignupIP(context.Background(), "9.8.7.6")
	}

	_, firstUser, err := svc.Register(signupCtx(), "signup-ip-keep-one@example.com", "password")
	require.NoError(t, err)
	_, secondUser, err := svc.Register(signupCtx(), "signup-ip-keep-two@example.com", "password")
	require.NoError(t, err)
	_, thirdUser, err := svc.Register(signupCtx(), "signup-ip-disable-old@example.com", "password")
	require.NoError(t, err)
	_, fourthUser, err := svc.Register(signupCtx(), "signup-ip-disable-current@example.com", "password")
	require.NoError(t, err)
	require.NotNil(t, fourthUser)
	require.Equal(t, service.StatusActive, fourthUser.Status)
	require.Zero(t, fourthUser.Balance)

	storedFirstUser, err := client.User.Get(context.Background(), firstUser.ID)
	require.NoError(t, err)
	require.Equal(t, service.StatusActive, storedFirstUser.Status)

	storedSecondUser, err := client.User.Get(context.Background(), secondUser.ID)
	require.NoError(t, err)
	require.Equal(t, service.StatusActive, storedSecondUser.Status)

	storedThirdUser, err := client.User.Get(context.Background(), thirdUser.ID)
	require.NoError(t, err)
	require.Equal(t, service.StatusActive, storedThirdUser.Status)
	require.Zero(t, storedThirdUser.Balance)

	storedFourthUser, err := client.User.Get(context.Background(), fourthUser.ID)
	require.NoError(t, err)
	require.Equal(t, service.StatusActive, storedFourthUser.Status)
	require.Zero(t, storedFourthUser.Balance)
}

func TestAuthServiceRegister_SkipsPaidAccountsForSignupIPRiskControl(t *testing.T) {
	svc, _, client := newAuthServiceWithEnt(t, map[string]string{
		service.SettingKeyRegistrationEnabled:             "true",
		service.SettingKeySignupIPRiskControlThreshold:    "2",
		service.SettingKeySignupIPDisablePreviousAccounts: "true",
		service.SettingKeySignupIPKeepPreviousAccounts:    "0",
	}, nil)

	signupCtx := func() context.Context {
		return service.WithSignupIP(context.Background(), "4.3.2.1")
	}

	_, paidUser, err := svc.Register(signupCtx(), "signup-ip-paid@example.com", "password")
	require.NoError(t, err)
	_, err = client.User.UpdateOneID(paidUser.ID).SetTotalRecharged(10).Save(context.Background())
	require.NoError(t, err)

	_, freeUser, err := svc.Register(signupCtx(), "signup-ip-free@example.com", "password")
	require.NoError(t, err)
	require.Equal(t, service.StatusActive, freeUser.Status)
	require.Zero(t, freeUser.Balance)

	storedPaidUser, err := client.User.Get(context.Background(), paidUser.ID)
	require.NoError(t, err)
	require.Equal(t, service.StatusActive, storedPaidUser.Status)
	require.Equal(t, 3.5, storedPaidUser.Balance)
	require.Equal(t, 10.0, storedPaidUser.TotalRecharged)
}
