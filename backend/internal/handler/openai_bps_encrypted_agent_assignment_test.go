package handler

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestExcelBPSNewEncryptedAssignmentReselectsOrdinaryAccount(t *testing.T) {
	for _, stream := range []bool{false, true} {
		for _, top := range []bool{false, true} {
			for _, ordinaryAvailable := range []bool{false, true} {
				t.Run(fmt.Sprintf("stream=%t/top=%t/ordinary=%t", stream, top, ordinaryAvailable), func(t *testing.T) {
					upstream := &excelBPSRateLimitUpstream{}
					var accounts *excelBPSRateLimitAccountRepo
					router := newExcelBPSRateLimitRouter(t, upstream, func(repo *excelBPSRateLimitAccountRepo, _ *service.Group) {
						accounts = repo
						for i := range repo.accounts {
							repo.accounts[i].Extra[service.ExcelBPSIgnoreEncryptedContentKey] = true
						}
						if ordinaryAvailable {
							ordinary := repo.accounts[0]
							ordinary.ID, ordinary.Priority = 803, 803
							ordinary.Extra = map[string]any{"openai_passthrough": true}
							repo.accounts = append(repo.accounts, ordinary)
						}
					})
					assignment := map[string]any{
						"type": "agent_message", "author": "/root", "recipient": "/root/worker",
						"content": []any{map[string]any{"type": "input_text", "text": "Header"}},
					}
					if top {
						assignment["encrypted_content"] = "SYNTHETIC_PRIVATE"
					} else {
						assignment["content"] = append(assignment["content"].([]any), map[string]any{"type": "encrypted_content", "data": "SYNTHETIC_PRIVATE"})
					}
					input := make([]any, 270)
					for i := range input {
						input[i] = map[string]any{"type": "message", "role": "user", "content": "history"}
					}
					input[269] = assignment
					input = append(input, map[string]any{"type": "additional_tools", "tools": []any{}})
					body, err := json.Marshal(map[string]any{"model": "gpt-5.6-sol", "stream": stream, "input": input})
					require.NoError(t, err)
					rec := httptest.NewRecorder()
					req := httptest.NewRequest(http.MethodPost, "/v1/responses", bytes.NewReader(body))
					req.Header.Set("Content-Type", "application/json")
					req.Header.Set("X-Sub2API-Retry-Account", "true")
					req.Header.Set("X-Sub2API-Max-Retries", "0")
					router.ServeHTTP(rec, req)

					require.Contains(t, accounts.admissionIDs, int64(801))
					require.NotContains(t, accounts.admissionIDs, int64(802))
					require.Equal(t, service.ExcelBPSEncryptedAgentAssignmentReason, rec.Header().Get("X-Codex2API-Basispoints-Bypass"))
					require.NotContains(t, rec.Body.String(), "SYNTHETIC_PRIVATE")
					if !ordinaryAvailable {
						require.Equal(t, http.StatusServiceUnavailable, rec.Code, rec.Body.String())
						require.Empty(t, upstream.hits)
						return
					}
					require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
					require.Equal(t, []int64{803}, upstream.hits)
					require.Equal(t, []string{"/backend-api/codex/responses"}, upstream.paths)
					require.Contains(t, rec.Body.String(), "ok")
					require.JSONEq(t, gjson.GetBytes(body, "input.269").Raw, gjson.GetBytes(upstream.bodies[0], "input.269").Raw, "retain the entire encrypted task on the native route")
				})
			}
		}
	}
}
