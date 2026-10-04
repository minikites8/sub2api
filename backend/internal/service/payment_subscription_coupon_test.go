//go:build unit

package service

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestValidateSubscriptionDiscountPercentRequiresValue(t *testing.T) {
	t.Parallel()

	require.Error(t, validateSubscriptionDiscountPercent(nil))
	for _, value := range []float64{0.01, 80, 100} {
		value := value
		require.NoError(t, validateSubscriptionDiscountPercent(&value))
	}
	for _, value := range []float64{0, -0.01, 100.01} {
		value := value
		require.Error(t, validateSubscriptionDiscountPercent(&value))
	}
}

func TestValidatePromoCodeConfigurationSeparatesCouponTypes(t *testing.T) {
	subscriptionDiscount := 80.0
	subscription := &PromoCode{
		CouponType:                   PromoCodeTypeSubscription,
		BonusAmount:                  10,
		FirstRechargeBonusAmount:     testFloat64Ptr(5),
		FirstRechargeDiscountPercent: testFloat64Ptr(90),
		FirstRechargeDiscountTimes:   4,
		SubscriptionDiscountPercent:  &subscriptionDiscount,
	}
	require.NoError(t, validatePromoCodeConfiguration(subscription))
	require.Equal(t, PromoCodeTypeSubscription, subscription.CouponType)
	require.Zero(t, subscription.BonusAmount)
	require.Nil(t, subscription.FirstRechargeBonusAmount)
	require.Nil(t, subscription.FirstRechargeDiscountPercent)
	require.Equal(t, PromoRechargeDiscountTimesDefault, subscription.FirstRechargeDiscountTimes)
	require.Equal(t, subscriptionDiscount, *subscription.SubscriptionDiscountPercent)

	registrationDiscount := 80.0
	registration := &PromoCode{
		CouponType:                  PromoCodeTypeRegistration,
		SubscriptionDiscountPercent: &registrationDiscount,
	}
	require.NoError(t, validatePromoCodeConfiguration(registration))
	require.Nil(t, registration.SubscriptionDiscountPercent)
}

func TestValidatePromoCodeConfigurationRejectsSubscriptionWithoutDiscount(t *testing.T) {
	require.Error(t, validatePromoCodeConfiguration(&PromoCode{CouponType: PromoCodeTypeSubscription}))
}

func TestSubscriptionPromoSnapshotPreservesHistoricalDiscount(t *testing.T) {
	discount := 80.0
	promo := &subscriptionPromoPlan{
		PromoCodeID:      17,
		PromoCode:        "SUB80",
		DiscountPercent:  discount,
		OriginalAmount:   100,
		DiscountAmount:   20,
		DiscountedAmount: 80,
	}

	snapshot := map[string]any{"subscription_promo_code": map[string]any{
		"promo_code_id": 17, "promo_code": "SUB80", "discount_percent": 80,
		"original_amount": 100, "discount_amount": 20, "discounted_amount": 80,
	}}
	restored, ok := subscriptionPromoPlanFromSnapshot(snapshot)

	require.True(t, ok)
	require.Equal(t, promo, restored)
}

func testFloat64Ptr(value float64) *float64 { return &value }
