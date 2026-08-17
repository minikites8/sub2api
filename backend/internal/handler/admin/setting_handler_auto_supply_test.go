package admin

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type autoSupplyHandlerSettingRepo struct {
	value string
}

func (r *autoSupplyHandlerSettingRepo) Get(_ context.Context, key string) (*service.Setting, error) {
	if r.value == "" {
		return nil, service.ErrSettingNotFound
	}
	return &service.Setting{Key: key, Value: r.value}, nil
}
func (r *autoSupplyHandlerSettingRepo) GetValue(ctx context.Context, key string) (string, error) {
	setting, err := r.Get(ctx, key)
	if err != nil {
		return "", err
	}
	return setting.Value, nil
}
func (r *autoSupplyHandlerSettingRepo) Set(_ context.Context, _ string, value string) error {
	r.value = value
	return nil
}
func (r *autoSupplyHandlerSettingRepo) GetMultiple(context.Context, []string) (map[string]string, error) {
	return map[string]string{}, nil
}
func (r *autoSupplyHandlerSettingRepo) SetMultiple(context.Context, map[string]string) error {
	return nil
}
func (r *autoSupplyHandlerSettingRepo) GetAll(context.Context) (map[string]string, error) {
	return map[string]string{}, nil
}
func (r *autoSupplyHandlerSettingRepo) Delete(context.Context, string) error { return nil }

type autoSupplyHandlerEncryptor struct{}

func (autoSupplyHandlerEncryptor) Encrypt(string) (string, error) { return "encrypted", nil }
func (autoSupplyHandlerEncryptor) Decrypt(string) (string, error) { return "secret", nil }

func newAutoSupplySettingHandler() *SettingHandler {
	cfg := &config.Config{}
	cfg.Totp.EncryptionKeyConfigured = true
	cfg.AutoSupply.CustomerToken = "config-secret"
	svc := service.NewAutoSupplyService(nil, nil, cfg)
	svc.SetSettingsDependencies(&autoSupplyHandlerSettingRepo{}, autoSupplyHandlerEncryptor{})
	handler := NewSettingHandler(nil, nil, nil, nil, nil, nil, nil)
	handler.SetAutoSupplyService(svc)
	return handler
}

func TestGetAutoSupplySettingsMasksToken(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodGet, "/api/v1/admin/settings/auto-supply", nil)

	newAutoSupplySettingHandler().GetAutoSupplySettings(c)
	require.Equal(t, http.StatusOK, recorder.Code)
	require.NotContains(t, recorder.Body.String(), "config-secret")
	require.Contains(t, recorder.Body.String(), `"customer_token_configured":true`)
}

func TestGetAutoSupplyOrders(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodGet, "/api/v1/admin/settings/auto-supply/orders", nil)

	newAutoSupplySettingHandler().GetAutoSupplyOrders(c)
	require.Equal(t, http.StatusOK, recorder.Code)
	require.Contains(t, recorder.Body.String(), `"data":[]`)
}

func TestUpdateAutoSupplySettingsValidatesInput(t *testing.T) {
	gin.SetMode(gin.TestMode)
	payload, err := json.Marshal(service.AutoSupplySettingsUpdate{
		Enabled: true, BaseURL: "https://supplier.example", IntervalSeconds: 1,
		RequestTimeoutSeconds: 20, MaxQuantityPerRun: 10,
		Groups: []service.AutoSupplyGroupSettings{{GroupID: 1, Product: "oauth_30d"}},
	})
	require.NoError(t, err)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPut, "/api/v1/admin/settings/auto-supply", bytes.NewReader(payload))
	c.Request.Header.Set("Content-Type", "application/json")

	newAutoSupplySettingHandler().UpdateAutoSupplySettings(c)
	require.Equal(t, http.StatusBadRequest, recorder.Code)
	require.Contains(t, recorder.Body.String(), "interval_seconds")
}
