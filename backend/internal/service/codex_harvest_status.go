package service

import (
	"context"
	"github.com/Wei-Shaw/sub2api/internal/config"
	"os"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/mihomo"
)

func (s *CodexHarvestService) Snapshot(ctx context.Context, proxy string) CodexHarvestControlSnapshot {
	return s.snapshot(ctx, config.OpenAICodexTicketConfig{CloudMint: s.cloudMint, HarvestProxyURL: proxy})
}

func (s *CodexHarvestService) SnapshotWithSettings(ctx context.Context, cfg *config.Config, settings *SettingService) CodexHarvestControlSnapshot {
	ticketCfg := config.OpenAICodexTicketConfig{CloudMint: s.cloudMint}
	if cfg != nil {
		ticketCfg = cfg.Gateway.OpenAICodexTicket
	}
	if settings != nil {
		ticketCfg = settings.codexTicketRuntimeConfig(ctx, ticketCfg)
	}
	return s.snapshot(ctx, ticketCfg)
}

func (s *CodexHarvestService) snapshot(ctx context.Context, ticketCfg config.OpenAICodexTicketConfig) CodexHarvestControlSnapshot {
	v, configured, err := s.Controls(ctx)
	out := CodexHarvestControlSnapshot{Settings: v, Configured: configured, Defaults: s.defaults,
		Presets: CodexHarvestSpeedPresets(), Bounds: CodexHarvestSpeedBounds(), Runtime: s.Runtime()}
	if err != nil {
		out.SettingsError = "harvest settings unavailable; retaining last valid values"
	}
	if remoteCodexMintSelected(ticketCfg, v) {
		_, urlErr := normalizeCodexCloudMintURL(ticketCfg.CloudMint.URL)
		out.Available = urlErr == nil && codexCloudMintKey(ticketCfg.CloudMint) != ""
		if !out.Available {
			out.AvailabilityReason = "configure the remote mint URL and key"
		}
		return out
	}
	proxy := ticketCfg.HarvestProxyURL
	if !v.NodeMemoryEnabled && strings.TrimSpace(proxy) != "" {
		out.Available = true
		return out
	}
	sidecar, err := mihomo.LoadDirectedSidecar(os.Getenv("DATA_DIR"), proxy)
	if err != nil {
		out.AvailabilityReason = err.Error()
		return out
	}
	query, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	if _, err := sidecar.Directory(query); err != nil {
		out.AvailabilityReason = err.Error()
		return out
	}
	out.Available = true
	return out
}
