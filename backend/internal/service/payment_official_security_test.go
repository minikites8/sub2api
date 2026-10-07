//go:build unit

package service

import (
	"context"
	"testing"
	"time"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/internal/payment"
	"github.com/stretchr/testify/require"
)

func TestOfficialPaymentSnapshotRejectsEmptyMetadata(t *testing.T) {
	for _, providerKey := range []string{payment.TypeAlipay, payment.TypeWxpay} {
		t.Run(providerKey, func(t *testing.T) {
			order := &dbent.PaymentOrder{ProviderSnapshot: map[string]any{
				"schema_version": 2, "provider_key": providerKey, "merchant_app_id": "expected-app", "merchant_id": "expected-merchant", "currency": "CNY",
			}}
			for _, metadata := range []map[string]string{nil, {}} {
				require.Error(t, validateProviderNotificationMetadata(order, providerKey, metadata))
			}
		})
	}
}

func TestOfficialPaymentNotificationRejectsMissingIdentityBeforeFulfillment(t *testing.T) {
	for _, providerKey := range []string{payment.TypeAlipay, payment.TypeWxpay} {
		t.Run(providerKey, func(t *testing.T) {
			ctx := context.Background()
			client := newPaymentConfigServiceTestClient(t)
			order := createPaymentFulfillmentSubscriptionOrder(t, ctx, client, OrderStatusPending, time.Now())
			order, err := client.PaymentOrder.UpdateOneID(order.ID).
				SetPaymentType(providerKey).
				SetProviderKey(providerKey).
				SetProviderSnapshot(map[string]any{
					"schema_version": 2, "provider_key": providerKey, "merchant_app_id": "expected-app", "merchant_id": "expected-merchant", "currency": "CNY",
				}).Save(ctx)
			require.NoError(t, err)
			svc := &PaymentService{entClient: client}
			err = svc.HandlePaymentNotification(ctx, &payment.PaymentNotification{
				OrderID: order.OutTradeNo, TradeNo: "platform-trade", Status: payment.NotificationStatusSuccess, Amount: order.PayAmount,
			}, providerKey)
			require.ErrorContains(t, err, "missing")
			reloaded, err := client.PaymentOrder.Get(ctx, order.ID)
			require.NoError(t, err)
			require.Equal(t, OrderStatusPending, reloaded.Status)
			require.Equal(t, order.PaymentTradeNo, reloaded.PaymentTradeNo)
		})
	}
}
