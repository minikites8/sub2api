package service

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestExcelBPSInlineImageLimitRequestsOrdinaryRouting(t *testing.T) {
	t.Setenv("DATA_DIR", t.TempDir())
	imageBody, _ := nativeGatewayBody(t)
	image := gjson.GetBytes(imageBody, "input.0.content.0").Value()
	for _, mode := range []string{ExcelBPSImageModeNative, ExcelBPSImageModeRelay} {
		for _, probe := range []bool{false, true} {
			for _, limit := range []int{1, 20} {
				t.Run(fmt.Sprintf("%s/probe=%t/limit=%d", mode, probe, limit), func(t *testing.T) {
					upstream := &httpUpstreamRecorder{}
					svc := openAIClientToolsTestService(upstream)
					t.Cleanup(func() { require.NoError(t, svc.CloseExcelBPSImages()) })
					svc.settingService = NewSettingService(&excelBPSImageSettingsRepo{values: map[string]string{
						SettingKeyExcelBPSImageRelayEnabled: "true",
						SettingKeyExcelBPSImageMode:         mode,
						SettingKeyExcelBPSImageBaseURL:      "https://images.example",
						SettingKeyExcelBPSImageMaxImages:    fmt.Sprint(limit),
					}}, svc.cfg)
					parts := make([]any, limit+1)
					for i := range parts {
						parts[i] = image
					}
					body, err := json.Marshal(map[string]any{
						"model": "gpt-5.6-sol", "stream": true,
						"input": []any{map[string]any{"role": "user", "content": parts}},
					})
					require.NoError(t, err)
					c, rec := newExcelBPSFallbackContext(body, true)
					c.Set(bpsAccountProbeRequiredContextKey, probe)
					account := excelAccount()
					result, err := svc.Forward(WithExcelBPSFailoverContext(context.Background()), c, account, body)
					require.Error(t, err)
					require.Nil(t, result)
					require.Empty(t, upstream.requests, "count validation must precede uploads and generation")
					require.True(t, account.IsExcelBPSEnabled())
					require.False(t, svc.isExcelBPSModelCoolingDown(account, "gpt-5.6-sol"))
					if probe {
						require.NotErrorIs(t, err, ErrExcelBPSNativeBypass)
						require.Equal(t, 400, rec.Code)
						require.Contains(t, rec.Body.String(), fmt.Sprintf("at most %d inline images per request", limit))
					} else {
						require.ErrorIs(t, err, ErrExcelBPSNativeBypass)
						require.False(t, c.Writer.Written())
						require.Empty(t, rec.Body.String())
						require.Equal(t, ExcelBPSInlineImageLimitReason, rec.Header().Get("X-Codex2API-Basispoints-Bypass"))
					}
				})
			}
		}
	}
}
