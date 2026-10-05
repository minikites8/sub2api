package service

import (
	"context"
	"fmt"
	"math"
	"net/mail"
	"strings"
	"unicode/utf8"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	dbuser "github.com/Wei-Shaw/sub2api/ent/user"
	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
)

const MaxDiscountCouponImportRows = 500

type DiscountCouponImportRow struct {
	RowNumber    int     `json:"row_number"`
	UserID       int64   `json:"user_id,omitempty"`
	Email        string  `json:"email,omitempty"`
	CouponType   string  `json:"coupon_type"`
	MinAmount    float64 `json:"min_amount"`
	DiscountRate float64 `json:"discount_rate"`
	TotalUses    int     `json:"total_uses"`
	Notes        string  `json:"notes,omitempty"`
}

type ImportDiscountCouponsInput struct {
	Rows      []DiscountCouponImportRow `json:"rows"`
	DryRun    bool                      `json:"dry_run"`
	CreatedBy int64                     `json:"-"`
}

type DiscountCouponImportRowResult struct {
	RowNumber int    `json:"row_number"`
	UserID    int64  `json:"user_id,omitempty"`
	Email     string `json:"email,omitempty"`
	CouponID  int64  `json:"coupon_id,omitempty"`
	Error     string `json:"error,omitempty"`
	ErrorCode string `json:"error_code,omitempty"`
}

type ImportDiscountCouponsResult struct {
	Valid       bool                            `json:"valid"`
	IssuedCount int                             `json:"issued_count"`
	Rows        []DiscountCouponImportRowResult `json:"rows"`
}

func validateDiscountCouponImportRow(row DiscountCouponImportRow) error {
	invalid := func(code, message string) error { return infraerrors.BadRequest(code, message) }
	if row.RowNumber < 2 || row.RowNumber > MaxDiscountCouponImportRows+1 {
		return invalid("INVALID_COUPON_ROW", "Excel row number must be between 2 and 501")
	}
	if row.UserID < 0 || row.UserID > 9007199254740991 || (row.UserID == 0 && row.Email == "") {
		return invalid("INVALID_COUPON_USER", "provide a positive user ID or a user email")
	}
	if row.Email != "" {
		address, err := mail.ParseAddress(row.Email)
		if err != nil || address.Address != row.Email || !strings.Contains(row.Email, "@") {
			return invalid("INVALID_COUPON_EMAIL", "provide a valid user email")
		}
	}
	if row.CouponType != "recharge" && row.CouponType != "subscription" {
		return invalid("INVALID_COUPON_TYPE", "coupon type must be recharge or subscription")
	}
	if math.IsNaN(row.MinAmount) || math.IsInf(row.MinAmount, 0) || row.MinAmount < 0.01 || row.MinAmount >= 1e12 {
		return invalid("INVALID_COUPON_MIN_AMOUNT", "minimum amount must be at least 0.01 and below 1000000000000")
	}
	if math.IsNaN(row.DiscountRate) || math.IsInf(row.DiscountRate, 0) || row.DiscountRate < 0.01 || row.DiscountRate > 9.99 {
		return invalid("INVALID_COUPON_DISCOUNT", "discount rate must be between 0.01 and 9.99 (8 means 80% payable)")
	}
	if row.TotalUses <= 0 || row.TotalUses > math.MaxInt32 {
		return invalid("INVALID_COUPON_USES", "coupon uses must be a positive integer up to 2147483647")
	}
	if utf8.RuneCountInString(row.Notes) > 2000 {
		return invalid("INVALID_COUPON_NOTES", "notes must contain at most 2000 characters")
	}
	return nil
}

// ImportDiscountCoupons checks every row before issuing either coupon type in one transaction.
func (s *adminServiceImpl) ImportDiscountCoupons(ctx context.Context, input ImportDiscountCouponsInput) (*ImportDiscountCouponsResult, error) {
	if len(input.Rows) == 0 || len(input.Rows) > MaxDiscountCouponImportRows {
		return nil, infraerrors.BadRequest("INVALID_COUPON_IMPORT_SIZE", "import must contain between 1 and 500 rows")
	}
	if input.CreatedBy <= 0 {
		return nil, infraerrors.BadRequest("INVALID_ADMIN_ID", "admin ID must be positive")
	}
	if s.entClient == nil {
		return nil, fmt.Errorf("discount coupon database is unavailable")
	}
	result := &ImportDiscountCouponsResult{Valid: true, Rows: make([]DiscountCouponImportRowResult, len(input.Rows))}
	seenRows := make(map[int]bool, len(input.Rows))
	for i, row := range input.Rows {
		row.Email = strings.TrimSpace(row.Email)
		row.Notes = strings.TrimSpace(row.Notes)
		input.Rows[i] = row
		item := DiscountCouponImportRowResult{RowNumber: row.RowNumber, UserID: row.UserID, Email: row.Email}
		err := validateDiscountCouponImportRow(row)
		if err == nil && seenRows[row.RowNumber] {
			err = infraerrors.BadRequest("INVALID_COUPON_ROW", "Excel row numbers must be unique")
		}
		seenRows[row.RowNumber] = true
		if err == nil {
			var user *User
			user, err = s.resolveDiscountCouponImportUser(ctx, row)
			if err == nil {
				item.UserID, item.Email = user.ID, user.Email
			}
		}
		if err != nil {
			if infraerrors.Code(err) >= 500 {
				return nil, fmt.Errorf("validate coupon import row %d: %w", row.RowNumber, err)
			}
			item.Error, item.ErrorCode = infraerrors.Message(err), infraerrors.Reason(err)
			result.Valid = false
		}
		result.Rows[i] = item
	}
	if input.DryRun {
		return result, nil
	}
	if !result.Valid {
		compactDiscountCouponImportResult(result)
		return result, nil
	}

	tx, err := s.entClient.Tx(ctx)
	if err != nil {
		return nil, fmt.Errorf("begin coupon import: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	// Reuse the single-coupon services with the transaction's client and user checks.
	issuer := &adminServiceImpl{entClient: tx.Client()}
	for i, row := range input.Rows {
		if row.CouponType == "subscription" {
			coupon, issueErr := issuer.IssueSubscriptionDiscountCoupon(ctx, result.Rows[i].UserID, IssueSubscriptionDiscountCouponInput{
				MinSubscriptionAmount: row.MinAmount, DiscountPercent: row.DiscountRate * 10,
				TotalUses: row.TotalUses, CreatedBy: input.CreatedBy, Notes: row.Notes,
			})
			if issueErr != nil {
				return nil, fmt.Errorf("issue coupon import row %d: %w", row.RowNumber, issueErr)
			}
			result.Rows[i].CouponID = coupon.ID
		} else {
			coupon, issueErr := issuer.IssueRechargeDiscountCoupon(ctx, result.Rows[i].UserID, IssueRechargeDiscountCouponInput{
				MinRechargeAmount: row.MinAmount, DiscountPercent: row.DiscountRate * 10,
				TotalUses: row.TotalUses, CreatedBy: input.CreatedBy, Notes: row.Notes,
			})
			if issueErr != nil {
				return nil, fmt.Errorf("issue coupon import row %d: %w", row.RowNumber, issueErr)
			}
			result.Rows[i].CouponID = coupon.ID
		}
	}
	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit coupon import: %w", err)
	}
	result.IssuedCount = len(result.Rows)
	compactDiscountCouponImportResult(result)
	return result, nil
}

// Keep the 500-row write response below the idempotency store's 64 KiB default.
// The dry-run response supplies user details; write responses carry row IDs and outcomes.
func compactDiscountCouponImportResult(result *ImportDiscountCouponsResult) {
	for i := range result.Rows {
		result.Rows[i].UserID = 0
		result.Rows[i].Email = ""
		result.Rows[i].Error = ""
	}
}

func (s *adminServiceImpl) resolveDiscountCouponImportUser(ctx context.Context, row DiscountCouponImportRow) (*User, error) {
	var found *User
	var err error
	if s.userRepo != nil {
		if row.UserID > 0 {
			found, err = s.userRepo.GetByID(ctx, row.UserID)
		} else {
			found, err = s.userRepo.GetByEmail(ctx, row.Email)
		}
	} else {
		query := s.entClient.User.Query().Where(dbuser.DeletedAtIsNil())
		if row.UserID > 0 {
			query.Where(dbuser.IDEQ(row.UserID))
		} else {
			query.Where(dbuser.EmailEqualFold(row.Email))
		}
		var entity *dbent.User
		entity, err = query.Only(ctx)
		if dbent.IsNotFound(err) {
			err = ErrUserNotFound
		}
		if err == nil {
			found = &User{ID: entity.ID, Email: entity.Email}
		}
	}
	if err != nil {
		return nil, err
	}
	if row.Email != "" && !strings.EqualFold(row.Email, found.Email) {
		return nil, infraerrors.BadRequest("COUPON_USER_EMAIL_MISMATCH", "user ID and email must identify the same user")
	}
	return found, nil
}
