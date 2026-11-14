package service

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestSIWCNamespaceDescriptions(t *testing.T) {
	body := []byte(`{"tools":[
		{"type":"namespace","name":"existing","description":"Keep this description","tools":[{"type":"function","name":"read","parameters":{"type":"object"}}]},
		{"type":"namespace","name":"missing","tools":[{"type":"custom","name":"edit"}]},
		{"type":"function","name":"exec","parameters":{"type":"object"}}
	],"input":[{"type":"function_call","namespace":"existing","name":"read","call_id":"call_1","arguments":"{}"}]}`)
	normalized, err := normalizeSIWCBody(body)
	require.NoError(t, err)
	require.Equal(t, "Keep this description", gjson.GetBytes(normalized, "tools.0.description").String())
	for _, tool := range gjson.GetBytes(normalized, "tools").Array() {
		require.Equal(t, "namespace", tool.Get("type").String())
		require.NotEmpty(t, tool.Get("description").String(), "namespace %s requires a description", tool.Get("name"))
	}
	require.Equal(t, "existing", gjson.GetBytes(normalized, "input.0.namespace").String())
	require.Equal(t, "read", gjson.GetBytes(normalized, "input.0.name").String())
	second, err := normalizeSIWCBody(normalized)
	require.NoError(t, err)
	require.JSONEq(t, string(normalized), string(second))
}

func TestSIWCAdditionalToolsDescriptions(t *testing.T) {
	normalized, err := normalizeSIWCBody([]byte(`{"input":[{"type":"additional_tools","role":"developer","tools":[
		{"type":"namespace","name":"codex_app","tools":[{"type":"function","name":"read_thread","parameters":{"type":"object"}}]},
		{"type":"custom","name":"apply_patch"}
	]}]}`))
	require.NoError(t, err)
	tools := gjson.GetBytes(normalized, "input.0.tools").Array()
	require.Len(t, tools, 2)
	for _, tool := range tools {
		require.Equal(t, "namespace", tool.Get("type").String())
		require.NotEmpty(t, tool.Get("description").String())
	}
	require.Equal(t, "additional_tools", gjson.GetBytes(normalized, "input.0.type").String())
}

func TestSIWCSupportedHistoryIsPreserved(t *testing.T) {
	body := `{"input":[
		{"type":"message","role":"system","content":"rules"},
		{"role":"user","content":[{"type":"input_text","text":"look"},{"type":"input_image","image_url":"data:image/png;base64,fixture"},{"type":"input_file","file_data":"fixture"}]},
		{"type":"reasoning","encrypted_content":"keep-reasoning"},
		{"type":"compaction_summary","encrypted_content":"keep-summary"},
		{"type":"function_call","namespace":"functions","name":"read","call_id":"c1","arguments":"{}"},
		{"type":"function_call_output","call_id":"c1","output":"file content"},
		{"type":"custom_tool_call","name":"apply_patch","call_id":"c2","input":"patch"},
		{"type":"custom_tool_call_output","call_id":"c2","output":"patched"},
		{"type":"item_reference","id":"item_fixture"},
		{"type":"web_search_call","id":"ws_fixture"},
		{"type":"programmatic_tool_call","id":"ptc_fixture"}
	]}`
	normalized, err := normalizeSIWCBody([]byte(body))
	require.NoError(t, err)
	want := gjson.Get(body, "input").Array()
	got := gjson.GetBytes(normalized, "input").Array()
	require.Len(t, got, len(want))
	require.Equal(t, "developer", got[0].Get("role").String())
	for index := 1; index < len(want); index++ {
		require.JSONEq(t, want[index].Raw, got[index].Raw)
	}
}

func TestSIWCNamespaceDescriptionFillsNullAndMergesFunctions(t *testing.T) {
	normalized, err := normalizeSIWCBody([]byte(`{"tools":[
		{"type":"namespace","name":"functions","description":null,"tools":[{"type":"function","name":"read"}]},
		{"type":"custom","name":"edit"}
	]}`))
	require.NoError(t, err)
	require.Len(t, gjson.GetBytes(normalized, "tools").Array(), 1)
	require.NotEmpty(t, gjson.GetBytes(normalized, "tools.0.description").String())
	require.Equal(t, "read", gjson.GetBytes(normalized, "tools.0.tools.0.name").String())
	require.Equal(t, "edit", gjson.GetBytes(normalized, "tools.0.tools.1.name").String())
}

func TestSIWCUnsupportedInputReportsSafePathAndType(t *testing.T) {
	for _, typ := range []string{"agent_message", "tool_search_call", "tool_search_output", "local_shell_call", "computer_call", "mcp_call", "compaction_trigger"} {
		t.Run(typ, func(t *testing.T) {
			body := []byte(`{"input":[{"role":"user","content":"SECRET_PROMPT"},{"type":"` + typ + `","encrypted_content":"SECRET_HISTORY"}]}`)
			_, err := normalizeSIWCBody(body)
			require.Error(t, err)
			require.Equal(t, "OPENAI_SIWC_UNSUPPORTED_INPUT", infraerrors.Reason(err))
			require.Contains(t, err.Error(), "input[1]")
			require.Contains(t, err.Error(), typ)
			require.NotContains(t, err.Error(), "SECRET")
		})
	}
}

func TestSIWCUnsupportedInputHidesMalformedType(t *testing.T) {
	body, err := json.Marshal(map[string]any{"input": []any{map[string]any{"type": "SECRET\n" + strings.Repeat("X", 100)}}})
	require.NoError(t, err)
	_, err = normalizeSIWCBody(body)
	require.Error(t, err)
	require.Contains(t, err.Error(), "unknown")
	require.NotContains(t, err.Error(), "SECRET")
}

func TestSIWCUnsupportedInputNeverSendsOrBecomesGenericGatewayError(t *testing.T) {
	body := []byte(`{"model":"gpt-6-astra","stream":true,"input":[{"type":"agent_message","encrypted_content":"SECRET_HISTORY"}]}`)
	upstream := &httpUpstreamRecorder{}
	gateway := openAIClientToolsTestService(upstream)
	response := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(response)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", strings.NewReader(string(body)))
	account := &Account{ID: 42, Platform: PlatformOpenAI, Type: AccountTypeOAuth, Status: StatusActive, Schedulable: true,
		Credentials: map[string]any{"access_token": "must-not-send"}, Extra: map[string]any{OpenAISIWCEnabledKey: true}}

	_, err := gateway.Forward(context.Background(), ctx, account, body)

	require.Error(t, err)
	require.Empty(t, upstream.requests)
	require.Equal(t, http.StatusBadRequest, response.Code)
	require.Equal(t, "OPENAI_SIWC_UNSUPPORTED_INPUT", gjson.Get(response.Body.String(), "error.code").String())
	require.Contains(t, gjson.Get(response.Body.String(), "error.message").String(), "agent_message")
	require.NotContains(t, response.Body.String(), "SECRET_HISTORY")
}
