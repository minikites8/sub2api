package service

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
)

type autoSupplyMemorySettingRepo struct {
	values map[string]string
}

func (r *autoSupplyMemorySettingRepo) Get(_ context.Context, key string) (*Setting, error) {
	value, ok := r.values[key]
	if !ok {
		return nil, ErrSettingNotFound
	}
	return &Setting{Key: key, Value: value}, nil
}

func (r *autoSupplyMemorySettingRepo) GetValue(ctx context.Context, key string) (string, error) {
	setting, err := r.Get(ctx, key)
	if err != nil {
		return "", err
	}
	return setting.Value, nil
}

func (r *autoSupplyMemorySettingRepo) Set(_ context.Context, key, value string) error {
	if r.values == nil {
		r.values = make(map[string]string)
	}
	r.values[key] = value
	return nil
}

func (r *autoSupplyMemorySettingRepo) GetMultiple(_ context.Context, keys []string) (map[string]string, error) {
	result := make(map[string]string)
	for _, key := range keys {
		if value, ok := r.values[key]; ok {
			result[key] = value
		}
	}
	return result, nil
}

func (r *autoSupplyMemorySettingRepo) SetMultiple(ctx context.Context, settings map[string]string) error {
	for key, value := range settings {
		if err := r.Set(ctx, key, value); err != nil {
			return err
		}
	}
	return nil
}

func (r *autoSupplyMemorySettingRepo) GetAll(_ context.Context) (map[string]string, error) {
	result := make(map[string]string, len(r.values))
	for key, value := range r.values {
		result[key] = value
	}
	return result, nil
}

func (r *autoSupplyMemorySettingRepo) Delete(_ context.Context, key string) error {
	delete(r.values, key)
	return nil
}

type autoSupplyTestEncryptor struct{}

func (autoSupplyTestEncryptor) Encrypt(plaintext string) (string, error) {
	return base64.StdEncoding.EncodeToString([]byte(plaintext)), nil
}

func (autoSupplyTestEncryptor) Decrypt(ciphertext string) (string, error) {
	plaintext, err := base64.StdEncoding.DecodeString(ciphertext)
	return string(plaintext), err
}

type autoSupplyAccountRepoStub struct {
	AccountRepository
	accounts []Account
}

func (r *autoSupplyAccountRepoStub) ListSchedulableByGroupIDAndPlatform(_ context.Context, groupID int64, platform string) ([]Account, error) {
	result := make([]Account, 0, len(r.accounts))
	for _, account := range r.accounts {
		if account.Platform != platform || account.Status != StatusActive || !account.Schedulable {
			continue
		}
		for _, candidate := range account.GroupIDs {
			if candidate == groupID {
				result = append(result, account)
				break
			}
		}
	}
	return result, nil
}

func (r *autoSupplyAccountRepoStub) FindByExtraField(_ context.Context, key string, value any) ([]Account, error) {
	want, _ := value.(string)
	result := make([]Account, 0)
	for _, account := range r.accounts {
		if account.Extra != nil {
			if got, ok := account.Extra[key].(string); ok && got == want {
				result = append(result, account)
			}
		}
	}
	return result, nil
}

type autoSupplyAdminStub struct {
	repo  *autoSupplyAccountRepoStub
	group *Group
	input *CreateAccountInput
}

func (a *autoSupplyAdminStub) GetGroup(_ context.Context, id int64) (*Group, error) {
	if a.group != nil && a.group.ID == id {
		return a.group, nil
	}
	return nil, ErrGroupNotFound
}

func (a *autoSupplyAdminStub) CreateAccount(_ context.Context, input *CreateAccountInput) (*Account, error) {
	a.input = input
	account := Account{
		ID:          int64(len(a.repo.accounts) + 1),
		Name:        input.Name,
		Platform:    input.Platform,
		Type:        input.Type,
		Credentials: input.Credentials,
		Extra:       input.Extra,
		GroupIDs:    input.GroupIDs,
		Status:      StatusActive,
		Schedulable: true,
	}
	a.repo.accounts = append(a.repo.accounts, account)
	return &account, nil
}

func TestAutoSupplyServiceCreatesAndImportsOrder(t *testing.T) {
	var idempotencyKey string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("X-Customer-Token"); got != "customer-secret" {
			t.Errorf("X-Customer-Token = %q, want customer-secret", got)
		}
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/api/customer/inventory":
			if got := r.URL.Query().Get("product"); got != "oauth_30d" {
				t.Errorf("inventory product = %q, want oauth_30d", got)
			}
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"available":0,"missing":1,"needs_production":true}`))
		case r.Method == http.MethodPost && r.URL.Path == "/api/customer/pickup/orders":
			idempotencyKey = r.Header.Get("Idempotency-Key")
			if idempotencyKey == "" {
				t.Error("Idempotency-Key is empty")
			}
			var payload map[string]any
			if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
				t.Errorf("decode order payload: %v", err)
			}
			if got := payload["product"]; got != "oauth_30d" {
				t.Errorf("order product = %v, want oauth_30d", got)
			}
			w.WriteHeader(http.StatusCreated)
			_, _ = w.Write([]byte(`{"order_id":"order-1","status":"pending"}`))
		case r.Method == http.MethodGet && r.URL.Path == "/api/customer/pickup/orders/order-1":
			_, _ = w.Write([]byte(`{"order_id":"order-1","status":"completed"}`))
		case r.Method == http.MethodGet && r.URL.Path == "/api/customer/pickup/orders/order-1/download":
			if got := r.URL.Query().Get("format"); got != "sub2" {
				t.Errorf("download format = %q, want sub2", got)
			}
			_, _ = w.Write([]byte(`{"accounts":[{"name":"upstream-1","platform":"openai","type":"oauth","credentials":{"access_token":"access","refresh_token":"refresh"}}]}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	repo := &autoSupplyAccountRepoStub{}
	admin := &autoSupplyAdminStub{repo: repo, group: &Group{ID: 42, Name: "OpenAI", Platform: PlatformOpenAI}}
	cfg := &config.Config{}
	cfg.AutoSupply = config.AutoSupplyConfig{
		Enabled:           true,
		BaseURL:           server.URL,
		CustomerToken:     "customer-secret",
		MaxQuantityPerRun: 5,
		Groups: []config.AutoSupplyGroupConfig{{
			GroupID:      42,
			MinAvailable: 1,
			Product:      "oauth_30d",
		}},
	}
	svc := NewAutoSupplyService(repo, admin, cfg)

	svc.RunOnce(context.Background())
	require.Empty(t, admin.input)
	require.NotEmpty(t, idempotencyKey)

	svc.RunOnce(context.Background())
	require.NotNil(t, admin.input)
	require.Equal(t, []int64{42}, admin.input.GroupIDs)
	require.Equal(t, AccountTypeOAuth, admin.input.Type)
	require.Equal(t, PlatformOpenAI, admin.input.Platform)
	require.Equal(t, "order-1:0", admin.input.Extra[autoSupplyOrderMarkerKey])

	// A completed import raises local capacity and prevents another order.
	svc.RunOnce(context.Background())
	require.Len(t, repo.accounts, 1)
}

func TestNormalizeAutoSupplyState(t *testing.T) {
	for _, test := range []struct {
		status string
		state  string
		want   string
	}{
		{status: "completed", want: "completed"},
		{status: "", state: "fulfilled", want: "completed"},
		{status: "cancelled", want: "cancelled"},
		{status: "queued", want: "queued"},
	} {
		require.Equal(t, test.want, normalizeAutoSupplyState(test.status, test.state))
	}
}

func TestParseAutoSupplyRetryAfter(t *testing.T) {
	now := time.Date(2026, 8, 17, 12, 0, 0, 0, time.UTC)
	require.Equal(t, 5*time.Second, parseAutoSupplyRetryAfter("5", now))
	require.Zero(t, parseAutoSupplyRetryAfter("0", now))
	require.Zero(t, parseAutoSupplyRetryAfter("invalid", now))
	require.InDelta(t, float64(30*time.Second), float64(parseAutoSupplyRetryAfter(now.Add(30*time.Second).Format(http.TimeFormat), now)), float64(time.Second))
}

func TestDecodeAutoSupplyAccountAndExpiry(t *testing.T) {
	account, err := decodeAutoSupplyAccount(json.RawMessage(`{"accessToken":"access","refreshToken":"refresh","email":"a@example.com"}`))
	require.NoError(t, err)
	require.Equal(t, "access", account.Credentials["access_token"])
	require.Equal(t, "refresh", account.Credentials["refresh_token"])
	require.Equal(t, "a@example.com", account.Credentials["email"])

	expiresAt, err := parseAutoSupplyUnix(json.RawMessage(`"1770000000"`))
	require.NoError(t, err)
	require.NotNil(t, expiresAt)
	require.Equal(t, int64(1770000000), *expiresAt)
}

func TestAutoSupplySettingsPersistEncryptedAndApplyImmediately(t *testing.T) {
	repo := &autoSupplyMemorySettingRepo{values: make(map[string]string)}
	cfg := &config.Config{}
	cfg.Totp.EncryptionKeyConfigured = true
	svc := NewAutoSupplyService(nil, nil, cfg)
	svc.SetSettingsDependencies(repo, autoSupplyTestEncryptor{})

	updated, err := svc.UpdateSettings(context.Background(), AutoSupplySettingsUpdate{
		Enabled:               true,
		BaseURL:               "https://supplier.example/",
		CustomerToken:         "customer-secret",
		IntervalSeconds:       60,
		RequestTimeoutSeconds: 15,
		MaxQuantityPerRun:     8,
		Groups: []AutoSupplyGroupSettings{{
			GroupID: 42, Product: "oauth_30d", MinAvailable: 3, Quantity: 4,
			Platform: PlatformOpenAI, AccountType: AccountTypeOAuth, Priority: 5, Concurrency: 2,
		}},
	})
	require.NoError(t, err)
	require.True(t, updated.CustomerTokenConfigured)
	require.True(t, updated.EncryptionKeyConfigured)
	require.Equal(t, "https://supplier.example", updated.BaseURL)
	raw := repo.values[SettingKeyAutoSupplySettings]
	require.NotContains(t, raw, "customer-secret")
	require.Contains(t, raw, "customer_token_encrypted")

	runtimeSettings := svc.currentAutoSupplyConfig()
	require.Equal(t, "customer-secret", runtimeSettings.CustomerToken)
	require.Equal(t, 60, runtimeSettings.IntervalSeconds)
	select {
	case <-svc.wakeCh:
	default:
		t.Fatal("settings update did not wake worker")
	}

	updated, err = svc.UpdateSettings(context.Background(), AutoSupplySettingsUpdate{
		Enabled:               true,
		BaseURL:               "https://supplier.example",
		IntervalSeconds:       90,
		RequestTimeoutSeconds: 20,
		MaxQuantityPerRun:     10,
		Groups: []AutoSupplyGroupSettings{{
			GroupID: 42, Product: "oauth_30d", MinAvailable: 5, Quantity: 5,
		}},
	})
	require.NoError(t, err)
	require.True(t, updated.CustomerTokenConfigured)
	require.Equal(t, "customer-secret", svc.currentAutoSupplyConfig().CustomerToken)

	reloaded := NewAutoSupplyService(nil, nil, cfg)
	reloaded.SetSettingsDependencies(repo, autoSupplyTestEncryptor{})
	settings, err := reloaded.GetSettings(context.Background())
	require.NoError(t, err)
	require.True(t, settings.CustomerTokenConfigured)
	require.Equal(t, 90, settings.IntervalSeconds)
	require.Equal(t, "customer-secret", reloaded.currentAutoSupplyConfig().CustomerToken)
}

func TestAutoSupplySettingsRequireConfiguredEncryptionKeyForNewToken(t *testing.T) {
	repo := &autoSupplyMemorySettingRepo{values: make(map[string]string)}
	svc := NewAutoSupplyService(nil, nil, &config.Config{})
	svc.SetSettingsDependencies(repo, autoSupplyTestEncryptor{})

	_, err := svc.UpdateSettings(context.Background(), AutoSupplySettingsUpdate{
		BaseURL:               "https://supplier.example",
		CustomerToken:         "new-secret",
		IntervalSeconds:       30,
		RequestTimeoutSeconds: 20,
		MaxQuantityPerRun:     10,
	})
	require.ErrorIs(t, err, ErrSecretEncryptionKeyNotConfigured)
	require.Empty(t, repo.values)
}

func TestAutoSupplySettingsRejectInvalidEnabledConfiguration(t *testing.T) {
	repo := &autoSupplyMemorySettingRepo{values: make(map[string]string)}
	cfg := &config.Config{}
	cfg.Totp.EncryptionKeyConfigured = true
	svc := NewAutoSupplyService(nil, nil, cfg)
	svc.SetSettingsDependencies(repo, autoSupplyTestEncryptor{})

	_, err := svc.UpdateSettings(context.Background(), AutoSupplySettingsUpdate{
		Enabled:               true,
		BaseURL:               "https://supplier.example",
		CustomerToken:         "customer-secret",
		IntervalSeconds:       1,
		RequestTimeoutSeconds: 20,
		MaxQuantityPerRun:     10,
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), "interval_seconds")
}
