package service

import (
	"net/http"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/pkg/requestretry"
	"github.com/gin-gonic/gin"
	"github.com/tidwall/gjson"
)

// requestAccountRetryOnError extends classification only for explicit upstream
// failures. Existing classifiers remain authoritative without the header.
// Callers still enforce pre-output replay safety and account-switch budgets.
func requestAccountRetryOnError(c *gin.Context, status int, message string, body []byte) bool {
	if c == nil || !requestretry.Enabled(c.Request) || c.Request.Context().Err() != nil ||
		isControlledExperiment(c.Request.Context()) || IsResponseCommitted(c) || status < http.StatusBadRequest {
		return false
	}
	if hit, _, _ := detectOpenAICyberPolicy(body); hit {
		return false
	}
	if isOpenAIContextWindowError(message, body) {
		return false
	}
	// Inspect error fields only: echoed prompts must not change classification.
	for _, path := range []string{"error", "response.error"} {
		if value := gjson.GetBytes(body, path); value.Exists() && isGrokContentPolicyRejection(http.StatusForbidden, []byte(value.Raw)) {
			return false
		}
	}
	if !gjson.ValidBytes(body) && isGrokContentPolicyRejection(http.StatusForbidden, body) {
		return false
	}
	for _, path := range []string{"error.code", "error.type", "response.error.code", "response.error.type"} {
		marker := strings.ToLower(gjson.GetBytes(body, path).String())
		if strings.Contains(marker, "content_policy") || strings.Contains(marker, "safety") || strings.Contains(marker, "content_filter") {
			return false
		}
	}
	return true
}

func requestAccountRetryFromContexts(contexts []*gin.Context, status int, message string, body []byte) bool {
	return len(contexts) > 0 && requestAccountRetryOnError(contexts[0], status, message, body)
}

func requestAccountRetryError(c *gin.Context, account *Account, resp *http.Response, body []byte) *UpstreamFailoverError {
	if account == nil || resp == nil || !requestAccountRetryOnError(c, resp.StatusCode, extractUpstreamErrorMessage(body), body) {
		return nil
	}
	appendOpsUpstreamError(c, OpsUpstreamErrorEvent{
		ProxyID: opsUpstreamProxyID(account), ProxyName: opsUpstreamProxyName(account),
		Platform: account.Platform, AccountID: account.ID, AccountName: account.Name,
		UpstreamStatusCode: resp.StatusCode, UpstreamRequestID: resp.Header.Get("x-request-id"),
		Kind: "failover", Message: sanitizeUpstreamErrorMessage(extractUpstreamErrorMessage(body)),
	})
	return &UpstreamFailoverError{
		StatusCode: resp.StatusCode, ResponseBody: body, ResponseHeaders: resp.Header.Clone(),
		NextAccountAction: NextAccountRetry,
	}
}
