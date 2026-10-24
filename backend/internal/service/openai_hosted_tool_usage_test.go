package service

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestOpenAIHostedSearchUsageJSONAndSSE(t *testing.T) {
	body := `{"output":[{"id":"ws1","type":"web_search_call","status":"completed"},{"id":"ws2","type":"web_search_call","status":"completed"},{"id":"fn1","type":"function_call","name":"web_search"},{"id":"ws3","type":"web_search_call","status":"failed"}],"usage":{"input_tokens":1000,"output_tokens":500}}`
	usage, ok := extractOpenAIUsageFromJSONBytes([]byte(body))
	require.True(t, ok)
	require.Equal(t, 2, usage.HostedWebSearchCalls)
	for _, terminal := range []string{"response.completed", "response.failed"} {
		t.Run(terminal, func(t *testing.T) {
			svc := &OpenAIGatewayService{}
			var stream OpenAIUsage
			svc.parseSSEUsage(`{"type":"response.output_item.added","item":{"id":"ws1","type":"web_search_call","status":"in_progress"}}`, &stream)
			require.Zero(t, stream.HostedWebSearchCalls)
			done := `{"type":"response.output_item.done","output_index":0,"item":{"id":"ws1","type":"web_search_call","status":"completed"}}`
			svc.parseSSEUsage(done, &stream)
			svc.parseSSEUsage(done, &stream)
			require.Equal(t, 1, stream.HostedWebSearchCalls, "partial stream meters only one completed call")
			svc.parseSSEUsage(`{"type":"`+terminal+`","response":`+body+`}`, &stream)
			require.Equal(t, 2, stream.HostedWebSearchCalls, "terminal snapshot does not double-charge done events")
			require.Equal(t, 1000, stream.InputTokens)
			require.Equal(t, 500, stream.OutputTokens)
		})
	}
}

func TestOpenAIHostedToolUsageWithoutTokensAndUnknownFees(t *testing.T) {
	usage, ok := extractOpenAIUsageFromJSONBytes([]byte(`{"output":[{"id":"ws1","type":"web_search_call","status":"completed"},{"id":"ci1","type":"code_interpreter_call","status":"completed"}]}`))
	require.True(t, ok)
	require.Equal(t, 1, usage.HostedWebSearchCalls)
	require.Equal(t, []string{"code_interpreter_call"}, usage.UnpricedHostedTools)
}

func TestOpenAIHostedSearchMalformedEventIsNotBillable(t *testing.T) {
	var usage OpenAIUsage
	(&OpenAIGatewayService{}).parseSSEUsage(`{"type":"response.output_item.done","item":{"id":"ws1","type":"web_search_call","status":"completed"}} trailing-invalid-json`, &usage)
	require.Zero(t, usage.HostedWebSearchCalls)
}
