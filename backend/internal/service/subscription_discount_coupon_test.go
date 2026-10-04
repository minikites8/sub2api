package service

import (
	"context"
	"fmt"
	"math"
	"testing"
	"time"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/internal/payment"
	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/stretchr/testify/require"
)

func subscriptionCouponFixture(t *testing.T) (*dbent.Client, *dbent.User, *adminServiceImpl, *PaymentService) {
	t.Helper()
	client := newRechargeCouponTestClient(t)
	_, err := client.ExecContext(context.Background(), `
CREATE TABLE subscription_discount_coupons (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  user_id INTEGER NOT NULL,
  min_subscription_amount DECIMAL(20,8) NOT NULL,
  discount_percent DECIMAL(5,2) NOT NULL,
  total_uses INTEGER NOT NULL,
  status VARCHAR(20) NOT NULL,
  created_by INTEGER NOT NULL,
  notes TEXT,
  created_at TIMESTAMP NOT NULL,
  updated_at TIMESTAMP NOT NULL,
  source_type VARCHAR(20) NOT NULL DEFAULT 'admin'
)`)
	require.NoError(t, err)
	user, err := client.User.Create().SetEmail("subscription-coupon@example.com").
		SetPasswordHash("hash").SetUsername("subscription-coupon").Save(context.Background())
	require.NoError(t, err)
	return client, user, &adminServiceImpl{entClient: client}, &PaymentService{entClient: client}
}

func TestSubscriptionCouponGrantAndBestEligibleDiscount(t *testing.T) {
	ctx := context.Background()
	_, user, admin, svc := subscriptionCouponFixture(t)
	issue := func(minimum, discount float64) *SubscriptionDiscountCoupon {
		coupon, err := admin.IssueSubscriptionDiscountCoupon(ctx, user.ID, IssueSubscriptionDiscountCouponInput{
			MinSubscriptionAmount: minimum, DiscountPercent: discount, TotalUses: 3, CreatedBy: 99, Notes: " retention ",
		})
		require.NoError(t, err)
		require.Equal(t, user.ID, coupon.UserID)
		require.Equal(t, 3, coupon.RemainingUses)
		require.Equal(t, "retention", coupon.Notes)
		return coupon
	}
	issue(200, 50)
	issue(10, 90)
	best := issue(100, 80)
	issue(50, 80)

	plan, err := svc.resolveSubscriptionDiscountCoupon(ctx, user.ID, 100)
	require.NoError(t, err)
	require.Equal(t, best.ID, plan.CouponID)
	require.Equal(t, 100.0, plan.OriginalAmount)
	require.Equal(t, 80.0, plan.DiscountedAmount)
	require.Equal(t, 20.0, plan.DiscountAmount)
	restored, ok := subscriptionDiscountPlanFromSnapshot(appendSubscriptionDiscountSnapshot(nil, plan))
	require.True(t, ok)
	require.Equal(t, plan, restored)

	plan, err = svc.resolveSubscriptionDiscountCoupon(ctx, user.ID, 9.99)
	require.NoError(t, err)
	require.Nil(t, plan)
	coupons, err := svc.ListAvailableSubscriptionDiscountCoupons(ctx, user.ID+1)
	require.NoError(t, err)
	require.Empty(t, coupons)
	all, err := admin.ListUserSubscriptionDiscountCoupons(ctx, user.ID)
	require.NoError(t, err)
	require.Len(t, all, 4)
}

func TestSubscriptionCouponGrantValidation(t *testing.T) {
	_, user, admin, _ := subscriptionCouponFixture(t)
	valid := IssueSubscriptionDiscountCouponInput{MinSubscriptionAmount: 100, DiscountPercent: 80, TotalUses: 1, CreatedBy: 99}
	for _, value := range []float64{0, -1, math.NaN(), math.Inf(1)} {
		input := valid
		input.MinSubscriptionAmount = value
		_, err := admin.IssueSubscriptionDiscountCoupon(context.Background(), user.ID, input)
		require.Error(t, err)
	}
	for _, value := range []float64{0, -1, 100, math.NaN(), math.Inf(1)} {
		input := valid
		input.DiscountPercent = value
		_, err := admin.IssueSubscriptionDiscountCoupon(context.Background(), user.ID, input)
		require.Error(t, err)
	}
	input := valid
	input.TotalUses = 0
	_, err := admin.IssueSubscriptionDiscountCoupon(context.Background(), user.ID, input)
	require.Error(t, err)
	input = valid
	input.CreatedBy = 0
	_, err = admin.IssueSubscriptionDiscountCoupon(context.Background(), user.ID, input)
	require.Error(t, err)
	_, err = admin.IssueSubscriptionDiscountCoupon(context.Background(), user.ID+1, valid)
	require.Error(t, err)
}

func TestSubscriptionCouponUsageFollowsOrderLifecycle(t *testing.T) {
	ctx := context.Background()
	client, user, admin, svc := subscriptionCouponFixture(t)
	_, err := admin.IssueSubscriptionDiscountCoupon(ctx, user.ID, IssueSubscriptionDiscountCouponInput{
		MinSubscriptionAmount: 100, DiscountPercent: 80, TotalUses: 20, CreatedBy: 99,
	})
	require.NoError(t, err)
	plan, err := svc.resolveSubscriptionDiscountCoupon(ctx, user.ID, 100)
	require.NoError(t, err)
	statuses := []string{OrderStatusPending, OrderStatusPaid, OrderStatusRecharging, OrderStatusCompleted,
		OrderStatusRefundRequested, OrderStatusRefunding, OrderStatusPartiallyRefunded,
		OrderStatusRefunded, OrderStatusRefundFailed, OrderStatusCancelled, OrderStatusExpired, OrderStatusFailed}
	for i, status := range statuses {
		_, err := client.PaymentOrder.Create().SetUserID(user.ID).SetUserEmail(user.Email).SetUserName(user.Username).
			SetAmount(80).SetPayAmount(80).SetFeeRate(0).SetRechargeCode(fmt.Sprintf("SUB-%d", i)).
			SetOutTradeNo(fmt.Sprintf("subscription-coupon-%d", i)).SetPaymentType(payment.TypeAlipay).
			SetPaymentTradeNo("").SetOrderType(payment.OrderTypeSubscription).SetStatus(status).
			SetExpiresAt(time.Now().Add(time.Hour)).SetClientIP("127.0.0.1").SetSrcHost("example.com").
			SetProviderSnapshot(appendSubscriptionDiscountSnapshot(nil, plan)).Save(ctx)
		require.NoError(t, err)
	}
	_, err = client.PaymentOrder.Create().SetUserID(user.ID).SetUserEmail(user.Email).SetUserName(user.Username).
		SetAmount(80).SetPayAmount(80).SetFeeRate(0).SetRechargeCode("BALANCE-COUPON").
		SetOutTradeNo("balance-coupon").SetPaymentType(payment.TypeAlipay).SetPaymentTradeNo("").
		SetOrderType(payment.OrderTypeBalance).SetStatus(OrderStatusCompleted).
		SetExpiresAt(time.Now().Add(time.Hour)).SetClientIP("127.0.0.1").SetSrcHost("example.com").
		SetProviderSnapshot(appendSubscriptionDiscountSnapshot(nil, plan)).Save(ctx)
	require.NoError(t, err)
	coupons, err := svc.ListAvailableSubscriptionDiscountCoupons(ctx, user.ID)
	require.NoError(t, err)
	require.Len(t, coupons, 1)
	require.Equal(t, 9, coupons[0].UsedCount)
	require.Equal(t, 11, coupons[0].RemainingUses)
}

func TestSubscriptionCouponTransactionEnforcesLastUseAndCancellation(t *testing.T) {
	ctx := context.Background()
	client, user, admin, svc := subscriptionCouponFixture(t)
	coupon, err := admin.IssueSubscriptionDiscountCoupon(ctx, user.ID, IssueSubscriptionDiscountCouponInput{
		MinSubscriptionAmount: 100, DiscountPercent: 80, TotalUses: 1, CreatedBy: 99,
	})
	require.NoError(t, err)
	plan, err := svc.resolveSubscriptionDiscountCoupon(ctx, user.ID, 100)
	require.NoError(t, err)
	create := func() (*dbent.PaymentOrder, error) {
		return svc.createOrderInTx(ctx, CreateOrderRequest{
			UserID: user.ID, PaymentType: payment.TypeAlipay, OrderType: payment.OrderTypeSubscription,
			ClientIP: "127.0.0.1", SrcHost: "example.com",
		}, &User{ID: user.ID, Email: user.Email, Username: user.Username}, nil,
			&PaymentConfig{MaxPendingOrders: 5, OrderTimeoutMin: 30}, 80, 80, 0, 80, 0, nil, firstRechargeAmountPlan{}, plan)
	}
	order, err := create()
	require.NoError(t, err)
	storedPlan, ok := subscriptionDiscountPlanFromSnapshot(order.ProviderSnapshot)
	require.True(t, ok)
	require.Equal(t, coupon.ID, storedPlan.CouponID)
	_, err = create()
	require.Equal(t, "SUBSCRIPTION_COUPON_LIMIT_REACHED", infraerrors.Reason(err))

	_, err = client.PaymentOrder.UpdateOneID(order.ID).SetStatus(OrderStatusCancelled).Save(ctx)
	require.NoError(t, err)
	order, err = create()
	require.NoError(t, err)
	_, err = client.PaymentOrder.UpdateOneID(order.ID).SetStatus(OrderStatusFailed).Save(ctx)
	require.NoError(t, err)
	_, err = create()
	require.NoError(t, err)
	available, err := svc.ListAvailableSubscriptionDiscountCoupons(ctx, user.ID)
	require.NoError(t, err)
	require.Empty(t, available)
}

func TestSubscriptionCouponReservationChecksOwnershipAndChanges(t *testing.T) {
	ctx := context.Background()
	client, user, admin, svc := subscriptionCouponFixture(t)
	coupon, err := admin.IssueSubscriptionDiscountCoupon(ctx, user.ID, IssueSubscriptionDiscountCouponInput{
		MinSubscriptionAmount: 100, DiscountPercent: 80, TotalUses: 1, CreatedBy: 99,
	})
	require.NoError(t, err)
	plan, err := svc.resolveSubscriptionDiscountCoupon(ctx, user.ID, 100)
	require.NoError(t, err)
	check := func(userID int64) error {
		tx, err := client.Tx(ctx)
		require.NoError(t, err)
		defer tx.Rollback()
		return svc.checkSubscriptionDiscountCouponOrderLimit(ctx, tx, userID, plan)
	}
	require.Equal(t, "SUBSCRIPTION_COUPON_UNAVAILABLE", infraerrors.Reason(check(user.ID+1)))
	for _, update := range []string{"discount_percent = 90", "min_subscription_amount = 200", "status = 'revoked'"} {
		_, err = client.ExecContext(ctx, "UPDATE subscription_discount_coupons SET "+update+" WHERE id = $1", coupon.ID)
		require.NoError(t, err)
		require.Equal(t, "SUBSCRIPTION_COUPON_UNAVAILABLE", infraerrors.Reason(check(user.ID)))
		_, err = client.ExecContext(ctx, "UPDATE subscription_discount_coupons SET discount_percent = 80, min_subscription_amount = 100, status = 'active' WHERE id = $1", coupon.ID)
		require.NoError(t, err)
	}
}
