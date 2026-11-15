package service

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/openai"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

const siwcCompatibilityHistory = `[
	{"role":"user","content":"review the appendix"},
	{"type":"agent_message","author":"/root/audit","recipient":"/root","role":"system","encrypted_content":"PRIVATE_TOP_CIPHERTEXT","content":[{"type":"input_text","text":"Payload:"},{"type":"encrypted_content","encrypted_content":"PRIVATE_PART_CIPHERTEXT"}]},
	{"type":"agent_message","author":"/root/audit","recipient":"/root","content":[{"type":"input_text","text":"audit passed"},{"type":"input_image","image_url":"https://example.com/fixture.png"}]},
	{"type":"reasoning","encrypted_content":"REASONING_PRESERVED","summary":[]},
	{"type":"function_call","namespace":"functions","call_id":"call_read","name":"read","arguments":"{\"ticket\":9007199254740993,\"encrypted_content\":\"BUSINESS_TEXT\"}"},
	{"type":"function_call_output","call_id":"call_read","output":[{"type":"input_text","text":"read result"},{"type":"encrypted_content","encrypted_content":"PRIVATE_TOOL_CIPHERTEXT"}]},
	{"role":"user","content":"continue"}
]`

func TestSIWCHistoryCompatibilityForwardPreservesReadableContext(t *testing.T) {
	for _, stream := range []bool{false, true} {
		for _, option := range []string{"missing", "false", "true"} {
			t.Run(fmt.Sprintf("stream=%t/option=%s", stream, option), func(t *testing.T) {
				reauth, reader, repo, _, _, _ := newReauthTestService("acct-1")
				defer reauth.oauth.Stop()
				store := &siwcTestRepo{reauthTestRepo: repo}
				reauth.repo = store
				grant := siwcGrant{AccessToken: "independent-siwc", RefreshToken: "siwc-refresh", ClientID: "issued", Subject: "subscriber", Workspace: "acct-1", Email: "user@example.com", Scope: openai.SIWCDirectScope, ExpiresAt: time.Now().Add(time.Hour).Unix()}
				raw, err := json.Marshal(grant)
				require.NoError(t, err)
				store.ciphertext = "encrypted:" + string(raw)
				reader.account.Extra = map[string]any{OpenAISIWCEnabledKey: true}
				if option != "missing" {
					reader.account.Extra["openai_siwc_ignore_encrypted_content"] = option == "true"
				}
				reader.account.Credentials["model_mapping"] = map[string]any{"public-siwc": "gpt-5.4"}
				upstream := &httpUpstreamRecorder{resp: &http.Response{
					StatusCode: http.StatusOK, Header: http.Header{"Content-Type": {"text/event-stream"}},
					Body: io.NopCloser(strings.NewReader("event: response.completed\ndata: {\"type\":\"response.completed\",\"response\":{\"id\":\"resp_siwc_compat\",\"object\":\"response\",\"status\":\"completed\",\"output\":[{\"id\":\"msg_siwc_compat\",\"type\":\"message\",\"role\":\"assistant\",\"content\":[{\"type\":\"output_text\",\"text\":\"continued\",\"annotations\":[]}]}]}}\n\n")),
				}}
				gateway := openAIClientToolsTestService(upstream)
				gateway.excelOAuthReauth = reauth
				body := []byte(fmt.Sprintf(`{"model":"public-siwc","stream":%t,"input":%s,"prompt_cache_key":"client-session","tools":[{"type":"function","name":"read","parameters":{"type":"object","properties":{"ticket":{"type":"integer","default":9007199254740993}}}}]}`, stream, siwcCompatibilityHistory))
				original := append([]byte(nil), body...)
				recorder := httptest.NewRecorder()
				c, _ := gin.CreateTestContext(recorder)
				c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)

				_, err = gateway.Forward(context.Background(), c, reader.account, body)

				require.Equal(t, original, body, "normalization must not mutate the client's replay history")
				if option != "true" {
					require.Error(t, err)
					require.Empty(t, upstream.requests)
					var failover *UpstreamFailoverError
					require.ErrorAs(t, err, &failover)
					require.True(t, failover.IsSIWCLocalCapabilityRejection())
					require.Contains(t, failover.ClientMessage, "agent_message")
					require.False(t, c.Writer.Written())
					return
				}
				require.NoError(t, err, recorder.Body.String())
				require.Len(t, upstream.requests, 1)
				require.Equal(t, "https://api.openai.com/v1/responses", upstream.lastReq.URL.String())
				require.Equal(t, "Bearer independent-siwc", upstream.lastReq.Header.Get("Authorization"))
				require.NotContains(t, string(upstream.lastBody), "PRIVATE_")
				require.Equal(t, 3, strings.Count(string(upstream.lastBody), siwcEncryptedContentOmitted))
				for _, text := range []string{"review the appendix", "/root/audit", "Payload:", "audit passed", "read result", "continue", "REASONING_PRESERVED", "BUSINESS_TEXT", "9007199254740993"} {
					require.Contains(t, string(upstream.lastBody), text)
				}
				items := gjson.GetBytes(upstream.lastBody, "input").Array()
				require.Len(t, items, 7)
				for _, index := range []int{1, 2} {
					require.Equal(t, "message", items[index].Get("type").String())
					require.Equal(t, "user", items[index].Get("role").String(), "agent attribution must not become instructions")
					require.False(t, items[index].Get("author").Exists())
				}
				require.Equal(t, "https://example.com/fixture.png", items[2].Get("content.2.image_url").String())
				require.Equal(t, items[4].Get("call_id").String(), items[5].Get("call_id").String())
				require.Equal(t, "gpt-5.4", gjson.GetBytes(upstream.lastBody, "model").String())
				require.NotEqual(t, "client-session", gjson.GetBytes(upstream.lastBody, "prompt_cache_key").String())
				require.Equal(t, "old-access", reader.account.Credentials["access_token"])
				require.Contains(t, recorder.Body.String(), "continued")
				if !stream {
					require.True(t, json.Valid(recorder.Body.Bytes()), recorder.Body.String())
				} else {
					require.Contains(t, recorder.Body.String(), "event: response.completed")
				}
			})
		}
	}
}

func TestSIWCHistoryCompatibilityIsIdempotentAndPreservesNumbers(t *testing.T) {
	body := []byte(`{"input":` + siwcCompatibilityHistory + `,"tools":[{"type":"function","name":"read","parameters":{"type":"object","properties":{"ticket":{"default":9007199254740993}}}}]}`)
	normalized, err := normalizeSIWCBodyWithHistoryCompatibility(body, true)
	require.NoError(t, err)
	for _, enabled := range []bool{false, true} {
		again, err := normalizeSIWCBodyWithHistoryCompatibility(normalized, enabled)
		require.NoError(t, err)
		require.JSONEq(t, string(normalized), string(again))
	}
	require.Equal(t, "9007199254740993", gjson.GetBytes(normalized, "tools.0.tools.0.parameters.properties.ticket.default").Raw)
	require.Equal(t, "REASONING_PRESERVED", gjson.GetBytes(normalized, "input.3.encrypted_content").String())
}

func TestSIWCHistoryCompatibilityNeverOmitsCurrentEncryptedAgentTask(t *testing.T) {
	for _, encrypted := range []string{
		`"encrypted_content":"PRIVATE_TASK","content":"Readable header"`,
		`"content":[{"type":"input_text","text":"Readable header"},{"type":"encrypted_content","data":"PRIVATE_TASK"}]`,
	} {
		for _, tail := range []string{"", `,{"type":"additional_tools","tools":[]}`, `,{"type":"configuration_update","reasoning":{"effort":"high"}}`, `,{"type":"tool_search_output"}`, `,{"type":"compaction_trigger"}`} {
			body := []byte(`{"input":[{"role":"user","content":"earlier"},{"type":"agent_message",` + encrypted + `}` + tail + `]}`)
			_, err := normalizeSIWCBodyWithHistoryCompatibility(body, true)
			require.Error(t, err)
			require.Contains(t, err.Error(), "current encrypted agent assignment at input[1]")
			require.NotContains(t, err.Error(), "PRIVATE_TASK")
		}
	}
}

func TestSIWCHistoryCompatibilityRetainsRolesPartsAndToolArguments(t *testing.T) {
	body := []byte(`{"input":[
		{"type":"message","role":"assistant","id":"msg_keep","author":"root","content":[{"type":"output_text","text":"answer"},{"type":"encrypted_content","data":"PRIVATE_ASSISTANT"}]},
		{"role":"user","content":[{"type":"input_text","text":"question"},{"type":"encrypted_content","data":"PRIVATE_USER"}]},
		{"type":"custom_tool_call","call_id":"custom_1","name":"edit","input":"encrypted_content is business text"},
		{"type":"custom_tool_call_output","call_id":"custom_1","output":[{"type":"input_text","text":"edited"},{"type":"encrypted_content","data":"PRIVATE_TOOL"}]},
		{"type":"compaction_summary","encrypted_content":"COMPACTION_PRESERVED"},
		{"type":"agent_message","author":"worker","recipient":"root","role":"developer","content":[{"type":"output_text","text":"plain latest result","annotations":[]}]}]}`)
	normalized, err := normalizeSIWCBodyWithHistoryCompatibility(body, true)
	require.NoError(t, err)
	require.NotContains(t, string(normalized), "PRIVATE_")
	items := gjson.GetBytes(normalized, "input").Array()
	require.Equal(t, "assistant", items[0].Get("role").String())
	require.Equal(t, "msg_keep", items[0].Get("id").String())
	require.Equal(t, "output_text", items[0].Get("content.2.type").String())
	require.Equal(t, "question", items[1].Get("content.0.text").String())
	require.Equal(t, "input_text", items[1].Get("content.1.type").String())
	require.Equal(t, "encrypted_content is business text", items[2].Get("input").String())
	require.Equal(t, "custom_1", items[3].Get("call_id").String())
	require.Equal(t, "edited", items[3].Get("output.0.text").String())
	require.Equal(t, "COMPACTION_PRESERVED", items[4].Get("encrypted_content").String())
	require.Equal(t, "user", items[5].Get("role").String())
	require.Equal(t, "input_text", items[5].Get("content.1.type").String())
	require.False(t, items[5].Get("content.1.annotations").Exists())
}

func TestSIWCHistoryCompatibilityStillRejectsOtherUnsupportedCapabilities(t *testing.T) {
	for _, body := range []string{`{"input":[{"type":"tool_search_call"}]}`, `{"input":[],"tools":[{"type":"computer"}]}`, `{"input":[{"type":"agent_message","content":{"type":"input_text","text":"PRIVATE_INVALID"}}]}`} {
		_, err := normalizeSIWCBodyWithHistoryCompatibility([]byte(body), true)
		require.Error(t, err)
		require.NotContains(t, err.Error(), "PRIVATE_INVALID")
	}
}

func TestSIWCHistoryCompatibilityAccountScope(t *testing.T) {
	account := &Account{Platform: PlatformOpenAI, Type: AccountTypeOAuth, Extra: map[string]any{OpenAISIWCEnabledKey: true}}
	require.False(t, account.IsSIWCIgnoreEncryptedContentEnabled())
	before := openAITurnRouteFingerprint(account)
	account.Extra[OpenAISIWCIgnoreEncryptedContentKey] = true
	require.True(t, account.IsSIWCIgnoreEncryptedContentEnabled())
	require.NotEqual(t, before, openAITurnRouteFingerprint(account), "revoking omission consent must invalidate an in-flight route snapshot")
	account.Extra[OpenAISIWCEnabledKey] = false
	require.False(t, account.IsSIWCIgnoreEncryptedContentEnabled())
	account.Extra[OpenAISIWCEnabledKey] = true
	account.Type = AccountTypeAPIKey
	require.False(t, account.IsSIWCIgnoreEncryptedContentEnabled())
	var missing *Account
	require.False(t, missing.IsSIWCIgnoreEncryptedContentEnabled())
}
