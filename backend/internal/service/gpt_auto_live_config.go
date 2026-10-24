package service

import (
	"strings"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
)

// GPTAutoVirtualModel is the public model id customers send to request live
// GPT-Auto routing. It never reaches an upstream: the live router rewrites it
// to one pool model before scheduling.
const GPTAutoVirtualModel = "gpt-auto"

// IsGPTAutoVirtualModel reports whether a customer-requested model is the
// GPT-Auto virtual model.
func IsGPTAutoVirtualModel(model string) bool {
	return strings.EqualFold(strings.TrimSpace(model), GPTAutoVirtualModel)
}

// Classifier failure policies for live routing.
const (
	// GPTAutoFailureCompatibleFallback serves the configured fallback model
	// (or the best remaining eligible candidate) when Jev cannot answer. It
	// never selects a model that fails the request's capability checks or the
	// customer's model permissions.
	GPTAutoFailureCompatibleFallback = "compatible_fallback"
	// GPTAutoFailureRejectRequest returns a structured 503 instead.
	GPTAutoFailureRejectRequest = "reject_request"
)

// Client reasoning-effort precedence for gpt-auto requests.
const (
	// GPTAutoClientEffortJev: Jev decides; an explicit client effort is
	// recorded (actual_effort) but not applied. Default.
	GPTAutoClientEffortJev = "jev"
	// GPTAutoClientEffortHonor: an explicit, valid client effort replaces
	// Jev's effort; the model is still Jev's choice. The effort is then
	// normalized to what the selected model supports, like any other.
	GPTAutoClientEffortHonor = "honor_client"
)

// GPTAutoModelCapabilities are per-model live-routing constraints. The
// gateway has no vendor capability metadata for the pool, so these are
// operator-maintained assertions; defaults assume the GPT-5.6/GPT-6 family
// baseline (vision, structured outputs, stateful Responses) and do not
// enforce a context window. Upstream channel support is checked separately
// against the group's schedulable accounts.
type GPTAutoModelCapabilities struct {
	Vision            bool `json:"vision"`
	StructuredOutputs bool `json:"structured_outputs"`
	// StatefulResponses gates Responses continuations that reference
	// upstream-held state (previous_response_id / conversation).
	StatefulResponses bool `json:"stateful_responses"`
	// MaxContextTokens rejects requests whose estimated prompt exceeds it.
	// 0 means not enforced.
	MaxContextTokens int `json:"max_context_tokens"`
}

// DefaultGPTAutoModelCapabilities returns the shipped capability baseline.
func DefaultGPTAutoModelCapabilities() *GPTAutoModelCapabilities {
	return &GPTAutoModelCapabilities{Vision: true, StructuredOutputs: true, StatefulResponses: true}
}

const gptAutoMaxContextTokensLimit = 10_000_000

func normalizeGPTAutoModelCapabilities(in *GPTAutoModelCapabilities) *GPTAutoModelCapabilities {
	if in == nil {
		return DefaultGPTAutoModelCapabilities()
	}
	out := *in
	out.MaxContextTokens = clampInt(out.MaxContextTokens, 0, gptAutoMaxContextTokensLimit)
	return &out
}

// GPTAutoLiveConfig controls Phase 2 live routing. Persisted inside the GPT
// Auto settings document under "live".
type GPTAutoLiveConfig struct {
	// Enabled is the live-routing kill switch. Default false: gpt-auto is
	// then unknown to every customer and every request behaves as before.
	Enabled bool `json:"live_enabled"`
	// InternalOnly limits gpt-auto to AllowedAPIKeyIDs. When false, keys in
	// AllowedGroupIDs are admitted too. Default true.
	InternalOnly     bool    `json:"internal_only"`
	AllowedGroupIDs  []int64 `json:"allowed_groups"`
	AllowedAPIKeyIDs []int64 `json:"allowed_api_keys"`
	// Endpoints that accept gpt-auto. Empty means every supported endpoint.
	Endpoints []string `json:"endpoints"`

	// The synchronous classifier path has its own timeout, concurrency limit
	// and circuit breaker, independent of the shadow workers.
	ClassifierTimeoutMS      int    `json:"classifier_timeout_ms"`
	ClassifierMaxConcurrency int    `json:"classifier_max_concurrency"`
	ClassifierRetryCount     int    `json:"classifier_retry_count"`
	ClassifierFailurePolicy  string `json:"classifier_failure_policy"`
	FallbackModel            string `json:"fallback_model"`
	FallbackEffort           string `json:"fallback_effort"`
	ClientEffortPolicy       string `json:"client_effort_policy"`

	SessionAffinityEnabled    bool `json:"session_affinity_enabled"`
	SessionAffinityTTLSeconds int  `json:"session_affinity_ttl_seconds"`

	// MarginGuardEnabled excludes a candidate whose estimated model margin is
	// a loss (with sufficient estimate data) when another eligible candidate
	// remains. It never makes an incompatible model eligible.
	MarginGuardEnabled bool `json:"margin_guard_enabled"`
	// ApplyBillingMultiplier charges the executed model's profile
	// BillingMultiplier on top of the normal customer price. Default true.
	ApplyBillingMultiplier bool `json:"apply_billing_multiplier"`
	// DailyRequestBudget caps synchronous live Jev classifications per UTC day
	// across the cluster (session-affinity hits do not consume it). 0 means
	// unlimited. Beyond it, requests follow the classifier failure policy. The
	// shared Jev daily call/cost budget applies on top.
	DailyRequestBudget int `json:"live_request_budget"`

	// CustomerExportEnabled lets customers export their own gpt-auto history.
	CustomerExportEnabled bool `json:"customer_export_enabled"`
}

const (
	gptAutoLiveDefaultTimeoutMS     = 2500
	gptAutoLiveDefaultConcurrency   = 16
	gptAutoLiveMaxConcurrency       = 256
	gptAutoLiveMaxRetryCount        = 1
	gptAutoLiveDefaultAffinityTTL   = 3600
	gptAutoLiveMinAffinityTTL       = 60
	gptAutoLiveMaxAffinityTTL       = 7 * 24 * 3600
	gptAutoLiveDefaultDailyBudget   = 10000
	gptAutoLiveMaxDailyBudget       = 100_000_000
	gptAutoLiveMaxAllowListEntries  = 1000
	gptAutoLiveMaxClassifierTimeout = 30000
)

// DefaultGPTAutoLiveConfig is disabled and internal-only.
func DefaultGPTAutoLiveConfig() GPTAutoLiveConfig {
	return GPTAutoLiveConfig{
		Enabled:                   false,
		InternalOnly:              true,
		Endpoints:                 GPTAutoSupportedEndpoints(),
		ClassifierTimeoutMS:       gptAutoLiveDefaultTimeoutMS,
		ClassifierMaxConcurrency:  gptAutoLiveDefaultConcurrency,
		ClassifierRetryCount:      0,
		ClassifierFailurePolicy:   GPTAutoFailureCompatibleFallback,
		FallbackModel:             GPTAutoModelGPT6Luna,
		FallbackEffort:            GPTAutoEffortMedium,
		ClientEffortPolicy:        GPTAutoClientEffortJev,
		SessionAffinityEnabled:    true,
		SessionAffinityTTLSeconds: gptAutoLiveDefaultAffinityTTL,
		MarginGuardEnabled:        false,
		ApplyBillingMultiplier:    true,
		DailyRequestBudget:        gptAutoLiveDefaultDailyBudget,
		CustomerExportEnabled:     true,
	}
}

func normalizeGPTAutoLiveConfig(cfg *GPTAutoLiveConfig) {
	if cfg == nil {
		return
	}
	cfg.ClassifierTimeoutMS = clampIntDefault(cfg.ClassifierTimeoutMS, gptAutoMinTimeoutMS, gptAutoLiveMaxClassifierTimeout, gptAutoLiveDefaultTimeoutMS)
	cfg.ClassifierMaxConcurrency = clampIntDefault(cfg.ClassifierMaxConcurrency, 1, gptAutoLiveMaxConcurrency, gptAutoLiveDefaultConcurrency)
	cfg.ClassifierRetryCount = clampInt(cfg.ClassifierRetryCount, 0, gptAutoLiveMaxRetryCount)
	if cfg.ClassifierFailurePolicy != GPTAutoFailureRejectRequest {
		cfg.ClassifierFailurePolicy = GPTAutoFailureCompatibleFallback
	}
	cfg.FallbackModel = strings.ToLower(strings.TrimSpace(cfg.FallbackModel))
	if !IsGPTAutoModel(cfg.FallbackModel) {
		cfg.FallbackModel = GPTAutoModelGPT6Luna
	}
	cfg.FallbackEffort = strings.ToLower(strings.TrimSpace(cfg.FallbackEffort))
	if !IsGPTAutoEffort(cfg.FallbackEffort) {
		cfg.FallbackEffort = GPTAutoEffortMedium
	}
	if cfg.ClientEffortPolicy != GPTAutoClientEffortHonor {
		cfg.ClientEffortPolicy = GPTAutoClientEffortJev
	}
	cfg.SessionAffinityTTLSeconds = clampIntDefault(cfg.SessionAffinityTTLSeconds, gptAutoLiveMinAffinityTTL, gptAutoLiveMaxAffinityTTL, gptAutoLiveDefaultAffinityTTL)
	cfg.DailyRequestBudget = clampInt(cfg.DailyRequestBudget, 0, gptAutoLiveMaxDailyBudget)
	cfg.Endpoints = normalizeGPTAutoEndpoints(cfg.Endpoints)
	cfg.AllowedGroupIDs = normalizeGPTAutoIDs(cfg.AllowedGroupIDs)
	cfg.AllowedAPIKeyIDs = normalizeGPTAutoIDs(cfg.AllowedAPIKeyIDs)
}

func normalizeGPTAutoIDs(in []int64) []int64 {
	if len(in) == 0 {
		return nil
	}
	seen := make(map[int64]struct{}, len(in))
	out := make([]int64, 0, len(in))
	for _, id := range in {
		if id <= 0 || len(out) >= gptAutoLiveMaxAllowListEntries {
			continue
		}
		if _, dup := seen[id]; dup {
			continue
		}
		seen[id] = struct{}{}
		out = append(out, id)
	}
	return out
}

// ValidateGPTAutoLiveConfig rejects invalid administrator input.
func ValidateGPTAutoLiveConfig(cfg GPTAutoLiveConfig) error {
	bad := func(msg string) error { return infraerrors.BadRequest("GPT_AUTO_CONFIG_INVALID", msg) }
	if cfg.ClassifierTimeoutMS < gptAutoMinTimeoutMS || cfg.ClassifierTimeoutMS > gptAutoLiveMaxClassifierTimeout {
		return bad("live.classifier_timeout_ms must be between 200 and 30000")
	}
	if cfg.ClassifierMaxConcurrency < 1 || cfg.ClassifierMaxConcurrency > gptAutoLiveMaxConcurrency {
		return bad("live.classifier_max_concurrency must be between 1 and 256")
	}
	if cfg.ClassifierRetryCount < 0 || cfg.ClassifierRetryCount > gptAutoLiveMaxRetryCount {
		return bad("live.classifier_retry_count must be 0 or 1")
	}
	switch cfg.ClassifierFailurePolicy {
	case GPTAutoFailureCompatibleFallback, GPTAutoFailureRejectRequest:
	default:
		return bad("live.classifier_failure_policy must be compatible_fallback or reject_request")
	}
	if !IsGPTAutoModel(cfg.FallbackModel) {
		return bad("live.fallback_model must be a GPT Auto pool model")
	}
	if !IsGPTAutoEffort(cfg.FallbackEffort) {
		return bad("live.fallback_effort is not a known reasoning effort")
	}
	switch cfg.ClientEffortPolicy {
	case GPTAutoClientEffortJev, GPTAutoClientEffortHonor:
	default:
		return bad("live.client_effort_policy must be jev or honor_client")
	}
	if cfg.SessionAffinityTTLSeconds < gptAutoLiveMinAffinityTTL || cfg.SessionAffinityTTLSeconds > gptAutoLiveMaxAffinityTTL {
		return bad("live.session_affinity_ttl_seconds must be between 60 and 604800")
	}
	if cfg.DailyRequestBudget < 0 || cfg.DailyRequestBudget > gptAutoLiveMaxDailyBudget {
		return bad("live.live_request_budget must not be negative")
	}
	if len(cfg.AllowedGroupIDs) > gptAutoLiveMaxAllowListEntries || len(cfg.AllowedAPIKeyIDs) > gptAutoLiveMaxAllowListEntries {
		return bad("live allow lists are limited to 1000 entries")
	}
	for _, id := range append(append([]int64(nil), cfg.AllowedGroupIDs...), cfg.AllowedAPIKeyIDs...) {
		if id <= 0 {
			return bad("live allow list ids must be positive")
		}
	}
	for _, endpoint := range cfg.Endpoints {
		if !isGPTAutoSupportedEndpoint(endpoint) {
			return bad("unsupported live endpoint: " + endpoint)
		}
	}
	return nil
}

// AdmitsAPIKey reports whether live routing is on and the key may use the
// virtual model. It never consults client-supplied identifiers: apiKeyID and
// groupID come from the authenticated key.
func (c GPTAutoLiveConfig) AdmitsAPIKey(apiKeyID int64, groupID *int64) bool {
	if !c.Enabled || apiKeyID <= 0 {
		return false
	}
	if groupID == nil {
		return false
	}
	if c.InternalOnly {
		return containsInt64(c.AllowedAPIKeyIDs, apiKeyID)
	}
	return len(c.AllowedGroupIDs) == 0 || containsInt64(c.AllowedGroupIDs, *groupID)
}

// IncludesEndpoint reports whether gpt-auto is accepted on an endpoint.
func (c GPTAutoLiveConfig) IncludesEndpoint(endpoint string) bool {
	return containsString(c.Endpoints, endpoint)
}

// retireGPTAutoModelProfiles moves stored profiles of retired models into the
// retired list. The newest stored profile wins; nothing is reset.
func retireGPTAutoModelProfiles(active, retired []GPTAutoModelProfile) []GPTAutoModelProfile {
	byModel := make(map[string]GPTAutoModelProfile, len(retired))
	order := make([]string, 0, len(retired))
	add := func(profile GPTAutoModelProfile) {
		model := strings.ToLower(strings.TrimSpace(profile.Model))
		if !isGPTAutoRetiredModel(model) {
			return
		}
		profile.Model = model
		if _, ok := byModel[model]; !ok {
			order = append(order, model)
		}
		byModel[model] = profile
	}
	for _, profile := range retired {
		add(profile)
	}
	for _, profile := range active {
		add(profile)
	}
	if len(order) == 0 {
		return nil
	}
	out := make([]GPTAutoModelProfile, 0, len(order))
	for _, model := range order {
		out = append(out, byModel[model])
	}
	return out
}
