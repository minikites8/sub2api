package service

import (
	"context"
	"net/http"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service/basispoints"
	"github.com/gin-gonic/gin"
	"github.com/tidwall/gjson"
)

func excelBPSRateLimitError(payload []byte) bool {
	for _, prefix := range []string{"error", "response.error", ""} {
		value := gjson.ParseBytes(payload)
		if prefix != "" {
			value = gjson.GetBytes(payload, prefix)
		}
		if !value.IsObject() {
			continue
		}
		if value.Get("status").Int() == http.StatusTooManyRequests || value.Get("status_code").Int() == http.StatusTooManyRequests {
			return true
		}
		for _, field := range []string{"code", "type"} {
			switch strings.ToLower(strings.TrimSpace(value.Get(field).String())) {
			case "rate_limit_exceeded", "rate_limit_error", "too_many_requests", "basispoints_rate_limited", "usage_limit_reached":
				return true
			}
		}
		message := strings.ToLower(strings.TrimSpace(value.Get("message").String()))
		if strings.HasPrefix(message, "rate limit reached") || strings.HasPrefix(message, "rate limit exceeded") {
			return true
		}
	}
	return false
}

func excelBPSCanRateLimitFailover(ctx context.Context, c *gin.Context) bool {
	return ctx.Err() == nil && c != nil && c.Writer != nil && !c.GetBool(bpsAccountProbeRequiredContextKey) &&
		(c.Request == nil || c.Request.Context().Err() == nil) && !IsResponseCommitted(c)
}

// A BPS throttle switches accounts within the request. Keep Codex quota and
// persistent cooldown state independent from endpoint-specific BPS throttling.
// Call only before semantic output; any bytes already sent are SSE keepalives.
func excelBPSRateLimitFailover(c *gin.Context, account *Account, headers http.Header, payload []byte) *UpstreamFailoverError {
	responseHeaders := headers.Clone()
	if responseHeaders == nil {
		responseHeaders = make(http.Header)
	}
	for _, prefix := range []string{"error.headers", "response.error.headers", "headers"} {
		retryAfter := strings.TrimSpace(gjson.GetBytes(payload, prefix+".retry-after").String())
		if validOpenAIPassthroughRetryAfter(retryAfter, time.Now()) {
			responseHeaders.Set("Retry-After", retryAfter)
			break
		}
	}
	const message = "Excel BPS rate limit exceeded"
	const body = "{\"error\":{\"type\":\"rate_limit_error\",\"code\":\"basispoints_rate_limited\",\"message\":\"Excel BPS rate limit exceeded\"}}"
	setOpsUpstreamError(c, http.StatusTooManyRequests, message, "")
	appendOpsUpstreamError(c, OpsUpstreamErrorEvent{
		Platform: account.Platform, AccountID: account.ID, AccountName: account.Name,
		ProxyID: opsUpstreamProxyID(account), ProxyName: opsUpstreamProxyName(account),
		UpstreamStatusCode: http.StatusTooManyRequests, UpstreamRequestID: headers.Get("x-request-id"),
		UpstreamURL: basispoints.ResponsesURL, Kind: "failover", Scope: "bps", Reason: "rate_limit_exceeded", Message: message,
	})
	return &UpstreamFailoverError{
		StatusCode: http.StatusTooManyRequests, ResponseBody: []byte(body), ResponseHeaders: responseHeaders,
		RequestScopedTransient: true, SafeToFailoverAfterWrite: c.Writer.Written(),
	}
}

// Empty lifecycle and content placeholders can precede a rejected generation.
// Reuse the gateway's content checks while preserving conservative treatment of
// unknown events, encrypted reasoning, tool arguments and actual text.
func excelBPSStreamEventIsPrelude(payload []byte, kind string) bool {
	if !gjson.ValidBytes(payload) {
		return false
	}
	switch kind {
	case "response.created", "response.in_progress", "keepalive":
		output := gjson.GetBytes(payload, "response.output")
		if output.Exists() && output.Type != gjson.Null && !output.IsArray() {
			return false
		}
		for _, item := range output.Array() {
			wrapped := []byte("{\"item\":" + item.Raw + "}")
			if openAIStreamItemHasVisibleOutput(item) || openAIStreamAddedEventStartsClientOutput(wrapped, "response.output_item.added") {
				return false
			}
		}
		return true
	case "response.output_item.added", "response.output_item.done":
		return !openAIStreamItemHasVisibleOutput(gjson.GetBytes(payload, "item")) &&
			!openAIStreamAddedEventStartsClientOutput(payload, "response.output_item.added")
	case "response.content_part.added", "response.content_part.done":
		return !openAIStreamAddedEventStartsClientOutput(payload, "response.content_part.added")
	case "response.reasoning_summary_part.added", "response.reasoning_summary_part.done":
		return !openAIStreamAddedEventStartsClientOutput(payload, "response.reasoning_summary_part.added")
	case "response.output_text.delta", "response.reasoning_summary_text.delta", "response.reasoning_text.delta",
		"response.refusal.delta", "response.function_call_arguments.delta", "response.custom_tool_call_input.delta":
		delta := gjson.GetBytes(payload, "delta")
		return delta.Type == gjson.String && delta.String() == ""
	case "response.output_text.done", "response.reasoning_summary_text.done", "response.reasoning_text.done":
		text := gjson.GetBytes(payload, "text")
		return text.Type == gjson.String && text.String() == ""
	default:
		return false
	}
}
