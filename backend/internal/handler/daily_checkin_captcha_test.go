//go:build unit

package handler

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	middleware "github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type checkinCaptchaRepo struct {
	service.DailyCheckinRepository
	claims int
}

func (*checkinCaptchaRepo) GetUserCheckin(context.Context, int64, string) (*service.DailyCheckinRecord, error) {
	return nil, nil
}
func (*checkinCaptchaRepo) GetUserLatestCheckin(context.Context, int64) (*service.DailyCheckinRecord, error) {
	return nil, nil
}
func (*checkinCaptchaRepo) SumRewardsByDate(context.Context, string) (float64, error) {
	return 0, nil
}
func (r *checkinCaptchaRepo) Claim(_ context.Context, input service.DailyCheckinClaimInput) (*service.DailyCheckinClaimResult, error) {
	r.claims++
	return &service.DailyCheckinClaimResult{
		Record:  service.DailyCheckinRecord{Reward: input.Reward, CreatedAt: time.Now()},
		Balance: input.Reward,
	}, nil
}

type checkinTurnstileVerifier struct{ token string }

func (v *checkinTurnstileVerifier) VerifyToken(_ context.Context, _ string, token, _ string) (*service.TurnstileVerifyResponse, error) {
	v.token = token
	return &service.TurnstileVerifyResponse{Success: true}, nil
}

type checkinAliyunVerifier struct{ param string }

func (v *checkinAliyunVerifier) VerifyCaptcha(_ context.Context, _ service.AliyunCaptchaCredentials, param string) (*service.AliyunCaptchaVerifyResult, error) {
	v.param = param
	return &service.AliyunCaptchaVerifyResult{VerifyResult: true}, nil
}

func TestDailyCheckinClaimRequiresConfiguredCaptcha(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, tc := range []struct {
		name, provider, body, reason string
		status                       int
	}{
		{"turnstile", "turnstile", `{"turnstile_token":"token"}`, "", http.StatusOK},
		{"tencent", "tencent", `{"tencent_captcha_ticket":"ticket","tencent_captcha_randstr":"randstr"}`, "", http.StatusOK},
		{"aliyun", "aliyun", `{"turnstile_token":"verify-param"}`, "", http.StatusOK},
		{"missing turnstile proof", "turnstile", `{}`, "TURNSTILE_VERIFICATION_FAILED", http.StatusBadRequest},
		{"missing tencent proof", "tencent", `{"turnstile_token":"wrong-provider"}`, "TENCENT_CAPTCHA_VERIFICATION_FAILED", http.StatusBadRequest},
		{"missing aliyun proof", "aliyun", `{}`, "ALIYUN_CAPTCHA_VERIFICATION_FAILED", http.StatusBadRequest},
		{"no enabled provider", "", `{"turnstile_token":"token"}`, "CAPTCHA_NOT_CONFIGURED", http.StatusServiceUnavailable},
		{"conflicting providers", "conflict", `{}`, "CAPTCHA_PROVIDER_CONFLICT", http.StatusServiceUnavailable},
		{"missing verifier", "nil", `{}`, "CAPTCHA_NOT_CONFIGURED", http.StatusServiceUnavailable},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cfg := &config.Config{DailyCheckin: config.DailyCheckinConfig{
				Enabled: true, DailyTotalLimit: 100, MinReward: 1, MaxReward: 1,
			}}
			values := map[string]string{
				service.SettingKeyTurnstileEnabled:             "false",
				service.SettingKeyTurnstileSecretKey:           "secret",
				service.SettingKeyTencentCaptchaAppID:          "123456789",
				service.SettingKeyTencentCaptchaAppSecretKey:   "app-secret",
				service.SettingKeyTencentCaptchaCloudSecretID:  "cloud-id",
				service.SettingKeyTencentCaptchaCloudSecretKey: "cloud-key",
				service.SettingKeyAliyunCaptchaAccessKeyID:     "ak-id",
				service.SettingKeyAliyunCaptchaAccessKeySecret: "ak-secret",
				service.SettingKeyAliyunCaptchaSceneID:         "scene",
			}
			switch tc.provider {
			case "turnstile":
				values[service.SettingKeyTurnstileEnabled] = "true"
			case "tencent":
				values[service.SettingKeyTencentCaptchaEnabled] = "true"
			case "aliyun":
				values[service.SettingKeyAliyunCaptchaEnabled] = "true"
			case "conflict":
				values[service.SettingKeyTurnstileEnabled] = "true"
				values[service.SettingKeyTencentCaptchaEnabled] = "true"
			}
			settings := service.NewSettingService(&oauthCaptchaSettingRepo{values: values}, cfg)
			turnstile, tencent, aliyun := &checkinTurnstileVerifier{}, &oauthCaptchaVerifier{}, &checkinAliyunVerifier{}
			auth := service.NewAuthService(nil, nil, nil, nil, cfg, settings, nil,
				service.NewTurnstileService(settings, turnstile), nil, nil, nil, nil, nil)
			auth.SetTencentCaptchaService(service.NewTencentCaptchaService(settings, tencent))
			auth.SetAliyunCaptchaService(service.NewAliyunCaptchaService(settings, aliyun))
			if tc.provider == "nil" {
				auth = nil
			}
			repo := &checkinCaptchaRepo{}
			h := NewDailyCheckinHandler(service.NewDailyCheckinService(repo, nil, cfg, nil), auth)
			recorder := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(recorder)
			c.Request = httptest.NewRequest(http.MethodPost, "/api/v1/user/daily-checkin", strings.NewReader(tc.body))
			c.Request.Header.Set("Content-Type", "application/json")
			c.Set(string(middleware.ContextKeyUser), middleware.AuthSubject{UserID: 7})

			h.Claim(c)

			require.Equal(t, tc.status, recorder.Code, recorder.Body.String())
			if tc.reason != "" {
				require.Contains(t, recorder.Body.String(), tc.reason)
				require.Zero(t, repo.claims)
				require.Empty(t, turnstile.token)
				require.Zero(t, tencent.calls)
				require.Empty(t, aliyun.param)
			} else {
				require.Equal(t, 1, repo.claims)
				switch tc.provider {
				case "turnstile":
					require.Equal(t, "token", turnstile.token)
				case "tencent":
					require.Equal(t, service.TencentCaptchaProof{Ticket: "ticket", Randstr: "randstr"}, tencent.proof)
				case "aliyun":
					require.Equal(t, "verify-param", aliyun.param)
				}
			}
		})
	}
}
