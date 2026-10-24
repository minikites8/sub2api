package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"log/slog"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
	"unicode/utf8"

	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
)

// GPT-Auto Phase 2 live routing.
//
// A customer request for GPTAutoVirtualModel is resolved here, synchronously
// and before account scheduling, to one pool model and reasoning effort. The
// handler then rewrites the request body and continues through the ordinary
// gateway pipeline: the same channel mapping, scheduler, failover, usage
// recording and billing as a request that named the model directly. Nothing
// here schedules accounts or computes charges on its own.
//
// Hot-path rules: no database access (config is cached, output history is
// cache-only, the decision row is persisted by a background writer), every
// external call is bounded by a deadline derived from the request context,
// and concurrency is limited by a fixed-size semaphore with no queueing
// goroutines.

// Live counters (this node; flushed to the cluster-wide per-day counters).
const (
	gptAutoCounterLiveRouted       = "live_routed"
	gptAutoCounterLiveClassified   = "live_classified"
	gptAutoCounterLiveAffinityHit  = "live_affinity_hit"
	gptAutoCounterLiveFallback     = "live_fallback"
	gptAutoCounterLiveRejected     = "live_rejected"
	gptAutoCounterLiveRerouted     = "live_rerouted"
	gptAutoCounterLivePersistDrop  = "live_persist_dropped"
	gptAutoCounterLiveDenied       = "live_not_admitted"
	gptAutoCounterLiveClassifyFail = "live_classifier_failed"
)

func gptAutoLiveCounterNames() []string {
	return []string{
		gptAutoCounterLiveRouted, gptAutoCounterLiveClassified, gptAutoCounterLiveAffinityHit,
		gptAutoCounterLiveFallback, gptAutoCounterLiveRejected, gptAutoCounterLiveRerouted,
		gptAutoCounterLivePersistDrop, gptAutoCounterLiveDenied, gptAutoCounterLiveClassifyFail,
	}
}

// Live failure categories (fallback_reason / error_category). Classifier
// classes (timeout, auth, rate_limited, ...) are used verbatim.
const (
	GPTAutoFailureCircuitOpen      = "circuit_open"
	GPTAutoFailureConcurrency      = "classifier_concurrency_exhausted"
	GPTAutoFailureLiveBudget       = "live_budget_exhausted"
	GPTAutoFailureNoContent        = "nothing_to_classify"
	GPTAutoFailureNoCompatible     = "no_compatible_model"
	GPTAutoFailureAffinityModel    = "affinity_model_unavailable"
	GPTAutoFailureClientCancelled  = "client_cancelled"
	GPTAutoFailureNoUpstream       = "no_upstream_available"
	GPTAutoFailureUpstream         = "upstream_error"
	GPTAutoFailureClientError      = "client_error"
	GPTAutoFailureRateLimited      = "rate_limited"
	GPTAutoFailureGatewayRejected  = "gateway_rejected"
	GPTAutoFailurePolicyRejectMode = "classifier_failed_reject_policy"
	GPTAutoFailureUnpricedFeature  = "unpriced_feature"
)

const (
	gptAutoLivePersistQueue  = 4096
	gptAutoLiveAffinityOpTTL = 300 * time.Millisecond
	gptAutoLiveBudgetDayKey  = "live:"
)

// GPTAutoRouteError is a structured, OpenAI-compatible routing failure. It is
// returned instead of a bare gateway error so a classifier problem never
// surfaces as an unexplained 502.
type GPTAutoRouteError struct {
	Status  int
	Type    string
	Code    string
	Message string
}

func (e *GPTAutoRouteError) Error() string { return e.Code + ": " + e.Message }

var (
	// errGPTAutoNotAvailable deliberately matches the gateway's unknown-model
	// wording: a customer without access learns nothing about the feature.
	errGPTAutoNotAvailable = &GPTAutoRouteError{Status: http.StatusNotFound, Type: "invalid_request_error", Code: "model_not_found",
		Message: "The model `" + GPTAutoVirtualModel + "` does not exist or you do not have access to it."}
	errGPTAutoEndpointUnsupported = &GPTAutoRouteError{Status: http.StatusBadRequest, Type: "invalid_request_error", Code: "gpt_auto_endpoint_unsupported",
		Message: "The model `" + GPTAutoVirtualModel + "` is not available on this endpoint."}
)

func gptAutoNoCompatibleError(detail string) *GPTAutoRouteError {
	message := "No model available to " + GPTAutoVirtualModel + " supports this request"
	if detail != "" {
		message += " (" + detail + ")"
	}
	return &GPTAutoRouteError{Status: http.StatusBadRequest, Type: "invalid_request_error", Code: "gpt_auto_no_compatible_model", Message: message + "."}
}

func gptAutoClassifierUnavailableError(reason string) *GPTAutoRouteError {
	return &GPTAutoRouteError{Status: http.StatusServiceUnavailable, Type: "api_error", Code: "gpt_auto_routing_unavailable",
		Message: "Automatic model routing is temporarily unavailable (" + reason + "). Retry, or request a specific model."}
}

// GPTAutoAffinity is the minimal per-session routing memory. No content.
type GPTAutoAffinity struct {
	Sequence        int64                `json:"sequence,omitempty"`
	TaskFingerprint string               `json:"task_fingerprint,omitempty"`
	DowngradeModel  string               `json:"downgrade_model,omitempty"`
	DowngradeCount  int                  `json:"downgrade_count,omitempty"`
	History         []GPTAutoSessionTurn `json:"history,omitempty"`
	Model           string               `json:"model"`
	Effort          string               `json:"effort"`
	AccountID       int64                `json:"account_id,omitempty"`
}

// GPTAutoAffinityStore keeps session affinity with a bounded TTL. Keys are
// opaque hashes already scoped to one API key (see gptAutoAffinityKey). The
// production implementation is Redis-backed.
type GPTAutoAffinityStore interface {
	// Get returns nil, nil on a miss.
	Get(ctx context.Context, key string) (*GPTAutoAffinity, error)
	Set(ctx context.Context, key string, value GPTAutoAffinity, ttl time.Duration) error
}

// gptAutoAffinityKey hashes a session reference together with the
// authenticated API key id, so two customers that happen to send the same
// response or session id can never share an entry.
func gptAutoAffinityKey(apiKeyID int64, kind, reference string) string {
	sum := sha256.Sum256([]byte(strconv.FormatInt(apiKeyID, 10) + "\x00" + kind + "\x00" + reference))
	return hex.EncodeToString(sum[:])
}

// localGPTAutoAffinityStore is the single-process fallback (tests, no Redis).
type localGPTAutoAffinityStore struct {
	mu      sync.Mutex
	entries map[string]localGPTAutoAffinityEntry
	now     func() time.Time
}

type localGPTAutoAffinityEntry struct {
	value     GPTAutoAffinity
	expiresAt time.Time
}

const gptAutoLocalAffinityMaxEntries = 10000

func newLocalGPTAutoAffinityStore() *localGPTAutoAffinityStore {
	return &localGPTAutoAffinityStore{entries: map[string]localGPTAutoAffinityEntry{}, now: time.Now}
}

func (s *localGPTAutoAffinityStore) Get(_ context.Context, key string) (*GPTAutoAffinity, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	entry, ok := s.entries[key]
	if !ok || !s.now().Before(entry.expiresAt) {
		delete(s.entries, key)
		return nil, nil
	}
	value := entry.value
	return &value, nil
}

func (s *localGPTAutoAffinityStore) Set(_ context.Context, key string, value GPTAutoAffinity, ttl time.Duration) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := s.now()
	if len(s.entries) >= gptAutoLocalAffinityMaxEntries {
		for k, entry := range s.entries {
			if !now.Before(entry.expiresAt) || len(s.entries) >= gptAutoLocalAffinityMaxEntries {
				delete(s.entries, k)
			}
			if len(s.entries) < gptAutoLocalAffinityMaxEntries/2 {
				break
			}
		}
	}
	s.entries[key] = localGPTAutoAffinityEntry{value: value, expiresAt: now.Add(ttl)}
	return nil
}

// gptAutoLiveState is the live router's per-node state, embedded in the
// shadow service so both share one classifier, budget, pricing and repo.
type gptAutoLiveState struct {
	semaphore chan struct{}
	active    atomic.Int64
	breaker   gptAutoBreaker
	affinity  GPTAutoAffinityStore
	persistCh chan gptAutoLivePersistTask
}

type gptAutoLivePersistTask struct {
	cfg GPTAutoShadowConfig
	row *GPTAutoShadowDecision
}

func (s *GPTAutoShadowService) initLiveState() {
	s.live.semaphore = make(chan struct{}, gptAutoLiveMaxConcurrency)
	s.live.affinity = newLocalGPTAutoAffinityStore()
	s.live.persistCh = make(chan gptAutoLivePersistTask, gptAutoLivePersistQueue)
}

// SetAffinityStore replaces the session-affinity store (Redis in production).
func (s *GPTAutoShadowService) SetAffinityStore(store GPTAutoAffinityStore) {
	if s != nil && store != nil {
		s.live.affinity = store
	}
}

func (s *GPTAutoShadowService) livePersistLoop() {
	defer s.wg.Done()
	for {
		select {
		case <-s.stopCh:
			// Drain what is already queued so completed requests are logged.
			for {
				select {
				case task := <-s.live.persistCh:
					s.persist(context.Background(), task.cfg, task.row)
				default:
					return
				}
			}
		case task := <-s.live.persistCh:
			s.persist(context.Background(), task.cfg, task.row)
		}
	}
}

// GPTAutoRouteInput is what a gateway handler supplies for a gpt-auto request.
// Every identity field comes from the authenticated API key.
type GPTAutoRouteInput struct {
	Endpoint  string
	Body      []byte
	RequestID string
	UserID    int64
	APIKeyID  int64
	GroupID   *int64
	// Group is the authenticated key's group as resolved by the gateway's
	// own group selection (never a client-supplied id). Its gpt_auto_enabled
	// switch is mandatory, and its model pricing may carry an explicit
	// gpt-auto price override.
	Group        *Group
	CustomerRate func(*Group, time.Time) float64
	// SessionKey is an explicit client session reference (session headers or
	// prompt_cache_key). It is hashed with APIKeyID before use.
	SessionKey string
	// ModelPermitted applies the group allowlist and per-user model denials
	// to an underlying pool model, so gpt-auto never bypasses them.
	ModelPermitted func(model string) bool
}

// GPTAutoRoute is one live routing decision. The handler forwards Body and
// reports the outcome through the Observe*/Finish methods.
type GPTAutoRoute struct {
	svc  *GPTAutoShadowService
	cfg  GPTAutoShadowConfig
	row  *GPTAutoShadowDecision
	keys []string

	previous        *GPTAutoAffinity
	hardState       bool
	requiredAccount int64
	taskFingerprint string
	downgradeModel  string
	downgradeCount  int
	started         time.Time

	mu                sync.Mutex
	model             string
	effort            string
	body              []byte
	billingMultiplier float64
	tariff            GPTAutoTariff
	customerRate      *float64
	freeFast          bool
	remaining         []string
	original          []byte
	endpoint          string
	clientEffort      string
	reroutes          int
	finished          bool
	responseID        string
	servedModel       string
	firstTokenMS      *int
	resultObserved    bool
}

// Model and Effort are the selected pool model and reasoning effort.
func (r *GPTAutoRoute) Model() string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.model
}

func (r *GPTAutoRoute) Effort() string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.effort
}

// Body is the request body rewritten to the selected model and effort.
func (r *GPTAutoRoute) Body() []byte {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.body
}

// BillingMultiplier is the executed model's profile multiplier applied on top
// of the ordinary customer price in legacy pricing mode (1 when disabled by
// configuration, and always 1 in fixed-rate mode).
func (r *GPTAutoRoute) BillingMultiplier() float64 {
	if r == nil {
		return 1
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.billingMultiplier <= 0 {
		return 1
	}
	return r.billingMultiplier
}

// FixedTariff returns the immutable tariff snapshot when the request is
// priced as the gpt-auto virtual model (virtual_model_fixed_rates).
func (r *GPTAutoRoute) FixedTariff() (GPTAutoTariff, bool) {
	if r == nil {
		return GPTAutoTariff{}, false
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.tariff, r.tariff.Mode == GPTAutoPricingModeFixed
}

// RequestID is the usage-log request id this decision is correlated with.
func (r *GPTAutoRoute) RequestID() string {
	if r == nil || r.row == nil {
		return ""
	}
	return r.row.RequestID
}

type gptAutoRouteCtxKey struct{}

// WithGPTAutoRoute attaches a live route to the request context so usage
// recording bills the executed model and records the virtual model.
func WithGPTAutoRoute(ctx context.Context, route *GPTAutoRoute) context.Context {
	if ctx == nil || route == nil {
		return ctx
	}
	return context.WithValue(ctx, gptAutoRouteCtxKey{}, route)
}

// GPTAutoRouteFromContext returns the live route, if any.
func GPTAutoRouteFromContext(ctx context.Context) *GPTAutoRoute {
	if ctx == nil {
		return nil
	}
	route, _ := ctx.Value(gptAutoRouteCtxKey{}).(*GPTAutoRoute)
	return route
}

// LiveAdmits reports whether the authenticated key may currently request
// gpt-auto. /v1/models uses it, so the listing shows exactly what Route
// would accept. Never performs I/O beyond the cached config read.
func (s *GPTAutoShadowService) LiveAdmits(ctx context.Context, input GPTAutoRouteInput) bool {
	if s == nil || s.settings == nil {
		return false
	}
	input = s.refreshAdmissionGroup(ctx, input)
	cfg := s.settings.GetGPTAutoShadowConfigCached(ctx)
	if !gptAutoAdmits(cfg, input) {
		return false
	}
	for _, model := range GPTAutoModelPool() {
		profile, ok := cfg.ProfileFor(model)
		if ok && profile.Enabled && (input.ModelPermitted == nil || input.ModelPermitted(model)) {
			return true
		}
	}
	return false
}

// gptAutoAdmits is the complete gpt-auto authorization rule. Every condition
// is mandatory:
//
//	global live_enabled
//	AND the group's own gpt_auto_enabled switch (OpenAI group)
//	AND the group model allowlist / user model denials permit gpt-auto
//	AND the rollout restrictions (internal-only key list, allowed groups)
//	AND a priceable tariff (legacy mode, or a complete fixed-rate tariff)
//
// At least one eligible underlying model is checked afterwards by Route.
func gptAutoAdmits(cfg GPTAutoShadowConfig, input GPTAutoRouteInput) bool {
	group := input.Group
	if group == nil || !group.GPTAutoEnabled || group.Platform != PlatformOpenAI {
		return false
	}
	if input.GroupID == nil || *input.GroupID != group.ID {
		return false
	}
	if !cfg.Live.AdmitsAPIKey(input.APIKeyID, input.GroupID) {
		return false
	}
	if group.ModelAllowlistEnabled() && !group.ModelAllowlist.Allows(GPTAutoVirtualModel) {
		return false
	}
	if input.ModelPermitted != nil && !input.ModelPermitted(GPTAutoVirtualModel) {
		return false
	}
	_, priced := cfg.Pricing.TariffFor(group)
	return priced
}

// gptAutoUnpricedFeature names a requested capability whose usage the
// fixed token tariff cannot price (per-image generation, audio). Such a
// request is rejected before execution rather than billed at zero.
// refreshAdmissionGroup resolves only the already authenticated group ID. A
// stale auth snapshot or affinity entry cannot undo a persisted revocation.
func (s *GPTAutoShadowService) refreshAdmissionGroup(ctx context.Context, input GPTAutoRouteInput) GPTAutoRouteInput {
	if s.admissionGroups == nil || input.GroupID == nil {
		return input
	}
	group, err := s.admissionGroups.GetByID(ctx, *input.GroupID)
	if err != nil || group == nil || !group.IsActive() {
		input.Group = nil
		return input
	}
	input.Group = group
	previous := input.ModelPermitted
	input.ModelPermitted = func(model string) bool {
		return (!group.ModelAllowlistEnabled() || group.ModelAllowlist.Allows(model)) && (previous == nil || previous(model))
	}
	return input
}

func gptAutoUnpricedFeature(body []byte) string {
	var walk func(gjson.Result) bool
	walk = func(value gjson.Result) bool {
		kind := value.Get("type").String()
		if kind == "input_audio" || kind == "audio" || kind == "video" || kind == "input_video" {
			return true
		}
		found := false
		if value.IsArray() || value.IsObject() {
			value.ForEach(func(_, child gjson.Result) bool {
				if walk(child) {
					found = true
					return false
				}
				return true
			})
		}
		return found
	}
	if walk(gjson.ParseBytes(body)) || gjson.GetBytes(body, "audio").Exists() {
		return "audio/video"
	}
	for _, modality := range gjson.GetBytes(body, "modalities").Array() {
		if !strings.EqualFold(modality.String(), "text") {
			return "non-text output"
		}
	}
	return ""
}

// gptAutoRequirements are the request properties live routing must honour.
type gptAutoRequirements struct {
	hasTools          bool
	hasImages         bool
	structuredOutput  bool
	stateful          bool
	previousResponse  string
	estimatedTokens   int
	clientEffort      string
	clientEffortValid bool
}

func gptAutoRequestRequirements(endpoint string, body []byte, snapshot GPTAutoRequestSnapshot) gptAutoRequirements {
	req := gptAutoRequirements{
		hasTools:        snapshot.HasTools,
		hasImages:       snapshot.HasImages,
		estimatedTokens: snapshot.EstimatedInputTokens,
		clientEffort:    snapshot.RequestedEffort,
	}
	req.clientEffortValid = IsGPTAutoEffort(req.clientEffort)
	switch endpoint {
	case GPTAutoEndpointResponses:
		req.previousResponse = strings.TrimSpace(gjson.GetBytes(body, "previous_response_id").String())
		req.stateful = req.previousResponse != "" || gjson.GetBytes(body, "conversation").Exists()
		switch gjson.GetBytes(body, "text.format.type").String() {
		case "json_schema", "json_object":
			req.structuredOutput = true
		}
	default:
		switch gjson.GetBytes(body, "response_format.type").String() {
		case "json_schema", "json_object":
			req.structuredOutput = true
		}
	}
	return req
}

// gptAutoLiveExclusions returns the hard constraints per pool model.
func gptAutoLiveExclusions(cfg GPTAutoShadowConfig, req gptAutoRequirements, permitted func(string) bool) map[string][]string {
	out := make(map[string][]string, len(GPTAutoModelPool()))
	for _, model := range GPTAutoModelPool() {
		profile, _ := cfg.ProfileFor(model)
		caps := profile.Capabilities
		if caps == nil {
			caps = DefaultGPTAutoModelCapabilities()
		}
		var reasons []string
		if req.hasImages && !caps.Vision {
			reasons = append(reasons, GPTAutoRuleModelVision)
		}
		if req.structuredOutput && !caps.StructuredOutputs {
			reasons = append(reasons, GPTAutoRuleModelStructured)
		}
		if req.stateful && !caps.StatefulResponses {
			reasons = append(reasons, GPTAutoRuleModelStateful)
		}
		if caps.MaxContextTokens > 0 && req.estimatedTokens > caps.MaxContextTokens {
			reasons = append(reasons, GPTAutoRuleModelContext)
		}
		if permitted != nil && !permitted(model) {
			reasons = append(reasons, GPTAutoRuleModelNotPermitted)
		}
		if len(reasons) > 0 {
			out[model] = reasons
		}
	}
	return out
}

// gptAutoStaticEligible lists models that pass every configured constraint,
// ordered by routing weight (highest first). Used for fallback and rerouting.
func gptAutoStaticEligible(cfg GPTAutoShadowConfig, features GPTAutoRequestFeatures, exclusions map[string][]string) []string {
	candidates := gptAutoCandidates(cfg, GPTAutoClassification{}, features, exclusions)
	sort.SliceStable(candidates, func(i, j int) bool { return candidates[i].RoutingWeight > candidates[j].RoutingWeight })
	out := make([]string, 0, len(candidates))
	for _, candidate := range candidates {
		if candidate.Eligible {
			out = append(out, candidate.Model)
		}
	}
	return out
}

// Route resolves a gpt-auto request. A nil route with a non-nil error means
// the request must be answered with that error; the error is always a
// *GPTAutoRouteError.
func (s *GPTAutoShadowService) Route(ctx context.Context, input GPTAutoRouteInput) (*GPTAutoRoute, error) {
	if s == nil || s.settings == nil {
		return nil, errGPTAutoNotAvailable
	}
	started := s.now()
	input = s.refreshAdmissionGroup(ctx, input)
	cfg := s.settings.GetGPTAutoShadowConfigCached(ctx)
	live := cfg.Live
	if !gptAutoAdmits(cfg, input) {
		s.incr(gptAutoCounterLiveDenied)
		return nil, errGPTAutoNotAvailable
	}
	// The tariff is captured here, before execution, and used unchanged at
	// settlement even if an administrator changes pricing mid-stream.
	tariff, _ := cfg.Pricing.TariffFor(input.Group)
	if !live.IncludesEndpoint(input.Endpoint) {
		return nil, errGPTAutoEndpointUnsupported
	}
	s.incr(gptAutoCounterLiveRouted)

	snapshot := BuildGPTAutoSnapshot(input.Endpoint, input.Body, cfg.ContextOptions())
	snapshot.RequestID = input.RequestID
	if snapshot.RequestID == "" {
		snapshot.RequestID = GPTAutoUsageRequestID(ctx)
	}
	snapshot.UserID, snapshot.APIKeyID, snapshot.GroupID = input.UserID, input.APIKeyID, cloneInt64Ptr(input.GroupID)
	req := gptAutoRequestRequirements(input.Endpoint, input.Body, snapshot)
	exclusions := gptAutoLiveExclusions(cfg, req, input.ModelPermitted)

	features := GPTAutoRequestFeatures{
		Endpoint:             input.Endpoint,
		HasTools:             snapshot.HasTools,
		EstimatedInputTokens: snapshot.EstimatedInputTokens,
		InputEstimateSource:  GPTAutoEstimateSourceHeuristic,
		Context:              gptAutoContextInfoPtr(snapshot.Context),
	}
	classifierHash, _, _ := NewGPTAutoClassifierSnapshot(cfg).Hash()
	row := &GPTAutoShadowDecision{
		Mode:                 GPTAutoModeLive,
		CreatedAt:            started.UTC(),
		RequestID:            snapshot.RequestID,
		UserID:               nonZeroInt64(input.UserID),
		APIKeyID:             nonZeroInt64(input.APIKeyID),
		GroupID:              cloneInt64Ptr(input.GroupID),
		Endpoint:             input.Endpoint,
		ActualModel:          GPTAutoVirtualModel,
		ActualEffort:         snapshot.RequestedEffort,
		Stream:               snapshot.Stream,
		HasTools:             snapshot.HasTools,
		ClassifierModel:      cfg.ClassifierModel,
		ClassifierConfigHash: classifierHash,
		PolicyVersion:        cfg.PolicyVersion,
		StateVersion:         GPTAutoClassifierStateVersion,
		ClassifierSchema:     GPTAutoClassifierSchemaVersion,
		Features:             features,
	}
	applyGPTAutoTariffToRow(row, tariff)
	route := &GPTAutoRoute{svc: s, cfg: cfg, row: row, started: started, original: input.Body,
		endpoint: input.Endpoint, clientEffort: req.clientEffort, tariff: tariff, freeFast: input.Group.FreeOpenAIFast}
	route.prepareDynamic(snapshot, input, req.stateful)
	if input.CustomerRate != nil {
		rate := input.CustomerRate(input.Group, started)
		route.customerRate = &rate
	}

	if tariff.Mode == GPTAutoPricingModeFixed {
		if feature := gptAutoUnpricedFeature(input.Body); feature != "" {
			return nil, s.rejectRoute(route, GPTAutoFailureUnpricedFeature, &GPTAutoRouteError{
				Status: http.StatusBadRequest, Type: "invalid_request_error", Code: "gpt_auto_unsupported_feature",
				Message: "The model `" + GPTAutoVirtualModel + "` does not support " + feature + "; request a specific model.",
			})
		}
	}

	eligible := gptAutoStaticEligible(cfg, features, exclusions)
	if len(eligible) == 0 {
		return nil, s.rejectRoute(route, GPTAutoFailureNoCompatible, gptAutoNoCompatibleError(gptAutoExclusionSummary(exclusions)))
	}

	if route.hardState && row.Features.Switching.StateDependency == "unknown" {
		return nil, s.rejectRoute(route, GPTAutoFailureAffinityModel, gptAutoStateError())
	}
	// Session affinity: a continuation stays on the model that holds its
	// state. Keys are scoped to the authenticated API key.
	row.AffinityOutcome = GPTAutoAffinityDisabled
	if live.SessionAffinityEnabled || route.dynamicEnabled() {
		row.AffinityOutcome = GPTAutoAffinityNone
		if req.previousResponse != "" {
			route.keys = append(route.keys, gptAutoAffinityKey(input.APIKeyID, "resp", req.previousResponse))
		}
		if session := strings.TrimSpace(input.SessionKey); session != "" && !route.hardState {
			route.keys = append(route.keys, gptAutoAffinityKey(input.APIKeyID, "sess", session))
		}
		if len(route.keys) > 0 {
			affinity, outcome := s.lookupAffinity(ctx, route.keys)
			row.AffinityOutcome = outcome
			route.rememberPrevious(affinity)
			if affinity != nil && (!route.dynamicEnabled() || route.hardState) {
				if route.hardState {
					route.requiredAccount = affinity.AccountID
					if affinity.AccountID <= 0 {
						return nil, s.rejectRoute(route, GPTAutoFailureAffinityModel, gptAutoStateError())
					}
				}
				if containsString(eligible, affinity.Model) {
					profile, _ := cfg.ProfileFor(affinity.Model)
					effort, _ := gptAutoPickEffort(profile.SupportedEfforts, affinity.Effort)
					if effort == "" {
						effort = affinity.Effort
					}
					s.incr(gptAutoCounterLiveAffinityHit)
					row.RoutingReason = GPTAutoRouteAffinity
					row.EvaluationStatus = GPTAutoEvaluationSkipped
					row.DecisionStatus = GPTAutoDecisionAccepted
					s.selectRoute(route, affinity.Model, effort, eligible)
					return route, nil
				}
				row.AffinityOutcome = GPTAutoAffinityBroken
				if req.stateful {
					// The upstream holds conversation state for a model this
					// request may no longer use; switching would silently lose
					// or corrupt it.
					return nil, s.rejectRoute(route, GPTAutoFailureAffinityModel, &GPTAutoRouteError{
						Status: http.StatusConflict, Type: "invalid_request_error", Code: "gpt_auto_session_model_unavailable",
						Message: "This conversation continues on a model that is no longer available to " + GPTAutoVirtualModel + "; start a new conversation.",
					})
				}
			}
		}
	}

	if route.hardState {
		return nil, s.rejectRoute(route, GPTAutoFailureAffinityModel, gptAutoStateError())
	}
	classification, failure := s.classifyLiveRequest(ctx, cfg, snapshot, row)
	if failure != "" {
		s.incr(gptAutoCounterLiveClassifyFail)
		if errors.Is(ctx.Err(), context.Canceled) {
			return nil, s.rejectRoute(route, GPTAutoFailureClientCancelled, &GPTAutoRouteError{
				Status: 499, Type: "api_error", Code: "client_cancelled", Message: "client closed the request"})
		}
		return s.fallbackRoute(route, failure, eligible, req)
	}
	row.EvaluationStatus = GPTAutoEvaluationSuccess
	row.Classification = classification
	s.incr(gptAutoCounterLiveClassified)

	cls := *classification
	if live.ClientEffortPolicy == GPTAutoClientEffortHonor && req.clientEffortValid {
		cls.Effort = req.clientEffort
	}
	policyInput := GPTAutoPolicyInput{
		Classification:    cls,
		Features:          features,
		Config:            cfg,
		Pricing:           s.pricing,
		OutputEstimate:    s.cachedOutputEstimator(),
		ClassifierCostUSD: row.ClassifierCost,
		Exclusions:        exclusions,
		Tariff:            &route.tariff,
	}
	if route.customerRate != nil {
		policyInput.CustomerRateMultiplier = *route.customerRate
		policyInput.CustomerRateSource = "request_snapshot"
	}
	decision := EvaluateGPTAutoPolicy(policyInput)
	if live.MarginGuardEnabled {
		decision = s.applyMarginGuard(policyInput, decision)
	}
	if cls.Effort != classification.Effort {
		decision.JevEffort = classification.Effort
		decision.Adjustments = append(decision.Adjustments, GPTAutoAdjustment{
			Rule: GPTAutoRuleClientEffort, From: classification.Effort, To: cls.Effort,
			Detail: "client_effort_policy honor_client: the request's explicit reasoning effort replaces Jev's",
		})
		if decision.Status == GPTAutoDecisionAccepted {
			decision.Status = GPTAutoDecisionAdjusted
		}
	}
	route.switchPolicy(&decision)
	if route.dynamicEnabled() && decision.FinalModel != "" {
		economics := EstimateGPTAutoEconomics(policyInput, decision.FinalModel, decision.FinalEffort)
		decision.Economics = &economics
	}
	s.applyDecision(row, classification, &decision)
	if decision.FinalModel == "" {
		return nil, s.rejectRoute(route, GPTAutoFailureNoCompatible, gptAutoNoCompatibleError(gptAutoExclusionSummary(exclusions)))
	}
	row.RoutingReason = GPTAutoRouteClassified
	order := gptAutoRerouteOrder(decision.Candidates, decision.FinalModel)
	s.selectRoute(route, decision.FinalModel, decision.FinalEffort, order)
	return route, nil
}

// gptAutoRerouteOrder lists the remaining eligible candidates by Jev
// probability, then routing weight, after the selected model.
func gptAutoRerouteOrder(candidates []GPTAutoCandidate, selected string) []string {
	eligible := make([]GPTAutoCandidate, 0, len(candidates))
	for _, candidate := range candidates {
		if candidate.Eligible {
			eligible = append(eligible, candidate)
		}
	}
	sort.SliceStable(eligible, func(i, j int) bool {
		if eligible[i].Probability != eligible[j].Probability {
			return eligible[i].Probability > eligible[j].Probability
		}
		return eligible[i].RoutingWeight > eligible[j].RoutingWeight
	})
	out := []string{selected}
	for _, candidate := range eligible {
		if candidate.Model != selected {
			out = append(out, candidate.Model)
		}
	}
	return out
}

func gptAutoExclusionSummary(exclusions map[string][]string) string {
	reasons := map[string]struct{}{}
	for _, list := range exclusions {
		for _, reason := range list {
			reasons[reason] = struct{}{}
		}
	}
	out := make([]string, 0, len(reasons))
	for reason := range reasons {
		out = append(out, reason)
	}
	sort.Strings(out)
	return strings.Join(out, ", ")
}

// applyMarginGuard excludes the selected model when its estimated model
// margin is a loss on sufficient data and another eligible model remains.
func (s *GPTAutoShadowService) applyMarginGuard(in GPTAutoPolicyInput, decision GPTAutoPolicyDecision) GPTAutoPolicyDecision {
	for attempts := 0; attempts < len(GPTAutoModelPool()); attempts++ {
		economics := decision.Economics
		if decision.FinalModel == "" || economics == nil || !economics.PricingAvailable ||
			economics.EstimateQuality == GPTAutoEstimateQualityInsufficient || economics.ModelMarginStatus != GPTAutoMarginLoss {
			return decision
		}
		exclusions := make(map[string][]string, len(in.Exclusions)+1)
		for model, reasons := range in.Exclusions {
			exclusions[model] = reasons
		}
		exclusions[decision.FinalModel] = append(append([]string(nil), exclusions[decision.FinalModel]...), GPTAutoRuleMarginGuard)
		next := in
		next.Exclusions = exclusions
		guarded := EvaluateGPTAutoPolicy(next)
		if guarded.FinalModel == "" {
			decision.Warnings = append(decision.Warnings, GPTAutoAdjustment{Rule: GPTAutoRuleMarginGuard, From: decision.FinalModel,
				Detail: "estimated model margin is a loss but no other eligible model remains; kept"})
			return decision
		}
		guarded.Adjustments = append(guarded.Adjustments, GPTAutoAdjustment{Rule: GPTAutoRuleMarginGuard, From: decision.FinalModel, To: guarded.FinalModel,
			Detail: "margin guard: estimated model margin was a loss"})
		guarded.Status = GPTAutoDecisionAdjusted
		decision, in = guarded, next
	}
	return decision
}

// classifyLiveRequest runs Jev synchronously within the live timeout. It
// returns a failure category instead of an error; the caller applies the
// configured failure policy.
func (s *GPTAutoShadowService) classifyLiveRequest(ctx context.Context, cfg GPTAutoShadowConfig, snapshot GPTAutoRequestSnapshot, row *GPTAutoShadowDecision) (*GPTAutoClassification, string) {
	live := cfg.Live
	if !snapshot.HasClassifiableContent() {
		return nil, GPTAutoFailureNoContent
	}
	if strings.TrimSpace(cfg.ClassifierAPIKey) == "" || s.classifier == nil {
		return nil, GPTAutoErrNotConfigured
	}
	now := s.now()
	if !s.live.breaker.allow(now) {
		return nil, GPTAutoFailureCircuitOpen
	}
	probeHeld := true
	defer func() {
		if probeHeld {
			s.live.breaker.releaseProbe()
		}
	}()
	// The live budget counts Jev calls actually attempted: it is reserved
	// after the circuit check and refunded when no call is made.
	reserved, failure := s.reserveLiveRequest(ctx, live)
	if failure != "" {
		return nil, failure
	}
	called := false
	defer func() {
		if reserved && !called {
			s.refundLiveRequest()
		}
	}()
	timeout := time.Duration(live.ClassifierTimeoutMS) * time.Millisecond
	deadlineCtx, cancel := context.WithTimeout(ctx, timeout*time.Duration(1+live.ClassifierRetryCount))
	defer cancel()
	if !s.acquireLive(deadlineCtx, live.ClassifierMaxConcurrency) {
		if ctx.Err() != nil {
			return nil, GPTAutoFailureClientCancelled
		}
		return nil, GPTAutoFailureConcurrency
	}
	defer s.releaseLive()

	attemptCfg := cfg
	attemptCfg.ClassifierTimeoutMS = live.ClassifierTimeoutMS
	attemptCfg.ClassifierRetryCount = live.ClassifierRetryCount
	classifyStart := s.now()
	result := s.classifyWithRetry(deadlineCtx, attemptCfg, snapshot)
	called = result.attempts > 0
	latency := int(s.now().Sub(classifyStart).Milliseconds())

	row.ClassifierLatencyMS = latency
	row.Attempts = result.attempts
	row.ClassifierInputTokens = result.inputTokens
	row.ClassifierOutputTokens = result.outputTokens
	row.ClassifierCost = result.costUSD
	if result.classifierModel != "" {
		row.ClassifierModel = result.classifierModel
	}
	if result.attempts == 0 {
		// The shared Jev budget denied the call: nothing was sent.
		if errors.Is(result.err, errGPTAutoBudgetUnavailable) {
			return nil, GPTAutoErrBudgetUnavailable
		}
		return nil, GPTAutoErrBudgetExhausted
	}
	if ctx.Err() == nil {
		// A cancelled client says nothing about classifier health.
		s.live.breaker.record(result.err == nil, s.now())
		probeHeld = false
	}
	if result.err != nil {
		class := GPTAutoErrTransport
		var classified *GPTAutoClassifierError
		if errors.As(result.err, &classified) {
			class = classified.Class
		}
		if ctx.Err() != nil {
			class = GPTAutoFailureClientCancelled
		}
		row.EvaluationStatus = GPTAutoEvaluationFailed
		row.ErrorClass = class
		return nil, class
	}
	s.observeLatency(latency)
	return result.classification, ""
}

// reserveLiveRequest books one live request against the cluster-wide daily
// live budget. A configured budget fails closed when the store is down.
func (s *GPTAutoShadowService) reserveLiveRequest(ctx context.Context, live GPTAutoLiveConfig) (bool, string) {
	if live.DailyRequestBudget <= 0 {
		return false, ""
	}
	opCtx, cancel := context.WithTimeout(ctx, gptAutoBudgetOpTimeout)
	defer cancel()
	_, allowed, err := s.budget.Reserve(opCtx, s.liveBudgetDay(), GPTAutoBudgetLimits{Calls: int64(live.DailyRequestBudget)}, 0)
	if err != nil {
		return false, GPTAutoErrBudgetUnavailable
	}
	if !allowed {
		return false, GPTAutoFailureLiveBudget
	}
	return true, ""
}

// refundLiveRequest returns a reservation for which no Jev call was made.
func (s *GPTAutoShadowService) refundLiveRequest() {
	ctx, cancel := context.WithTimeout(context.Background(), gptAutoBudgetOpTimeout)
	defer cancel()
	if err := s.budget.Adjust(ctx, s.liveBudgetDay(), -1, 0); err != nil {
		s.incr(gptAutoCounterBudgetError)
	}
}

func (s *GPTAutoShadowService) liveBudgetDay() string {
	return gptAutoLiveBudgetDayKey + gptAutoDayStart(s.now()).Format(time.DateOnly)
}

// LiveBudgetUsage returns today's live request count (cluster-wide).
func (s *GPTAutoShadowService) LiveBudgetUsage(ctx context.Context) (int64, error) {
	usage, _, err := s.budget.Usage(ctx, s.liveBudgetDay())
	return usage.Calls, err
}

func (s *GPTAutoShadowService) acquireLive(ctx context.Context, limit int) bool {
	for {
		select {
		case s.live.semaphore <- struct{}{}:
			if s.live.active.Add(1) <= int64(limit) {
				return true
			}
			s.live.active.Add(-1)
			<-s.live.semaphore
		case <-ctx.Done():
			return false
		}
		select {
		case <-time.After(10 * time.Millisecond):
		case <-ctx.Done():
			return false
		}
	}
}

func (s *GPTAutoShadowService) releaseLive() {
	s.live.active.Add(-1)
	<-s.live.semaphore
}

func (s *GPTAutoShadowService) lookupAffinity(ctx context.Context, keys []string) (*GPTAutoAffinity, string) {
	if s.live.affinity == nil {
		return nil, GPTAutoAffinityUnavailable
	}
	opCtx, cancel := context.WithTimeout(ctx, gptAutoLiveAffinityOpTTL)
	defer cancel()
	for _, key := range keys {
		value, err := s.live.affinity.Get(opCtx, key)
		if err != nil {
			slog.Warn("gpt_auto.affinity_lookup_failed", "error", err)
			return nil, GPTAutoAffinityUnavailable
		}
		if value != nil && IsGPTAutoModel(value.Model) {
			return value, GPTAutoAffinityHit
		}
	}
	return nil, GPTAutoAffinityMiss
}

// fallbackRoute applies the classifier failure policy.
func (s *GPTAutoShadowService) fallbackRoute(route *GPTAutoRoute, failure string, eligible []string, req gptAutoRequirements) (*GPTAutoRoute, error) {
	row, live := route.row, route.cfg.Live
	row.FallbackReason = failure
	if t := row.Features.Switching; t != nil {
		t.ClassificationTrigger = "classifier_unavailable"
		t.SwitchReason = "classifier_failure_fallback"
	}
	if row.EvaluationStatus == "" {
		row.EvaluationStatus = GPTAutoEvaluationFailed
		row.ErrorClass = failure
	}
	if live.ClassifierFailurePolicy == GPTAutoFailureRejectRequest {
		return nil, s.rejectRoute(route, failure, gptAutoClassifierUnavailableError(failure))
	}
	model := eligible[0]
	if containsString(eligible, live.FallbackModel) {
		model = live.FallbackModel
	}
	effort := live.FallbackEffort
	if live.ClientEffortPolicy == GPTAutoClientEffortHonor && req.clientEffortValid {
		effort = req.clientEffort
	}
	profile, _ := route.cfg.ProfileFor(model)
	if picked, _ := gptAutoPickEffort(profile.SupportedEfforts, effort); picked != "" {
		effort = picked
	}
	s.incr(gptAutoCounterLiveFallback)
	row.RoutingReason = GPTAutoRouteFallback
	row.DecisionStatus = GPTAutoDecisionAdjusted
	order := append([]string{model}, removeString(eligible, model)...)
	s.selectRoute(route, model, effort, order)
	return route, nil
}

func removeString(values []string, target string) []string {
	out := make([]string, 0, len(values))
	for _, value := range values {
		if value != target {
			out = append(out, value)
		}
	}
	return out
}

// rejectRoute records a routing rejection (persisted like any decision) and
// returns the structured error to answer with.
func (s *GPTAutoShadowService) rejectRoute(route *GPTAutoRoute, category string, routeErr *GPTAutoRouteError) error {
	s.incr(gptAutoCounterLiveRejected)
	row := route.row
	if row.EvaluationStatus == "" {
		row.EvaluationStatus = GPTAutoEvaluationSkipped
	}
	row.RequestStatus = GPTAutoRequestRejected
	if routeErr.Status == 499 {
		row.RequestStatus = GPTAutoRequestCancelled
	}
	row.ErrorCategory = category
	status := routeErr.Status
	row.HTTPStatus = &status
	elapsed := int(s.now().Sub(route.started).Milliseconds())
	row.GatewayLatencyMS = &elapsed
	row.TotalLatencyMS = &elapsed
	route.mu.Lock()
	route.finished = true
	route.mu.Unlock()
	s.enqueueLivePersist(route.cfg, row)
	return routeErr
}

// selectRoute fixes the model/effort and rewrites the body.
func (s *GPTAutoShadowService) selectRoute(route *GPTAutoRoute, model, effort string, order []string) {
	route.mu.Lock()
	defer route.mu.Unlock()
	s.applySelectionLocked(route, model, effort)
	if len(order) > 0 && order[0] == model {
		order = order[1:]
	}
	route.remaining = removeString(order, model)
	elapsed := int(s.now().Sub(route.started).Milliseconds())
	route.row.GatewayLatencyMS = &elapsed
}

func (s *GPTAutoShadowService) applySelectionLocked(route *GPTAutoRoute, model, effort string) {
	profile, _ := route.cfg.ProfileFor(model)
	route.model, route.effort = model, effort
	route.body = GPTAutoRewriteBody(route.endpoint, route.original, model, effort)
	route.billingMultiplier = 1
	// The legacy profile multiplier and the fixed tariff are never combined.
	if route.tariff.Mode == GPTAutoPricingModeLegacy && route.cfg.Live.ApplyBillingMultiplier && profile.BillingMultiplier > 0 {
		route.billingMultiplier = profile.BillingMultiplier
	}
	row := route.row
	row.FinalModel, row.FinalEffort = model, effort
	route.updateSwitchTelemetry()
	row.PricingModel = route.cfg.Pricing.PricingModel(model)
	if route.tariff.Mode == GPTAutoPricingModeFixed {
		row.PricingModel = GPTAutoVirtualModel
	}
	multiplier := route.billingMultiplier
	row.BillingMultiplier = &multiplier
	weight := profile.RoutingWeight
	row.RoutingWeight = &weight
}

// GPTAutoRewriteBody replaces the virtual model and sets the reasoning effort
// in the endpoint's own field. The original buffer is never modified.
func GPTAutoRewriteBody(endpoint string, body []byte, model, effort string) []byte {
	out, err := sjson.SetBytes(append([]byte(nil), body...), "model", model)
	if err != nil {
		return body
	}
	if effort == "" {
		return out
	}
	switch endpoint {
	case GPTAutoEndpointResponses:
		if next, err := sjson.SetBytes(out, "reasoning.effort", effort); err == nil {
			out = next
		}
		if gjson.GetBytes(out, "reasoning_effort").Exists() {
			out, _ = sjson.DeleteBytes(out, "reasoning_effort")
		}
	default:
		if next, err := sjson.SetBytes(out, "reasoning_effort", effort); err == nil {
			out = next
		}
		if gjson.GetBytes(out, "reasoning.effort").Exists() {
			out, _ = sjson.DeleteBytes(out, "reasoning.effort")
		}
	}
	return out
}

// Reroute moves the request to the next eligible candidate when the selected
// model cannot be scheduled (no upstream account) before anything was sent
// upstream. Session-affinity and stateful continuations never reroute.
func (r *GPTAutoRoute) Reroute(reason string) bool {
	if r == nil {
		return false
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.finished || r.hardState || gjson.GetBytes(r.original, "conversation").Exists() || len(r.remaining) == 0 || r.row.RoutingReason == GPTAutoRouteAffinity ||
		gjson.GetBytes(r.original, "previous_response_id").String() != "" {
		return false
	}
	// Pre-execution capacity fallback must not undo explicit switch controls.
	if r.dynamicEnabled() && r.previous != nil {
		c := r.cfg.Live.DynamicRouting
		for len(r.remaining) > 0 {
			delta := c.CapabilityTiers[r.remaining[0]] - c.CapabilityTiers[r.previous.Model]
			if (delta > 0 && !c.AllowModelUpgrade) || (delta < 0 && !c.AllowModelDowngrade) {
				r.remaining = r.remaining[1:]
				continue
			}
			break
		}
		if len(r.remaining) == 0 {
			return false
		}
	}
	from := r.model
	next := r.remaining[0]
	r.remaining = r.remaining[1:]
	profile, _ := r.cfg.ProfileFor(next)
	effort, _ := gptAutoPickEffort(profile.SupportedEfforts, r.effort)
	if effort == "" {
		effort = r.effort
	}
	r.svc.applySelectionLocked(r, next, effort)
	r.reroutes++
	r.row.RetryCount = r.reroutes
	if t := r.row.Features.Switching; t != nil {
		t.SwitchReason = "no_upstream_available"
	}
	r.row.DecisionStatus = GPTAutoDecisionAdjusted
	if r.row.FallbackReason == "" {
		r.row.FallbackReason = reason
	}
	if r.row.Decision != nil {
		r.row.Decision.Adjustments = append(r.row.Decision.Adjustments, GPTAutoAdjustment{
			Rule: GPTAutoRuleModelUnavailable, From: from, To: next, Detail: "no schedulable upstream account for the selected model",
		})
		r.row.Decision.FinalModel, r.row.Decision.FinalEffort = next, effort
	}
	r.svc.incr(gptAutoCounterLiveRerouted)
	return true
}

// ObserveResult records what the upstream reported for the executed request.
// Safe to call more than once; the first successful observation wins.
func (r *GPTAutoRoute) ObserveResult(result *OpenAIForwardResult) {
	if r == nil || result == nil {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.resultObserved {
		return
	}
	r.resultObserved = true
	if result.ReasoningEffort != nil && IsGPTAutoEffort(*result.ReasoningEffort) {
		r.effort = *result.ReasoningEffort
		r.row.FinalEffort = r.effort
		if r.row.Decision != nil {
			r.row.Decision.FinalEffort = r.effort
		}
		r.updateSwitchTelemetry()
	}
	r.responseID = strings.TrimSpace(result.ResponseID)
	r.servedModel = strings.TrimSpace(result.UpstreamResponseModel)
	if result.FirstTokenMs != nil {
		value := *result.FirstTokenMs
		r.firstTokenMS = &value
	}
}

// GPTAutoRouteOutcome is what the handler knows when the request ends.
type GPTAutoRouteOutcome struct {
	HTTPStatus     int
	AccountID      int64
	ClientGone     bool
	StreamStarted  bool
	ErrorCategory  string
	UpstreamFailed bool
}

// Finish records the request outcome, stores session affinity and enqueues
// the decision for persistence. Idempotent.
func (r *GPTAutoRoute) Finish(ctx context.Context, outcome GPTAutoRouteOutcome) {
	if r == nil {
		return
	}
	r.mu.Lock()
	if r.finished {
		r.mu.Unlock()
		return
	}
	r.finished = true
	row := r.row
	total := int(r.svc.now().Sub(r.started).Milliseconds())
	row.TotalLatencyMS = &total
	row.UpstreamTTFTMS = r.firstTokenMS
	row.ServedModel = r.servedModel
	if outcome.AccountID > 0 {
		account := outcome.AccountID
		row.UpstreamAccountID = &account
	}
	if outcome.HTTPStatus > 0 {
		status := outcome.HTTPStatus
		row.HTTPStatus = &status
	}
	switch {
	case outcome.ClientGone:
		// The gateway may already have recorded a 4xx (e.g. 499) for the
		// disconnect; the cause is the client either way. Any usage the
		// upstream reported is still billed through usage_logs.
		row.RequestStatus = GPTAutoRequestCancelled
		row.ErrorCategory = GPTAutoFailureClientCancelled
	case outcome.HTTPStatus >= 400 || outcome.UpstreamFailed:
		row.RequestStatus = GPTAutoRequestFailed
		row.ErrorCategory = outcome.ErrorCategory
		if row.ErrorCategory == "" {
			row.ErrorCategory = gptAutoHTTPErrorCategory(outcome.HTTPStatus)
		}
	default:
		row.RequestStatus = GPTAutoRequestSucceeded
	}
	succeeded := row.RequestStatus == GPTAutoRequestSucceeded
	affinity := GPTAutoAffinity{Model: r.model, Effort: r.effort, AccountID: outcome.AccountID}
	keys := append([]string(nil), r.keys...)
	if t := row.Features.Switching; t != nil {
		affinity.Sequence = t.Sequence
		affinity.TaskFingerprint = r.taskFingerprint
		affinity.DowngradeCount = r.downgradeCount
		affinity.DowngradeModel = r.downgradeModel
		affinity.History = append(append([]GPTAutoSessionTurn(nil), t.History...), GPTAutoSessionTurn{Sequence: t.Sequence, Model: r.model, Effort: r.effort, Direction: t.SwitchDirection})
		if len(affinity.History) > 10 {
			affinity.History = affinity.History[len(affinity.History)-10:]
		}
		// Response references are immutable state bindings, never overwrite a parent response.
		if r.hardState {
			keys = nil
		}
	}
	if succeeded && r.responseID != "" && row.APIKeyID != nil {
		keys = append([]string{gptAutoAffinityKey(*row.APIKeyID, "resp", r.responseID)}, keys...)
	}
	cfg := r.cfg
	r.mu.Unlock()

	if succeeded && (cfg.Live.SessionAffinityEnabled || r.dynamicEnabled()) && r.svc.live.affinity != nil && len(keys) > 0 {
		opCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), gptAutoLiveAffinityOpTTL)
		ttl := time.Duration(cfg.Live.SessionAffinityTTLSeconds) * time.Second
		for _, key := range keys {
			if r.dynamicEnabled() && containsString(r.keys, key) && !r.hardState {
				if cas, ok := r.svc.live.affinity.(GPTAutoAffinityCASStore); ok {
					expected := int64(0)
					if r.previous != nil {
						expected = r.previous.Sequence
					}
					_, err := cas.CompareAndSet(opCtx, key, expected, affinity, ttl)
					if err != nil {
						slog.Warn("gpt_auto.affinity_cas_failed", "error", err)
					}
					continue
				}
				continue
			}
			if err := r.svc.live.affinity.Set(opCtx, key, affinity, ttl); err != nil {
				slog.Warn("gpt_auto.affinity_store_failed", "error", err)
				break
			}
		}
		cancel()
	}
	r.svc.enqueueLivePersist(cfg, row)
}

func gptAutoHTTPErrorCategory(status int) string {
	switch {
	case status == http.StatusTooManyRequests:
		return GPTAutoFailureRateLimited
	case status == http.StatusServiceUnavailable:
		return GPTAutoFailureNoUpstream
	case status >= 500:
		return GPTAutoFailureUpstream
	case status >= 400:
		return GPTAutoFailureClientError
	default:
		return ""
	}
}

func (s *GPTAutoShadowService) enqueueLivePersist(cfg GPTAutoShadowConfig, row *GPTAutoShadowDecision) {
	if s.repo == nil || row == nil {
		return
	}
	// Bound the decision payload: diagnostics are already capped, and the
	// classification/decision JSON is small and content-free.
	if row.RequestID != "" && utf8.RuneCountInString(row.RequestID) > 128 {
		row.RequestID = trimRunes(row.RequestID, 128)
	}
	select {
	case s.live.persistCh <- gptAutoLivePersistTask{cfg: cfg, row: row}:
	default:
		s.incr(gptAutoCounterLivePersistDrop)
		slog.Warn("gpt_auto.live_persist_queue_full", "request_id", row.RequestID)
	}
}

// LiveRuntimeStatus reports this node's live-routing state.
type GPTAutoLiveRuntimeStatus struct {
	Enabled          bool   `json:"enabled"`
	InternalOnly     bool   `json:"internal_only"`
	CircuitState     string `json:"circuit_state"`
	ActiveCalls      int64  `json:"active_calls"`
	PersistQueueLen  int    `json:"persist_queue_length"`
	TodayLiveCount   *int64 `json:"today_live_requests,omitempty"`
	DailyBudget      int    `json:"daily_budget"`
	FailurePolicy    string `json:"failure_policy"`
	AffinityEnabled  bool   `json:"session_affinity_enabled"`
	MarginGuard      bool   `json:"margin_guard_enabled"`
	BillingMultipier bool   `json:"apply_billing_multiplier"`
	PricingMode      string `json:"pricing_mode"`
	PricingVersion   int    `json:"pricing_config_version"`
	PricingReady     bool   `json:"pricing_ready"`
}

// applyGPTAutoTariffToRow records the pricing snapshot on the decision row.
func applyGPTAutoTariffToRow(row *GPTAutoShadowDecision, tariff GPTAutoTariff) {
	row.PricingMode = tariff.Mode
	version := tariff.Version
	row.PricingConfigVersion = &version
	row.PricingSource = tariff.Source
	if tariff.Mode != GPTAutoPricingModeFixed {
		return
	}
	row.PricingModel = GPTAutoVirtualModel
	input, cached, output, write := tariff.Input, tariff.CachedInput, tariff.Output, tariff.CacheWrite
	row.TariffInputPerMTok, row.TariffCachedPerMTok, row.TariffOutputPerMTok, row.TariffCacheWritePerMTok = &input, &cached, &output, &write
}

func (s *GPTAutoShadowService) liveRuntimeStatus(ctx context.Context, cfg GPTAutoShadowConfig) GPTAutoLiveRuntimeStatus {
	status := GPTAutoLiveRuntimeStatus{
		Enabled:          cfg.Live.Enabled,
		InternalOnly:     cfg.Live.InternalOnly,
		CircuitState:     s.live.breaker.state(s.now()),
		ActiveCalls:      s.live.active.Load(),
		PersistQueueLen:  len(s.live.persistCh),
		DailyBudget:      cfg.Live.DailyRequestBudget,
		FailurePolicy:    cfg.Live.ClassifierFailurePolicy,
		AffinityEnabled:  cfg.Live.SessionAffinityEnabled,
		MarginGuard:      cfg.Live.MarginGuardEnabled,
		BillingMultipier: cfg.Live.ApplyBillingMultiplier && !cfg.Pricing.IsFixed(),
		PricingMode:      cfg.Pricing.Mode,
		PricingVersion:   cfg.Pricing.Version,
		PricingReady:     cfg.Pricing.Ready(),
	}
	opCtx, cancel := context.WithTimeout(ctx, gptAutoBudgetOpTimeout)
	defer cancel()
	if count, err := s.LiveBudgetUsage(opCtx); err == nil {
		status.TodayLiveCount = &count
	}
	return status
}

// applyGPTAutoBillingMultiplier scales a customer charge in decimal and rounds
// to the usage-log precision, so repeated multipliers never drift.
func applyGPTAutoBillingMultiplier(charge, multiplier float64) float64 {
	if multiplier <= 0 || charge == 0 {
		return charge
	}
	return gptAutoUSD(gptAutoDecimal(charge).Mul(gptAutoDecimal(multiplier)))
}
