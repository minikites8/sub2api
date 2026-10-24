package service

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// gptAutoEconomicsInput prices every pool model at list in/out per M (plus a
// cached rate) with one procurement channel for gpt-6-luna.
func gptAutoEconomicsInput(inPerM, outPerM, cachedPerM float64, mutate func(*GPTAutoShadowConfig)) GPTAutoPolicyInput {
	cfg := DefaultGPTAutoShadowConfig()
	if mutate != nil {
		mutate(cfg)
	}
	return GPTAutoPolicyInput{
		Config:   *cfg,
		Features: GPTAutoRequestFeatures{Endpoint: GPTAutoEndpointChatCompletions},
		Pricing: func(string) (*ModelPricing, error) {
			return &ModelPricing{InputPricePerToken: inPerM / 1e6, OutputPricePerToken: outPerM / 1e6, CacheReadPricePerToken: cachedPerM / 1e6}, nil
		},
	}
}

func setProfile(model string, apply func(*GPTAutoModelProfile)) func(*GPTAutoShadowConfig) {
	return func(c *GPTAutoShadowConfig) {
		for i := range c.ModelProfiles {
			if c.ModelProfiles[i].Model == model {
				apply(&c.ModelProfiles[i])
			}
		}
	}
}

func TestGPTAutoReproducesReportedTinyRequestMargin(t *testing.T) {
	// Reported test-lab result: revenue $0.000006, upstream $0.000005, Jev
	// $0.00005292 (1260 input tokens at $0.042/M), 1 input / 10 output tokens.
	// $0.5/M in and $0.55/M out at 1.1× gives exactly that workload's figures.
	input := gptAutoEconomicsInput(0.5, 0.55, 0, setProfile(GPTAutoModelGPT6Luna, func(p *GPTAutoModelProfile) { p.BillingMultiplier = 1.0 }))
	input.Assumptions = &GPTAutoWorkloadAssumptions{InputTokens: intPtr(1), OutputTokens: intPtr(10)}
	input.ClassifierCostUSD = gptAutoClassifierCostUSD(nil, "jev-latest", 1260, 233)
	require.Equal(t, 0.00005292, input.ClassifierCostUSD, "Jev output is free; cost is exact, without float noise")

	economics := EstimateGPTAutoEconomics(input, GPTAutoModelGPT6Luna, GPTAutoEffortNone)
	require.Equal(t, 0.000006, economics.EstimatedRevenue)
	require.Equal(t, 0.000006, economics.EstimatedUpstreamCost, "list-price procurement with a 1.0 multiplier")
	require.Equal(t, GPTAutoMarginBelowTarget, economics.ModelMarginStatus)

	// Same numbers as reported: revenue 0.000006, upstream 0.000005.
	input = gptAutoEconomicsInput(0.5, 0.55, 0, setProfile(GPTAutoModelGPT6Luna, func(p *GPTAutoModelProfile) {
		p.BillingMultiplier = 1.0
		p.UpstreamCosts = []GPTAutoUpstreamCost{{Channel: "c", Enabled: true, InputPerM: 0.5, OutputPerM: 0.45}}
	}))
	input.Assumptions = &GPTAutoWorkloadAssumptions{InputTokens: intPtr(1), OutputTokens: intPtr(10)}
	input.ClassifierCostUSD = 0.00005292
	economics = EstimateGPTAutoEconomics(input, GPTAutoModelGPT6Luna, GPTAutoEffortNone)
	require.Equal(t, 0.000006, economics.EstimatedRevenue)
	require.Equal(t, 0.000005, economics.EstimatedUpstreamCost)
	// A. model margin is positive; B. fully loaded is the reported -865.3%.
	require.InDelta(t, 1.0/6.0, *economics.ModelGrossMargin, 1e-6)
	require.InDelta(t, -8.653333, *economics.EstimatedGrossMargin, 1e-6)
	require.Equal(t, GPTAutoMarginLoss, economics.FullyLoadedMarginStatus, "negative margins are not clamped")
	require.Equal(t, GPTAutoEstimateQualityAssumption, economics.EstimateQuality, "labelled as a simulation, not production")
	require.True(t, economics.Hypothetical)
	require.Equal(t, 0.00005292, economics.EstimatedClassifierCost, "C. Jev overhead kept separately")
}

func TestGPTAutoPositiveMarginBelowTargetIsNotALoss(t *testing.T) {
	// 33.1% against an effective 43% target (38% + 5% buffer).
	input := gptAutoEconomicsInput(1, 0, 0, func(c *GPTAutoShadowConfig) {
		c.GlobalMinMargin, c.ProfitSafetyBuffer = 0.38, 0.05
		setProfile(GPTAutoModelGPT6Luna, func(p *GPTAutoModelProfile) {
			p.BillingMultiplier = 1
			p.UpstreamCosts = []GPTAutoUpstreamCost{{Channel: "c", Enabled: true, InputPerM: 0.669, OutputPerM: 0}}
		})(c)
	})
	input.Assumptions = &GPTAutoWorkloadAssumptions{InputTokens: intPtr(1_000_000), OutputTokens: intPtr(0)}
	economics := EstimateGPTAutoEconomics(input, GPTAutoModelGPT6Luna, GPTAutoEffortLow)
	require.InDelta(t, 0.331, *economics.ModelGrossMargin, 1e-9)
	require.InDelta(t, 0.43, economics.TargetMargin, 1e-12)
	require.Equal(t, GPTAutoMarginBelowTarget, economics.ModelMarginStatus)
	require.Equal(t, GPTAutoMarginBelowTarget, economics.FullyLoadedMarginStatus)
	decision := EvaluateGPTAutoPolicy(GPTAutoPolicyInput{
		Classification: *gptAutoSampleClassification(GPTAutoModelGPT6Luna, GPTAutoEffortLow),
		Features:       input.Features, Config: input.Config, Pricing: input.Pricing, Assumptions: input.Assumptions,
	})
	require.Contains(t, gptAutoWarningRules(decision), GPTAutoRuleModelMarginBelowTarget)
	require.NotContains(t, gptAutoWarningRules(decision), GPTAutoRuleModelMarginLoss)
	require.Empty(t, decision.Adjustments, "below target is a warning, not a route modification")
	require.Equal(t, GPTAutoModelGPT6Luna, decision.FinalModel)
}

func TestGPTAutoZeroRevenueAndMissingHistoryAreUnknown(t *testing.T) {
	// Zero list price: margin undefined, never 0% or -100%.
	input := gptAutoEconomicsInput(0, 0, 0, nil)
	input.Assumptions = &GPTAutoWorkloadAssumptions{InputTokens: intPtr(100), OutputTokens: intPtr(100)}
	economics := EstimateGPTAutoEconomics(input, GPTAutoModelGPT6Luna, GPTAutoEffortLow)
	require.False(t, economics.PricingAvailable)
	require.Nil(t, economics.ModelGrossMargin)
	require.Nil(t, economics.EstimatedGrossMargin)
	require.Equal(t, GPTAutoMarginUnknown, economics.ModelMarginStatus)

	// Assumed zero tokens: revenue zero, margin unknown.
	input = gptAutoEconomicsInput(2, 10, 0, nil)
	input.Assumptions = &GPTAutoWorkloadAssumptions{InputTokens: intPtr(0), OutputTokens: intPtr(0)}
	economics = EstimateGPTAutoEconomics(input, GPTAutoModelGPT6Luna, GPTAutoEffortLow)
	require.Equal(t, GPTAutoEstimateQualityInsufficient, economics.EstimateQuality)
	require.Nil(t, economics.EstimatedGrossMargin)

	// Too little output history (3 samples of 10 tokens): no output estimate,
	// no margin, explicit warning; the 10-token placeholder is not used.
	input = gptAutoEconomicsInput(2, 10, 0, nil)
	input.Features.EstimatedInputTokens = 1
	input.OutputEstimate = func(string, string) GPTAutoOutputEstimate {
		return GPTAutoOutputEstimate{Tokens: 10, Source: GPTAutoOutputSourceModelEffort, Samples: 3}
	}
	economics = EstimateGPTAutoEconomics(input, GPTAutoModelGPT6Luna, GPTAutoEffortNone)
	require.Equal(t, GPTAutoOutputSourceNone, economics.OutputEstimateSource)
	require.EqualValues(t, 3, economics.OutputEstimateSamples)
	require.Zero(t, economics.EstimatedOutputTokens)
	require.Equal(t, GPTAutoEstimateQualityInsufficient, economics.EstimateQuality)
	require.Nil(t, economics.ModelGrossMargin)
	require.Nil(t, economics.EstimatedGrossMargin)
	require.Equal(t, []string{GPTAutoRuleInsufficientEstimate}, gptAutoWarningRules(GPTAutoPolicyDecision{Warnings: gptAutoEconomicsWarnings(economics)}))

	// Enough history: heuristic input + matched history → heuristic quality.
	input.OutputEstimate = func(string, string) GPTAutoOutputEstimate {
		return GPTAutoOutputEstimate{Tokens: 400, Source: GPTAutoOutputSourceModelEffort, Samples: 200}
	}
	input.Features.EstimatedInputTokens = 900
	economics = EstimateGPTAutoEconomics(input, GPTAutoModelGPT6Luna, GPTAutoEffortNone)
	require.Equal(t, GPTAutoEstimateSourceHeuristic, economics.InputEstimateSource)
	require.Equal(t, GPTAutoEstimateQualityHeuristic, economics.EstimateQuality)
	require.NotNil(t, economics.ModelGrossMargin)
}

func TestGPTAutoCachedInputPricing(t *testing.T) {
	// List: $2/M in, $0.5/M cached, $10/M out. Channel: $1/M in, $0.25/M
	// cached, $5/M out.
	input := gptAutoEconomicsInput(2, 10, 0.5, setProfile(GPTAutoModelGPT6Luna, func(p *GPTAutoModelProfile) {
		p.BillingMultiplier = 1.5
		p.UpstreamCosts = []GPTAutoUpstreamCost{{Channel: "c", Enabled: true, InputPerM: 1, CachedInputPerM: 0.25, OutputPerM: 5}}
	}))
	input.Assumptions = &GPTAutoWorkloadAssumptions{InputTokens: intPtr(200_000), CachedInputTokens: intPtr(800_000), OutputTokens: intPtr(100_000)}
	economics := EstimateGPTAutoEconomics(input, GPTAutoModelGPT6Luna, GPTAutoEffortMedium)
	// list = 0.4 + 0.4 + 1.0 = 1.8; revenue = 2.7; upstream = 0.2 + 0.2 + 0.5 = 0.9
	require.Equal(t, 2.7, economics.EstimatedRevenue)
	require.Equal(t, 0.9, economics.EstimatedUpstreamCost)
	require.Equal(t, 0.5, economics.ListCachedInputPerM)
	require.Equal(t, 0.25, economics.UpstreamCachedInputPerM)
	require.InDelta(t, 2.0/3.0, *economics.ModelGrossMargin, 1e-6)

	// A channel without a cached rate is assumed to charge full input price.
	input.Config.ModelProfiles[1].UpstreamCosts[0].CachedInputPerM = 0
	economics = EstimateGPTAutoEconomics(input, GPTAutoModelGPT6Luna, GPTAutoEffortMedium)
	require.Equal(t, 1.0, economics.UpstreamCachedInputPerM)
	require.Equal(t, 1.5, economics.EstimatedUpstreamCost)
}

func TestGPTAutoMonetaryPrecision(t *testing.T) {
	// $0.1/M stored per token as 1e-7 multiplies back to 0.09999999999999999
	// in float64; the economics must report 0.1 exactly.
	input := gptAutoEconomicsInput(0.1, 0.3, 0.025, nil)
	input.Assumptions = &GPTAutoWorkloadAssumptions{InputTokens: intPtr(3), OutputTokens: intPtr(7)}
	economics := EstimateGPTAutoEconomics(input, GPTAutoModelGPT6Luna, GPTAutoEffortLow)
	require.Equal(t, 0.1, economics.ListInputPerM)
	require.Equal(t, 0.3, economics.ListOutputPerM)
	require.Equal(t, 0.025, economics.ListCachedInputPerM)
	raw, err := json.Marshal(economics)
	require.NoError(t, err)
	for _, artifact := range []string{"9999999", "0000001", "e-0"} {
		require.NotContains(t, string(raw), artifact)
	}
	// 3×0.1/M + 7×0.3/M = 0.0000024 list, ×1.1 = 0.00000264.
	require.Equal(t, 0.00000264, economics.EstimatedRevenue)
	require.Equal(t, 0.0000024, economics.EstimatedUpstreamCost)
}

func TestGPTAutoTestLabNeverPresentsPlaceholderWorkloadAsReliable(t *testing.T) {
	svc, repo, _, _ := newGPTAutoTestService(t, nil)
	// A handful of tiny historical answers, below the sample minimum.
	repo.history = []GPTAutoOutputHistory{{Model: GPTAutoModelGPT6Luna, Effort: GPTAutoEffortMedium, AvgOutput: 10, Samples: 3}}
	ctx := context.Background()

	result, err := svc.RunTest(ctx, GPTAutoTestInput{Prompt: "hi"})
	require.NoError(t, err)
	economics := result.Decision.Economics
	require.EqualValues(t, 1, economics.EstimatedInputTokens, "2 characters ÷ 4, rounded up")
	require.Equal(t, GPTAutoEstimateSourceHeuristic, economics.InputEstimateSource)
	require.Equal(t, GPTAutoOutputSourceNone, economics.OutputEstimateSource)
	require.Equal(t, GPTAutoEstimateQualityInsufficient, economics.EstimateQuality)
	require.Nil(t, economics.EstimatedGrossMargin, "no confident margin from a placeholder")
	require.Greater(t, result.ClassifierCost, 0.0, "actual Jev cost is preserved separately")

	// Explicit simulation assumptions produce a labelled estimate.
	result, err = svc.RunTest(ctx, GPTAutoTestInput{Prompt: "hi", Assumptions: &GPTAutoWorkloadAssumptions{
		InputTokens: intPtr(3000), CachedInputTokens: intPtr(1000), OutputTokens: intPtr(800),
	}})
	require.NoError(t, err)
	economics = result.Decision.Economics
	require.Equal(t, GPTAutoEstimateQualityAssumption, economics.EstimateQuality)
	require.EqualValues(t, 1000, economics.EstimatedCachedInputTokens)
	require.NotNil(t, economics.ModelGrossMargin)
	require.NotNil(t, economics.EstimatedGrossMargin)
	require.Greater(t, *economics.ModelGrossMargin, *economics.EstimatedGrossMargin, "fully loaded includes Jev")

	_, err = svc.RunTest(ctx, GPTAutoTestInput{Prompt: "hi", Assumptions: &GPTAutoWorkloadAssumptions{InputTokens: intPtr(-1)}})
	require.Error(t, err)
}

func TestGPTAutoCompareRunsTwoBudgetedJevCallsWithoutActivatingDraft(t *testing.T) {
	svc, _, _, settings := newGPTAutoTestService(t, nil)
	classifier := &recordingClassifier{}
	svc.classifier = classifier
	ctx := context.Background()
	before, ok := svc.budget.(*localGPTAutoBudgetStore)
	require.True(t, ok)
	day := gptAutoDayStart(svc.now()).Format("2006-01-02")

	result, err := svc.CompareClassifierInstructions(ctx, GPTAutoCompareInput{
		GPTAutoTestInput:  GPTAutoTestInput{Prompt: "Write a function that reverses a list"},
		DraftInstructions: gptAutoCustomInstructions(),
	})
	require.NoError(t, err)
	require.Equal(t, 2, classifier.count(), "saved and draft are separate Jev calls")
	require.Equal(t, 2, result.JevCalls)
	require.NotNil(t, result.Saved.Result)
	require.NotNil(t, result.Draft.Result)
	require.Equal(t, GPTAutoModelGPT61Sol, result.Saved.Result.Classification.Model)
	require.Equal(t, GPTAutoModelGPT6Luna, result.Draft.Result.Classification.Model)
	require.NotEqual(t, result.Saved.ClassifierConfigHash, result.Draft.ClassifierConfigHash)
	require.InDelta(t, result.Saved.Result.ClassifierCost+result.Draft.Result.ClassifierCost, result.TotalClassifierCost, 1e-12)
	usage, _, _ := before.Usage(ctx, day)
	require.EqualValues(t, 2, usage.Calls, "both arms are counted against the cluster budget")
	require.False(t, strings.HasPrefix(classifier.calls[0].ClassifierInstructions.ModelSelection, "CUSTOM"))
	require.True(t, strings.HasPrefix(classifier.calls[1].ClassifierInstructions.ModelSelection, "CUSTOM"))

	saved, err := settings.GetGPTAutoShadowConfig(ctx)
	require.NoError(t, err)
	require.True(t, sameGPTAutoInstructionText(DefaultGPTAutoClassifierInstructions(), saved.ClassifierInstructions), "draft is never activated")

	// A draft identical to the saved text is rejected without any call.
	_, err = svc.CompareClassifierInstructions(ctx, GPTAutoCompareInput{
		GPTAutoTestInput: GPTAutoTestInput{Prompt: "x"}, DraftInstructions: DefaultGPTAutoClassifierInstructions(),
	})
	require.Error(t, err)
	require.Equal(t, 2, classifier.count())
}

func TestGPTAutoCompareHonoursDailyCallLimitPerArm(t *testing.T) {
	svc, _, _, _ := newGPTAutoTestService(t, func(c *GPTAutoShadowConfig) { c.ClassifierDailyCallLimit = 1 })
	classifier := &recordingClassifier{}
	svc.classifier = classifier
	result, err := svc.CompareClassifierInstructions(context.Background(), GPTAutoCompareInput{
		GPTAutoTestInput: GPTAutoTestInput{Prompt: "hello"}, DraftInstructions: gptAutoCustomInstructions(),
	})
	require.NoError(t, err)
	require.Equal(t, 1, classifier.count(), "the second arm is refused by the budget")
	require.NotNil(t, result.Saved.Result)
	require.Nil(t, result.Draft.Result)
	require.Equal(t, "GPT_AUTO_BUDGET_EXHAUSTED", result.Draft.ErrorCode)
	require.Equal(t, 1, result.JevCalls)
}

func TestGPTAutoObservedEconomicsFromUsageLog(t *testing.T) {
	observed := NewGPTAutoObservedEconomics(&GPTAutoActualUsage{Model: GPTAutoModelGPT56Sol, ListCost: 0.01, ChargedCost: 0.015, AccountCost: 0.006}, 0.0000126)
	require.Equal(t, 0.009, observed.ModelGrossProfit)
	require.InDelta(t, 0.6, *observed.ModelGrossMargin, 1e-9)
	require.InDelta(t, (0.009-0.0000126)/0.015, *observed.FullyLoadedMargin, 1e-6)
	require.InDelta(t, 1.5, *observed.EffectiveRate, 1e-9)
	zero := NewGPTAutoObservedEconomics(&GPTAutoActualUsage{}, 0.00005)
	require.Nil(t, zero.ModelGrossMargin, "zero revenue: margin unknown")
	require.Nil(t, zero.EffectiveRate)
}

func TestGPTAutoOutputEstimatorRequiresMinimumSamples(t *testing.T) {
	svc, repo, _, _ := newGPTAutoTestService(t, nil)
	repo.history = []GPTAutoOutputHistory{
		{Model: GPTAutoModelGPT6Luna, Effort: GPTAutoEffortHigh, AvgOutput: 10, Samples: 5},
		{Model: GPTAutoModelGPT6Luna, Effort: GPTAutoEffortLow, AvgOutput: 300, Samples: 40},
		{Model: GPTAutoModelGPT56Luna, Effort: GPTAutoEffortLow, AvgOutput: 50, Samples: 4},
	}
	estimate := svc.outputEstimator()
	high := estimate(GPTAutoModelGPT6Luna, GPTAutoEffortHigh)
	require.Equal(t, GPTAutoOutputSourceModel, high.Source, "thin effort history falls back to model-level history")
	require.EqualValues(t, 45, high.Samples)
	require.InDelta(t, (10*5+300*40)/45.0, high.Tokens, 1e-9)
	thin := estimate(GPTAutoModelGPT56Luna, GPTAutoEffortLow)
	require.Equal(t, GPTAutoOutputSourceNone, thin.Source)
	require.EqualValues(t, 4, thin.Samples)
}

func TestGPTAutoDecisionDetailReestimatesWithActualInput(t *testing.T) {
	svc, repo, _, _ := newGPTAutoTestService(t, nil)
	repo.history = []GPTAutoOutputHistory{{Model: GPTAutoModelGPT6Luna, Effort: GPTAutoEffortMedium, AvgOutput: 500, Samples: 100}}
	svc.Start()
	observeTestRequest(svc, gptAutoTestBody)
	repo.waitRows(t, 1)
	repo.actual["req-1"] = &GPTAutoActualUsage{Model: "gpt-5.6-sol", InputTokens: 1200, CacheReadTokens: 300, OutputTokens: 800, ListCost: 0.01, ChargedCost: 0.008, AccountCost: 0.006}
	detail, err := svc.GetDecision(context.Background(), repo.rows[0].ID)
	require.NoError(t, err)
	require.NotNil(t, detail.ObservedEconomics)
	require.InDelta(t, 0.25, *detail.ObservedEconomics.ModelGrossMargin, 1e-9)
	actual := detail.ActualInputEconomics
	require.NotNil(t, actual)
	require.Equal(t, GPTAutoEstimateSourceActual, actual.InputEstimateSource)
	require.EqualValues(t, 1200, actual.EstimatedInputTokens)
	require.EqualValues(t, 300, actual.EstimatedCachedInputTokens)
	require.Equal(t, GPTAutoOutputSourceModelEffort, actual.OutputEstimateSource, "output is still an estimate")
	require.Equal(t, GPTAutoRateSourceObserved, actual.CustomerRateSource)
	require.InDelta(t, 0.8, actual.CustomerRateMultiplier, 1e-9)
	require.Equal(t, GPTAutoEstimateQualityHistorical, actual.EstimateQuality)
}
