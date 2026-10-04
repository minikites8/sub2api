package admin

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type subscriptionCouponAdminServiceStub struct {
	service.AdminService
	userID     int64
	listUserID int64
	input      service.IssueSubscriptionDiscountCouponInput
	coupons    []service.SubscriptionDiscountCoupon
}

func (s *subscriptionCouponAdminServiceStub) ListUserSubscriptionDiscountCoupons(_ context.Context, userID int64) ([]service.SubscriptionDiscountCoupon, error) {
	s.listUserID = userID
	return s.coupons, nil
}

func (s *subscriptionCouponAdminServiceStub) IssueSubscriptionDiscountCoupon(_ context.Context, userID int64, input service.IssueSubscriptionDiscountCouponInput) (*service.SubscriptionDiscountCoupon, error) {
	s.userID = userID
	s.input = input
	return &service.SubscriptionDiscountCoupon{ID: 7, UserID: userID}, nil
}

func TestUserHandlerIssueSubscriptionDiscountCouponConvertsRate(t *testing.T) {
	gin.SetMode(gin.TestMode)
	stub := &subscriptionCouponAdminServiceStub{}
	handler := NewUserHandler(stub, nil, nil, nil, nil, nil, nil)
	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set(string(middleware.ContextKeyUser), middleware.AuthSubject{UserID: 99})
		c.Next()
	})
	router.POST("/api/v1/admin/users/:id/subscription-discount-coupons", handler.IssueSubscriptionDiscountCoupon)

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/v1/admin/users/42/subscription-discount-coupons", bytes.NewBufferString(`{
  "min_subscription_amount": 200,
  "discount_rate": 8.5,
  "total_uses": 3,
  "notes": "retention"
}`))
	request.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(recorder, request)

	require.Equal(t, http.StatusOK, recorder.Code)
	require.Equal(t, int64(42), stub.userID)
	require.Equal(t, 200.0, stub.input.MinSubscriptionAmount)
	require.Equal(t, 85.0, stub.input.DiscountPercent)
	require.Equal(t, 3, stub.input.TotalUses)
	require.Equal(t, int64(99), stub.input.CreatedBy)
	require.Equal(t, "retention", stub.input.Notes)
}

func TestUserHandlerIssueSubscriptionDiscountCouponValidatesFields(t *testing.T) {
	gin.SetMode(gin.TestMode)
	stub := &subscriptionCouponAdminServiceStub{}
	handler := NewUserHandler(stub, nil, nil, nil, nil, nil, nil)
	router := gin.New()
	router.POST("/api/v1/admin/users/:id/subscription-discount-coupons", handler.IssueSubscriptionDiscountCoupon)

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/v1/admin/users/42/subscription-discount-coupons", bytes.NewBufferString(`{"min_subscription_amount":100,"discount_rate":10,"total_uses":1}`))
	request.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(recorder, request)

	require.Equal(t, http.StatusBadRequest, recorder.Code)
	require.Zero(t, stub.userID)
}

func TestUserHandlerListSubscriptionDiscountCoupons(t *testing.T) {
	gin.SetMode(gin.TestMode)
	stub := &subscriptionCouponAdminServiceStub{coupons: []service.SubscriptionDiscountCoupon{{
		ID:                    7,
		UserID:                42,
		MinSubscriptionAmount: 200,
		DiscountPercent:       85,
		TotalUses:             3,
		UsedCount:             1,
		RemainingUses:         2,
		Status:                "active",
	}}}
	handler := NewUserHandler(stub, nil, nil, nil, nil, nil, nil)
	router := gin.New()
	router.GET("/api/v1/admin/users/:id/subscription-discount-coupons", handler.ListSubscriptionDiscountCoupons)

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/api/v1/admin/users/42/subscription-discount-coupons", nil)
	router.ServeHTTP(recorder, request)

	require.Equal(t, http.StatusOK, recorder.Code)
	require.Equal(t, int64(42), stub.listUserID)
	require.Contains(t, recorder.Body.String(), `"remaining_uses":2`)
}
