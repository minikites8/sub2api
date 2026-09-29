package handler

import (
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
)

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
