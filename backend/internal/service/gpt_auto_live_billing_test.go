package service

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// TestGPTAutoLiveBillingUsesExecutedModel runs the real OpenAI usage pipeline
// twice for the same upstream result: once as a direct request for the model
// and once as a gpt-auto request routed to it. The routed request must be
// priced on the executed model, scaled once by its GPT-Auto multiplier, and
// recorded under the virtual model name; list cost and tokens are identical.
func TestGPTAutoLiveBillingUsesExecutedModel(t *testing.T) {
	result := func() *OpenAIForwardResult {
		return &OpenAIForwardResult{
			RequestID: "resp_gpt_auto_bill",
			Model:     GPTAutoModelGPT61Sol,
			Usage:     OpenAIUsage{InputTokens: 1200, OutputTokens: 800, CacheReadInputTokens: 300},
			Duration:  time.Second,
		}
	}
	record := func(ctx context.Context) (*UsageLog, *UsageBillingCommand) {
		usageRepo := &openAIRecordUsageLogRepoStub{inserted: true}
		billingRepo := &openAIRecordUsageBillingRepoStub{}
		svc := newOpenAIRecordUsageServiceWithBillingRepoForTest(usageRepo, billingRepo, &openAIRecordUsageUserRepoStub{}, &openAIRecordUsageSubRepoStub{}, nil)
		require.NoError(t, svc.RecordUsage(ctx, &OpenAIRecordUsageInput{
			Result:  result(),
			APIKey:  &APIKey{ID: 10},
			User:    &User{ID: 20},
			Account: &Account{ID: 30, Platform: PlatformOpenAI},
		}))
		require.NotNil(t, usageRepo.lastLog)
		return usageRepo.lastLog, billingRepo.lastCmd
	}

	direct, directCmd := record(context.Background())
	route := &GPTAutoRoute{model: GPTAutoModelGPT61Sol, billingMultiplier: 1.5, row: &GPTAutoShadowDecision{}}
	routed, routedCmd := record(WithGPTAutoRoute(context.Background(), route))

	require.Greater(t, direct.ActualCost, 0.0, "the executed model has a price")
	require.Equal(t, GPTAutoModelGPT61Sol, routed.Model, "billed and recorded on the model that ran")
	require.Equal(t, GPTAutoVirtualModel, routed.RequestedModel)
	require.Equal(t, direct.RequestedModel, GPTAutoModelGPT61Sol)
	require.Equal(t, direct.InputTokens, routed.InputTokens)
	require.Equal(t, direct.CacheReadTokens, routed.CacheReadTokens, "cached input is billed at the cached rate, unchanged")
	require.Equal(t, direct.OutputTokens, routed.OutputTokens)
	require.InDelta(t, direct.TotalCost, routed.TotalCost, 1e-12, "list cost is not scaled")
	require.InDelta(t, direct.ActualCost*1.5, routed.ActualCost, 1e-9)
	require.InDelta(t, direct.RateMultiplier*1.5, routed.RateMultiplier, 1e-9)
	require.NotNil(t, directCmd)
	require.NotNil(t, routedCmd)
	require.InDelta(t, directCmd.BalanceCost*1.5, routedCmd.BalanceCost, 1e-9, "the balance deduction is the routed charge, once")

	// An unrouted request is never affected by GPT-Auto.
	again, _ := record(WithGPTAutoRoute(context.Background(), nil))
	require.InDelta(t, direct.ActualCost, again.ActualCost, 1e-12)
}

// Exercise the actual settlement command, including mutually exclusive token
// buckets, independently of the selected model and its legacy multiplier.
func TestGPTAutoFixedTariffActualSettlement(t *testing.T) {
	tariff, ok := gptAutoExampleTariff(t).TariffFor(nil)
	require.True(t, ok)
	for _, model := range GPTAutoModelPool() {
		for _, stream := range []bool{false, true} {
			t.Run(model+map[bool]string{false: "/json", true: "/stream-partial"}[stream], func(t *testing.T) {
				logs := &openAIRecordUsageLogRepoStub{inserted: true}
				ledger := &openAIRecordUsageBillingRepoStub{}
				svc := newOpenAIRecordUsageServiceWithBillingRepoForTest(logs, ledger, &openAIRecordUsageUserRepoStub{}, &openAIRecordUsageSubRepoStub{}, nil)
				rate := 0.8
				route := &GPTAutoRoute{model: model, tariff: tariff, billingMultiplier: 1, customerRate: &rate, row: &GPTAutoShadowDecision{}}
				ctx := WithGPTAutoRoute(context.Background(), route)
				gid := int64(7)
				input := &OpenAIRecordUsageInput{
					Result: &OpenAIForwardResult{RequestID: "fixed-settlement", Model: model, Stream: stream, Usage: OpenAIUsage{InputTokens: 1000, CacheReadInputTokens: 200, OutputTokens: 500}, Duration: time.Second},
					APIKey: &APIKey{ID: 10, GroupID: &gid, Group: &Group{RateMultiplier: 9, ModelRateMultipliers: map[string]float64{model: 7}}},
					User:   &User{ID: 20}, Account: &Account{ID: 30, Platform: PlatformOpenAI},
				}
				require.NoError(t, svc.RecordUsage(ctx, input))
				require.InDelta(t, 0.00456, logs.lastLog.ActualCost, 1e-12)
				require.InDelta(t, 0.00456, ledger.lastCmd.BalanceCost, 1e-12)
				require.Equal(t, 1, ledger.calls)
				require.Equal(t, 800, logs.lastLog.InputTokens)
				require.Equal(t, 200, logs.lastLog.CacheReadTokens)
				require.Equal(t, 500, logs.lastLog.OutputTokens)
				require.Equal(t, GPTAutoVirtualModel, logs.lastLog.RequestedModel)
				require.Equal(t, model, logs.lastLog.Model)
				require.Equal(t, 0.8, logs.lastLog.RateMultiplier)
				require.Greater(t, logs.lastLog.TotalCost, 0.0)
			})
		}
	}
}

func TestGPTAutoFreeFastPreservesFlexDiscount(t *testing.T) {
	tariff, _ := gptAutoExampleTariff(t).TariffFor(nil)
	for _, tc := range []struct {
		tier   string
		charge float64
	}{{"priority", 0.0057}, {"fast", 0.0057}, {"flex", 0.00285}} {
		t.Run(tc.tier, func(t *testing.T) {
			logs := &openAIRecordUsageLogRepoStub{inserted: true}
			ledger := &openAIRecordUsageBillingRepoStub{}
			svc := newOpenAIRecordUsageServiceWithBillingRepoForTest(logs, ledger, &openAIRecordUsageUserRepoStub{}, &openAIRecordUsageSubRepoStub{}, nil)
			rate := 1.0
			route := &GPTAutoRoute{tariff: tariff, freeFast: true, customerRate: &rate, billingMultiplier: 1, row: &GPTAutoShadowDecision{}}
			err := svc.RecordUsage(WithGPTAutoRoute(context.Background(), route), &OpenAIRecordUsageInput{
				Result: &OpenAIForwardResult{Model: GPTAutoModelGPT61Sol, RequestID: "tier", ServiceTier: &tc.tier, Usage: OpenAIUsage{InputTokens: 1000, CacheReadInputTokens: 200, OutputTokens: 500}},
				APIKey: &APIKey{ID: 1}, User: &User{ID: 2}, Account: &Account{ID: 3, Platform: PlatformOpenAI},
			})
			require.NoError(t, err)
			require.InDelta(t, tc.charge, logs.lastLog.ActualCost, 1e-12)
		})
	}
}

func TestGPTAutoHostedSearchPassesThroughProcurementOnce(t *testing.T) {
	tariff, _ := gptAutoExampleTariff(t).TariffFor(nil)
	for _, model := range GPTAutoModelPool() {
		for _, mode := range []string{"hosted", "alpha", "search", "zero", "search-only"} {
			t.Run(model+"/"+mode, func(t *testing.T) {
				logs := &openAIRecordUsageLogRepoStub{inserted: true}
				ledger := &openAIRecordUsageBillingRepoStub{}
				svc := newOpenAIRecordUsageServiceWithBillingRepoForTest(logs, ledger, &openAIRecordUsageUserRepoStub{}, &openAIRecordUsageSubRepoStub{}, nil)
				rate, accountRate, customerAccountRate := 0.8, 0.5, 2.0
				route := &GPTAutoRoute{model: model, tariff: tariff, customerRate: &rate, billingMultiplier: 9, row: &GPTAutoShadowDecision{}}
				result := &OpenAIForwardResult{Model: model, RequestID: "hosted-search", Usage: OpenAIUsage{InputTokens: 1000, CacheReadInputTokens: 200, OutputTokens: 500}}
				price, per1k := 0.01, 10.0
				extra, textCharge := 0.01, 0.00912
				switch mode {
				case "hosted":
					result.Usage.HostedWebSearchCalls = 2
				case "alpha":
					result.WebSearchCalls = 2
					result.Usage.HostedWebSearchCalls = 2
				case "search":
					result.SearchCount = 2
				case "zero":
					result.Usage.HostedWebSearchCalls = 2
					price, extra = 0, 0
				case "search-only":
					result.Usage = OpenAIUsage{HostedWebSearchCalls: 2}
					textCharge = 0
				}
				gid := int64(7)
				err := svc.RecordUsage(WithGPTAutoRoute(context.Background(), route), &OpenAIRecordUsageInput{
					Result: result, APIKey: &APIKey{ID: 1, GroupID: &gid, Group: &Group{RateMultiplier: 7, WebSearchPricePerCall: &price, SearchPricePer1k: &per1k}},
					User: &User{ID: 2}, Account: &Account{ID: 3, Platform: PlatformOpenAI, RateMultiplier: &accountRate, GroupRateMultiplier: &customerAccountRate},
				})
				require.NoError(t, err)
				require.InDelta(t, textCharge+extra, logs.lastLog.ActualCost, 1e-12)
				require.InDelta(t, textCharge+extra, ledger.lastCmd.BalanceCost, 1e-12)
				require.Equal(t, 1, ledger.calls)
				require.GreaterOrEqual(t, logs.lastLog.TotalCost, extra/accountRate)
			})
		}
	}
}

func TestGPTAutoGeneratedMediaUsesProcurementCost(t *testing.T) {
	tariff, _ := gptAutoExampleTariff(t).TariffFor(nil)
	logs := &openAIRecordUsageLogRepoStub{inserted: true}
	ledger := &openAIRecordUsageBillingRepoStub{}
	svc := newOpenAIRecordUsageServiceWithBillingRepoForTest(logs, ledger, &openAIRecordUsageUserRepoStub{}, &openAIRecordUsageSubRepoStub{}, nil)
	rate, accountRate := 7.0, 0.5
	route := &GPTAutoRoute{tariff: tariff, customerRate: &rate, billingMultiplier: 9, row: &GPTAutoShadowDecision{}}
	err := svc.RecordUsage(WithGPTAutoRoute(context.Background(), route), &OpenAIRecordUsageInput{
		Result: &OpenAIForwardResult{Model: "gpt-image-1", RequestID: "hosted-image", ImageCount: 1, ImageSize: "1024x1024", Usage: OpenAIUsage{InputTokens: 100, ImageOutputTokens: 100}},
		APIKey: &APIKey{ID: 1}, User: &User{ID: 2}, Account: &Account{ID: 3, Platform: PlatformOpenAI, RateMultiplier: &accountRate},
	})
	require.NoError(t, err)
	require.Greater(t, logs.lastLog.ActualCost, 0.0)
	require.InDelta(t, logs.lastLog.TotalCost*accountRate, logs.lastLog.ActualCost, 1e-12)
	require.InDelta(t, logs.lastLog.ActualCost, ledger.lastCmd.BalanceCost, 1e-12)
	require.Equal(t, 1, ledger.calls)
}
