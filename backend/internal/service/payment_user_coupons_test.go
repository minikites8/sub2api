//go:build unit

package service

import (
	"context"
	"encoding/json"
	"testing"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/internal/payment"
	"github.com/stretchr/testify/require"
)

func TestListMyDiscountCouponsOwnershipAndUsage(t *testing.T) {
	ctx := context.Background()
	client, user, admin, svc := subscriptionCouponFixture(t)
	createRechargeCouponTestTable(t, client)
	recharge, err := admin.IssueRechargeDiscountCoupon(ctx, user.ID, IssueRechargeDiscountCouponInput{
		MinRechargeAmount: 100, DiscountPercent: 80, TotalUses: 2, CreatedBy: 99, Notes: "internal recharge note",
	})
	require.NoError(t, err)
	subscription, err := admin.IssueSubscriptionDiscountCoupon(ctx, user.ID, IssueSubscriptionDiscountCouponInput{
		MinSubscriptionAmount: 20, DiscountPercent: 90, TotalUses: 1, CreatedBy: 99, Notes: "internal subscription note",
	})
	require.NoError(t, err)
	inactive, err := admin.IssueRechargeDiscountCoupon(ctx, user.ID, IssueRechargeDiscountCouponInput{
		MinRechargeAmount: 200, DiscountPercent: 70, TotalUses: 1, CreatedBy: 99,
	})
	require.NoError(t, err)
	_, err = client.ExecContext(ctx, "UPDATE recharge_discount_coupons SET status = 'revoked' WHERE id = $1", inactive.ID)
	require.NoError(t, err)
	unlimited, err := admin.IssueRechargeDiscountCoupon(ctx, user.ID, IssueRechargeDiscountCouponInput{
		MinRechargeAmount: 1, DiscountPercent: 95, TotalUses: 1, CreatedBy: 99,
	})
	require.NoError(t, err)
	_, err = client.ExecContext(ctx, "UPDATE recharge_discount_coupons SET total_uses = 0 WHERE id = $1", unlimited.ID)
	require.NoError(t, err)
	other, err := client.User.Create().SetEmail("other-coupon@example.com").SetPasswordHash("hash").Save(ctx)
	require.NoError(t, err)
	_, err = admin.IssueRechargeDiscountCoupon(ctx, other.ID, IssueRechargeDiscountCouponInput{
		MinRechargeAmount: 50, DiscountPercent: 50, TotalUses: 1, CreatedBy: 99,
	})
	require.NoError(t, err)
	_, err = admin.IssueSubscriptionDiscountCoupon(ctx, other.ID, IssueSubscriptionDiscountCouponInput{
		MinSubscriptionAmount: 50, DiscountPercent: 50, TotalUses: 1, CreatedBy: 99,
	})
	require.NoError(t, err)
	quote, err := svc.resolveSubscriptionDiscountCoupon(ctx, user.ID, 25)
	require.NoError(t, err)
	order, err := svc.createOrderInTx(ctx, CreateOrderRequest{
		UserID: user.ID, OrderType: payment.OrderTypeSubscription, PaymentType: payment.TypeAlipay,
	}, &User{ID: user.ID, Email: user.Email}, &dbent.SubscriptionPlan{ID: 1, GroupID: 7, Price: 25, ValidityDays: 1},
		&PaymentConfig{}, 22.5, 22.5, 0, 22.5, 0, nil, firstRechargeAmountPlan{}, quote)
	require.NoError(t, err)
	items, err := svc.ListMyDiscountCoupons(ctx, user.ID)
	require.NoError(t, err)
	require.Len(t, items, 4)
	byTypeID := make(map[string]map[int64]UserDiscountCoupon)
	for _, item := range items {
		if byTypeID[item.CouponType] == nil {
			byTypeID[item.CouponType] = make(map[int64]UserDiscountCoupon)
		}
		byTypeID[item.CouponType][item.ID] = item
		require.NotEqual(t, 50.0, item.DiscountPercent)
	}
	require.Equal(t, 2, byTypeID["recharge"][recharge.ID].RemainingUses)
	require.Equal(t, "available", byTypeID["recharge"][unlimited.ID].Status)
	require.Equal(t, 0, byTypeID["recharge"][unlimited.ID].TotalUses)
	require.Equal(t, "inactive", byTypeID["recharge"][inactive.ID].Status)
	require.Equal(t, "exhausted", byTypeID["subscription"][subscription.ID].Status)
	require.Equal(t, 1, byTypeID["subscription"][subscription.ID].UsedCount)
	require.Equal(t, 0, byTypeID["subscription"][subscription.ID].RemainingUses)
	require.Equal(t, "available", items[0].Status)
	raw, err := json.Marshal(items)
	require.NoError(t, err)
	for _, field := range []string{"user_id", "notes", "created_by", "source_code"} {
		require.NotContains(t, string(raw), field)
	}
	_, err = client.PaymentOrder.UpdateOneID(order.ID).SetStatus(OrderStatusCancelled).Save(ctx)
	require.NoError(t, err)
	items, err = svc.ListMyDiscountCoupons(ctx, user.ID)
	require.NoError(t, err)
	for _, item := range items {
		if item.CouponType == "subscription" {
			require.Equal(t, "available", item.Status)
			require.Equal(t, 1, item.RemainingUses)
		}
	}
}

func TestListMyDiscountCouponsHandlesMissingTables(t *testing.T) {
	ctx := context.Background()
	client := newPaymentConfigServiceTestClient(t)
	svc := &PaymentService{entClient: client}
	coupons, err := svc.ListMyDiscountCoupons(ctx, 42)
	require.NoError(t, err)
	require.NotNil(t, coupons)
	require.Empty(t, coupons)
}
