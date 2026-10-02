//go:build unit

package service

import (
	"context"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
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

func TestResolveSubscriptionPromoCodeCalculatesPayableAmount(t *testing.T) {
	discount := 80.0
	repo := &subscriptionPromoRepoStub{promo: &PromoCode{
		ID:                          17,
		Code:                        "SUB80",
		CouponType:                  PromoCodeTypeSubscription,
		SubscriptionDiscountPercent: &discount,
		Status:                      PromoCodeStatusActive,
	}}
	svc := &PaymentService{promoRepo: repo}

	resolved, err := svc.resolveSubscriptionPromoCode(context.Background(), 42, " sub80 ", 100)

	require.NoError(t, err)
	require.Equal(t, int64(17), resolved.PromoCodeID)
	require.Equal(t, "SUB80", resolved.PromoCode)
	require.Equal(t, 80.0, resolved.DiscountPercent)
	require.Equal(t, 100.0, resolved.OriginalAmount)
	require.Equal(t, 20.0, resolved.DiscountAmount)
	require.Equal(t, 80.0, resolved.DiscountedAmount)
}

func TestResolveSubscriptionPromoCodeRejectsWrongTypeAndReuse(t *testing.T) {
	registrationDiscount := 80.0
	registrationRepo := &subscriptionPromoRepoStub{promo: &PromoCode{
		ID:                          18,
		Code:                        "REG80",
		CouponType:                  PromoCodeTypeRegistration,
		SubscriptionDiscountPercent: &registrationDiscount,
		Status:                      PromoCodeStatusActive,
	}}
	_, err := (&PaymentService{promoRepo: registrationRepo}).resolveSubscriptionPromoCode(context.Background(), 42, "REG80", 100)
	require.ErrorIs(t, err, ErrPromoCodeWrongType)

	subscriptionDiscount := 80.0
	subscriptionRepo := &subscriptionPromoRepoStub{promo: &PromoCode{
		ID:                          19,
		Code:                        "SUB80",
		CouponType:                  PromoCodeTypeSubscription,
		SubscriptionDiscountPercent: &subscriptionDiscount,
		Status:                      PromoCodeStatusActive,
	}, existingUsage: &PromoCodeUsage{ID: 99, PromoCodeID: 19, UserID: 42}}
	_, err = (&PaymentService{promoRepo: subscriptionRepo}).resolveSubscriptionPromoCode(context.Background(), 42, "SUB80", 100)
	require.ErrorIs(t, err, ErrPromoCodeAlreadyUsed)
}

func TestSubscriptionPromoSnapshotRoundTrip(t *testing.T) {
	discount := 80.0
	promo := &subscriptionPromoPlan{
		PromoCodeID:      17,
		PromoCode:        "SUB80",
		DiscountPercent:  discount,
		OriginalAmount:   100,
		DiscountAmount:   20,
		DiscountedAmount: 80,
	}

	snapshot := appendSubscriptionPromoSnapshot(nil, promo)
	restored, ok := subscriptionPromoPlanFromSnapshot(snapshot)

	require.True(t, ok)
	require.Equal(t, promo, restored)
}

type subscriptionPromoRepoStub struct {
	promo         *PromoCode
	existingUsage *PromoCodeUsage
}

func (s *subscriptionPromoRepoStub) Create(context.Context, *PromoCode) error {
	panic("unexpected Create call")
}
func (s *subscriptionPromoRepoStub) GetByID(context.Context, int64) (*PromoCode, error) {
	panic("unexpected GetByID call")
}
func (s *subscriptionPromoRepoStub) GetByCode(_ context.Context, code string) (*PromoCode, error) {
	if s.promo == nil || !strings.EqualFold(s.promo.Code, strings.TrimSpace(code)) {
		return nil, ErrPromoCodeNotFound
	}
	return s.promo, nil
}
func (s *subscriptionPromoRepoStub) GetByCodeForUpdate(context.Context, string) (*PromoCode, error) {
	panic("unexpected GetByCodeForUpdate call")
}
func (s *subscriptionPromoRepoStub) Update(context.Context, *PromoCode) error {
	panic("unexpected Update call")
}
func (s *subscriptionPromoRepoStub) Delete(context.Context, int64) error {
	panic("unexpected Delete call")
}
func (s *subscriptionPromoRepoStub) List(context.Context, pagination.PaginationParams) ([]PromoCode, *pagination.PaginationResult, error) {
	panic("unexpected List call")
}
func (s *subscriptionPromoRepoStub) ListWithFilters(context.Context, pagination.PaginationParams, string, string) ([]PromoCode, *pagination.PaginationResult, error) {
	panic("unexpected ListWithFilters call")
}
func (s *subscriptionPromoRepoStub) CreateUsage(context.Context, *PromoCodeUsage) error {
	panic("unexpected CreateUsage call")
}
func (s *subscriptionPromoRepoStub) GetUsageByPromoCodeAndUser(context.Context, int64, int64) (*PromoCodeUsage, error) {
	return s.existingUsage, nil
}
func (s *subscriptionPromoRepoStub) GetFirstRechargePromoByUser(context.Context, int64) (*PromoCode, error) {
	panic("unexpected GetFirstRechargePromoByUser call")
}
func (s *subscriptionPromoRepoStub) ListUsagesByUser(context.Context, int64) ([]PromoCodeUsage, error) {
	panic("unexpected ListUsagesByUser call")
}
func (s *subscriptionPromoRepoStub) ListUsagesByPromoCode(context.Context, int64, pagination.PaginationParams) ([]PromoCodeUsage, *pagination.PaginationResult, error) {
	panic("unexpected ListUsagesByPromoCode call")
}
func (s *subscriptionPromoRepoStub) ListRechargeStatsByPromoCodeIDs(context.Context, []int64) (map[int64]PromoCodeRechargeStats, error) {
	panic("unexpected ListRechargeStatsByPromoCodeIDs call")
}
func (s *subscriptionPromoRepoStub) IncrementUsedCount(context.Context, int64) error {
	panic("unexpected IncrementUsedCount call")
}
func (s *subscriptionPromoRepoStub) ReleaseSubscriptionPromoCode(context.Context, int64, int64) error {
	panic("unexpected ReleaseSubscriptionPromoCode call")
}

func testFloat64Ptr(value float64) *float64 {
	return &value
}
