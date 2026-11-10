package handler

import (
	"errors"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
)

// Native capability bypass is a routing decision, so it neither consumes an
// upstream retry nor reports an account health failure. Slots are released by
// the caller before this helper runs.
func reselectExcelBPSNativeBypass(c *gin.Context, writerSizeBeforeForward int, err error, accountID int64, failedAccountIDs map[int64]struct{}) bool {
	if !errors.Is(err, service.ErrExcelBPSNativeBypass) || !openAIRequestAllowsFailoverReplay(c) ||
		!openAIForwardMayFailover(c, writerSizeBeforeForward, nil) {
		return false
	}
	reason := c.Writer.Header().Get("X-Codex2API-Basispoints-Bypass")
	if reason == "" {
		return false
	}
	c.Request = c.Request.WithContext(service.WithExcelBPSNativeBypassContext(c.Request.Context(), reason))
	service.ResetExcelBPSFallbackContext(c)
	failedAccountIDs[accountID] = struct{}{}
	return true
}

// canStartExcelBPSOrdinaryFallback keeps the replay within the pre-output
// window. SSE keepalive bytes remain eligible for the same request replay.
func canStartExcelBPSOrdinaryFallback(c *gin.Context, writerSizeBeforeForward int, failoverErr *service.UpstreamFailoverError) bool {
	return service.IsExcelBPSFailover(failoverErr) && openAIForwardMayFailover(c, writerSizeBeforeForward, failoverErr)
}

// enterExcelBPSOrdinaryFallback marks the request's second routing phase and
// clears attempt-local upstream metadata before the ordinary route writes.
func enterExcelBPSOrdinaryFallback(c *gin.Context) bool {
	if c == nil || c.Request == nil {
		return false
	}
	c.Request = c.Request.WithContext(service.WithExcelBPSFallbackContext(c.Request.Context()))
	service.ResetExcelBPSFallbackContext(c)
	return true
}

func clearExcelBPSFailoverMaps(failedAccountIDs map[int64]struct{}, sameAccountRetryCount map[int64]int) {
	for accountID := range failedAccountIDs {
		delete(failedAccountIDs, accountID)
	}
	for accountID := range sameAccountRetryCount {
		delete(sameAccountRetryCount, accountID)
	}
}
