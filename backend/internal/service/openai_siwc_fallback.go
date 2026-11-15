package service

import (
	"context"
	"encoding/json"
	"net/http"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/gin-gonic/gin"
)

type openAISIWCNativeFallbackKey struct{}

// WithOpenAISIWCNativeFallback skips SIWC for the remaining account selections
// without changing the account's stored protocol or authorization state.
func WithOpenAISIWCNativeFallback(ctx context.Context) context.Context {
	return context.WithValue(ctx, openAISIWCNativeFallbackKey{}, true)
}

func openAISIWCNativeFallback(ctx context.Context) bool {
	enabled, _ := ctx.Value(openAISIWCNativeFallbackKey{}).(bool)
	return enabled
}

func openAISIWCRequestCompatible(ctx context.Context, account *Account) bool {
	return account != nil && (!openAISIWCNativeFallback(ctx) || account.HasManagedOpenAIConnections() || !account.IsSIWCEnabled())
}

func siwcUnsupportedCapability(code string) bool {
	switch code {
	case "OPENAI_SIWC_UNSUPPORTED_INPUT", "OPENAI_SIWC_UNSUPPORTED_TOOL", "OPENAI_SIWC_UNSUPPORTED_ENDPOINT":
		return true
	default:
		return false
	}
}

// Local capability validation has not acquired a token or sent inference.
// Keep the original history and exclude SIWC only for this request.
func siwcCapabilityFailover(ctx context.Context, c *gin.Context, account *Account, err error) *UpstreamFailoverError {
	code := infraerrors.Reason(err)
	if !siwcUnsupportedCapability(code) || !excelBPSCanReplayFailure(ctx, c) {
		return nil
	}
	message := infraerrors.Message(err)
	body, _ := json.Marshal(gin.H{"error": gin.H{"type": "invalid_request_error", "code": code, "message": message}})
	appendOpsUpstreamError(c, OpsUpstreamErrorEvent{
		Platform: account.Platform, AccountID: account.ID, AccountName: account.Name,
		Kind: "siwc_native_bypass", Stage: "route", Scope: "siwc", Reason: code, Message: message,
	})
	return &UpstreamFailoverError{
		StatusCode: http.StatusBadRequest, ResponseBody: body,
		Stage: "route", Scope: GatewayFailureScopeRequest, Reason: GatewayFailureReason(code), NextAccountAction: NextAccountRetry,
		ClientStatusCode: http.StatusBadRequest, ClientMessage: message,
	}
}

func (e *UpstreamFailoverError) IsSIWCLocalCapabilityRejection() bool {
	return e != nil && e.Stage == "route" && e.Scope == GatewayFailureScopeRequest && siwcUnsupportedCapability(string(e.Reason))
}
