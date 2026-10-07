//go:build unit

package provider

import (
	"context"
	"crypto"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/payment"
	"github.com/smartwalle/alipay/v3"
	"github.com/smartwalle/nsign"
	"github.com/stretchr/testify/require"
	"github.com/wechatpay-apiv3/wechatpay-go/core"
	"github.com/wechatpay-apiv3/wechatpay-go/core/auth/verifiers"
	"github.com/wechatpay-apiv3/wechatpay-go/core/option"
	"github.com/wechatpay-apiv3/wechatpay-go/utils"
)

// Fixtures use separate merchant and platform keys. All HTTP responses are local.
func newOfficialAlipayFixture(t *testing.T) (*Alipay, *alipay.Client, *rsa.PrivateKey) {
	t.Helper()
	merchantKey, _ := generateTestKeyPair(t)
	platformKey, platformPublicKey := generateTestKeyPair(t)
	p, err := NewAlipay("test", map[string]string{
		"appId": "202610070001", "privateKey": merchantKey, "publicKey": platformPublicKey,
	})
	require.NoError(t, err)
	signer, err := alipay.New("platform-fixture", platformKey, true)
	require.NoError(t, err)
	key, err := utils.LoadPrivateKey(platformKey)
	require.NoError(t, err)
	return p, signer, key
}

func signedAlipayNotification(t *testing.T, signer *alipay.Client, values url.Values) string {
	t.Helper()
	sign, err := signer.SignValues(values, nsign.WithIgnore("sign", "sign_type", "alipay_cert_sn"))
	require.NoError(t, err)
	values.Set("sign", base64.StdEncoding.EncodeToString(sign))
	return values.Encode()
}

func validAlipayNotification() url.Values {
	return url.Values{
		"app_id": {"202610070001"}, "out_trade_no": {"ORDER-1"}, "trade_no": {"ALI-1"},
		"trade_status": {"TRADE_SUCCESS"}, "total_amount": {"650.00"},
		"receipt_amount": {"640.00"}, "buyer_pay_amount": {"630.00"}, "sign_type": {"RSA2"},
	}
}

func TestOfficialAlipayNotificationSecurity(t *testing.T) {
	p, signer, _ := newOfficialAlipayFixture(t)
	for _, tc := range []struct {
		name  string
		edit  func(url.Values)
		valid bool
	}{
		{"success", func(url.Values) {}, true},
		{"finished", func(v url.Values) { v.Set("trade_status", "TRADE_FINISHED") }, true},
		{"closed_without_total", func(v url.Values) { v.Set("trade_status", "TRADE_CLOSED"); v.Del("total_amount") }, true},
		{"wrong_app", func(v url.Values) { v.Set("app_id", "other-app") }, false},
		{"missing_app", func(v url.Values) { v.Del("app_id") }, false},
		{"duplicate_app", func(v url.Values) { v.Add("app_id", "other-app") }, false},
		{"duplicate_amount", func(v url.Values) { v.Add("total_amount", "1.00") }, false},
		{"missing_total_with_receipt", func(v url.Values) { v.Del("total_amount") }, false},
		{"invalid_total_with_receipt", func(v url.Values) { v.Set("total_amount", "invalid") }, false},
		{"nan_total", func(v url.Values) { v.Set("total_amount", "NaN") }, false},
		{"infinite_total", func(v url.Values) { v.Set("total_amount", "+Inf") }, false},
		{"negative_total", func(v url.Values) { v.Set("total_amount", "-650") }, false},
		{"zero_total", func(v url.Values) { v.Set("total_amount", "0") }, false},
		{"missing_order", func(v url.Values) { v.Del("out_trade_no") }, false},
		{"missing_trade", func(v url.Values) { v.Del("trade_no") }, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			values := validAlipayNotification()
			tc.edit(values)
			raw := signedAlipayNotification(t, signer, values)
			client, err := p.getClient()
			require.NoError(t, err)
			require.NoError(t, client.VerifySign(context.Background(), values), "fixture must have a valid platform signature")
			n, err := p.VerifyNotification(context.Background(), raw, nil)
			if !tc.valid {
				require.Error(t, err)
				require.Nil(t, n)
				return
			}
			require.NoError(t, err)
			if values.Get("trade_status") == "TRADE_CLOSED" {
				require.Equal(t, payment.ProviderStatusFailed, n.Status)
				require.Zero(t, n.Amount)
				return
			}
			require.Equal(t, payment.ProviderStatusSuccess, n.Status)
			require.Equal(t, 650.0, n.Amount, "use order total when coupons change receipt and payer amounts")
			require.Equal(t, "202610070001", n.Metadata["app_id"])
		})
	}
	t.Run("merchant_checkout_signature", func(t *testing.T) {
		client, err := p.getClient()
		require.NoError(t, err)
		_, err = p.VerifyNotification(context.Background(), signedAlipayNotification(t, client, validAlipayNotification()), nil)
		require.Error(t, err)
	})
	t.Run("tampered_amount", func(t *testing.T) {
		values := validAlipayNotification()
		signedAlipayNotification(t, signer, values)
		values.Set("total_amount", "99999.00")
		_, err := p.VerifyNotification(context.Background(), values.Encode(), nil)
		require.Error(t, err)
	})
}

func newOfficialWxpayFixture(t *testing.T) (*Wxpay, *rsa.PrivateKey) {
	t.Helper()
	merchantKey, _ := generateTestKeyPair(t)
	platformKey, platformPublicKey := generateTestKeyPair(t)
	p, err := NewWxpay("test", map[string]string{
		"appId": "wx-base-app", "mpAppId": "wx-mp-app", "mchId": "1900000001",
		"privateKey": merchantKey, "publicKey": platformPublicKey, "publicKeyId": "PUB_KEY_ID_TEST",
		"certSerial": "MERCHANT_CERT_TEST", "apiV3Key": "01234567890123456789012345678901",
	})
	require.NoError(t, err)
	key, err := utils.LoadPrivateKey(platformKey)
	require.NoError(t, err)
	return p, key
}

func signOfficialMessage(t *testing.T, key *rsa.PrivateKey, message string) string {
	t.Helper()
	hash := sha256.Sum256([]byte(message))
	signature, err := rsa.SignPKCS1v15(rand.Reader, key, crypto.SHA256, hash[:])
	require.NoError(t, err)
	return base64.StdEncoding.EncodeToString(signature)
}

func officialWxpayHeaders(t *testing.T, key *rsa.PrivateKey, body string) map[string]string {
	t.Helper()
	ts, nonce := strconv.FormatInt(time.Now().Unix(), 10), "fixture-header-nonce"
	return map[string]string{
		"Wechatpay-Timestamp": ts, "Wechatpay-Nonce": nonce, "Wechatpay-Serial": "PUB_KEY_ID_TEST",
		"Wechatpay-Signature": signOfficialMessage(t, key, ts+"\n"+nonce+"\n"+body+"\n"),
	}
}

func validWxpayTransaction() map[string]any {
	return map[string]any{
		"appid": "wx-base-app", "mchid": "1900000001", "out_trade_no": "ORDER-1", "transaction_id": "WX-1",
		"trade_state": "SUCCESS", "amount": map[string]any{"total": 65000, "payer_total": 63000, "currency": "CNY"},
	}
}

func encryptedWxpayNotification(t *testing.T, p *Wxpay, tx map[string]any) string {
	t.Helper()
	plaintext, err := json.Marshal(tx)
	require.NoError(t, err)
	block, err := aes.NewCipher([]byte(p.config["apiV3Key"]))
	require.NoError(t, err)
	gcm, err := cipher.NewGCM(block)
	require.NoError(t, err)
	nonce, associatedData := "123456789012", "transaction"
	ciphertext := gcm.Seal(nil, []byte(nonce), plaintext, []byte(associatedData))
	body, err := json.Marshal(map[string]any{
		"id": "fixture-notify-1", "event_type": "TRANSACTION.SUCCESS", "resource_type": "encrypt-resource",
		"resource": map[string]any{
			"algorithm": "AEAD_AES_256_GCM", "original_type": "transaction", "nonce": nonce,
			"associated_data": associatedData, "ciphertext": base64.StdEncoding.EncodeToString(ciphertext),
		},
	})
	require.NoError(t, err)
	return string(body)
}

func officialWxpayTransactionCases() []struct {
	name  string
	edit  func(map[string]any)
	valid bool
} {
	return []struct {
		name  string
		edit  func(map[string]any)
		valid bool
	}{
		{"base_app", func(map[string]any) {}, true},
		{"mp_app", func(tx map[string]any) { tx["appid"] = "wx-mp-app" }, true},
		{"wrong_app", func(tx map[string]any) { tx["appid"] = "other-app" }, false},
		{"missing_app", func(tx map[string]any) { delete(tx, "appid") }, false},
		{"wrong_merchant", func(tx map[string]any) { tx["mchid"] = "other-merchant" }, false},
		{"missing_merchant", func(tx map[string]any) { delete(tx, "mchid") }, false},
		{"wrong_currency", func(tx map[string]any) { tx["amount"].(map[string]any)["currency"] = "USD" }, false},
		{"missing_currency", func(tx map[string]any) { delete(tx["amount"].(map[string]any), "currency") }, false},
		{"missing_amount", func(tx map[string]any) { delete(tx, "amount") }, false},
		{"missing_total", func(tx map[string]any) { delete(tx["amount"].(map[string]any), "total") }, false},
		{"negative_total", func(tx map[string]any) { tx["amount"].(map[string]any)["total"] = -65000 }, false},
		{"zero_total", func(tx map[string]any) { tx["amount"].(map[string]any)["total"] = 0 }, false},
		{"missing_order", func(tx map[string]any) { delete(tx, "out_trade_no") }, false},
		{"missing_trade", func(tx map[string]any) { delete(tx, "transaction_id") }, false},
	}
}

func TestOfficialWxpayNotificationSecurity(t *testing.T) {
	p, key := newOfficialWxpayFixture(t)
	for _, tc := range officialWxpayTransactionCases() {
		t.Run(tc.name, func(t *testing.T) {
			tx := validWxpayTransaction()
			tc.edit(tx)
			body := encryptedWxpayNotification(t, p, tx)
			n, err := p.VerifyNotification(context.Background(), body, officialWxpayHeaders(t, key, body))
			if !tc.valid {
				require.Error(t, err)
				require.Nil(t, n)
				return
			}
			require.NoError(t, err)
			require.Equal(t, payment.ProviderStatusSuccess, n.Status)
			require.Equal(t, 650.0, n.Amount)
		})
	}
	for _, kind := range []string{"signature", "body", "serial", "timestamp", "api_key"} {
		t.Run("tampered_"+kind, func(t *testing.T) {
			body := encryptedWxpayNotification(t, p, validWxpayTransaction())
			headers := officialWxpayHeaders(t, key, body)
			verifier := p
			switch kind {
			case "signature":
				headers["Wechatpay-Signature"] = "invalid"
			case "body":
				body += " "
			case "serial":
				headers["Wechatpay-Serial"] = "unknown"
			case "timestamp":
				headers["Wechatpay-Timestamp"] = "1"
			case "api_key":
				config := make(map[string]string)
				for k, v := range p.config {
					config[k] = v
				}
				config["apiV3Key"] = "11234567890123456789012345678901"
				var err error
				verifier, err = NewWxpay("other", config)
				require.NoError(t, err)
			}
			_, err := verifier.VerifyNotification(context.Background(), body, headers)
			require.Error(t, err)
		})
	}
	t.Run("irrelevant_event", func(t *testing.T) {
		body := strings.Replace(encryptedWxpayNotification(t, p, map[string]any{}), "TRANSACTION.SUCCESS", "REFUND.SUCCESS", 1)
		n, err := p.VerifyNotification(context.Background(), body, officialWxpayHeaders(t, key, body))
		require.NoError(t, err)
		require.Nil(t, n)
	})
}

type officialPaymentRoundTripper func(*http.Request) (*http.Response, error)

func (f officialPaymentRoundTripper) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestOfficialAlipayQueryOrderSecurity(t *testing.T) {
	p, _, key := newOfficialAlipayFixture(t)
	client, err := p.getClient()
	require.NoError(t, err)
	for _, kind := range []string{"valid", "pending", "wrong_order", "missing_order", "missing_trade", "missing_total", "invalid_total", "nan_total"} {
		t.Run(kind, func(t *testing.T) {
			data := map[string]any{"code": "10000", "msg": "Success", "out_trade_no": "ORDER-1", "trade_no": "ALI-1", "trade_status": "TRADE_SUCCESS", "total_amount": "650.00", "receipt_amount": "650.00"}
			switch kind {
			case "pending":
				data["trade_status"] = "WAIT_BUYER_PAY"
				delete(data, "total_amount")
				delete(data, "trade_no")
			case "wrong_order":
				data["out_trade_no"] = "OTHER-ORDER"
			case "missing_order":
				delete(data, "out_trade_no")
			case "missing_trade":
				delete(data, "trade_no")
			case "missing_total":
				delete(data, "total_amount")
			case "invalid_total":
				data["total_amount"] = "invalid"
			case "nan_total":
				data["total_amount"] = "NaN"
			}
			biz, err := json.Marshal(data)
			require.NoError(t, err)
			body := fmt.Sprintf(`{"alipay_trade_query_response":%s,"sign":%q}`, biz, signOfficialMessage(t, key, string(biz)))
			client.Client = &http.Client{Transport: officialPaymentRoundTripper(func(r *http.Request) (*http.Response, error) {
				return &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": {"application/json"}}, Body: io.NopCloser(strings.NewReader(body)), Request: r}, nil
			})}
			result, err := p.QueryOrder(context.Background(), "ORDER-1")
			if kind != "valid" && kind != "pending" {
				require.Error(t, err)
				require.Nil(t, result)
				return
			}
			require.NoError(t, err)
			if kind == "pending" {
				require.Equal(t, payment.ProviderStatusPending, result.Status)
				require.Zero(t, result.Amount)
				return
			}
			require.Equal(t, payment.ProviderStatusPaid, result.Status)
			require.Equal(t, 650.0, result.Amount)
		})
	}
}

func TestOfficialWxpayQueryOrderSecurity(t *testing.T) {
	p, key := newOfficialWxpayFixture(t)
	merchantKey, err := utils.LoadPrivateKey(p.config["privateKey"])
	require.NoError(t, err)
	cases := append(officialWxpayTransactionCases(), struct {
		name  string
		edit  func(map[string]any)
		valid bool
	}{
		"wrong_order", func(tx map[string]any) { tx["out_trade_no"] = "OTHER-ORDER" }, false,
	}, struct {
		name  string
		edit  func(map[string]any)
		valid bool
	}{
		"pending", func(tx map[string]any) {
			tx["trade_state"] = "NOTPAY"
			delete(tx, "amount")
			delete(tx, "transaction_id")
		}, true,
	})
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			tx := validWxpayTransaction()
			tc.edit(tx)
			body, err := json.Marshal(tx)
			require.NoError(t, err)
			pubKey, err := utils.LoadPublicKey(p.config["publicKey"])
			require.NoError(t, err)
			p.coreClient, err = core.NewClient(context.Background(),
				option.WithMerchantCredential(p.config["mchId"], p.config["certSerial"], merchantKey),
				option.WithVerifier(verifiers.NewSHA256WithRSAPubkeyVerifier(p.config["publicKeyId"], *pubKey)),
				option.WithHTTPClient(&http.Client{Transport: officialPaymentRoundTripper(func(r *http.Request) (*http.Response, error) {
					header := make(http.Header)
					for k, v := range officialWxpayHeaders(t, key, string(body)) {
						header.Set(k, v)
					}
					header.Set("Content-Type", "application/json")
					return &http.Response{StatusCode: 200, Header: header, Body: io.NopCloser(strings.NewReader(string(body))), Request: r}, nil
				})}),
			)
			require.NoError(t, err)
			result, err := p.QueryOrder(context.Background(), "ORDER-1")
			total, refundErr := p.queryOrderTotalFen(context.Background(), p.coreClient, "ORDER-1")
			if !tc.valid {
				require.Error(t, err)
				require.Nil(t, result)
				require.Error(t, refundErr)
				return
			}
			require.NoError(t, err)
			if tc.name == "pending" {
				require.Equal(t, payment.ProviderStatusPending, result.Status)
				require.Zero(t, result.Amount)
				require.Error(t, refundErr)
				return
			}
			require.NoError(t, refundErr)
			require.EqualValues(t, 65000, total)
			require.Equal(t, payment.ProviderStatusPaid, result.Status)
			require.Equal(t, 650.0, result.Amount)
		})
	}
}
