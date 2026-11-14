package service

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/pkg/requestretry"
	"github.com/gin-gonic/gin"
	"github.com/tidwall/gjson"
)

const excelBPSFailureScope GatewayFailureScope = "bps"

// ExcelBPSLocalCapabilityReason describes a BPS-only rejection before inference
// submission. The handler can select a native route without changing health.
const ExcelBPSLocalCapabilityReason = "bps_local_capability_unavailable"

// A rejected BPS route can change accounts without penalizing the parent's
// independent native grant. Request errors and already-metered work cannot.
func excelBPSReplayableRejection(account *Account, status int, body []byte) bool {
	if status != http.StatusUnauthorized && status != http.StatusForbidden && status < http.StatusInternalServerError {
		return false
	}
	// Bridge terminals retain their active error under response.error. Existing
	// HTTP request classifiers consume an error envelope, so normalize only that
	// field instead of scanning messages or echoed request content.
	if detail := gjson.GetBytes(body, "response.error"); detail.IsObject() {
		body = []byte(`{"error":` + detail.Raw + `}`)
	}
	if retryable := gjson.GetBytes(body, "error.retryable"); retryable.Type == gjson.False {
		return false
	}
	if extractUpstreamErrorCode(body) == "cyber_access_model_denied" {
		return false
	}
	if isOpenAIUpstreamSessionBlocked(body) || isOpenAIStreamSafetyFailure(body) ||
		isOpenAIRequestScoped403(account, status, body) || isOpenAIContextWindowError("", body) {
		return false
	}
	if hit, _, _ := detectOpenAICyberPolicy(body); hit {
		return false
	}
	return !openAIEncryptedErrorCode(extractUpstreamErrorCode(body))
}

func excelBPSCanReplayFailure(ctx context.Context, c *gin.Context) bool {
	return ctx.Err() == nil && !isQualityObservation(ctx) && !isControlledExperiment(ctx) &&
		c != nil && c.Request != nil && c.Request.Context().Err() == nil &&
		c.Writer != nil && !c.Writer.Written() && !IsResponseCommitted(c)
}

func newExcelBPSRequestFailover(status int, headers http.Header, code, message string) *UpstreamFailoverError {
	body, _ := json.Marshal(map[string]any{"error": map[string]string{
		"type": "upstream_error", "code": code, "message": message,
	}})
	return &UpstreamFailoverError{
		StatusCode: status, ResponseHeaders: headers.Clone(), ResponseBody: body,
		Stage: GatewayFailureStageInference, Scope: excelBPSFailureScope,
		Reason: GatewayFailureReason(code), NextAccountAction: NextAccountRetry,
	}
}

// Respect explicit opt-outs without treating an absent opt-in as disabled.
func excelBPSRetryExplicitlyDisabled(c *gin.Context, headers http.Header) bool {
	if strings.EqualFold(strings.TrimSpace(headers.Get("X-Should-Retry")), "false") {
		return true
	}
	if c == nil || c.Request == nil {
		return true
	}
	// Only a valid, single true value opts into immediate account switching.
	// False, malformed and repeated values preserve the default policy.
	return requestretry.Enabled(c.Request)
}
