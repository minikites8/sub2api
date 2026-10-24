package service

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/require"
)

// Phase 2.1: independent gpt-auto pricing and the mandatory group switch.

func gptAutoTestDecimal(t *testing.T, value string) *decimal.Decimal {
	t.Helper()
	out, err := decimal.NewFromString(value)
	require.NoError(t, err)
	return &out
}

// gptAutoExampleTariff is the specification's example (a test fixture, not a
// recommended price): $2 input, $0.50 cached input, $8 output per 1M tokens.
func gptAutoExampleTariff(t *testing.T) GPTAutoPricingConfig {
	return GPTAutoPricingConfig{
		Mode:                    GPTAutoPricingModeFixed,
		InputPricePerMTok:       gptAutoTestDecimal(t, "2"),
		CachedInputPricePerMTok: gptAutoTestDecimal(t, "0.5"),
		OutputPricePerMTok:      gptAutoTestDecimal(t, "8"),
		Version:                 3,
	}
}

func newGPTAutoFixedPricingTestService(t *testing.T, mutate func(*GPTAutoShadowConfig)) (*GPTAutoShadowService, *gptAutoTestRepo, *gptAutoTestClassifier) {
	t.Helper()
	return newGPTAutoLiveTestService(t, func(c *GPTAutoShadowConfig) {
		c.Pricing = gptAutoExampleTariff(t)
		if mutate != nil {
			mutate(c)
		}
	})
}

func TestGPTAutoPricingDefaultsAndMigration(t *testing.T) {
	fresh := DefaultGPTAutoShadowConfig()
	require.Equal(t, GPTAutoPricingModeFixed, fresh.Pricing.Mode, "new installations use independent pricing")
	require.False(t, fresh.Pricing.Ready(), "and expose no paid gpt-auto traffic until a tariff exists")

	// A Phase 2 document already serving gpt-auto keeps its billing.
	phase2Live := parseGPTAutoShadowConfig(`{"all_groups":true,"live":{"live_enabled":true}}`)
	require.Equal(t, GPTAutoPricingModeLegacy, phase2Live.Pricing.Mode)
	require.True(t, phase2Live.Pricing.Ready())

	// Existing configurations preserve legacy mode even with live routing off.
	phase2Off := parseGPTAutoShadowConfig(`{"all_groups":true,"live":{"live_enabled":false}}`)
	require.Equal(t, GPTAutoPricingModeLegacy, phase2Off.Pricing.Mode)
	require.True(t, phase2Off.Pricing.Ready())

	// Once stored, the pricing section is authoritative (round trip, decimals intact).
	stored := gptAutoExampleTariff(t)
	raw, err := json.Marshal(map[string]any{"all_groups": true, "live": map[string]any{"live_enabled": true}, "pricing": stored})
	require.NoError(t, err)
	parsed := parseGPTAutoShadowConfig(string(raw))
	require.Equal(t, GPTAutoPricingModeFixed, parsed.Pricing.Mode)
	require.Equal(t, "0.5", parsed.Pricing.CachedInputPricePerMTok.String())
	require.Equal(t, 3, parsed.Pricing.Version)
}

func TestGPTAutoPricingValidation(t *testing.T) {
	valid := gptAutoExampleTariff(t)
	require.NoError(t, ValidateGPTAutoPricingActivation(valid))

	zero := valid
	zero.InputPricePerMTok, zero.CachedInputPricePerMTok, zero.OutputPricePerMTok = gptAutoTestDecimal(t, "0"), gptAutoTestDecimal(t, "0"), gptAutoTestDecimal(t, "0")
	require.NoError(t, ValidateGPTAutoPricingActivation(zero), "zero is an explicit administrative choice")
	require.True(t, zero.Ready())

	for name, mutate := range map[string]func(*GPTAutoPricingConfig){
		"negative":     func(p *GPTAutoPricingConfig) { p.OutputPricePerMTok = gptAutoTestDecimal(t, "-1") },
		"out of range": func(p *GPTAutoPricingConfig) { p.InputPricePerMTok = gptAutoTestDecimal(t, "1000.000001") },
		"precision":    func(p *GPTAutoPricingConfig) { p.InputPricePerMTok = gptAutoTestDecimal(t, "0.0000001") },
		"bad mode":     func(p *GPTAutoPricingConfig) { p.Mode = "free" },
		"missing rate": func(p *GPTAutoPricingConfig) { p.CachedInputPricePerMTok = nil },
	} {
		candidate := valid
		mutate(&candidate)
		require.Error(t, ValidateGPTAutoPricingActivation(candidate), name)
	}
	// Non-finite values never become a decimal.
	var bad GPTAutoPricingConfig
	require.Error(t, json.Unmarshal([]byte(`{"input_price_per_mtok":"NaN"}`), &bad))
	require.Error(t, json.Unmarshal([]byte(`{"input_price_per_mtok":"Infinity"}`), &bad))

	// The general config save accepts an incomplete fixed tariff (it cannot
	// be sold anyway); only the dedicated pricing save requires it.
	incomplete := GPTAutoPricingConfig{Mode: GPTAutoPricingModeFixed}
	require.NoError(t, ValidateGPTAutoPricingConfig(incomplete))
	require.Error(t, ValidateGPTAutoPricingActivation(incomplete))
}

func TestGPTAutoTariffSpecificationExample(t *testing.T) {
	tariff, ok := gptAutoExampleTariff(t).TariffFor(nil)
	require.True(t, ok)
	// Total input 1000 of which 200 cached, output 500 (reasoning included).
	usage := GPTAutoTariffUsageFromTokens(UsageTokens{InputTokens: 800, CacheReadTokens: 200, OutputTokens: 500})
	require.Equal(t, "0.0057", tariff.CustomerCharge(usage, 1, 1).String())
	require.Equal(t, "0.00456", tariff.CustomerCharge(usage, 1, 0.8).String(), "the customer rate applies once")
	// Cache creation without a configured rate is billed at the input rate, never zero.
	withWrite := GPTAutoTariffUsageFromTokens(UsageTokens{CacheCreationTokens: 1_000_000})
	require.Equal(t, "2", tariff.CustomerCharge(withWrite, 1, 1).String())
}

func TestGPTAutoTariffGroupOverride(t *testing.T) {
	pricing := gptAutoExampleTariff(t)
	input, output := 0.000001, 0.000004 // $1 and $4 per 1M tokens
	group := &Group{ID: 7, ModelPricing: []ChannelModelPricing{
		{Models: []string{"gpt-*"}, InputPrice: &output}, // a wildcard for real models never reprices gpt-auto
	}}
	tariff, ok := pricing.TariffFor(group)
	require.True(t, ok)
	require.Equal(t, GPTAutoPricingSourceGlobal, tariff.Source)
	require.Equal(t, "2", tariff.Input.String())

	group.ModelPricing = append(group.ModelPricing, ChannelModelPricing{Models: []string{"GPT-Auto"}, InputPrice: &input, OutputPrice: &output})
	tariff, ok = pricing.TariffFor(group)
	require.True(t, ok)
	require.Equal(t, GPTAutoPricingSourceGroupOverride, tariff.Source)
	require.Equal(t, "1", tariff.Input.String())
	require.Equal(t, "4", tariff.Output.String())
	require.Equal(t, "0.5", tariff.CachedInput.String(), "unset override fields keep the global tariff")

	view := GPTAutoGroupPricingViewFor(&GPTAutoShadowConfig{Pricing: pricing}, &Group{ID: 8, Platform: PlatformOpenAI, ModelRateMultipliers: map[string]float64{"gpt-auto": 0.5}})
	require.Equal(t, GPTAutoPricingSourceGroupAdjusted, view.PricingSource)
	require.Equal(t, "1", view.EffectiveInput.String())
	require.Equal(t, "group_model_rate", view.RateSource)
}

// TestGPTAutoAdmissionMatrix covers the mandatory authorization rule.
func TestGPTAutoAdmissionMatrix(t *testing.T) {
	groupID := int64(7)
	base := func() (GPTAutoShadowConfig, GPTAutoRouteInput) {
		cfg := *DefaultGPTAutoShadowConfig()
		cfg.Live.Enabled = true
		cfg.Live.AllowedAPIKeyIDs = []int64{22}
		cfg.Pricing = gptAutoExampleTariff(t)
		return cfg, GPTAutoRouteInput{APIKeyID: 22, GroupID: &groupID,
			Group: &Group{ID: groupID, Platform: PlatformOpenAI, GPTAutoEnabled: true}}
	}
	cfg, input := base()
	require.True(t, gptAutoAdmits(cfg, input), "every condition holds")

	cases := map[string]func(*GPTAutoShadowConfig, *GPTAutoRouteInput){
		"group switch off":          func(_ *GPTAutoShadowConfig, in *GPTAutoRouteInput) { in.Group.GPTAutoEnabled = false },
		"no group":                  func(_ *GPTAutoShadowConfig, in *GPTAutoRouteInput) { in.Group = nil },
		"non-OpenAI group":          func(_ *GPTAutoShadowConfig, in *GPTAutoRouteInput) { in.Group.Platform = PlatformAnthropic },
		"group id mismatch":         func(_ *GPTAutoShadowConfig, in *GPTAutoRouteInput) { other := int64(8); in.GroupID = &other },
		"live disabled":             func(c *GPTAutoShadowConfig, _ *GPTAutoRouteInput) { c.Live.Enabled = false },
		"internal-only, not listed": func(_ *GPTAutoShadowConfig, in *GPTAutoRouteInput) { in.APIKeyID = 23 },
		"tariff missing":            func(c *GPTAutoShadowConfig, _ *GPTAutoRouteInput) { c.Pricing.OutputPricePerMTok = nil },
		"allowlist denies gpt-auto": func(_ *GPTAutoShadowConfig, in *GPTAutoRouteInput) {
			in.ModelPermitted = func(model string) bool { return model != GPTAutoVirtualModel }
		},
		"rollout excludes group": func(c *GPTAutoShadowConfig, in *GPTAutoRouteInput) {
			c.Live.InternalOnly, c.Live.AllowedAPIKeyIDs, c.Live.AllowedGroupIDs = false, nil, []int64{9}
			in.APIKeyID = 23
		},
	}
	for name, mutate := range cases {
		cfg, input := base()
		mutate(&cfg, &input)
		require.False(t, gptAutoAdmits(cfg, input), name)
	}

	// internal_only=false: an empty group rollout list means every group whose
	// own switch is on; a configured list still restricts.
	cfg, input = base()
	cfg.Live.InternalOnly, cfg.Live.AllowedAPIKeyIDs = false, nil
	input.APIKeyID = 23
	require.True(t, gptAutoAdmits(cfg, input))
	cfg.Live.AllowedGroupIDs = []int64{7}
	require.True(t, gptAutoAdmits(cfg, input))
	input.Group.GPTAutoEnabled = false
	require.False(t, gptAutoAdmits(cfg, input), "the rollout list never replaces the group switch")

	// Legacy mode needs no tariff.
	cfg, input = base()
	cfg.Pricing = GPTAutoPricingConfig{Mode: GPTAutoPricingModeLegacy}
	require.True(t, gptAutoAdmits(cfg, input))
}

func TestGPTAutoDisabledGroupNeverReachesJev(t *testing.T) {
	svc, repo, classifier := newGPTAutoFixedPricingTestService(t, nil)
	input := gptAutoLiveInput(gptAutoLiveChatBody)
	input.Group.GPTAutoEnabled = false
	route, err := svc.Route(context.Background(), input)
	require.Nil(t, route)
	routeErr := gptAutoRouteErr(t, err)
	require.Equal(t, http.StatusNotFound, routeErr.Status)
	require.Equal(t, "model_not_found", routeErr.Code)
	require.NotContains(t, routeErr.Message, "group", "no authorization detail is disclosed")
	require.Zero(t, classifier.calls.Load(), "no Jev call and no classifier fee")
	require.Zero(t, repo.count())
	require.False(t, svc.LiveAdmits(context.Background(), input), "and not listed in /v1/models")
}

func TestGPTAutoRevokedGroupCannotUseSessionAffinity(t *testing.T) {
	svc, _, classifier := newGPTAutoFixedPricingTestService(t, nil)
	gptAutoClassifyAs(classifier, GPTAutoModelGPT61Sol, GPTAutoEffortHigh)
	input := gptAutoLiveInput(gptAutoLiveChatBody)
	input.SessionKey = "agent-session-1"
	route, err := svc.Route(context.Background(), input)
	require.NoError(t, err)
	route.Finish(context.Background(), GPTAutoRouteOutcome{HTTPStatus: http.StatusOK})
	require.Equal(t, int64(1), classifier.calls.Load())

	// Affinity hit while enabled.
	again, err := svc.Route(context.Background(), input)
	require.NoError(t, err)
	require.Equal(t, GPTAutoModelGPT61Sol, again.Model())
	require.Equal(t, int64(1), classifier.calls.Load(), "affinity hit")

	// Revoke the group switch: the stored affinity is not a bypass.
	input.Group.GPTAutoEnabled = false
	denied, err := svc.Route(context.Background(), input)
	require.Nil(t, denied)
	require.Equal(t, http.StatusNotFound, gptAutoRouteErr(t, err).Status)
	require.Equal(t, int64(1), classifier.calls.Load())
}

func TestGPTAutoFixedRouteSnapshotsTariff(t *testing.T) {
	svc, repo, classifier := newGPTAutoFixedPricingTestService(t, func(c *GPTAutoShadowConfig) {
		c.Live.ApplyBillingMultiplier = true // legacy multipliers must not apply in fixed mode
	})
	gptAutoClassifyAs(classifier, GPTAutoModelGPT61Sol, GPTAutoEffortHigh)
	route, err := svc.Route(context.Background(), gptAutoLiveInput(gptAutoLiveChatBody))
	require.NoError(t, err)
	require.Equal(t, 1.0, route.BillingMultiplier(), "profile multiplier is not combined with the tariff")
	tariff, fixed := route.FixedTariff()
	require.True(t, fixed)
	require.Equal(t, 3, tariff.Version)

	// An administrator changes the tariff while the request streams.
	cfg, err := svc.settings.GetGPTAutoShadowConfig(context.Background())
	require.NoError(t, err)
	cfg.Pricing.OutputPricePerMTok = gptAutoTestDecimal(t, "80")
	cfg.Pricing.Version = 4
	require.NoError(t, svc.settings.SetGPTAutoShadowConfig(context.Background(), cfg))
	snapshot, _ := route.FixedTariff()
	require.Equal(t, "8", snapshot.Output.String(), "the request keeps the tariff captured before execution")
	require.Equal(t, 3, snapshot.Version)

	// A reroute keeps the same snapshot.
	require.True(t, route.Reroute(GPTAutoFailureNoUpstream))
	afterReroute, _ := route.FixedTariff()
	require.Equal(t, snapshot, afterReroute)
	require.Equal(t, 1.0, route.BillingMultiplier())

	route.Finish(context.Background(), GPTAutoRouteOutcome{HTTPStatus: http.StatusOK})
	repo.waitRows(t, 1)
	row := repo.lastRow(t)
	require.Equal(t, GPTAutoVirtualModel, row.PricingModel)
	require.Equal(t, GPTAutoPricingModeFixed, row.PricingMode)
	require.Equal(t, 3, *row.PricingConfigVersion)
	require.Equal(t, GPTAutoPricingSourceGlobal, row.PricingSource)
	require.Equal(t, "8", row.TariffOutputPerMTok.String())
	require.Equal(t, 1.0, *row.BillingMultiplier)
}

func TestGPTAutoFixedPricingRejectsUnpricedFeatures(t *testing.T) {
	svc, repo, classifier := newGPTAutoFixedPricingTestService(t, nil)
	input := gptAutoLiveInput(`{"model":"gpt-auto","input":[{"type":"input_audio","data":"test"}]}`)
	input.Endpoint = GPTAutoEndpointResponses
	route, err := svc.Route(context.Background(), input)
	require.Nil(t, route)
	routeErr := gptAutoRouteErr(t, err)
	require.Equal(t, http.StatusBadRequest, routeErr.Status)
	require.Equal(t, "gpt_auto_unsupported_feature", routeErr.Code)
	require.Zero(t, classifier.calls.Load())
	repo.waitRows(t, 1)
	require.Equal(t, GPTAutoFailureUnpricedFeature, repo.lastRow(t).ErrorCategory)
}

func TestGPTAutoFixedPricingAllowsHostedTools(t *testing.T) {
	for _, kind := range []string{"web_search", "web_search_preview", "file_search", "code_interpreter", "image_generation", "mcp", "computer", "function", "custom"} {
		t.Run(kind, func(t *testing.T) {
			svc, _, classifier := newGPTAutoFixedPricingTestService(t, nil)
			input := gptAutoLiveInput(`{"model":"gpt-auto","input":"use a tool","tools":[{"type":"` + kind + `"}]}`)
			input.Endpoint = GPTAutoEndpointResponses
			route, err := svc.Route(context.Background(), input)
			require.NoError(t, err)
			require.NotNil(t, route)
			require.EqualValues(t, 1, classifier.calls.Load())
		})
	}
}

func TestGPTAutoLegacyModeKeepsProfileMultiplier(t *testing.T) {
	svc, _, classifier := newGPTAutoLiveTestService(t, func(c *GPTAutoShadowConfig) {
		c.Live.ApplyBillingMultiplier = true
	})
	gptAutoClassifyAs(classifier, GPTAutoModelGPT61Sol, GPTAutoEffortHigh)
	route, err := svc.Route(context.Background(), gptAutoLiveInput(gptAutoLiveChatBody))
	require.NoError(t, err)
	profile, _ := route.cfg.ProfileFor(GPTAutoModelGPT61Sol)
	require.Equal(t, profile.BillingMultiplier, route.BillingMultiplier())
	_, fixed := route.FixedTariff()
	require.False(t, fixed)
}

func TestGPTAutoUpdatePricingIsConfirmedAndVersioned(t *testing.T) {
	svc, _, _ := newGPTAutoLiveTestService(t, nil)
	ctx := context.Background()
	next := gptAutoExampleTariff(t)
	_, err := svc.UpdatePricing(ctx, GPTAutoPricingUpdate{Pricing: next, ExpectedVersion: 0})
	require.Error(t, err, "confirmation is required")
	_, err = svc.UpdatePricing(ctx, GPTAutoPricingUpdate{Pricing: next, ExpectedVersion: 5, Confirmed: true})
	require.Error(t, err, "a stale version is a conflict")
	state, err := svc.UpdatePricing(ctx, GPTAutoPricingUpdate{Pricing: next, ExpectedVersion: 0, Confirmed: true, ActorID: 1})
	require.NoError(t, err)
	require.Equal(t, 1, state.Pricing.Version)
	require.NotNil(t, state.Pricing.UpdatedAt)
	require.Equal(t, int64(1), state.Pricing.UpdatedBy)
	require.True(t, state.Ready)
	stored, err := svc.settings.GetGPTAutoShadowConfig(ctx)
	require.NoError(t, err)
	require.Equal(t, 1, stored.Pricing.Version)
}

func TestGPTAutoPricingSimulationSameChargeDifferentMargins(t *testing.T) {
	svc, _, _ := newGPTAutoFixedPricingTestService(t, func(c *GPTAutoShadowConfig) {
		for i := range c.ModelProfiles {
			c.ModelProfiles[i].UpstreamCosts = []GPTAutoUpstreamCost{{Channel: "test", Enabled: true, InputPerM: float64(i + 1), CachedInputPerM: 0.1, OutputPerM: float64(i+1) * 3}}
		}
	})
	sim, err := svc.SimulatePricing(context.Background(), GPTAutoPricingSimulationInput{
		TotalInputTokens: 1000, CachedInputTokens: 200, OutputTokens: 500,
	})
	require.NoError(t, err)
	require.Len(t, sim.Rows, len(GPTAutoModelPool()))
	require.Equal(t, 800, sim.UncachedInput)
	upstream := map[string]bool{}
	for _, row := range sim.Rows {
		require.Equal(t, "0.0057", row.CustomerCharge.String(), row.Model)
		if row.UpstreamCost != nil {
			upstream[row.UpstreamCost.String()] = true
			require.Equal(t, row.CustomerCharge.Sub(*row.UpstreamCost).String(), row.GrossProfit.String())
			require.Equal(t, row.GrossProfit.Sub(row.ClassifierCost).String(), row.FullyLoadedProfit.String())
		}
	}
	require.Greater(t, len(upstream), 1, "procurement differs by executed model")

	_, err = svc.SimulatePricing(context.Background(), GPTAutoPricingSimulationInput{TotalInputTokens: 10, CachedInputTokens: 20})
	require.Error(t, err, "cached input cannot exceed total input")
	_, err = svc.SimulatePricing(context.Background(), GPTAutoPricingSimulationInput{ExecutedModel: "gpt-5.6-sol"})
	require.Error(t, err, "the retired model is not part of the pool")
}

func TestGPTAutoEconomicsUseTariffInFixedMode(t *testing.T) {
	svc, _, _ := newGPTAutoFixedPricingTestService(t, nil)
	cfg := svc.settings.GetGPTAutoShadowConfigCached(context.Background())
	in, cached, out := 800, 200, 500
	revenues := map[string]bool{}
	for _, model := range GPTAutoModelPool() {
		economics := EstimateGPTAutoEconomics(GPTAutoPolicyInput{
			Config: cfg, Pricing: svc.pricing,
			Assumptions: &GPTAutoWorkloadAssumptions{InputTokens: &in, CachedInputTokens: &cached, OutputTokens: &out},
		}, model, GPTAutoEffortHigh)
		require.Equal(t, GPTAutoRevenueBasisTariff, economics.RevenueBasis)
		require.Equal(t, 1.0, economics.BillingMultiplier)
		revenues[decimal.NewFromFloat(economics.EstimatedRevenue).String()] = true
		require.True(t, economics.Hypothetical)
	}
	require.Len(t, revenues, 1, "projected revenue does not depend on the executed model")
	require.True(t, revenues["0.0057"])
}

var _ = time.Second

func TestGPTAutoListedKeyCannotBypassGroupRollout(t *testing.T) {
	live := DefaultGPTAutoLiveConfig()
	live.Enabled = true
	live.InternalOnly = false
	live.AllowedAPIKeyIDs = []int64{22}
	live.AllowedGroupIDs = []int64{9}
	group := int64(7)
	require.False(t, live.AdmitsAPIKey(22, &group))
}

func TestGPTAutoFixedTariffRetainsAudioRestrictions(t *testing.T) {
	for _, body := range []string{
		`{"messages":[{"content":[{"type":"input_audio","input_audio":{"data":"x"}}]}]}`,
		`{"modalities":["audio"]}`,
	} {
		require.NotEmpty(t, gptAutoUnpricedFeature([]byte(body)))
	}
	require.Empty(t, gptAutoUnpricedFeature([]byte(`{"tools":[{"type":"function","function":{"name":"lookup"}}]}`)))
}
