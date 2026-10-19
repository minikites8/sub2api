//go:build unit

package service

import (
	"context"
	"errors"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
)

func newAuthServiceForCaptchaRepoTest(repo *settingRepoStub, required bool, turnstileVerifier TurnstileVerifier, tencentVerifier TencentCaptchaVerifier) *AuthService {
	cfg := &config.Config{
		Server:    config.ServerConfig{Mode: "release"},
		Turnstile: config.TurnstileConfig{Required: required},
	}
	settingService := NewSettingService(repo, cfg)
	turnstileService := NewTurnstileService(settingService, turnstileVerifier)
	tencentService := NewTencentCaptchaService(settingService, tencentVerifier)
	svc := NewAuthService(nil, &userRepoStub{}, nil, nil, cfg, settingService, nil, turnstileService, nil, nil, nil, nil, nil)
	svc.SetTencentCaptchaService(tencentService)
	return svc
}

func newAuthServiceForCaptchaTest(settings map[string]string, required bool, turnstileVerifier TurnstileVerifier, tencentVerifier TencentCaptchaVerifier) *AuthService {
	cfg := &config.Config{
		Server:    config.ServerConfig{Mode: "release"},
		Turnstile: config.TurnstileConfig{Required: required},
	}
	settingService := NewSettingService(&settingRepoStub{values: settings}, cfg)
	var turnstileService *TurnstileService
	if turnstileVerifier != nil {
		turnstileService = NewTurnstileService(settingService, turnstileVerifier)
	}
	svc := NewAuthService(nil, &userRepoStub{}, nil, nil, cfg, settingService, nil, turnstileService, nil, nil, nil, nil, nil)
	if tencentVerifier != nil {
		svc.SetTencentCaptchaService(NewTencentCaptchaService(settingService, tencentVerifier))
	}
	return svc
}

func tencentCaptchaSettings() map[string]string {
	return map[string]string{
		SettingKeyTencentCaptchaEnabled:        "true",
		SettingKeyTencentCaptchaAppID:          "123456789",
		SettingKeyTencentCaptchaAppSecretKey:   "app-secret",
		SettingKeyTencentCaptchaCloudSecretID:  "cloud-secret-id",
		SettingKeyTencentCaptchaCloudSecretKey: "cloud-secret-key",
	}
}

func TestVerifyCaptchaUsesTencentWhenEnabled(t *testing.T) {
	verifier := &tencentCaptchaVerifierStub{response: &TencentCaptchaVerifyResponse{CaptchaCode: 1}}
	svc := newAuthServiceForCaptchaTest(tencentCaptchaSettings(), false, nil, verifier)

	err := svc.VerifyCaptcha(context.Background(), CaptchaProof{
		TencentTicket:  "ticket",
		TencentRandstr: "@rand",
	}, "203.0.113.10")

	require.NoError(t, err)
	require.Equal(t, 1, verifier.calls)
}

func TestVerifyRequiredCaptchaDispatchesAllProviders(t *testing.T) {
	for _, provider := range []string{"turnstile", "tencent", "aliyun"} {
		t.Run(provider, func(t *testing.T) {
			turnstile := &turnstileVerifierSpy{}
			tencent := &tencentCaptchaVerifierStub{response: &TencentCaptchaVerifyResponse{CaptchaCode: 1}}
			aliyun := &aliyunVerifierSpy{}
			settings := map[string]string{}
			switch provider {
			case "turnstile":
				settings[SettingKeyTurnstileEnabled] = "true"
				settings[SettingKeyTurnstileSecretKey] = "secret"
			case "tencent":
				settings = tencentCaptchaSettings()
			case "aliyun":
				settings = aliyunEnabledSettings()
			}
			svc := newAuthServiceForCaptchaTest(settings, false, turnstile, tencent)
			svc.SetAliyunCaptchaService(NewAliyunCaptchaService(svc.settingService, aliyun))
			svc.cfg.Server.Mode = "debug"

			err := svc.VerifyRequiredCaptcha(context.Background(), CaptchaProof{
				TurnstileToken: "token-or-param", TencentTicket: "ticket", TencentRandstr: "randstr",
			}, "203.0.113.10")

			require.NoError(t, err)
			require.Equal(t, 1, turnstile.called+tencent.calls+aliyun.called)
			switch provider {
			case "turnstile":
				require.Equal(t, 1, turnstile.called)
			case "tencent":
				require.Equal(t, TencentCaptchaProof{Ticket: "ticket", Randstr: "randstr"}, tencent.proof)
			case "aliyun":
				require.Equal(t, "token-or-param", aliyun.lastParam)
			}
		})
	}
}

func TestVerifyRequiredCaptchaNeverSkipsWhenProviderIsMissing(t *testing.T) {
	for _, svc := range []*AuthService{nil, {}, newAuthServiceForCaptchaTest(map[string]string{}, false, nil, nil)} {
		require.ErrorIs(t, svc.VerifyRequiredCaptcha(context.Background(), CaptchaProof{}, ""), ErrCaptchaNotConfigured)
	}
}

func TestVerifyRequiredCaptchaRejectsEmptyProofs(t *testing.T) {
	turnstile := newAuthServiceForCaptchaTest(map[string]string{
		SettingKeyTurnstileEnabled: "true", SettingKeyTurnstileSecretKey: "secret",
	}, false, &turnstileVerifierSpy{}, nil)
	tencent := newAuthServiceForCaptchaTest(tencentCaptchaSettings(), false, nil, &tencentCaptchaVerifierStub{})
	aliyun := newAliyunAuthServiceForTest(&config.Config{}, aliyunEnabledSettings(), &aliyunVerifierSpy{})
	for _, tc := range []struct {
		svc *AuthService
		err error
	}{
		{turnstile, ErrTurnstileVerificationFailed},
		{tencent, ErrTencentCaptchaVerificationFailed},
		{aliyun, ErrAliyunCaptchaVerificationFailed},
	} {
		require.ErrorIs(t, tc.svc.VerifyRequiredCaptcha(context.Background(), CaptchaProof{}, ""), tc.err)
	}
}

func TestVerifyRequiredCaptchaFailsClosedOnInvalidConfiguration(t *testing.T) {
	repo := &settingRepoStub{err: errors.New("settings unavailable")}
	svc := newAuthServiceForCaptchaRepoTest(repo, false, &turnstileVerifierSpy{}, nil)
	require.ErrorIs(t, svc.VerifyRequiredCaptcha(context.Background(), CaptchaProof{}, ""), ErrServiceUnavailable)

	settings := tencentCaptchaSettings()
	settings[SettingKeyAliyunCaptchaEnabled] = "true"
	svc = newAuthServiceForCaptchaTest(settings, false, nil, &tencentCaptchaVerifierStub{})
	require.ErrorIs(t, svc.VerifyRequiredCaptcha(context.Background(), CaptchaProof{}, ""), ErrCaptchaProviderConflict)
}

func TestVerifyCaptchaRejectsDirtyDoubleEnabledSettings(t *testing.T) {
	settings := tencentCaptchaSettings()
	settings[SettingKeyTurnstileEnabled] = "true"
	settings[SettingKeyTurnstileSecretKey] = "turnstile-secret"
	turnstileVerifier := &turnstileVerifierSpy{}
	tencentVerifier := &tencentCaptchaVerifierStub{response: &TencentCaptchaVerifyResponse{CaptchaCode: 1}}
	svc := newAuthServiceForCaptchaTest(settings, false, turnstileVerifier, tencentVerifier)

	err := svc.VerifyCaptcha(context.Background(), CaptchaProof{
		TurnstileToken: "turnstile-token",
		TencentTicket:  "ticket",
		TencentRandstr: "@rand",
	}, "203.0.113.10")

	require.ErrorIs(t, err, ErrCaptchaProviderConflict)
	require.Zero(t, turnstileVerifier.called)
	require.Zero(t, tencentVerifier.calls)
}

func TestVerifyCaptchaRequiredModeAcceptsCompleteTencentProvider(t *testing.T) {
	verifier := &tencentCaptchaVerifierStub{response: &TencentCaptchaVerifyResponse{CaptchaCode: 1}}
	svc := newAuthServiceForCaptchaTest(tencentCaptchaSettings(), true, nil, verifier)

	err := svc.VerifyCaptcha(context.Background(), CaptchaProof{
		TencentTicket:  "ticket",
		TencentRandstr: "@rand",
	}, "203.0.113.10")

	require.NoError(t, err)
}

func TestVerifyCaptchaForRegisterSkipsDuplicateTencentTicketAfterEmailCode(t *testing.T) {
	settings := tencentCaptchaSettings()
	settings[SettingKeyEmailVerifyEnabled] = "true"
	verifier := &tencentCaptchaVerifierStub{response: &TencentCaptchaVerifyResponse{CaptchaCode: 1}}
	svc := newAuthServiceForCaptchaTest(settings, true, nil, verifier)

	err := svc.VerifyCaptchaForRegister(context.Background(), CaptchaProof{}, "203.0.113.10", "123456")

	require.NoError(t, err)
	require.Zero(t, verifier.calls)
}

func TestVerifyCaptchaFailsClosedWhenProviderSettingsCannotBeRead(t *testing.T) {
	repo := &settingRepoStub{err: errors.New("settings unavailable")}
	svc := newAuthServiceForCaptchaRepoTest(repo, false, &turnstileVerifierSpy{}, &tencentCaptchaVerifierStub{})

	err := svc.VerifyCaptcha(context.Background(), CaptchaProof{}, "203.0.113.10")

	require.ErrorIs(t, err, ErrServiceUnavailable)
}

func TestVerifyCaptchaReadsProviderConfigurationOnce(t *testing.T) {
	repo := &settingRepoStub{values: tencentCaptchaSettings()}
	verifier := &tencentCaptchaVerifierStub{response: &TencentCaptchaVerifyResponse{CaptchaCode: 1}}
	svc := newAuthServiceForCaptchaRepoTest(repo, false, &turnstileVerifierSpy{}, verifier)

	err := svc.VerifyCaptcha(context.Background(), CaptchaProof{
		TencentTicket:  "ticket",
		TencentRandstr: "@rand",
	}, "203.0.113.10")

	require.NoError(t, err)
	require.Equal(t, 1, repo.getMultipleCalls)
	require.Zero(t, repo.getValueCalls)
	require.Equal(t, 1, verifier.calls)
}

func TestVerifyCaptchaRejectsEnabledTencentProviderWithIncompleteCredentials(t *testing.T) {
	repo := &settingRepoStub{values: map[string]string{
		SettingKeyTencentCaptchaEnabled: "true",
		SettingKeyTencentCaptchaAppID:   "123456789",
	}}
	verifier := &tencentCaptchaVerifierStub{response: &TencentCaptchaVerifyResponse{CaptchaCode: 1}}
	svc := newAuthServiceForCaptchaRepoTest(repo, false, &turnstileVerifierSpy{}, verifier)

	err := svc.VerifyCaptcha(context.Background(), CaptchaProof{
		TencentTicket:  "ticket",
		TencentRandstr: "@rand",
	}, "203.0.113.10")

	require.ErrorIs(t, err, ErrTencentCaptchaNotConfigured)
	require.Equal(t, 1, repo.getMultipleCalls)
	require.Zero(t, verifier.calls)
}

func TestVerifyActionCaptchaIfEnabledVerifiesTencentProof(t *testing.T) {
	verifier := &tencentCaptchaVerifierStub{response: &TencentCaptchaVerifyResponse{CaptchaCode: 1}}
	svc := newAuthServiceForCaptchaTest(tencentCaptchaSettings(), false, nil, verifier)

	err := svc.VerifyActionCaptchaIfEnabled(context.Background(), CaptchaProof{
		TencentTicket:  "ticket",
		TencentRandstr: "@rand",
	}, "203.0.113.10")

	require.NoError(t, err)
	require.Equal(t, 1, verifier.calls)
	require.Equal(t, TencentCaptchaProof{Ticket: "ticket", Randstr: "@rand"}, verifier.proof)
}

func TestVerifyActionCaptchaIfEnabledDoesNotExpandTurnstileCoverage(t *testing.T) {
	settings := map[string]string{
		SettingKeyTurnstileEnabled:   "true",
		SettingKeyTurnstileSecretKey: "turnstile-secret",
	}
	turnstileVerifier := &turnstileVerifierSpy{}
	svc := newAuthServiceForCaptchaTest(settings, false, turnstileVerifier, nil)

	err := svc.VerifyActionCaptchaIfEnabled(context.Background(), CaptchaProof{}, "203.0.113.10")

	require.NoError(t, err)
	require.Zero(t, turnstileVerifier.called)
}

func TestVerifyActionCaptchaIfEnabledFailsClosedOnSettingReadError(t *testing.T) {
	repo := &settingRepoStub{err: errors.New("settings unavailable")}
	svc := newAuthServiceForCaptchaRepoTest(repo, false, &turnstileVerifierSpy{}, &tencentCaptchaVerifierStub{})

	err := svc.VerifyActionCaptchaIfEnabled(context.Background(), CaptchaProof{}, "203.0.113.10")

	require.ErrorIs(t, err, ErrServiceUnavailable)
}
