package service

import "context"

type openAIBPSNativeFallbackKey struct{}

// WithOpenAIBPSNativeFallback excludes the unsupported BPS protocol only for
// this request. Independent native grants remain usable.
func WithOpenAIBPSNativeFallback(ctx context.Context) context.Context {
	return context.WithValue(ctx, openAIBPSNativeFallbackKey{}, true)
}
func OpenAIBPSNativeFallback(ctx context.Context) bool {
	enabled, _ := ctx.Value(openAIBPSNativeFallbackKey{}).(bool)
	return enabled
}

func (s *OpenAIGatewayService) openAIBPSRouteCompatibleWithRequest(ctx context.Context, account *Account, model string) bool {
	return account != nil && (!OpenAIBPSNativeFallback(ctx) || account.HasManagedOpenAIConnections() ||
		!account.IsExcelBPSEnabledForModel(model) || !s.excelBPSGloballyEnabled(ctx))
}
