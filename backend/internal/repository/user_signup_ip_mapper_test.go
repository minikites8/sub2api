package repository

import (
	"testing"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/stretchr/testify/require"
)

func TestUserEntityToService_MapsSignupIP(t *testing.T) {
	signupIP := "203.0.113.42"
	out := userEntityToService(&dbent.User{
		ID:       123,
		Email:    "signup-ip@example.com",
		SignupIP: &signupIP,
	})

	require.NotNil(t, out)
	require.NotNil(t, out.SignupIP)
	require.Equal(t, signupIP, *out.SignupIP)
}
