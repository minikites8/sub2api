package service

import (
	"context"
	"strings"
	"time"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/shopspring/decimal"
)

// GPT-Auto Phase 2.1 pricing administration: read, simulate and save the
// gpt-auto retail tariff. Saving is a separate, confirmed and versioned
// operation; nothing here rewrites historical usage or decision rows.

// gptAutoDefaultSimulationClassifierTokens is a typical Jev request size used
// when the administrator does not enter one.
const gptAutoDefaultSimulationClassifierTokens = 1500

// GPTAutoPricingState is the admin view of the tariff and its readiness.
type GPTAutoPricingState struct {
	Pricing           GPTAutoPricingConfig `json:"pricing"`
	Ready             bool                 `json:"ready"`
	TariffConfigured  bool                 `json:"tariff_configured"`
	LegacyMultipliers bool                 `json:"legacy_multipliers_applied"`
	LiveEnabled       bool                 `json:"live_enabled"`
	InternalOnly      bool                 `json:"internal_only"`
	AllowedGroupIDs   []int64              `json:"allowed_group_ids"`
}

// PricingState returns the stored tariff (fresh read, not the cache).
func (s *GPTAutoShadowService) PricingState(ctx context.Context) (*GPTAutoPricingState, error) {
	cfg, err := s.settings.GetGPTAutoShadowConfig(ctx)
	if err != nil {
		return nil, err
	}
	return gptAutoPricingStateFor(cfg), nil
}

func gptAutoPricingStateFor(cfg *GPTAutoShadowConfig) *GPTAutoPricingState {
	return &GPTAutoPricingState{
		Pricing:           cfg.Pricing,
		Ready:             cfg.Pricing.Ready(),
		TariffConfigured:  cfg.Pricing.TariffConfigured(),
		LegacyMultipliers: !cfg.Pricing.IsFixed() && cfg.Live.ApplyBillingMultiplier,
		LiveEnabled:       cfg.Live.Enabled,
		InternalOnly:      cfg.Live.InternalOnly,
		AllowedGroupIDs:   append([]int64{}, cfg.Live.AllowedGroupIDs...),
	}
}

// GPTAutoPricingUpdate is a confirmed tariff change.
type GPTAutoPricingUpdate struct {
	Pricing GPTAutoPricingConfig
	// ExpectedVersion guards against overwriting a concurrent change.
	ExpectedVersion int
	Confirmed       bool
	ActorID         int64
}

// UpdatePricing validates, versions and stores a new tariff. The previous
// tariff keeps applying to requests already routed (their snapshot).
func (s *GPTAutoShadowService) UpdatePricing(ctx context.Context, update GPTAutoPricingUpdate) (*GPTAutoPricingState, error) {
	if !update.Confirmed {
		return nil, infraerrors.BadRequest("GPT_AUTO_PRICING_CONFIRMATION_REQUIRED", "confirm the pricing change before saving")
	}
	next := update.Pricing
	next.Mode = strings.TrimSpace(next.Mode)
	if err := ValidateGPTAutoPricingActivation(next); err != nil {
		return nil, err
	}
	cfg, err := s.settings.GetGPTAutoShadowConfig(ctx)
	if err != nil {
		return nil, err
	}
	if cfg.Pricing.Version != update.ExpectedVersion {
		return nil, infraerrors.Conflict("GPT_AUTO_PRICING_VERSION_CONFLICT", "the pricing was changed by someone else; reload and review it again")
	}
	now := s.now().UTC()
	next.Version = cfg.Pricing.Version + 1
	next.UpdatedAt = &now
	next.UpdatedBy = update.ActorID
	cfg.Pricing = next
	if err := s.settings.SetGPTAutoShadowConfig(ctx, cfg); err != nil {
		return nil, err
	}
	return gptAutoPricingStateFor(cfg), nil
}

// GPTAutoGroupPricingView is one group's effective gpt-auto price and access
// state, for the pricing page and the group form.
type GPTAutoGroupPricingView struct {
	GroupID        int64  `json:"group_id"`
	Name           string `json:"name"`
	Status         string `json:"status"`
	GPTAutoEnabled bool   `json:"gpt_auto_enabled"`
	// PricingSource: global, group_adjusted (group rate ≠ 1) or
	// group_override (explicit gpt-auto entry in the group model pricing).
	PricingSource  string  `json:"pricing_source"`
	RateMultiplier float64 `json:"rate_multiplier"`
	// RateSource: group_model_rate (a gpt-auto rate on the group) or
	// group_default. User-specific group rates still apply per user.
	RateSource      string `json:"rate_source"`
	PeakRateEnabled bool   `json:"peak_rate_enabled"`
	// Effective rates after the group rate (USD / 1M tokens). Nil when no
	// tariff applies (legacy mode or unconfigured).
	EffectiveInput      *decimal.Decimal `json:"effective_input_price_per_mtok,omitempty"`
	EffectiveCached     *decimal.Decimal `json:"effective_cached_input_price_per_mtok,omitempty"`
	EffectiveOutput     *decimal.Decimal `json:"effective_output_price_per_mtok,omitempty"`
	EffectiveCacheWrite *decimal.Decimal `json:"effective_cache_write_price_per_mtok,omitempty"`
	// Access: why a switched-on group may still not reach gpt-auto.
	Operational           bool `json:"operational"`
	RequiresKeyAllowlist  bool `json:"requires_key_allowlist"`
	ExcludedByRollout     bool `json:"excluded_by_rollout"`
	ModelAllowlistBlocked bool `json:"model_allowlist_blocks_gpt_auto"`
}

// GroupPricingViews describes every OpenAI group under the current tariff.
func (s *GPTAutoShadowService) GroupPricingViews(ctx context.Context, groups []Group) ([]GPTAutoGroupPricingView, error) {
	cfg, err := s.settings.GetGPTAutoShadowConfig(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]GPTAutoGroupPricingView, 0, len(groups))
	for i := range groups {
		group := &groups[i]
		if group.Platform != PlatformOpenAI {
			continue
		}
		out = append(out, GPTAutoGroupPricingViewFor(cfg, group))
	}
	return out, nil
}

// GPTAutoGroupPricingViewFor builds one group's view from a config.
func GPTAutoGroupPricingViewFor(cfg *GPTAutoShadowConfig, group *Group) GPTAutoGroupPricingView {
	rate, rateSource := gptAutoGroupRate(group)
	view := GPTAutoGroupPricingView{
		GroupID: group.ID, Name: group.Name, Status: group.Status, GPTAutoEnabled: group.GPTAutoEnabled,
		RateMultiplier: rate, RateSource: rateSource, PeakRateEnabled: group.PeakRateEnabled,
	}
	groupID := group.ID
	view.RequiresKeyAllowlist = cfg.Live.InternalOnly
	view.ExcludedByRollout = !cfg.Live.InternalOnly && len(cfg.Live.AllowedGroupIDs) > 0 && !containsInt64(cfg.Live.AllowedGroupIDs, groupID)
	view.ModelAllowlistBlocked = group.ModelAllowlistEnabled() && !group.ModelAllowlist.Allows(GPTAutoVirtualModel)
	view.Operational = cfg.Live.Enabled && group.GPTAutoEnabled && cfg.Pricing.Ready() && !view.ExcludedByRollout && !view.ModelAllowlistBlocked
	tariff, ok := cfg.Pricing.TariffFor(group)
	if !ok || tariff.Mode != GPTAutoPricingModeFixed {
		view.PricingSource = tariff.Source
		return view
	}
	view.PricingSource = tariff.Source
	if tariff.Source == GPTAutoPricingSourceGlobal && rate != 1 {
		view.PricingSource = GPTAutoPricingSourceGroupAdjusted
	}
	scale := func(value decimal.Decimal) *decimal.Decimal {
		scaled := value.Mul(decimal.NewFromFloat(rate)).Round(gptAutoPriceDecimals + 4)
		return &scaled
	}
	view.EffectiveInput, view.EffectiveCached = scale(tariff.Input), scale(tariff.CachedInput)
	view.EffectiveOutput, view.EffectiveCacheWrite = scale(tariff.Output), scale(tariff.CacheWrite)
	return view
}

// gptAutoGroupRate is the group's customer rate for the gpt-auto identity,
// matching gptAutoCustomerRateMultiplier without the per-user override.
func gptAutoGroupRate(group *Group) (float64, string) {
	if group == nil {
		return 1, "group_default"
	}
	for model := range group.ModelRateMultipliers {
		if strings.EqualFold(strings.TrimSpace(model), GPTAutoVirtualModel) {
			return group.RateMultiplierForModels(GPTAutoVirtualModel), "group_model_rate"
		}
	}
	return group.RateMultiplierForModels(GPTAutoVirtualModel), "group_default"
}

// GPTAutoPricingSimulationInput is the pricing calculator request. Total
// input includes cached input, as the upstream reports it.
type GPTAutoPricingSimulationInput struct {
	// Pricing is a candidate tariff to preview; nil uses the stored one.
	Pricing               *GPTAutoPricingConfig
	Group                 *Group
	TotalInputTokens      int
	CachedInputTokens     int
	OutputTokens          int
	ExecutedModel         string
	ClassifierInputTokens int
	Now                   time.Time
}

// GPTAutoPricingSimulationRow compares one executed model at the same usage.
type GPTAutoPricingSimulationRow struct {
	Model              string           `json:"model"`
	CustomerCharge     decimal.Decimal  `json:"customer_charge_usd"`
	UpstreamCost       *decimal.Decimal `json:"estimated_upstream_cost_usd,omitempty"`
	UpstreamCostSource string           `json:"upstream_cost_source"`
	ClassifierCost     decimal.Decimal  `json:"estimated_classifier_cost_usd"`
	GrossProfit        *decimal.Decimal `json:"gross_profit_usd,omitempty"`
	FullyLoadedProfit  *decimal.Decimal `json:"fully_loaded_profit_usd,omitempty"`
	GrossMargin        *float64         `json:"gross_margin,omitempty"`
	FullyLoadedMargin  *float64         `json:"fully_loaded_margin,omitempty"`
	MarginStatus       string           `json:"margin_status"`
}

// GPTAutoPricingSimulation is a projection, never a realised margin.
type GPTAutoPricingSimulation struct {
	Mode            string                        `json:"mode"`
	Tariff          *GPTAutoTariff                `json:"tariff,omitempty"`
	PricingSource   string                        `json:"pricing_source"`
	RateMultiplier  float64                       `json:"rate_multiplier"`
	UncachedInput   int                           `json:"uncached_input_tokens"`
	Rows            []GPTAutoPricingSimulationRow `json:"rows"`
	Warnings        []string                      `json:"warnings"`
	Hypothetical    bool                          `json:"hypothetical"`
	ClassifierModel string                        `json:"classifier_model"`
}

const gptAutoMaxPricingSimulationTokens = 10_000_000

// SimulatePricing projects the customer charge and margins of one usage
// across the executed models. In fixed-rate mode every row has the same
// customer charge; procurement differs by model.
func (s *GPTAutoShadowService) SimulatePricing(ctx context.Context, in GPTAutoPricingSimulationInput) (*GPTAutoPricingSimulation, error) {
	bad := func(msg string) error { return infraerrors.BadRequest("GPT_AUTO_PRICING_SIMULATION_INVALID", msg) }
	for _, value := range []int{in.TotalInputTokens, in.CachedInputTokens, in.OutputTokens, in.ClassifierInputTokens} {
		if value < 0 || value > gptAutoMaxPricingSimulationTokens {
			return nil, bad("token counts must be between 0 and 10,000,000")
		}
	}
	if in.CachedInputTokens > in.TotalInputTokens {
		return nil, bad("cached input tokens cannot exceed total input tokens")
	}
	models := GPTAutoModelPool()
	if model := strings.TrimSpace(in.ExecutedModel); model != "" {
		if !containsString(models, model) {
			return nil, bad("executed model must be one of the GPT Auto pool models")
		}
		models = []string{model}
	}
	stored, err := s.settings.GetGPTAutoShadowConfig(ctx)
	if err != nil {
		return nil, err
	}
	cfg := *stored
	if in.Pricing != nil {
		candidate := *in.Pricing
		if err := ValidateGPTAutoPricingActivation(candidate); err != nil {
			return nil, err
		}
		cfg.Pricing = candidate
	}
	classifierTokens := in.ClassifierInputTokens
	if classifierTokens == 0 {
		classifierTokens = gptAutoDefaultSimulationClassifierTokens
	}
	classifierCost := gptAutoDecimal(gptAutoClassifierCostUSD(s.pricing, cfg.ClassifierModel, classifierTokens, gptAutoEstimatedOutputTokens))
	rate, _ := gptAutoGroupRate(in.Group)
	uncached := max(in.TotalInputTokens-in.CachedInputTokens, 0)
	out := &GPTAutoPricingSimulation{
		Mode: cfg.Pricing.Mode, RateMultiplier: rate, UncachedInput: uncached, Hypothetical: true,
		ClassifierModel: cfg.ClassifierModel, Warnings: []string{},
	}
	tariff, ok := cfg.Pricing.TariffFor(in.Group)
	if cfg.Pricing.IsFixed() && !ok {
		return nil, bad("configure a valid independent tariff before simulation")
	}
	if ok && tariff.Mode == GPTAutoPricingModeFixed {
		out.Tariff = &tariff
		out.PricingSource = tariff.Source
		if tariff.Source == GPTAutoPricingSourceGlobal && rate != 1 {
			out.PricingSource = GPTAutoPricingSourceGroupAdjusted
		}
	} else {
		out.PricingSource = GPTAutoPricingSourceLegacy
	}
	outputTokens, cachedTokens := in.OutputTokens, in.CachedInputTokens
	assumptions := &GPTAutoWorkloadAssumptions{InputTokens: &uncached, CachedInputTokens: &cachedTokens, OutputTokens: &outputTokens}
	losses := []string{}
	for _, model := range models {
		policy := GPTAutoPolicyInput{
			Config: cfg, Pricing: s.pricing, Assumptions: assumptions,
			CustomerRateMultiplier: rate, CustomerRateSource: "group",
			ClassifierCostUSD: gptAutoUSD(classifierCost), ClassifierCostSource: GPTAutoClassifierCostEstimated,
		}
		if out.Tariff != nil {
			policy.Tariff = out.Tariff
		}
		economics := EstimateGPTAutoEconomics(policy, model, GPTAutoEffortMedium)
		row := GPTAutoPricingSimulationRow{Model: model, ClassifierCost: classifierCost, UpstreamCostSource: economics.UpstreamCostSource, MarginStatus: economics.ModelMarginStatus}
		if out.Tariff != nil {
			row.CustomerCharge = out.Tariff.CustomerCharge(GPTAutoTariffUsage{
				InputTokens: uncached, CachedInputTokens: in.CachedInputTokens, OutputTokens: in.OutputTokens,
			}, 1, rate)
		} else {
			row.CustomerCharge = gptAutoDecimal(economics.EstimatedRevenue)
		}
		if economics.PricingAvailable && economics.EstimateQuality != GPTAutoEstimateQualityInsufficient {
			upstream := gptAutoDecimal(economics.EstimatedUpstreamCost)
			gross := row.CustomerCharge.Sub(upstream)
			loaded := gross.Sub(classifierCost)
			row.UpstreamCost, row.GrossProfit, row.FullyLoadedProfit = &upstream, &gross, &loaded
			row.GrossMargin = gptAutoMargin(gross, row.CustomerCharge)
			row.FullyLoadedMargin = gptAutoMargin(loaded, row.CustomerCharge)
			row.MarginStatus = GPTAutoMarginStatus(row.GrossMargin, cfg.MinMarginFor(model))
			if gross.IsNegative() {
				losses = append(losses, model)
			} else if loaded.IsNegative() {
				out.Warnings = append(out.Warnings, "fully_loaded_loss:"+model)
			}
		} else {
			row.UpstreamCostSource = GPTAutoCostSourceNone
			out.Warnings = append(out.Warnings, "upstream_cost_unknown:"+model)
		}
		out.Rows = append(out.Rows, row)
	}
	for _, model := range losses {
		out.Warnings = append(out.Warnings, "gross_loss:"+model)
	}
	if out.Tariff != nil && out.Tariff.Input.IsZero() && out.Tariff.Output.IsZero() && out.Tariff.CachedInput.IsZero() {
		out.Warnings = append(out.Warnings, "tariff_is_free")
	}
	return out, nil
}
