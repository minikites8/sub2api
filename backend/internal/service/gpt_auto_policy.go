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
	// customer's prompt size (characters ÷ 4) unless InputEstimateSource says
	// otherwise.
	EstimatedInputTokens int `json:"estimated_input_tokens"`
	// InputEstimateSource is empty on rows written before estimate sources
	// existed; those were always the character heuristic.
	InputEstimateSource string `json:"input_estimate_source,omitempty"`
	// CachedInputTokens is only set from actual usage (never persisted).
	CachedInputTokens int `json:"-"`
	// Context is the extraction metadata of the classified request: counts,
	// budgets and limitations only, never content. Nil on older rows.
	Context *GPTAutoContextInfo `json:"context,omitempty"`
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
	// GPTAutoOutputSourceNone: no history, or fewer than
	// gptAutoMinOutputHistorySamples samples. Rows written before the minimum
	// existed may carry the legacy value "no_history".
	GPTAutoOutputSourceNone = "insufficient_data"
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
	// ClassifierCostSource labels ClassifierCostUSD (default: actual call).
	ClassifierCostSource string
	// Assumptions override token estimates with administrator-entered values.
	Assumptions *GPTAutoWorkloadAssumptions
	// CustomerRateMultiplier is the customer's effective rate; 0 means the
	// list rate (1×) is assumed.
	CustomerRateMultiplier float64
	CustomerRateSource     string
	// Tariff is the gpt-auto retail tariff when independent pricing is
	// active (the live route passes its group-resolved snapshot). Nil uses
	// the global tariff of Config.Pricing in fixed-rate mode, else the
	// legacy executed-model price times the profile multiplier.
	Tariff *GPTAutoTariff
	// Exclusions are additional hard constraints per model, supplied by live
	// routing (request capabilities, customer permissions, upstream
	// availability, margin guard). Each listed reason makes the model
	// ineligible. Shadow evaluation and replay leave it nil.
	Exclusions map[string][]string
}

// Decision statuses.
const (
	GPTAutoDecisionAccepted   = "accepted"
	GPTAutoDecisionAdjusted   = "adjusted"
	GPTAutoDecisionNoEligible = "no_eligible_candidate"
)

// Adjustment rule identifiers. Stable strings: the dashboard translates them.
// Only these rules can change or withhold a recommendation.
const (
	GPTAutoRuleModelDisabled       = "model_disabled"
	GPTAutoRuleModelEndpoint       = "model_endpoint_unsupported"
	GPTAutoRuleModelTools          = "model_tools_unsupported"
	GPTAutoRuleModelUnknown        = "model_unknown"
	GPTAutoRuleRoutingWeightTie    = "routing_weight_tiebreak"
	GPTAutoRuleEffortUnsupported   = "effort_unsupported"
	GPTAutoRuleEffortUnknown       = "effort_unknown"
	GPTAutoRuleNoEligibleCandidate = "no_eligible_candidate"

	// Live-routing constraints (Exclusions).
	GPTAutoRuleModelVision       = "model_vision_unsupported"
	GPTAutoRuleModelStructured   = "model_structured_output_unsupported"
	GPTAutoRuleModelStateful     = "model_stateful_continuation_unsupported"
	GPTAutoRuleModelContext      = "model_context_exceeded"
	GPTAutoRuleModelNotPermitted = "model_not_permitted"
	GPTAutoRuleModelUnavailable  = "model_no_upstream_available"
	GPTAutoRuleMarginGuard       = "margin_guard_loss"
	GPTAutoRuleClientEffort      = "client_effort_honored"
)

// Warning identifiers. Warnings are informational: they are stored in
// GPTAutoPolicyDecision.Warnings and never change the recommendation.
// Decisions written before the split carry margin_below_target,
// no_output_history and pricing_unavailable in Adjustments instead.
const (
	GPTAutoRuleMarginBelowTarget          = "margin_below_target"
	GPTAutoRuleNoOutputHistory            = "no_output_history"
	GPTAutoRulePricingUnavailable         = "pricing_unavailable"
	GPTAutoRuleModelMarginBelowTarget     = "model_margin_below_target"
	GPTAutoRuleModelMarginLoss            = "model_margin_loss"
	GPTAutoRuleFullyLoadedBelowTarget     = "fully_loaded_margin_below_target"
	GPTAutoRuleFullyLoadedLoss            = "fully_loaded_margin_loss"
	GPTAutoRuleInsufficientEstimate       = "insufficient_estimate_data"
	GPTAutoRuleHeuristicEstimate          = "heuristic_estimate"
	GPTAutoRuleLowModelConfidence         = "low_model_confidence"
	GPTAutoRuleLowEffortConfidence        = "low_effort_confidence"
	GPTAutoRuleModelProbabilitiesMissing  = "model_probabilities_missing"
	GPTAutoRuleChoiceNotMostProbable      = "choice_not_most_probable"
	GPTAutoRuleEffortProbabilitiesMissing = "effort_probabilities_missing"
)

// gptAutoLowConfidenceThreshold flags (never overrides) a Jev answer whose
// self-reported confidence is low. Confidence is not a calibrated success
// probability, so it is a review signal only.
const gptAutoLowConfidenceThreshold = 0.5

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

	Candidates []GPTAutoCandidate `json:"candidates"`
	// Adjustments are the rules that changed or withheld the recommendation.
	Adjustments []GPTAutoAdjustment `json:"adjustments,omitempty"`
	// Warnings never change the recommendation (margins, estimate quality,
	// classifier uncertainty).
	Warnings           []GPTAutoAdjustment `json:"warnings,omitempty"`
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

	decision.Candidates = gptAutoCandidates(cfg, cls, in.Features, in.Exclusions)
	decision.Warnings = gptAutoClassificationWarnings(cls)
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
			Detail: "no enabled model satisfies the request's capability and permission requirements",
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
	decision.Warnings = append(decision.Warnings, gptAutoEconomicsWarnings(economics)...)
	decision.Status = GPTAutoDecisionAccepted
	if finalModel != cls.Model || finalEffort != cls.Effort {
		decision.Status = GPTAutoDecisionAdjusted
	}
	return decision
}

func gptAutoCandidates(cfg GPTAutoShadowConfig, cls GPTAutoClassification, features GPTAutoRequestFeatures, exclusions map[string][]string) []GPTAutoCandidate {
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
		for _, reason := range exclusions[model] {
			if !containsString(candidate.Reasons, reason) {
				candidate.Reasons = append(candidate.Reasons, reason)
			}
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
	// actually show; missing probabilities are never treated as a tie. The
	// window is symmetric: a model Jev rated clearly *more* likely than its
	// own choice is not a tie either (that inconsistency is reported as a
	// warning instead of being resolved by commercial weight).
	nearTie := func(anchor float64, anchorKnown bool, candidate GPTAutoCandidate) bool {
		return anchorKnown && candidate.probabilityKnown &&
			math.Abs(anchor-candidate.Probability) <= gptAutoWeightTieTolerance+1e-12
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

// gptAutoClassificationWarnings flags uncertain or internally inconsistent Jev
// answers. They are recorded for review only: an uncertain answer is kept as
// Jev's recommendation rather than replaced by a guess.
func gptAutoClassificationWarnings(cls GPTAutoClassification) []GPTAutoAdjustment {
	var warnings []GPTAutoAdjustment
	if len(cls.ModelProbabilities) == 0 {
		warnings = append(warnings, GPTAutoAdjustment{Rule: GPTAutoRuleModelProbabilitiesMissing,
			Detail: "classifier reported no model probabilities; routing weight cannot break ties"})
	} else {
		// A missing probability for the chosen model counts as zero here.
		chosen := cls.ModelProbabilities[cls.Model]
		best, bestProbability := "", chosen
		for _, model := range GPTAutoModelPool() {
			if probability, ok := cls.ModelProbabilities[model]; ok && model != cls.Model && probability > bestProbability+1e-9 {
				best, bestProbability = model, probability
			}
		}
		if best != "" {
			warnings = append(warnings, GPTAutoAdjustment{Rule: GPTAutoRuleChoiceNotMostProbable, From: cls.Model, To: best,
				Detail: "the selected model is not the most probable one in the reported distribution; Jev's choice is kept"})
		}
	}
	if len(cls.EffortProbabilities) == 0 {
		warnings = append(warnings, GPTAutoAdjustment{Rule: GPTAutoRuleEffortProbabilitiesMissing,
			Detail: "classifier reported no effort probabilities"})
	}
	if cls.ModelConfidence < gptAutoLowConfidenceThreshold {
		warnings = append(warnings, GPTAutoAdjustment{Rule: GPTAutoRuleLowModelConfidence, From: cls.Model,
			Detail: "model confidence below 50%; recommendation kept, review before relying on it"})
	}
	if cls.EffortConfidence < gptAutoLowConfidenceThreshold {
		warnings = append(warnings, GPTAutoAdjustment{Rule: GPTAutoRuleLowEffortConfidence, From: cls.Effort,
			Detail: "effort confidence below 50%; recommendation kept, review before relying on it"})
	}
	return warnings
}

// gptAutoEconomicsWarnings reports estimate quality and margin status. A low
// margin is recorded but never changes the recommendation: commercial
// optimization must not override capability or quality.
func gptAutoEconomicsWarnings(economics GPTAutoEconomics) []GPTAutoAdjustment {
	var warnings []GPTAutoAdjustment
	if !economics.PricingAvailable {
		return append(warnings, GPTAutoAdjustment{Rule: GPTAutoRulePricingUnavailable,
			Detail: "no pricing-table entry; economics are not estimated"})
	}
	switch economics.EstimateQuality {
	case GPTAutoEstimateQualityInsufficient:
		return append(warnings, GPTAutoAdjustment{Rule: GPTAutoRuleInsufficientEstimate,
			Detail: "not enough token data for a margin estimate (input " + economics.InputEstimateSource +
				", output " + economics.OutputEstimateSource + "); margins are not shown"})
	case GPTAutoEstimateQualityHeuristic:
		warnings = append(warnings, GPTAutoAdjustment{Rule: GPTAutoRuleHeuristicEstimate,
			Detail: "input tokens are a character-count heuristic; treat the margin as indicative"})
	}
	switch economics.ModelMarginStatus {
	case GPTAutoMarginLoss:
		warnings = append(warnings, GPTAutoAdjustment{Rule: GPTAutoRuleModelMarginLoss,
			Detail: "estimated model revenue is below upstream model cost"})
	case GPTAutoMarginBelowTarget:
		warnings = append(warnings, GPTAutoAdjustment{Rule: GPTAutoRuleModelMarginBelowTarget,
			Detail: "estimated model margin is positive but below the effective target"})
	}
	switch economics.FullyLoadedMarginStatus {
	case GPTAutoMarginLoss:
		warnings = append(warnings, GPTAutoAdjustment{Rule: GPTAutoRuleFullyLoadedLoss,
			Detail: "including the Jev classification cost, the estimate is a loss"})
	case GPTAutoMarginBelowTarget:
		warnings = append(warnings, GPTAutoAdjustment{Rule: GPTAutoRuleFullyLoadedBelowTarget,
			Detail: "including the Jev classification cost, the estimated margin is below the effective target"})
	}
	return warnings
}
