package service

import (
	"context"
	"strconv"
	"strings"
)

const (
	defaultSignupIPRiskControlThreshold        = 3
	defaultSignupIPDisablePreviousAccounts     = true
	defaultSignupIPKeepPreviousAccounts        = 1
	defaultAPIUsageIPUARiskControlThreshold    = 4
	defaultAPIUsageIPUADisablePreviousAccounts = false
	defaultAPIUsageIPUAKeepPreviousAccounts    = 0
)

func (s *SettingService) GetSignupIPRiskControlThreshold(ctx context.Context) int {
	value, err := s.settingRepo.GetValue(ctx, SettingKeySignupIPRiskControlThreshold)
	if err != nil {
		return defaultSignupIPRiskControlThreshold
	}
	return parseSignupIPRiskControlThreshold(value)
}

func (s *SettingService) GetSignupIPDisablePreviousAccounts(ctx context.Context) bool {
	value, err := s.settingRepo.GetValue(ctx, SettingKeySignupIPDisablePreviousAccounts)
	if err != nil {
		return defaultSignupIPDisablePreviousAccounts
	}
	return parseSignupIPDisablePreviousAccounts(value)
}

func (s *SettingService) GetSignupIPKeepPreviousAccounts(ctx context.Context) int {
	value, err := s.settingRepo.GetValue(ctx, SettingKeySignupIPKeepPreviousAccounts)
	if err != nil {
		return defaultSignupIPKeepPreviousAccounts
	}
	return parseSignupIPKeepPreviousAccounts(value)
}

func (s *SettingService) GetAPIUsageIPUARiskControlThreshold(ctx context.Context) int {
	value, err := s.settingRepo.GetValue(ctx, SettingKeyAPIUsageIPUARiskControlThreshold)
	if err != nil {
		return defaultAPIUsageIPUARiskControlThreshold
	}
	return parseAPIUsageIPUARiskControlThreshold(value)
}

func (s *SettingService) GetAPIUsageIPUADisablePreviousAccounts(ctx context.Context) bool {
	value, err := s.settingRepo.GetValue(ctx, SettingKeyAPIUsageIPUADisablePreviousAccounts)
	if err != nil {
		return defaultAPIUsageIPUADisablePreviousAccounts
	}
	return parseAPIUsageIPUADisablePreviousAccounts(value)
}

func (s *SettingService) GetAPIUsageIPUAKeepPreviousAccounts(ctx context.Context) int {
	value, err := s.settingRepo.GetValue(ctx, SettingKeyAPIUsageIPUAKeepPreviousAccounts)
	if err != nil {
		return defaultAPIUsageIPUAKeepPreviousAccounts
	}
	return parseAPIUsageIPUAKeepPreviousAccounts(value)
}

func parseSignupIPRiskControlThreshold(raw string) int {
	value, err := strconv.Atoi(strings.TrimSpace(raw))
	if err != nil || value < 1 {
		return defaultSignupIPRiskControlThreshold
	}
	return value
}

func parseSignupIPDisablePreviousAccounts(raw string) bool {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return defaultSignupIPDisablePreviousAccounts
	}
	return trimmed == "true"
}

func parseSignupIPKeepPreviousAccounts(raw string) int {
	value, err := strconv.Atoi(strings.TrimSpace(raw))
	if err != nil || value < 0 {
		return defaultSignupIPKeepPreviousAccounts
	}
	return value
}

func parseAPIUsageIPUARiskControlThreshold(raw string) int {
	value, err := strconv.Atoi(strings.TrimSpace(raw))
	if err != nil || value < 1 {
		return defaultAPIUsageIPUARiskControlThreshold
	}
	return value
}

func parseAPIUsageIPUADisablePreviousAccounts(raw string) bool {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return defaultAPIUsageIPUADisablePreviousAccounts
	}
	return trimmed == "true"
}

func parseAPIUsageIPUAKeepPreviousAccounts(raw string) int {
	value, err := strconv.Atoi(strings.TrimSpace(raw))
	if err != nil || value < 0 {
		return defaultAPIUsageIPUAKeepPreviousAccounts
	}
	return value
}

func publicTransitPageEnabledFromSettings(settings map[string]string) bool {
	if settings == nil || isFalseSettingValue(settings[SettingKeyPublicTransitEnabled]) {
		return false
	}
	value, exists := settings[SettingKeyPublicTransitPageEnabled]
	if exists {
		return value == "true"
	}
	return settings[SettingKeyPublicTransitEnabled] == "true"
}
