package handler

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestExcelBPSConfigurationUpdateReselectsOrdinaryAccount(t *testing.T) {
	for _, stream := range []bool{false, true} {
		for _, ordinaryAvailable := range []bool{false, true} {
			t.Run(fmt.Sprintf("stream=%t/ordinary=%t", stream, ordinaryAvailable), func(t *testing.T) {
				upstream := &excelBPSRateLimitUpstream{}
				var accounts *excelBPSRateLimitAccountRepo
				router := newExcelBPSRateLimitRouter(t, upstream, func(repo *excelBPSRateLimitAccountRepo, _ *service.Group) {
					accounts = repo
					if ordinaryAvailable {
						ordinary := repo.accounts[0]
						ordinary.ID, ordinary.Priority = 803, 803
						ordinary.Extra = map[string]any{"openai_passthrough": true}
						repo.accounts = append(repo.accounts, ordinary)
					}
				})
				body := fmt.Sprintf(`{"model":"gpt-5.6-sol","stream":%t,"reasoning":{"effort":"medium"},"input":[{"type":"message","role":"user","content":"retain context"},{"type":"configuration_update","reasoning":{"effort":"high"}}]}`, stream)
				rec := httptest.NewRecorder()
				req := httptest.NewRequest(http.MethodPost, "/v1/responses", strings.NewReader(body))
				req.Header.Set("Content-Type", "application/json")
				req.Header.Set("X-Sub2API-Retry-Account", "true")
				req.Header.Set("X-Sub2API-Max-Retries", "0")
				router.ServeHTTP(rec, req)

				require.Contains(t, accounts.admissionIDs, int64(801))
				require.NotContains(t, accounts.admissionIDs, int64(802))
				require.Equal(t, service.ExcelBPSConfigurationUpdateReason, rec.Header().Get("X-Codex2API-Basispoints-Bypass"))
				if !ordinaryAvailable {
					require.Equal(t, http.StatusServiceUnavailable, rec.Code, rec.Body.String())
					require.Empty(t, upstream.hits)
					require.NotContains(t, rec.Body.String(), "basispoints does not support configuration_update")
					return
				}
				require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
				require.Equal(t, []int64{803}, upstream.hits)
				require.Equal(t, []string{"/backend-api/codex/responses"}, upstream.paths)
				require.Contains(t, rec.Body.String(), "ok")
				for _, field := range []string{"model", "input", "reasoning"} {
					require.Equal(t, gjson.Get(body, field).Raw, gjson.GetBytes(upstream.bodies[0], field).Raw, "preserve original %s on the native route", field)
				}
			})
		}
	}
}
