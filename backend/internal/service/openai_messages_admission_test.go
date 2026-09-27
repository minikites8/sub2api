package service

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
	"maps"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestMessagesAdmissionRefreshesDisabledBPS(t *testing.T) {
	selected := excelAccount()
	latest := *selected
	latest.Extra = maps.Clone(selected.Extra)
	latest.Extra["openai_excel_bps"] = false
	u := &httpUpstreamRecorder{responses: []*http.Response{excelBPSOrdinaryResponse()}}
	s := openAIClientToolsTestService(u)
	s.accountRepo = &turnAdmissionRepo{account: &latest}
	body := []byte(`{"model":"gpt-5.6-sol","max_tokens":64,"messages":[{"role":"user","content":"keep full context"}],"stream":false}`)
	c, _ := newExcelBPSFallbackContext(body, false)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", strings.NewReader(string(body)))
	result, err := s.ForwardAsAnthropic(context.Background(), c, selected, body, "", "")
	require.NoError(t, err)
	require.NotNil(t, result)
	require.Len(t, u.requests, 1)
	require.Equal(t, chatgptCodexURL, u.requests[0].URL.String())
	require.Contains(t, string(u.bodies[0]), "keep full context")
}
func TestMessagesAdmissionPreservesDetailedReason(t *testing.T) {
	selected := excelAccount()
	selected.Extra["openai_excel_bps"] = false
	latest := *selected
	latest.Schedulable = false
	u := &httpUpstreamRecorder{}
	s := openAIClientToolsTestService(u)
	s.accountRepo = &turnAdmissionRepo{account: &latest}
	body := []byte(`{"model":"gpt-5.6-sol","max_tokens":64,"messages":[{"role":"user","content":"private text"}]}`)
	c, _ := newExcelBPSFallbackContext(body, false)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", strings.NewReader(string(body)))
	_, err := s.ForwardAsAnthropic(context.Background(), c, selected, body, "", "")
	require.Equal(t, "account_ineligible", OpenAITurnAdmissionReason(err))
	require.Empty(t, u.requests)
	require.Contains(t, c.GetString(OpsUpstreamErrorMessageKey), "account_ineligible")
	require.Contains(t, c.GetString(OpsUpstreamErrorDetailKey), "request_path=/v1/messages")
	require.NotContains(t, c.GetString(OpsUpstreamErrorDetailKey), "private text")
}

func TestMessagesAdmissionUsesMappedModel(t *testing.T) {
	account := excelAccount()
	account.Extra["openai_excel_bps"] = false
	u := &httpUpstreamRecorder{}
	svc := openAIClientToolsTestService(u)
	repo := &turnAdmissionRepo{account: account}
	svc.accountRepo = repo
	now := time.Now()
	state := svc.getOpenAIAccountModelTransientState()
	state.recordFailure(account.ID, "gpt-5.6-sol", now)
	state.recordFailure(account.ID, "gpt-5.6-sol", now)
	body := []byte(`{"model":"claude-sonnet-4-5","max_tokens":32,"messages":[{"role":"user","content":"hello"}]}`)
	c, _ := newExcelBPSFallbackContext(body, false)
	_, err := svc.ForwardAsAnthropic(context.Background(), c, account, body, "", "gpt-5.6-sol")
	require.Equal(t, "model_runtime_blocked", OpenAITurnAdmissionReason(err))
	require.Empty(t, u.requests)
	require.Equal(t, 1, repo.reads)
}
func TestMessagesAdmissionRefreshReplaysFullContext(t *testing.T) {
	selected := excelAccount()
	selected.Type = AccountTypeAPIKey
	selected.Extra["openai_excel_bps"] = false
	selected.Credentials = map[string]any{"api_key": "sk-test", "base_url": "https://api.openai.com/v1"}
	latest := *selected
	latest.Extra = maps.Clone(selected.Extra)
	latest.Extra["openai_responses_flatten_namespaces"] = true
	u := &httpUpstreamRecorder{resp: openAICompatSSECompletedResponse("resp_fresh", "gpt-5.3-codex")}
	svc := openAIClientToolsTestService(u)
	svc.accountRepo = &turnAdmissionRepo{account: &latest}
	svc.bindOpenAICompatSessionResponseID(context.Background(), nil, selected, "stable-key", "resp_old_route")
	messages := make([]map[string]string, 0, 21)
	for i := 0; i < 21; i++ {
		role := "user"
		if i%2 == 1 {
			role = "assistant"
		}
		messages = append(messages, map[string]string{"role": role, "content": fmt.Sprintf("original-context-%d", i)})
	}
	body, err := json.Marshal(map[string]any{"model": "claude-sonnet-4-5", "max_tokens": 32, "messages": messages, "stream": false})
	require.NoError(t, err)
	c, _ := newExcelBPSFallbackContext(body, false)
	_, err = svc.ForwardAsAnthropic(context.Background(), c, selected, body, "stable-key", "gpt-5.3-codex")
	require.NoError(t, err)
	require.Len(t, u.requests, 1)
	require.False(t, gjson.GetBytes(u.bodies[0], "previous_response_id").Exists())
	require.Contains(t, string(u.bodies[0]), "original-context-0")
	require.Contains(t, string(u.bodies[0]), "original-context-20")
	require.GreaterOrEqual(t, gjson.GetBytes(u.bodies[0], "input.#").Int(), int64(21))
}
