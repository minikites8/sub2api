package service

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
)

type codexTicketSettingsSnapshot struct {
	values  map[string]string
	expires time.Time
}

// parseCodexTicketModels preserves exact upstream model names and removes duplicates.
func parseCodexTicketModels(raw string) []string {
	var result []string
	seen := map[string]bool{}
	for _, model := range strings.FieldsFunc(raw, func(r rune) bool { return r == ',' || r == '\n' || r == '\r' }) {
		model = strings.TrimSpace(model)
		if model != "" && !seen[model] {
			result = append(result, model)
			seen[model] = true
		}
	}
	return result
}

func normalizeCodexTicketConfig(cfg config.OpenAICodexTicketConfig) config.OpenAICodexTicketConfig {
	if cfg.TTLSeconds <= 0 {
		cfg.TTLSeconds = 3600
	}
	if cfg.RefreshBeforeSeconds <= 0 {
		cfg.RefreshBeforeSeconds = 600
	}
	cfg.RefreshBeforeSeconds = min(cfg.RefreshBeforeSeconds, cfg.TTLSeconds/2)
	if cfg.HarvestProbeIntervalSeconds <= 0 {
		cfg.HarvestProbeIntervalSeconds = 6
	}
	if cfg.HarvestAttemptTimeoutSeconds <= 0 {
		cfg.HarvestAttemptTimeoutSeconds = 25
	}
	cfg.HarvestAttemptTimeoutSeconds = min(cfg.HarvestAttemptTimeoutSeconds, 120)
	if cfg.Models == nil {
		cfg.Models = []string{"gpt-6-astra", "gpt-5.6-sol"}
	} else {
		models := parseCodexTicketModels(strings.Join(cfg.Models, ","))
		if models == nil {
			models = []string{}
		}
		cfg.Models = models
	}
	cfg.HarvestProxyURL = strings.TrimSpace(cfg.HarvestProxyURL)
	return cfg
}

func (s *SettingService) codexTicketRuntimeConfig(ctx context.Context, fallback config.OpenAICodexTicketConfig) config.OpenAICodexTicketConfig {
	if s == nil || s.settingRepo == nil {
		return normalizeCodexTicketConfig(fallback)
	}
	cached, _ := s.codexTicketSettingsCache.Load().(*codexTicketSettingsSnapshot)
	if cached == nil || !time.Now().Before(cached.expires) {
		value, _, _ := s.codexTicketSettingsSF.Do("codex_ticket", func() (any, error) {
			if current, _ := s.codexTicketSettingsCache.Load().(*codexTicketSettingsSnapshot); current != nil && time.Now().Before(current.expires) {
				return current, nil
			}
			dbCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
			defer cancel()
			keys := []string{SettingKeyOpenAICodexTicketFailClosed, SettingKeyOpenAICodexTicketModels, SettingKeyOpenAICodexRelayURL, SettingKeyOpenAICodexRelayKey, SettingKeyOpenAICodexRelayKeyEnv, SettingKeyOpenAICodexRelayTransport, SettingKeyOpenAICodexRelayGateway, SettingKeyOpenAICodexRelayTimeoutSeconds}
			values := make(map[string]string, len(keys))
			err := error(nil)
			for _, key := range keys {
				value, readErr := s.settingRepo.GetValue(dbCtx, key)
				if readErr != nil && !errors.Is(readErr, ErrSettingNotFound) {
					err = readErr
					break
				}
				values[key] = value
			}
			ttl := 15 * time.Second
			if err != nil {
				values = map[string]string{}
				ttl = time.Second
			}
			next := &codexTicketSettingsSnapshot{values: values, expires: time.Now().Add(ttl)}
			s.codexTicketSettingsCache.Store(next)
			return next, nil
		})
		cached, _ = value.(*codexTicketSettingsSnapshot)
	}
	if cached != nil {

		if value := strings.TrimSpace(cached.values[SettingKeyOpenAICodexTicketFailClosed]); value != "" {
			fallback.FailClosed = value == "true"
		}
		if raw, ok := cached.values[SettingKeyOpenAICodexTicketModels]; ok && strings.TrimSpace(raw) != "" {
			var models []string
			if err := json.Unmarshal([]byte(raw), &models); err == nil {
				fallback.Models = NormalizeOpenAICodexTicketModels(models)
			} else {
				fallback.Models = parseCodexTicketModels(raw)
			}
		}
		if value := strings.TrimSpace(cached.values[SettingKeyOpenAICodexRelayURL]); value != "" {
			fallback.CloudMint.URL = value
			fallback.CloudMint.Enabled = true
		}
		if value := strings.TrimSpace(cached.values[SettingKeyOpenAICodexRelayKey]); value != "" {
			fallback.CloudMint.Key = value
		}
		if value := strings.TrimSpace(cached.values[SettingKeyOpenAICodexRelayKeyEnv]); value != "" {
			if looksLikeCodexRelayKey(value) {
				if fallback.CloudMint.Key == "" {
					fallback.CloudMint.Key = value
				}
				fallback.CloudMint.KeyEnv = "SUB2API_CODEX_CLOUD_MINT_KEY"
			} else {
				fallback.CloudMint.KeyEnv = value
			}
		}
		if value := strings.TrimSpace(cached.values[SettingKeyOpenAICodexRelayTransport]); value != "" {
			fallback.CloudMint.Transport = value
		}
		if value := strings.TrimSpace(cached.values[SettingKeyOpenAICodexRelayGateway]); value != "" {
			fallback.CloudMint.Gateway = value
		}
		if value := strings.TrimSpace(cached.values[SettingKeyOpenAICodexRelayTimeoutSeconds]); value != "" {
			if seconds, err := strconv.Atoi(value); err == nil {
				fallback.CloudMint.TimeoutSeconds = seconds
			}
		}
	}
	// These two settings have dedicated caches and are invalidated immediately
	// after an admin save, so read them here to avoid stale model inventories.
	if proxy := s.GetOpenAICodexTicketHarvestProxyURL(ctx); proxy != "" {
		fallback.HarvestProxyURL = proxy
	}
	fallback.Models = s.GetOpenAICodexTicketModels(ctx, fallback.Models)
	fallback.FailClosed = s.GetOpenAICodexTicketFailClosed(ctx)
	return normalizeCodexTicketConfig(fallback)
}

func MaskCodexTicketProxyURL(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" || ValidateOpenAICodexTicketHarvestProxyURL(raw) != nil {
		return ""
	}
	u, _ := url.Parse(raw)
	if u.User != nil {
		if _, ok := u.User.Password(); ok {
			u.User = url.UserPassword(u.User.Username(), "***")
		}
	}
	return u.String()
}

func IsMaskedCodexTicketProxyURL(raw string) bool {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || u.User == nil {
		return false
	}
	p, ok := u.User.Password()
	return ok && p == "***"
}

// ValidateOpenAICodexRelaySettings validates settings exposed by the admin UI.
func ValidateOpenAICodexRelaySettings(rawURL, key, keyEnv, transport, gateway string, timeoutSeconds int) error {
	if strings.TrimSpace(rawURL) != "" {
		if _, err := normalizeCodexCloudMintURL(rawURL); err != nil {
			return infraerrors.BadRequest("INVALID_CODEX_RELAY_URL", err.Error())
		}
	}
	if strings.TrimSpace(key) == "" && strings.TrimSpace(keyEnv) == "" {
		return infraerrors.BadRequest("INVALID_CODEX_RELAY_KEY", "relay key or environment variable is required")
	}
	if _, err := normalizeCodexCloudMintTransport(transport); err != nil {
		return infraerrors.BadRequest("INVALID_CODEX_RELAY_TRANSPORT", err.Error())
	}
	gateway = normalizeCodex780Gateway(gateway)
	if gateway != "any" && !regexp.MustCompile(`^unified-[0-9]{1,5}$`).MatchString(gateway) {
		return infraerrors.BadRequest("INVALID_CODEX_RELAY_GATEWAY", "relay gateway must be any or unified-N")
	}
	if timeoutSeconds < 5 || timeoutSeconds > 120 {
		return infraerrors.BadRequest("INVALID_CODEX_RELAY_TIMEOUT", "relay timeout must be between 5 and 120 seconds")
	}
	return nil
}

// IsStaticCodexTicketHarvestProxyURL identifies a reusable externally configured exit.
func IsStaticCodexTicketHarvestProxyURL(raw string) bool {
	raw = strings.TrimSpace(raw)
	if raw == "" || raw == OpenAICodexTicketHarvestIPPoolURL || ValidateOpenAICodexTicketHarvestProxyURL(raw) != nil {
		return false
	}
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Hostname() == "localhost" {
		return false
	}
	if ip := net.ParseIP(parsed.Hostname()); ip != nil && ip.IsLoopback() {
		return false
	}
	return true
}
