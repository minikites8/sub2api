package provider

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/payment"
	stripe "github.com/stripe/stripe-go/v85"
)

const (
	stripeCheckoutEmbedded          = "embedded"
	stripeCheckoutHosted            = "hosted"
	stripeEventCheckoutCompleted    = "checkout.session.completed"
	stripeEventCheckoutAsyncSuccess = "checkout.session.async_payment_succeeded"
	stripeEventCheckoutAsyncFailed  = "checkout.session.async_payment_failed"
)

func (s *Stripe) createCheckoutSession(ctx context.Context, req payment.CreatePaymentRequest) (*payment.CreatePaymentResponse, error) {
	returnURL, err := url.Parse(strings.TrimSpace(req.ReturnURL))
	if err != nil || returnURL.Host == "" || returnURL.User != nil || (returnURL.Scheme != "https" && returnURL.Scheme != "http") {
		return nil, fmt.Errorf("stripe hosted checkout requires an absolute http/https return URL")
	}
	currency := s.currency()
	amount, err := payment.AmountToMinorUnit(req.Amount, currency)
	if err != nil {
		return nil, fmt.Errorf("stripe create checkout: %w", err)
	}
	methods, err := resolveStripeOrderMethodTypes(req)
	if err != nil {
		return nil, err
	}
	// Checkout offers Link alongside cards; a standalone Link selection is not
	// supported. Keep the card fee locked instead of silently enabling cards
	// for an order quoted with a separate Link fee.
	if hasStripeMethod(methods, "link") && !hasStripeMethod(methods, "card") {
		return nil, fmt.Errorf("stripe hosted checkout offers Link through the card payment method; select card instead")
	}
	pmTypes := make([]*string, len(methods))
	for i, method := range methods {
		pmTypes[i] = stripe.String(method)
	}
	metadata := map[string]string{"orderId": req.OrderID, "checkoutMode": stripeCheckoutHosted}
	params := &stripe.CheckoutSessionCreateParams{
		Mode:               stripe.String(string(stripe.CheckoutSessionModePayment)),
		UIMode:             stripe.String(string(stripe.CheckoutSessionUIModeHostedPage)),
		ClientReferenceID:  stripe.String(req.OrderID),
		SuccessURL:         stripe.String(returnURL.String()),
		CancelURL:          stripe.String(returnURL.String()),
		PaymentMethodTypes: pmTypes,
		Metadata:           metadata,
		PaymentIntentData: &stripe.CheckoutSessionCreatePaymentIntentDataParams{
			Description: stripe.String(req.Subject), Metadata: metadata,
		},
		LineItems: []*stripe.CheckoutSessionCreateLineItemParams{{
			Quantity: stripe.Int64(1),
			PriceData: &stripe.CheckoutSessionCreateLineItemPriceDataParams{
				Currency: stripe.String(strings.ToLower(currency)), UnitAmount: stripe.Int64(amount),
				ProductData: &stripe.CheckoutSessionCreateLineItemPriceDataProductDataParams{Name: stripe.String(req.Subject)},
			},
		}},
	}
	if hasStripeMethod(methods, "wechat_pay") {
		params.PaymentMethodOptions = &stripe.CheckoutSessionCreatePaymentMethodOptionsParams{
			WeChatPay: &stripe.CheckoutSessionCreatePaymentMethodOptionsWeChatPayParams{Client: stripe.String("web")},
		}
	}
	// Keep parameters stable on retries. Local cancellation/expiry explicitly
	// expires the Session through CancelPayment, including deadlines under 30m.
	params.SetIdempotencyKey("cs-" + req.OrderID)
	session, err := s.sc.V1CheckoutSessions.Create(ctx, params)
	if err != nil {
		return nil, fmt.Errorf("stripe create checkout: %w", err)
	}
	if session.ID == "" || session.URL == "" {
		return nil, fmt.Errorf("stripe create checkout returned no session ID or payment URL")
	}
	return &payment.CreatePaymentResponse{TradeNo: session.ID, PayURL: session.URL, Currency: currency}, nil
}

func (s *Stripe) queryCheckoutSession(ctx context.Context, id string) (*payment.QueryOrderResponse, error) {
	session, err := s.sc.V1CheckoutSessions.Retrieve(ctx, id, nil)
	if err != nil {
		return nil, fmt.Errorf("stripe query checkout: %w", err)
	}
	status := payment.ProviderStatusPending
	if session.PaymentStatus == stripe.CheckoutSessionPaymentStatusPaid {
		status = payment.ProviderStatusPaid
	} else if session.Status == stripe.CheckoutSessionStatusExpired {
		status = payment.ProviderStatusFailed
	}
	currency := stripeIntentCurrency(session.Currency, s.currency())
	return &payment.QueryOrderResponse{
		TradeNo: session.ID, Status: status, Amount: payment.MinorUnitToAmount(session.AmountTotal, currency),
		Metadata: map[string]string{"currency": currency},
	}, nil
}

// Resolve from the stored transaction ID, so changing checkoutMode cannot
// break queries, cancellations or refunds for orders created in either mode.
func (s *Stripe) checkoutPaymentIntentID(ctx context.Context, tradeNo string) (string, error) {
	if !strings.HasPrefix(tradeNo, "cs_") {
		return tradeNo, nil
	}
	session, err := s.sc.V1CheckoutSessions.Retrieve(ctx, tradeNo, nil)
	if err != nil {
		return "", fmt.Errorf("stripe retrieve checkout payment intent: %w", err)
	}
	if session.PaymentIntent == nil || session.PaymentIntent.ID == "" {
		return "", fmt.Errorf("stripe checkout has no payment intent")
	}
	return session.PaymentIntent.ID, nil
}

func (s *Stripe) cancelCheckoutSession(ctx context.Context, id string) error {
	session, err := s.sc.V1CheckoutSessions.Retrieve(ctx, id, nil)
	if err != nil {
		return fmt.Errorf("stripe retrieve checkout for cancellation: %w", err)
	}
	if session.Status == stripe.CheckoutSessionStatusExpired {
		return nil
	}
	if session.Status != stripe.CheckoutSessionStatusOpen {
		return fmt.Errorf("stripe cannot cancel a completed checkout")
	}
	_, err = s.sc.V1CheckoutSessions.Expire(ctx, id, nil)
	if err != nil {
		return fmt.Errorf("stripe expire checkout: %w", err)
	}
	return nil
}

func parseStripeCheckoutSession(event *stripe.Event, rawBody string) (*payment.PaymentNotification, error) {
	var session stripe.CheckoutSession
	if event == nil || event.Data == nil {
		return nil, fmt.Errorf("stripe checkout notification is missing event data")
	}
	if err := json.Unmarshal(event.Data.Raw, &session); err != nil {
		return nil, fmt.Errorf("stripe parse checkout session: %w", err)
	}
	if session.Metadata["checkoutMode"] != stripeCheckoutHosted {
		return nil, nil
	}
	status := payment.ProviderStatusSuccess
	if event.Type == stripeEventCheckoutAsyncFailed {
		status = payment.ProviderStatusFailed
	} else if session.PaymentStatus != stripe.CheckoutSessionPaymentStatusPaid {
		// Completion can precede settlement for delayed payment methods.
		return nil, nil
	}
	currency := stripeIntentCurrency(session.Currency, payment.DefaultPaymentCurrency)
	return &payment.PaymentNotification{
		TradeNo: session.ID, OrderID: session.Metadata["orderId"], Status: status,
		Amount: payment.MinorUnitToAmount(session.AmountTotal, currency), RawData: rawBody,
		Metadata: map[string]string{"currency": currency},
	}, nil
}
