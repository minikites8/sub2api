package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strconv"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
)

const (
	quotaLeaseDemoDefaultPrefetchLowWatermarkAmount = 0.2
	quotaLeaseDemoDefaultPrefetchAverageWindow      = 5
	quotaLeaseDemoDefaultPrefetchAverageMultiplier  = 3.0
	quotaLeaseDemoDefaultPrefetchDebounceSeconds    = 10
	quotaLeaseDemoSettingsCacheTTL                  = 15 * time.Second
)

type QuotaLeaseDemoSettings struct {
	PrefetchLowWatermarkAmount float64 `json:"prefetch_low_watermark_amount"`
	PrefetchAverageWindow      int     `json:"prefetch_average_window"`
	PrefetchAverageMultiplier  float64 `json:"prefetch_average_multiplier"`
	PrefetchDebounceSeconds    int     `json:"prefetch_debounce_seconds"`
}

type QuotaLeaseDemoSettingsPatch struct {
	PrefetchLowWatermarkAmount *float64 `json:"prefetch_low_watermark_amount"`
	PrefetchAverageWindow      *int     `json:"prefetch_average_window"`
	PrefetchAverageMultiplier  *float64 `json:"prefetch_average_multiplier"`
	PrefetchDebounceSeconds    *int     `json:"prefetch_debounce_seconds"`
}

type quotaLeaseDemoPrefetchState struct {
	InFlight      bool
	LastAttemptAt time.Time
	Samples       []float64
}

func (s *SettingService) GetQuotaLeaseDemoSettings(ctx context.Context) (*QuotaLeaseDemoSettings, error) {
	defaults := s.defaultQuotaLeaseDemoSettings()
	if s == nil || s.settingRepo == nil {
		return defaults, nil
	}
	if ctx == nil {
		ctx = context.Background()
	}
	raw, err := s.settingRepo.GetValue(ctx, SettingKeyQuotaLeaseSettings)
	if err != nil {
		if errors.Is(err, ErrSettingNotFound) {
			legacyRaw, legacyErr := s.settingRepo.GetValue(ctx, SettingKeyQuotaLeaseDemoSettings)
			if legacyErr != nil {
				if errors.Is(legacyErr, ErrSettingNotFound) {
					return defaults, nil
				}
				return nil, fmt.Errorf("get quota lease settings: %w", legacyErr)
			}
			raw = legacyRaw
		} else {
			return nil, fmt.Errorf("get quota lease settings: %w", err)
		}
	}
	settings, err := parseQuotaLeaseDemoSettingsJSON(raw, defaults)
	if err != nil {
		return nil, fmt.Errorf("parse quota lease settings: %w", err)
	}
	return settings, nil
}

func (s *SettingService) SetQuotaLeaseDemoSettings(ctx context.Context, patch *QuotaLeaseDemoSettingsPatch) (*QuotaLeaseDemoSettings, error) {
	if s == nil || s.settingRepo == nil {
		return nil, fmt.Errorf("setting repository is unavailable")
	}
	if patch == nil {
		return nil, infraerrors.BadRequest("INVALID_QUOTA_LEASE_SETTINGS", "settings cannot be nil")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	current, err := s.GetQuotaLeaseDemoSettings(ctx)
	if err != nil {
		slog.Warn("quota_lease.settings_load_for_update_failed", "error", err)
		current = s.defaultQuotaLeaseDemoSettings()
	}
	settings := applyQuotaLeaseDemoSettingsPatch(current, patch)
	normalized, err := validateQuotaLeaseDemoSettings(settings)
	if err != nil {
		return nil, err
	}
	data, err := json.Marshal(normalized)
	if err != nil {
		return nil, fmt.Errorf("marshal quota lease settings: %w", err)
	}
	if err := s.settingRepo.SetMultiple(ctx, map[string]string{
		SettingKeyQuotaLeaseSettings:     string(data),
		SettingKeyQuotaLeaseDemoSettings: string(data),
	}); err != nil {
		return nil, fmt.Errorf("save quota lease settings: %w", err)
	}
	if s.onUpdate != nil {
		s.onUpdate()
	}
	return normalized, nil
}

func (s *SettingService) defaultQuotaLeaseDemoSettings() *QuotaLeaseDemoSettings {
	if s == nil || s.cfg == nil {
		return defaultQuotaLeaseDemoSettings()
	}
	cfg := s.cfg.Gateway.QuotaLeaseDemo
	if s.cfg.Gateway.QuotaLease.Enabled {
		cfg = s.cfg.Gateway.QuotaLease
	}
	return quotaLeaseDemoSettingsFromConfig(cfg)
}

func defaultQuotaLeaseDemoSettings() *QuotaLeaseDemoSettings {
	return &QuotaLeaseDemoSettings{
		PrefetchLowWatermarkAmount: quotaLeaseDemoDefaultPrefetchLowWatermarkAmount,
		PrefetchAverageWindow:      quotaLeaseDemoDefaultPrefetchAverageWindow,
		PrefetchAverageMultiplier:  quotaLeaseDemoDefaultPrefetchAverageMultiplier,
		PrefetchDebounceSeconds:    quotaLeaseDemoDefaultPrefetchDebounceSeconds,
	}
}

func quotaLeaseDemoSettingsFromConfig(cfg config.GatewayQuotaLeaseDemoConfig) *QuotaLeaseDemoSettings {
	settings := &QuotaLeaseDemoSettings{
		PrefetchLowWatermarkAmount: cfg.PrefetchLowWatermarkAmount,
		PrefetchAverageWindow:      cfg.PrefetchAverageWindow,
		PrefetchAverageMultiplier:  cfg.PrefetchAverageMultiplier,
		PrefetchDebounceSeconds:    cfg.PrefetchDebounceSeconds,
	}
	normalized, err := validateQuotaLeaseDemoSettings(settings)
	if err != nil {
		return defaultQuotaLeaseDemoSettings()
	}
	return normalized
}

func parseQuotaLeaseDemoSettingsJSON(raw string, defaults *QuotaLeaseDemoSettings) (*QuotaLeaseDemoSettings, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return cloneQuotaLeaseDemoSettings(defaults), nil
	}
	var patch QuotaLeaseDemoSettingsPatch
	if err := json.Unmarshal([]byte(raw), &patch); err != nil {
		return nil, err
	}
	settings := applyQuotaLeaseDemoSettingsPatch(defaults, &patch)
	return validateQuotaLeaseDemoSettings(settings)
}

func applyQuotaLeaseDemoSettingsPatch(base *QuotaLeaseDemoSettings, patch *QuotaLeaseDemoSettingsPatch) *QuotaLeaseDemoSettings {
	settings := cloneQuotaLeaseDemoSettings(base)
	if settings == nil {
		settings = defaultQuotaLeaseDemoSettings()
	}
	if patch == nil {
		return settings
	}
	if patch.PrefetchLowWatermarkAmount != nil {
		settings.PrefetchLowWatermarkAmount = *patch.PrefetchLowWatermarkAmount
	}
	if patch.PrefetchAverageWindow != nil {
		settings.PrefetchAverageWindow = *patch.PrefetchAverageWindow
	}
	if patch.PrefetchAverageMultiplier != nil {
		settings.PrefetchAverageMultiplier = *patch.PrefetchAverageMultiplier
	}
	if patch.PrefetchDebounceSeconds != nil {
		settings.PrefetchDebounceSeconds = *patch.PrefetchDebounceSeconds
	}
	return settings
}

func validateQuotaLeaseDemoSettings(settings *QuotaLeaseDemoSettings) (*QuotaLeaseDemoSettings, error) {
	if settings == nil {
		return nil, infraerrors.BadRequest("INVALID_QUOTA_LEASE_SETTINGS", "settings cannot be nil")
	}
	if !isFiniteNonNegativeFloat(settings.PrefetchLowWatermarkAmount) {
		return nil, infraerrors.BadRequest("INVALID_QUOTA_LEASE_SETTINGS", "prefetch_low_watermark_amount must be >= 0")
	}
	if settings.PrefetchAverageWindow < 0 {
		return nil, infraerrors.BadRequest("INVALID_QUOTA_LEASE_SETTINGS", "prefetch_average_window must be >= 0")
	}
	if !isFiniteNonNegativeFloat(settings.PrefetchAverageMultiplier) {
		return nil, infraerrors.BadRequest("INVALID_QUOTA_LEASE_SETTINGS", "prefetch_average_multiplier must be >= 0")
	}
	if settings.PrefetchDebounceSeconds < 0 {
		return nil, infraerrors.BadRequest("INVALID_QUOTA_LEASE_SETTINGS", "prefetch_debounce_seconds must be >= 0")
	}
	normalized := *settings
	return &normalized, nil
}

func cloneQuotaLeaseDemoSettings(settings *QuotaLeaseDemoSettings) *QuotaLeaseDemoSettings {
	if settings == nil {
		return nil
	}
	value := *settings
	return &value
}

func (s *QuotaLeaseDemoService) SetSettingService(settingService *SettingService) {
	if s == nil {
		return
	}
	s.settingsMu.Lock()
	s.settingService = settingService
	s.runtimeSettings = nil
	s.runtimeSettingsExpiresAt = time.Time{}
	s.settingsMu.Unlock()
}

func (s *QuotaLeaseDemoService) GetSettings(ctx context.Context) (*QuotaLeaseDemoSettings, error) {
	return s.runtimeSettingsSnapshot(ctx, true)
}

func (s *QuotaLeaseDemoService) UpdateSettings(ctx context.Context, patch *QuotaLeaseDemoSettingsPatch) (*QuotaLeaseDemoSettings, error) {
	if s == nil {
		return nil, ErrQuotaLeaseDemoDisabled
	}
	s.settingsMu.RLock()
	settingService := s.settingService
	s.settingsMu.RUnlock()
	if settingService == nil {
		return nil, infraerrors.ServiceUnavailable("QUOTA_LEASE_SETTINGS_UNAVAILABLE", "quota lease settings service is unavailable")
	}
	settings, err := settingService.SetQuotaLeaseDemoSettings(ctx, patch)
	if err != nil {
		return nil, err
	}
	s.cacheRuntimeSettings(settings, time.Now().Add(quotaLeaseDemoSettingsCacheTTL))
	s.resetPrefetchState()
	return cloneQuotaLeaseDemoSettings(settings), nil
}

func (s *QuotaLeaseDemoService) runtimeSettingsSnapshot(ctx context.Context, requireFresh bool) (*QuotaLeaseDemoSettings, error) {
	if s == nil {
		return defaultQuotaLeaseDemoSettings(), nil
	}
	now := time.Now()
	s.settingsMu.RLock()
	cached := cloneQuotaLeaseDemoSettings(s.runtimeSettings)
	expiresAt := s.runtimeSettingsExpiresAt
	settingService := s.settingService
	s.settingsMu.RUnlock()
	if cached != nil && !requireFresh && (expiresAt.IsZero() || now.Before(expiresAt)) {
		return cached, nil
	}
	if settingService == nil && s.remoteMode() {
		settings, err := s.fetchRemoteSettings(ctx)
		if err == nil && settings != nil {
			s.cacheRuntimeSettings(settings, now.Add(quotaLeaseDemoSettingsCacheTTL))
			return cloneQuotaLeaseDemoSettings(settings), nil
		}
		if cached != nil {
			return cached, nil
		}
		slog.Warn("quota_lease.remote_settings_load_failed", "error", err)
	}
	if settingService == nil {
		settings := quotaLeaseDemoSettingsFromConfig(s.cfgSnapshot())
		s.cacheRuntimeSettings(settings, now.Add(quotaLeaseDemoSettingsCacheTTL))
		return cloneQuotaLeaseDemoSettings(settings), nil
	}
	settings, err := settingService.GetQuotaLeaseDemoSettings(ctx)
	if err != nil {
		if cached != nil {
			return cached, nil
		}
		slog.Warn("quota_lease.settings_load_failed", "error", err)
		settings = quotaLeaseDemoSettingsFromConfig(s.cfgSnapshot())
	}
	s.cacheRuntimeSettings(settings, now.Add(quotaLeaseDemoSettingsCacheTTL))
	return cloneQuotaLeaseDemoSettings(settings), nil
}

func (s *QuotaLeaseDemoService) cacheRuntimeSettings(settings *QuotaLeaseDemoSettings, expiresAt time.Time) {
	if s == nil || settings == nil {
		return
	}
	s.settingsMu.Lock()
	s.runtimeSettings = cloneQuotaLeaseDemoSettings(settings)
	s.runtimeSettingsExpiresAt = expiresAt
	s.settingsMu.Unlock()
}

func (s *QuotaLeaseDemoService) resetPrefetchState() {
	if s == nil {
		return
	}
	s.mu.Lock()
	s.prefetchState = make(map[string]*quotaLeaseDemoPrefetchState)
	s.mu.Unlock()
}

func (s *QuotaLeaseDemoService) maybePrefetchUsageLease(ctx context.Context, lease *QuotaLeaseDemoLease, consumedAmount float64) {
	if s == nil || !s.remoteMode() || lease == nil || consumedAmount <= 0 {
		return
	}
	nodeID := strings.TrimSpace(lease.NodeID)
	if nodeID == "" {
		nodeID = s.activeNodeID()
	}
	if nodeID == "" || lease.UserID <= 0 || lease.APIKeyID <= 0 {
		return
	}
	settings, err := s.runtimeSettingsSnapshot(ctx, false)
	if err != nil || settings == nil {
		return
	}
	remaining := lease.Remaining()
	avg, shouldAttempt := s.recordPrefetchSampleAndShouldAttempt(nodeID, lease.UserID, lease.APIKeyID, consumedAmount, remaining, settings, time.Now())
	if !shouldAttempt {
		return
	}
	target := s.prefetchTargetAmount(remaining, avg, settings)
	go func() {
		defer s.markPrefetchComplete(nodeID, lease.UserID, lease.APIKeyID)
		reqCtx, cancel := context.WithTimeout(context.Background(), s.RemoteTimeout())
		defer cancel()
		if flushErr := s.FlushPendingUsage(reqCtx); flushErr != nil {
			slog.Warn("quota_lease.prefetch_usage_flush_failed",
				"node_id", nodeID,
				"user_id", lease.UserID,
				"api_key_id", lease.APIKeyID,
				"error", flushErr,
			)
			return
		}
		if _, requestErr := s.RequestLease(reqCtx, QuotaLeaseDemoLeaseRequest{
			NodeID:   nodeID,
			UserID:   lease.UserID,
			APIKeyID: lease.APIKeyID,
			Amount:   target,
		}); requestErr != nil {
			slog.Warn("quota_lease.prefetch_failed",
				"node_id", nodeID,
				"user_id", lease.UserID,
				"api_key_id", lease.APIKeyID,
				"remaining", remaining,
				"target", target,
				"error", requestErr,
			)
		}
	}()
}

func (s *QuotaLeaseDemoService) recordPrefetchSampleAndShouldAttempt(
	nodeID string,
	userID, apiKeyID int64,
	amount float64,
	remaining float64,
	settings *QuotaLeaseDemoSettings,
	now time.Time,
) (float64, bool) {
	if s == nil || settings == nil {
		return 0, false
	}
	key := quotaLeaseDemoPrefetchKey(nodeID, userID, apiKeyID)
	debounce := time.Duration(settings.PrefetchDebounceSeconds) * time.Second
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.prefetchState == nil {
		s.prefetchState = make(map[string]*quotaLeaseDemoPrefetchState)
	}
	state := s.prefetchState[key]
	if state == nil {
		state = &quotaLeaseDemoPrefetchState{}
		s.prefetchState[key] = state
	}
	if isFiniteNonNegativeFloat(amount) && amount > 0 {
		state.Samples = append(state.Samples, amount)
		if maxSamples := quotaLeaseDemoPrefetchSampleLimit(settings); len(state.Samples) > maxSamples {
			state.Samples = append([]float64(nil), state.Samples[len(state.Samples)-maxSamples:]...)
		}
	}
	avg := quotaLeaseDemoPrefetchAverage(state.Samples, settings.PrefetchAverageWindow)
	threshold := settings.PrefetchLowWatermarkAmount
	if settings.PrefetchAverageWindow > 0 && avg > 0 && settings.PrefetchAverageMultiplier > 0 {
		averageThreshold := avg * settings.PrefetchAverageMultiplier
		if averageThreshold > threshold {
			threshold = averageThreshold
		}
	}
	if threshold <= 0 || remaining >= threshold {
		return avg, false
	}
	if state.InFlight {
		return avg, false
	}
	if debounce > 0 && !state.LastAttemptAt.IsZero() && now.Sub(state.LastAttemptAt) < debounce {
		return avg, false
	}
	state.InFlight = true
	state.LastAttemptAt = now
	return avg, true
}

func (s *QuotaLeaseDemoService) markPrefetchComplete(nodeID string, userID, apiKeyID int64) {
	if s == nil {
		return
	}
	key := quotaLeaseDemoPrefetchKey(nodeID, userID, apiKeyID)
	s.mu.Lock()
	if state := s.prefetchState[key]; state != nil {
		state.InFlight = false
	}
	s.mu.Unlock()
}

func (s *QuotaLeaseDemoService) prefetchTargetAmount(remaining, avg float64, settings *QuotaLeaseDemoSettings) float64 {
	defaultGrant := s.cfgSnapshot().DefaultGrantAmount
	if defaultGrant <= 0 {
		defaultGrant = 1
	}
	blockSize := defaultGrant
	if settings != nil && settings.PrefetchAverageWindow > 0 && avg > 0 && settings.PrefetchAverageMultiplier > 0 {
		if dynamicBlockSize := avg * settings.PrefetchAverageMultiplier; dynamicBlockSize > blockSize {
			blockSize = dynamicBlockSize
		}
	}
	if remaining < 0 {
		remaining = 0
	}
	return remaining + blockSize
}

func quotaLeaseDemoPrefetchKey(nodeID string, userID, apiKeyID int64) string {
	return strings.TrimSpace(nodeID) + "\x1f" + strconv.FormatInt(userID, 10) + "\x1f" + strconv.FormatInt(apiKeyID, 10)
}

func quotaLeaseDemoPrefetchSampleLimit(settings *QuotaLeaseDemoSettings) int {
	limit := 16
	if settings != nil && settings.PrefetchAverageWindow > limit {
		limit = settings.PrefetchAverageWindow
	}
	if limit > 100 {
		return 100
	}
	return limit
}

func quotaLeaseDemoPrefetchAverage(samples []float64, window int) float64 {
	if window <= 0 || len(samples) == 0 {
		return 0
	}
	if window > len(samples) {
		window = len(samples)
	}
	start := len(samples) - window
	sum := 0.0
	count := 0
	for _, value := range samples[start:] {
		if !isFiniteNonNegativeFloat(value) || value <= 0 {
			continue
		}
		sum += value
		count++
	}
	if count == 0 {
		return 0
	}
	return sum / float64(count)
}
