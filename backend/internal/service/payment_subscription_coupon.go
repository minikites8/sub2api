package service

import (
	"context"
	"fmt"
	"log/slog"
	"math"
	"strings"
	"time"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/internal/payment"
	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
)

type SubscriptionPromoCodePreview struct {
	PromoCode        string  `json:"promo_code"`
	DiscountPercent  float64 `json:"discount_percent"`
	OriginalAmount   float64 `json:"original_amount"`
	DiscountAmount   float64 `json:"discount_amount"`
	DiscountedAmount float64 `json:"discounted_amount"`
}

type subscriptionPromoPlan struct {
	PromoCodeID      int64
	PromoCode        string
	DiscountPercent  float64
	OriginalAmount   float64
	DiscountAmount   float64
	DiscountedAmount float64
}

func (p *subscriptionPromoPlan) preview() *SubscriptionPromoCodePreview {
	if p == nil {
		return nil
	}
	return &SubscriptionPromoCodePreview{
		PromoCode:        p.PromoCode,
		DiscountPercent:  p.DiscountPercent,
		OriginalAmount:   p.OriginalAmount,
		DiscountAmount:   p.DiscountAmount,
		DiscountedAmount: p.DiscountedAmount,
	}
}

func (s *PaymentService) resolveSubscriptionPromoCode(ctx context.Context, userID int64, code string, originalAmount float64) (*subscriptionPromoPlan, error) {
	code = strings.TrimSpace(code)
	if code == "" {
		return nil, nil
	}
	if s == nil || s.promoRepo == nil {
		return nil, infraerrors.ServiceUnavailable("PROMO_CODE_UNAVAILABLE", "promo code service is unavailable")
	}
	promoCode, err := s.promoRepo.GetByCode(ctx, code)
	if err != nil {
		return nil, err
	}
	if err := validatePromoCodeStatus(promoCode); err != nil {
		return nil, err
	}
	if normalizePromoCodeType(promoCode.CouponType) != PromoCodeTypeSubscription {
		return nil, ErrPromoCodeWrongType
	}
	if promoCode.SubscriptionDiscountPercent == nil {
		return nil, infraerrors.BadRequest("INVALID_SUBSCRIPTION_DISCOUNT", "subscription promo code has no discount")
	}
	if usage, err := s.promoRepo.GetUsageByPromoCodeAndUser(ctx, promoCode.ID, userID); err != nil {
		return nil, fmt.Errorf("check subscription promo usage: %w", err)
	} else if usage != nil {
		return nil, ErrPromoCodeAlreadyUsed
	}
	discountPercent := *promoCode.SubscriptionDiscountPercent
	if err := validateSubscriptionDiscountPercent(&discountPercent); err != nil {
		return nil, err
	}
	originalAmount = roundTo(originalAmount, 8)
	discountedAmount := roundTo(originalAmount*(discountPercent/100), 8)
	return &subscriptionPromoPlan{
		PromoCodeID:      promoCode.ID,
		PromoCode:        strings.ToUpper(strings.TrimSpace(promoCode.Code)),
		DiscountPercent:  roundTo(discountPercent, 8),
		OriginalAmount:   originalAmount,
		DiscountAmount:   roundTo(math.Max(0, originalAmount-discountedAmount), 8),
		DiscountedAmount: discountedAmount,
	}, nil
}

func (s *PaymentService) PreviewSubscriptionPromoCode(ctx context.Context, userID, planID int64, code string) (*SubscriptionPromoCodePreview, error) {
	if planID <= 0 {
		return nil, infraerrors.BadRequest("INVALID_INPUT", "subscription order requires a plan")
	}
	plan, err := s.validateSubOrder(ctx, CreateOrderRequest{OrderType: payment.OrderTypeSubscription, PlanID: planID})
	if err != nil {
		return nil, err
	}
	resolved, err := s.resolveSubscriptionPromoCode(ctx, userID, code, plan.Price)
	if err != nil {
		return nil, err
	}
	if resolved == nil {
		return nil, infraerrors.BadRequest("PROMO_CODE_INVALID", "subscription promo code is required")
	}
	return resolved.preview(), nil
}

func appendSubscriptionPromoSnapshot(snapshot map[string]any, promo *subscriptionPromoPlan) map[string]any {
	if promo == nil || promo.PromoCodeID <= 0 {
		return snapshot
	}
	if snapshot == nil {
		snapshot = map[string]any{}
	}
	snapshot["subscription_promo_code"] = map[string]any{
		"promo_code_id":     promo.PromoCodeID,
		"promo_code":        promo.PromoCode,
		"discount_percent":  promo.DiscountPercent,
		"original_amount":   promo.OriginalAmount,
		"discount_amount":   promo.DiscountAmount,
		"discounted_amount": promo.DiscountedAmount,
	}
	return snapshot
}

func subscriptionPromoPlanFromSnapshot(snapshot map[string]any) (*subscriptionPromoPlan, bool) {
	if len(snapshot) == 0 {
		return nil, false
	}
	raw, ok := snapshot["subscription_promo_code"].(map[string]any)
	if !ok || raw == nil {
		return nil, false
	}
	id, ok := snapshotInt64(raw["promo_code_id"])
	if !ok || id <= 0 {
		return nil, false
	}
	code, _ := raw["promo_code"].(string)
	discountPercent, _ := snapshotFloat64(raw["discount_percent"])
	originalAmount, _ := snapshotFloat64(raw["original_amount"])
	discountAmount, _ := snapshotFloat64(raw["discount_amount"])
	discountedAmount, _ := snapshotFloat64(raw["discounted_amount"])
	return &subscriptionPromoPlan{
		PromoCodeID:      id,
		PromoCode:        strings.TrimSpace(code),
		DiscountPercent:  discountPercent,
		OriginalAmount:   originalAmount,
		DiscountAmount:   discountAmount,
		DiscountedAmount: discountedAmount,
	}, true
}

func snapshotFloat64(value any) (float64, bool) {
	switch v := value.(type) {
	case float64:
		return v, true
	case float32:
		return float64(v), true
	case int:
		return float64(v), true
	case int64:
		return float64(v), true
	case jsonNumber:
		f, err := v.Float64()
		return f, err == nil
	default:
		return 0, false
	}
}

type jsonNumber interface {
	Float64() (float64, error)
}

func snapshotInt64(value any) (int64, bool) {
	switch v := value.(type) {
	case int:
		return int64(v), true
	case int64:
		return v, true
	case float64:
		return int64(v), v == math.Trunc(v)
	case float32:
		f := float64(v)
		return int64(v), f == math.Trunc(f)
	case jsonNumber:
		f, err := v.Float64()
		return int64(f), err == nil && f == math.Trunc(f)
	default:
		return 0, false
	}
}

func (s *PaymentService) reserveSubscriptionPromoCode(ctx context.Context, tx *dbent.Tx, userID int64, expected *subscriptionPromoPlan) error {
	if expected == nil || expected.PromoCodeID <= 0 {
		return nil
	}
	if s == nil || s.promoRepo == nil || tx == nil {
		return infraerrors.ServiceUnavailable("PROMO_CODE_UNAVAILABLE", "promo code service is unavailable")
	}
	txCtx := dbent.NewTxContext(ctx, tx)
	locked, err := s.promoRepo.GetByCodeForUpdate(txCtx, expected.PromoCode)
	if err != nil {
		return err
	}
	if locked.ID != expected.PromoCodeID || normalizePromoCodeType(locked.CouponType) != PromoCodeTypeSubscription {
		return infraerrors.Conflict("PROMO_CODE_CHANGED", "subscription promo code changed during checkout")
	}
	if err := validatePromoCodeStatus(locked); err != nil {
		return err
	}
	if locked.SubscriptionDiscountPercent == nil || math.Abs(*locked.SubscriptionDiscountPercent-expected.DiscountPercent) > 0.00000001 {
		return infraerrors.Conflict("PROMO_CODE_CHANGED", "subscription promo code changed during checkout")
	}
	if usage, err := s.promoRepo.GetUsageByPromoCodeAndUser(txCtx, locked.ID, userID); err != nil {
		return fmt.Errorf("check subscription promo usage: %w", err)
	} else if usage != nil {
		return ErrPromoCodeAlreadyUsed
	}
	usage := &PromoCodeUsage{
		PromoCodeID: locked.ID,
		UserID:      userID,
		BonusAmount: 0,
		UsedAt:      time.Now(),
	}
	if err := s.promoRepo.CreateUsage(txCtx, usage); err != nil {
		return fmt.Errorf("reserve subscription promo usage: %w", err)
	}
	if err := s.promoRepo.IncrementUsedCount(txCtx, locked.ID); err != nil {
		return fmt.Errorf("increment subscription promo usage: %w", err)
	}
	return nil
}

func (s *PaymentService) releaseSubscriptionPromoReservation(ctx context.Context, order *dbent.PaymentOrder) {
	if s == nil || s.promoRepo == nil || order == nil || order.OrderType != payment.OrderTypeSubscription {
		return
	}
	promo, ok := subscriptionPromoPlanFromSnapshot(order.ProviderSnapshot)
	if !ok || promo.PromoCodeID <= 0 {
		return
	}
	if err := s.promoRepo.ReleaseSubscriptionPromoCode(ctx, promo.PromoCodeID, order.UserID); err != nil {
		slog.Warn("release subscription promo reservation failed", "orderID", order.ID, "promoCodeID", promo.PromoCodeID, "error", err)
	}
}
