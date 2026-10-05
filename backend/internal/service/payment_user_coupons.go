package service

import (
	"context"
	"fmt"
	"sort"
	"time"
)

// UserDiscountCoupon contains the fields users need to review their own coupons.
type UserDiscountCoupon struct {
	ID              int64     `json:"id"`
	CouponType      string    `json:"coupon_type"`
	MinAmount       float64   `json:"min_amount"`
	DiscountPercent float64   `json:"discount_percent"`
	TotalUses       int       `json:"total_uses"`
	UsedCount       int       `json:"used_count"`
	RemainingUses   int       `json:"remaining_uses"`
	Status          string    `json:"status"`
	CreatedAt       time.Time `json:"created_at"`
}

func (s *PaymentService) ListMyDiscountCoupons(ctx context.Context, userID int64) ([]UserDiscountCoupon, error) {
	result := make([]UserDiscountCoupon, 0)
	if s == nil || s.entClient == nil || userID <= 0 {
		return result, nil
	}
	rechargeCoupons, err := listUserRechargeDiscountCoupons(ctx, s.entClient, userID)
	if err != nil && !isRechargeDiscountCouponTableUnavailable(err) {
		return nil, fmt.Errorf("list profile recharge coupons: %w", err)
	}
	for _, coupon := range rechargeCoupons {
		result = append(result, UserDiscountCoupon{
			ID: coupon.ID, CouponType: "recharge", MinAmount: coupon.MinRechargeAmount,
			DiscountPercent: coupon.DiscountPercent, TotalUses: coupon.TotalUses,
			UsedCount: coupon.UsedCount, RemainingUses: coupon.RemainingUses,
			Status: userDiscountCouponStatus(coupon.Status, coupon.TotalUses, coupon.RemainingUses), CreatedAt: coupon.CreatedAt,
		})
	}
	subscriptionCoupons, err := listUserSubscriptionDiscountCoupons(ctx, s.entClient, userID)
	if err != nil && !isSubscriptionDiscountCouponTableUnavailable(err) {
		return nil, fmt.Errorf("list profile subscription coupons: %w", err)
	}
	for _, coupon := range subscriptionCoupons {
		result = append(result, UserDiscountCoupon{
			ID: coupon.ID, CouponType: "subscription", MinAmount: coupon.MinSubscriptionAmount,
			DiscountPercent: coupon.DiscountPercent, TotalUses: coupon.TotalUses,
			UsedCount: coupon.UsedCount, RemainingUses: coupon.RemainingUses,
			Status: userDiscountCouponStatus(coupon.Status, coupon.TotalUses, coupon.RemainingUses), CreatedAt: coupon.CreatedAt,
		})
	}
	sort.SliceStable(result, func(i, j int) bool {
		if (result[i].Status == "available") != (result[j].Status == "available") {
			return result[i].Status == "available"
		}
		return result[i].CreatedAt.After(result[j].CreatedAt)
	})
	return result, nil
}

func userDiscountCouponStatus(status string, totalUses, remainingUses int) string {
	if status != "active" {
		return "inactive"
	}
	if totalUses > 0 && remainingUses <= 0 {
		return "exhausted"
	}
	return "available"
}
