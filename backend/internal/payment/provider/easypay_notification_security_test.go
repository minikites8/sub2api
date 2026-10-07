package provider

import (
	"context"
	"net/url"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/payment"
)

func TestEasyPayNotificationRejectsCheckoutSignatureReuse(t *testing.T) {
	t.Parallel()
	e := &EasyPay{config: map[string]string{
		"pid": "1000", "pkey": "test-merchant-secret",
		"apiBase": "https://pay.example.com", "paymentMode": "popup",
		"notifyUrl": "https://site.example.com/api/v1/payment/webhook/easypay",
	}}
	resp, err := e.CreatePayment(context.Background(), payment.CreatePaymentRequest{
		OrderID: "ORDER123", Amount: "650.00", Subject: "balance recharge", PaymentType: "alipay",
		ReturnURL: "https://site.example.com/payment/result?order_id=99&out_trade_no=ORDER123&status=success&trade_status=TRADE_SUCCESS",
	})
	if err != nil {
		t.Fatal(err)
	}
	checkout, err := url.Parse(resp.PayURL)
	if err != nil {
		t.Fatal(err)
	}
	for _, replay := range []bool{false, true} {
		cb := checkout.Query()
		if !replay {
			cb.Set("return_url", strings.TrimSuffix(cb.Get("return_url"), "&trade_status=TRADE_SUCCESS"))
			cb.Set("trade_status", "TRADE_SUCCESS")
		}
		if n, err := e.VerifyNotification(context.Background(), cb.Encode(), nil); err == nil {
			t.Errorf("checkout signature accepted as notification (replay=%v): %+v", replay, n)
		}
	}
	// A second collision uses only allowed names by folding checkout-only
	// fields into name and pid. It must also be rejected for pending orders
	// whose checkout URL was generated before return URL sanitization.
	cb := checkout.Query()
	cb.Set("return_url", strings.TrimSuffix(cb.Get("return_url"), "&trade_status=TRADE_SUCCESS"))
	cb.Set("trade_status", "TRADE_SUCCESS")
	cb.Set("name", cb.Get("name")+"&notify_url="+cb.Get("notify_url"))
	cb.Del("notify_url")
	cb.Set("pid", cb.Get("pid")+"&return_url="+cb.Get("return_url"))
	cb.Del("return_url")
	params := make(map[string]string, len(cb))
	for k := range cb {
		params[k] = cb.Get(k)
	}
	if !easyPayVerifySign(params, e.config["pkey"], cb.Get("sign")) {
		t.Fatal("test payload must reuse the original checkout signature")
	}
	if n, err := e.VerifyNotification(context.Background(), cb.Encode(), nil); err == nil {
		t.Fatalf("folded checkout signature accepted as notification: %+v", n)
	}
}

func TestEasyPayNotificationParameters(t *testing.T) {
	t.Parallel()
	e := &EasyPay{config: map[string]string{"pid": "1000", "pkey": "test-merchant-secret"}}
	base := map[string]string{
		"pid": "1000", "trade_no": "UPSTREAM123", "out_trade_no": "ORDER123", "type": "alipay",
		"name": "充值 & balance=650", "money": "650.00", "trade_status": "TRADE_SUCCESS", "param": "memo&value=1",
	}
	for _, tc := range []struct {
		name, extra, value, duplicate string
		wantErr                       bool
	}{
		{name: "standard"},
		{name: "processor trade number", extra: "api_trade_no", value: "PROCESSOR123"},
		{name: "return URL", extra: "return_url", value: "https://site.example.com/payment/result", wantErr: true},
		{name: "notify URL", extra: "notify_url", value: "https://site.example.com/notify", wantErr: true},
		{name: "empty unknown", extra: "device", wantErr: true},
		{name: "unknown", extra: "unknown", value: "value", wantErr: true},
		{name: "duplicate money", duplicate: "money", value: "1.00", wantErr: true},
		{name: "duplicate sign", duplicate: "sign", value: "ignored", wantErr: true},
		{name: "duplicate status", duplicate: "trade_status", value: "TRADE_SUCCESS", wantErr: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			params := cloneStringMap(base)
			if tc.extra != "" {
				params[tc.extra] = tc.value
			}
			cb := url.Values{}
			for k, v := range params {
				cb.Set(k, v)
			}
			cb.Set("sign", easyPaySign(params, e.config["pkey"]))
			cb.Set("sign_type", "MD5")
			if tc.duplicate != "" {
				cb.Add(tc.duplicate, tc.value)
			}
			n, err := e.VerifyNotification(context.Background(), cb.Encode(), nil)
			if tc.wantErr {
				if err == nil || n != nil {
					t.Fatalf("invalid notification accepted: %+v, err=%v", n, err)
				}
				return
			}
			if err != nil || n.Status != payment.ProviderStatusSuccess || n.OrderID != "ORDER123" || n.Amount != 650 || n.TradeNo != "UPSTREAM123" {
				t.Fatalf("standard notification = %+v, err=%v", n, err)
			}
		})
	}
}
