package middleware

import (
	"context"
	"errors"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/service"
)

func resolveAPIKeyWithQuotaLeaseDemoFallback(ctx context.Context, apiKeyService *service.APIKeyService, cfg *config.Config, apiKeyString string) (*service.APIKey, error) {
	if apiKeyService == nil {
		return nil, service.ErrAPIKeyNotFound
	}
	apiKey, err := apiKeyService.GetByKey(ctx, apiKeyString)
	if err == nil {
		return apiKey, nil
	}
	if !errors.Is(err, service.ErrAPIKeyNotFound) {
		return nil, err
	}
	remoteAPIKey, usedRemote, remoteErr := resolveAPIKeyFromQuotaLeaseDemoControlPlane(ctx, apiKeyService, cfg, apiKeyString)
	if usedRemote {
		return remoteAPIKey, remoteErr
	}
	return nil, err
}

func resolveAPIKeyFromQuotaLeaseDemoControlPlane(ctx context.Context, apiKeyService *service.APIKeyService, cfg *config.Config, apiKeyString string) (*service.APIKey, bool, error) {
	if apiKeyService == nil || cfg == nil || !cfg.IsNodeRole() || !service.QuotaLeaseDemoEnabled(cfg) {
		return nil, false, nil
	}
	if strings.TrimSpace(cfg.Gateway.QuotaLeaseDemo.ControlPlaneBaseURL) == "" {
		return nil, false, nil
	}
	leaseDemo := service.GetQuotaLeaseDemoService(cfg)
	result, err := leaseDemo.AuthorizeClientKeyViaControlPlane(ctx, apiKeyString, 0)
	if err != nil {
		return nil, true, err
	}
	apiKey := apiKeyService.APIKeyFromAuthSnapshot(apiKeyString, result.Snapshot)
	if apiKey == nil {
		return nil, true, service.ErrAPIKeyNotFound
	}
	return apiKey, true, nil
}
