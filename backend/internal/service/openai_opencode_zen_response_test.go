//go:build unit

package service

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestOpenCodeZenChatAggregationPreservesReasoningToolsAndUsage(t *testing.T) {
	frames := []string{
		`{"id":"c1","created":9007199254740993,"model":"beam-free","service_tier":"default","choices":[{"index":0,"delta":{"role":"assistant","content":"hel","reasoning_content":"thi","tool_calls":[{"index":0,"id":"call_1","type":"function","function":{"name":"read","arguments":"{\"path\":"}}]},"finish_reason":null},{"index":1,"delta":{"content":"other"},"finish_reason":null}]}`,
		`{"choices":[{"index":0,"delta":{"content":"lo","reasoning_content":"nk","tool_calls":[{"index":0,"function":{"arguments":"\"a\"}"}}]},"finish_reason":"tool_calls","logprobs":{"content":[{"token":"hello"}]}},{"index":1,"delta":{},"finish_reason":"stop"}]}`,
		`{"choices":[],"usage":{"prompt_tokens":5,"completion_tokens":3,"total_tokens":8,"completion_tokens_details":{"reasoning_tokens":2}},"vendor":{"number":9007199254740993}}`,
		`[DONE]`,
	}
	var stream strings.Builder
	for _, frame := range frames {
		stream.WriteString("data: " + frame + "\n\n")
	}
	body, err := aggregateOpenCodeZenChatSSE([]byte(stream.String()))
	require.NoError(t, err)
	require.Equal(t, "chat.completion", gjson.GetBytes(body, "object").String())
	require.Equal(t, "9007199254740993", gjson.GetBytes(body, "created").Raw)
	require.Equal(t, "9007199254740993", gjson.GetBytes(body, "vendor.number").Raw)
	require.Equal(t, "hello", gjson.GetBytes(body, "choices.0.message.content").String())
	require.Equal(t, "think", gjson.GetBytes(body, "choices.0.message.reasoning_content").String())
	require.Equal(t, "other", gjson.GetBytes(body, "choices.1.message.content").String())
	require.Equal(t, "tool_calls", gjson.GetBytes(body, "choices.0.finish_reason").String())
	require.Equal(t, "read", gjson.GetBytes(body, "choices.0.message.tool_calls.0.function.name").String())
	require.Equal(t, `{"path":"a"}`, gjson.GetBytes(body, "choices.0.message.tool_calls.0.function.arguments").String())
	require.False(t, gjson.GetBytes(body, "choices.0.message.tool_calls.0.index").Exists())
	require.Equal(t, int64(2), gjson.GetBytes(body, "usage.completion_tokens_details.reasoning_tokens").Int())
	require.Equal(t, "hello", gjson.GetBytes(body, "choices.0.logprobs.content.0.token").String())
}

func TestOpenCodeZenChatAggregationRejectsErrorsAndTruncation(t *testing.T) {
	for _, stream := range []string{
		"data: {\"choices\":[{\"index\":0,\"delta\":{\"content\":\"partial\"},\"finish_reason\":null}]}\n\n",
		"data: {\"choices\":[{\"index\":0,\"delta\":{\"content\":\"partial\"}}]}\n\ndata: [DONE]\n\n",
		"data: [DONE]\n\n",
		"event: error\ndata: {\"error\":{\"message\":\"upstream failed\"}}\n\n",
	} {
		_, err := aggregateOpenCodeZenChatSSE([]byte(stream))
		require.Error(t, err)
	}
}
