package service

import (
	"context"
	"database/sql"
	"fmt"
	"math"
	"sort"
	"strings"
	"time"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/ent/paymentorder"
	"github.com/Wei-Shaw/sub2api/internal/payment"
	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
)

const (
	subscriptionDiscountCouponStatusActive = "active"
	subscriptionDiscountCouponSourceAdmin  = "admin"
)

type SubscriptionDiscountCoupon struct {
	ID                    int64     `json:"id"`
	UserID                int64     `json:"user_id"`
	MinSubscriptionAmount float64   `json:"min_subscription_amount"`
	DiscountPercent       float64   `json:"discount_percent"`
	TotalUses             int       `json:"total_uses"`
	UsedCount             int       `json:"used_count"`
	RemainingUses         int       `json:"remaining_uses"`
	Status                string    `json:"status"`
	CreatedBy             int64     `json:"created_by"`
	Notes                 string    `json:"notes"`
	CreatedAt             time.Time `json:"created_at"`
	UpdatedAt             time.Time `json:"updated_at"`
	SourceType            string    `json:"source_type"`
}

type SubscriptionDiscountCouponPreview struct {
	ID                    int64   `json:"id"`
	MinSubscriptionAmount float64 `json:"min_subscription_amount"`
	DiscountPercent       float64 `json:"discount_percent"`
	TotalUses             int     `json:"total_uses"`
	UsedCount             int     `json:"used_count"`
	RemainingUses         int     `json:"remaining_uses"`
}

type IssueSubscriptionDiscountCouponInput struct {
	MinSubscriptionAmount float64
	DiscountPercent       float64
	TotalUses             int
	CreatedBy             int64
	Notes                 string
}

func (s *adminServiceImpl) IssueSubscriptionDiscountCoupon(ctx context.Context, userID int64, input IssueSubscriptionDiscountCouponInput) (*SubscriptionDiscountCoupon, error) {
	if userID <= 0 {
		return nil, infraerrors.BadRequest("INVALID_USER_ID", "user ID must be positive")
	}
	if math.IsNaN(input.MinSubscriptionAmount) || math.IsInf(input.MinSubscriptionAmount, 0) || input.MinSubscriptionAmount <= 0 {
		return nil, infraerrors.BadRequest("INVALID_COUPON_MIN_AMOUNT", "minimum subscription amount must be greater than 0")
	}
	if math.IsNaN(input.DiscountPercent) || math.IsInf(input.DiscountPercent, 0) || input.DiscountPercent <= 0 || input.DiscountPercent >= 100 {
		return nil, infraerrors.BadRequest("INVALID_COUPON_DISCOUNT", "discount rate must be between 0 and 10")
	}
	if input.TotalUses <= 0 {
		return nil, infraerrors.BadRequest("INVALID_COUPON_USES", "coupon uses must be greater than 0")
	}
	if input.CreatedBy <= 0 {
		return nil, infraerrors.BadRequest("INVALID_ADMIN_ID", "admin ID must be positive")
	}
	if s.entClient == nil {
		return nil, fmt.Errorf("subscription discount coupon database is unavailable")
	}
	if s.userRepo != nil {
		if _, err := s.userRepo.GetByID(ctx, userID); err != nil {
			return nil, err
		}
	} else if _, err := s.entClient.User.Get(ctx, userID); err != nil {
		return nil, err
	}

	now := time.Now().UTC()
	rows, err := s.entClient.QueryContext(ctx, `
INSERT INTO subscription_discount_coupons
    (user_id, min_subscription_amount, discount_percent, total_uses, status, created_by, notes, created_at, updated_at, source_type)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $8, $9)
RETURNING id, user_id, min_subscription_amount, discount_percent, total_uses, status,
          created_by, notes, created_at, updated_at, source_type`,
		userID,
		roundTo(input.MinSubscriptionAmount, 8),
		roundTo(input.DiscountPercent, 2),
		input.TotalUses,
		subscriptionDiscountCouponStatusActive,
		input.CreatedBy,
		nullableTrimmedString(input.Notes),
		now,
		subscriptionDiscountCouponSourceAdmin,
	)
	if err != nil {
		return nil, fmt.Errorf("issue subscription discount coupon: %w", err)
	}
	defer rows.Close()
	if !rows.Next() {
		if err := rows.Err(); err != nil {
			return nil, fmt.Errorf("issue subscription discount coupon: %w", err)
		}
		return nil, fmt.Errorf("issue subscription discount coupon: insert returned no row")
	}
	coupon, err := scanSubscriptionDiscountCoupon(rows)
	if err != nil {
		return nil, fmt.Errorf("issue subscription discount coupon: %w", err)
	}
	coupon.RemainingUses = coupon.TotalUses
	return coupon, nil
}

type subscriptionDiscountCouponScanner interface {
	Scan(dest ...any) error
}

func scanSubscriptionDiscountCoupon(scanner subscriptionDiscountCouponScanner) (*SubscriptionDiscountCoupon, error) {
	var notes sql.NullString
	coupon := &SubscriptionDiscountCoupon{}
	if err := scanner.Scan(
		&coupon.ID,
		&coupon.UserID,
		&coupon.MinSubscriptionAmount,
		&coupon.DiscountPercent,
		&coupon.TotalUses,
		&coupon.Status,
		&coupon.CreatedBy,
		&notes,
		&coupon.CreatedAt,
		&coupon.UpdatedAt,
		&coupon.SourceType,
	); err != nil {
		return nil, err
	}
	if notes.Valid {
		coupon.Notes = notes.String
	}
	return coupon, nil
}

func (s *adminServiceImpl) ListUserSubscriptionDiscountCoupons(ctx context.Context, userID int64) ([]SubscriptionDiscountCoupon, error) {
	if userID <= 0 {
		return nil, infraerrors.BadRequest("INVALID_USER_ID", "user ID must be positive")
	}
	if s.entClient == nil {
		return nil, fmt.Errorf("subscription discount coupon database is unavailable")
	}
	if s.userRepo != nil {
		if _, err := s.userRepo.GetByID(ctx, userID); err != nil {
			return nil, err
		}
	} else if _, err := s.entClient.User.Get(ctx, userID); err != nil {
		return nil, err
	}
	return listUserSubscriptionDiscountCoupons(ctx, s.entClient, userID)
}

func listUserSubscriptionDiscountCoupons(ctx context.Context, client *dbent.Client, userID int64) ([]SubscriptionDiscountCoupon, error) {
	rows, err := client.QueryContext(ctx, `
SELECT id, user_id, min_subscription_amount, discount_percent, total_uses, status,
       created_by, notes, created_at, updated_at, source_type
FROM subscription_discount_coupons
WHERE user_id = $1
ORDER BY created_at DESC, id DESC`, userID)
	if err != nil {
		return nil, fmt.Errorf("list user subscription discount coupons: %w", err)
	}
	defer rows.Close()

	coupons := make([]SubscriptionDiscountCoupon, 0)
	for rows.Next() {
		coupon, scanErr := scanSubscriptionDiscountCoupon(rows)
		if scanErr != nil {
			return nil, fmt.Errorf("scan user subscription discount coupon: %w", scanErr)
		}
		coupons = append(coupons, *coupon)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list user subscription discount coupons: %w", err)
	}
	if err := rows.Close(); err != nil {
		return nil, fmt.Errorf("close user subscription discount coupon rows: %w", err)
	}

	for i := range coupons {
		used, countErr := countSubscriptionDiscountCouponOrders(ctx, client, userID, coupons[i])
		if countErr != nil {
			return nil, countErr
		}
		coupons[i].UsedCount = used
		coupons[i].RemainingUses = coupons[i].TotalUses - used
		if coupons[i].RemainingUses < 0 {
			coupons[i].RemainingUses = 0
		}
	}
	return coupons, nil
}

func (s *PaymentService) ListAvailableSubscriptionDiscountCoupons(ctx context.Context, userID int64) ([]SubscriptionDiscountCouponPreview, error) {
	if s == nil || s.entClient == nil || userID <= 0 {
		return []SubscriptionDiscountCouponPreview{}, nil
	}
	coupons, err := listUserSubscriptionDiscountCoupons(ctx, s.entClient, userID)
	if err != nil {
		if isSubscriptionDiscountCouponTableUnavailable(err) {
			return []SubscriptionDiscountCouponPreview{}, nil
		}
		return nil, fmt.Errorf("list subscription discount coupons: %w", err)
	}
	sort.SliceStable(coupons, func(i, j int) bool {
		if coupons[i].DiscountPercent != coupons[j].DiscountPercent {
			return coupons[i].DiscountPercent < coupons[j].DiscountPercent
		}
		if coupons[i].MinSubscriptionAmount != coupons[j].MinSubscriptionAmount {
			return coupons[i].MinSubscriptionAmount > coupons[j].MinSubscriptionAmount
		}
		return coupons[i].ID < coupons[j].ID
	})

	available := make([]SubscriptionDiscountCouponPreview, 0, len(coupons))
	for i := range coupons {
		if coupons[i].Status == subscriptionDiscountCouponStatusActive && coupons[i].RemainingUses > 0 {
			available = append(available, SubscriptionDiscountCouponPreview{
				ID:                    coupons[i].ID,
				MinSubscriptionAmount: coupons[i].MinSubscriptionAmount,
				DiscountPercent:       coupons[i].DiscountPercent,
				TotalUses:             coupons[i].TotalUses,
				UsedCount:             coupons[i].UsedCount,
				RemainingUses:         coupons[i].RemainingUses,
			})
		}
	}
	return available, nil
}

func isSubscriptionDiscountCouponTableUnavailable(err error) bool {
	if err == nil {
		return false
	}
	message := strings.ToLower(err.Error())
	return strings.Contains(message, "no such table: subscription_discount_coupons") ||
		(strings.Contains(message, "subscription_discount_coupons") && strings.Contains(message, "does not exist"))
}

type subscriptionDiscountPlan struct {
	CouponID              int64
	MinSubscriptionAmount float64
	DiscountPercent       float64
	OriginalAmount        float64
	DiscountAmount        float64
	DiscountedAmount      float64
}

func (s *PaymentService) resolveSubscriptionDiscountCoupon(ctx context.Context, userID int64, amount float64) (*subscriptionDiscountPlan, error) {
	coupons, err := s.ListAvailableSubscriptionDiscountCoupons(ctx, userID)
	if err != nil {
		return nil, err
	}
	for _, coupon := range coupons {
		if amount+0.00000001 >= coupon.MinSubscriptionAmount {
			original := roundTo(amount, 8)
			discounted := roundTo(original*(coupon.DiscountPercent/100), 8)
			return &subscriptionDiscountPlan{
				CouponID: coupon.ID, MinSubscriptionAmount: coupon.MinSubscriptionAmount,
				DiscountPercent: coupon.DiscountPercent, OriginalAmount: original,
				DiscountAmount: roundTo(original-discounted, 8), DiscountedAmount: discounted,
			}, nil
		}
	}
	return nil, nil
}

func appendSubscriptionDiscountSnapshot(snapshot map[string]any, plan *subscriptionDiscountPlan) map[string]any {
	if plan == nil || plan.CouponID <= 0 {
		return snapshot
	}
	if snapshot == nil {
		snapshot = map[string]any{}
	}
	snapshot["subscription_discount_coupon"] = map[string]any{
		"coupon_id": plan.CouponID, "min_subscription_amount": plan.MinSubscriptionAmount,
		"discount_percent": plan.DiscountPercent, "original_amount": plan.OriginalAmount,
		"discount_amount": plan.DiscountAmount, "discounted_amount": plan.DiscountedAmount,
	}
	return snapshot
}

func subscriptionDiscountPlanFromSnapshot(snapshot map[string]any) (*subscriptionDiscountPlan, bool) {
	raw, ok := snapshot["subscription_discount_coupon"].(map[string]any)
	if !ok || raw == nil {
		return nil, false
	}
	id, ok := snapshotInt64(raw["coupon_id"])
	if !ok || id <= 0 {
		return nil, false
	}
	minimum, _ := snapshotFloat64(raw["min_subscription_amount"])
	discount, _ := snapshotFloat64(raw["discount_percent"])
	original, _ := snapshotFloat64(raw["original_amount"])
	saved, _ := snapshotFloat64(raw["discount_amount"])
	discounted, _ := snapshotFloat64(raw["discounted_amount"])
	return &subscriptionDiscountPlan{CouponID: id, MinSubscriptionAmount: minimum,
		DiscountPercent: discount, OriginalAmount: original, DiscountAmount: saved,
		DiscountedAmount: discounted}, true
}

func countSubscriptionDiscountCouponOrders(ctx context.Context, client *dbent.Client, userID int64, coupon SubscriptionDiscountCoupon) (int, error) {
	if client == nil || coupon.ID <= 0 {
		return 0, nil
	}
	orders, err := client.PaymentOrder.Query().
		Where(
			paymentorder.UserIDEQ(userID),
			paymentorder.OrderTypeEQ(payment.OrderTypeSubscription),
			paymentorder.StatusIn(
				OrderStatusPending,
				OrderStatusPaid,
				OrderStatusRecharging,
				OrderStatusCompleted,
				OrderStatusRefundRequested,
				OrderStatusRefunding,
				OrderStatusPartiallyRefunded,
				OrderStatusRefunded,
				OrderStatusRefundFailed,
			),
			paymentorder.ProviderSnapshotNotNil(),
		).
		All(ctx)
	if err != nil {
		return 0, fmt.Errorf("count subscription discount coupon orders: %w", err)
	}
	count := 0
	for _, order := range orders {
		plan, ok := subscriptionDiscountPlanFromSnapshot(order.ProviderSnapshot)
		if ok && plan.CouponID == coupon.ID {
			count++
		}
	}
	return count, nil
}

func (s *PaymentService) checkSubscriptionDiscountCouponOrderLimit(ctx context.Context, tx *dbent.Tx, userID int64, plan *subscriptionDiscountPlan) error {
	if plan == nil || plan.CouponID <= 0 {
		return nil
	}
	query := `
SELECT id, user_id, min_subscription_amount, discount_percent, total_uses, status,
       created_by, notes, created_at, updated_at, source_type
FROM subscription_discount_coupons
WHERE id = $1 AND user_id = $2`
	if paymentTxSupportsForUpdate(tx) {
		query += " FOR UPDATE"
	}
	rows, err := tx.Client().QueryContext(ctx, query, plan.CouponID, userID)
	if err != nil {
		return fmt.Errorf("lock subscription discount coupon: %w", err)
	}
	defer rows.Close()
	if !rows.Next() {
		return infraerrors.Conflict("SUBSCRIPTION_COUPON_UNAVAILABLE", "subscription discount coupon is unavailable")
	}
	coupon, err := scanSubscriptionDiscountCoupon(rows)
	if err != nil {
		return fmt.Errorf("lock subscription discount coupon: %w", err)
	}
	if err := rows.Close(); err != nil {
		return fmt.Errorf("close subscription discount coupon lock rows: %w", err)
	}
	if coupon.Status != subscriptionDiscountCouponStatusActive ||
		plan.OriginalAmount+0.00000001 < coupon.MinSubscriptionAmount ||
		math.Abs(coupon.DiscountPercent-plan.DiscountPercent) > 0.00000001 {
		return infraerrors.Conflict("SUBSCRIPTION_COUPON_UNAVAILABLE", "subscription discount coupon is unavailable")
	}
	used, err := countSubscriptionDiscountCouponOrders(ctx, tx.Client(), userID, *coupon)
	if err != nil {
		return err
	}
	if coupon.TotalUses > 0 && used >= coupon.TotalUses {
		return infraerrors.Conflict("SUBSCRIPTION_COUPON_LIMIT_REACHED", "subscription discount coupon usage limit has been reached")
	}
	return nil
}
