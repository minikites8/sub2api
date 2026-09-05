package service

import (
	"context"
	"database/sql"
	"fmt"
	"testing"
	"time"

	"entgo.io/ent/dialect"
	entsql "entgo.io/ent/dialect/sql"
	_ "modernc.org/sqlite"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/ent/enttest"
	"github.com/Wei-Shaw/sub2api/internal/payment"
	"github.com/stretchr/testify/require"
)

func newRechargeCouponTestClient(t *testing.T) *dbent.Client {
	t.Helper()
	dsn := fmt.Sprintf("file:recharge_coupon_%d?mode=memory&cache=shared&_fk=1", time.Now().UnixNano())
	db, err := sql.Open("sqlite", dsn)
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	_, err = db.Exec("PRAGMA foreign_keys = ON")
	require.NoError(t, err)
	driver := entsql.OpenDB(dialect.SQLite, db)
	client := enttest.NewClient(t, enttest.WithOptions(dbent.Driver(driver)))
	t.Cleanup(func() { _ = client.Close() })
	return client
}

func createRechargeCouponTestTable(t *testing.T, client *dbent.Client) {
	t.Helper()
	_, err := client.ExecContext(context.Background(), `
CREATE TABLE recharge_discount_coupons (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  user_id INTEGER NOT NULL,
  min_recharge_amount DECIMAL(20,8) NOT NULL,
  discount_percent DECIMAL(5,2) NOT NULL,
  total_uses INTEGER NOT NULL,
  status VARCHAR(20) NOT NULL,
  created_by INTEGER NOT NULL,
  notes TEXT,
  created_at TIMESTAMP NOT NULL,
  updated_at TIMESTAMP NOT NULL
)`)
	require.NoError(t, err)
}

func TestApplyRechargeDiscountCouponAppliesEligibleStrongerDiscount(t *testing.T) {
	plan := buildFirstRechargeAmountPlan(100, 90, &firstRechargePromo{
		PromoCodeID:     8,
		BonusAmount:     10,
		DiscountPercent: 90,
		DiscountTimes:   2,
		DiscountSet:     true,
	})
	coupon := &RechargeDiscountCouponPreview{
		ID:                21,
		MinRechargeAmount: 100,
		DiscountPercent:   80,
		TotalUses:         3,
		RemainingUses:     2,
	}

	plan = applyRechargeDiscountCoupon(100, plan, coupon)

	require.Equal(t, int64(21), plan.CouponID)
	require.Equal(t, 100.0, plan.BaseCreditAmount)
	require.Equal(t, 110.0, plan.CreditAmount)
	require.Equal(t, 80.0, plan.PaymentAmount)
	require.Equal(t, 80.0, plan.DiscountPercent)

	snapshot := appendFirstRechargePromoSnapshot(nil, plan)
	restored, ok := firstRechargeAmountPlanFromSnapshot(snapshot)
	require.True(t, ok)
	require.Equal(t, plan, restored)
}

func TestApplyRechargeDiscountCouponKeepsStrongerPromoAndEnforcesThreshold(t *testing.T) {
	promoPlan := buildFirstRechargeAmountPlan(100, 100, &firstRechargePromo{
		PromoCodeID:     8,
		DiscountPercent: 70,
		DiscountSet:     true,
	})
	coupon := &RechargeDiscountCouponPreview{
		ID:                21,
		MinRechargeAmount: 100,
		DiscountPercent:   80,
		TotalUses:         3,
		RemainingUses:     3,
	}

	require.Equal(t, promoPlan, applyRechargeDiscountCoupon(100, promoPlan, coupon))
	require.Equal(t, firstRechargeAmountPlan{BaseCreditAmount: 99, CreditAmount: 99, PaymentAmount: 99}, applyRechargeDiscountCoupon(99, firstRechargeAmountPlan{BaseCreditAmount: 99, CreditAmount: 99, PaymentAmount: 99}, coupon))
}

func TestListAvailableRechargeDiscountCouponsTracksReservedStatuses(t *testing.T) {
	ctx := context.Background()
	client := newRechargeCouponTestClient(t)
	createRechargeCouponTestTable(t, client)

	user, err := client.User.Create().
		SetEmail("coupon@example.com").
		SetPasswordHash("hash").
		SetUsername("coupon-user").
		Save(ctx)
	require.NoError(t, err)
	now := time.Now().UTC()
	_, err = client.ExecContext(ctx, `
INSERT INTO recharge_discount_coupons
  (user_id, min_recharge_amount, discount_percent, total_uses, status, created_by, created_at, updated_at)
VALUES ($1, 100, 80, 2, 'active', 9, $2, $2)`, user.ID, now)
	require.NoError(t, err)

	createOrder := func(status, tradeNo string) {
		_, createErr := client.PaymentOrder.Create().
			SetUserID(user.ID).
			SetUserEmail(user.Email).
			SetUserName(user.Username).
			SetAmount(100).
			SetPayAmount(80).
			SetFeeRate(0).
			SetRechargeCode("PAY-" + tradeNo).
			SetOutTradeNo(tradeNo).
			SetPaymentType(payment.TypeAlipay).
			SetPaymentTradeNo("").
			SetOrderType(payment.OrderTypeBalance).
			SetStatus(status).
			SetExpiresAt(now.Add(time.Hour)).
			SetClientIP("127.0.0.1").
			SetSrcHost("example.com").
			SetProviderSnapshot(map[string]any{
				"recharge_discount_coupon": map[string]any{
					"coupon_id":           1,
					"min_recharge_amount": 100,
					"base_amount":         100,
					"discount_percent":    80,
					"total_uses":          2,
					"credited_amount":     100,
					"payment_amount":      80,
				},
			}).
			Save(ctx)
		require.NoError(t, createErr)
	}
	createOrder(OrderStatusPending, "coupon-pending")
	createOrder(OrderStatusCancelled, "coupon-cancelled")

	svc := &PaymentService{entClient: client}
	coupons, err := svc.ListAvailableRechargeDiscountCoupons(ctx, user.ID)

	require.NoError(t, err)
	require.Len(t, coupons, 1)
	require.Equal(t, 1, coupons[0].UsedCount)
	require.Equal(t, 1, coupons[0].RemainingUses)
}

func TestIssueRechargeDiscountCouponPersistsAdminGrant(t *testing.T) {
	ctx := context.Background()
	client := newRechargeCouponTestClient(t)
	createRechargeCouponTestTable(t, client)
	user, err := client.User.Create().
		SetEmail("issued-coupon@example.com").
		SetPasswordHash("hash").
		SetUsername("issued-coupon-user").
		Save(ctx)
	require.NoError(t, err)

	svc := &adminServiceImpl{entClient: client}
	coupon, err := svc.IssueRechargeDiscountCoupon(ctx, user.ID, IssueRechargeDiscountCouponInput{
		MinRechargeAmount: 200,
		DiscountPercent:   85,
		TotalUses:         3,
		CreatedBy:         99,
		Notes:             " retention ",
	})

	require.NoError(t, err)
	require.Equal(t, user.ID, coupon.UserID)
	require.Equal(t, 200.0, coupon.MinRechargeAmount)
	require.Equal(t, 85.0, coupon.DiscountPercent)
	require.Equal(t, 3, coupon.RemainingUses)
	require.Equal(t, int64(99), coupon.CreatedBy)
	require.Equal(t, "retention", coupon.Notes)

	coupons, err := svc.ListUserRechargeDiscountCoupons(ctx, user.ID)
	require.NoError(t, err)
	require.Len(t, coupons, 1)
	require.Equal(t, coupon.ID, coupons[0].ID)
	require.Equal(t, 0, coupons[0].UsedCount)
	require.Equal(t, 3, coupons[0].RemainingUses)
}
