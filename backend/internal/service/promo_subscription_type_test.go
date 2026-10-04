//go:build unit

package service

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

type registrationPromoCreateRepository struct {
	PromoCodeRepository
	created *PromoCode
}

func (r *registrationPromoCreateRepository) Create(_ context.Context, value *PromoCode) error {
	r.created = value
	return nil
}

func TestCreatePromoCodeRegistrationWithRechargeBenefits(t *testing.T) {
	repo := &registrationPromoCreateRepository{}
	svc := &PromoService{promoRepo: repo}
	bonus, discount := 10.0, 80.0
	created, err := svc.Create(context.Background(), &CreatePromoCodeInput{
		Code: "welcome", BonusAmount: 5, FirstRechargeBonusAmount: &bonus, FirstRechargeDiscountPercent: &discount,
	})
	require.NoError(t, err)
	require.Same(t, created, repo.created)
	require.Equal(t, "WELCOME", created.Code)
	require.Equal(t, PromoCodeTypeRegistration, created.CouponType)
	require.Equal(t, 5.0, created.BonusAmount)
	require.Equal(t, bonus, *created.FirstRechargeBonusAmount)
	require.Equal(t, discount, *created.FirstRechargeDiscountPercent)
	require.Nil(t, created.SubscriptionDiscountPercent)
}

func TestPromoCodeWritesRequireRegistrationType(t *testing.T) {
	repo := &registrationPromoCreateRepository{}
	svc := &PromoService{promoRepo: repo}
	discount := 80.0
	_, err := svc.Create(context.Background(), &CreatePromoCodeInput{
		Code: "subscription", CouponType: PromoCodeTypeSubscription, SubscriptionDiscountPercent: &discount,
	})
	require.ErrorIs(t, err, ErrPromoCodeWrongType)
	require.Nil(t, repo.created)
	typeValue := PromoCodeTypeSubscription
	_, err = svc.Update(context.Background(), 7, &UpdatePromoCodeInput{CouponType: &typeValue})
	require.ErrorIs(t, err, ErrPromoCodeWrongType)
}
