//go:build unit

package provider

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/payment"
	"github.com/stretchr/testify/require"
	stripe "github.com/stripe/stripe-go/v85"
	"github.com/stripe/stripe-go/v85/webhook"
)

type stripeCheckoutBackend struct {
	stripeRefundBackend
	call    func(string, string, stripe.ParamsContainer, stripe.LastResponseSetter) error
	callRaw func(string, string, []byte, stripe.LastResponseSetter) error
}

func (b *stripeCheckoutBackend) Call(method, path, _ string, params stripe.ParamsContainer, result stripe.LastResponseSetter) error {
	return b.call(method, path, params, result)
}

func (b *stripeCheckoutBackend) CallRaw(method, path, _ string, body []byte, _ *stripe.Params, result stripe.LastResponseSetter) error {
	return b.callRaw(method, path, body, result)
}

func newStripeCheckoutTestProvider(backend *stripeCheckoutBackend, mode string) *Stripe {
	return &Stripe{config: map[string]string{"currency": "HKD", "checkoutMode": mode, "webhookSecret": "whsec_test"},
		initialized: true, sc: stripe.NewClient("sk_test", stripe.WithBackends(&stripe.Backends{API: backend}))}
}

func TestStripeCheckoutModeValidationAndDefaults(t *testing.T) {
	for _, tc := range []struct{ input, want string }{{"", "embedded"}, {"embedded", "embedded"}, {"hosted", "hosted"}, {" hosted ", "hosted"}} {
		config := map[string]string{"secretKey": "sk_test", "checkoutMode": tc.input}
		provider, err := NewStripe("test", config)
		require.NoError(t, err)
		require.Equal(t, tc.want, provider.config["checkoutMode"])
		require.Equal(t, tc.input, config["checkoutMode"], "constructor must not mutate stored config")
	}
	_, err := NewStripe("test", map[string]string{"secretKey": "sk_test", "checkoutMode": "invalid"})
	require.ErrorContains(t, err, "checkoutMode")
}

func TestStripeHostedCheckoutLocksAmountMethodAndReturnURL(t *testing.T) {
	for _, subtype := range []string{"card", "alipay", "wxpay"} {
		t.Run(subtype, func(t *testing.T) {
			var calls int
			var first *stripe.CheckoutSessionCreateParams
			backend := &stripeCheckoutBackend{call: func(method, path string, raw stripe.ParamsContainer, result stripe.LastResponseSetter) error {
				calls++
				require.Equal(t, http.MethodPost, method)
				require.Equal(t, "/v1/checkout/sessions", path)
				params := raw.(*stripe.CheckoutSessionCreateParams)
				require.Equal(t, "hosted_page", *params.UIMode)
				require.Equal(t, "payment", *params.Mode)
				require.Equal(t, "order-hosted", *params.ClientReferenceID)
				require.Equal(t, "cs-order-hosted", *params.IdempotencyKey)
				require.Equal(t, "https://merchant.example/payment/result?order_id=42&resume_token=signed", *params.SuccessURL)
				require.Equal(t, params.SuccessURL, params.CancelURL)
				require.Equal(t, "order-hosted", params.Metadata["orderId"])
				require.Equal(t, "hosted", params.PaymentIntentData.Metadata["checkoutMode"])
				require.Equal(t, params.Metadata, params.PaymentIntentData.Metadata)
				require.Len(t, params.PaymentMethodTypes, 1)
				methodType := subtype
				if subtype == "wxpay" {
					methodType = "wechat_pay"
					require.Equal(t, "web", *params.PaymentMethodOptions.WeChatPay.Client)
				}
				require.Equal(t, methodType, *params.PaymentMethodTypes[0])
				require.Len(t, params.LineItems, 1)
				require.EqualValues(t, 1, *params.LineItems[0].Quantity)
				require.EqualValues(t, 10250, *params.LineItems[0].PriceData.UnitAmount)
				require.Equal(t, "hkd", *params.LineItems[0].PriceData.Currency)
				require.Equal(t, "Balance recharge", *params.LineItems[0].PriceData.ProductData.Name)
				if first != nil {
					require.Equal(t, first.LineItems, params.LineItems)
					require.Equal(t, first.ExpiresAt, params.ExpiresAt)
				}
				first = params
				*result.(*stripe.CheckoutSession) = stripe.CheckoutSession{ID: "cs_hosted", URL: "https://checkout.stripe.com/c/pay/cs_hosted"}
				return nil
			}}
			provider := newStripeCheckoutTestProvider(backend, "hosted")
			for range 2 {
				response, err := provider.CreatePayment(context.Background(), payment.CreatePaymentRequest{
					OrderID: "order-hosted", Amount: "102.50", Subject: "Balance recharge", PaymentType: "stripe", PaymentSubtype: subtype,
					InstanceSubMethods: "card,link,alipay,wxpay", ReturnURL: "https://merchant.example/payment/result?order_id=42&resume_token=signed",
				})
				require.NoError(t, err)
				require.Equal(t, "cs_hosted", response.TradeNo)
				require.Equal(t, "https://checkout.stripe.com/c/pay/cs_hosted", response.PayURL)
				require.Equal(t, "HKD", response.Currency)
				require.Empty(t, response.ClientSecret)
			}
			require.Equal(t, 2, calls)
		})
	}
}

func TestStripeHostedCheckoutRejectsInvalidInputsBeforeAPI(t *testing.T) {
	backend := &stripeCheckoutBackend{call: func(string, string, stripe.ParamsContainer, stripe.LastResponseSetter) error {
		t.Fatal("invalid checkout must not reach Stripe")
		return nil
	}}
	provider := newStripeCheckoutTestProvider(backend, "hosted")
	for _, req := range []payment.CreatePaymentRequest{
		{ReturnURL: "", Amount: "100"}, {ReturnURL: "javascript:alert(1)", Amount: "100"},
		{ReturnURL: "https://user:pass@merchant.example/payment/result", Amount: "100"},
		{ReturnURL: "https://merchant.example/payment/result", Amount: "bad"},
		{ReturnURL: "https://merchant.example/payment/result", Amount: "100", PaymentSubtype: "wxpay", InstanceSubMethods: "card"},
		{ReturnURL: "https://merchant.example/payment/result", Amount: "100", PaymentSubtype: "link", InstanceSubMethods: "card,link"},
		{ReturnURL: "https://merchant.example/payment/result", Amount: "100", InstanceSubMethods: "card,link"},
	} {
		_, err := provider.CreatePayment(context.Background(), req)
		require.Error(t, err)
	}
}

func TestStripeCheckoutQueryUsesStoredTransactionAfterModeChange(t *testing.T) {
	for _, tc := range []struct{ status, paid, want string }{
		{"open", "unpaid", payment.ProviderStatusPending}, {"complete", "unpaid", payment.ProviderStatusPending},
		{"complete", "paid", payment.ProviderStatusPaid}, {"expired", "unpaid", payment.ProviderStatusFailed},
	} {
		t.Run(tc.status+"/"+tc.paid, func(t *testing.T) {
			backend := &stripeCheckoutBackend{call: func(method, path string, _ stripe.ParamsContainer, result stripe.LastResponseSetter) error {
				require.Equal(t, http.MethodGet, method)
				require.Equal(t, "/v1/checkout/sessions/cs_original", path)
				*result.(*stripe.CheckoutSession) = stripe.CheckoutSession{ID: "cs_original", AmountTotal: 1234, Currency: "hkd", Status: stripe.CheckoutSessionStatus(tc.status), PaymentStatus: stripe.CheckoutSessionPaymentStatus(tc.paid)}
				return nil
			}}
			result, err := newStripeCheckoutTestProvider(backend, "embedded").QueryOrder(context.Background(), "cs_original")
			require.NoError(t, err)
			require.Equal(t, tc.want, result.Status)
			require.Equal(t, "cs_original", result.TradeNo)
			require.Equal(t, 12.34, result.Amount)
			require.Equal(t, "HKD", result.Metadata["currency"])
		})
	}
	backend := &stripeCheckoutBackend{call: func(_, path string, _ stripe.ParamsContainer, result stripe.LastResponseSetter) error {
		require.Equal(t, "/v1/payment_intents/pi_original", path)
		*result.(*stripe.PaymentIntent) = stripe.PaymentIntent{ID: "pi_original", Status: stripe.PaymentIntentStatusSucceeded, Currency: "hkd", Amount: 100}
		return nil
	}}
	result, err := newStripeCheckoutTestProvider(backend, "hosted").QueryOrder(context.Background(), "pi_original")
	require.NoError(t, err)
	require.Equal(t, payment.ProviderStatusPaid, result.Status)
}

func TestStripeCheckoutCancellationExpiresOnlyOpenSessions(t *testing.T) {
	for _, status := range []stripe.CheckoutSessionStatus{stripe.CheckoutSessionStatusOpen, stripe.CheckoutSessionStatusExpired, stripe.CheckoutSessionStatusComplete} {
		t.Run(string(status), func(t *testing.T) {
			calls := 0
			backend := &stripeCheckoutBackend{call: func(method, path string, _ stripe.ParamsContainer, result stripe.LastResponseSetter) error {
				calls++
				if calls == 1 {
					require.Equal(t, http.MethodGet, method)
					require.Equal(t, "/v1/checkout/sessions/cs_order", path)
					*result.(*stripe.CheckoutSession) = stripe.CheckoutSession{ID: "cs_order", Status: status}
				} else {
					require.Equal(t, http.MethodPost, method)
					require.Equal(t, "/v1/checkout/sessions/cs_order/expire", path)
				}
				return nil
			}}
			err := newStripeCheckoutTestProvider(backend, "embedded").CancelPayment(context.Background(), "cs_order")
			if status == stripe.CheckoutSessionStatusComplete {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
			if status == stripe.CheckoutSessionStatusOpen {
				require.Equal(t, 2, calls)
			} else {
				require.Equal(t, 1, calls)
			}
		})
	}
}

func TestStripeCheckoutRefundResolvesPaymentIntent(t *testing.T) {
	backend := &stripeCheckoutBackend{call: func(method, path string, params stripe.ParamsContainer, result stripe.LastResponseSetter) error {
		switch path {
		case "/v1/checkout/sessions/cs_order":
			require.Equal(t, http.MethodGet, method)
			*result.(*stripe.CheckoutSession) = stripe.CheckoutSession{PaymentIntent: &stripe.PaymentIntent{ID: "pi_checkout"}}
		case "/v1/refunds":
			{
				require.Equal(t, http.MethodPost, method)
				request := params.(*stripe.RefundCreateParams)
				require.Equal(t, "pi_checkout", *request.PaymentIntent)
				require.EqualValues(t, 1234, *request.Amount)
				*result.(*stripe.Refund) = stripe.Refund{ID: "re_checkout", Status: stripe.RefundStatusSucceeded}
			}
		default:
			t.Fatalf("unexpected Stripe path: %s", path)
		}
		return nil
	}, callRaw: func(method, path string, raw []byte, result stripe.LastResponseSetter) error {
		require.Equal(t, http.MethodGet, method)
		require.Equal(t, "/v1/refunds", path)
		params, err := url.ParseQuery(string(raw))
		require.NoError(t, err)
		require.Equal(t, "pi_checkout", params.Get("payment_intent"))
		return json.Unmarshal([]byte(`{"object":"list","data":[{"id":"re_checkout","status":"succeeded"}],"has_more":false}`), result)
	}}
	provider := newStripeCheckoutTestProvider(backend, "embedded")
	result, err := provider.Refund(context.Background(), payment.RefundRequest{TradeNo: "cs_order", OrderID: "order", Amount: "12.34"})
	require.NoError(t, err)
	require.Equal(t, "re_checkout", result.RefundID)
	queried, err := provider.QueryRefund(context.Background(), payment.RefundQueryRequest{TradeNo: "cs_order"})
	require.NoError(t, err)
	require.Equal(t, "re_checkout", queried.RefundID)
	require.Equal(t, payment.ProviderStatusSuccess, queried.Status)
}

func signedStripeCheckoutEvent(t *testing.T, eventType string, object map[string]any) (string, map[string]string) {
	t.Helper()
	body, err := json.Marshal(map[string]any{"id": "evt_test", "object": "event", "api_version": stripe.APIVersion, "type": eventType, "data": map[string]any{"object": object}})
	require.NoError(t, err)
	signed := webhook.GenerateTestSignedPayload(&webhook.UnsignedPayload{Payload: body, Secret: "whsec_test"})
	return string(body), map[string]string{"stripe-signature": signed.Header}
}

func TestStripeCheckoutWebhookOnlyCreditsSettledPayments(t *testing.T) {
	provider := newStripeCheckoutTestProvider(nil, "embedded")
	for _, tc := range []struct{ event, paid, want string }{
		{stripeEventCheckoutCompleted, "paid", payment.ProviderStatusSuccess},
		{stripeEventCheckoutCompleted, "unpaid", ""},
		{stripeEventCheckoutAsyncSuccess, "paid", payment.ProviderStatusSuccess},
		{stripeEventCheckoutAsyncSuccess, "unpaid", ""},
		{stripeEventCheckoutAsyncFailed, "unpaid", payment.ProviderStatusFailed},
	} {
		t.Run(tc.event+"/"+tc.paid, func(t *testing.T) {
			body, headers := signedStripeCheckoutEvent(t, tc.event, map[string]any{
				"id": "cs_order", "payment_status": tc.paid, "amount_total": 1234, "currency": "hkd",
				"metadata": map[string]string{"orderId": "merchant-order", "checkoutMode": "hosted"},
			})
			result, err := provider.VerifyNotification(context.Background(), body, headers)
			require.NoError(t, err)
			if tc.want == "" {
				require.Nil(t, result)
				return
			}
			require.Equal(t, tc.want, result.Status)
			require.Equal(t, "cs_order", result.TradeNo)
			require.Equal(t, "merchant-order", result.OrderID)
			require.Equal(t, 12.34, result.Amount)
			require.Equal(t, "HKD", result.Metadata["currency"])
			headers["stripe-signature"] = "invalid"
			_, err = provider.VerifyNotification(context.Background(), body, headers)
			require.Error(t, err)
			_, err = provider.VerifyNotification(context.Background(), strings.Replace(body, "1234", "9999", 1), map[string]string{"stripe-signature": signedHeader(t, body)})
			require.Error(t, err, "tampered amount must fail signature verification")
		})
	}
	for _, event := range []string{stripeEventPaymentSuccess, stripeEventPaymentFailed} {
		for _, mode := range []string{"embedded", "hosted"} {
			body, headers := signedStripeCheckoutEvent(t, event, map[string]any{
				"id": "pi_order", "amount": 1234, "currency": "hkd", "metadata": map[string]string{"orderId": "merchant-order", "checkoutMode": mode},
			})
			result, err := provider.VerifyNotification(context.Background(), body, headers)
			require.NoError(t, err)
			if mode == "hosted" {
				require.Nil(t, result, "Checkout PI retries must not prematurely fail or fulfill the order")
			} else {
				require.NotNil(t, result, "existing embedded orders must keep processing after mode changes")
			}
		}
	}
}

func signedHeader(t *testing.T, body string) string {
	t.Helper()
	return webhook.GenerateTestSignedPayload(&webhook.UnsignedPayload{Payload: []byte(body), Secret: "whsec_test"}).Header
}

func TestStripeCheckoutMissingPaymentIntentCannotRefund(t *testing.T) {
	backend := &stripeCheckoutBackend{call: func(_, path string, _ stripe.ParamsContainer, _ stripe.LastResponseSetter) error {
		if path != "/v1/checkout/sessions/cs_unpaid" {
			return fmt.Errorf("refund attempted without a payment intent")
		}
		return nil
	}}
	_, err := newStripeCheckoutTestProvider(backend, "hosted").Refund(context.Background(), payment.RefundRequest{TradeNo: "cs_unpaid", OrderID: "order", Amount: "1.00"})
	require.ErrorContains(t, err, "no payment intent")
}
