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
		service.SettingKeyRegistrationEnabled: "true",
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
