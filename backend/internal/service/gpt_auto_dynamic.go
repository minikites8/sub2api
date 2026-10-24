package service

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"github.com/tidwall/gjson"
	"net/http"
	"strings"
	"time"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
)

// GPTAutoDynamicConfig is part of the existing versioned settings document.
// Capability tiers are operator assertions, independent of names and routing weights.
type GPTAutoDynamicConfig struct {
	Enabled                   bool           `json:"enabled"`
	StatelessStrategy         string         `json:"stateless_strategy"`
	StatefulStrategy          string         `json:"stateful_strategy"`
	AllowModelUpgrade         bool           `json:"allow_model_upgrade"`
	AllowModelDowngrade       bool           `json:"allow_model_downgrade"`
	AllowEffortChange         bool           `json:"allow_effort_change"`
	DowngradeStabilityCount   int            `json:"downgrade_stability_count"`
	EnableClassificationCache bool           `json:"enable_classification_cache"`
	PreserveAccountAffinity   bool           `json:"preserve_account_affinity_when_required"`
	CapabilityTiers           map[string]int `json:"capability_tiers"`
}

func DefaultGPTAutoDynamicConfig() *GPTAutoDynamicConfig {
	return &GPTAutoDynamicConfig{StatelessStrategy: "per_request", StatefulStrategy: "preserve_required_affinity", AllowModelUpgrade: true, AllowModelDowngrade: true, AllowEffortChange: true, DowngradeStabilityCount: 2, PreserveAccountAffinity: true, CapabilityTiers: map[string]int{GPTAutoModelGPT56Luna: 1, GPTAutoModelGPT6Luna: 2, GPTAutoModelGPT61Sol: 3}}
}
func validateGPTAutoDynamic(c *GPTAutoDynamicConfig) error {
	if c == nil {
		return nil
	}
	bad := func(s string) error {
		return infraerrors.BadRequest("GPT_AUTO_CONFIG_INVALID", "live.dynamic_routing: "+s)
	}
	if c.StatelessStrategy != "per_request" || c.StatefulStrategy != "preserve_required_affinity" {
		return bad("only per_request / preserve_required_affinity are supported")
	}
	if c.EnableClassificationCache {
		return bad("classification caching is not implemented; keep disabled")
	}
	if !c.PreserveAccountAffinity {
		return bad("required upstream account affinity cannot be disabled")
	}
	if c.DowngradeStabilityCount < 1 || c.DowngradeStabilityCount > 20 {
		return bad("downgrade_stability_count must be between 1 and 20")
	}
	if len(c.CapabilityTiers) != len(GPTAutoModelPool()) {
		return bad("configure a capability tier for each active model")
	}
	for _, m := range GPTAutoModelPool() {
		if c.CapabilityTiers[m] < 1 || c.CapabilityTiers[m] > 100 {
			return bad("capability tiers must be between 1 and 100")
		}
	}
	return nil
}

type GPTAutoSessionTurn struct {
	Sequence  int64  `json:"sequence"`
	Model     string `json:"model"`
	Effort    string `json:"effort"`
	Direction string `json:"direction"`
}

// Persisted in the existing features JSON. Old records retain nil/unknown.
type GPTAutoSwitchTelemetry struct {
	PreviousModel          string               `json:"previous_model,omitempty"`
	PreviousEffort         string               `json:"previous_effort,omitempty"`
	PolicyModel            string               `json:"policy_model,omitempty"`
	PolicyEffort           string               `json:"policy_effort,omitempty"`
	ModelSwitched          bool                 `json:"model_switched"`
	EffortChanged          bool                 `json:"effort_changed"`
	SwitchDirection        string               `json:"switch_direction"`
	SwitchReason           string               `json:"switch_reason"`
	SwitchBlockReason      string               `json:"switch_block_reason,omitempty"`
	ClassificationTrigger  string               `json:"classification_trigger"`
	ClassificationCacheHit bool                 `json:"classification_cache_hit"`
	AffinityType           string               `json:"affinity_type"`
	StateDependency        string               `json:"state_dependency"`
	SessionID              string               `json:"session_correlation_id,omitempty"`
	Sequence               int64                `json:"session_decision_sequence"`
	History                []GPTAutoSessionTurn `json:"history,omitempty"`
}

// CAS prevents two in-flight turns from counting as two sequential downgrade
// votes or overwriting newer session state. A losing branch remains logged.
type GPTAutoAffinityCASStore interface {
	CompareAndSet(context.Context, string, int64, GPTAutoAffinity, time.Duration) (bool, error)
}

func (s *localGPTAutoAffinityStore) CompareAndSet(_ context.Context, key string, expected int64, value GPTAutoAffinity, ttl time.Duration) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	entry, ok := s.entries[key]
	if ok && !s.now().Before(entry.expiresAt) {
		delete(s.entries, key)
		ok = false
	}
	if (ok && entry.value.Sequence != expected) || (!ok && expected != 0) {
		return false, nil
	}
	if !ok && len(s.entries) >= gptAutoLocalAffinityMaxEntries {
		for k := range s.entries {
			delete(s.entries, k)
			break
		}
	}
	s.entries[key] = localGPTAutoAffinityEntry{value: value, expiresAt: s.now().Add(ttl)}
	return true, nil
}
func gptAutoPrivateFingerprint(secret, scope, value string) string {
	h := hmac.New(sha256.New, []byte(secret))
	h.Write([]byte(scope))
	h.Write([]byte{0})
	h.Write([]byte(value))
	return hex.EncodeToString(h.Sum(nil))
}
func (r *GPTAutoRoute) dynamicEnabled() bool {
	return r.cfg.Live.DynamicRouting != nil && r.cfg.Live.DynamicRouting.Enabled
}
func gptAutoStateError() *GPTAutoRouteError {
	return &GPTAutoRouteError{Status: http.StatusConflict, Type: "invalid_request_error", Code: "gpt_auto_state_continuation_unavailable", Message: "The required conversation state binding is unavailable or incompatible; start a new self-contained request."}
}

// ValidateAccount prevents fallback scheduling from moving server-side state.
func (r *GPTAutoRoute) ValidateAccount(id int64) *GPTAutoRouteError {
	if r != nil && r.hardState && (r.requiredAccount <= 0 || id != r.requiredAccount) {
		return gptAutoStateError()
	}
	return nil
}
func (r *GPTAutoRoute) RequiresStateBinding() bool { return r != nil && r.hardState }

func (r *GPTAutoRoute) prepareDynamic(snapshot GPTAutoRequestSnapshot, input GPTAutoRouteInput, stateful bool) {
	if !r.dynamicEnabled() {
		return
	}
	t := &GPTAutoSwitchTelemetry{AffinityType: "none", StateDependency: "none", ClassificationTrigger: "new_session", SwitchDirection: "unchanged", Sequence: 1}
	r.row.Features.Switching = t
	if strings.TrimSpace(input.SessionKey) != "" {
		t.AffinityType = "soft"
		t.SessionID = gptAutoPrivateFingerprint(r.cfg.ClassifierAPIKey, "session", gptAutoAffinityKey(input.APIKeyID, "sess", input.SessionKey))
	}
	if stateful {
		r.hardState = true
		t.AffinityType = "hard"
		t.StateDependency = "hard_model_and_account"
		if gjson.GetBytes(input.Body, "conversation").Exists() {
			t.StateDependency = "unknown"
		}
		t.ClassificationTrigger = "stateful_continuation"
		t.SwitchBlockReason = "stateful_continuation"
	}
	anchor := snapshot.LatestUser
	if snapshot.LatestUserKind == GPTAutoUserKindFollowUp && snapshot.TaskContext != nil {
		anchor = snapshot.TaskContext.ContinuesRequest
	}
	// Missing anchors never inherit a downgrade streak. No content stored.
	if anchor != "" {
		policyHash, _, _ := NewGPTAutoPolicySnapshot(r.cfg).Hash()
		r.taskFingerprint = gptAutoPrivateFingerprint(r.cfg.ClassifierAPIKey, "task:"+policyHash, anchor)
	}
}
func (r *GPTAutoRoute) rememberPrevious(a *GPTAutoAffinity) {
	r.previous = a
	t := r.row.Features.Switching
	if t == nil || a == nil {
		return
	}
	t.PreviousModel = a.Model
	t.PreviousEffort = a.Effort
	t.Sequence = a.Sequence + 1
	t.History = append([]GPTAutoSessionTurn(nil), a.History...)
	if !r.hardState {
		t.ClassificationTrigger = "per_request"
	}
}
func (r *GPTAutoRoute) switchPolicy(d *GPTAutoPolicyDecision) {
	if !r.dynamicEnabled() || r.hardState {
		return
	}
	t, c, a := r.row.Features.Switching, r.cfg.Live.DynamicRouting, r.previous
	t.SwitchReason = "current_classification"
	if a == nil {
		t.PolicyModel = d.FinalModel
		t.PolicyEffort = d.FinalEffort
		return
	}
	// Only an eligible policy candidate can be retained. Never undo an exclusion.
	var prior *GPTAutoCandidate
	for i := range d.Candidates {
		if d.Candidates[i].Model == a.Model && d.Candidates[i].Eligible {
			prior = &d.Candidates[i]
		}
	}
	if prior != nil && d.FinalModel != "" {
		delta := c.CapabilityTiers[d.FinalModel] - c.CapabilityTiers[a.Model]
		block := ""
		if delta > 0 && !c.AllowModelUpgrade || delta < 0 && !c.AllowModelDowngrade {
			block = "policy_constraint"
		}
		if delta < 0 && block == "" {
			r.downgradeCount = 1
			if r.taskFingerprint != "" && a.TaskFingerprint == r.taskFingerprint && a.DowngradeModel == d.FinalModel {
				r.downgradeCount += a.DowngradeCount
			}
			r.downgradeModel = d.FinalModel
			if r.downgradeCount < c.DowngradeStabilityCount {
				block = "downgrade_hysteresis"
			} else {
				r.downgradeCount = 0
				r.downgradeModel = ""
			}
		}
		if block != "" {
			d.FinalModel = a.Model
			t.SwitchBlockReason = block
			d.Status = GPTAutoDecisionAdjusted
		}
		if !c.AllowEffortChange && r.cfg.Live.ClientEffortPolicy != GPTAutoClientEffortHonor {
			d.FinalEffort = a.Effort
		}
		p, _ := r.cfg.ProfileFor(d.FinalModel)
		d.FinalEffort, _ = gptAutoPickEffort(p.SupportedEfforts, d.FinalEffort)
	}
	t.PolicyModel = d.FinalModel
	t.PolicyEffort = d.FinalEffort
}
func (r *GPTAutoRoute) updateSwitchTelemetry() {
	t := r.row.Features.Switching
	if t == nil {
		return
	}
	t.ModelSwitched = t.PreviousModel != "" && t.PreviousModel != r.model
	t.EffortChanged = t.PreviousEffort != "" && t.PreviousEffort != r.effort
	t.SwitchDirection = "unchanged"
	if t.ModelSwitched {
		delta := r.cfg.Live.DynamicRouting.CapabilityTiers[r.model] - r.cfg.Live.DynamicRouting.CapabilityTiers[t.PreviousModel]
		t.SwitchDirection = "lateral"
		if delta > 0 {
			t.SwitchDirection = "upgrade"
		}
		if delta < 0 {
			t.SwitchDirection = "downgrade"
		}
	}
}

func (r *GPTAutoRoute) DynamicRoutingEnabled() bool { return r != nil && r.dynamicEnabled() }

// ApplyExecutionEffort records the existing group effort policy before forwarding.
// It does not relax that policy or independently select a new effort.
func (r *GPTAutoRoute) ApplyExecutionEffort(body []byte) {
	if r == nil || !r.dynamicEnabled() {
		return
	}
	field := "reasoning_effort"
	if r.endpoint == GPTAutoEndpointResponses {
		field = "reasoning.effort"
	}
	effort := gjson.GetBytes(body, field).String()
	if !IsGPTAutoEffort(effort) {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.effort = effort
	r.row.FinalEffort = effort
	if r.row.Decision != nil {
		r.row.Decision.FinalEffort = effort
	}
	r.updateSwitchTelemetry()
}
