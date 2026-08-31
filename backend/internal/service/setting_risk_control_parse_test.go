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
		require.True(t, got.AntiAbuseEnabled)
		require.Equal(t, defaultAntiAbuseScoreThreshold, got.AntiAbuseScoreThreshold)
		require.Equal(t, 1, got.AntiAbuseFingerprintWeight)
	})

	t.Run("stored values", func(t *testing.T) {
		got := svc.parseSettings(map[string]string{
			SettingKeySignupIPRiskControlThreshold:        "7",
			SettingKeySignupIPDisablePreviousAccounts:     "false",
			SettingKeySignupIPKeepPreviousAccounts:        "3",
			SettingKeyAPIUsageIPUARiskControlThreshold:    "8",
			SettingKeyAPIUsageIPUADisablePreviousAccounts: "true",
			SettingKeyAPIUsageIPUAKeepPreviousAccounts:    "2",
			SettingKeyAntiAbuseEnabled:                    "false",
			SettingKeyAntiAbuseScoreThreshold:             "77",
			SettingKeyAntiAbuseFingerprintWeight:          "3",
			SettingKeyAntiAbuseIPWeight:                   "2",
			SettingKeyAntiAbuseEmailWeight:                "4",
			SettingKeyAntiAbuseUserAgentWeight:            "5",
			SettingKeyAntiAbuseTLSFingerprintWeight:       "6",
			SettingKeyAntiAbuseIPReputationEndpoint:       "https://risk.example/check",
			SettingKeyAntiAbuseIPReputationAPIKey:         "secret",
		})

		require.Equal(t, 7, got.SignupIPRiskControlThreshold)
		require.False(t, got.SignupIPDisablePreviousAccounts)
		require.Equal(t, 3, got.SignupIPKeepPreviousAccounts)
		require.Equal(t, 8, got.APIUsageIPUARiskControlThreshold)
		require.True(t, got.APIUsageIPUADisablePreviousAccounts)
		require.Equal(t, 2, got.APIUsageIPUAKeepPreviousAccounts)
		require.False(t, got.AntiAbuseEnabled)
		require.Equal(t, 77, got.AntiAbuseScoreThreshold)
		require.Equal(t, 3, got.AntiAbuseFingerprintWeight)
		require.Equal(t, 2, got.AntiAbuseIPWeight)
		require.Equal(t, 4, got.AntiAbuseEmailWeight)
		require.Equal(t, 5, got.AntiAbuseUserAgentWeight)
		require.Equal(t, 6, got.AntiAbuseTLSFingerprintWeight)
		require.Equal(t, "https://risk.example/check", got.AntiAbuseIPReputationEndpoint)
		require.True(t, got.AntiAbuseIPReputationAPIKeyConfigured)
	})
}
