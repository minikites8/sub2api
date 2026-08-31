package service

import (
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
)

func TestSettingServiceParseSettingsRiskControlFields(t *testing.T) {
	svc := NewSettingService(nil, &config.Config{})

	t.Run("defaults", func(t *testing.T) {
		got := svc.parseSettings(map[string]string{})

		require.Equal(t, defaultSignupIPRiskControlThreshold, got.SignupIPRiskControlThreshold)
		require.Equal(t, defaultSignupIPDisablePreviousAccounts, got.SignupIPDisablePreviousAccounts)
		require.Equal(t, defaultSignupIPKeepPreviousAccounts, got.SignupIPKeepPreviousAccounts)
		require.Equal(t, defaultAPIUsageIPUARiskControlThreshold, got.APIUsageIPUARiskControlThreshold)
		require.Equal(t, defaultAPIUsageIPUADisablePreviousAccounts, got.APIUsageIPUADisablePreviousAccounts)
		require.Equal(t, defaultAPIUsageIPUAKeepPreviousAccounts, got.APIUsageIPUAKeepPreviousAccounts)
	})

	t.Run("stored values", func(t *testing.T) {
		got := svc.parseSettings(map[string]string{
			SettingKeySignupIPRiskControlThreshold:        "7",
			SettingKeySignupIPDisablePreviousAccounts:     "false",
			SettingKeySignupIPKeepPreviousAccounts:        "3",
			SettingKeyAPIUsageIPUARiskControlThreshold:    "8",
			SettingKeyAPIUsageIPUADisablePreviousAccounts: "true",
			SettingKeyAPIUsageIPUAKeepPreviousAccounts:    "2",
		})

		require.Equal(t, 7, got.SignupIPRiskControlThreshold)
		require.False(t, got.SignupIPDisablePreviousAccounts)
		require.Equal(t, 3, got.SignupIPKeepPreviousAccounts)
		require.Equal(t, 8, got.APIUsageIPUARiskControlThreshold)
		require.True(t, got.APIUsageIPUADisablePreviousAccounts)
		require.Equal(t, 2, got.APIUsageIPUAKeepPreviousAccounts)
	})
}
