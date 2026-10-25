package handler

import "github.com/gin-gonic/gin"

func (h *OpenAIGatewayHandler) prepareGPTQoSRequest(c *gin.Context, groupID *int64, body []byte) func() {
	if h == nil || h.gatewayService == nil {
		return func() {}
	}
	parent := c.Request.Context()
	ctx, finish := h.gatewayService.PrepareGPTQoSRequest(parent, groupID, body)
	c.Request = c.Request.WithContext(ctx)
	return func() {
		finish(c.Writer.Status() < 400 && ctx.Err() == nil)
		// Canceling our timer is cleanup, not a client disconnect. Outer handler
		// defers still inspect Request.Context before Gin commits an error status.
		c.Request = c.Request.WithContext(parent)
	}
}
