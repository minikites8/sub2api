package service

import (
	"fmt"
	"math"
	"strings"
	"time"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/shopspring/decimal"
)

// GPT-Auto Phase 2.1: independent virtual-model pricing.
//
// Three model identities are kept apart for every live request:
//
//	requested_model = gpt-auto         what the customer asked for
//	pricing_model   = gpt-auto         the retail tariff the customer pays
//	executed_model  = e.g. gpt-6.1-sol what Jev selected and the upstream ran
//
// In virtual_model_fixed_rates mode the customer charge depends only on the
// usage quantities, the GPT-Auto tariff and the customer's ordinary pricing
// modifiers (user / group / account rate, peak, service tier). It does not
// change with the executed model. The executed model keeps driving the
// upstream (procurement) cost through usage_logs.total_cost and
// account_stats_cost exactly as for a direct request.
//
// legacy_executed_model_multiplier keeps the Phase 2 behaviour: the executed
// model's ordinary price times its GPT-Auto profile billing_multiplier. The
// two systems are never combined: in fixed-rate mode the profile multiplier
// is not applied.

const (
	GPTAutoPricingModeLegacy = "legacy_executed_model_multiplier"
	GPTAutoPricingModeFixed  = "virtual_model_fixed_rates"

	// Pricing sources recorded on the decision row.
	GPTAutoPricingSourceGlobal        = "global"
	GPTAutoPricingSourceGroupAdjusted = "group_adjusted"
	GPTAutoPricingSourceGroupOverride = "group_override"
	GPTAutoPricingSourceLegacy        = "executed_model"

	// gptAutoMaxPricePerMTok bounds every configured rate (USD / 1M tokens).
	gptAutoMaxPricePerMTok = 1000
	// gptAutoPriceDecimals is the precision of a configured rate.
	gptAutoPriceDecimals = 6
)

var gptAutoMillion = decimal.NewFromInt(1_000_000)

// GPTAutoPricingConfig is the retail tariff of the gpt-auto virtual model.
// Rates are USD per one million tokens. A nil rate is "not configured"; zero
// is an explicit administrative choice. It is edited through its own admin
// endpoint (validated, previewed, confirmed and versioned); the general
// GPT-Auto config update preserves it.
type GPTAutoPricingConfig struct {
	Mode string `json:"mode"`

	InputPricePerMTok       *decimal.Decimal `json:"input_price_per_mtok"`
	CachedInputPricePerMTok *decimal.Decimal `json:"cached_input_price_per_mtok"`
	OutputPricePerMTok      *decimal.Decimal `json:"output_price_per_mtok"`
	// CacheWritePricePerMTok prices separately reported cache-creation input.
	// Nil bills it at the uncached input rate (never at zero).
	CacheWritePricePerMTok *decimal.Decimal `json:"cache_write_price_per_mtok"`

	// Version increases on every saved change; captured per request.
	Version   int        `json:"version"`
	UpdatedAt *time.Time `json:"updated_at,omitempty"`
	UpdatedBy int64      `json:"updated_by,omitempty"`
}

// DefaultGPTAutoPricingConfig is the new-installation default: independent
// pricing selected but no tariff, so paid gpt-auto traffic stays unavailable
// until an administrator configures one.
func DefaultGPTAutoPricingConfig() GPTAutoPricingConfig {
	return GPTAutoPricingConfig{Mode: GPTAutoPricingModeFixed}
}

// IsFixed reports whether independent virtual-model pricing is active.
func (p GPTAutoPricingConfig) IsFixed() bool { return p.Mode == GPTAutoPricingModeFixed }

// TariffConfigured reports whether the three mandatory token rates are set.
func (p GPTAutoPricingConfig) TariffConfigured() bool {
	return p.InputPricePerMTok != nil && p.CachedInputPricePerMTok != nil && p.OutputPricePerMTok != nil
}

// Ready reports whether a gpt-auto request can be priced. Legacy mode is
// always ready (it prices like the executed model); fixed mode needs a tariff.
func (p GPTAutoPricingConfig) Ready() bool {
	switch p.Mode {
	case GPTAutoPricingModeLegacy:
		return true
	case GPTAutoPricingModeFixed:
		return p.TariffConfigured() && ValidateGPTAutoPricingConfig(p) == nil
	}
	return false
}

// PricingModel is the retail pricing identity recorded for a request.
func (p GPTAutoPricingConfig) PricingModel(executedModel string) string {
	if p.IsFixed() {
		return GPTAutoVirtualModel
	}
	return executedModel
}

func normalizeGPTAutoPricingConfig(p *GPTAutoPricingConfig) {
	p.Mode = strings.TrimSpace(p.Mode)
	if p.Mode != GPTAutoPricingModeLegacy && p.Mode != GPTAutoPricingModeFixed {
		p.Mode = GPTAutoPricingModeFixed
	}
	if p.Version < 0 {
		p.Version = 0
	}
}

// ValidateGPTAutoPricingConfig rejects malformed tariffs. Decimal values are
// always finite; NaN / Inf never parse into decimal.Decimal.
func ValidateGPTAutoPricingConfig(p GPTAutoPricingConfig) error {
	if err := validateGPTAutoPricingConfig(p); err != nil {
		return infraerrors.BadRequest("GPT_AUTO_PRICING_INVALID", err.Error())
	}
	return nil
}

func validateGPTAutoPricingConfig(p GPTAutoPricingConfig) error {
	if p.Mode != GPTAutoPricingModeLegacy && p.Mode != GPTAutoPricingModeFixed {
		return fmt.Errorf("pricing.mode must be %s or %s", GPTAutoPricingModeLegacy, GPTAutoPricingModeFixed)
	}
	rates := []struct {
		name  string
		value *decimal.Decimal
	}{
		{"input_price_per_mtok", p.InputPricePerMTok},
		{"cached_input_price_per_mtok", p.CachedInputPricePerMTok},
		{"output_price_per_mtok", p.OutputPricePerMTok},
		{"cache_write_price_per_mtok", p.CacheWritePricePerMTok},
	}
	for _, rate := range rates {
		if rate.value == nil {
			continue
		}
		if rate.value.IsNegative() {
			return fmt.Errorf("pricing.%s must not be negative", rate.name)
		}
		if rate.value.GreaterThan(decimal.NewFromInt(gptAutoMaxPricePerMTok)) {
			return fmt.Errorf("pricing.%s must be at most %d USD per 1M tokens", rate.name, gptAutoMaxPricePerMTok)
		}
		if -rate.value.Exponent() > gptAutoPriceDecimals {
			return fmt.Errorf("pricing.%s allows at most %d decimal places", rate.name, gptAutoPriceDecimals)
		}
	}
	return nil
}

// ValidateGPTAutoPricingActivation is the stricter check of the dedicated
// pricing save: fixed-rate mode must carry a complete tariff.
func ValidateGPTAutoPricingActivation(p GPTAutoPricingConfig) error {
	if err := ValidateGPTAutoPricingConfig(p); err != nil {
		return err
	}
	if p.Mode == GPTAutoPricingModeFixed && !p.TariffConfigured() {
		return infraerrors.BadRequest("GPT_AUTO_PRICING_INVALID", "independent pricing needs input, cached input and output prices")
	}
	return nil
}

// GPTAutoTariff is the immutable per-request tariff snapshot (USD / 1M
// tokens, before the customer's multipliers). It is captured when the route
// is created, before execution, and used unchanged at settlement.
type GPTAutoTariff struct {
	Mode          string          `json:"mode"`
	Version       int             `json:"version"`
	Source        string          `json:"source"`
	Input         decimal.Decimal `json:"input_price_per_mtok"`
	CachedInput   decimal.Decimal `json:"cached_input_price_per_mtok"`
	Output        decimal.Decimal `json:"output_price_per_mtok"`
	CacheWrite    decimal.Decimal `json:"cache_write_price_per_mtok"`
	CacheWriteSet bool            `json:"cache_write_configured"`
}

// gptAutoGroupOverride returns the group's explicit gpt-auto price entry. Only
// an exact token-mode entry counts: a wildcard written for real models (for
// example gpt-*) must not silently reprice the virtual model.
func gptAutoGroupOverride(group *Group) *ChannelModelPricing {
	if group == nil {
		return nil
	}
	for i := range group.ModelPricing {
		entry := &group.ModelPricing[i]
		if entry.BillingMode != "" && entry.BillingMode != BillingModeToken {
			continue
		}
		for _, model := range entry.Models {
			if strings.EqualFold(strings.TrimSpace(model), GPTAutoVirtualModel) {
				return entry
			}
		}
	}
	return nil
}

func gptAutoPerTokenToMTok(perToken float64) decimal.Decimal {
	return decimal.NewFromFloat(perToken).Mul(gptAutoMillion).Round(gptAutoPriceDecimals + 4)
}

// TariffFor resolves the base tariff for a group: the global GPT-Auto tariff,
// with any field of an explicit group gpt-auto price entry replacing it. The
// second result is false when the tariff is incomplete.
func (p GPTAutoPricingConfig) TariffFor(group *Group) (GPTAutoTariff, bool) {
	if !p.IsFixed() {
		return GPTAutoTariff{Mode: p.Mode, Version: p.Version, Source: GPTAutoPricingSourceLegacy}, true
	}
	if !p.TariffConfigured() || ValidateGPTAutoPricingConfig(p) != nil {
		return GPTAutoTariff{Mode: p.Mode, Version: p.Version}, false
	}
	t := GPTAutoTariff{
		Mode: p.Mode, Version: p.Version, Source: GPTAutoPricingSourceGlobal,
		Input: *p.InputPricePerMTok, CachedInput: *p.CachedInputPricePerMTok, Output: *p.OutputPricePerMTok,
	}
	if p.CacheWritePricePerMTok != nil {
		t.CacheWrite, t.CacheWriteSet = *p.CacheWritePricePerMTok, true
	}
	if override := gptAutoGroupOverride(group); override != nil {
		for _, value := range []*float64{override.InputPrice, override.CacheReadPrice, override.OutputPrice, override.CacheWritePrice} {
			if value != nil && (math.IsNaN(*value) || math.IsInf(*value, 0) || *value < 0 || *value > float64(gptAutoMaxPricePerMTok)/1_000_000) {
				return GPTAutoTariff{}, false
			}
		}
		t.Source = GPTAutoPricingSourceGroupOverride
		if override.InputPrice != nil {
			t.Input = gptAutoPerTokenToMTok(*override.InputPrice)
		}
		if override.CacheReadPrice != nil {
			t.CachedInput = gptAutoPerTokenToMTok(*override.CacheReadPrice)
		}
		if override.OutputPrice != nil {
			t.Output = gptAutoPerTokenToMTok(*override.OutputPrice)
		}
		if override.CacheWritePrice != nil {
			t.CacheWrite, t.CacheWriteSet = gptAutoPerTokenToMTok(*override.CacheWritePrice), true
		}
	}
	if !t.CacheWriteSet {
		t.CacheWrite = t.Input
	}
	return t, true
}

// GPTAutoTariffUsage is the metered usage of one request, in mutually
// exclusive buckets (the gateway's UsageTokens convention): Input excludes
// cached and cache-creation tokens; Output already includes reasoning tokens.
type GPTAutoTariffUsage struct {
	InputTokens         int
	CachedInputTokens   int
	CacheCreationTokens int
	OutputTokens        int
}

// GPTAutoTariffUsageFromTokens maps the gateway's billing buckets.
func GPTAutoTariffUsageFromTokens(tokens UsageTokens) GPTAutoTariffUsage {
	return GPTAutoTariffUsage{
		InputTokens:         max(tokens.InputTokens, 0),
		CachedInputTokens:   max(tokens.CacheReadTokens, 0),
		CacheCreationTokens: max(tokens.CacheCreationTokens, 0),
		OutputTokens:        max(tokens.OutputTokens, 0),
	}
}

// ListCharge is the tariff price of the usage before any customer multiplier.
func (t GPTAutoTariff) ListCharge(u GPTAutoTariffUsage) decimal.Decimal {
	sum := decimal.NewFromInt(int64(u.InputTokens)).Mul(t.Input).
		Add(decimal.NewFromInt(int64(u.CachedInputTokens)).Mul(t.CachedInput)).
		Add(decimal.NewFromInt(int64(u.CacheCreationTokens)).Mul(t.CacheWrite)).
		Add(decimal.NewFromInt(int64(u.OutputTokens)).Mul(t.Output))
	return sum.Div(gptAutoMillion)
}

// CustomerCharge applies the service-tier factor and the customer's combined
// rate multiplier exactly once, rounded to the 10 decimals of usage_logs.
func (t GPTAutoTariff) CustomerCharge(u GPTAutoTariffUsage, tierMultiplier, rateMultiplier float64) decimal.Decimal {
	if tierMultiplier < 0 {
		tierMultiplier = 0
	}
	if rateMultiplier < 0 {
		rateMultiplier = 0
	}
	return t.ListCharge(u).Mul(decimal.NewFromFloat(tierMultiplier)).Mul(decimal.NewFromFloat(rateMultiplier)).Round(10)
}

// gptAutoTariffUnsupportedDimension reports a usage dimension the token
// tariff cannot price (generated media, audio, video). These dimensions use
// the existing upstream cost path rather than the virtual text tariff.
func gptAutoTariffUnsupportedDimension(result *OpenAIForwardResult) string {
	if result == nil {
		return ""
	}
	switch {
	case result.ImageCount > 0 || result.Usage.ImageOutputTokens > 0:
		return "image_generation"
	case result.AudioUsage != nil:
		return "audio"
	case result.VideoCount > 0:
		return "video"
	}
	return ""
}
