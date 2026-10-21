package handler

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type gptAutoHookSettingRepo struct{ values map[string]string }

func (r *gptAutoHookSettingRepo) Get(context.Context, string) (*service.Setting, error) {
	return nil, service.ErrSettingNotFound
}
func (r *gptAutoHookSettingRepo) GetValue(_ context.Context, key string) (string, error) {
	if value, ok := r.values[key]; ok {
		return value, nil
	}
	return "", service.ErrSettingNotFound
}
func (r *gptAutoHookSettingRepo) Set(_ context.Context, key, value string) error {
	r.values[key] = value
	return nil
}
func (r *gptAutoHookSettingRepo) GetMultiple(context.Context, []string) (map[string]string, error) {
	return map[string]string{}, nil
}
func (r *gptAutoHookSettingRepo) SetMultiple(context.Context, map[string]string) error { return nil }
func (r *gptAutoHookSettingRepo) GetAll(context.Context) (map[string]string, error) {
	return map[string]string{}, nil
}
func (r *gptAutoHookSettingRepo) Delete(context.Context, string) error { return nil }

// The gateway hook must leave the customer's request and response untouched:
// no bytes written, no status set, no headers added, body unchanged.
func TestObserveGPTAutoShadowDoesNotTouchResponseOrBody(t *testing.T) {
	gin.SetMode(gin.TestMode)
	settings := service.NewSettingService(&gptAutoHookSettingRepo{values: map[string]string{}}, nil)
	cfg := service.DefaultGPTAutoShadowConfig()
	cfg.ShadowEnabled = true
	cfg.SampleRate = 100
	cfg.ClassifierAPIKey = "ts-key"
	require.NoError(t, settings.SetGPTAutoShadowConfig(context.Background(), cfg))
	// No workers started: the request is only snapshotted and queued.
	shadow := service.NewGPTAutoShadowService(settings, nil, nil, nil)

	body := []byte(`{"model":"gpt-5.6-sol","stream":true,"messages":[{"role":"user","content":"hi"}]}`)
	original := append([]byte(nil), body...)
	group := int64(3)
	for _, svc := range []*service.GPTAutoShadowService{shadow, nil} {
		rec := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(rec)
		c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewReader(body))
		observeGPTAutoShadow(c, svc, &service.APIKey{ID: 1, UserID: 2, GroupID: &group}, service.GPTAutoEndpointChatCompletions, body)

		require.False(t, c.Writer.Written())
		require.Zero(t, rec.Body.Len())
		require.Empty(t, rec.Header())
		require.Equal(t, original, body)
	}
	require.EqualValues(t, 1, shadow.RuntimeStatus(context.Background()).QueueLength)
}
