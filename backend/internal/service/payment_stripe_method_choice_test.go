//go:build unit

package service

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"testing"

	"github.com/Wei-Shaw/sub2api/ent/paymentauditlog"
	"github.com/Wei-Shaw/sub2api/internal/payment"
	"github.com/stretchr/testify/require"
	stripe "github.com/stripe/stripe-go/v85"
)

// Use a real database, order quotation, load balancer and Stripe SDK encoding.
// An older purchase page may still submit its default Alipay subtype after a
// deployment; the resulting hosted Session must offer all equal-fee methods.
func TestCreateStripeOrderPreservesCheckoutMethodChoice(t *testing.T) {
	for _, tc := range []struct {
		name, mode, subtype, wantSubtype string
		rate                             float64
		methodFees                       map[string]float64
		wantMethods                      []string
	}{
		{"legacy_alipay", "hosted", "alipay", "", 0, nil, []string{"card", "alipay", "wechat_pay"}},
		{"explicit_wechat_equal_fee", "hosted", "wxpay", "", 3, nil, []string{"card", "alipay", "wechat_pay"}},
		{"unspecified", "hosted", "", "", 0, nil, []string{"card", "alipay", "wechat_pay"}},
		{"different_fees", "hosted", "alipay", "alipay", 0, map[string]float64{"card": 3, "wxpay": 1}, []string{"alipay"}},
		{"explicit_zero_override", "hosted", "alipay", "alipay", 3, map[string]float64{"alipay": 0}, []string{"alipay"}},
		{"embedded_selection", "embedded", "alipay", "alipay", 0, nil, []string{"alipay"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()
			client := newPaymentConfigServiceTestClient(t)
			type observedRequest struct {
				path string
				form url.Values
			}
			requests := make(chan observedRequest, 2)
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if err := r.ParseForm(); err != nil {
					http.Error(w, "invalid test form", http.StatusBadRequest)
					return
				}
				requests <- observedRequest{r.URL.Path, r.PostForm}
				w.Header().Set("Content-Type", "application/json")
				if tc.mode == "hosted" {
					_, _ = w.Write([]byte(`{"id":"cs_method_choice","object":"checkout.session","url":"https://checkout.stripe.com/c/pay/cs_method_choice"}`))
				} else {
					_, _ = w.Write([]byte(`{"id":"pi_method_choice","object":"payment_intent","client_secret":"test_client_secret"}`))
				}
			}))
			t.Cleanup(server.Close)
			previousBackend := stripe.GetBackend(stripe.APIBackend)
			stripe.SetBackend(stripe.APIBackend, stripe.GetBackendWithConfig(stripe.APIBackend, &stripe.BackendConfig{
				URL: stripe.String(server.URL), EnableTelemetry: stripe.Bool(false), MaxNetworkRetries: stripe.Int64(0),
			}))
			t.Cleanup(func() { stripe.SetBackend(stripe.APIBackend, previousBackend) })

			config, err := json.Marshal(map[string]string{"secretKey": "sk_test_method_choice", "currency": "CNY", "checkoutMode": tc.mode})
			require.NoError(t, err)
			_, err = client.PaymentProviderInstance.Create().SetProviderKey(payment.TypeStripe).SetName("Stripe").
				SetConfig(string(config)).SetSupportedTypes("card,alipay,wxpay").SetFeeRate(tc.rate).SetMethodFeeRates(tc.methodFees).Save(ctx)
			require.NoError(t, err)
			user, err := client.User.Create().SetEmail("choice@example.com").SetUsername("choice").SetPasswordHash("hash").Save(ctx)
			require.NoError(t, err)
			settings := &paymentFulfillmentSettingRepoStub{values: map[string]string{SettingPaymentEnabled: "true", SettingRechargeFeeRate: "9"}}
			svc := &PaymentService{entClient: client, configService: NewPaymentConfigService(client, settings, nil),
				loadBalancer: payment.NewDefaultLoadBalancer(client, nil), registry: payment.NewRegistry(),
				userRepo: &mockUserRepo{getByIDUser: &User{ID: user.ID, Email: user.Email, Username: user.Username, Status: StatusActive}}}
			result, err := svc.CreateOrder(ctx, CreateOrderRequest{UserID: user.ID, Amount: 100, PaymentType: payment.TypeStripe,
				PaymentSubtype: tc.subtype, ReturnURL: "https://merchant.example/payment/result", SrcHost: "merchant.example"})
			require.NoError(t, err)
			var sent observedRequest
			select {
			case sent = <-requests:
			default:
				t.Fatal("order creation did not send a Stripe request")
			}
			require.Empty(t, requests, "create exactly one Stripe payment")
			var methods []string
			for i := 0; ; i++ {
				method := sent.form.Get(fmt.Sprintf("payment_method_types[%d]", i))
				if method == "" {
					break
				}
				methods = append(methods, method)
			}
			require.Equal(t, tc.wantMethods, methods, "check the actual form sent by the SDK")
			wantRate := tc.rate
			if override, ok := tc.methodFees[tc.subtype]; ok {
				wantRate = override
			}
			amountField := "amount"
			if tc.mode == "hosted" {
				require.Equal(t, "/v1/checkout/sessions", sent.path)
				amountField = "line_items[0][price_data][unit_amount]"
			} else {
				require.Equal(t, "/v1/payment_intents", sent.path)
			}
			require.Equal(t, strconv.Itoa(10000+int(wantRate*100)), sent.form.Get(amountField))
			require.Equal(t, tc.wantSubtype, result.PaymentSubtype)
			require.Equal(t, wantRate, result.FeeRate)
			order, err := client.PaymentOrder.Get(ctx, result.OrderID)
			require.NoError(t, err)
			require.Equal(t, tc.wantSubtype, PaymentOrderSubtype(order), "persist the effective method selection for later resumes")
			audit, err := client.PaymentAuditLog.Query().Where(paymentauditlog.OrderIDEQ(strconv.FormatInt(order.ID, 10)), paymentauditlog.ActionEQ("ORDER_CREATED")).Only(ctx)
			require.NoError(t, err)
			var detail map[string]any
			require.NoError(t, json.Unmarshal([]byte(audit.Detail), &detail))
			require.Equal(t, tc.mode, detail["checkoutMode"])
			require.Equal(t, tc.wantSubtype, detail["paymentSubtype"])
			require.Equal(t, "card,alipay,wxpay", detail["supportedPaymentTypes"])
			require.NotEmpty(t, detail["providerTradeNo"])
			require.NotContains(t, audit.Detail, "sk_test")
			require.NotContains(t, audit.Detail, "test_client_secret")
		})
	}
}
