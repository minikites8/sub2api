package service

import (
	"context"
	"encoding/json"
	"math"
	"strings"
	"testing"
	"time"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/stretchr/testify/require"
)

func TestDiscountCouponImportDryRunAndMixedIssuance(t *testing.T) {
	ctx := context.Background()
	client, user, admin, _ := subscriptionCouponFixture(t)
	createRechargeCouponTestTable(t, client)
	other, err := client.User.Create().SetEmail("other@example.com").SetPasswordHash("hash").Save(ctx)
	require.NoError(t, err)
	input := ImportDiscountCouponsInput{CreatedBy: 99, DryRun: true, Rows: []DiscountCouponImportRow{
		{RowNumber: 2, Email: " SUBSCRIPTION-COUPON@example.com ", CouponType: "subscription", MinAmount: 20, DiscountRate: 8.5, TotalUses: 3, Notes: " retention "},
		{RowNumber: 4, UserID: other.ID, CouponType: "recharge", MinAmount: 100, DiscountRate: 8, TotalUses: 2},
	}}
	result, err := admin.ImportDiscountCoupons(ctx, input)
	require.NoError(t, err)
	require.True(t, result.Valid)
	require.Zero(t, result.IssuedCount)
	require.Equal(t, user.ID, result.Rows[0].UserID)
	require.Equal(t, other.Email, result.Rows[1].Email)
	coupons, err := admin.ListUserSubscriptionDiscountCoupons(ctx, user.ID)
	require.NoError(t, err)
	require.Empty(t, coupons)

	input.DryRun = false
	result, err = admin.ImportDiscountCoupons(ctx, input)
	require.NoError(t, err)
	require.Equal(t, 2, result.IssuedCount)
	require.Positive(t, result.Rows[0].CouponID)
	require.Positive(t, result.Rows[1].CouponID)
	coupons, err = admin.ListUserSubscriptionDiscountCoupons(ctx, user.ID)
	require.NoError(t, err)
	require.Len(t, coupons, 1)
	require.Equal(t, 85.0, coupons[0].DiscountPercent)
	require.Equal(t, 20.0, coupons[0].MinSubscriptionAmount)
	require.Equal(t, "retention", coupons[0].Notes)
	require.Equal(t, int64(99), coupons[0].CreatedBy)
	recharges, err := admin.ListUserRechargeDiscountCoupons(ctx, other.ID)
	require.NoError(t, err)
	require.Len(t, recharges, 1)
	require.Equal(t, 80.0, recharges[0].DiscountPercent)
	require.Equal(t, 100.0, recharges[0].MinRechargeAmount)
}

func TestDiscountCouponImportCollectsRowErrorsBeforeIssuance(t *testing.T) {
	ctx := context.Background()
	client, user, admin, _ := subscriptionCouponFixture(t)
	deleted, err := client.User.Create().SetEmail("deleted@example.com").SetPasswordHash("hash").SetDeletedAt(time.Now()).Save(ctx)
	require.NoError(t, err)
	result, err := admin.ImportDiscountCoupons(ctx, ImportDiscountCouponsInput{CreatedBy: 99, Rows: []DiscountCouponImportRow{
		{RowNumber: 2, UserID: user.ID, CouponType: "subscription", MinAmount: 20, DiscountRate: 8, TotalUses: 1},
		{RowNumber: 3, UserID: user.ID, Email: "other@example.com", CouponType: "subscription", MinAmount: 20, DiscountRate: 8, TotalUses: 1},
		{RowNumber: 4, UserID: user.ID, CouponType: "subscription", MinAmount: 20, DiscountRate: 10, TotalUses: 1},
		{RowNumber: 6, UserID: deleted.ID, CouponType: "subscription", MinAmount: 20, DiscountRate: 8, TotalUses: 1},
	}})
	require.NoError(t, err)
	require.False(t, result.Valid)
	require.Zero(t, result.IssuedCount)
	require.Empty(t, result.Rows[0].Error)
	require.Equal(t, "COUPON_USER_EMAIL_MISMATCH", result.Rows[1].ErrorCode)
	require.Equal(t, "INVALID_COUPON_DISCOUNT", result.Rows[2].ErrorCode)
	require.Equal(t, "USER_NOT_FOUND", result.Rows[3].ErrorCode)
	coupons, err := admin.ListUserSubscriptionDiscountCoupons(ctx, user.ID)
	require.NoError(t, err)
	require.Empty(t, coupons)
}

func TestDiscountCouponImportRollsBackEarlierRowsOnWriteFailure(t *testing.T) {
	ctx := context.Background()
	client, user, admin, _ := subscriptionCouponFixture(t)
	createRechargeCouponTestTable(t, client)
	_, err := client.ExecContext(ctx, `CREATE TRIGGER fail_coupon_import BEFORE INSERT ON recharge_discount_coupons BEGIN SELECT RAISE(ABORT, 'test write failure'); END`)
	require.NoError(t, err)
	result, err := admin.ImportDiscountCoupons(ctx, ImportDiscountCouponsInput{CreatedBy: 99, Rows: []DiscountCouponImportRow{
		{RowNumber: 2, UserID: user.ID, CouponType: "subscription", MinAmount: 20, DiscountRate: 8, TotalUses: 1},
		{RowNumber: 3, UserID: user.ID, CouponType: "recharge", MinAmount: 100, DiscountRate: 8, TotalUses: 1},
	}})
	require.Error(t, err)
	require.Nil(t, result)
	coupons, err := admin.ListUserSubscriptionDiscountCoupons(ctx, user.ID)
	require.NoError(t, err)
	require.Empty(t, coupons)
}

func TestDiscountCouponImportValidation(t *testing.T) {
	valid := DiscountCouponImportRow{RowNumber: 2, UserID: 1, CouponType: "recharge", MinAmount: 100, DiscountRate: 8, TotalUses: 1}
	for _, test := range []struct {
		name string
		edit func(*DiscountCouponImportRow)
	}{
		{"row", func(row *DiscountCouponImportRow) { row.RowNumber = 1 }},
		{"identity", func(row *DiscountCouponImportRow) { row.UserID = 0 }},
		{"unsafe ID", func(row *DiscountCouponImportRow) { row.UserID = 9007199254740992 }},
		{"email", func(row *DiscountCouponImportRow) { row.Email = "User <user@example.com>" }},
		{"type", func(row *DiscountCouponImportRow) { row.CouponType = "promo" }},
		{"minimum", func(row *DiscountCouponImportRow) { row.MinAmount = 0 }},
		{"minimum overflow", func(row *DiscountCouponImportRow) { row.MinAmount = 1e12 }},
		{"NaN", func(row *DiscountCouponImportRow) { row.MinAmount = math.NaN() }},
		{"rate", func(row *DiscountCouponImportRow) { row.DiscountRate = 0 }},
		{"infinite rate", func(row *DiscountCouponImportRow) { row.DiscountRate = math.Inf(1) }},
		{"uses", func(row *DiscountCouponImportRow) { row.TotalUses = 0 }},
		{"notes", func(row *DiscountCouponImportRow) { row.Notes = strings.Repeat("券", 2001) }},
	} {
		t.Run(test.name, func(t *testing.T) {
			row := valid
			test.edit(&row)
			require.Error(t, validateDiscountCouponImportRow(row))
		})
	}
	_, _, admin, _ := subscriptionCouponFixture(t)
	for _, rows := range [][]DiscountCouponImportRow{nil, make([]DiscountCouponImportRow, 501)} {
		_, err := admin.ImportDiscountCoupons(context.Background(), ImportDiscountCouponsInput{Rows: rows, CreatedBy: 99})
		require.Equal(t, "INVALID_COUPON_IMPORT_SIZE", infraerrors.Reason(err))
	}
}

func TestDiscountCouponImportMaximumBatchFitsIdempotencyReplay(t *testing.T) {
	ctx := context.Background()
	_, user, admin, _ := subscriptionCouponFixture(t)
	rows := make([]DiscountCouponImportRow, MaxDiscountCouponImportRows)
	for i := range rows {
		rows[i] = DiscountCouponImportRow{RowNumber: i + 2, UserID: user.ID,
			CouponType: "subscription", MinAmount: 20, DiscountRate: 8, TotalUses: 1}
	}
	result, err := admin.ImportDiscountCoupons(ctx, ImportDiscountCouponsInput{Rows: rows, CreatedBy: 99})
	require.NoError(t, err)
	require.Equal(t, 500, result.IssuedCount)
	body, err := json.Marshal(result)
	require.NoError(t, err)
	require.Less(t, len(body), DefaultIdempotencyConfig().MaxStoredResponseLen)
	require.Empty(t, result.Rows[0].Email)
	require.Positive(t, result.Rows[0].CouponID)
}
