package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/service/basispoints"
	"github.com/gin-gonic/gin"
	"github.com/tidwall/gjson"
)

var errExcelBPSModelUnavailable = errors.New("Excel BPS model unavailable; continue through the ordinary upstream")

var errExcelBPSAccountEligibilityChanged = errors.New("Excel BPS account eligibility changed; continue through the ordinary upstream")

// Match the BPS admission response specifically; local account admission remains
// authoritative when the ordinary upstream revalidates this request.
func excelBPSAccountEligibilityChanged(status int, payload []byte) bool {
	if status != http.StatusServiceUnavailable && status != http.StatusOK {
		return false
	}
	if !gjson.ValidBytes(payload) {
		return false
	}
	errValue := gjson.GetBytes(payload, "error")
	if !errValue.IsObject() {
		errValue = gjson.GetBytes(payload, "response.error")
	}
	if !errValue.IsObject() {
		errValue = gjson.ParseBytes(payload)
	}
	return strings.EqualFold(strings.TrimSpace(errValue.Get("message").String()), "Account eligibility changed; please retry with complete context")
}

func recordExcelBPSAccountEligibilityFallback(ctx context.Context, c *gin.Context, account *Account, body []byte, resp *http.Response) {
	const reason = "account_eligibility_changed"
	requestPath := ""
	if c != nil && c.Request != nil && c.Request.URL != nil {
		requestPath = c.Request.URL.Path
	}
	recordExcelBPSNativeFallback(ctx, account, reason)
	model := gjson.GetBytes(body, "model").String()
	appendOpsUpstreamError(c, OpsUpstreamErrorEvent{
		Platform: account.Platform, AccountID: account.ID, AccountName: account.Name,
		RequestedModel: model, MappedModel: account.GetMappedModel(model), HasTools: gjson.GetBytes(body, "tools").IsArray(),
		ProxyID: opsUpstreamProxyID(account), ProxyName: opsUpstreamProxyName(account),
		UpstreamStatusCode: resp.StatusCode, UpstreamRequestID: resp.Header.Get("x-request-id"),
		UpstreamURL: basispoints.ResponsesURL, Kind: "bps_native_fallback", Stage: "route", Scope: "bps", Reason: reason,
		Message: "Excel BPS account eligibility changed; retrying ordinary upstream with original context",
		Detail:  fmt.Sprintf("request_path=%s stream=%t", requestPath, gjson.GetBytes(body, "stream").Bool()),
	})
}

// excelBPSModelUnavailable identifies a model admission rejection. Authentication,
// quota, malformed-input and transport failures retain their original handling.
func excelBPSModelUnavailable(status int, payload []byte) bool {
	switch status {
	case http.StatusOK, http.StatusBadRequest, http.StatusForbidden, http.StatusNotFound, http.StatusUnprocessableEntity:
	default:
		return false
	}
	if !gjson.ValidBytes(payload) {
		return false
	}
	errValue := gjson.GetBytes(payload, "error")
	if !errValue.IsObject() {
		errValue = gjson.GetBytes(payload, "response.error")
	}
	if !errValue.IsObject() {
		errValue = gjson.ParseBytes(payload)
	}
	code := strings.ToLower(strings.TrimSpace(errValue.Get("code").String()))
	kind := strings.ToLower(strings.TrimSpace(errValue.Get("type").String()))
	known := func(value string) bool {
		switch value {
		case "basispoints_model_access_changed", "model_not_found", "model_not_available", "model_not_supported", "unsupported_model", "invalid_model", "model_access_denied":
			return true
		default:
			return false
		}
	}
	if known(code) || known(kind) {
		return true
	}
	switch code {
	case "", "invalid_request_error", "invalid_value", "unsupported_value":
	default:
		return false
	}
	switch kind {
	case "", "error", "invalid_request_error":
	default:
		return false
	}
	param := strings.TrimSpace(errValue.Get("param").String())
	if param != "" && param != "model" {
		return false
	}
	return isExplicitOpenAIModelAvailabilityMessage(errValue.Get("message").String())
}

func excelBPSCanFallback(c *gin.Context) bool {
	return c != nil && c.Writer != nil && !c.GetBool(bpsAccountProbeRequiredContextKey) &&
		(c.Request == nil || c.Request.Context().Err() == nil) && !IsResponseCommitted(c) && !c.Writer.Written()
}

// accountForExcelBPSFallback is a request-local view. The shared account keeps
// its BPS setting for subsequent requests and ordinary routing regains its flags.
func accountForExcelBPSFallback(account *Account) *Account {
	ordinary := *account
	ordinary.Extra = make(map[string]any, len(account.Extra)+1)
	for key, value := range account.Extra {
		ordinary.Extra[key] = value
	}
	ordinary.Extra["openai_excel_bps"] = false
	return &ordinary
}

func resetExcelBPSFallbackContext(c *gin.Context) {
	// Keep the attempt event history while clearing the current-attempt summary.
	c.Set(OpsUpstreamStatusCodeKey, 0)
	c.Set(OpsUpstreamErrorMessageKey, "")
	c.Set(OpsUpstreamErrorDetailKey, "")
	ClearActualOpenAIUpstreamEndpoint(c)
	ClearOpsUpstreamModel(c)
	for _, header := range []string{"Content-Type", "Cache-Control", "X-Accel-Buffering"} {
		c.Writer.Header().Del(header)
	}
}

type excelBPSFallbackContextKey struct{}
type excelBPSFailoverContextKey struct{}

func withExcelBPSFallbackContext(ctx context.Context) context.Context {
	return context.WithValue(ctx, excelBPSFallbackContextKey{}, true)
}

func withExcelBPSFailoverContext(ctx context.Context) context.Context {
	return context.WithValue(ctx, excelBPSFailoverContextKey{}, true)
}

func excelBPSFallbackContextEnabled(ctx context.Context) bool {
	if ctx == nil {
		return false
	}
	enabled, _ := ctx.Value(excelBPSFallbackContextKey{}).(bool)
	return enabled
}

func excelBPSFailoverContextEnabled(ctx context.Context) bool {
	if ctx == nil {
		return false
	}
	enabled, _ := ctx.Value(excelBPSFailoverContextKey{}).(bool)
	return enabled
}

func accountForExcelBPSFallbackAdmission(ctx context.Context, selected, latest *Account) (*Account, bool) {
	if !excelBPSFallbackContextEnabled(ctx) || selected == nil || latest == nil {
		return nil, false
	}
	ordinarySelected := accountForExcelBPSFallback(selected)
	ordinaryLatest := accountForExcelBPSFallback(latest)
	if openAITurnRouteFingerprint(ordinaryLatest) != openAITurnRouteFingerprint(ordinarySelected) {
		return nil, false
	}
	return ordinaryLatest, true
}

// WithExcelBPSFallbackContext marks the second routing phase that uses the
// ordinary OpenAI upstream after the BPS account attempts are exhausted.
func WithExcelBPSFallbackContext(ctx context.Context) context.Context {
	return withExcelBPSFallbackContext(ctx)
}

// WithExcelBPSFailoverContext enables request-level BPS failover classification.
func WithExcelBPSFailoverContext(ctx context.Context) context.Context {
	return withExcelBPSFailoverContext(ctx)
}

// ResetExcelBPSFallbackContext clears attempt-local BPS response metadata before
// the ordinary upstream phase writes its response.
func ResetExcelBPSFallbackContext(c *gin.Context) {
	if c == nil || c.Writer == nil {
		return
	}
	resetExcelBPSFallbackContext(c)
}

// IsExcelBPSFailover identifies an account failover raised by the BPS route.
func IsExcelBPSFailover(err error) bool {
	var failover *UpstreamFailoverError
	if !errors.As(err, &failover) || failover == nil {
		return false
	}
	scope := strings.ToLower(strings.TrimSpace(string(failover.Scope)))
	return scope == "bps" || scope == "excel_bps"
}

func excelBPSCanFailover(ctx context.Context, c *gin.Context) bool {
	return excelBPSCanRateLimitFailover(ctx, c)
}

func excelBPSShouldFailoverStatus(status int) bool {
	return status == http.StatusUnauthorized || status == http.StatusForbidden || status == http.StatusTooManyRequests || status >= http.StatusInternalServerError
}

func newExcelBPSFailureFailover(c *gin.Context, account *Account, status int, headers http.Header, code, message string) *UpstreamFailoverError {
	body, _ := json.Marshal(map[string]any{"error": map[string]string{
		"type": "server_error", "code": code, "message": message,
	}})
	responseHeaders := headers.Clone()
	if responseHeaders == nil {
		responseHeaders = make(http.Header)
	}
	if c != nil {
		setOpsUpstreamError(c, status, message, "")
		if account != nil {
			appendOpsUpstreamError(c, OpsUpstreamErrorEvent{
				Platform: account.Platform, AccountID: account.ID, AccountName: account.Name,
				ProxyID: opsUpstreamProxyID(account), ProxyName: opsUpstreamProxyName(account),
				UpstreamStatusCode: status, UpstreamRequestID: responseHeaders.Get("x-request-id"),
				UpstreamURL: basispoints.ResponsesURL, Kind: "failover", Stage: string(GatewayFailureStageInference), Scope: "bps",
				Reason: code, Message: message,
			})
		}
	}
	return &UpstreamFailoverError{
		StatusCode: status, ResponseBody: body, ResponseHeaders: responseHeaders,
		Stage: GatewayFailureStageInference, Scope: GatewayFailureScope("bps"),
		Reason: GatewayFailureReason(code), NextAccountAction: NextAccountRetry,
		SafeToFailoverAfterWrite: c != nil && c.Writer != nil && c.Writer.Written(),
	}
}
