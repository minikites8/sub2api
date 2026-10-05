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

type couponImportAdminStub struct {
	service.AdminService
	input service.ImportDiscountCouponsInput
	calls int
}

func (s *couponImportAdminStub) ImportDiscountCoupons(_ context.Context, input service.ImportDiscountCouponsInput) (*service.ImportDiscountCouponsResult, error) {
	s.calls++
	s.input = input
	return &service.ImportDiscountCouponsResult{Valid: true, Rows: []service.DiscountCouponImportRowResult{{RowNumber: 2, UserID: 42}}}, nil
}

func TestUserCouponImportHandlerPreservesRowsAndUsesAuthenticatedAdmin(t *testing.T) {
	gin.SetMode(gin.TestMode)
	stub := &couponImportAdminStub{}
	handler := NewUserHandler(stub, nil, nil, nil, nil, nil, nil)
	router := gin.New()
	router.Use(func(c *gin.Context) { c.Set(string(middleware.ContextKeyUser), middleware.AuthSubject{UserID: 99}) })
	router.POST("/users/discount-coupons/import", handler.ImportDiscountCoupons)
	request := httptest.NewRequest(http.MethodPost, "/users/discount-coupons/import", bytes.NewBufferString(`{
	  "dry_run":true,"created_by":1,"rows":[{"row_number":2,"user_id":42,"coupon_type":"subscription","min_amount":20,"discount_rate":8.5,"total_uses":3}]
	}`))
	request.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)
	require.Equal(t, http.StatusOK, recorder.Code)
	require.Equal(t, int64(99), stub.input.CreatedBy)
	require.True(t, stub.input.DryRun)
	require.Len(t, stub.input.Rows, 1)
	require.Equal(t, 8.5, stub.input.Rows[0].DiscountRate)
	require.Equal(t, 20.0, stub.input.Rows[0].MinAmount)
}

func TestUserCouponImportHandlerRejectsMalformedJSON(t *testing.T) {
	gin.SetMode(gin.TestMode)
	stub := &couponImportAdminStub{}
	handler := NewUserHandler(stub, nil, nil, nil, nil, nil, nil)
	router := gin.New()
	router.POST("/users/discount-coupons/import", handler.ImportDiscountCoupons)
	request := httptest.NewRequest(http.MethodPost, "/users/discount-coupons/import", bytes.NewBufferString(`{"rows":[{"total_uses":1.5}]}`))
	request.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)
	require.Equal(t, http.StatusBadRequest, recorder.Code)
	require.Zero(t, stub.calls)
}

func TestUserCouponImportHandlerReplaysIssuanceAndRejectsChangedBatch(t *testing.T) {
	gin.SetMode(gin.TestMode)
	previous := service.DefaultIdempotencyCoordinator()
	cfg := service.DefaultIdempotencyConfig()
	cfg.ObserveOnly = false
	service.SetDefaultIdempotencyCoordinator(service.NewIdempotencyCoordinator(newMemoryIdempotencyRepoStub(), cfg))
	t.Cleanup(func() { service.SetDefaultIdempotencyCoordinator(previous) })
	stub := &couponImportAdminStub{}
	handler := NewUserHandler(stub, nil, nil, nil, nil, nil, nil)
	router := gin.New()
	router.Use(func(c *gin.Context) { c.Set(string(middleware.ContextKeyUser), middleware.AuthSubject{UserID: 99}) })
	router.POST("/users/discount-coupons/import", handler.ImportDiscountCoupons)
	call := func(body, key string) *httptest.ResponseRecorder {
		request := httptest.NewRequest(http.MethodPost, "/users/discount-coupons/import", bytes.NewBufferString(body))
		request.Header.Set("Content-Type", "application/json")
		request.Header.Set("Idempotency-Key", key)
		recorder := httptest.NewRecorder()
		router.ServeHTTP(recorder, request)
		return recorder
	}
	body := `{"rows":[{"row_number":2,"user_id":42,"coupon_type":"subscription","min_amount":20,"discount_rate":8,"total_uses":3}]}`
	require.Equal(t, http.StatusBadRequest, call(body, "").Code)
	first := call(body, "coupon-batch-once")
	require.Equal(t, http.StatusOK, first.Code, first.Body.String())
	replayed := call(body, "coupon-batch-once")
	require.Equal(t, http.StatusOK, replayed.Code, replayed.Body.String())
	require.Equal(t, "true", replayed.Header().Get("X-Idempotency-Replayed"))
	require.JSONEq(t, first.Body.String(), replayed.Body.String())
	require.Equal(t, 1, stub.calls)
	changed := `{"rows":[{"row_number":2,"user_id":42,"coupon_type":"subscription","min_amount":21,"discount_rate":8,"total_uses":3}]}`
	require.Equal(t, http.StatusConflict, call(changed, "coupon-batch-once").Code)
	require.Equal(t, 1, stub.calls)
}
