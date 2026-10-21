package service

import (
	"math"
	"sort"
	"strings"
)

// GPTAutoClassification is the raw Jev output for one request. It is stored
// verbatim on the decision row so the policy can be replayed later without
// another classifier call.
type GPTAutoClassification struct {
	ClassifierModel string `json:"classifier_model"`

	Model              string             `json:"model"`
	ModelConfidence    float64            `json:"model_confidence"`
	ModelProbabilities map[string]float64 `json:"model_probabilities,omitempty"`

	Effort              string             `json:"effort"`
	EffortConfidence    float64            `json:"effort_confidence"`
	EffortProbabilities map[string]float64 `json:"effort_probabilities,omitempty"`

	// Complexity is Jev's probability-weighted expected rubric level, 0..4.
	Complexity              float64            `json:"complexity"`
	ComplexityConfidence    float64            `json:"complexity_confidence"`
	ComplexityProbabilities map[string]float64 `json:"complexity_probabilities,omitempty"`

	Category              string             `json:"category"`
	CategoryConfidence    float64            `json:"category_confidence"`
	CategoryProbabilities map[string]float64 `json:"category_probabilities,omitempty"`

	InputTokens  int `json:"input_tokens"`
	OutputTokens int `json:"output_tokens"`
}

// GPTAutoRequestFeatures are the request properties the policy may rely on.
// They come from the sanitized snapshot, never the raw body.
type GPTAutoRequestFeatures struct {
	Endpoint string `json:"endpoint"`
	HasTools bool   `json:"has_tools"`
	// EstimatedInputTokens is a character-based approximation of the
	// customer's prompt size (see gptAutoEstimateTokens).
	EstimatedInputTokens int `json:"estimated_input_tokens"`
}

// GPTAutoOutputEstimate is a historical output-token estimate for a
// model/effort pair. Source records which history the number came from so the
// dashboard can label it honestly.
type GPTAutoOutputEstimate struct {
	Tokens  float64 `json:"tokens"`
	Source  string  `json:"source"`
	Samples int64   `json:"samples"`
}

// Output-estimate sources.
const (
	GPTAutoOutputSourceModelEffort = "history_model_effort"
	GPTAutoOutputSourceModel       = "history_model"
	GPTAutoOutputSourceNone        = "no_history"
)

// GPTAutoPricingLookup returns the gateway pricing-table entry for a model in
// USD per single token. Satisfied by (*BillingService).GetModelPricing.
type GPTAutoPricingLookup func(model string) (*ModelPricing, error)

// GPTAutoOutputEstimator returns historical output-token usage for a model at
// a reasoning effort.
type GPTAutoOutputEstimator func(model, effort string) GPTAutoOutputEstimate

// GPTAutoPolicyInput bundles everything the policy needs.
type GPTAutoPolicyInput struct {
	Classification GPTAutoClassification
	Features       GPTAutoRequestFeatures
	Config         GPTAutoShadowConfig
	Pricing        GPTAutoPricingLookup
	OutputEstimate GPTAutoOutputEstimator
	// ClassifierCostUSD is the cost of the Jev call that produced
	// Classification, or of a typical call when simulating.
	ClassifierCostUSD float64
}

// Decision statuses.
const (
	GPTAutoDecisionAccepted   = "accepted"
	GPTAutoDecisionAdjusted   = "adjusted"
	GPTAutoDecisionNoEligible = "no_eligible_candidate"
)

// Adjustment rule identifiers. Stable strings: the dashboard translates them.
const (
	GPTAutoRuleModelDisabled       = "model_disabled"
	GPTAutoRuleModelEndpoint       = "model_endpoint_unsupported"
	GPTAutoRuleModelTools          = "model_tools_unsupported"
	GPTAutoRuleModelUnknown        = "model_unknown"
	GPTAutoRuleRoutingWeightTie    = "routing_weight_tiebreak"
	GPTAutoRuleEffortUnsupported   = "effort_unsupported"
	GPTAutoRuleEffortUnknown       = "effort_unknown"
	GPTAutoRuleMarginBelowTarget   = "margin_below_target"
	GPTAutoRuleNoOutputHistory     = "no_output_history"
	GPTAutoRulePricingUnavailable  = "pricing_unavailable"
	GPTAutoRuleNoEligibleCandidate = "no_eligible_candidate"
)

// GPTAutoAdjustment explains one policy step.
type GPTAutoAdjustment struct {
	Rule   string `json:"rule"`
	From   string `json:"from,omitempty"`
	To     string `json:"to,omitempty"`
	Detail string `json:"detail,omitempty"`
}

// GPTAutoCandidate records why each pool model was or was not eligible.
type GPTAutoCandidate struct {
	Model         string   `json:"model"`
	Eligible      bool     `json:"eligible"`
	Reasons       []string `json:"reasons,omitempty"`
	Probability   float64  `json:"probability"`
	RoutingWeight int      `json:"routing_weight"`
	// probabilityKnown is false when Jev reported no probability for the
	// model; a zero Probability then means "unknown", not "unlikely".
	probabilityKnown bool
}

// GPTAutoEconomics is a hypothetical per-request estimate. Nothing here is an
// observed charge or cost.
type GPTAutoEconomics struct {
	Hypothetical bool `json:"hypothetical"`

	Model             string  `json:"model"`
	Effort            string  `json:"effort"`
	BillingMultiplier float64 `json:"billing_multiplier"`

	EstimatedInputTokens  float64 `json:"estimated_input_tokens"`
	EstimatedOutputTokens float64 `json:"estimated_output_tokens"`
	OutputEstimateSource  string  `json:"output_estimate_source"`
	OutputEstimateSamples int64   `json:"output_estimate_samples"`

	// List prices from the gateway pricing table, USD per 1M tokens.
	ListInputPerM  float64 `json:"list_input_per_m"`
	ListOutputPerM float64 `json:"list_output_per_m"`

	// UpstreamChannel is the preferred hypothetical procurement channel, or
	// empty when the estimate assumes procurement at list price.
	UpstreamChannel    string  `json:"upstream_channel,omitempty"`
	UpstreamInputPerM  float64 `json:"upstream_input_per_m"`
	UpstreamOutputPerM float64 `json:"upstream_output_per_m"`
	UpstreamCostSource string  `json:"upstream_cost_source"`

	EstimatedRevenue        float64 `json:"estimated_revenue"`
	EstimatedUpstreamCost   float64 `json:"estimated_upstream_cost"`
	EstimatedClassifierCost float64 `json:"estimated_classifier_cost"`
	EstimatedGrossProfit    float64 `json:"estimated_gross_profit"`
	// EstimatedGrossMargin is nil when revenue is zero (margin undefined).
	EstimatedGrossMargin *float64 `json:"estimated_gross_margin,omitempty"`
	TargetMargin         float64  `json:"target_margin"`
	// MinRequiredMultiplier is the billing multiplier at which the estimate
	// would reach TargetMargin. Nil when the list price is zero.
	MinRequiredMultiplier *float64 `json:"min_required_multiplier,omitempty"`
	BelowTarget           bool     `json:"below_target"`
	PricingAvailable      bool     `json:"pricing_available"`
}

// Upstream cost sources.
const (
	GPTAutoCostSourceChannel   = "configured_channel"
	GPTAutoCostSourceListPrice = "pricing_table_list_price"
	GPTAutoCostSourceNone      = "unavailable"
)

// GPTAutoPolicyDecision is the policy output. Jev's raw choice and the final
// recommendation are kept side by side so the adjustment is never silent.
type GPTAutoPolicyDecision struct {
	PolicyVersion string `json:"policy_version"`
	Status        string `json:"status"`

	JevModel    string `json:"jev_model"`
	JevEffort   string `json:"jev_effort"`
	FinalModel  string `json:"final_model,omitempty"`
	FinalEffort string `json:"final_effort,omitempty"`

	Candidates         []GPTAutoCandidate  `json:"candidates"`
	Adjustments        []GPTAutoAdjustment `json:"adjustments,omitempty"`
	CapabilityWarnings []string            `json:"capability_warnings,omitempty"`
	Economics          *GPTAutoEconomics   `json:"economics,omitempty"`
}

// gptAutoWeightTieTolerance is how close (in Jev probability) another eligible
// model must be to Jev's own choice before routing weight may prefer it. It
// keeps commercial preference from overriding a clear quality signal.
const gptAutoWeightTieTolerance = 0.05

// EvaluateGPTAutoPolicy turns a Jev classification into a shadow
// recommendation. It is pure: identical inputs give identical outputs, which
// is what makes policy-only replay possible.
func EvaluateGPTAutoPolicy(in GPTAutoPolicyInput) GPTAutoPolicyDecision {
	cfg := in.Config
	cls := in.Classification
	decision := GPTAutoPolicyDecision{
		PolicyVersion: cfg.PolicyVersion,
		JevModel:      cls.Model,
		JevEffort:     cls.Effort,
	}

	decision.Candidates = gptAutoCandidates(cfg, cls, in.Features)
	finalModel, modelAdjustments := gptAutoPickModel(decision.Candidates, cls.Model)
	decision.Adjustments = append(decision.Adjustments, modelAdjustments...)
	for _, candidate := range decision.Candidates {
		if candidate.Model == cls.Model && !candidate.Eligible {
			decision.CapabilityWarnings = append(decision.CapabilityWarnings, candidate.Reasons...)
		}
	}
	if finalModel == "" {
		decision.Status = GPTAutoDecisionNoEligible
		decision.Adjustments = append(decision.Adjustments, GPTAutoAdjustment{
			Rule: GPTAutoRuleNoEligibleCandidate, From: cls.Model,
			Detail: "no enabled model satisfies the request's endpoint and tool requirements",
		})
		return decision
	}
	decision.FinalModel = finalModel

	profile, _ := cfg.ProfileFor(finalModel)
	finalEffort, effortAdjustment := gptAutoPickEffort(profile.SupportedEfforts, cls.Effort)
	if effortAdjustment != nil {
		decision.Adjustments = append(decision.Adjustments, *effortAdjustment)
		if effortAdjustment.Rule == GPTAutoRuleEffortUnsupported {
			decision.CapabilityWarnings = append(decision.CapabilityWarnings, effortAdjustment.Detail)
		}
	}
	decision.FinalEffort = finalEffort

	economics := EstimateGPTAutoEconomics(in, finalModel, finalEffort)
	decision.Economics = &economics
	if !economics.PricingAvailable {
		decision.Adjustments = append(decision.Adjustments, GPTAutoAdjustment{
			Rule: GPTAutoRulePricingUnavailable, To: finalModel,
			Detail: "no pricing-table entry; economics are not estimated",
		})
	}
	if economics.OutputEstimateSource == GPTAutoOutputSourceNone {
		decision.Adjustments = append(decision.Adjustments, GPTAutoAdjustment{
			Rule:   GPTAutoRuleNoOutputHistory,
			Detail: "no historical output usage for this model; output cost is excluded from the estimate",
		})
	}
	// A low margin is recorded but never changes the recommendation: commercial
	// optimization must not override capability or quality.
	if economics.BelowTarget {
		decision.Adjustments = append(decision.Adjustments, GPTAutoAdjustment{
			Rule: GPTAutoRuleMarginBelowTarget, To: finalModel,
			Detail: "estimated gross margin is below the configured target; recommendation unchanged",
		})
	}

	decision.Status = GPTAutoDecisionAccepted
	if finalModel != cls.Model || finalEffort != cls.Effort {
		decision.Status = GPTAutoDecisionAdjusted
	}
	return decision
}

func gptAutoCandidates(cfg GPTAutoShadowConfig, cls GPTAutoClassification, features GPTAutoRequestFeatures) []GPTAutoCandidate {
	candidates := make([]GPTAutoCandidate, 0, len(GPTAutoModelPool()))
	for _, model := range GPTAutoModelPool() {
		profile, ok := cfg.ProfileFor(model)
		probability, known := cls.ModelProbabilities[model]
		candidate := GPTAutoCandidate{
			Model:            model,
			Probability:      probability,
			RoutingWeight:    profile.RoutingWeight,
			probabilityKnown: known,
		}
		if !ok || !profile.Enabled {
			candidate.Reasons = append(candidate.Reasons, GPTAutoRuleModelDisabled)
		}
		if len(profile.Endpoints) > 0 && !containsString(profile.Endpoints, features.Endpoint) {
			candidate.Reasons = append(candidate.Reasons, GPTAutoRuleModelEndpoint)
		}
		if features.HasTools && !profile.SupportsTools {
			candidate.Reasons = append(candidate.Reasons, GPTAutoRuleModelTools)
		}
		candidate.Eligible = len(candidate.Reasons) == 0
		candidates = append(candidates, candidate)
	}
	return candidates
}

// gptAutoPickModel keeps Jev's choice when it is eligible, unless another
// eligible model is within gptAutoWeightTieTolerance of it and carries a
// higher routing weight. When Jev's choice is ineligible, the eligible model
// with the highest Jev probability wins, again with weight breaking near-ties.
func gptAutoPickModel(candidates []GPTAutoCandidate, jevModel string) (string, []GPTAutoAdjustment) {
	eligible := make([]GPTAutoCandidate, 0, len(candidates))
	var jevCandidate *GPTAutoCandidate
	for i := range candidates {
		if candidates[i].Model == jevModel {
			jevCandidate = &candidates[i]
		}
		if candidates[i].Eligible {
			eligible = append(eligible, candidates[i])
		}
	}
	if len(eligible) == 0 {
		return "", nil
	}

	var adjustments []GPTAutoAdjustment
	anchor := -1.0
	if jevCandidate == nil {
		adjustments = append(adjustments, GPTAutoAdjustment{
			Rule: GPTAutoRuleModelUnknown, From: jevModel,
			Detail: "classifier returned a model outside the Phase 1 pool",
		})
	} else if jevCandidate.Eligible {
		anchor = jevCandidate.Probability
	} else {
		adjustments = append(adjustments, GPTAutoAdjustment{
			Rule: strings.Join(jevCandidate.Reasons, ","), From: jevModel,
			Detail: "classifier choice fails a hard capability constraint",
		})
	}

	sort.SliceStable(eligible, func(i, j int) bool {
		if eligible[i].Probability != eligible[j].Probability {
			return eligible[i].Probability > eligible[j].Probability
		}
		return eligible[i].RoutingWeight > eligible[j].RoutingWeight
	})

	// Routing weight may only break a near-tie that Jev's probabilities
	// actually show; missing probabilities are never treated as a tie.
	nearTie := func(anchor float64, anchorKnown bool, candidate GPTAutoCandidate) bool {
		return anchorKnown && candidate.probabilityKnown &&
			anchor-candidate.Probability <= gptAutoWeightTieTolerance+1e-12
	}

	if anchor >= 0 {
		best := *jevCandidate
		for _, candidate := range eligible {
			if candidate.Model == best.Model {
				continue
			}
			if nearTie(anchor, jevCandidate.probabilityKnown, candidate) && candidate.RoutingWeight > best.RoutingWeight {
				best = candidate
			}
		}
		if best.Model != jevModel {
			adjustments = append(adjustments, GPTAutoAdjustment{
				Rule: GPTAutoRuleRoutingWeightTie, From: jevModel, To: best.Model,
				Detail: "near-equal classifier probability; higher routing weight preferred",
			})
		}
		return best.Model, adjustments
	}

	top := eligible[0]
	best := top
	for _, candidate := range eligible[1:] {
		if nearTie(top.Probability, top.probabilityKnown, candidate) && candidate.RoutingWeight > best.RoutingWeight {
			best = candidate
		}
	}
	if len(adjustments) > 0 {
		adjustments[len(adjustments)-1].To = best.Model
	}
	return best.Model, adjustments
}

// gptAutoPickEffort keeps Jev's effort when the model supports it. Otherwise
// it moves up to the nearest supported level (to avoid under-serving the
// task), and only moves down when nothing higher exists.
func gptAutoPickEffort(supported []string, jevEffort string) (string, *GPTAutoAdjustment) {
	if len(supported) == 0 {
		return "", nil
	}
	rank := gptAutoEffortRank(jevEffort)
	if rank < 0 {
		// Unknown effort: fall back to the model's lowest supported level that
		// is at least medium, else its highest.
		target := supported[len(supported)-1]
		for _, effort := range supported {
			if gptAutoEffortRank(effort) >= gptAutoEffortRank(GPTAutoEffortMedium) {
				target = effort
				break
			}
		}
		return target, &GPTAutoAdjustment{Rule: GPTAutoRuleEffortUnknown, From: jevEffort, To: target,
			Detail: "classifier returned an unknown reasoning effort"}
	}
	if containsString(supported, jevEffort) {
		return jevEffort, nil
	}
	ladder := GPTAutoEffortLadder()
	for i := rank + 1; i < len(ladder); i++ {
		if containsString(supported, ladder[i]) {
			return ladder[i], &GPTAutoAdjustment{Rule: GPTAutoRuleEffortUnsupported, From: jevEffort, To: ladder[i],
				Detail: "model does not support reasoning effort " + jevEffort + "; raised to " + ladder[i]}
		}
	}
	for i := rank - 1; i >= 0; i-- {
		if containsString(supported, ladder[i]) {
			return ladder[i], &GPTAutoAdjustment{Rule: GPTAutoRuleEffortUnsupported, From: jevEffort, To: ladder[i],
				Detail: "model does not support reasoning effort " + jevEffort + "; lowered to " + ladder[i]}
		}
	}
	return "", nil
}

// EstimateGPTAutoEconomics computes hypothetical per-request economics for a
// model/effort. Revenue is list price × billing multiplier; upstream cost is
// the cheapest enabled configured channel, else list price.
//
//	gross_margin = (revenue - upstream_cost - classifier_cost) / revenue
func EstimateGPTAutoEconomics(in GPTAutoPolicyInput, model, effort string) GPTAutoEconomics {
	cfg := in.Config
	profile, _ := cfg.ProfileFor(model)
	out := GPTAutoEconomics{
		Hypothetical:            true,
		Model:                   model,
		Effort:                  effort,
		BillingMultiplier:       profile.BillingMultiplier,
		EstimatedInputTokens:    float64(in.Features.EstimatedInputTokens),
		EstimatedClassifierCost: in.ClassifierCostUSD,
		TargetMargin:            cfg.MinMarginFor(model),
		OutputEstimateSource:    GPTAutoOutputSourceNone,
		UpstreamCostSource:      GPTAutoCostSourceNone,
	}
	if in.OutputEstimate != nil {
		estimate := in.OutputEstimate(model, effort)
		if estimate.Tokens > 0 && estimate.Source != "" {
			out.EstimatedOutputTokens = estimate.Tokens
			out.OutputEstimateSource = estimate.Source
			out.OutputEstimateSamples = estimate.Samples
		}
	}

	var pricing *ModelPricing
	if in.Pricing != nil {
		pricing, _ = in.Pricing(model)
	}
	if pricing == nil || (pricing.InputPricePerToken <= 0 && pricing.OutputPricePerToken <= 0) {
		return out
	}
	out.PricingAvailable = true
	out.ListInputPerM = pricing.InputPricePerToken * gptAutoTokensPerMillion
	out.ListOutputPerM = pricing.OutputPricePerToken * gptAutoTokensPerMillion

	upstreamIn, upstreamOut := out.ListInputPerM, out.ListOutputPerM
	out.UpstreamCostSource = GPTAutoCostSourceListPrice
	if channel, ok := gptAutoCheapestChannel(profile.UpstreamCosts, out.EstimatedInputTokens, out.EstimatedOutputTokens); ok {
		upstreamIn, upstreamOut = channel.InputPerM, channel.OutputPerM
		out.UpstreamChannel = channel.Channel
		out.UpstreamCostSource = GPTAutoCostSourceChannel
	}
	out.UpstreamInputPerM = upstreamIn
	out.UpstreamOutputPerM = upstreamOut

	listCost := (out.EstimatedInputTokens*out.ListInputPerM + out.EstimatedOutputTokens*out.ListOutputPerM) / gptAutoTokensPerMillion
	out.EstimatedRevenue = listCost * profile.BillingMultiplier
	out.EstimatedUpstreamCost = (out.EstimatedInputTokens*upstreamIn + out.EstimatedOutputTokens*upstreamOut) / gptAutoTokensPerMillion
	out.EstimatedGrossProfit = out.EstimatedRevenue - out.EstimatedUpstreamCost - out.EstimatedClassifierCost
	if out.EstimatedRevenue > 0 {
		margin := out.EstimatedGrossProfit / out.EstimatedRevenue
		out.EstimatedGrossMargin = &margin
		out.BelowTarget = margin < out.TargetMargin
	}
	if listCost > 0 && out.TargetMargin < 1 {
		required := (out.EstimatedUpstreamCost + out.EstimatedClassifierCost) / ((1 - out.TargetMargin) * listCost)
		required = math.Round(required*10000) / 10000
		out.MinRequiredMultiplier = &required
	}
	return out
}

func gptAutoCheapestChannel(channels []GPTAutoUpstreamCost, inputTokens, outputTokens float64) (GPTAutoUpstreamCost, bool) {
	var best GPTAutoUpstreamCost
	bestCost := math.Inf(1)
	for _, channel := range channels {
		if !channel.Enabled {
			continue
		}
		cost := inputTokens*channel.InputPerM + outputTokens*channel.OutputPerM
		// Without a token estimate every channel costs zero; fall back to the
		// cheaper unit price so the preferred channel is still meaningful.
		if inputTokens == 0 && outputTokens == 0 {
			cost = channel.InputPerM + channel.OutputPerM
		}
		if cost < bestCost {
			best, bestCost = channel, cost
		}
	}
	return best, !math.IsInf(bestCost, 1)
}
