package service

import (
	"context"
	"errors"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/tidwall/gjson"
)

var errExcelBPSModelUnavailable = errors.New("Excel BPS model unavailable; continue through the ordinary upstream")

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
	return c != nil && c.Writer != nil && !IsResponseCommitted(c) && !c.Writer.Written()
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

func withExcelBPSFallbackContext(ctx context.Context) context.Context {
	return context.WithValue(ctx, excelBPSFallbackContextKey{}, true)
}

func excelBPSFallbackContextEnabled(ctx context.Context) bool {
	if ctx == nil {
		return false
	}
	enabled, _ := ctx.Value(excelBPSFallbackContextKey{}).(bool)
	return enabled
}

func accountForExcelBPSFallbackAdmission(ctx context.Context, selected, latest *Account) (*Account, bool) {
	if !excelBPSFallbackContextEnabled(ctx) || selected == nil || latest == nil {
		return nil, false
	}
	ordinary := accountForExcelBPSFallback(latest)
	if openAITurnRouteFingerprint(ordinary) != openAITurnRouteFingerprint(selected) {
		return nil, false
	}
	return ordinary, true
}
