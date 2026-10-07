//go:build unit

package handler

import (
	"context"
	"crypto"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"entgo.io/ent/dialect"
	entsql "entgo.io/ent/dialect/sql"
	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/ent/enttest"
	"github.com/Wei-Shaw/sub2api/internal/payment"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestWxpayWebhookBindsDecryptedNotificationToLegacyOrderInstance(t *testing.T) {
	ctx := context.Background()
	db, err := sql.Open("sqlite", "file:wxpay_webhook_instance_security?mode=memory&cache=shared")
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	_, err = db.Exec("PRAGMA foreign_keys = ON")
	require.NoError(t, err)
	client := enttest.NewClient(t, enttest.WithOptions(dbent.Driver(entsql.OpenDB(dialect.SQLite, db))))
	t.Cleanup(func() { _ = client.Close() })
	platformKey, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)
	merchantKey, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)
	privateDER, err := x509.MarshalPKCS8PrivateKey(merchantKey)
	require.NoError(t, err)
	publicDER, err := x509.MarshalPKIXPublicKey(&platformKey.PublicKey)
	require.NoError(t, err)
	encryptionKey := []byte("0123456789abcdef0123456789abcdef")
	configs := make(map[string]map[string]string)
	instances := make(map[string]*dbent.PaymentProviderInstance)
	for _, suffix := range []string{"a", "b"} {
		config := map[string]string{
			"appId": "wx-app-" + suffix, "mchId": "merchant-" + suffix,
			"privateKey":  string(pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: privateDER})),
			"publicKey":   string(pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: publicDER})),
			"publicKeyId": "PUB_KEY_ID_TEST", "certSerial": "MERCHANT_CERT", "apiV3Key": strings.Repeat(suffix, 32),
		}
		configs[suffix] = config
		data, err := json.Marshal(config)
		require.NoError(t, err)
		encrypted, err := payment.Encrypt(string(data), encryptionKey)
		require.NoError(t, err)
		instances[suffix], err = client.PaymentProviderInstance.Create().SetName(suffix).SetProviderKey(payment.TypeWxpay).
			SetSupportedTypes(payment.TypeWxpay).SetEnabled(true).SetConfig(encrypted).SetSortOrder(len(instances)).Save(ctx)
		require.NoError(t, err)
	}
	user, err := client.User.Create().SetEmail("wxpay-webhook-security@example.com").SetUsername("wxpay-security").SetPasswordHash("hash").Save(ctx)
	require.NoError(t, err)
	instanceID := strconv.FormatInt(instances["b"].ID, 10)
	order, err := client.PaymentOrder.Create().SetUserID(user.ID).SetUserEmail(user.Email).SetUserName(user.Username).
		SetAmount(88).SetPayAmount(88).SetFeeRate(0).SetRechargeCode("WX-LEGACY-SECURITY").SetOutTradeNo("WX-LEGACY-ORDER").
		SetPaymentType(payment.TypeWxpay).SetProviderKey(payment.TypeWxpay).SetProviderInstanceID(instanceID).
		SetProviderSnapshot(map[string]any{"schema_version": 1, "provider_key": payment.TypeWxpay, "provider_instance_id": instanceID}).
		SetPaymentTradeNo("WX-TRADE").SetOrderType(payment.OrderTypeBalance).SetStatus(service.OrderStatusCompleted).
		SetExpiresAt(time.Now().Add(time.Hour)).SetClientIP("127.0.0.1").SetSrcHost("api.example.com").Save(ctx)
	require.NoError(t, err)
	registry := payment.NewRegistry()
	paymentSvc := service.NewPaymentService(client, registry, payment.NewDefaultLoadBalancer(client, encryptionKey), nil, nil, nil, nil, nil, nil, nil)
	handler := NewPaymentWebhookHandler(paymentSvc, registry)
	for _, tc := range []struct {
		name, merchant         string
		legacyReference, unpin bool
		want                   int
	}{
		{name: "foreign_merchant", merchant: "a", want: http.StatusBadRequest},
		{name: "original_merchant", merchant: "b", want: http.StatusOK},
		{name: "legacy_foreign_merchant", merchant: "a", legacyReference: true, want: http.StatusBadRequest},
		{name: "legacy_original_merchant", merchant: "b", legacyReference: true, want: http.StatusOK},
		{name: "ambiguous_unpinned_order", merchant: "b", unpin: true, want: http.StatusBadRequest},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if tc.unpin {
				_, err := client.PaymentOrder.UpdateOneID(order.ID).ClearProviderInstanceID().ClearProviderSnapshot().Save(ctx)
				require.NoError(t, err)
			}
			config := configs[tc.merchant]
			outTradeNo := order.OutTradeNo
			if tc.legacyReference {
				outTradeNo = fmt.Sprintf("sub2_%d", order.ID)
			}
			transaction, err := json.Marshal(map[string]any{
				"appid": config["appId"], "mchid": config["mchId"], "out_trade_no": outTradeNo,
				"transaction_id": "WX-TRADE", "trade_state": "SUCCESS", "amount": map[string]any{"total": 8800, "currency": "CNY"},
			})
			require.NoError(t, err)
			block, err := aes.NewCipher([]byte(config["apiV3Key"]))
			require.NoError(t, err)
			gcm, err := cipher.NewGCM(block)
			require.NoError(t, err)
			nonce := "123456789012"
			body, err := json.Marshal(map[string]any{
				"id": "fixture-notify", "event_type": "TRANSACTION.SUCCESS", "resource_type": "encrypt-resource",
				"resource": map[string]any{"algorithm": "AEAD_AES_256_GCM", "nonce": nonce, "associated_data": "transaction",
					"ciphertext": base64.StdEncoding.EncodeToString(gcm.Seal(nil, []byte(nonce), transaction, []byte("transaction")))},
			})
			require.NoError(t, err)
			ts, headerNonce := strconv.FormatInt(time.Now().Unix(), 10), "header-nonce"
			hash := sha256.Sum256([]byte(fmt.Sprintf("%s\n%s\n%s\n", ts, headerNonce, body)))
			signature, err := rsa.SignPKCS1v15(rand.Reader, platformKey, crypto.SHA256, hash[:])
			require.NoError(t, err)
			recorder := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(recorder)
			c.Request = httptest.NewRequest(http.MethodPost, "/api/v1/payment/webhook/wxpay", strings.NewReader(string(body)))
			c.Request.Header.Set("Wechatpay-Serial", "PUB_KEY_ID_TEST")
			c.Request.Header.Set("Wechatpay-Nonce", headerNonce)
			c.Request.Header.Set("Wechatpay-Timestamp", ts)
			c.Request.Header.Set("Wechatpay-Signature", base64.StdEncoding.EncodeToString(signature))
			handler.WxpayNotify(c)
			require.Equal(t, tc.want, recorder.Code)
		})
	}
}
