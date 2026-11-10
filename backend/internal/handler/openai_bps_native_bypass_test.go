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

func TestExcelBPSNativeBypassReselectsOrdinaryAccount(t *testing.T) {
	for _, path := range []string{"/v1/responses", "/v1/chat/completions"} {
		for _, stream := range []bool{false, true} {
			for _, capability := range []struct {
				name, tools, choice, reason string
			}{
				{"web_search", `[{"type":"web_search","external_web_access":true}]`, `"auto"`, "web_search"},
				{"image_generation", `[{"type":"image_generation"}]`, `"auto"`, "image_generation"},
				{"forced_web_search", `[{"type":"web_search"}]`, `{"type":"web_search"}`, "tool_choice"},
			} {
				t.Run(fmt.Sprintf("%s/%s/stream=%t", path, capability.name, stream), func(t *testing.T) {
					upstream := &excelBPSRateLimitUpstream{}
					var accountRepo *excelBPSRateLimitAccountRepo
					router := newExcelBPSRateLimitRouter(t, upstream, func(repo *excelBPSRateLimitAccountRepo, group *service.Group) {
						accountRepo = repo
						group.AllowImageGeneration = true
						ordinary := repo.accounts[0]
						ordinary.ID, ordinary.Name, ordinary.Priority = 803, "ordinary", 803
						ordinary.Extra = map[string]any{"openai_passthrough": true}
						repo.accounts = append(repo.accounts, ordinary)
					})
					body := fmt.Sprintf(`{"model":"gpt-5.6-sol","input":"retain native capability input","stream":%t,"tools":%s,"tool_choice":%s}`, stream, capability.tools, capability.choice)
					req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(body))
					req.Header.Set("Content-Type", "application/json")
					if stream {
						// Capability routing must work even when upstream retries are disabled.
						req.Header.Set("X-Sub2API-Retry-Account", "true")
						req.Header.Set("X-Sub2API-Max-Retries", "0")
					}
					rec := httptest.NewRecorder()
					router.ServeHTTP(rec, req)

					require.Equal(t, http.StatusOK, rec.Code, "%s admissions=%v hits=%v", rec.Body.String(), accountRepo.admissionIDs, upstream.hits)
					require.Equal(t, []int64{803}, upstream.hits, "BPS accounts must never send the native request")
					require.NotContains(t, accountRepo.admissionIDs, int64(802))
					require.Equal(t, []string{"/backend-api/codex/responses"}, upstream.paths)
					require.Equal(t, capability.reason, rec.Header().Get("X-Codex2API-Basispoints-Bypass"))
					require.Equal(t, "codex", rec.Header().Get("X-Codex2API-Upstream"))
					require.Contains(t, rec.Body.String(), "ok")
					if path == "/v1/chat/completions" {
						require.Contains(t, rec.Body.String(), "chat.completion")
					}
					require.Contains(t, string(upstream.bodies[0]), "retain native capability input")
					require.Equal(t, "gpt-5.6-sol", gjson.GetBytes(upstream.bodies[0], "model").String())
					require.Equal(t, gjson.Parse(capability.tools).Array()[0].Get("type").String(), gjson.GetBytes(upstream.bodies[0], "tools.0.type").String())
				})
			}
		}
	}
}

func TestExcelBPSNativeBypassWithoutOrdinaryAccountStops(t *testing.T) {
	for _, path := range []string{"/v1/responses", "/v1/chat/completions"} {
		for _, stream := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/stream=%t", path, stream), func(t *testing.T) {
				upstream := &excelBPSRateLimitUpstream{}
				router := newExcelBPSRateLimitRouter(t, upstream)
				body := fmt.Sprintf(`{"model":"gpt-5.6-sol","input":"search","stream":%t,"tools":[{"type":"web_search","external_web_access":true}]}`, stream)
				rec := httptest.NewRecorder()
				req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(body))
				req.Header.Set("Content-Type", "application/json")
				router.ServeHTTP(rec, req)

				require.Equal(t, http.StatusServiceUnavailable, rec.Code, rec.Body.String())
				require.Empty(t, upstream.hits)
				require.NotEmpty(t, rec.Body.String())
			})
		}
	}
}

func TestExcelBPSNativeBypassPreservesOmitToolsOptIn(t *testing.T) {
	upstream := &excelBPSRateLimitUpstream{}
	var account *service.Account
	router := newExcelBPSRateLimitRouter(t, upstream, func(repo *excelBPSRateLimitAccountRepo, _ *service.Group) {
		repo.accounts = repo.accounts[1:]
		account = &repo.accounts[0]
		account.Extra[service.ExcelBPSOmitUnsupportedToolsKey] = true
	})
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/v1/responses", strings.NewReader(`{"model":"gpt-5.6-sol","input":"search","stream":false,"tools":[{"type":"web_search","external_web_access":true}]}`))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	require.Equal(t, []int64{802}, upstream.hits)
	require.Equal(t, []string{"/basispoints/api/responses"}, upstream.paths)
	require.Empty(t, rec.Header().Get("X-Codex2API-Basispoints-Bypass"))
	require.True(t, account.IsExcelBPSEnabled(), "routing must not mutate persistent BPS settings")
}

func TestExcelBPSToolHistoryErrorsRouteToOrdinaryAccounts(t *testing.T) {
	for _, stream := range []bool{false, true} {
		for _, tc := range []struct{ name, input, reason string }{
			{"missing original", `[{"type":"message","role":"user","content":"before"},{"type":"function_call","call_id":"call_paired","name":"exec","arguments":"{}"},{"type":"function_call_output","call_id":"call_paired","output":"paired result"},{"type":"message","role":"assistant","content":"history"},{"type":"message","role":"user","content":"continue"},{"type":"message","role":"assistant","content":"history"},{"type":"function_call_output","call_id":"call_unavailable","output":"keep orphan result"}]`, service.OpenAIToolHistoryUnavailableReason},
			{"missing call id", `[{"type":"function_call_output","output":"keep orphan result"}]`, service.OpenAIToolResultMissingCallIDReason},
			{"missing custom call id", `[{"type":"custom_tool_call_output","name":"patch","output":"keep orphan result"}]`, service.OpenAIToolResultMissingCallIDReason},
		} {
			t.Run(fmt.Sprintf("%s/stream=%t", tc.name, stream), func(t *testing.T) {
				upstream := &excelBPSRateLimitUpstream{}
				var accounts *excelBPSRateLimitAccountRepo
				router := newExcelBPSRateLimitRouter(t, upstream, func(repo *excelBPSRateLimitAccountRepo, _ *service.Group) {
					accounts = repo
					ordinary := repo.accounts[0]
					ordinary.ID, ordinary.Priority = 803, 803
					ordinary.Extra = map[string]any{"openai_passthrough": true}
					repo.accounts = append(repo.accounts, ordinary)
				})
				body := fmt.Sprintf(`{"model":"gpt-5.6-sol","stream":%t,"tools":[{"type":"function","name":"exec","parameters":{"type":"object"}}],"input":%s}`, stream, tc.input)
				rec := httptest.NewRecorder()
				req := httptest.NewRequest(http.MethodPost, "/v1/responses", strings.NewReader(body))
				req.Header.Set("Content-Type", "application/json")
				req.Header.Set("X-Sub2API-Retry-Account", "true")
				req.Header.Set("X-Sub2API-Max-Retries", "0")
				router.ServeHTTP(rec, req)

				require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
				require.Equal(t, []int64{803}, upstream.hits)
				require.NotContains(t, accounts.admissionIDs, int64(802))
				require.Equal(t, tc.reason, rec.Header().Get("X-Codex2API-Basispoints-Bypass"))
				require.Equal(t, []string{"/backend-api/codex/responses"}, upstream.paths)
				require.Contains(t, string(upstream.bodies[0]), "keep orphan result")
				require.Contains(t, string(upstream.bodies[0]), "Historical tool result")
				if tc.name == "missing original" {
					require.Contains(t, accounts.admissionIDs, int64(801), "BPS must first classify its replay-cache miss")
					require.Equal(t, "function_call_output", gjson.GetBytes(upstream.bodies[0], "input.2.type").String())
					require.Equal(t, "call_paired", gjson.GetBytes(upstream.bodies[0], "input.2.call_id").String())
					require.Equal(t, "message", gjson.GetBytes(upstream.bodies[0], "input.6.type").String())
				} else {
					require.NotContains(t, accounts.admissionIDs, int64(801), "missing call IDs must route before HTTP validation")
				}
			})
		}
	}
}

func TestExcelBPSToolHistoryErrorsWithoutOrdinaryAccountStop(t *testing.T) {
	for _, input := range []string{
		`[{"type":"function_call_output","output":"keep result"}]`,
		`[{"type":"function_call","call_id":"paired","name":"exec","arguments":"{}"},{"type":"function_call_output","call_id":"paired","output":"paired result"},{"type":"function_call_output","call_id":"unavailable","output":"keep result"}]`,
	} {
		upstream := &excelBPSRateLimitUpstream{}
		router := newExcelBPSRateLimitRouter(t, upstream)
		body := `{"model":"gpt-5.6-sol","stream":true,"tools":[{"type":"function","name":"exec","parameters":{"type":"object"}}],"input":` + input + `}`
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/v1/responses", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		router.ServeHTTP(rec, req)
		require.Equal(t, http.StatusServiceUnavailable, rec.Code, rec.Body.String())
		require.Empty(t, upstream.hits)
		require.NotContains(t, rec.Body.String(), "function_call_output requires call_id")
		require.NotContains(t, rec.Body.String(), "original tool item is unavailable")
	}
}
