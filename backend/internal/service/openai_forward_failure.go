package service

import (
	"context"
	"errors"

	"github.com/gin-gonic/gin"
)

// RecordOpenAIForwardFailure retains local failures behind the generic client
// error. A local failure has no upstream HTTP status or inference proxy route.
func RecordOpenAIForwardFailure(c *gin.Context, account *Account, stage, reason string, err error) {
	if c == nil || err == nil || errors.Is(err, context.Canceled) ||
		(c.Request != nil && c.Request.Context().Err() != nil) {
		return
	}
	message := truncateString(redactContentModerationSecrets(sanitizeUpstreamErrorMessage(err.Error())), 2048)
	c.Set(OpsUpstreamStatusCodeKey, 0)
	c.Set(OpsUpstreamErrorDetailKey, "")
	setOpsUpstreamError(c, 0, message, "")
	event := OpsUpstreamErrorEvent{
		Platform: PlatformOpenAI, Kind: "forward_error", Stage: stage,
		Reason: reason, Message: message,
	}
	if account != nil {
		event.Platform, event.AccountID, event.AccountName = account.Platform, account.ID, account.Name
	}
	appendOpsUpstreamError(c, event)
}

// Compare diagnostics per attempt: an earlier account's error must not hide a
// later local failure, and a specific error from this attempt must take priority.
func openAIForwardFailureRecorder(c *gin.Context, account *Account) func(error) {
	if c == nil {
		return func(error) {}
	}
	initialMessage := c.GetString(OpsUpstreamErrorMessageKey)
	initialEvents, _ := c.Get(OpsUpstreamErrorsKey)
	before, _ := initialEvents.([]*OpsUpstreamErrorEvent)
	return func(err error) {
		var failover *UpstreamFailoverError
		if err == nil || errors.As(err, &failover) || IsOpenAITurnAdmissionError(err) {
			return
		}
		currentEvents, _ := c.Get(OpsUpstreamErrorsKey)
		after, _ := currentEvents.([]*OpsUpstreamErrorEvent)
		if len(after) != len(before) || c.GetString(OpsUpstreamErrorMessageKey) != initialMessage {
			return
		}
		RecordOpenAIForwardFailure(c, account, "forward", "unclassified_forward_error", err)
	}
}
