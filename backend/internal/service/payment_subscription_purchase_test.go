//go:build unit

package service

import (
	"context"
	"math"
	"net/url"
	"strconv"
	"testing"
	"time"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/internal/payment"
	"github.com/stretchr/testify/require"
)

type subscriptionPurchaseRenewalRepo struct {
	*subscriptionUserSubRepoStub
}

type subscriptionPurchaseUserRepo struct {
	UserRepository
	user *User
}

func (r *subscriptionPurchaseUserRepo) GetByID(context.Context, int64) (*User, error) {
	return r.user, nil
}

type subscriptionPurchaseLoadBalancer struct {
	captureLoadBalancer
	amount float64
}

func (b *subscriptionPurchaseLoadBalancer) SelectInstance(_ context.Context, _ string, _ payment.PaymentType, _ payment.Strategy, amount float64) (*payment.InstanceSelection, error) {
	b.amount = amount
	return &payment.InstanceSelection{
		InstanceID: "1", ProviderKey: payment.TypeEasyPay, PaymentMode: "popup", SupportedTypes: "alipay",
		Config: map[string]string{"pid": "1", "pkey": "test-key", "apiBase": "https://pay.example.com",
			"notifyUrl": "https://app.example.com/notify", "returnUrl": "https://app.example.com/purchase", "paymentMode": "popup"},
	}, nil
}

func TestSubscriptionPurchaseCreateOrderUsesServerPrice(t *testing.T) {
	for _, quantity := range []int{0, 3, 5} {
		t.Run(strconv.Itoa(quantity), func(t *testing.T) {
			ctx := context.Background()
			client, user, _, svc := subscriptionCouponFixture(t)
			plan, err := client.SubscriptionPlan.Create().SetGroupID(7).SetName("Day card").
				SetPrice(9.9).SetValidityDays(1).SetValidityUnit("day").Save(ctx)
			require.NoError(t, err)
			svc.configService = &PaymentConfigService{entClient: client, settingRepo: &paymentConfigSettingRepoStub{
				values: map[string]string{SettingPaymentEnabled: "true"},
			}}
			svc.userRepo = &subscriptionPurchaseUserRepo{user: &User{ID: user.ID, Email: user.Email, Status: payment.EntityStatusActive}}
			svc.groupRepo = &subscriptionGroupRepoStub{group: &Group{ID: 7, Status: payment.EntityStatusActive, SubscriptionType: SubscriptionTypeSubscription}}
			lb := &subscriptionPurchaseLoadBalancer{}
			svc.loadBalancer = lb
			result, err := svc.CreateOrder(ctx, CreateOrderRequest{
				UserID: user.ID, PlanID: plan.ID, Quantity: quantity, Amount: 0.01,
				OrderType: payment.OrderTypeSubscription, PaymentType: payment.TypeAlipay,
			})
			require.NoError(t, err)
			wantQuantity := quantity
			if wantQuantity == 0 {
				wantQuantity = 1
			}
			quote, err := quoteSubscriptionPurchase(plan, wantQuantity)
			require.NoError(t, err)
			require.Equal(t, quote.Amount, result.Amount)
			require.Equal(t, quote.Amount, result.PayAmount)
			require.Equal(t, quote.Amount, lb.amount)
			require.Equal(t, wantQuantity, result.Quantity)
			providerURL, err := url.Parse(result.PayURL)
			require.NoError(t, err)
			require.Equal(t, payment.FormatAmountForCurrency(quote.Amount, "CNY"), providerURL.Query().Get("money"))
			order, err := client.PaymentOrder.Get(ctx, result.OrderID)
			require.NoError(t, err)
			require.Equal(t, wantQuantity, *order.SubscriptionDays)
		})
	}
}

func (r *subscriptionPurchaseRenewalRepo) ExtendExpiry(ctx context.Context, id int64, expiresAt time.Time) error {
	sub, err := r.GetByID(ctx, id)
	if err != nil {
		return err
	}
	sub.ExpiresAt = expiresAt
	return r.Update(ctx, sub)
}

func (r *subscriptionPurchaseRenewalRepo) UpdateNotes(ctx context.Context, id int64, notes string) error {
	sub, err := r.GetByID(ctx, id)
	if err != nil {
		return err
	}
	sub.Notes = notes
	return r.Update(ctx, sub)
}

func TestQuoteSubscriptionPurchase(t *testing.T) {
	for _, tc := range []struct {
		name           string
		quantity, days int
		unit           string
		wantDays       int
		wantAmount     float64
	}{
		{"legacy request", 0, 1, "day", 1, 9.9},
		{"three day cards", 3, 1, "days", 3, 29.7},
		{"five day cards", 5, 1, "day", 5, 49.5},
		{"weeks", 3, 2, "weeks", 42, 29.7},
		{"months", 5, 1, "months", 150, 49.5},
	} {
		t.Run(tc.name, func(t *testing.T) {
			quote, err := quoteSubscriptionPurchase(&dbent.SubscriptionPlan{Price: 9.9, ValidityDays: tc.days, ValidityUnit: tc.unit}, tc.quantity)
			require.NoError(t, err)
			require.Equal(t, tc.wantDays, quote.Days)
			require.Equal(t, tc.wantAmount, quote.Amount)
		})
	}
}

func TestQuoteSubscriptionPurchaseValidatesQuantityAndDuration(t *testing.T) {
	for _, tc := range []struct {
		quantity, days int
		unit           string
	}{
		{-1, 1, "day"}, {1001, 1, "day"}, {2, 36500, "day"},
		{1000, 2, "months"}, {1, math.MaxInt, "weeks"}, {1, 0, "day"},
	} {
		_, err := quoteSubscriptionPurchase(&dbent.SubscriptionPlan{Price: 9.9, ValidityDays: tc.days, ValidityUnit: tc.unit}, tc.quantity)
		require.Error(t, err)
	}
	for _, price := range []float64{0, -1, math.NaN(), math.Inf(1), math.MaxFloat64} {
		_, err := quoteSubscriptionPurchase(&dbent.SubscriptionPlan{Price: price, ValidityDays: 1}, 5)
		require.Error(t, err)
	}
}

func TestSubscriptionPurchaseCombinedCheckoutAndRenewal(t *testing.T) {
	ctx := context.Background()
	client, user, admin, svc := subscriptionCouponFixture(t)
	ensurePaymentAuditOrderActionUniqueIndex(t, ctx, client)
	_, err := admin.IssueSubscriptionDiscountCoupon(ctx, user.ID, IssueSubscriptionDiscountCouponInput{
		MinSubscriptionAmount: 20, DiscountPercent: 80, TotalUses: 2, CreatedBy: 99,
	})
	require.NoError(t, err)
	plan := &dbent.SubscriptionPlan{ID: 100, GroupID: 7, Price: 9.9, ValidityDays: 1, ValidityUnit: "days"}
	quote, err := quoteSubscriptionPurchase(plan, 3)
	require.NoError(t, err)
	coupon, err := svc.resolveSubscriptionDiscountCoupon(ctx, user.ID, quote.Amount)
	require.NoError(t, err)
	require.NotNil(t, coupon)
	require.Equal(t, 23.76, coupon.DiscountedAmount)
	order, err := svc.createOrderInTx(ctx, CreateOrderRequest{
		UserID: user.ID, PaymentType: payment.TypeAlipay, OrderType: payment.OrderTypeSubscription,
		PlanID: plan.ID, Quantity: 3,
	}, &User{ID: user.ID, Email: user.Email, Username: user.Username}, plan,
		&PaymentConfig{MaxPendingOrders: 3}, coupon.DiscountedAmount, coupon.DiscountedAmount,
		0, coupon.DiscountedAmount, 0, nil, firstRechargeAmountPlan{}, coupon)
	require.NoError(t, err)
	order, err = client.PaymentOrder.Get(ctx, order.ID)
	require.NoError(t, err)
	require.Equal(t, 3, *order.SubscriptionDays)
	require.Equal(t, 3, PaymentOrderQuantity(order))
	require.Equal(t, 23.76, order.PayAmount)
	count, err := client.PaymentOrder.Query().Count(ctx)
	require.NoError(t, err)
	require.Equal(t, 1, count)

	expiresAt := time.Now().Add(10 * 24 * time.Hour).Truncate(time.Second)
	subRepo := &subscriptionPurchaseRenewalRepo{newSubscriptionUserSubRepoStub()}
	subRepo.seed(&UserSubscription{ID: 99, UserID: user.ID, GroupID: 7, StartsAt: time.Now().Add(-time.Hour),
		ExpiresAt: expiresAt, Status: SubscriptionStatusActive})
	groupRepo := &subscriptionGroupRepoStub{group: &Group{ID: 7, Status: payment.EntityStatusActive, SubscriptionType: SubscriptionTypeSubscription}}
	svc.groupRepo = groupRepo
	svc.subscriptionSvc = NewSubscriptionService(groupRepo, subRepo, nil, nil, nil)
	_, err = client.PaymentOrder.UpdateOneID(order.ID).SetStatus(OrderStatusPaid).Save(ctx)
	require.NoError(t, err)
	require.NoError(t, svc.ExecuteSubscriptionFulfillment(ctx, order.ID))
	require.NoError(t, svc.ExecuteSubscriptionFulfillment(ctx, order.ID))
	assertPaymentSubscriptionExpiry(t, subRepo.subscriptionUserSubRepoStub, order, expiresAt.AddDate(0, 0, 3))
	coupons, err := svc.ListAvailableSubscriptionDiscountCoupons(ctx, user.ID)
	require.NoError(t, err)
	require.Len(t, coupons, 1)
	require.Equal(t, 1, coupons[0].RemainingUses)
}

func TestSubscriptionPurchaseOAuthQuantity(t *testing.T) {
	raw, err := buildWeChatPaymentOAuthStartURL(CreateOrderRequest{
		PaymentType: payment.TypeWxpay, OrderType: payment.OrderTypeSubscription, PlanID: 7, Quantity: 5,
	}, "snsapi_base")
	require.NoError(t, err)
	parsed, err := url.Parse(raw)
	require.NoError(t, err)
	require.Equal(t, "5", parsed.Query().Get("quantity"))
	require.Equal(t, 1, PaymentOrderQuantity(&dbent.PaymentOrder{OrderType: payment.OrderTypeSubscription}))
}
