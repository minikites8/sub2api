package service

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestRechargeTiersComposeWithFirstRechargeAndCoupons(t *testing.T) {
	promo := &firstRechargePromo{PromoCodeID: 1, BonusAmount: 5, DiscountSet: true, DiscountPercent: 90}
	coupon := &RechargeDiscountCouponPreview{ID: 2, DiscountPercent: 80}
	for _, tc := range []struct {
		name, mode             string
		quote                  rechargeBonusQuote
		credit, payment, bonus float64
	}{
		{"global bonus", RechargeBonusModeBonus, rechargeBonusQuote{PayBase: 100, Credited: 110, Bonus: 10}, 115, 80, 10},
		{"global discount", RechargeBonusModeDiscount, rechargeBonusQuote{PayBase: 90, Credited: 100, Bonus: 10}, 105, 72, 10},
		{"legacy multiplier with coupon", RechargeBonusModeBonus, rechargeBonusQuote{PayBase: 100, Credited: 200}, 105, 80, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			plan, bonus := composeRechargeAmountPlan(100, tc.quote, tc.mode, promo, coupon)
			require.Equal(t, tc.credit, plan.CreditAmount)
			require.Equal(t, tc.payment, plan.PaymentAmount)
			require.Equal(t, tc.bonus, bonus)
			require.Equal(t, int64(2), plan.CouponID)
			require.Equal(t, 5.0, plan.BonusAmount)
		})
	}
}

func TestRechargeTiersRetainMultiplierWithoutCoupons(t *testing.T) {
	promo := &firstRechargePromo{PromoCodeID: 1, BonusAmount: 5}
	plan, bonus := composeRechargeAmountPlan(100, rechargeBonusQuote{PayBase: 100, Credited: 220, Bonus: 20}, RechargeBonusModeBonus, promo, nil)
	require.Equal(t, 225.0, plan.CreditAmount)
	require.Equal(t, 100.0, plan.PaymentAmount)
	require.Equal(t, 20.0, bonus)
}
