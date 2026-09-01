package service

import (
	"context"
	"strconv"
	"strings"
	"time"
)

const (
	defaultSignupIPRiskControlThreshold        = 3
	defaultSignupIPDisablePreviousAccounts     = true
	defaultSignupIPKeepPreviousAccounts        = 1
	defaultAPIUsageIPUARiskControlThreshold    = 4
	defaultAPIUsageIPUADisablePreviousAccounts = false
	defaultAPIUsageIPUAKeepPreviousAccounts    = 0
	defaultAntiAbuseEnabled                    = true
	defaultAntiAbuseFingerprintWeight          = 1
	defaultAntiAbuseIPWeight                   = 1
	defaultAntiAbuseEmailWeight                = 1
	defaultAntiAbuseUserAgentWeight            = 1
	defaultAntiAbuseTLSFingerprintWeight       = 1
)

const antiAbuseRuntimeCacheTTL = 30 * time.Second

type cachedAntiAbuseRuntime struct {
	policy    AntiAbusePolicy
	endpoint  string
	apiKey    string
	expiresAt int64
}

type AntiAbuseConfigView struct {
	Enabled                             bool   `json:"enabled"`
	ScoreThreshold                      int    `json:"score_threshold"`
	FingerprintWeight                   int    `json:"fingerprint_weight"`
	IPWeight                            int    `json:"ip_weight"`
	EmailWeight                         int    `json:"email_weight"`
	UserAgentWeight                     int    `json:"user_agent_weight"`
	TLSFingerprintWeight                int    `json:"tls_fingerprint_weight"`
	SignupIPRiskControlThreshold        int    `json:"signup_ip_risk_control_threshold"`
	SignupIPDisablePreviousAccounts     bool   `json:"signup_ip_disable_previous_accounts"`
	SignupIPKeepPreviousAccounts        int    `json:"signup_ip_keep_previous_accounts"`
	APIUsageIPUARiskControlThreshold    int    `json:"api_usage_ip_ua_risk_control_threshold"`
	APIUsageIPUADisablePreviousAccounts bool   `json:"api_usage_ip_ua_disable_previous_accounts"`
	APIUsageIPUAKeepPreviousAccounts    int    `json:"api_usage_ip_ua_keep_previous_accounts"`
	IPReputationEndpoint                string `json:"ip_reputation_endpoint"`
	IPReputationAPIKeyConfigured        bool   `json:"ip_reputation_api_key_configured"`
}

type UpdateAntiAbuseConfigInput struct {
	Enabled                             bool
	ScoreThreshold                      int
	FingerprintWeight                   int
	IPWeight                            int
	EmailWeight                         int
	UserAgentWeight                     int
	TLSFingerprintWeight                int
	SignupIPRiskControlThreshold        int
	SignupIPDisablePreviousAccounts     bool
	SignupIPKeepPreviousAccounts        int
	APIUsageIPUARiskControlThreshold    int
	APIUsageIPUADisablePreviousAccounts bool
	APIUsageIPUAKeepPreviousAccounts    int
	IPReputationEndpoint                string
	IPReputationAPIKey                  *string
}

func antiAbuseConfigView(runtime cachedAntiAbuseRuntime) AntiAbuseConfigView {
	return AntiAbuseConfigView{
		Enabled: runtime.policy.Enabled, ScoreThreshold: runtime.policy.ScoreThreshold,
		FingerprintWeight: runtime.policy.FingerprintWeight, IPWeight: runtime.policy.IPWeight,
		EmailWeight: runtime.policy.EmailWeight, UserAgentWeight: runtime.policy.UserAgentWeight,
		TLSFingerprintWeight:                runtime.policy.TLSFingerprintWeight,
		SignupIPRiskControlThreshold:        runtime.policy.SignupIPRiskControlThreshold,
		SignupIPDisablePreviousAccounts:     runtime.policy.SignupIPDisablePreviousAccounts,
		SignupIPKeepPreviousAccounts:        runtime.policy.SignupIPKeepPreviousAccounts,
		APIUsageIPUARiskControlThreshold:    runtime.policy.APIUsageIPUARiskControlThreshold,
		APIUsageIPUADisablePreviousAccounts: runtime.policy.APIUsageIPUADisablePreviousAccounts,
		APIUsageIPUAKeepPreviousAccounts:    runtime.policy.APIUsageIPUAKeepPreviousAccounts,
		IPReputationEndpoint:                runtime.endpoint, IPReputationAPIKeyConfigured: runtime.apiKey != "",
	}
}

func (s *SettingService) GetAntiAbuseConfig(ctx context.Context) AntiAbuseConfigView {
	return antiAbuseConfigView(s.getAntiAbuseRuntime(ctx))
}

func (s *SettingService) UpdateAntiAbuseConfig(ctx context.Context, input UpdateAntiAbuseConfigInput) (AntiAbuseConfigView, error) {
	if s == nil || s.settingRepo == nil {
		return AntiAbuseConfigView{}, ErrServiceUnavailable
	}
	runtime := s.getAntiAbuseRuntime(ctx)
	runtime.policy.Enabled = input.Enabled
	runtime.policy.ScoreThreshold = parsePositiveInt(strconv.Itoa(input.ScoreThreshold), defaultAntiAbuseScoreThreshold)
	runtime.policy.FingerprintWeight = parsePositiveInt(strconv.Itoa(input.FingerprintWeight), defaultAntiAbuseFingerprintWeight)
	runtime.policy.IPWeight = parsePositiveInt(strconv.Itoa(input.IPWeight), defaultAntiAbuseIPWeight)
	runtime.policy.EmailWeight = parsePositiveInt(strconv.Itoa(input.EmailWeight), defaultAntiAbuseEmailWeight)
	runtime.policy.UserAgentWeight = parsePositiveInt(strconv.Itoa(input.UserAgentWeight), defaultAntiAbuseUserAgentWeight)
	runtime.policy.TLSFingerprintWeight = parsePositiveInt(strconv.Itoa(input.TLSFingerprintWeight), defaultAntiAbuseTLSFingerprintWeight)
	runtime.policy.SignupIPRiskControlThreshold = parseSignupIPRiskControlThreshold(strconv.Itoa(input.SignupIPRiskControlThreshold))
	runtime.policy.SignupIPDisablePreviousAccounts = input.SignupIPDisablePreviousAccounts
	runtime.policy.SignupIPKeepPreviousAccounts = parseSignupIPKeepPreviousAccounts(strconv.Itoa(input.SignupIPKeepPreviousAccounts))
	runtime.policy.APIUsageIPUARiskControlThreshold = parseAPIUsageIPUARiskControlThreshold(strconv.Itoa(input.APIUsageIPUARiskControlThreshold))
	runtime.policy.APIUsageIPUADisablePreviousAccounts = input.APIUsageIPUADisablePreviousAccounts
	runtime.policy.APIUsageIPUAKeepPreviousAccounts = parseAPIUsageIPUAKeepPreviousAccounts(strconv.Itoa(input.APIUsageIPUAKeepPreviousAccounts))
	runtime.endpoint = strings.TrimSpace(input.IPReputationEndpoint)
	if input.IPReputationAPIKey != nil {
		runtime.apiKey = strings.TrimSpace(*input.IPReputationAPIKey)
	}
	values := map[string]string{
		SettingKeyAntiAbuseEnabled: strconv.FormatBool(runtime.policy.Enabled), SettingKeyAntiAbuseScoreThreshold: strconv.Itoa(runtime.policy.ScoreThreshold),
		SettingKeyAntiAbuseFingerprintWeight: strconv.Itoa(runtime.policy.FingerprintWeight), SettingKeyAntiAbuseIPWeight: strconv.Itoa(runtime.policy.IPWeight),
		SettingKeyAntiAbuseEmailWeight: strconv.Itoa(runtime.policy.EmailWeight), SettingKeyAntiAbuseUserAgentWeight: strconv.Itoa(runtime.policy.UserAgentWeight),
		SettingKeyAntiAbuseTLSFingerprintWeight: strconv.Itoa(runtime.policy.TLSFingerprintWeight), SettingKeyAntiAbuseIPReputationEndpoint: runtime.endpoint,
		SettingKeySignupIPRiskControlThreshold:        strconv.Itoa(runtime.policy.SignupIPRiskControlThreshold),
		SettingKeySignupIPDisablePreviousAccounts:     strconv.FormatBool(runtime.policy.SignupIPDisablePreviousAccounts),
		SettingKeySignupIPKeepPreviousAccounts:        strconv.Itoa(runtime.policy.SignupIPKeepPreviousAccounts),
		SettingKeyAPIUsageIPUARiskControlThreshold:    strconv.Itoa(runtime.policy.APIUsageIPUARiskControlThreshold),
		SettingKeyAPIUsageIPUADisablePreviousAccounts: strconv.FormatBool(runtime.policy.APIUsageIPUADisablePreviousAccounts),
		SettingKeyAPIUsageIPUAKeepPreviousAccounts:    strconv.Itoa(runtime.policy.APIUsageIPUAKeepPreviousAccounts),
	}
	if input.IPReputationAPIKey != nil {
		values[SettingKeyAntiAbuseIPReputationAPIKey] = runtime.apiKey
	}
	if err := s.settingRepo.SetMultiple(ctx, values); err != nil {
		return AntiAbuseConfigView{}, err
	}
	runtime.expiresAt = time.Now().Add(antiAbuseRuntimeCacheTTL).UnixNano()
	s.antiAbuseRuntimeSF.Forget("anti_abuse_runtime")
	s.antiAbuseRuntimeCache.Store(&runtime)
	return antiAbuseConfigView(runtime), nil
}

func (s *SettingService) getAntiAbuseRuntime(ctx context.Context) cachedAntiAbuseRuntime {
	fallback := cachedAntiAbuseRuntime{policy: DefaultAntiAbusePolicy()}
	if s == nil || s.settingRepo == nil {
		return fallback
	}
	if cached, _ := s.antiAbuseRuntimeCache.Load().(*cachedAntiAbuseRuntime); cached != nil && time.Now().UnixNano() < cached.expiresAt {
		return *cached
	}
	loaded, err, _ := s.antiAbuseRuntimeSF.Do("anti_abuse_runtime", func() (any, error) {
		if cached, _ := s.antiAbuseRuntimeCache.Load().(*cachedAntiAbuseRuntime); cached != nil && time.Now().UnixNano() < cached.expiresAt {
			return *cached, nil
		}
		keys := []string{SettingKeyAntiAbuseEnabled, SettingKeyAntiAbuseScoreThreshold, SettingKeyAntiAbuseFingerprintWeight, SettingKeyAntiAbuseIPWeight, SettingKeyAntiAbuseEmailWeight, SettingKeyAntiAbuseUserAgentWeight, SettingKeyAntiAbuseTLSFingerprintWeight, SettingKeySignupIPRiskControlThreshold, SettingKeySignupIPDisablePreviousAccounts, SettingKeySignupIPKeepPreviousAccounts, SettingKeyAPIUsageIPUARiskControlThreshold, SettingKeyAPIUsageIPUADisablePreviousAccounts, SettingKeyAPIUsageIPUAKeepPreviousAccounts, SettingKeyAntiAbuseIPReputationEndpoint, SettingKeyAntiAbuseIPReputationAPIKey}
		values, loadErr := s.settingRepo.GetMultiple(ctx, keys)
		if loadErr != nil {
			return fallback, loadErr
		}
		runtime := cachedAntiAbuseRuntime{
			policy: AntiAbusePolicy{
				Enabled:                             parseBoolWithDefault(values[SettingKeyAntiAbuseEnabled], defaultAntiAbuseEnabled),
				ScoreThreshold:                      parsePositiveInt(values[SettingKeyAntiAbuseScoreThreshold], defaultAntiAbuseScoreThreshold),
				FingerprintWeight:                   parsePositiveInt(values[SettingKeyAntiAbuseFingerprintWeight], defaultAntiAbuseFingerprintWeight),
				IPWeight:                            parsePositiveInt(values[SettingKeyAntiAbuseIPWeight], defaultAntiAbuseIPWeight),
				EmailWeight:                         parsePositiveInt(values[SettingKeyAntiAbuseEmailWeight], defaultAntiAbuseEmailWeight),
				UserAgentWeight:                     parsePositiveInt(values[SettingKeyAntiAbuseUserAgentWeight], defaultAntiAbuseUserAgentWeight),
				TLSFingerprintWeight:                parsePositiveInt(values[SettingKeyAntiAbuseTLSFingerprintWeight], defaultAntiAbuseTLSFingerprintWeight),
				SignupIPRiskControlThreshold:        parseSignupIPRiskControlThreshold(values[SettingKeySignupIPRiskControlThreshold]),
				SignupIPDisablePreviousAccounts:     parseSignupIPDisablePreviousAccounts(values[SettingKeySignupIPDisablePreviousAccounts]),
				SignupIPKeepPreviousAccounts:        parseSignupIPKeepPreviousAccounts(values[SettingKeySignupIPKeepPreviousAccounts]),
				APIUsageIPUARiskControlThreshold:    parseAPIUsageIPUARiskControlThreshold(values[SettingKeyAPIUsageIPUARiskControlThreshold]),
				APIUsageIPUADisablePreviousAccounts: parseAPIUsageIPUADisablePreviousAccounts(values[SettingKeyAPIUsageIPUADisablePreviousAccounts]),
				APIUsageIPUAKeepPreviousAccounts:    parseAPIUsageIPUAKeepPreviousAccounts(values[SettingKeyAPIUsageIPUAKeepPreviousAccounts]),
			},
			endpoint:  strings.TrimSpace(values[SettingKeyAntiAbuseIPReputationEndpoint]),
			apiKey:    strings.TrimSpace(values[SettingKeyAntiAbuseIPReputationAPIKey]),
			expiresAt: time.Now().Add(antiAbuseRuntimeCacheTTL).UnixNano(),
		}
		s.antiAbuseRuntimeCache.Store(&runtime)
		return runtime, nil
	})
	if err != nil {
		return fallback
	}
	return loaded.(cachedAntiAbuseRuntime)
}

func (s *SettingService) GetAntiAbusePolicy(ctx context.Context) AntiAbusePolicy {
	return s.getAntiAbuseRuntime(ctx).policy
}

func parsePositiveInt(raw string, fallback int) int {
	value, err := strconv.Atoi(strings.TrimSpace(raw))
	if err != nil || value < 1 {
		return fallback
	}
	return value
}
func parseBoolWithDefault(raw string, fallback bool) bool {
	value := strings.TrimSpace(strings.ToLower(raw))
	if value == "" {
		return fallback
	}
	return value == "true"
}

func (s *SettingService) GetAntiAbuseIPReputationConfig(ctx context.Context) (string, string) {
	runtime := s.getAntiAbuseRuntime(ctx)
	return runtime.endpoint, runtime.apiKey
}

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
