package service

import (
	"math"

	"github.com/shopspring/decimal"
)

// GPT Auto economics share one accounting foundation (this file) for the
// shadow pipeline, the test lab, the simulator, replay and the observed
// production view. All money is computed in decimal from per-million rates
// rounded to gptAutoRateScale, then rounded to gptAutoUSDScale (the precision
// of the NUMERIC(20,10) columns) before it leaves this file, so float noise
// such as 0.09999999999999999 can never reach storage or the UI.
//
// Three figures are kept strictly apart:
//
//	A. model gross margin       = (revenue - upstream model cost) / revenue
//	B. fully loaded gross margin = (revenue - upstream model cost - Jev cost) / revenue
//	C. shadow classification overhead = the Jev spend itself, shown on its own
//
// Phase 1 never bills customers for Jev, so C is an observation cost; B shows
// what the margin would be if every request also paid for one classification,
// as it would under live routing.

const (
	gptAutoUSDScale    = 10
	gptAutoRateScale   = 6
	gptAutoMarginScale = 6
)

// gptAutoMinOutputHistorySamples is the smallest usage-log sample accepted as
// a matched historical output estimate. Below it an average of a handful of
// requests (for example a few test pings answering "ok") would be presented
// as a reliable workload, which is exactly how a 10-token placeholder reached
// the test lab before.
const gptAutoMinOutputHistorySamples = 20

// Token estimate sources. Every token figure in an estimate carries one.
const (
	// GPTAutoEstimateSourceActual: read from the request's usage-log row.
	GPTAutoEstimateSourceActual = "actual_usage"
	// GPTAutoEstimateSourceAssumption: entered by an administrator for a
	// simulation; it describes a hypothetical workload, not traffic.
	GPTAutoEstimateSourceAssumption = "admin_assumption"
	// GPTAutoEstimateSourceHeuristic: characters ÷ 4 of the request text. For a
	// test-lab sample this is only the sample, without any system prompt or
	// tool schema a real client would send.
	GPTAutoEstimateSourceHeuristic = "heuristic_char_count"
	// GPTAutoEstimateSourceAssumedZero: cached input with no data; treated as
	// zero (all input billed and procured at the uncached rate).
	GPTAutoEstimateSourceAssumedZero = "assumed_zero"
)

// Overall estimate quality, from most to least trustworthy. Hypothetical
// economics are never "observed": the recommended model was not called.
const (
	GPTAutoEstimateQualityHistorical   = "historical"
	GPTAutoEstimateQualityAssumption   = "admin_assumption"
	GPTAutoEstimateQualityHeuristic    = "heuristic"
	GPTAutoEstimateQualityInsufficient = "insufficient_data"
)

// Margin statuses. A negative margin is a loss; a positive margin under the
// effective target is merely below target; an unknown margin is never shown
// as a number.
const (
	GPTAutoMarginUnknown     = "unknown"
	GPTAutoMarginLoss        = "loss"
	GPTAutoMarginBelowTarget = "below_target"
	GPTAutoMarginMeetsTarget = "meets_target"
)

// Customer rate sources.
const (
	GPTAutoRateSourceList     = "list_rate_assumed"
	GPTAutoRateSourceObserved = "observed_request_rate"
)

// Jev cost sources.
const (
	GPTAutoClassifierCostActual    = "actual_jev_call"
	GPTAutoClassifierCostEstimated = "estimated"
	GPTAutoClassifierCostExcluded  = "excluded"
)

// GPTAutoWorkloadAssumptions are administrator-entered token counts for a
// hypothetical workload. InputTokens excludes cached input, matching how
// usage logs and the billing engine split the two.
type GPTAutoWorkloadAssumptions struct {
	InputTokens       *int `json:"input_tokens,omitempty"`
	CachedInputTokens *int `json:"cached_input_tokens,omitempty"`
	OutputTokens      *int `json:"output_tokens,omitempty"`
}

func (a *GPTAutoWorkloadAssumptions) empty() bool {
	return a == nil || (a.InputTokens == nil && a.CachedInputTokens == nil && a.OutputTokens == nil)
}

func (a *GPTAutoWorkloadAssumptions) valid() bool {
	if a == nil {
		return true
	}
	for _, value := range []*int{a.InputTokens, a.CachedInputTokens, a.OutputTokens} {
		if value != nil && (*value < 0 || *value > gptAutoMaxSimulationTokens) {
			return false
		}
	}
	return true
}

// GPTAutoEconomics is a hypothetical per-request estimate for one model and
// effort. Nothing here is an observed charge or cost.
type GPTAutoEconomics struct {
	Hypothetical bool `json:"hypothetical"`

	Model             string  `json:"model"`
	Effort            string  `json:"effort"`
	BillingMultiplier float64 `json:"billing_multiplier"`
	// EffortBillingMultiplier mirrors the billing engine's per-effort price
	// multiplier from the pricing entry (1 when none is configured).
	EffortBillingMultiplier float64 `json:"effort_billing_multiplier"`
	// CustomerRateMultiplier is the customer's effective rate (group rate and
	// discounts). 1 unless observed from the request's usage-log row.
	CustomerRateMultiplier float64 `json:"customer_rate_multiplier"`
	CustomerRateSource     string  `json:"customer_rate_source"`
	// RevenueBasis says how EstimatedRevenue was priced: the gpt-auto tariff
	// (independent pricing; identical for every model) or the executed
	// model's list price times its GPT-Auto profile multiplier (legacy).
	RevenueBasis         string `json:"revenue_basis,omitempty"`
	TariffPricingVersion *int   `json:"tariff_pricing_version,omitempty"`

	EstimatedInputTokens       float64 `json:"estimated_input_tokens"`
	InputEstimateSource        string  `json:"input_estimate_source"`
	EstimatedCachedInputTokens float64 `json:"estimated_cached_input_tokens"`
	CachedInputEstimateSource  string  `json:"cached_input_estimate_source"`
	// EstimatedOutputTokens includes reasoning tokens: providers bill them as
	// output, and per-effort usage history captures how much effort adds.
	EstimatedOutputTokens float64 `json:"estimated_output_tokens"`
	OutputEstimateSource  string  `json:"output_estimate_source"`
	OutputEstimateSamples int64   `json:"output_estimate_samples"`
	EstimateQuality       string  `json:"estimate_quality"`

	// List prices from the gateway pricing table, USD per 1M tokens.
	ListInputPerM       float64 `json:"list_input_per_m"`
	ListCachedInputPerM float64 `json:"list_cached_input_per_m"`
	ListOutputPerM      float64 `json:"list_output_per_m"`

	// UpstreamChannel is the preferred hypothetical procurement channel, or
	// empty when the estimate assumes procurement at list price.
	UpstreamChannel         string  `json:"upstream_channel,omitempty"`
	UpstreamInputPerM       float64 `json:"upstream_input_per_m"`
	UpstreamCachedInputPerM float64 `json:"upstream_cached_input_per_m"`
	UpstreamOutputPerM      float64 `json:"upstream_output_per_m"`
	UpstreamCostSource      string  `json:"upstream_cost_source"`

	// A. Model economics (no classification overhead).
	EstimatedRevenue          float64  `json:"estimated_revenue"`
	EstimatedUpstreamCost     float64  `json:"estimated_upstream_cost"`
	EstimatedModelGrossProfit float64  `json:"estimated_model_gross_profit"`
	ModelGrossMargin          *float64 `json:"model_gross_margin,omitempty"`
	ModelMarginStatus         string   `json:"model_margin_status"`
	ModelBelowTarget          bool     `json:"model_below_target"`

	// C. Classification overhead included in B.
	EstimatedClassifierCost float64 `json:"estimated_classifier_cost"`
	ClassifierCostSource    string  `json:"classifier_cost_source"`

	// B. Fully loaded. The JSON names predate the split and are kept so stored
	// decisions keep their meaning: they always included the Jev cost.
	EstimatedGrossProfit    float64  `json:"estimated_gross_profit"`
	EstimatedGrossMargin    *float64 `json:"estimated_gross_margin,omitempty"`
	FullyLoadedMarginStatus string   `json:"fully_loaded_margin_status"`
	BelowTarget             bool     `json:"below_target"`

	TargetMargin float64 `json:"target_margin"`
	// Billing multipliers at which each margin would reach TargetMargin. Nil
	// when the estimate is insufficient or the list price is zero.
	MinRequiredMultiplier            *float64 `json:"min_required_multiplier,omitempty"`
	MinRequiredMultiplierFullyLoaded *float64 `json:"min_required_multiplier_fully_loaded,omitempty"`
	PricingAvailable                 bool     `json:"pricing_available"`
}

func gptAutoDecimal(value float64) decimal.Decimal {
	if math.IsNaN(value) || math.IsInf(value, 0) {
		return decimal.Zero
	}
	return decimal.NewFromFloat(value)
}

func gptAutoUSD(value decimal.Decimal) float64 {
	out, _ := value.Round(gptAutoUSDScale).Float64()
	return out
}

// gptAutoPerM converts a per-token price to a rounded per-million rate.
func gptAutoPerM(perToken float64) decimal.Decimal {
	return gptAutoDecimal(perToken).Mul(decimal.NewFromInt(gptAutoTokensPerMillion)).Round(gptAutoRateScale)
}

func gptAutoRate(value decimal.Decimal) float64 {
	out, _ := value.Round(gptAutoRateScale).Float64()
	return out
}

// gptAutoTokenCost prices tokens at a per-million rate.
func gptAutoTokenCost(tokens float64, perM decimal.Decimal) decimal.Decimal {
	return gptAutoDecimal(tokens).Mul(perM).Div(decimal.NewFromInt(gptAutoTokensPerMillion))
}

// GPTAutoMarginStatus classifies a margin against its target.
func GPTAutoMarginStatus(margin *float64, target float64) string {
	switch {
	case margin == nil:
		return GPTAutoMarginUnknown
	case *margin < 0:
		return GPTAutoMarginLoss
	case *margin < target:
		return GPTAutoMarginBelowTarget
	default:
		return GPTAutoMarginMeetsTarget
	}
}

func gptAutoMargin(profit, revenue decimal.Decimal) *float64 {
	if !revenue.IsPositive() {
		return nil
	}
	margin, _ := profit.DivRound(revenue, gptAutoMarginScale+2).Round(gptAutoMarginScale).Float64()
	return &margin
}

// gptAutoRequiredMultiplier solves cost / ((1 - target) × base) for the
// billing multiplier that reaches the target margin.
func gptAutoRequiredMultiplier(cost, base decimal.Decimal, target float64) *float64 {
	if !base.IsPositive() || target >= 1 {
		return nil
	}
	denominator := decimal.NewFromInt(1).Sub(gptAutoDecimal(target)).Mul(base)
	if !denominator.IsPositive() {
		return nil
	}
	required, _ := cost.DivRound(denominator, 8).Round(4).Float64()
	return &required
}

func gptAutoQualityRank(quality string) int {
	switch quality {
	case GPTAutoEstimateQualityHistorical:
		return 3
	case GPTAutoEstimateQualityAssumption:
		return 2
	case GPTAutoEstimateQualityHeuristic:
		return 1
	default:
		return 0
	}
}

func gptAutoSourceQuality(source string) string {
	switch source {
	case GPTAutoEstimateSourceActual, GPTAutoOutputSourceModelEffort, GPTAutoOutputSourceModel:
		return GPTAutoEstimateQualityHistorical
	case GPTAutoEstimateSourceAssumption:
		return GPTAutoEstimateQualityAssumption
	case GPTAutoEstimateSourceHeuristic:
		return GPTAutoEstimateQualityHeuristic
	default:
		return GPTAutoEstimateQualityInsufficient
	}
}

// EstimateGPTAutoEconomics computes hypothetical per-request economics for a
// model/effort.
//
//	list      = uncached_in × list_in + cached_in × list_cached + out × list_out
//	revenue   = list × billing_multiplier × effort_multiplier × customer_rate
//	upstream  = the same tokens at the cheapest enabled channel, else list
//	A margin  = (revenue - upstream) / revenue
//	B margin  = (revenue - upstream - jev) / revenue
//
// Margins are nil (status "unknown") whenever pricing or any token estimate
// is insufficient; a margin built on a placeholder is never produced.
func EstimateGPTAutoEconomics(in GPTAutoPolicyInput, model, effort string) GPTAutoEconomics {
	cfg := in.Config
	profile, _ := cfg.ProfileFor(model)
	out := GPTAutoEconomics{
		Hypothetical:            true,
		Model:                   model,
		Effort:                  effort,
		BillingMultiplier:       profile.BillingMultiplier,
		EffortBillingMultiplier: 1,
		CustomerRateMultiplier:  1,
		CustomerRateSource:      GPTAutoRateSourceList,
		EstimatedClassifierCost: gptAutoUSD(gptAutoDecimal(in.ClassifierCostUSD)),
		ClassifierCostSource:    in.ClassifierCostSource,
		TargetMargin:            cfg.MinMarginFor(model),
		OutputEstimateSource:    GPTAutoOutputSourceNone,
		UpstreamCostSource:      GPTAutoCostSourceNone,
		ModelMarginStatus:       GPTAutoMarginUnknown,
		FullyLoadedMarginStatus: GPTAutoMarginUnknown,
	}
	if out.ClassifierCostSource == "" {
		out.ClassifierCostSource = GPTAutoClassifierCostActual
	}
	if in.CustomerRateMultiplier > 0 || in.CustomerRateMultiplier == 0 && in.CustomerRateSource != "" {
		out.CustomerRateMultiplier = in.CustomerRateMultiplier
		out.CustomerRateSource = in.CustomerRateSource
	}

	// Token estimates, each with its source.
	assumptions := in.Assumptions
	switch {
	case assumptions != nil && assumptions.InputTokens != nil:
		out.EstimatedInputTokens = float64(*assumptions.InputTokens)
		out.InputEstimateSource = GPTAutoEstimateSourceAssumption
	case in.Features.EstimatedInputTokens > 0:
		out.EstimatedInputTokens = float64(in.Features.EstimatedInputTokens)
		out.InputEstimateSource = in.Features.InputEstimateSource
		if out.InputEstimateSource == "" {
			out.InputEstimateSource = GPTAutoEstimateSourceHeuristic
		}
	default:
		out.InputEstimateSource = GPTAutoOutputSourceNone
	}
	switch {
	case assumptions != nil && assumptions.CachedInputTokens != nil:
		out.EstimatedCachedInputTokens = float64(*assumptions.CachedInputTokens)
		out.CachedInputEstimateSource = GPTAutoEstimateSourceAssumption
	case in.Features.InputEstimateSource == GPTAutoEstimateSourceActual && (assumptions == nil || assumptions.InputTokens == nil):
		out.EstimatedCachedInputTokens = float64(in.Features.CachedInputTokens)
		out.CachedInputEstimateSource = GPTAutoEstimateSourceActual
	default:
		out.CachedInputEstimateSource = GPTAutoEstimateSourceAssumedZero
	}
	switch {
	case assumptions != nil && assumptions.OutputTokens != nil:
		out.EstimatedOutputTokens = float64(*assumptions.OutputTokens)
		out.OutputEstimateSource = GPTAutoEstimateSourceAssumption
	case in.OutputEstimate != nil:
		estimate := in.OutputEstimate(model, effort)
		out.OutputEstimateSamples = estimate.Samples
		if estimate.Tokens > 0 && estimate.Samples >= gptAutoMinOutputHistorySamples &&
			(estimate.Source == GPTAutoOutputSourceModelEffort || estimate.Source == GPTAutoOutputSourceModel) {
			out.EstimatedOutputTokens = estimate.Tokens
			out.OutputEstimateSource = estimate.Source
		}
	}
	out.EstimateQuality = GPTAutoEstimateQualityHistorical
	for _, source := range []string{out.InputEstimateSource, out.OutputEstimateSource} {
		if quality := gptAutoSourceQuality(source); gptAutoQualityRank(quality) < gptAutoQualityRank(out.EstimateQuality) {
			out.EstimateQuality = quality
		}
	}
	if out.EstimatedInputTokens+out.EstimatedCachedInputTokens+out.EstimatedOutputTokens <= 0 {
		out.EstimateQuality = GPTAutoEstimateQualityInsufficient
	}

	var pricing *ModelPricing
	if in.Pricing != nil {
		pricing, _ = in.Pricing(model)
	}
	if pricing == nil || (pricing.InputPricePerToken <= 0 && pricing.OutputPricePerToken <= 0) {
		out.EstimateQuality = GPTAutoEstimateQualityInsufficient
		return out
	}
	out.PricingAvailable = true
	listIn, listCached, listOut := gptAutoPerM(pricing.InputPricePerToken), gptAutoPerM(pricing.CacheReadPricePerToken), gptAutoPerM(pricing.OutputPricePerToken)
	out.ListInputPerM, out.ListCachedInputPerM, out.ListOutputPerM = gptAutoRate(listIn), gptAutoRate(listCached), gptAutoRate(listOut)
	out.EffortBillingMultiplier = reasoningEffortBillingMultiplier(effort, pricing.ReasoningEffortMultipliers)

	// Procurement: the cheapest enabled channel; a channel without a cached
	// rate is assumed to charge cached input at its full input rate (the
	// conservative choice). Without channels, procurement is at list price.
	upIn, upCached, upOut := listIn, listCached, listOut
	out.UpstreamCostSource = GPTAutoCostSourceListPrice
	if channel, ok := gptAutoCheapestChannel(profile.UpstreamCosts, out.EstimatedInputTokens, out.EstimatedCachedInputTokens, out.EstimatedOutputTokens); ok {
		upIn = gptAutoDecimal(channel.InputPerM).Round(gptAutoRateScale)
		upOut = gptAutoDecimal(channel.OutputPerM).Round(gptAutoRateScale)
		upCached = upIn
		if channel.CachedInputPerM > 0 {
			upCached = gptAutoDecimal(channel.CachedInputPerM).Round(gptAutoRateScale)
		}
		out.UpstreamChannel = channel.Channel
		out.UpstreamCostSource = GPTAutoCostSourceChannel
	}
	out.UpstreamInputPerM, out.UpstreamCachedInputPerM, out.UpstreamOutputPerM = gptAutoRate(upIn), gptAutoRate(upCached), gptAutoRate(upOut)

	if out.EstimateQuality == GPTAutoEstimateQualityInsufficient {
		return out
	}

	list := gptAutoTokenCost(out.EstimatedInputTokens, listIn).
		Add(gptAutoTokenCost(out.EstimatedCachedInputTokens, listCached)).
		Add(gptAutoTokenCost(out.EstimatedOutputTokens, listOut))
	pricedList := list.Mul(gptAutoDecimal(out.EffortBillingMultiplier)).Mul(gptAutoDecimal(out.CustomerRateMultiplier))
	revenue := pricedList.Mul(gptAutoDecimal(profile.BillingMultiplier)).Round(gptAutoUSDScale)
	out.RevenueBasis = GPTAutoRevenueBasisLegacy
	if tariff := gptAutoEconomicsTariff(in); tariff != nil {
		// Independent pricing: the customer pays the gpt-auto tariff whatever
		// model runs; the profile multiplier and per-effort pricing of the
		// executed model do not apply. Procurement stays the executed model.
		pricedList = tariff.ListCharge(GPTAutoTariffUsage{
			InputTokens:       int(out.EstimatedInputTokens),
			CachedInputTokens: int(out.EstimatedCachedInputTokens),
			OutputTokens:      int(out.EstimatedOutputTokens),
		}).Mul(gptAutoDecimal(out.CustomerRateMultiplier))
		revenue = pricedList.Round(gptAutoUSDScale)
		out.BillingMultiplier = 1
		out.EffortBillingMultiplier = 1
		out.RevenueBasis = GPTAutoRevenueBasisTariff
		version := tariff.Version
		out.TariffPricingVersion = &version
	}
	upstream := gptAutoTokenCost(out.EstimatedInputTokens, upIn).
		Add(gptAutoTokenCost(out.EstimatedCachedInputTokens, upCached)).
		Add(gptAutoTokenCost(out.EstimatedOutputTokens, upOut)).Round(gptAutoUSDScale)
	classifier := gptAutoDecimal(out.EstimatedClassifierCost)

	modelProfit := revenue.Sub(upstream)
	fullProfit := modelProfit.Sub(classifier)
	out.EstimatedRevenue = gptAutoUSD(revenue)
	out.EstimatedUpstreamCost = gptAutoUSD(upstream)
	out.EstimatedModelGrossProfit = gptAutoUSD(modelProfit)
	out.EstimatedGrossProfit = gptAutoUSD(fullProfit)
	out.ModelGrossMargin = gptAutoMargin(modelProfit, revenue)
	out.EstimatedGrossMargin = gptAutoMargin(fullProfit, revenue)
	out.ModelMarginStatus = GPTAutoMarginStatus(out.ModelGrossMargin, out.TargetMargin)
	out.FullyLoadedMarginStatus = GPTAutoMarginStatus(out.EstimatedGrossMargin, out.TargetMargin)
	out.ModelBelowTarget = out.ModelMarginStatus == GPTAutoMarginLoss || out.ModelMarginStatus == GPTAutoMarginBelowTarget
	out.BelowTarget = out.FullyLoadedMarginStatus == GPTAutoMarginLoss || out.FullyLoadedMarginStatus == GPTAutoMarginBelowTarget
	out.MinRequiredMultiplier = gptAutoRequiredMultiplier(upstream, pricedList, out.TargetMargin)
	out.MinRequiredMultiplierFullyLoaded = gptAutoRequiredMultiplier(upstream.Add(classifier), pricedList, out.TargetMargin)
	return out
}

// Revenue bases of an economics estimate.
const (
	GPTAutoRevenueBasisTariff = "gpt_auto_tariff"
	GPTAutoRevenueBasisLegacy = "executed_model_list_x_profile_multiplier"
)

func gptAutoEconomicsTariff(in GPTAutoPolicyInput) *GPTAutoTariff {
	if in.Tariff != nil {
		if in.Tariff.Mode == GPTAutoPricingModeFixed {
			return in.Tariff
		}
		return nil
	}
	if !in.Config.Pricing.IsFixed() {
		return nil
	}
	tariff, ok := in.Config.Pricing.TariffFor(nil)
	if !ok {
		return nil
	}
	return &tariff
}

func gptAutoCheapestChannel(channels []GPTAutoUpstreamCost, inputTokens, cachedTokens, outputTokens float64) (GPTAutoUpstreamCost, bool) {
	var best GPTAutoUpstreamCost
	bestCost := math.Inf(1)
	for _, channel := range channels {
		if !channel.Enabled {
			continue
		}
		cached := channel.CachedInputPerM
		if cached <= 0 {
			cached = channel.InputPerM
		}
		cost := inputTokens*channel.InputPerM + cachedTokens*cached + outputTokens*channel.OutputPerM
		// Without a token estimate every channel costs zero; fall back to the
		// cheaper unit price so the preferred channel is still meaningful.
		if inputTokens == 0 && cachedTokens == 0 && outputTokens == 0 {
			cost = channel.InputPerM + channel.OutputPerM
		}
		if cost < bestCost {
			best, bestCost = channel, cost
		}
	}
	return best, !math.IsInf(bestCost, 1)
}

// gptAutoClassifierCostUSD prices a Jev call from the gateway pricing table,
// falling back to the documented built-in jev-latest rate.
func gptAutoClassifierCostUSD(pricing GPTAutoPricingLookup, model string, inputTokens, outputTokens int) float64 {
	inputPerM := decimal.NewFromFloat(gptAutoJevFallbackInputPerM)
	outputPerM := decimal.Zero
	if pricing != nil {
		if p, err := pricing(model); err == nil && p != nil && (p.InputPricePerToken > 0 || p.OutputPricePerToken > 0) {
			inputPerM, outputPerM = gptAutoPerM(p.InputPricePerToken), gptAutoPerM(p.OutputPricePerToken)
		}
	}
	return gptAutoUSD(gptAutoTokenCost(float64(inputTokens), inputPerM).Add(gptAutoTokenCost(float64(outputTokens), outputPerM)))
}

// gptAutoJevFallbackInputPerM is the built-in jev-latest input price
// ($0.042 per 1M tokens, docs/typesafe-jev.md); output is free.
const gptAutoJevFallbackInputPerM = 0.042

// GPTAutoObservedEconomics is the real outcome of the production request the
// decision observed, from its usage-log row: what the customer was charged
// for the model that actually served it, the recorded upstream account cost,
// and the shadow Jev spend. Unlike GPTAutoEconomics it is not hypothetical.
type GPTAutoObservedEconomics struct {
	Model             string   `json:"model"`
	ChargedRevenue    float64  `json:"charged_revenue"`
	AccountCost       float64  `json:"account_cost"`
	ClassifierCost    float64  `json:"classifier_cost"`
	ModelGrossProfit  float64  `json:"model_gross_profit"`
	ModelGrossMargin  *float64 `json:"model_gross_margin,omitempty"`
	FullyLoadedProfit float64  `json:"fully_loaded_profit"`
	FullyLoadedMargin *float64 `json:"fully_loaded_margin,omitempty"`
	// EffectiveRate is charged ÷ list cost for this request (group rate,
	// discounts); nil when the list cost is zero.
	EffectiveRate *float64 `json:"effective_rate,omitempty"`
}

// NewGPTAutoObservedEconomics derives observed figures from a usage row.
func NewGPTAutoObservedEconomics(actual *GPTAutoActualUsage, classifierCost float64) *GPTAutoObservedEconomics {
	if actual == nil {
		return nil
	}
	revenue := gptAutoDecimal(actual.ChargedCost).Round(gptAutoUSDScale)
	cost := gptAutoDecimal(actual.AccountCost).Round(gptAutoUSDScale)
	classifier := gptAutoDecimal(classifierCost).Round(gptAutoUSDScale)
	out := &GPTAutoObservedEconomics{
		Model:             actual.Model,
		ChargedRevenue:    gptAutoUSD(revenue),
		AccountCost:       gptAutoUSD(cost),
		ClassifierCost:    gptAutoUSD(classifier),
		ModelGrossProfit:  gptAutoUSD(revenue.Sub(cost)),
		ModelGrossMargin:  gptAutoMargin(revenue.Sub(cost), revenue),
		FullyLoadedProfit: gptAutoUSD(revenue.Sub(cost).Sub(classifier)),
		FullyLoadedMargin: gptAutoMargin(revenue.Sub(cost).Sub(classifier), revenue),
	}
	if list := gptAutoDecimal(actual.ListCost); list.IsPositive() {
		rate, _ := revenue.DivRound(list, 8).Round(6).Float64()
		out.EffectiveRate = &rate
	}
	return out
}
