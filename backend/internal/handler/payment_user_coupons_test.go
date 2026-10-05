//go:build unit

package handler

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"entgo.io/ent/dialect"
	entsql "entgo.io/ent/dialect/sql"
	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/ent/enttest"
	"github.com/Wei-Shaw/sub2api/internal/payment"
	"github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	_ "modernc.org/sqlite"
)

func TestGetMyDiscountCouponsRequiresAuthentication(t *testing.T) {
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodGet, "/api/v1/payment/coupons", nil)
	NewPaymentHandler(nil, nil).GetMyDiscountCoupons(c)
	require.Equal(t, http.StatusUnauthorized, recorder.Code)
}

func TestGetMyDiscountCouponsUsesAuthenticatedUser(t *testing.T) {
	ctx := context.Background()
	db, err := sql.Open("sqlite", "file:profile_coupons_handler?mode=memory&cache=shared&_pragma=foreign_keys(1)")
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	client := enttest.NewClient(t, enttest.WithOptions(dbent.Driver(entsql.OpenDB(dialect.SQLite, db))))
	t.Cleanup(func() { _ = client.Close() })
	_, err = client.ExecContext(ctx, `CREATE TABLE recharge_discount_coupons (
		id INTEGER PRIMARY KEY, user_id INTEGER, min_recharge_amount REAL, discount_percent REAL,
		total_uses INTEGER, status TEXT, created_by INTEGER, notes TEXT, created_at TIMESTAMP,
		updated_at TIMESTAMP, source_type TEXT, source_id INTEGER, source_code TEXT)`)
	require.NoError(t, err)
	_, err = client.ExecContext(ctx, `CREATE TABLE subscription_discount_coupons (
		id INTEGER PRIMARY KEY, user_id INTEGER, min_subscription_amount REAL, discount_percent REAL,
		total_uses INTEGER, status TEXT, created_by INTEGER, notes TEXT, created_at TIMESTAMP,
		updated_at TIMESTAMP, source_type TEXT)`)
	require.NoError(t, err)
	now := time.Now().UTC()
	for _, owner := range []int64{42, 99} {
		_, err = client.ExecContext(ctx, `INSERT INTO recharge_discount_coupons
			VALUES ($1, $1, 100, 80, 3, 'active', 7, 'internal note', $2, $2, 'promo_code', 5, 'PRIVATE')`, owner, now)
		require.NoError(t, err)
		_, err = client.ExecContext(ctx, `INSERT INTO subscription_discount_coupons
			VALUES ($1, $1, 20, 90, 2, 'active', 7, 'internal note', $2, $2, 'admin')`, owner, now)
		require.NoError(t, err)
	}
	svc := service.NewPaymentService(client, payment.NewRegistry(), nil, nil, nil, nil, nil, nil, nil, nil)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodGet, "/api/v1/payment/coupons?user_id=99", nil)
	c.Set(string(middleware.ContextKeyUser), middleware.AuthSubject{UserID: 42})
	NewPaymentHandler(svc, nil).GetMyDiscountCoupons(c)
	require.Equal(t, http.StatusOK, recorder.Code)
	var payload struct {
		Data []map[string]any `json:"data"`
	}
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &payload))
	require.Len(t, payload.Data, 2)
	for _, coupon := range payload.Data {
		require.Equal(t, float64(42), coupon["id"])
		require.Equal(t, "available", coupon["status"])
		for _, field := range []string{"user_id", "created_by", "notes", "source_id", "source_code"} {
			require.NotContains(t, coupon, field)
		}
	}
}
