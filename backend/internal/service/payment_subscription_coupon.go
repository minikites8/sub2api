package service

import (
	"context"
	"log/slog"
	"math"
	"strings"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/internal/payment"
)

// subscriptionPromoPlan preserves discounts on historical promo-code orders.
type subscriptionPromoPlan struct {
	PromoCodeID      int64
	PromoCode        string
	DiscountPercent  float64
	OriginalAmount   float64
	DiscountAmount   float64
	DiscountedAmount float64
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
