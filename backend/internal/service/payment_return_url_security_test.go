//go:build unit

package service

import (
	"net/url"
	"testing"
)

func TestPaymentReturnURLDropsUserParameters(t *testing.T) {
	for _, raw := range []string{
		"https://site.example.com/payment/result?trade_status=TRADE_SUCCESS&order_id=999&resume_token=attacker#fragment",
		"https://site.example.com/payment/result?%74rade_status=TRADE_SUCCESS",
		"https://site.example.com/payment/result?",
	} {
		t.Run(raw, func(t *testing.T) {
			canonical, err := CanonicalizeReturnURL(raw, "site.example.com", "")
			if err != nil || canonical != "https://site.example.com/payment/result" {
				t.Errorf("canonical = %q, err=%v", canonical, err)
			}
			// Also cover return URLs persisted before query sanitization.
			built, err := buildPaymentReturnURL(raw, 99, "ORDER123", "server-token")
			if err != nil {
				t.Fatal(err)
			}
			parsed, err := url.Parse(built)
			if err != nil {
				t.Fatal(err)
			}
			want := url.Values{"order_id": {"99"}, "out_trade_no": {"ORDER123"}, "resume_token": {"server-token"}, "status": {"success"}}
			if parsed.RawQuery != want.Encode() || parsed.Fragment != "" {
				t.Errorf("built return URL = %q", built)
			}
		})
	}
}
