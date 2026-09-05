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

const rechargeDiscountCouponStatusActive = "active"

type RechargeDiscountCoupon struct {
	ID                int64     `json:"id"`
	UserID            int64     `json:"user_id"`
	MinRechargeAmount float64   `json:"min_recharge_amount"`
	DiscountPercent   float64   `json:"discount_percent"`
	TotalUses         int       `json:"total_uses"`
	UsedCount         int       `json:"used_count"`
	RemainingUses     int       `json:"remaining_uses"`
	Status            string    `json:"status"`
	CreatedBy         int64     `json:"created_by"`
	Notes             string    `json:"notes"`
	CreatedAt         time.Time `json:"created_at"`
	UpdatedAt         time.Time `json:"updated_at"`
}

type RechargeDiscountCouponPreview struct {
	ID                int64   `json:"id"`
	MinRechargeAmount float64 `json:"min_recharge_amount"`
	DiscountPercent   float64 `json:"discount_percent"`
	TotalUses         int     `json:"total_uses"`
	UsedCount         int     `json:"used_count"`
	RemainingUses     int     `json:"remaining_uses"`
}

type IssueRechargeDiscountCouponInput struct {
	MinRechargeAmount float64
	DiscountPercent   float64
	TotalUses         int
	CreatedBy         int64
	Notes             string
}

func (s *adminServiceImpl) IssueRechargeDiscountCoupon(ctx context.Context, userID int64, input IssueRechargeDiscountCouponInput) (*RechargeDiscountCoupon, error) {
	if userID <= 0 {
		return nil, infraerrors.BadRequest("INVALID_USER_ID", "user ID must be positive")
	}
	if math.IsNaN(input.MinRechargeAmount) || math.IsInf(input.MinRechargeAmount, 0) || input.MinRechargeAmount <= 0 {
		return nil, infraerrors.BadRequest("INVALID_COUPON_MIN_AMOUNT", "minimum recharge amount must be greater than 0")
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
		return nil, fmt.Errorf("recharge discount coupon database is unavailable")
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
INSERT INTO recharge_discount_coupons
    (user_id, min_recharge_amount, discount_percent, total_uses, status, created_by, notes, created_at, updated_at)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $8)
RETURNING id, user_id, min_recharge_amount, discount_percent, total_uses, status,
          created_by, notes, created_at, updated_at`,
		userID,
		roundTo(input.MinRechargeAmount, 8),
		roundTo(input.DiscountPercent, 2),
		input.TotalUses,
		rechargeDiscountCouponStatusActive,
		input.CreatedBy,
		nullableTrimmedString(input.Notes),
		now,
	)
	if err != nil {
		return nil, fmt.Errorf("issue recharge discount coupon: %w", err)
	}
	defer rows.Close()
	if !rows.Next() {
		if err := rows.Err(); err != nil {
			return nil, fmt.Errorf("issue recharge discount coupon: %w", err)
		}
		return nil, fmt.Errorf("issue recharge discount coupon: insert returned no row")
	}
	coupon, err := scanRechargeDiscountCoupon(rows)
	if err != nil {
		return nil, fmt.Errorf("issue recharge discount coupon: %w", err)
	}
	coupon.RemainingUses = coupon.TotalUses
	return coupon, nil
}

func nullableTrimmedString(value string) any {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}
	return value
}

type rechargeDiscountCouponScanner interface {
	Scan(dest ...any) error
}

func scanRechargeDiscountCoupon(scanner rechargeDiscountCouponScanner) (*RechargeDiscountCoupon, error) {
	var notes sql.NullString
	coupon := &RechargeDiscountCoupon{}
	if err := scanner.Scan(
		&coupon.ID,
		&coupon.UserID,
		&coupon.MinRechargeAmount,
		&coupon.DiscountPercent,
		&coupon.TotalUses,
		&coupon.Status,
		&coupon.CreatedBy,
		&notes,
		&coupon.CreatedAt,
		&coupon.UpdatedAt,
	); err != nil {
		return nil, err
	}
	if notes.Valid {
		coupon.Notes = notes.String
	}
	return coupon, nil
}

func (s *adminServiceImpl) ListUserRechargeDiscountCoupons(ctx context.Context, userID int64) ([]RechargeDiscountCoupon, error) {
	if userID <= 0 {
		return nil, infraerrors.BadRequest("INVALID_USER_ID", "user ID must be positive")
	}
	if s.entClient == nil {
		return nil, fmt.Errorf("recharge discount coupon database is unavailable")
	}
	if s.userRepo != nil {
		if _, err := s.userRepo.GetByID(ctx, userID); err != nil {
			return nil, err
		}
	} else if _, err := s.entClient.User.Get(ctx, userID); err != nil {
		return nil, err
	}
	return listUserRechargeDiscountCoupons(ctx, s.entClient, userID)
}

func listUserRechargeDiscountCoupons(ctx context.Context, client *dbent.Client, userID int64) ([]RechargeDiscountCoupon, error) {
	rows, err := client.QueryContext(ctx, `
SELECT id, user_id, min_recharge_amount, discount_percent, total_uses, status,
       created_by, notes, created_at, updated_at
FROM recharge_discount_coupons
WHERE user_id = $1
ORDER BY created_at DESC, id DESC`, userID)
	if err != nil {
		return nil, fmt.Errorf("list user recharge discount coupons: %w", err)
	}
	defer rows.Close()

	coupons := make([]RechargeDiscountCoupon, 0)
	for rows.Next() {
		coupon, scanErr := scanRechargeDiscountCoupon(rows)
		if scanErr != nil {
			return nil, fmt.Errorf("scan user recharge discount coupon: %w", scanErr)
		}
		coupons = append(coupons, *coupon)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list user recharge discount coupons: %w", err)
	}
	if err := rows.Close(); err != nil {
		return nil, fmt.Errorf("close user recharge discount coupon rows: %w", err)
	}

	for i := range coupons {
		used, countErr := countRechargeDiscountCouponOrders(ctx, client, userID, coupons[i].ID)
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

func (s *PaymentService) ListAvailableRechargeDiscountCoupons(ctx context.Context, userID int64) ([]RechargeDiscountCouponPreview, error) {
	if s == nil || s.entClient == nil || userID <= 0 {
		return []RechargeDiscountCouponPreview{}, nil
	}
	coupons, err := listUserRechargeDiscountCoupons(ctx, s.entClient, userID)
	if err != nil {
		if isRechargeDiscountCouponTableUnavailable(err) {
			return []RechargeDiscountCouponPreview{}, nil
		}
		return nil, fmt.Errorf("list recharge discount coupons: %w", err)
	}
	sort.SliceStable(coupons, func(i, j int) bool {
		if coupons[i].DiscountPercent != coupons[j].DiscountPercent {
			return coupons[i].DiscountPercent < coupons[j].DiscountPercent
		}
		if coupons[i].MinRechargeAmount != coupons[j].MinRechargeAmount {
			return coupons[i].MinRechargeAmount > coupons[j].MinRechargeAmount
		}
		return coupons[i].ID < coupons[j].ID
	})

	available := make([]RechargeDiscountCouponPreview, 0, len(coupons))
	for i := range coupons {
		if coupons[i].Status == rechargeDiscountCouponStatusActive && coupons[i].RemainingUses > 0 {
			available = append(available, RechargeDiscountCouponPreview{
				ID:                coupons[i].ID,
				MinRechargeAmount: coupons[i].MinRechargeAmount,
				DiscountPercent:   coupons[i].DiscountPercent,
				TotalUses:         coupons[i].TotalUses,
				UsedCount:         coupons[i].UsedCount,
				RemainingUses:     coupons[i].RemainingUses,
			})
		}
	}
	return available, nil
}

func isRechargeDiscountCouponTableUnavailable(err error) bool {
	if err == nil {
		return false
	}
	message := strings.ToLower(err.Error())
	return strings.Contains(message, "no such table: recharge_discount_coupons") ||
		(strings.Contains(message, "recharge_discount_coupons") && strings.Contains(message, "does not exist"))
}

func (s *PaymentService) resolveRechargeDiscountCoupon(ctx context.Context, userID int64, amount float64) (*RechargeDiscountCouponPreview, error) {
	coupons, err := s.ListAvailableRechargeDiscountCoupons(ctx, userID)
	if err != nil {
		return nil, err
	}
	for i := range coupons {
		if amount+0.00000001 >= coupons[i].MinRechargeAmount {
			return &coupons[i], nil
		}
	}
	return nil, nil
}

func applyRechargeDiscountCoupon(requestAmount float64, plan firstRechargeAmountPlan, coupon *RechargeDiscountCouponPreview) firstRechargeAmountPlan {
	if coupon == nil || coupon.ID <= 0 || coupon.RemainingUses <= 0 || requestAmount+0.00000001 < coupon.MinRechargeAmount {
		return plan
	}
	discountPercent := clampFirstRechargeDiscount(coupon.DiscountPercent)
	if discountPercent <= 0 || discountPercent >= 100 {
		return plan
	}
	if plan.DiscountSet && plan.DiscountPercent <= discountPercent {
		return plan
	}
	plan.CouponID = coupon.ID
	plan.CouponMinRechargeAmount = roundTo(coupon.MinRechargeAmount, 8)
	plan.DiscountPercent = discountPercent
	plan.DiscountTimes = coupon.TotalUses
	plan.DiscountSet = true
	plan.BaseCreditAmount = roundTo(requestAmount, 8)
	plan.CreditAmount = roundTo(plan.BaseCreditAmount+plan.BonusAmount, 8)
	plan.PaymentAmount = roundTo(requestAmount*(discountPercent/100), 8)
	return plan
}

func countRechargeDiscountCouponOrders(ctx context.Context, client *dbent.Client, userID, couponID int64) (int, error) {
	if client == nil || couponID <= 0 {
		return 0, nil
	}
	orders, err := client.PaymentOrder.Query().
		Where(
			paymentorder.UserIDEQ(userID),
			paymentorder.OrderTypeEQ(payment.OrderTypeBalance),
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
		return 0, fmt.Errorf("count recharge discount coupon orders: %w", err)
	}
	count := 0
	for _, order := range orders {
		plan, ok := firstRechargePromoPlanForOrder(order)
		if ok && plan.CouponID == couponID && plan.discountApplied() {
			count++
		}
	}
	return count, nil
}

func (s *PaymentService) checkRechargeDiscountCouponOrderLimit(ctx context.Context, tx *dbent.Tx, userID int64, requestAmount float64, plan firstRechargeAmountPlan) error {
	if plan.CouponID <= 0 {
		return nil
	}
	query := `
SELECT id, user_id, min_recharge_amount, discount_percent, total_uses, status,
       created_by, notes, created_at, updated_at
FROM recharge_discount_coupons
WHERE id = $1 AND user_id = $2`
	if paymentTxSupportsForUpdate(tx) {
		query += " FOR UPDATE"
	}
	rows, err := tx.Client().QueryContext(ctx, query, plan.CouponID, userID)
	if err != nil {
		return fmt.Errorf("lock recharge discount coupon: %w", err)
	}
	defer rows.Close()
	if !rows.Next() {
		return infraerrors.Conflict("RECHARGE_COUPON_UNAVAILABLE", "recharge discount coupon is unavailable")
	}
	coupon, err := scanRechargeDiscountCoupon(rows)
	if err != nil {
		return fmt.Errorf("lock recharge discount coupon: %w", err)
	}
	if err := rows.Close(); err != nil {
		return fmt.Errorf("close recharge discount coupon lock rows: %w", err)
	}
	if coupon.Status != rechargeDiscountCouponStatusActive ||
		requestAmount+0.00000001 < coupon.MinRechargeAmount ||
		math.Abs(coupon.DiscountPercent-plan.DiscountPercent) > 0.00000001 {
		return infraerrors.Conflict("RECHARGE_COUPON_UNAVAILABLE", "recharge discount coupon is unavailable")
	}
	used, err := countRechargeDiscountCouponOrders(ctx, tx.Client(), userID, coupon.ID)
	if err != nil {
		return err
	}
	if used >= coupon.TotalUses {
		return infraerrors.Conflict("RECHARGE_COUPON_LIMIT_REACHED", "recharge discount coupon usage limit has been reached")
	}
	return nil
}
