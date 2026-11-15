package service

import (
	"context"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/payment"
	"github.com/stretchr/testify/require"
)

func TestStripeCheckoutPublicKeyPrefersEmbeddedAndPreservesLegacyFallback(t *testing.T) {
	ctx := context.Background()
	client := newPaymentConfigServiceTestClient(t)
	svc := &PaymentConfigService{entClient: client}
	create := func(config string, enabled bool) {
		_, err := client.PaymentProviderInstance.Create().SetProviderKey(payment.TypeStripe).SetName("Stripe").
			SetConfig(config).SetSupportedTypes("card").SetEnabled(enabled).Save(ctx)
		require.NoError(t, err)
	}
	create(`{"checkoutMode":"hosted"}`, true)
	create(`{"checkoutMode":"embedded","publishableKey":"pk_disabled"}`, false)
	require.Empty(t, svc.getStripePublishableKey(ctx), "hosted checkout requires no publishable key")
	create(`{"checkoutMode":"hosted","publishableKey":"pk_old_orders"}`, true)
	require.Equal(t, "pk_old_orders", svc.getStripePublishableKey(ctx), "keep an old embedded order usable after switching its provider to hosted")
	create(`{"publishableKey":"pk_embedded"}`, true)
	require.Equal(t, "pk_embedded", svc.getStripePublishableKey(ctx), "a hosted instance must not hide the active embedded provider key")
}

func TestStripeCheckoutConfigRejectsStandaloneLinkOnCreateAndUpdate(t *testing.T) {
	ctx := context.Background()
	svc := &PaymentConfigService{entClient: newPaymentConfigServiceTestClient(t)}
	req := CreateProviderInstanceRequest{ProviderKey: payment.TypeStripe, Name: "Stripe", SupportedTypes: []string{"link"}, Config: map[string]string{"checkoutMode": "hosted"}}
	_, err := svc.CreateProviderInstance(ctx, req)
	require.ErrorContains(t, err, "select card")

	req.Config["checkoutMode"] = "embedded"
	instance, err := svc.CreateProviderInstance(ctx, req)
	require.NoError(t, err, "standalone Link remains available for built-in checkout")
	_, err = svc.UpdateProviderInstance(ctx, instance.ID, UpdateProviderInstanceRequest{Config: map[string]string{"checkoutMode": "hosted"}})
	require.ErrorContains(t, err, "select card")

	updated, err := svc.UpdateProviderInstance(ctx, instance.ID, UpdateProviderInstanceRequest{Config: map[string]string{"checkoutMode": "hosted"}, SupportedTypes: []string{"card"}})
	require.NoError(t, err)
	require.Equal(t, "card", updated.SupportedTypes)
}
