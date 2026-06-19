//go:build unit

package service

import (
	"context"
	"math"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
	"github.com/stretchr/testify/require"
)

func TestValidatePromoFirstRechargeValue_BoundaryAndInvalid(t *testing.T) {
	t.Parallel()

	require.NoError(t, validatePromoFirstRechargeValue(nil, 0, math.MaxFloat64, "INVALID_FIRST_RECHARGE_BONUS", "invalid bonus"))
	for _, v := range []float64{0, 0.01, 10, 999999} {
		v := v
		require.NoError(t, validatePromoFirstRechargeValue(&v, 0, math.MaxFloat64, "INVALID_FIRST_RECHARGE_BONUS", "invalid bonus"))
	}
	for _, v := range []float64{-0.01, math.NaN(), math.Inf(1), math.Inf(-1)} {
		v := v
		require.Error(t, validatePromoFirstRechargeValue(&v, 0, math.MaxFloat64, "INVALID_FIRST_RECHARGE_BONUS", "invalid bonus"))
	}

	for _, v := range []float64{0.01, 50, 100} {
		v := v
		require.NoError(t, validatePromoFirstRechargeValue(&v, 0.01, 100, "INVALID_FIRST_RECHARGE_DISCOUNT", "invalid discount"))
	}
	for _, v := range []float64{0, -0.01, 100.01, math.NaN(), math.Inf(1), math.Inf(-1)} {
		v := v
		require.Error(t, validatePromoFirstRechargeValue(&v, 0.01, 100, "INVALID_FIRST_RECHARGE_DISCOUNT", "invalid discount"))
	}
}

func TestApplyPromoCode_ZeroBonusCreatesUsageWithoutBalanceUpdate(t *testing.T) {
	ctx := context.Background()
	client := newOrderNotFoundTestClient(t)

	discountPercent := 80.0
	promoRepo := &promoCodeRepoStub{
		promo: &PromoCode{
			ID:                           9,
			Code:                         "PARTNER80",
			BonusAmount:                  0,
			FirstRechargeDiscountPercent: &discountPercent,
			FirstRechargeDiscountTimes:   3,
			Status:                       PromoCodeStatusActive,
		},
	}
	svc := NewPromoService(promoRepo, &userRepoStub{}, nil, client, nil)

	err := svc.ApplyPromoCode(ctx, 42, " partner80 ")

	require.NoError(t, err)
	require.NotNil(t, promoRepo.createdUsage)
	require.Equal(t, int64(9), promoRepo.createdUsage.PromoCodeID)
	require.Equal(t, int64(42), promoRepo.createdUsage.UserID)
	require.Zero(t, promoRepo.createdUsage.BonusAmount)
	require.Equal(t, []int64{9}, promoRepo.incrementedIDs)
	require.Equal(t, 1, promoRepo.promo.UsedCount)
}

func TestApplyPromoCode_BonusDoesNotCountAsRecharge(t *testing.T) {
	ctx := context.Background()
	client := newOrderNotFoundTestClient(t)

	user, err := client.User.Create().
		SetEmail("promo-bonus@example.com").
		SetPasswordHash("hash").
		SetUsername("promo-bonus-user").
		Save(ctx)
	require.NoError(t, err)

	promoRepo := &promoCodeRepoStub{
		promo: &PromoCode{
			ID:          9,
			Code:        "WELCOME10",
			BonusAmount: 10,
			Status:      PromoCodeStatusActive,
		},
	}
	svc := NewPromoService(promoRepo, &userRepoStub{}, nil, client, nil)

	err = svc.ApplyPromoCode(ctx, user.ID, "WELCOME10")
	require.NoError(t, err)

	reloaded, err := client.User.Get(ctx, user.ID)
	require.NoError(t, err)
	require.Equal(t, 10.0, reloaded.Balance)
	require.Zero(t, reloaded.TotalRecharged)
	require.Equal(t, []int64{9}, promoRepo.incrementedIDs)
}

type promoCodeRepoStub struct {
	promo              *PromoCode
	firstRechargePromo *PromoCode
	existingUsage      *PromoCodeUsage
	createdUsage       *PromoCodeUsage
	incrementedIDs     []int64
}

func (s *promoCodeRepoStub) Create(context.Context, *PromoCode) error {
	panic("unexpected Create call")
}

func (s *promoCodeRepoStub) GetByID(context.Context, int64) (*PromoCode, error) {
	panic("unexpected GetByID call")
}

func (s *promoCodeRepoStub) GetByCode(_ context.Context, code string) (*PromoCode, error) {
	if s.promo == nil || !strings.EqualFold(s.promo.Code, strings.TrimSpace(code)) {
		return nil, ErrPromoCodeNotFound
	}
	return s.promo, nil
}

func (s *promoCodeRepoStub) GetByCodeForUpdate(_ context.Context, code string) (*PromoCode, error) {
	if s.promo == nil || !strings.EqualFold(s.promo.Code, strings.TrimSpace(code)) {
		return nil, ErrPromoCodeNotFound
	}
	return s.promo, nil
}

func (s *promoCodeRepoStub) Update(context.Context, *PromoCode) error {
	panic("unexpected Update call")
}

func (s *promoCodeRepoStub) Delete(context.Context, int64) error {
	panic("unexpected Delete call")
}

func (s *promoCodeRepoStub) List(context.Context, pagination.PaginationParams) ([]PromoCode, *pagination.PaginationResult, error) {
	panic("unexpected List call")
}

func (s *promoCodeRepoStub) ListWithFilters(context.Context, pagination.PaginationParams, string, string) ([]PromoCode, *pagination.PaginationResult, error) {
	panic("unexpected ListWithFilters call")
}

func (s *promoCodeRepoStub) CreateUsage(_ context.Context, usage *PromoCodeUsage) error {
	clone := *usage
	clone.ID = 100
	s.createdUsage = &clone
	return nil
}

func (s *promoCodeRepoStub) GetUsageByPromoCodeAndUser(context.Context, int64, int64) (*PromoCodeUsage, error) {
	return s.existingUsage, nil
}

func (s *promoCodeRepoStub) GetFirstRechargePromoByUser(context.Context, int64) (*PromoCode, error) {
	return s.firstRechargePromo, nil
}

func (s *promoCodeRepoStub) ListUsagesByUser(context.Context, int64) ([]PromoCodeUsage, error) {
	panic("unexpected ListUsagesByUser call")
}

func (s *promoCodeRepoStub) ListUsagesByPromoCode(context.Context, int64, pagination.PaginationParams) ([]PromoCodeUsage, *pagination.PaginationResult, error) {
	panic("unexpected ListUsagesByPromoCode call")
}

func (s *promoCodeRepoStub) ListRechargeStatsByPromoCodeIDs(context.Context, []int64) (map[int64]PromoCodeRechargeStats, error) {
	panic("unexpected ListRechargeStatsByPromoCodeIDs call")
}

func (s *promoCodeRepoStub) IncrementUsedCount(_ context.Context, id int64) error {
	s.incrementedIDs = append(s.incrementedIDs, id)
	if s.promo != nil && s.promo.ID == id {
		s.promo.UsedCount++
	}
	return nil
}
