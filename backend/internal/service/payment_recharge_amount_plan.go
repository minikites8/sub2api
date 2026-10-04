package service

// composeRechargeAmountPlan preserves coupon credit semantics while applying
// the global tier to the charge and recording its free balance separately.
func composeRechargeAmountPlan(amount float64, quote rechargeBonusQuote, mode string, promo *firstRechargePromo, coupon *RechargeDiscountCouponPreview) (firstRechargeAmountPlan, float64) {
	plan := buildFirstRechargeAmountPlan(amount, quote.Credited, promo)
	plan = applyRechargeDiscountCoupon(amount, plan, coupon)
	bonus := quote.Bonus
	if plan.discountApplied() && amount > 0 {
		if mode == RechargeBonusModeDiscount {
			bonus = roundTo(plan.BaseCreditAmount*(1-quote.PayBase/amount), 2)
		} else {
			plan.BaseCreditAmount = roundTo(plan.BaseCreditAmount+bonus, 8)
			plan.CreditAmount = roundTo(plan.CreditAmount+bonus, 8)
		}
		plan.PaymentAmount = roundTo(plan.PaymentAmount*quote.PayBase/amount, 8)
	} else {
		plan.PaymentAmount = quote.PayBase
	}
	return plan, bonus
}
