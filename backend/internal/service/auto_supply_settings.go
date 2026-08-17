package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
)

const (
	autoSupplyFallbackVersion = "config-fallback"
	autoSupplyReloadInterval  = 30 * time.Second
)

// AutoSupplyGroupSettings defines one trigger group replenishment rule and its optional deployment groups.
type AutoSupplyGroupSettings struct {
	GroupID                     int64   `json:"group_id"`
	DeployGroupIDs              []int64 `json:"deploy_group_ids"`
	Product                     string  `json:"product"`
	MinAvailable                int     `json:"min_available"`
	Quantity                    int     `json:"quantity"`
	Platform                    string  `json:"platform"`
	AccountType                 string  `json:"account_type"`
	Priority                    int     `json:"priority"`
	Concurrency                 int     `json:"concurrency"`
	ProxyMode                   string  `json:"proxy_mode"`
	ProxyID                     *int64  `json:"proxy_id,omitempty"`
	CodexFingerprintMode        string  `json:"codex_fingerprint_mode"`
	EnableAccountGuard          bool    `json:"enable_account_guard"`
	AccountGuardIntervalMinutes int     `json:"account_guard_interval_minutes"`
}

// AutoSupplySettings is the masked admin-facing configuration.
type AutoSupplySettings struct {
	Enabled                 bool                      `json:"enabled"`
	BaseURL                 string                    `json:"base_url"`
	CustomerTokenConfigured bool                      `json:"customer_token_configured"`
	EncryptionKeyConfigured bool                      `json:"encryption_key_configured"`
	IntervalSeconds         int                       `json:"interval_seconds"`
	RequestTimeoutSeconds   int                       `json:"request_timeout_seconds"`
	MaxQuantityPerRun       int                       `json:"max_quantity_per_run"`
	Groups                  []AutoSupplyGroupSettings `json:"groups"`
}

// AutoSupplySettingsUpdate accepts a replacement token. An empty token keeps
// the currently stored token or the configuration-file fallback.
type AutoSupplySettingsUpdate struct {
	Enabled               bool                      `json:"enabled"`
	BaseURL               string                    `json:"base_url"`
	CustomerToken         string                    `json:"customer_token"`
	IntervalSeconds       int                       `json:"interval_seconds"`
	RequestTimeoutSeconds int                       `json:"request_timeout_seconds"`
	MaxQuantityPerRun     int                       `json:"max_quantity_per_run"`
	Groups                []AutoSupplyGroupSettings `json:"groups"`
}

type autoSupplyStoredSettings struct {
	Enabled                bool                      `json:"enabled"`
	BaseURL                string                    `json:"base_url"`
	CustomerTokenEncrypted string                    `json:"customer_token_encrypted,omitempty"`
	IntervalSeconds        int                       `json:"interval_seconds"`
	RequestTimeoutSeconds  int                       `json:"request_timeout_seconds"`
	MaxQuantityPerRun      int                       `json:"max_quantity_per_run"`
	Groups                 []AutoSupplyGroupSettings `json:"groups"`
}

// SetSettingsDependencies attaches persistence and encryption before Start.
func (s *AutoSupplyService) SetSettingsDependencies(settingRepo SettingRepository, encryptor SecretEncryptor) {
	if s == nil {
		return
	}
	s.settingRepo = settingRepo
	s.encryptor = encryptor
}

// GetSettings returns the effective configuration with the customer token masked.
func (s *AutoSupplyService) GetSettings(ctx context.Context) (*AutoSupplySettings, error) {
	if s == nil {
		return nil, errors.New("auto supply service is unavailable")
	}
	if _, err := s.reloadRuntimeSettings(ctx); err != nil {
		return nil, err
	}
	settings := s.currentAutoSupplyConfig()
	return s.adminSettingsFromConfig(settings), nil
}

// UpdateSettings persists a configuration, applies it in memory, and wakes the worker.
func (s *AutoSupplyService) UpdateSettings(ctx context.Context, input AutoSupplySettingsUpdate) (*AutoSupplySettings, error) {
	if s == nil || s.settingRepo == nil {
		return nil, errors.New("auto supply settings storage is unavailable")
	}
	normalizeAutoSupplySettingsUpdate(&input)

	stored, _, err := s.loadStoredSettings(ctx)
	if err != nil {
		return nil, err
	}
	if stored == nil {
		stored = &autoSupplyStoredSettings{}
	}

	effectiveToken := strings.TrimSpace(input.CustomerToken)
	encryptedToken := stored.CustomerTokenEncrypted
	if effectiveToken != "" {
		if !s.encryptionKeyConfigured() {
			return nil, ErrSecretEncryptionKeyNotConfigured
		}
		if s.encryptor == nil {
			return nil, errors.New("auto supply token encryptor is unavailable")
		}
		encryptedToken, err = s.encryptor.Encrypt(effectiveToken)
		if err != nil {
			return nil, fmt.Errorf("encrypt auto supply customer token: %w", err)
		}
	} else if encryptedToken != "" {
		effectiveToken, err = s.decryptCustomerToken(encryptedToken)
		if err != nil {
			return nil, err
		}
	} else {
		effectiveToken = strings.TrimSpace(s.fallbackAutoSupplyConfig().CustomerToken)
	}

	if err := validateAutoSupplySettings(input, effectiveToken); err != nil {
		return nil, infraerrors.BadRequest("AUTO_SUPPLY_SETTINGS_INVALID", err.Error())
	}

	stored = &autoSupplyStoredSettings{
		Enabled:                input.Enabled,
		BaseURL:                input.BaseURL,
		CustomerTokenEncrypted: encryptedToken,
		IntervalSeconds:        input.IntervalSeconds,
		RequestTimeoutSeconds:  input.RequestTimeoutSeconds,
		MaxQuantityPerRun:      input.MaxQuantityPerRun,
		Groups:                 cloneAutoSupplyGroupSettings(input.Groups),
	}
	raw, err := json.Marshal(stored)
	if err != nil {
		return nil, fmt.Errorf("marshal auto supply settings: %w", err)
	}
	if err := s.settingRepo.Set(ctx, SettingKeyAutoSupplySettings, string(raw)); err != nil {
		return nil, fmt.Errorf("save auto supply settings: %w", err)
	}

	runtimeSettings := autoSupplyConfigFromStored(stored, effectiveToken)
	s.applyRuntimeSettings(runtimeSettings, string(raw))
	s.wake()
	return s.adminSettingsFromConfig(runtimeSettings), nil
}

func (s *AutoSupplyService) reloadRuntimeSettings(ctx context.Context) (bool, error) {
	if s == nil {
		return false, nil
	}
	stored, raw, err := s.loadStoredSettings(ctx)
	if err != nil {
		return false, err
	}
	if stored == nil {
		return s.applyRuntimeSettings(s.fallbackAutoSupplyConfig(), autoSupplyFallbackVersion), nil
	}
	token := strings.TrimSpace(s.fallbackAutoSupplyConfig().CustomerToken)
	if stored.CustomerTokenEncrypted != "" {
		token, err = s.decryptCustomerToken(stored.CustomerTokenEncrypted)
		if err != nil {
			return false, err
		}
	}
	return s.applyRuntimeSettings(autoSupplyConfigFromStored(stored, token), raw), nil
}

func (s *AutoSupplyService) loadStoredSettings(ctx context.Context) (*autoSupplyStoredSettings, string, error) {
	if s.settingRepo == nil {
		return nil, "", nil
	}
	raw, err := s.settingRepo.GetValue(ctx, SettingKeyAutoSupplySettings)
	if err != nil {
		if errors.Is(err, ErrSettingNotFound) {
			return nil, "", nil
		}
		return nil, "", fmt.Errorf("load auto supply settings: %w", err)
	}
	var stored autoSupplyStoredSettings
	if err := json.Unmarshal([]byte(raw), &stored); err != nil {
		return nil, "", fmt.Errorf("decode auto supply settings: %w", err)
	}
	return &stored, raw, nil
}

func (s *AutoSupplyService) decryptCustomerToken(ciphertext string) (string, error) {
	if s.encryptor == nil {
		return "", errors.New("auto supply token decryptor is unavailable")
	}
	value, err := s.encryptor.Decrypt(ciphertext)
	if err != nil {
		return "", fmt.Errorf("decrypt auto supply customer token: %w", err)
	}
	return strings.TrimSpace(value), nil
}

func (s *AutoSupplyService) encryptionKeyConfigured() bool {
	return s != nil && s.cfg != nil && s.cfg.Totp.EncryptionKeyConfigured
}

func (s *AutoSupplyService) fallbackAutoSupplyConfig() config.AutoSupplyConfig {
	if s == nil || s.cfg == nil {
		return normalizeAutoSupplyConfig(config.AutoSupplyConfig{})
	}
	return normalizeAutoSupplyConfig(s.cfg.AutoSupply)
}

func (s *AutoSupplyService) adminSettingsFromConfig(settings config.AutoSupplyConfig) *AutoSupplySettings {
	settings = normalizeAutoSupplyConfig(settings)
	groups := make([]AutoSupplyGroupSettings, 0, len(settings.Groups))
	for _, group := range settings.Groups {
		groups = append(groups, autoSupplyGroupSettingsFromConfig(group))
	}
	return &AutoSupplySettings{
		Enabled:                 settings.Enabled,
		BaseURL:                 settings.BaseURL,
		CustomerTokenConfigured: strings.TrimSpace(settings.CustomerToken) != "",
		EncryptionKeyConfigured: s.encryptionKeyConfigured(),
		IntervalSeconds:         settings.IntervalSeconds,
		RequestTimeoutSeconds:   settings.RequestTimeoutSeconds,
		MaxQuantityPerRun:       settings.MaxQuantityPerRun,
		Groups:                  groups,
	}
}

func (s *AutoSupplyService) currentAutoSupplyConfig() config.AutoSupplyConfig {
	s.settingsMu.RLock()
	defer s.settingsMu.RUnlock()
	return cloneAutoSupplyConfig(s.runtimeSettings)
}

func (s *AutoSupplyService) applyRuntimeSettings(settings config.AutoSupplyConfig, version string) bool {
	settings = normalizeAutoSupplyConfig(settings)
	s.settingsMu.Lock()
	defer s.settingsMu.Unlock()
	if s.settingsVersion == version {
		return false
	}
	s.runtimeSettings = settings
	s.settingsVersion = version
	return true
}

func (s *AutoSupplyService) wake() {
	select {
	case s.wakeCh <- struct{}{}:
	default:
	}
}

func autoSupplyEnabled(settings config.AutoSupplyConfig) bool {
	return settings.Enabled && strings.TrimSpace(settings.BaseURL) != "" &&
		strings.TrimSpace(settings.CustomerToken) != "" && len(settings.Groups) > 0
}

func normalizeAutoSupplyConfig(settings config.AutoSupplyConfig) config.AutoSupplyConfig {
	settings.BaseURL = strings.TrimRight(strings.TrimSpace(settings.BaseURL), "/")
	if settings.BaseURL == "" {
		settings.BaseURL = "https://bugteam.team"
	}
	settings.CustomerToken = strings.TrimSpace(settings.CustomerToken)
	if settings.IntervalSeconds <= 0 {
		settings.IntervalSeconds = 30
	}
	if settings.RequestTimeoutSeconds <= 0 {
		settings.RequestTimeoutSeconds = 20
	}
	if settings.MaxQuantityPerRun <= 0 {
		settings.MaxQuantityPerRun = 10
	}
	settings.Groups = cloneAutoSupplyConfig(settings).Groups
	for index := range settings.Groups {
		settings.Groups[index].Product = strings.TrimSpace(settings.Groups[index].Product)
		settings.Groups[index].Platform = strings.TrimSpace(settings.Groups[index].Platform)
		settings.Groups[index].AccountType = strings.TrimSpace(settings.Groups[index].AccountType)
		settings.Groups[index].ProxyMode = normalizeAutoSupplyProxyMode(settings.Groups[index].ProxyMode)
		if settings.Groups[index].ProxyMode != "specified" {
			settings.Groups[index].ProxyID = nil
		}
		settings.Groups[index].CodexFingerprintMode = normalizeAutoSupplyFingerprintMode(settings.Groups[index].CodexFingerprintMode)
		if settings.Groups[index].AccountGuardIntervalMinutes <= 0 {
			settings.Groups[index].AccountGuardIntervalMinutes = OpenAIAccountGuardDefaultIntervalMinutes
		}
		settings.Groups[index].DeployGroupIDs = normalizeAutoSupplyDeployGroupIDs(
			settings.Groups[index].GroupID,
			settings.Groups[index].DeployGroupIDs,
		)
	}
	return settings
}

func cloneAutoSupplyConfig(settings config.AutoSupplyConfig) config.AutoSupplyConfig {
	groups := make([]config.AutoSupplyGroupConfig, len(settings.Groups))
	for index, group := range settings.Groups {
		group.DeployGroupIDs = append([]int64(nil), group.DeployGroupIDs...)
		group.ProxyID = cloneInt64Pointer(group.ProxyID)
		groups[index] = group
	}
	settings.Groups = groups
	return settings
}

func autoSupplyConfigFromStored(stored *autoSupplyStoredSettings, token string) config.AutoSupplyConfig {
	groups := make([]config.AutoSupplyGroupConfig, 0, len(stored.Groups))
	for _, group := range stored.Groups {
		groups = append(groups, config.AutoSupplyGroupConfig{
			GroupID: group.GroupID, DeployGroupIDs: append([]int64(nil), group.DeployGroupIDs...),
			Product: group.Product, MinAvailable: group.MinAvailable,
			Quantity: group.Quantity, Platform: group.Platform, AccountType: group.AccountType,
			Priority: group.Priority, Concurrency: group.Concurrency,
			ProxyMode: group.ProxyMode, ProxyID: cloneInt64Pointer(group.ProxyID),
			CodexFingerprintMode:        group.CodexFingerprintMode,
			EnableAccountGuard:          group.EnableAccountGuard,
			AccountGuardIntervalMinutes: group.AccountGuardIntervalMinutes,
		})
	}
	return normalizeAutoSupplyConfig(config.AutoSupplyConfig{
		Enabled: stored.Enabled, BaseURL: stored.BaseURL, CustomerToken: token,
		IntervalSeconds: stored.IntervalSeconds, RequestTimeoutSeconds: stored.RequestTimeoutSeconds,
		MaxQuantityPerRun: stored.MaxQuantityPerRun, Groups: groups,
	})
}

func autoSupplyGroupSettingsFromConfig(group config.AutoSupplyGroupConfig) AutoSupplyGroupSettings {
	return AutoSupplyGroupSettings{
		GroupID: group.GroupID, DeployGroupIDs: append([]int64{}, group.DeployGroupIDs...),
		Product: group.Product, MinAvailable: group.MinAvailable,
		Quantity: group.Quantity, Platform: group.Platform, AccountType: group.AccountType,
		Priority: group.Priority, Concurrency: group.Concurrency,
		ProxyMode: group.ProxyMode, ProxyID: cloneInt64Pointer(group.ProxyID),
		CodexFingerprintMode:        group.CodexFingerprintMode,
		EnableAccountGuard:          group.EnableAccountGuard,
		AccountGuardIntervalMinutes: group.AccountGuardIntervalMinutes,
	}
}

func cloneAutoSupplyGroupSettings(groups []AutoSupplyGroupSettings) []AutoSupplyGroupSettings {
	cloned := make([]AutoSupplyGroupSettings, len(groups))
	for index, group := range groups {
		group.DeployGroupIDs = append([]int64(nil), group.DeployGroupIDs...)
		group.ProxyID = cloneInt64Pointer(group.ProxyID)
		cloned[index] = group
	}
	return cloned
}

func normalizeAutoSupplySettingsUpdate(input *AutoSupplySettingsUpdate) {
	input.BaseURL = strings.TrimRight(strings.TrimSpace(input.BaseURL), "/")
	input.CustomerToken = strings.TrimSpace(input.CustomerToken)
	for index := range input.Groups {
		input.Groups[index].Product = strings.TrimSpace(input.Groups[index].Product)
		input.Groups[index].Platform = strings.TrimSpace(input.Groups[index].Platform)
		input.Groups[index].AccountType = strings.TrimSpace(input.Groups[index].AccountType)
		input.Groups[index].ProxyMode = normalizeAutoSupplyProxyMode(input.Groups[index].ProxyMode)
		if input.Groups[index].ProxyMode != "specified" {
			input.Groups[index].ProxyID = nil
		}
		input.Groups[index].CodexFingerprintMode = normalizeAutoSupplyFingerprintMode(input.Groups[index].CodexFingerprintMode)
		if input.Groups[index].AccountGuardIntervalMinutes <= 0 {
			input.Groups[index].AccountGuardIntervalMinutes = OpenAIAccountGuardDefaultIntervalMinutes
		}
		input.Groups[index].DeployGroupIDs = normalizeAutoSupplyDeployGroupIDs(
			input.Groups[index].GroupID,
			input.Groups[index].DeployGroupIDs,
		)
	}
}

func validateAutoSupplySettings(input AutoSupplySettingsUpdate, effectiveToken string) error {
	if len(effectiveToken) > 4096 {
		return errors.New("customer_token is too long")
	}
	if len(input.BaseURL) > 2048 {
		return errors.New("base_url is too long")
	}
	if input.BaseURL != "" {
		parsed, err := url.Parse(input.BaseURL)
		if err != nil || parsed.Scheme == "" || parsed.Host == "" || parsed.User != nil {
			return errors.New("base_url must be an absolute URL without userinfo")
		}
		if parsed.Scheme != "https" {
			host := strings.ToLower(parsed.Hostname())
			if host != "localhost" && host != "127.0.0.1" && host != "::1" {
				return errors.New("base_url must use HTTPS")
			}
		}
	}
	if input.IntervalSeconds < 5 || input.IntervalSeconds > 86400 {
		return errors.New("interval_seconds must be between 5 and 86400")
	}
	if input.RequestTimeoutSeconds < 1 || input.RequestTimeoutSeconds > 300 {
		return errors.New("request_timeout_seconds must be between 1 and 300")
	}
	if input.MaxQuantityPerRun < 1 || input.MaxQuantityPerRun > 1000 {
		return errors.New("max_quantity_per_run must be between 1 and 1000")
	}
	if len(input.Groups) > 100 {
		return errors.New("groups must contain at most 100 rules")
	}
	seen := make(map[int64]struct{}, len(input.Groups))
	for index, group := range input.Groups {
		if group.GroupID <= 0 {
			return fmt.Errorf("groups[%d].group_id must be positive", index)
		}
		if len(group.DeployGroupIDs) > 100 {
			return fmt.Errorf("groups[%d].deploy_group_ids must contain at most 100 groups", index)
		}
		for targetIndex, targetID := range group.DeployGroupIDs {
			if targetID <= 0 {
				return fmt.Errorf("groups[%d].deploy_group_ids[%d] must be positive", index, targetIndex)
			}
		}
		if group.ProxyMode == "specified" && (group.ProxyID == nil || *group.ProxyID <= 0) {
			return fmt.Errorf("groups[%d].proxy_id is required for specified proxy mode", index)
		}
		if group.ProxyMode != "none" && group.ProxyMode != "specified" && group.ProxyMode != "random" {
			return fmt.Errorf("groups[%d].proxy_mode must be none, specified, or random", index)
		}
		if group.CodexFingerprintMode != AdminAPIKeyCodexFingerprintOff &&
			group.CodexFingerprintMode != AdminAPIKeyCodexFingerprintDevice &&
			group.CodexFingerprintMode != AdminAPIKeyCodexFingerprintSession &&
			group.CodexFingerprintMode != AdminAPIKeyCodexFingerprintFull {
			return fmt.Errorf("groups[%d].codex_fingerprint_mode is invalid", index)
		}
		if group.AccountGuardIntervalMinutes < OpenAIAccountGuardMinIntervalMinutes ||
			group.AccountGuardIntervalMinutes > OpenAIAccountGuardMaxIntervalMinutes {
			return fmt.Errorf("groups[%d].account_guard_interval_minutes must be between %d and %d", index, OpenAIAccountGuardMinIntervalMinutes, OpenAIAccountGuardMaxIntervalMinutes)
		}
		if _, exists := seen[group.GroupID]; exists {
			return fmt.Errorf("groups[%d].group_id is duplicated", index)
		}
		seen[group.GroupID] = struct{}{}
		if group.Product == "" || len(group.Product) > 128 {
			return fmt.Errorf("groups[%d].product must contain 1-128 characters", index)
		}
		if group.MinAvailable < 0 || group.MinAvailable > 100000 {
			return fmt.Errorf("groups[%d].min_available must be between 0 and 100000", index)
		}
		if group.Quantity < 0 || group.Quantity > 1000 {
			return fmt.Errorf("groups[%d].quantity must be between 0 and 1000", index)
		}
		if group.Priority < 0 || group.Priority > 100000 {
			return fmt.Errorf("groups[%d].priority must be between 0 and 100000", index)
		}
		if group.Concurrency < 0 || group.Concurrency > 1000 {
			return fmt.Errorf("groups[%d].concurrency must be between 0 and 1000", index)
		}
	}
	if input.Enabled {
		if input.BaseURL == "" {
			return errors.New("base_url is required when auto supply is enabled")
		}
		if strings.TrimSpace(effectiveToken) == "" {
			return errors.New("customer_token is required when auto supply is enabled")
		}
		if len(input.Groups) == 0 {
			return errors.New("groups must contain at least one rule when auto supply is enabled")
		}
	}
	return nil
}

func normalizeAutoSupplyDeployGroupIDs(triggerGroupID int64, values []int64) []int64 {
	if len(values) == 0 {
		return []int64{}
	}
	seen := make(map[int64]struct{}, len(values))
	result := make([]int64, 0, len(values))
	for _, value := range values {
		if value == triggerGroupID {
			continue
		}
		if _, exists := seen[value]; exists {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	return result
}

func normalizeAutoSupplyProxyMode(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	if value == "" {
		return "none"
	}
	if value == AdminAPIKeyProxyModeFixed {
		return "specified"
	}
	return value
}

func normalizeAutoSupplyFingerprintMode(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	if value == "" {
		return AdminAPIKeyCodexFingerprintOff
	}
	return value
}
