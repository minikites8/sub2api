package service

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/service/basispoints"
	"github.com/gin-gonic/gin"
	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
)

// ExcelBPSPublicError contains only protocol-independent client diagnostics.
// Keep upstream identifiers and repair/scheduling details in Ops, never here.
type ExcelBPSPublicError struct {
	Status    int               `json:"status"`
	Type      string            `json:"type"`
	Code      string            `json:"code"`
	Message   string            `json:"message"`
	Retryable *bool             `json:"retryable,omitempty"`
	Headers   map[string]string `json:"headers,omitempty"`
}

func excelBPSPublicError(status int) ExcelBPSPublicError {
	if status < 400 || status > 599 {
		status = http.StatusBadGateway
	}
	result := ExcelBPSPublicError{Status: status, Type: "invalid_request_error", Code: "invalid_request_error"}
	message := http.StatusText(status)
	switch status {
	case http.StatusBadRequest, http.StatusUnprocessableEntity:
		message = "Invalid request body"
	case http.StatusUnauthorized:
		result.Type, result.Code = "authentication_error", "authentication_error"
	case http.StatusForbidden:
		result.Type, result.Code = "permission_error", "permission_denied"
	case http.StatusNotFound:
		result.Code = "not_found"
	case http.StatusRequestTimeout:
		result.Code = "request_timeout"
	case http.StatusRequestEntityTooLarge:
		result.Code, message = "request_too_large", "Request body too large"
	case http.StatusTooManyRequests:
		result.Type, result.Code, message = "rate_limit_error", "rate_limit_exceeded", "Too many requests"
	default:
		if status >= 500 {
			result.Type, result.Code = "server_error", "server_error"
		}
	}
	switch status {
	case http.StatusBadGateway:
		result.Code, message = "bad_gateway", "Bad gateway"
	case http.StatusServiceUnavailable:
		result.Code, message = "service_unavailable", "Service unavailable"
	case http.StatusGatewayTimeout:
		result.Code, message = "gateway_timeout", "Gateway timeout"
	}
	if message == "" {
		message = "Request failed"
	}
	result.Message = fmt.Sprintf("%d: %s.", status, message)
	return result
}

// ExcelBPSPublicError maps the final rejection only; the original error still
// drives retries, cooldowns and internal diagnostics.
func (e *UpstreamFailoverError) ExcelBPSPublicError() *ExcelBPSPublicError {
	if e == nil || (e.Scope != excelBPSFailureScope && e.Reason != ExcelBPSRateLimitedReason && !e.IsExcelBPSImageAdmissionRejection()) {
		return nil
	}
	status := e.StatusCode
	if e.ClientStatusCode != 0 {
		status = e.ClientStatusCode
	}
	public := excelBPSPublicError(status)
	return &public
}

func writeExcelBPSPublicError(c *gin.Context, status int, param string) {
	writeExcelBPSPublicErrorBody(c, excelBPSPublicError(status), param)
}

func (p *ExcelBPSPublicError) retainRetryHints(failure *basispoints.UpstreamFailure) {
	if failure == nil {
		return
	}
	if failure.ReplayUnsafe {
		retryable := false
		p.Retryable = &retryable
	}
	if failure.RetryAfter != "" {
		p.Headers = map[string]string{"retry-after": failure.RetryAfter}
	}
}

func writeExcelBPSPublicErrorBody(c *gin.Context, public ExcelBPSPublicError, param string) {
	committed := StopOpenAICompactSSEKeepaliveCommitted(c) ||
		c.Writer.Written() && strings.HasPrefix(c.Writer.Header().Get("Content-Type"), "text/event-stream")
	MarkResponseCommitted(c)
	if committed {
		writeOpenAICompactSSEFailureMessageParam(c, public.Status, public.Code, public.Message, param)
		return
	}
	c.Header("Content-Type", "application/json; charset=utf-8")
	errorBody := gin.H{"status": public.Status, "type": public.Type, "code": public.Code, "message": public.Message}
	if param != "" {
		errorBody["param"] = param
	}
	if public.Retryable != nil {
		errorBody["retryable"] = *public.Retryable
	}
	if public.Headers != nil {
		errorBody["headers"] = public.Headers
	}
	c.JSON(public.Status, gin.H{"error": errorBody})
}

// Sanitize only after interpreting the original terminal for retry and usage
// accounting. Replace the whole error object so bridge-only metadata cannot
// escape, while preserving response identity, output and usage.
func excelBPSPublicFailureEvent(payload []byte, status int) []byte {
	path := "error"
	if gjson.GetBytes(payload, "response").IsObject() {
		path = "response.error"
	}
	public := excelBPSPublicError(status)
	public.retainRetryHints(basispoints.ParseUpstreamFailure(payload))
	out, err := sjson.SetBytes(payload, path, public)
	if err != nil {
		return []byte(fmt.Sprintf(`{"type":"error","error":{"status":%d,"type":%q,"code":%q,"message":%q}}`, public.Status, public.Type, public.Code, public.Message))
	}
	// Error frames can also carry a redundant top-level error envelope.
	if path != "error" && gjson.GetBytes(out, "error").Exists() {
		out, _ = sjson.DeleteBytes(out, "error")
	}
	return out
}

func excelBPSPublicImagesError(err *OpenAIImagesUpstreamError) *OpenAIImagesUpstreamError {
	public := excelBPSPublicError(err.clientStatusCode())
	return &OpenAIImagesUpstreamError{StatusCode: public.Status, ErrorType: public.Type, Code: public.Code, Message: public.Message}
}
