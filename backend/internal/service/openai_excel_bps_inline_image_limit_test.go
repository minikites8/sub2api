package service

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestExcelBPSInlineImageValidationRequestsOrdinaryRouting(t *testing.T) {
	t.Setenv("DATA_DIR", t.TempDir())
	imageBody, _ := nativeGatewayBody(t)
	image := gjson.GetBytes(imageBody, "input.0.content.0").Value()
	for _, mode := range []string{ExcelBPSImageModeNative, ExcelBPSImageModeRelay} {
		for _, probe := range []bool{false, true} {
			for _, tc := range []struct {
				name, url, reason, message string
				limit, count               int
			}{
				{name: "custom count limit", limit: 1, count: 2, reason: ExcelBPSInlineImageLimitReason, message: "at most 1 inline images per request"},
				{name: "default count limit", limit: 20, count: 21, reason: ExcelBPSInlineImageLimitReason, message: "at most 20 inline images per request"},
				{name: "media type mismatch", limit: 20, count: 1, url: strings.Replace(gjson.GetBytes(imageBody, "input.0.content.0.image_url").String(), "image/png", "image/jpeg", 1), reason: ExcelBPSInlineImageMediaTypeReason, message: "does not match"},
				{name: "invalid base64", limit: 20, count: 1, url: "data:image/png;base64,PRIVATE_INVALID", message: "invalid base64 data"},
			} {
				t.Run(fmt.Sprintf("%s/probe=%t/%s", mode, probe, tc.name), func(t *testing.T) {
					upstream := &httpUpstreamRecorder{}
					svc := openAIClientToolsTestService(upstream)
					t.Cleanup(func() { require.NoError(t, svc.CloseExcelBPSImages()) })
					svc.settingService = NewSettingService(&excelBPSImageSettingsRepo{values: map[string]string{
						SettingKeyExcelBPSImageRelayEnabled: "true",
						SettingKeyExcelBPSImageMode:         mode,
						SettingKeyExcelBPSImageBaseURL:      "https://images.example",
						SettingKeyExcelBPSImageMaxImages:    fmt.Sprint(tc.limit),
					}}, svc.cfg)
					parts := make([]any, tc.count)
					for i := range parts {
						parts[i] = image
						if tc.url != "" {
							parts[i] = map[string]any{"type": "input_image", "image_url": tc.url}
						}
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
					if probe || tc.reason == "" {
						require.NotErrorIs(t, err, ErrExcelBPSNativeBypass)
						require.Equal(t, 400, rec.Code)
						require.Contains(t, rec.Body.String(), tc.message)
					} else {
						require.ErrorIs(t, err, ErrExcelBPSNativeBypass)
						require.False(t, c.Writer.Written())
						require.Empty(t, rec.Body.String())
						require.Equal(t, tc.reason, rec.Header().Get("X-Codex2API-Basispoints-Bypass"))
					}
					require.NotContains(t, rec.Body.String(), "PRIVATE_INVALID")
				})
			}
		}
	}
}
