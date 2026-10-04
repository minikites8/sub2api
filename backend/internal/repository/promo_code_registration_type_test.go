package repository

import (
	"context"
	"database/sql"
	"fmt"
	"testing"
	"time"

	"entgo.io/ent/dialect"
	entsql "entgo.io/ent/dialect/sql"
	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/ent/enttest"
	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
	_ "modernc.org/sqlite"
)

func TestPromoCodeListReturnsRegistrationCodesWithPagination(t *testing.T) {
	ctx := context.Background()
	db, err := sql.Open("sqlite", fmt.Sprintf("file:promo_registration_%d?mode=memory&cache=shared", time.Now().UnixNano()))
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	_, err = db.Exec("PRAGMA foreign_keys = ON")
	require.NoError(t, err)
	client := enttest.NewClient(t, enttest.WithOptions(dbent.Driver(entsql.OpenDB(dialect.SQLite, db))))
	t.Cleanup(func() { _ = client.Close() })
	for _, column := range []string{
		"coupon_type VARCHAR(20) NOT NULL DEFAULT 'registration'",
		"subscription_discount_percent DECIMAL(5,2)",
		"first_recharge_bonus_amount DECIMAL(20,8)",
		"first_recharge_discount_percent DECIMAL(5,2)",
		"first_recharge_discount_times INTEGER DEFAULT 1",
	} {
		_, err := client.ExecContext(ctx, "ALTER TABLE promo_codes ADD COLUMN "+column)
		require.NoError(t, err)
	}
	_, err = client.PromoCode.Create().SetCode("WELCOME").Save(ctx)
	require.NoError(t, err)
	legacy, err := client.PromoCode.Create().SetCode("SUB80").Save(ctx)
	require.NoError(t, err)
	_, err = client.ExecContext(ctx, "UPDATE promo_codes SET coupon_type = 'subscription', subscription_discount_percent = 80 WHERE id = $1", legacy.ID)
	require.NoError(t, err)
	repo := &promoCodeRepository{client: client}
	codes, page, err := repo.List(ctx, pagination.PaginationParams{Page: 1, PageSize: 1})
	require.NoError(t, err)
	require.Len(t, codes, 1)
	require.Equal(t, int64(1), page.Total)
	require.Equal(t, "WELCOME", codes[0].Code)
	require.Equal(t, service.PromoCodeTypeRegistration, codes[0].CouponType)
	codes, page, err = repo.ListWithFilters(ctx, pagination.PaginationParams{Page: 1, PageSize: 10}, "active", "SUB")
	require.NoError(t, err)
	require.Empty(t, codes)
	require.Zero(t, page.Total)
	stored, err := repo.GetByID(ctx, legacy.ID)
	require.NoError(t, err)
	require.Equal(t, service.PromoCodeTypeSubscription, stored.CouponType)
}
