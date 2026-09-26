package service

import (
	"context"
	"strings"
)

func (s *CodexHarvestService) Snapshot(ctx context.Context, proxy string) CodexHarvestControlSnapshot {
	v, configured, err := s.Controls(ctx)
	out := CodexHarvestControlSnapshot{Settings: v, Configured: configured, Defaults: s.defaults,
		Presets: CodexHarvestSpeedPresets(), Bounds: CodexHarvestSpeedBounds(), Runtime: s.Runtime()}
	if err != nil {
		out.SettingsError = "harvest settings unavailable; retaining last valid values"
	}
	out.Available = strings.TrimSpace(proxy) != ""
	if !out.Available {
		out.AvailabilityReason = "configure an external harvest proxy"
	}
	return out
}
