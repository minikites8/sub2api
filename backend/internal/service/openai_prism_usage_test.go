package service

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/ctxkey"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestPrismBrowserUsageJSONAndSSE(t *testing.T) {
	request := []byte(`{"instructions":"Keep full UTF-8: 字😀","input":"Read the code.","tools":[{"type":"custom","name":"exec","format":{"type":"grammar","syntax":"lark","definition":"start: /.+/"}}]}`)
	for _, stream := range []bool{false, true} {
		for _, measured := range []bool{false, true} {
			t.Run(fmt.Sprintf("stream=%v/measured=%v", stream, measured), func(t *testing.T) {
				usageRaw := "null"
				if measured {
					usageRaw = `{"input_tokens":123,"output_tokens":45,"total_tokens":168,"input_tokens_details":{"cached_tokens":23}}`
				}
				terminal := []byte(`{"id":"resp_fixture","status":"completed","model":"gpt-6.1-sol","metadata":{"existing":"retained"},"usage":` + usageRaw + `,"output":[{"type":"custom_tool_call","name":"exec","input":"print('字😀')"}]}`)
				response := terminal
				if stream {
					response = []byte("event: response.completed\ndata: {\"type\":\"response.completed\",\"sequence_number\":0,\"response\":" + string(terminal) + "}\n\n")
				}
				raw, usage, source, err := prismBrowserUsage(request, response, "gpt-6.1-sol", stream)
				require.NoError(t, err)
				require.Positive(t, usage.InputTokens)
				require.Positive(t, usage.OutputTokens)
				require.Contains(t, string(raw), `"existing":"retained"`)
				require.Contains(t, string(raw), `"input":"print('字😀')"`)
				if measured {
					require.Equal(t, "upstream", source)
					require.Equal(t, OpenAIUsage{InputTokens: 123, OutputTokens: 45, CacheReadInputTokens: 23}, usage)
					require.Contains(t, string(raw), usageRaw)
				} else {
					require.Equal(t, "estimated", source)
					require.Zero(t, usage.CacheReadInputTokens)
					require.Contains(t, string(raw), `"cached_tokens":0`)
				}
			})
		}
	}
}

func TestPrismBrowserMeasuredUsageRejectsInvalidCounters(t *testing.T) {
	for _, raw := range []string{`null`, `{}`, `{"input_tokens":-1,"output_tokens":2}`,
		`{"input_tokens":1.5,"output_tokens":2}`, `{"input_tokens":1,"output_tokens":2,"input_tokens_details":{"cached_tokens":3}}`,
		`{"input_tokens":1e30,"output_tokens":2}`, `{"input_tokens":"1","output_tokens":2}`} {
		_, ok := prismBrowserMeasuredUsage(gjson.Parse(raw))
		require.False(t, ok, raw)
	}
	usage, ok := prismBrowserMeasuredUsage(gjson.Parse(`{"input_tokens":0,"output_tokens":0}`))
	require.True(t, ok)
	require.Equal(t, OpenAIUsage{}, usage)
}

func TestPrismBrowserCountsCustomHistoryAndGrammar(t *testing.T) {
	small, err := prismBrowserCountInput([]byte(`{"input":[{"type":"custom_tool_call","input":"short"}],"tools":[]}`), "gpt-6.1-sol")
	require.NoError(t, err)
	large, err := prismBrowserCountInput([]byte(`{"input":[{"type":"custom_tool_call","input":"a longer custom tool input with several words and characters 字😀"},{"type":"reasoning","summary":[{"text":"a readable summary"}]}],"tools":[{"type":"custom","format":{"type":"grammar","definition":"start: /.+/"}}]}`), "gpt-6.1-sol")
	require.NoError(t, err)
	require.Greater(t, large, small)
	first, err := prismBrowserCountInput([]byte(`{"input":"fixture","tools":[{"type":"function","name":"lookup","parameters":{"type":"object"}}]}`), "gpt-6.1-sol")
	require.NoError(t, err)
	second, err := prismBrowserCountInput([]byte(`{"tools": [ { "parameters": { "type": "object" }, "name": "lookup", "type": "function" } ], "input": "fixture"}`), "gpt-6.1-sol")
	require.NoError(t, err)
	require.Equal(t, first, second)
}

func TestPrismBrowserForwardRecordsUsageAndBills(t *testing.T) {
	for _, subscription := range []bool{false, true} {
		t.Run(fmt.Sprintf("subscription=%v", subscription), func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				_, _ = io.WriteString(w, `{"id":"resp_prism_billed","status":"completed","model":"gpt-5.1","reasoning":{"effort":"high"},"usage":null,"output":[{"content":[{"text":"Here are the project files and their purpose."}]}]}`)
			}))
			defer server.Close()
			forwarder, account := prismTestService(server.URL)
			c, _ := gin.CreateTestContext(httptest.NewRecorder())
			c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
			result, err := forwarder.forwardPrismBrowser(context.Background(), c, account,
				[]byte(`{"model":"gpt-5.1","input":"Read the project code.","reasoning":{"effort":"xhigh"}}`), time.Now())
			require.NoError(t, err)
			usageRepo := &openAIRecordUsageLogRepoStub{inserted: true}
			billingRepo := &openAIRecordUsageBillingRepoStub{}
			billing := newOpenAIRecordUsageServiceWithBillingRepoForTest(usageRepo, billingRepo,
				&openAIRecordUsageUserRepoStub{}, &openAIRecordUsageSubRepoStub{}, nil)
			input := &OpenAIRecordUsageInput{Result: result, User: &User{ID: 20}, Account: account,
				APIKeyService: &openAIRecordUsageAPIKeyQuotaStub{},
				APIKey:        &APIKey{ID: 7, Quota: 100, Group: &Group{RateMultiplier: 1.5}}}
			if subscription {
				input.APIKey.Group.SubscriptionType = SubscriptionTypeSubscription
				input.Subscription = &UserSubscription{ID: 10}
			}
			ctx := context.WithValue(context.Background(), ctxkey.RequestID, "gateway-http-attempt-1")
			require.NoError(t, billing.RecordUsage(ctx, input))
			require.Equal(t, 1, usageRepo.calls)
			require.Equal(t, "prism:resp_prism_billed", usageRepo.lastLog.RequestID)
			require.Equal(t, result.Usage.InputTokens, usageRepo.lastLog.InputTokens)
			require.Equal(t, result.Usage.OutputTokens, usageRepo.lastLog.OutputTokens)
			require.Equal(t, "high", *usageRepo.lastLog.ReasoningEffort)
			require.Positive(t, usageRepo.lastLog.ActualCost)
			require.Equal(t, "prism:resp_prism_billed", billingRepo.lastCmd.RequestID)
			require.Positive(t, billingRepo.lastCmd.APIKeyQuotaCost)
			if subscription {
				require.Positive(t, billingRepo.lastCmd.SubscriptionCost)
				require.Zero(t, billingRepo.lastCmd.BalanceCost)
			} else {
				require.Positive(t, billingRepo.lastCmd.BalanceCost)
			}
			firstFingerprint := billingRepo.lastCmd.RequestFingerprint
			billingRepo.result = &UsageBillingApplyResult{Applied: false}
			ctx = context.WithValue(context.Background(), ctxkey.RequestID, "gateway-http-attempt-2")
			input.RequestPayloadHash = "different-stream-encoding"
			require.NoError(t, billing.RecordUsage(ctx, input))
			require.Equal(t, "prism:resp_prism_billed", billingRepo.lastCmd.RequestID)
			require.Equal(t, firstFingerprint, billingRepo.lastCmd.RequestFingerprint)
		})
	}
}
