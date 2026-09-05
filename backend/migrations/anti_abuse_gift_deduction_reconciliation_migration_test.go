package migrations

import (
	"strings"
	"testing"
)

func TestAntiAbuseGiftDeductionReconciliationMigration(t *testing.T) {
	raw, err := FS.ReadFile("239_reconcile_anti_abuse_gift_deductions.sql")
	if err != nil {
		t.Fatal(err)
	}
	sql := string(raw)
	for _, required := range []string{
		"r.type = 'risk_control_balance'",
		"r.value < 0",
		"ABS(r.value) AS amount",
		"'risk_control_gift_deduction'",
		"e.gift_balance_deducted = d.amount",
		"INTERVAL '30 seconds'",
	} {
		if !strings.Contains(sql, required) {
			t.Fatalf("migration missing %q", required)
		}
	}
}
