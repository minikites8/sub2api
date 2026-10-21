package service

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"strings"
	"time"
	"unicode/utf8"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/Wei-Shaw/sub2api/internal/pkg/typesafe"
	"github.com/Wei-Shaw/sub2api/internal/util/urlvalidator"
)

// GPTAutoPolicyVersionDefault identifies the shipped policy rule set. Stored on
// every decision row so historical decisions stay reproducible after the rules
// or weights change.
const GPTAutoPolicyVersionDefault = "shadow-v1"

// Inbound endpoints the shadow observer supports. Streaming is irrelevant here
// because the observer never touches the response, but endpoints whose request
// shape is not an OpenAI-style chat payload are excluded outright.
const (
	GPTAutoEndpointChatCompletions = "chat_completions"
	GPTAutoEndpointResponses       = "responses"
)

// GPTAutoSupportedEndpoints lists the endpoints eligible for shadow sampling.
//
// Deliberately excluded, and why:
//   - /v1/messages (Anthropic), Gemini and Antigravity native shapes: the Phase 1
//     candidate pool is OpenAI-family only, so a recommendation would not be
//     actionable in Phase 2.
//   - /v1/systemone: that endpoint is the Jev classifier itself; observing it
//     would recurse.
//   - GET /v1/responses (WebSocket) and realtime endpoints: no single bounded
//     request body exists to snapshot.
//   - count_tokens, images, videos: no reasoning-effort decision to make.
func GPTAutoSupportedEndpoints() []string {
	return []string{GPTAutoEndpointChatCompletions, GPTAutoEndpointResponses}
}

// GPTAutoUpstreamCost is a shadow-only procurement cost for one upstream
// channel serving a model, in USD per 1,000,000 tokens. The admin UI uses
// per-million figures; the gateway pricing table stores USD per single token,
// so the policy engine converts with gptAutoTokensPerMillion.
type GPTAutoUpstreamCost struct {
	Channel         string  `json:"channel"`
	Enabled         bool    `json:"enabled"`
	InputPerM       float64 `json:"input_per_m"`
	OutputPerM      float64 `json:"output_per_m"`
	CachedInputPerM float64 `json:"cached_input_per_m"`
}

const gptAutoTokensPerMillion = 1_000_000

const gptAutoMaxUpstreamChannels = 16

// GPTAutoModelProfile holds the per-model shadow configuration.
//
// BillingMultiplier and RoutingWeight are deliberately independent:
// BillingMultiplier only scales the hypothetical retail price used in profit
// estimates, while RoutingWeight only breaks ties between models that already
// satisfy the request's capability requirements. Raising one never changes the
// other, and neither can make an incompatible model eligible.
type GPTAutoModelProfile struct {
	Model string `json:"model"`
	// Description is the capability positioning sent to Jev as this model's
	// choice criterion. It is operator-maintained: the gateway has no vendor
	// capability metadata for these models, so the defaults are neutral.
	Description       string   `json:"description"`
	Enabled           bool     `json:"enabled"`
	BillingMultiplier float64  `json:"billing_multiplier"`
	RoutingWeight     int      `json:"routing_weight"`
	SupportedEfforts  []string `json:"supported_efforts"`
	// SupportsTools gates candidacy for requests that carry tool definitions.
	SupportsTools bool `json:"supports_tools"`
	// Endpoints restricts the model to specific inbound endpoints. Empty means
	// every supported endpoint.
	Endpoints []string `json:"endpoints,omitempty"`
	// MinMargin overrides GlobalMinMargin for this model when non-nil.
	MinMargin *float64 `json:"min_margin,omitempty"`
	// UpstreamCosts lists procurement costs per upstream channel. The cheapest
	// enabled channel becomes the preferred hypothetical channel. When empty,
	// the estimate assumes procurement at the pricing-table list price.
	UpstreamCosts []GPTAutoUpstreamCost `json:"upstream_costs,omitempty"`
}

// GPTAutoShadowConfig is persisted as one JSON document under
// SettingKeyGPTAutoShadowConfig.
type GPTAutoShadowConfig struct {
	// ShadowEnabled is the global kill switch. Default false.
	ShadowEnabled bool `json:"shadow_enabled"`
	// SampleRate is a percentage from 0 to 100. Default 0.
	SampleRate int      `json:"sample_rate"`
	Endpoints  []string `json:"endpoints"`
	AllGroups  bool     `json:"all_groups"`
	GroupIDs   []int64  `json:"group_ids"`
	// ModelFilter restricts sampling to these customer-requested models.
	// Empty means every model.
	ModelFilter []string `json:"model_filter"`

	// Classifier credentials are stored the same way content moderation stores
	// its TypeSafe credentials: inline in this settings document, never routed
	// through a customer-billable Account row and never sent to the frontend
	// (the admin GET path returns ClassifierAPIKeyMasked instead, see
	// gpt_auto_shadow_handler.go).
	ClassifierAPIKey         string `json:"classifier_api_key,omitempty"`
	ClassifierBaseURL        string `json:"classifier_base_url"`
	ClassifierProxyID        *int64 `json:"classifier_proxy_id,omitempty"`
	ClassifierModel          string `json:"classifier_model"`
	ClassifierTimeoutMS      int    `json:"classifier_timeout_ms"`
	ClassifierMaxConcurrency int    `json:"classifier_max_concurrency"`
	// ClassifierRetryCount is the number of extra attempts after a transient
	// failure (transport, 5xx, 429). Timeouts and bad answers are not retried.
	ClassifierRetryCount          int     `json:"classifier_retry_count"`
	ClassifierDailyCallLimit      int     `json:"classifier_daily_call_limit"`
	ClassifierDailyCostLimit      float64 `json:"classifier_daily_cost_limit"`
	ClassificationMaxContextChars int     `json:"classification_max_context_chars"`
	ClassificationContextMessages int     `json:"classification_context_messages"`

	WorkerCount int `json:"worker_count"`
	QueueSize   int `json:"queue_size"`

	DecisionRetentionDays int `json:"decision_retention_days"`

	GlobalMinMargin    float64               `json:"global_min_margin"`
	ProfitSafetyBuffer float64               `json:"profit_safety_buffer"`
	ModelProfiles      []GPTAutoModelProfile `json:"model_profiles"`
	PolicyVersion      string                `json:"policy_version"`
}

// Bounds. Validation rejects anything outside these; normalization clamps.
const (
	gptAutoMaxSampleRate          = 100
	gptAutoMaxWorkerCount         = 32
	gptAutoMaxQueueSize           = 100000
	gptAutoMaxTimeoutMS           = 60000
	gptAutoMinTimeoutMS           = 200
	gptAutoMaxConcurrency         = 64
	gptAutoMaxContextChars        = 200000
	gptAutoMaxContextMessages     = 20
	gptAutoMaxRetentionDays       = 3650
	gptAutoMaxBillingMultiplier   = 100
	gptAutoMaxRoutingWeight       = 1000
	gptAutoMaxCostPerM            = 10000
	gptAutoMaxDailyCallLimit      = 10000000
	gptAutoMaxDailyCostLimit      = 1000000
	gptAutoMaxPolicyVersionLength = 64
	gptAutoMaxDescriptionRunes    = 500
	gptAutoMaxRetryCount          = 3
)

const (
	gptAutoDefaultTimeoutMS       = 8000
	gptAutoDefaultConcurrency     = 4
	gptAutoDefaultWorkerCount     = 2
	gptAutoDefaultQueueSize       = 4096
	gptAutoDefaultContextChars    = 4000
	gptAutoDefaultContextMessages = 4
	gptAutoDefaultRetentionDays   = 30
	gptAutoDefaultDailyCallLimit  = 10000
	gptAutoDefaultDailyCostLimit  = 5.0
	gptAutoDefaultMinMargin       = 0.30
	gptAutoDefaultSafetyBuffer    = 0.05
)

const (
	gptAutoConfigCacheTTL  = 30 * time.Second
	gptAutoConfigErrorTTL  = 5 * time.Second
	gptAutoConfigDBTimeout = 5 * time.Second
)

type cachedGPTAutoShadowConfig struct {
	config    GPTAutoShadowConfig
	expiresAt int64 // unix nano
}

// DefaultGPTAutoModelProfiles returns the shipped starting profiles.
//
// The multipliers and weights below are the Phase 1 example values from the
// product brief, not measured optima. Supported efforts start at each model's
// natively supported set (see gptAutoNativeSupportedEfforts).
func DefaultGPTAutoModelProfiles() []GPTAutoModelProfile {
	seed := map[string]struct {
		multiplier float64
		weight     int
	}{
		GPTAutoModelGPT56Luna: {1.20, 60},
		GPTAutoModelGPT56Sol:  {1.40, 55},
		GPTAutoModelGPT6Luna:  {1.10, 100},
		GPTAutoModelGPT61Sol:  {1.50, 90},
	}
	profiles := make([]GPTAutoModelProfile, 0, len(seed))
	for _, model := range GPTAutoModelPool() {
		profiles = append(profiles, GPTAutoModelProfile{
			Model:             model,
			Description:       gptAutoDefaultModelDescription(model),
			Enabled:           true,
			BillingMultiplier: seed[model].multiplier,
			RoutingWeight:     seed[model].weight,
			SupportedEfforts:  gptAutoNativeSupportedEfforts(model),
			SupportsTools:     true,
		})
	}
	return profiles
}

// gptAutoDefaultModelDescription states only what the identifiers themselves
// convey (generation and tier). Operators are expected to refine these.
func gptAutoDefaultModelDescription(model string) string {
	switch model {
	case GPTAutoModelGPT56Luna:
		return "GPT-5.6 generation, Luna tier: the lighter option of the previous generation."
	case GPTAutoModelGPT56Sol:
		return "GPT-5.6 generation, Sol tier: the stronger option of the previous generation."
	case GPTAutoModelGPT6Luna:
		return "GPT-6 generation, Luna tier: the lighter option of the current generation."
	case GPTAutoModelGPT61Sol:
		return "GPT-6.1 generation, Sol tier: the strongest option in the pool, for the hardest tasks; does not run with reasoning disabled."
	default:
		return model
	}
}

// DefaultGPTAutoShadowConfig is disabled and zero-sampled: deploying this
// feature must be a no-op until an administrator opts in.
func DefaultGPTAutoShadowConfig() *GPTAutoShadowConfig {
	return &GPTAutoShadowConfig{
		ShadowEnabled:                 false,
		SampleRate:                    0,
		Endpoints:                     GPTAutoSupportedEndpoints(),
		AllGroups:                     true,
		GroupIDs:                      nil,
		ModelFilter:                   nil,
		ClassifierBaseURL:             typesafe.DefaultBaseURL,
		ClassifierModel:               typesafe.JevLatestModel,
		ClassifierTimeoutMS:           gptAutoDefaultTimeoutMS,
		ClassifierMaxConcurrency:      gptAutoDefaultConcurrency,
		ClassifierRetryCount:          1,
		ClassifierDailyCallLimit:      gptAutoDefaultDailyCallLimit,
		ClassifierDailyCostLimit:      gptAutoDefaultDailyCostLimit,
		ClassificationMaxContextChars: gptAutoDefaultContextChars,
		ClassificationContextMessages: gptAutoDefaultContextMessages,
		WorkerCount:                   gptAutoDefaultWorkerCount,
		QueueSize:                     gptAutoDefaultQueueSize,
		DecisionRetentionDays:         gptAutoDefaultRetentionDays,
		GlobalMinMargin:               gptAutoDefaultMinMargin,
		ProfitSafetyBuffer:            gptAutoDefaultSafetyBuffer,
		ModelProfiles:                 DefaultGPTAutoModelProfiles(),
		PolicyVersion:                 GPTAutoPolicyVersionDefault,
	}
}

// normalizeGPTAutoShadowConfig coerces a stored document into a usable shape.
// Reads never fail on bad data; writes are validated separately by
// ValidateGPTAutoShadowConfig.
func normalizeGPTAutoShadowConfig(cfg *GPTAutoShadowConfig) {
	if cfg == nil {
		return
	}
	cfg.SampleRate = clampInt(cfg.SampleRate, 0, gptAutoMaxSampleRate)
	cfg.ClassifierTimeoutMS = clampIntDefault(cfg.ClassifierTimeoutMS, gptAutoMinTimeoutMS, gptAutoMaxTimeoutMS, gptAutoDefaultTimeoutMS)
	cfg.ClassifierMaxConcurrency = clampIntDefault(cfg.ClassifierMaxConcurrency, 1, gptAutoMaxConcurrency, gptAutoDefaultConcurrency)
	cfg.ClassifierRetryCount = clampInt(cfg.ClassifierRetryCount, 0, gptAutoMaxRetryCount)
	cfg.WorkerCount = clampIntDefault(cfg.WorkerCount, 1, gptAutoMaxWorkerCount, gptAutoDefaultWorkerCount)
	cfg.QueueSize = clampIntDefault(cfg.QueueSize, 1, gptAutoMaxQueueSize, gptAutoDefaultQueueSize)
	cfg.ClassificationMaxContextChars = clampIntDefault(cfg.ClassificationMaxContextChars, 100, gptAutoMaxContextChars, gptAutoDefaultContextChars)
	cfg.ClassificationContextMessages = clampInt(cfg.ClassificationContextMessages, 0, gptAutoMaxContextMessages)
	cfg.DecisionRetentionDays = clampIntDefault(cfg.DecisionRetentionDays, 1, gptAutoMaxRetentionDays, gptAutoDefaultRetentionDays)
	cfg.ClassifierDailyCallLimit = clampInt(cfg.ClassifierDailyCallLimit, 0, gptAutoMaxDailyCallLimit)
	cfg.ClassifierDailyCostLimit = clampFloat(cfg.ClassifierDailyCostLimit, 0, gptAutoMaxDailyCostLimit)
	cfg.GlobalMinMargin = clampFloat(cfg.GlobalMinMargin, 0, 0.99)
	cfg.ProfitSafetyBuffer = clampFloat(cfg.ProfitSafetyBuffer, 0, 0.99)

	if strings.TrimSpace(cfg.ClassifierModel) == "" {
		cfg.ClassifierModel = typesafe.JevLatestModel
	}
	if strings.TrimSpace(cfg.ClassifierBaseURL) == "" {
		cfg.ClassifierBaseURL = typesafe.DefaultBaseURL
	}
	cfg.ClassifierAPIKey = strings.TrimSpace(cfg.ClassifierAPIKey)
	if strings.TrimSpace(cfg.PolicyVersion) == "" {
		cfg.PolicyVersion = GPTAutoPolicyVersionDefault
	}
	cfg.Endpoints = normalizeGPTAutoEndpoints(cfg.Endpoints)
	cfg.ModelProfiles = normalizeGPTAutoModelProfiles(cfg.ModelProfiles)
}

func normalizeGPTAutoEndpoints(in []string) []string {
	supported := GPTAutoSupportedEndpoints()
	out := make([]string, 0, len(supported))
	for _, endpoint := range supported {
		for _, candidate := range in {
			if strings.EqualFold(strings.TrimSpace(candidate), endpoint) {
				out = append(out, endpoint)
				break
			}
		}
	}
	if len(out) == 0 {
		return supported
	}
	return out
}

// normalizeGPTAutoModelProfiles guarantees exactly one profile per pool model,
// in pool order, so the admin table and the policy engine always agree.
func normalizeGPTAutoModelProfiles(in []GPTAutoModelProfile) []GPTAutoModelProfile {
	byModel := make(map[string]GPTAutoModelProfile, len(in))
	for _, profile := range in {
		model := strings.ToLower(strings.TrimSpace(profile.Model))
		if !IsGPTAutoModel(model) {
			continue
		}
		profile.Model = model
		byModel[model] = profile
	}
	defaults := DefaultGPTAutoModelProfiles()
	out := make([]GPTAutoModelProfile, 0, len(defaults))
	for _, fallback := range defaults {
		profile, ok := byModel[fallback.Model]
		if !ok {
			out = append(out, fallback)
			continue
		}
		profile.BillingMultiplier = clampFloatDefault(profile.BillingMultiplier, 0, gptAutoMaxBillingMultiplier, fallback.BillingMultiplier)
		profile.RoutingWeight = clampInt(profile.RoutingWeight, 0, gptAutoMaxRoutingWeight)
		// Admins may narrow the native set but never widen it: an effort the
		// gateway would reject must not become a shadow recommendation.
		profile.SupportedEfforts = intersectEfforts(profile.SupportedEfforts, fallback.SupportedEfforts)
		if len(profile.SupportedEfforts) == 0 {
			profile.SupportedEfforts = fallback.SupportedEfforts
		}
		profile.Endpoints = normalizeGPTAutoProfileEndpoints(profile.Endpoints)
		profile.Description = trimRunes(strings.TrimSpace(profile.Description), gptAutoMaxDescriptionRunes)
		if profile.Description == "" {
			profile.Description = fallback.Description
		}
		if profile.MinMargin != nil {
			margin := clampFloat(*profile.MinMargin, 0, 0.99)
			profile.MinMargin = &margin
		}
		costs := make([]GPTAutoUpstreamCost, 0, len(profile.UpstreamCosts))
		for _, cost := range profile.UpstreamCosts {
			cost.Channel = strings.TrimSpace(cost.Channel)
			if cost.Channel == "" || len(costs) >= gptAutoMaxUpstreamChannels {
				continue
			}
			cost.InputPerM = clampFloat(cost.InputPerM, 0, gptAutoMaxCostPerM)
			cost.OutputPerM = clampFloat(cost.OutputPerM, 0, gptAutoMaxCostPerM)
			cost.CachedInputPerM = clampFloat(cost.CachedInputPerM, 0, gptAutoMaxCostPerM)
			costs = append(costs, cost)
		}
		profile.UpstreamCosts = costs
		out = append(out, profile)
	}
	return out
}

func normalizeGPTAutoProfileEndpoints(in []string) []string {
	if len(in) == 0 {
		return nil
	}
	out := make([]string, 0, len(in))
	for _, endpoint := range GPTAutoSupportedEndpoints() {
		for _, candidate := range in {
			if strings.EqualFold(strings.TrimSpace(candidate), endpoint) {
				out = append(out, endpoint)
				break
			}
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func intersectEfforts(requested, allowed []string) []string {
	allowedSet := make(map[string]struct{}, len(allowed))
	for _, effort := range allowed {
		allowedSet[effort] = struct{}{}
	}
	out := make([]string, 0, len(requested))
	for _, effort := range gptAutoSortEfforts(requested) {
		if _, ok := allowedSet[effort]; ok {
			out = append(out, effort)
		}
	}
	return out
}

// ValidateGPTAutoShadowConfig rejects an administrator's input instead of
// silently coercing it, so a typo surfaces as a 400 rather than a surprise.
func ValidateGPTAutoShadowConfig(cfg *GPTAutoShadowConfig) error {
	if cfg == nil {
		return infraerrors.BadRequest("GPT_AUTO_CONFIG_INVALID", "configuration is required")
	}
	if cfg.SampleRate < 0 || cfg.SampleRate > gptAutoMaxSampleRate {
		return infraerrors.BadRequest("GPT_AUTO_CONFIG_INVALID", "sample_rate must be between 0 and 100")
	}
	if cfg.ClassifierTimeoutMS < gptAutoMinTimeoutMS || cfg.ClassifierTimeoutMS > gptAutoMaxTimeoutMS {
		return infraerrors.BadRequest("GPT_AUTO_CONFIG_INVALID", "classifier_timeout_ms must be between 200 and 60000")
	}
	if cfg.ClassifierMaxConcurrency < 1 || cfg.ClassifierMaxConcurrency > gptAutoMaxConcurrency {
		return infraerrors.BadRequest("GPT_AUTO_CONFIG_INVALID", "classifier_max_concurrency must be between 1 and 64")
	}
	if cfg.ClassifierRetryCount < 0 || cfg.ClassifierRetryCount > gptAutoMaxRetryCount {
		return infraerrors.BadRequest("GPT_AUTO_CONFIG_INVALID", "classifier_retry_count must be between 0 and 3")
	}
	if cfg.WorkerCount < 1 || cfg.WorkerCount > gptAutoMaxWorkerCount {
		return infraerrors.BadRequest("GPT_AUTO_CONFIG_INVALID", "worker_count must be between 1 and 32")
	}
	if cfg.QueueSize < 1 || cfg.QueueSize > gptAutoMaxQueueSize {
		return infraerrors.BadRequest("GPT_AUTO_CONFIG_INVALID", "queue_size must be between 1 and 100000")
	}
	if cfg.ClassifierDailyCallLimit < 0 || cfg.ClassifierDailyCallLimit > gptAutoMaxDailyCallLimit {
		return infraerrors.BadRequest("GPT_AUTO_CONFIG_INVALID", "classifier_daily_call_limit must not be negative")
	}
	if cfg.ClassifierDailyCostLimit < 0 || cfg.ClassifierDailyCostLimit > gptAutoMaxDailyCostLimit {
		return infraerrors.BadRequest("GPT_AUTO_CONFIG_INVALID", "classifier_daily_cost_limit must not be negative")
	}
	if cfg.ClassificationMaxContextChars < 100 || cfg.ClassificationMaxContextChars > gptAutoMaxContextChars {
		return infraerrors.BadRequest("GPT_AUTO_CONFIG_INVALID", "classification_max_context_chars must be between 100 and 200000")
	}
	if cfg.ClassificationContextMessages < 0 || cfg.ClassificationContextMessages > gptAutoMaxContextMessages {
		return infraerrors.BadRequest("GPT_AUTO_CONFIG_INVALID", "classification_context_messages must be between 0 and 20")
	}
	if cfg.DecisionRetentionDays < 1 || cfg.DecisionRetentionDays > gptAutoMaxRetentionDays {
		return infraerrors.BadRequest("GPT_AUTO_CONFIG_INVALID", "decision_retention_days must be between 1 and 3650")
	}
	if cfg.GlobalMinMargin < 0 || cfg.GlobalMinMargin >= 1 {
		return infraerrors.BadRequest("GPT_AUTO_CONFIG_INVALID", "global_min_margin must be between 0 and 1 (exclusive)")
	}
	if cfg.ProfitSafetyBuffer < 0 || cfg.ProfitSafetyBuffer >= 1 {
		return infraerrors.BadRequest("GPT_AUTO_CONFIG_INVALID", "profit_safety_buffer must be between 0 and 1 (exclusive)")
	}
	if strings.TrimSpace(cfg.ClassifierModel) == "" || len(cfg.ClassifierModel) > 64 {
		return infraerrors.BadRequest("GPT_AUTO_CONFIG_INVALID", "classifier_model is required (max 64 characters)")
	}
	// The classifier receives sampled customer text, so the endpoint must be
	// HTTPS and must not point at an internal address.
	if strings.TrimSpace(cfg.ClassifierBaseURL) != "" {
		if _, err := urlvalidator.ValidateHTTPSURL(cfg.ClassifierBaseURL, urlvalidator.ValidationOptions{}); err != nil {
			return infraerrors.BadRequest("GPT_AUTO_CONFIG_INVALID", "classifier_base_url must be a public https URL")
		}
	}
	if len(cfg.ClassifierAPIKey) > 512 {
		return infraerrors.BadRequest("GPT_AUTO_CONFIG_INVALID", "classifier_api_key is too long")
	}
	if len(cfg.PolicyVersion) > gptAutoMaxPolicyVersionLength {
		return infraerrors.BadRequest("GPT_AUTO_CONFIG_INVALID", "policy_version is too long")
	}
	for _, endpoint := range cfg.Endpoints {
		if !isGPTAutoSupportedEndpoint(endpoint) {
			return infraerrors.BadRequest("GPT_AUTO_CONFIG_INVALID", "unsupported endpoint: "+endpoint)
		}
	}
	if !cfg.AllGroups && len(cfg.GroupIDs) == 0 {
		return infraerrors.BadRequest("GPT_AUTO_CONFIG_INVALID", "select at least one group or enable all_groups")
	}
	seen := make(map[string]struct{}, len(cfg.ModelProfiles))
	for _, profile := range cfg.ModelProfiles {
		model := strings.ToLower(strings.TrimSpace(profile.Model))
		if !IsGPTAutoModel(model) {
			return infraerrors.BadRequest("GPT_AUTO_CONFIG_INVALID", "unsupported model: "+profile.Model)
		}
		if _, dup := seen[model]; dup {
			return infraerrors.BadRequest("GPT_AUTO_CONFIG_INVALID", "duplicate model profile: "+model)
		}
		seen[model] = struct{}{}
		if utf8.RuneCountInString(profile.Description) > gptAutoMaxDescriptionRunes {
			return infraerrors.BadRequest("GPT_AUTO_CONFIG_INVALID", "description is too long for "+model)
		}
		if profile.BillingMultiplier <= 0 || profile.BillingMultiplier > gptAutoMaxBillingMultiplier {
			return infraerrors.BadRequest("GPT_AUTO_CONFIG_INVALID", "billing_multiplier must be greater than 0 for "+model)
		}
		if profile.RoutingWeight < 0 || profile.RoutingWeight > gptAutoMaxRoutingWeight {
			return infraerrors.BadRequest("GPT_AUTO_CONFIG_INVALID", "routing_weight must be between 0 and 1000 for "+model)
		}
		native := gptAutoNativeSupportedEfforts(model)
		nativeSet := make(map[string]struct{}, len(native))
		for _, effort := range native {
			nativeSet[effort] = struct{}{}
		}
		for _, effort := range profile.SupportedEfforts {
			normalized := strings.ToLower(strings.TrimSpace(effort))
			if !IsGPTAutoEffort(normalized) {
				return infraerrors.BadRequest("GPT_AUTO_CONFIG_INVALID", "unknown reasoning effort "+effort+" for "+model)
			}
			if _, ok := nativeSet[normalized]; !ok {
				return infraerrors.BadRequest("GPT_AUTO_CONFIG_INVALID",
					model+" does not support reasoning effort "+normalized)
			}
		}
		if profile.MinMargin != nil && (*profile.MinMargin < 0 || *profile.MinMargin >= 1) {
			return infraerrors.BadRequest("GPT_AUTO_CONFIG_INVALID", "min_margin must be between 0 and 1 (exclusive) for "+model)
		}
		if len(profile.UpstreamCosts) > gptAutoMaxUpstreamChannels {
			return infraerrors.BadRequest("GPT_AUTO_CONFIG_INVALID", "too many upstream channels for "+model)
		}
		channels := make(map[string]struct{}, len(profile.UpstreamCosts))
		for _, cost := range profile.UpstreamCosts {
			name := strings.TrimSpace(cost.Channel)
			if name == "" || len(name) > 100 {
				return infraerrors.BadRequest("GPT_AUTO_CONFIG_INVALID", "upstream channel name is required for "+model)
			}
			if _, dup := channels[strings.ToLower(name)]; dup {
				return infraerrors.BadRequest("GPT_AUTO_CONFIG_INVALID", "duplicate upstream channel "+name+" for "+model)
			}
			channels[strings.ToLower(name)] = struct{}{}
			for _, value := range []float64{cost.InputPerM, cost.OutputPerM, cost.CachedInputPerM} {
				if value < 0 || value > gptAutoMaxCostPerM {
					return infraerrors.BadRequest("GPT_AUTO_CONFIG_INVALID", "upstream cost must be between 0 and 10000 for "+model)
				}
			}
		}
		for _, endpoint := range profile.Endpoints {
			if !isGPTAutoSupportedEndpoint(endpoint) {
				return infraerrors.BadRequest("GPT_AUTO_CONFIG_INVALID", "unsupported endpoint "+endpoint+" for "+model)
			}
		}
	}
	return nil
}

func isGPTAutoSupportedEndpoint(endpoint string) bool {
	for _, supported := range GPTAutoSupportedEndpoints() {
		if strings.EqualFold(strings.TrimSpace(endpoint), supported) {
			return true
		}
	}
	return false
}

// GetGPTAutoShadowConfig reads the stored document for the admin read/write
// path. Missing, empty or corrupt values fall back to defaults rather than
// failing the request.
func (s *SettingService) GetGPTAutoShadowConfig(ctx context.Context) (*GPTAutoShadowConfig, error) {
	if s == nil || s.settingRepo == nil {
		return DefaultGPTAutoShadowConfig(), nil
	}
	raw, err := s.settingRepo.GetValue(ctx, SettingKeyGPTAutoShadowConfig)
	if err != nil {
		if errors.Is(err, ErrSettingNotFound) {
			return DefaultGPTAutoShadowConfig(), nil
		}
		return nil, err
	}
	return parseGPTAutoShadowConfig(raw), nil
}

func parseGPTAutoShadowConfig(raw string) *GPTAutoShadowConfig {
	cfg := DefaultGPTAutoShadowConfig()
	if strings.TrimSpace(raw) == "" {
		return cfg
	}
	if err := json.Unmarshal([]byte(raw), cfg); err != nil {
		slog.Warn("gpt_auto.config_unmarshal_failed", "error", err)
		return DefaultGPTAutoShadowConfig()
	}
	normalizeGPTAutoShadowConfig(cfg)
	return cfg
}

// SetGPTAutoShadowConfig validates and persists the document, then primes this
// node's cache so the next request on this node sees the new value immediately.
// Other nodes converge within gptAutoConfigCacheTTL.
func (s *SettingService) SetGPTAutoShadowConfig(ctx context.Context, cfg *GPTAutoShadowConfig) error {
	if s == nil || s.settingRepo == nil {
		return errors.New("setting repository unavailable")
	}
	if err := ValidateGPTAutoShadowConfig(cfg); err != nil {
		return err
	}
	normalized := *cfg
	normalizeGPTAutoShadowConfig(&normalized)
	raw, err := json.Marshal(&normalized)
	if err != nil {
		return err
	}
	if err := s.settingRepo.Set(ctx, SettingKeyGPTAutoShadowConfig, string(raw)); err != nil {
		return err
	}
	s.storeGPTAutoShadowConfigCache(normalized, gptAutoConfigCacheTTL)
	return nil
}

// GetGPTAutoShadowConfigCached is the hot-path reader used by the gateway
// observation hook. It never returns an error and never blocks on the DB for
// longer than gptAutoConfigDBTimeout.
func (s *SettingService) GetGPTAutoShadowConfigCached(ctx context.Context) GPTAutoShadowConfig {
	if s == nil || s.settingRepo == nil {
		return *DefaultGPTAutoShadowConfig()
	}
	if cached, ok := s.loadGPTAutoShadowConfigCache(); ok {
		return cached
	}
	value, _, _ := s.gptAutoConfigSF.Do(SettingKeyGPTAutoShadowConfig, func() (any, error) {
		if cached, ok := s.loadGPTAutoShadowConfigCache(); ok {
			return cached, nil
		}
		// Detach from the caller: a disconnecting client must not poison the
		// cache for every other request on this node.
		loadCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), gptAutoConfigDBTimeout)
		defer cancel()
		cfg, err := s.GetGPTAutoShadowConfig(loadCtx)
		if err != nil || cfg == nil {
			slog.Warn("gpt_auto.config_load_failed", "error", err)
			fallback := *DefaultGPTAutoShadowConfig()
			if previous := s.gptAutoConfigCache.Load(); previous != nil {
				if entry, ok := previous.(*cachedGPTAutoShadowConfig); ok {
					fallback = entry.config
				}
			}
			s.storeGPTAutoShadowConfigCache(fallback, gptAutoConfigErrorTTL)
			return fallback, nil
		}
		s.storeGPTAutoShadowConfigCache(*cfg, gptAutoConfigCacheTTL)
		return *cfg, nil
	})
	if cfg, ok := value.(GPTAutoShadowConfig); ok {
		return cfg
	}
	return *DefaultGPTAutoShadowConfig()
}

func (s *SettingService) loadGPTAutoShadowConfigCache() (GPTAutoShadowConfig, bool) {
	entry, ok := s.gptAutoConfigCache.Load().(*cachedGPTAutoShadowConfig)
	if !ok || entry == nil || time.Now().UnixNano() >= entry.expiresAt {
		return GPTAutoShadowConfig{}, false
	}
	return entry.config, true
}

func (s *SettingService) storeGPTAutoShadowConfigCache(cfg GPTAutoShadowConfig, ttl time.Duration) {
	s.gptAutoConfigCache.Store(&cachedGPTAutoShadowConfig{
		config:    cfg,
		expiresAt: time.Now().Add(ttl).UnixNano(),
	})
}

// GatewayPublicURLs returns this gateway's configured public URLs (API base
// and frontend), used to stop the classifier from being pointed at the
// customer-billable gateway itself.
func (s *SettingService) GatewayPublicURLs(ctx context.Context) ([]string, error) {
	if s == nil || s.settingRepo == nil {
		return nil, nil
	}
	values, err := s.settingRepo.GetMultiple(ctx, []string{SettingKeyAPIBaseURL, SettingKeyFrontendURL})
	if err != nil {
		return nil, err
	}
	urls := make([]string, 0, 2)
	for _, key := range []string{SettingKeyAPIBaseURL, SettingKeyFrontendURL} {
		if value := strings.TrimSpace(values[key]); value != "" {
			urls = append(urls, value)
		}
	}
	return urls, nil
}

// ProfileFor returns the configured profile for a model.
func (c *GPTAutoShadowConfig) ProfileFor(model string) (GPTAutoModelProfile, bool) {
	target := strings.ToLower(strings.TrimSpace(model))
	for _, profile := range c.ModelProfiles {
		if profile.Model == target {
			return profile, true
		}
	}
	return GPTAutoModelProfile{}, false
}

// IncludesGroup reports whether a group is in the sampling scope.
func (c *GPTAutoShadowConfig) IncludesGroup(groupID *int64) bool {
	if c.AllGroups {
		return true
	}
	if groupID == nil {
		return false
	}
	for _, id := range c.GroupIDs {
		if id == *groupID {
			return true
		}
	}
	return false
}

// IncludesModel reports whether a customer-requested model is in scope.
func (c *GPTAutoShadowConfig) IncludesModel(model string) bool {
	if len(c.ModelFilter) == 0 {
		return true
	}
	target := strings.ToLower(strings.TrimSpace(model))
	for _, candidate := range c.ModelFilter {
		if strings.ToLower(strings.TrimSpace(candidate)) == target {
			return true
		}
	}
	return false
}

// IncludesEndpoint reports whether an inbound endpoint is in scope.
func (c *GPTAutoShadowConfig) IncludesEndpoint(endpoint string) bool {
	for _, candidate := range c.Endpoints {
		if candidate == endpoint {
			return true
		}
	}
	return false
}

// MinMarginFor resolves the effective margin floor for a model, including the
// global safety buffer.
func (c *GPTAutoShadowConfig) MinMarginFor(model string) float64 {
	minMargin := c.GlobalMinMargin
	if profile, ok := c.ProfileFor(model); ok && profile.MinMargin != nil {
		minMargin = *profile.MinMargin
	}
	return minMargin + c.ProfitSafetyBuffer
}

func clampInt(value, low, high int) int {
	if value < low {
		return low
	}
	if value > high {
		return high
	}
	return value
}

func clampIntDefault(value, low, high, fallback int) int {
	if value <= 0 {
		return fallback
	}
	return clampInt(value, low, high)
}

func clampFloat(value, low, high float64) float64 {
	if value < low {
		return low
	}
	if value > high {
		return high
	}
	return value
}

func clampFloatDefault(value, low, high, fallback float64) float64 {
	if value <= 0 {
		return fallback
	}
	return clampFloat(value, low, high)
}
