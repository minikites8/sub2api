package migrations

import (
	"strings"
	"testing"
)

func TestSubscriptionPromoCouponMigration(t *testing.T) {
	raw, err := FS.ReadFile("239_subscription_promo_coupon.sql")
	if err != nil {
		t.Fatal(err)
	}
	sql := string(raw)
	for _, required := range []string{
		"ADD COLUMN IF NOT EXISTS coupon_type",
		"DEFAULT 'registration'",
		"subscription_discount_percent",
		"promo_codes_coupon_type_valid",
		"'subscription'",
		"promo_codes_subscription_discount_range",
		"idx_promo_codes_coupon_type",
	} {
		if !strings.Contains(sql, required) {
			t.Fatalf("migration missing %q", required)
		}
	}
}
