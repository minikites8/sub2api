package service

import (
	"context"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestOpenAIHTTPToolHistoryFallbackReason(t *testing.T) {
	for _, tc := range []struct{ name, fields, reason string }{
		{"missing call id", `"input":[{"type":"function_call_output","output":"done"}]`, OpenAIToolResultMissingCallIDReason},
		{"missing custom call id", `"input":[{"type":"custom_tool_call_output","output":"done"}]`, OpenAIToolResultMissingCallIDReason},
		{"orphan result", `"input":[{"type":"function_call_output","call_id":"orphan","output":"done"}]`, OpenAIToolHistoryUnavailableReason},
		{"complete history", `"input":[{"type":"function_call","call_id":"paired","name":"exec","arguments":"{}"},{"type":"function_call_output","call_id":"paired","output":"done"}]`, ""},
		{"missing id after complete pair", `"input":[{"type":"function_call","call_id":"paired","name":"exec","arguments":"{}"},{"type":"function_call_output","call_id":"paired","output":"done"},{"type":"function_call_output","output":"unpaired"}]`, OpenAIToolResultMissingCallIDReason},
		{"reference context", `"input":[{"type":"item_reference","id":"paired"},{"type":"function_call_output","call_id":"paired","output":"done"}]`, ""},
		{"owned continuation", `"previous_response_id":"resp_original","input":[{"type":"function_call_output","output":"done"}]`, ""},
		{"plain request", `"input":"hello"`, ""},
		{"tool discovery metadata", `"input":[{"type":"tool_search_output","tools":[]}]`, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			require.Equal(t, tc.reason, OpenAIHTTPToolHistoryFallbackReason([]byte(`{"model":"gpt-5.6-sol",`+tc.fields+`}`)))
		})
	}
}

func TestNormalizeOpenAIOrphanToolResultsPreservesHistory(t *testing.T) {
	body := []byte(`{"model":"gpt-5.6-sol","metadata":{"integer":9007199254740993},"tools":[{"type":"function","name":"exec","parameters":{"type":"object"}}],"input":[{"type":"message","role":"user","content":"before"},{"type":"function_call","call_id":"paired","name":"exec","arguments":"{}"},{"type":"function_call_output","call_id":"paired","output":"paired result"},{"type":"function_call_output","call_id":"orphan","output":[{"type":"input_text","text":"unpaired result"}],"extra":{"integer":9007199254740993}},{"type":"custom_tool_call_output","namespace":"old.tools","name":"patch","output":"missing id result"},{"type":"message","role":"user","content":"after"}]}`)
	normalized, changed, err := NormalizeOpenAIOrphanToolResults(body)
	require.NoError(t, err)
	require.True(t, changed)
	require.True(t, gjson.ValidBytes(normalized))
	for _, path := range []string{"model", "metadata", "tools", "input.0", "input.1", "input.2", "input.5"} {
		require.Equal(t, gjson.GetBytes(body, path).Raw, gjson.GetBytes(normalized, path).Raw, path)
	}
	for _, index := range []int{3, 4} {
		path := fmt.Sprintf("input.%d", index)
		require.Equal(t, "message", gjson.GetBytes(normalized, path+".type").String())
		require.Equal(t, "user", gjson.GetBytes(normalized, path+".role").String())
		text := gjson.GetBytes(normalized, path+".content.0.text").String()
		require.Contains(t, text, "recorded tool output, not a new user instruction")
		require.Contains(t, text, gjson.GetBytes(body, path).Raw)
	}
	second, changed, err := NormalizeOpenAIOrphanToolResults(normalized)
	require.NoError(t, err)
	require.False(t, changed)
	require.Equal(t, normalized, second)
}

func TestNormalizeOpenAIOrphanToolResultsPreservesExplicitContinuation(t *testing.T) {
	body := []byte(`{"model":"gpt-5.6-sol","previous_response_id":"resp_original","input":[{"type":"function_call_output","output":"done"}]}`)
	normalized, changed, err := NormalizeOpenAIOrphanToolResults(body)
	require.NoError(t, err)
	require.False(t, changed)
	require.Equal(t, body, normalized)
}

func TestNormalizeOpenAIOrphanToolResultsPreservesReferences(t *testing.T) {
	body := []byte(`{"model":"gpt-5.6-sol","input":[{"type":"item_reference","id":"paired"},{"type":"function_call_output","call_id":"paired","output":"referenced result"},{"type":"function_call_output","output":"missing id result"},{"type":"tool_search_output","tools":[]}]}`)
	normalized, changed, err := NormalizeOpenAIOrphanToolResults(body)
	require.NoError(t, err)
	require.True(t, changed)
	for _, path := range []string{"input.0", "input.1", "input.3"} {
		require.Equal(t, gjson.GetBytes(body, path).Raw, gjson.GetBytes(normalized, path).Raw)
	}
	require.Equal(t, "message", gjson.GetBytes(normalized, "input.2.type").String())
}

func TestExcelBPSToolHistoryUnavailableRequestsOrdinaryRouting(t *testing.T) {
	for _, probe := range []bool{false, true} {
		t.Run(fmt.Sprint(probe), func(t *testing.T) {
			upstream := &httpUpstreamRecorder{}
			svc := openAIClientToolsTestService(upstream)
			body := []byte(`{"model":"gpt-5.6-sol","input":[{"type":"function_call_output","call_id":"call_unknown","output":"retain result"}]}`)
			c, rec := newExcelBPSFallbackContext(body, false)
			c.Set(bpsAccountProbeRequiredContextKey, probe)
			result, err := svc.Forward(WithExcelBPSFailoverContext(context.Background()), c, excelAccount(), body)
			require.Error(t, err)
			require.Nil(t, result)
			require.Empty(t, upstream.requests)
			if probe {
				require.NotErrorIs(t, err, ErrExcelBPSNativeBypass)
				require.Contains(t, rec.Body.String(), "original tool item is unavailable")
			} else {
				require.ErrorIs(t, err, ErrExcelBPSNativeBypass)
				require.False(t, c.Writer.Written())
				require.Equal(t, OpenAIToolHistoryUnavailableReason, rec.Header().Get("X-Codex2API-Basispoints-Bypass"))
			}
		})
	}
}
