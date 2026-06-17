//go:build unit

package service

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestBuildFirstRechargeAmountPlan_UsesPaymentPercentage(t *testing.T) {
	plan := buildFirstRechargeAmountPlan(100, 100, &firstRechargePromo{
		PromoCodeID:     9,
		PromoCode:       "PARTNER8",
		BonusAmount:     10,
		DiscountPercent: 80,
		DiscountTimes:   3,
		DiscountSet:     true,
	})

	require.Equal(t, int64(9), plan.PromoCodeID)
	require.Equal(t, "PARTNER8", plan.PromoCode)
	require.Equal(t, 100.0, plan.BaseCreditAmount)
	require.Equal(t, 10.0, plan.BonusAmount)
	require.Equal(t, 80.0, plan.DiscountPercent)
	require.Equal(t, 3, plan.DiscountTimes)
	require.Equal(t, 110.0, plan.CreditAmount)
	require.Equal(t, 80.0, plan.PaymentAmount)
}

func TestBuildFirstRechargeAmountPlan_LowPaymentRateIsActive(t *testing.T) {
	plan := buildFirstRechargeAmountPlan(100, 100, &firstRechargePromo{
		PromoCodeID:     9,
		DiscountPercent: 0.01,
		DiscountSet:     true,
	})

	require.True(t, plan.active())
	require.Equal(t, 100.0, plan.CreditAmount)
	require.Equal(t, 0.01, plan.PaymentAmount)
}

func TestFirstRechargeAmountPlanFromSnapshot_UsesDiscountSetFlag(t *testing.T) {
	plan, ok := firstRechargeAmountPlanFromSnapshot(map[string]any{
		"first_recharge_promo": map[string]any{
			"promo_code_id":    9,
			"promo_code":       "PARTNER8",
			"base_amount":      100,
			"bonus_amount":     10,
			"discount_percent": 0,
			"discount_times":   3,
			"discount_set":     false,
			"credited_amount":  110,
			"payment_amount":   100,
		},
	})

	require.True(t, ok)
	require.False(t, plan.DiscountSet)
	require.Equal(t, int64(9), plan.PromoCodeID)
	require.Equal(t, "PARTNER8", plan.PromoCode)
	require.Equal(t, 10.0, plan.BonusAmount)
	require.Equal(t, 3, plan.DiscountTimes)
	require.Equal(t, 100.0, plan.PaymentAmount)
}

func TestGetFirstRechargePromoPreview_ReturnsAvailablePromo(t *testing.T) {
	ctx := context.Background()
	bonus := 10.0
	discountPercent := 80.0
	promoRepo := &promoCodeRepoStub{
		firstRechargePromo: &PromoCode{
			ID:                           9,
			Code:                         "PARTNER80",
			FirstRechargeBonusAmount:     &bonus,
			FirstRechargeDiscountPercent: &discountPercent,
			FirstRechargeDiscountTimes:   3,
			Status:                       PromoCodeStatusActive,
		},
	}
	svc := NewPaymentService(newOrderNotFoundTestClient(t), nil, nil, nil, nil, nil, &userRepoStub{
		user: &User{ID: 42, Status: StatusActive},
	}, nil, nil, promoRepo)

	preview, err := svc.GetFirstRechargePromoPreview(ctx, 42)

	require.NoError(t, err)
	require.NotNil(t, preview)
	require.Equal(t, "PARTNER80", preview.PromoCode)
	require.Equal(t, 10.0, preview.BonusAmount)
	require.Equal(t, 80.0, preview.DiscountPercent)
	require.Equal(t, 3, preview.DiscountTimes)
	require.Equal(t, 0, preview.DiscountUsed)
	require.Equal(t, 3, preview.DiscountRemaining)
	require.True(t, preview.DiscountSet)
}
