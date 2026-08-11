package handler

import (
	"net/http"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
)

// ModelAvailabilityMiddleware records the outcome of each authenticated POST
// gateway call after the handler has parsed and stored its requested model.
// The marketplace filters the resulting observations to image/video models.
func ModelAvailabilityMiddleware(tracker *service.ModelAvailabilityTracker, cfgs ...*config.Config) gin.HandlerFunc {
	if tracker == nil {
		tracker = service.DefaultModelAvailabilityTracker()
	}
	return func(c *gin.Context) {
		c.Next()
		if c == nil || c.Request == nil || c.Request.Method != http.MethodPost || tracker == nil || isCountTokensRequest(c) {
			return
		}
		value, exists := c.Get(opsModelKey)
		model, ok := value.(string)
		model = strings.TrimSpace(model)
		if !exists || !ok || model == "" {
			return
		}
		status := c.Writer.Status()
		success := status >= http.StatusOK && status < http.StatusMultipleChoices
		// Streaming handlers keep the wire status at 200 after flushing the
		// response. OpsStreamError carries the final in-band failure in that case.
		if _, streamFailed := service.GetOpsStreamError(c); streamFailed {
			success = false
		}
		checkedAt := time.Now().UTC()
		tracker.RecordAt(model, success, checkedAt)
		for _, cfg := range cfgs {
			service.EnqueueQuotaLeaseDemoModelAvailability(c.Request.Context(), cfg, model, success, checkedAt)
		}
	}
}
