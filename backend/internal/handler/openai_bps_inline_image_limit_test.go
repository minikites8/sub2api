package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

type excelBPSInlineImageLimitSettings struct {
	service.SettingRepository
	mode string
}

func (r excelBPSInlineImageLimitSettings) GetMultiple(context.Context, []string) (map[string]string, error) {
	return map[string]string{
		service.SettingKeyExcelBPSImageRelayEnabled: "true",
		service.SettingKeyExcelBPSImageMode:         r.mode,
		service.SettingKeyExcelBPSImageBaseURL:      "https://images.example",
	}, nil
}

func (excelBPSInlineImageLimitSettings) GetValue(context.Context, string) (string, error) {
	return "", service.ErrSettingNotFound
}

func TestExcelBPSInlineImageLimitReselectsOrdinaryAccount(t *testing.T) {
	t.Setenv("DATA_DIR", t.TempDir())
	for _, mode := range []string{service.ExcelBPSImageModeNative, service.ExcelBPSImageModeRelay} {
		for _, path := range []string{"/v1/responses", "/v1/chat/completions"} {
			for _, stream := range []bool{false, true} {
				for _, ordinaryAvailable := range []bool{false, true} {
					t.Run(fmt.Sprintf("%s/%s/stream=%t/ordinary=%t", mode, path, stream, ordinaryAvailable), func(t *testing.T) {
						upstream := &excelBPSRateLimitUpstream{}
						var accounts *excelBPSRateLimitAccountRepo
						settings := service.NewSettingService(excelBPSInlineImageLimitSettings{mode: mode}, nil)
						router := newExcelBPSRateLimitRouterWithSettings(t, upstream, settings, func(repo *excelBPSRateLimitAccountRepo, _ *service.Group) {
							accounts = repo
							if ordinaryAvailable {
								ordinary := repo.accounts[0]
								ordinary.ID, ordinary.Priority = 803, 803
								ordinary.Extra = map[string]any{"openai_passthrough": true}
								repo.accounts = append(repo.accounts, ordinary)
							}
						})
						const dataURL = "data:image/png;base64,iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVQIHWP4z8DwHwAFgAI/ScLttAAAAABJRU5ErkJggg=="
						parts := make([]any, 21)
						for i := range parts {
							if path == "/v1/chat/completions" {
								parts[i] = map[string]any{"type": "image_url", "image_url": map[string]any{"url": dataURL}}
							} else {
								parts[i] = map[string]any{"type": "input_image", "image_url": dataURL}
							}
						}
						request := map[string]any{"model": "gpt-5.6-sol", "stream": stream}
						inputField := "input"
						if path == "/v1/chat/completions" {
							inputField = "messages"
						}
						request[inputField] = []any{map[string]any{"role": "user", "content": parts}}
						body, err := json.Marshal(request)
						require.NoError(t, err)
						rec := httptest.NewRecorder()
						req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(string(body)))
						req.Header.Set("Content-Type", "application/json")
						req.Header.Set("X-Sub2API-Retry-Account", "true")
						req.Header.Set("X-Sub2API-Max-Retries", "0")
						router.ServeHTTP(rec, req)

						require.Contains(t, accounts.admissionIDs, int64(801))
						require.NotContains(t, accounts.admissionIDs, int64(802), "all other BPS routes share the image count limit")
						require.Equal(t, service.ExcelBPSInlineImageLimitReason, rec.Header().Get("X-Codex2API-Basispoints-Bypass"))
						if !ordinaryAvailable {
							require.Equal(t, http.StatusServiceUnavailable, rec.Code, rec.Body.String())
							require.Empty(t, upstream.hits)
							return
						}
						require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
						require.Equal(t, []int64{803}, upstream.hits)
						require.Equal(t, []string{"/backend-api/codex/responses"}, upstream.paths)
						require.Contains(t, rec.Body.String(), "ok")
						images := gjson.GetBytes(upstream.bodies[0], "input.0.content").Array()
						require.Len(t, images, 21)
						for _, image := range images {
							require.Equal(t, dataURL, image.Get("image_url").String(), "fallback must preserve the original images")
						}
					})
				}
			}
		}
	}
}
