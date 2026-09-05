package repository

import (
	"context"
	"database/sql"
	"fmt"
	"testing"
	"time"

	"entgo.io/ent/dialect"
	entsql "entgo.io/ent/dialect/sql"
	_ "modernc.org/sqlite"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/ent/enttest"
	"github.com/Wei-Shaw/sub2api/internal/payment"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func TestPromoCodeRechargeStatsIncludesSourcedCouponOrders(t *testing.T) {
	ctx := context.Background()
	dsn := fmt.Sprintf("file:promo_coupon_stats_%d?mode=memory&cache=shared&_fk=1", time.Now().UnixNano())
	db, err := sql.Open("sqlite", dsn)
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	_, err = db.Exec("PRAGMA foreign_keys = ON")
	require.NoError(t, err)
	driver := entsql.OpenDB(dialect.SQLite, db)
	client := enttest.NewClient(t, enttest.WithOptions(dbent.Driver(driver)))
	t.Cleanup(func() { _ = client.Close() })

	user, err := client.User.Create().
		SetEmail("promo-coupon-stats@example.com").
		SetPasswordHash("hash").
		SetUsername("promo-coupon-stats").
		Save(ctx)
	require.NoError(t, err)
	promoCodeID := int64(9)
	_, err = client.PaymentOrder.Create().
		SetUserID(user.ID).
		SetUserEmail(user.Email).
		SetUserName(user.Username).
		SetAmount(100).
		SetPayAmount(80).
		SetFeeRate(0).
		SetRechargeCode("PAY-PROMO-COUPON-STATS").
		SetOutTradeNo("promo-coupon-stats-order").
		SetPaymentType(payment.TypeAlipay).
		SetPaymentTradeNo("").
		SetOrderType(payment.OrderTypeBalance).
		SetStatus(service.OrderStatusCompleted).
		SetExpiresAt(time.Now().Add(time.Hour)).
		SetClientIP("127.0.0.1").
		SetSrcHost("example.com").
		SetProviderSnapshot(map[string]any{
			"recharge_discount_coupon": map[string]any{
				"coupon_id":        17,
				"source_type":      "promo_code",
				"source_id":        promoCodeID,
				"source_code":      "PRICEAI",
				"base_amount":      100,
				"discount_percent": 80,
				"credited_amount":  100,
				"payment_amount":   80,
			},
		}).
		Save(ctx)
	require.NoError(t, err)

	stats, err := (&promoCodeRepository{client: client}).ListRechargeStatsByPromoCodeIDs(ctx, []int64{promoCodeID})

	require.NoError(t, err)
	require.Equal(t, 1, stats[promoCodeID].OrderCount)
	require.Equal(t, 1, stats[promoCodeID].RechargedUserCount)
	require.Equal(t, 80.0, stats[promoCodeID].TotalPayAmount)
	require.Equal(t, 100.0, stats[promoCodeID].TotalRechargeAmount)
}
