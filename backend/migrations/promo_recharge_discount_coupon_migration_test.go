package migrations

import (
	"strings"
	"testing"
)

func TestPromoRechargeDiscountCouponMigration(t *testing.T) {
	raw, err := FS.ReadFile("238_promo_recharge_discounts_to_coupons.sql")
	if err != nil {
		t.Fatal(err)
	}
	sql := string(raw)
	for _, required := range []string{
		"CHECK (min_recharge_amount >= 0)",
		"CHECK (total_uses >= 0)",
		"source_type IN ('admin', 'promo_code')",
		"idx_recharge_discount_coupons_promo_source",
		"FROM promo_code_usages pcu",
		"JOIN promo_codes pc ON pc.id = pcu.promo_code_id",
		"pc.first_recharge_discount_times",
		"'promo_code'",
		"ON CONFLICT (user_id, source_type, source_id)",
		"DO NOTHING",
	} {
		if !strings.Contains(sql, required) {
			t.Fatalf("migration missing %q", required)
		}
	}
}
