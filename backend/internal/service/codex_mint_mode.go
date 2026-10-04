package service

import (
	"context"
	"github.com/Wei-Shaw/sub2api/internal/config"
)

func remoteCodexMintSelected(cfg config.OpenAICodexTicketConfig, controls CodexHarvestControls) bool {
	switch controls.MintMode {
	case "remote":
		return true
	case "local":
		return false
	default:
		return cfg.CloudMint.Enabled
	}
}

func (s *OpenAIGatewayService) usesRemoteCodexMint(ctx context.Context) bool {
	if s == nil {
		return false
	}
	controls, _ := s.harvestControls(ctx)
	return remoteCodexMintSelected(s.openAICodexTicketConfig(), controls)
}
