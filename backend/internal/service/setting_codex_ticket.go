package service

import (
	"context"
	"errors"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
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
	cfg.Models = parseCodexTicketModels(strings.Join(cfg.Models, ","))
	if len(cfg.Models) == 0 {
		cfg.Models = []string{"gpt-6-astra", "gpt-5.6-sol"}
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
			values, err := s.settingRepo.GetMultiple(dbCtx, []string{SettingKeyOpenAICodexTicketEnabled, SettingKeyOpenAICodexTicketHarvestProxyURL, SettingKeyOpenAICodexTicketModels})
			ttl := 15 * time.Second
			if err != nil {
				values = map[string]string{SettingKeyOpenAICodexTicketEnabled: "false"}
				ttl = time.Second
			}
			next := &codexTicketSettingsSnapshot{values: values, expires: time.Now().Add(ttl)}
			s.codexTicketSettingsCache.Store(next)
			return next, nil
		})
		cached, _ = value.(*codexTicketSettingsSnapshot)
	}
	if cached != nil {
		if v := cached.values[SettingKeyOpenAICodexTicketEnabled]; v != "" {
			fallback.Enabled = v == "true"
		}
		if v := strings.TrimSpace(cached.values[SettingKeyOpenAICodexTicketHarvestProxyURL]); v != "" {
			fallback.HarvestProxyURL = v
		}
		if v := parseCodexTicketModels(cached.values[SettingKeyOpenAICodexTicketModels]); len(v) > 0 {
			fallback.Models = v
		}
	}
	return normalizeCodexTicketConfig(fallback)
}

func ValidateOpenAICodexTicketHarvestProxyURL(raw string) error {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	u, err := url.Parse(raw)
	if err != nil || u.Hostname() == "" || u.Opaque != "" || u.RawQuery != "" || u.ForceQuery || u.Fragment != "" || (u.Path != "" && u.Path != "/") {
		return errors.New("ticket proxy requires a host and an HTTP(S) or SOCKS5(h) URL without a path, query or fragment")
	}
	switch u.Scheme {
	case "http", "https", "socks5", "socks5h":
	default:
		return errors.New("ticket proxy scheme must be http, https, socks5 or socks5h")
	}
	if p := u.Port(); p != "" {
		n, err := strconv.Atoi(p)
		if err != nil || n < 1 || n > 65535 {
			return errors.New("ticket proxy port must be between 1 and 65535")
		}
	}
	return nil
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
