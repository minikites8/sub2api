//go:build unit

package service_test

import (
	"context"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func TestAuthServiceRegister_DisablesSubsequentAccountsFromSameSignupIP(t *testing.T) {
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
	require.ErrorIs(t, err, service.ErrUserNotActive)
	require.NotNil(t, thirdUser)
	require.Equal(t, service.StatusDisabled, thirdUser.Status)

	storedFirstUser, err := client.User.Get(context.Background(), firstUser.ID)
	require.NoError(t, err)
	require.Equal(t, service.StatusActive, storedFirstUser.Status)
	require.NotNil(t, storedFirstUser.SignupIP)
	require.Equal(t, "1.2.3.4", *storedFirstUser.SignupIP)

	storedSecondUser, err := client.User.Get(context.Background(), secondUser.ID)
	require.NoError(t, err)
	require.Equal(t, service.StatusDisabled, storedSecondUser.Status)

	storedThirdUser, err := client.User.Get(context.Background(), thirdUser.ID)
	require.NoError(t, err)
	require.Equal(t, service.StatusDisabled, storedThirdUser.Status)
}

func TestAuthServiceRegister_OnlyDisablesCurrentAccountWhenConfigured(t *testing.T) {
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
	require.ErrorIs(t, err, service.ErrUserNotActive)
	require.NotNil(t, secondUser)
	require.Equal(t, service.StatusDisabled, secondUser.Status)

	storedFirstUser, err := client.User.Get(context.Background(), firstUser.ID)
	require.NoError(t, err)
	require.Equal(t, service.StatusActive, storedFirstUser.Status)

	storedSecondUser, err := client.User.Get(context.Background(), secondUser.ID)
	require.NoError(t, err)
	require.Equal(t, service.StatusDisabled, storedSecondUser.Status)
}

func TestAuthServiceRegister_DisablesEarlierAccountsBeyondKeepCount(t *testing.T) {
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
	require.ErrorIs(t, err, service.ErrUserNotActive)
	require.NotNil(t, fourthUser)
	require.Equal(t, service.StatusDisabled, fourthUser.Status)

	storedFirstUser, err := client.User.Get(context.Background(), firstUser.ID)
	require.NoError(t, err)
	require.Equal(t, service.StatusActive, storedFirstUser.Status)

	storedSecondUser, err := client.User.Get(context.Background(), secondUser.ID)
	require.NoError(t, err)
	require.Equal(t, service.StatusActive, storedSecondUser.Status)

	storedThirdUser, err := client.User.Get(context.Background(), thirdUser.ID)
	require.NoError(t, err)
	require.Equal(t, service.StatusDisabled, storedThirdUser.Status)

	storedFourthUser, err := client.User.Get(context.Background(), fourthUser.ID)
	require.NoError(t, err)
	require.Equal(t, service.StatusDisabled, storedFourthUser.Status)
}
