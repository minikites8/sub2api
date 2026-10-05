package handler

import (
	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/gin-gonic/gin"
)

// GetMyDiscountCoupons returns coupons owned by the authenticated user.
// GET /api/v1/payment/coupons
func (h *PaymentHandler) GetMyDiscountCoupons(c *gin.Context) {
	subject, ok := requireAuth(c)
	if !ok {
		return
	}
	coupons, err := h.paymentService.ListMyDiscountCoupons(c.Request.Context(), subject.UserID)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, coupons)
}
