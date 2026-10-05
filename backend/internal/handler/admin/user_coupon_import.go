package admin

import (
	"context"
	"net/http"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
)

// ImportDiscountCoupons validates or atomically issues the rows parsed from Excel.
func (h *UserHandler) ImportDiscountCoupons(c *gin.Context) {
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 5<<20)
	var input service.ImportDiscountCouponsInput
	if err := c.ShouldBindJSON(&input); err != nil {
		response.BadRequest(c, "Invalid coupon import request")
		return
	}
	input.CreatedBy = getAdminIDFromContext(c)
	if input.DryRun {
		result, err := h.adminService.ImportDiscountCoupons(c.Request.Context(), input)
		if err != nil {
			response.ErrorFrom(c, err)
			return
		}
		response.Success(c, result)
		return
	}
	executeAdminIdempotentJSONWithTimeout(c, "admin.users.discount_coupons.import", input,
		service.DefaultWriteIdempotencyTTL(), 2*time.Minute, func(ctx context.Context) (any, error) {
			return h.adminService.ImportDiscountCoupons(ctx, input)
		})
}
