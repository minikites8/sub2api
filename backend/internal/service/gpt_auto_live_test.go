package service

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

// ---- helpers ---------------------------------------------------------------

func newGPTAutoLiveTestService(t *testing.T, mutate func(*GPTAutoShadowConfig)) (*GPTAutoShadowService, *gptAutoTestRepo, *gptAutoTestClassifier) {
	t.Helper()
	svc, repo, classifier, _ := newGPTAutoTestService(t, func(c *GPTAutoShadowConfig) {
		c.ShadowEnabled = false
		c.Live.Enabled = true
		c.Live.AllowedAPIKeyIDs = []int64{22}
		c.Live.ClassifierTimeoutMS = 1000
		c.Live.DailyRequestBudget = 0
		// Phase 2 tests exercise the legacy pricing mode; Phase 2.1 tests
		// opt into independent pricing explicitly.
		c.Pricing = GPTAutoPricingConfig{Mode: GPTAutoPricingModeLegacy}
		if mutate != nil {
			mutate(c)
		}
	})
	svc.Start()
	return svc, repo, classifier
}

const gptAutoLiveChatBody = `{"model":"gpt-auto","stream":true,"messages":[{"role":"user","content":"Help me debug this Go concurrency issue."}]}`

func gptAutoLiveInput(body string) GPTAutoRouteInput {
	group := int64(7)
	return GPTAutoRouteInput{
		Endpoint: GPTAutoEndpointChatCompletions, Body: []byte(body), RequestID: "req-live-1",
		UserID: 11, APIKeyID: 22, GroupID: &group,
		Group: &Group{ID: group, Platform: PlatformOpenAI, GPTAutoEnabled: true},
	}
}

func gptAutoRouteErr(t *testing.T, err error) *GPTAutoRouteError {
	t.Helper()
	var routeErr *GPTAutoRouteError
	require.True(t, errors.As(err, &routeErr), "expected GPTAutoRouteError, got %v", err)
	return routeErr
}

func gptAutoClassifyAs(classifier *gptAutoTestClassifier, model, effort string) {
	classifier.result = func() (*GPTAutoClassification, error) {
		return gptAutoSampleClassification(model, effort), nil
	}
}

func (r *gptAutoTestRepo) lastRow(t *testing.T) GPTAutoShadowDecision {
	t.Helper()
	r.mu.Lock()
	defer r.mu.Unlock()
	require.NotEmpty(t, r.rows)
	return r.rows[len(r.rows)-1]
}

// ---- pool --------------------------------------------------------------------

func TestGPTAutoPoolHasExactlyThreeModels(t *testing.T) {
	require.Equal(t, []string{GPTAutoModelGPT56Luna, GPTAutoModelGPT6Luna, GPTAutoModelGPT61Sol}, GPTAutoModelPool())
	require.False(t, IsGPTAutoModel(GPTAutoModelGPT56Sol), "gpt-5.6-sol is retired from the GPT Auto pool")
	require.True(t, isGPTAutoRetiredModel("GPT-5.6-SOL"))
	for _, profile := range DefaultGPTAutoModelProfiles() {
		require.NotEqual(t, GPTAutoModelGPT56Sol, profile.Model)
	}
	questions := buildGPTAutoQuestions(*DefaultGPTAutoShadowConfig())
	raw, err := json.Marshal(questions)
	require.NoError(t, err)
	require.NotContains(t, string(raw), GPTAutoModelGPT56Sol, "Jev must not be offered the retired model")
}

func TestGPTAutoFourModelConfigMigratesWithoutResettingRetainedProfiles(t *testing.T) {
	stored := map[string]any{
		"policy_version": "shadow-v1",
		"model_profiles": []map[string]any{
			{"model": "gpt-5.6-luna", "enabled": true, "billing_multiplier": 1.33, "routing_weight": 41, "supported_efforts": []string{"low", "medium"},
				"upstream_costs": []map[string]any{{"channel": "c1", "enabled": true, "input_per_m": 0.7, "output_per_m": 2.1}}},
			{"model": "gpt-5.6-sol", "enabled": true, "billing_multiplier": 1.77, "routing_weight": 12},
			{"model": "gpt-6-luna", "enabled": false, "billing_multiplier": 1.05, "routing_weight": 300},
			{"model": "gpt-6.1-sol", "enabled": true, "billing_multiplier": 2.5, "routing_weight": 9, "min_margin": 0.4},
		},
	}
	raw, err := json.Marshal(stored)
	require.NoError(t, err)
	cfg := parseGPTAutoShadowConfig(string(raw))

	require.Len(t, cfg.ModelProfiles, 3)
	luna, _ := cfg.ProfileFor(GPTAutoModelGPT56Luna)
	require.Equal(t, 1.33, luna.BillingMultiplier)
	require.Equal(t, 41, luna.RoutingWeight)
	require.Equal(t, []string{"low", "medium"}, luna.SupportedEfforts)
	require.Len(t, luna.UpstreamCosts, 1)
	six, _ := cfg.ProfileFor(GPTAutoModelGPT6Luna)
	require.False(t, six.Enabled)
	require.Equal(t, 300, six.RoutingWeight)
	sol, _ := cfg.ProfileFor(GPTAutoModelGPT61Sol)
	require.Equal(t, 2.5, sol.BillingMultiplier)
	require.NotNil(t, sol.MinMargin)
	require.InDelta(t, 0.4, *sol.MinMargin, 1e-9)

	require.Len(t, cfg.RetiredModelProfiles, 1, "the retired profile is preserved, not deleted")
	require.Equal(t, GPTAutoModelGPT56Sol, cfg.RetiredModelProfiles[0].Model)
	require.Equal(t, 1.77, cfg.RetiredModelProfiles[0].BillingMultiplier)
	require.Equal(t, GPTAutoModelPoolVersion, cfg.ModelPoolVersion)
	require.Equal(t, GPTAutoPolicyVersionDefault, cfg.PolicyVersion, "the untouched Phase 1 default is upgraded")
	require.False(t, cfg.Live.Enabled, "a config stored before Phase 2 keeps live routing off")
	require.True(t, cfg.Live.InternalOnly)

	// A customised policy version is never overwritten.
	stored["policy_version"] = "ops-2026-09"
	raw, _ = json.Marshal(stored)
	require.Equal(t, "ops-2026-09", parseGPTAutoShadowConfig(string(raw)).PolicyVersion)

	// Re-normalizing is idempotent: the retired profile is not duplicated.
	again := *cfg
	normalizeGPTAutoShadowConfig(&again)
	require.Len(t, again.RetiredModelProfiles, 1)
}

func TestGPTAutoConfigRejectsRetiredModelProfile(t *testing.T) {
	cfg := DefaultGPTAutoShadowConfig()
	cfg.ModelProfiles = append(cfg.ModelProfiles, GPTAutoModelProfile{Model: GPTAutoModelGPT56Sol, Enabled: true, BillingMultiplier: 1, SupportedEfforts: []string{"low"}})
	err := ValidateGPTAutoShadowConfig(cfg)
	require.Error(t, err)
	require.Contains(t, err.Error(), "retired")
}

func TestGPTAutoHistoricalRetiredDecisionStillReplays(t *testing.T) {
	// A Phase 1 decision recommended gpt-5.6-sol. Replaying it against the
	// current pool must not panic and must explain why it moved.
	cls := *gptAutoSampleClassification(GPTAutoModelGPT56Sol, GPTAutoEffortHigh)
	cls.ModelProbabilities = map[string]float64{GPTAutoModelGPT56Sol: 0.6, GPTAutoModelGPT61Sol: 0.3, GPTAutoModelGPT6Luna: 0.1}
	decision := EvaluateGPTAutoPolicy(gptAutoPolicyInput(cls, nil))
	require.Equal(t, GPTAutoModelGPT56Sol, decision.JevModel, "Jev's historical answer is kept verbatim")
	require.Equal(t, GPTAutoModelGPT61Sol, decision.FinalModel)
	require.Equal(t, GPTAutoRuleModelUnknown, decision.Adjustments[0].Rule)
}

// ---- admission ---------------------------------------------------------------

func TestGPTAutoLiveDisabledByDefaultIsUnknownModel(t *testing.T) {
	svc, repo, classifier, _ := newGPTAutoTestService(t, nil)
	svc.Start()
	route, err := svc.Route(context.Background(), gptAutoLiveInput(gptAutoLiveChatBody))
	require.Nil(t, route)
	routeErr := gptAutoRouteErr(t, err)
	require.Equal(t, http.StatusNotFound, routeErr.Status)
	require.Equal(t, "model_not_found", routeErr.Code)
	require.Zero(t, classifier.calls.Load())
	require.Zero(t, repo.count(), "unadmitted requests are not logged")
}

func TestGPTAutoLiveAdmission(t *testing.T) {
	group := int64(7)
	live := DefaultGPTAutoLiveConfig()
	live.Enabled = true
	live.AllowedGroupIDs = []int64{7}
	require.False(t, live.AdmitsAPIKey(22, &group), "internal_only ignores group allow list")
	live.AllowedAPIKeyIDs = []int64{22}
	require.True(t, live.AdmitsAPIKey(22, &group))
	require.False(t, live.AdmitsAPIKey(23, &group))
	live.InternalOnly = false
	require.True(t, live.AdmitsAPIKey(23, &group))
	other := int64(8)
	require.False(t, live.AdmitsAPIKey(23, &other))
	require.False(t, live.AdmitsAPIKey(23, nil))
	live.Enabled = false
	require.False(t, live.AdmitsAPIKey(22, &group), "kill switch wins over every allow list")
}

func TestGPTAutoLiveEndpointRestriction(t *testing.T) {
	svc, _, _ := newGPTAutoLiveTestService(t, func(c *GPTAutoShadowConfig) {
		c.Live.Endpoints = []string{GPTAutoEndpointResponses}
	})
	_, err := svc.Route(context.Background(), gptAutoLiveInput(gptAutoLiveChatBody))
	require.Equal(t, "gpt_auto_endpoint_unsupported", gptAutoRouteErr(t, err).Code)
}

// ---- model and effort selection ---------------------------------------------

func TestGPTAutoLiveSelectsEachPoolModelAndRewritesBody(t *testing.T) {
	for _, model := range GPTAutoModelPool() {
		t.Run(model, func(t *testing.T) {
			svc, repo, classifier := newGPTAutoLiveTestService(t, nil)
			gptAutoClassifyAs(classifier, model, GPTAutoEffortHigh)
			route, err := svc.Route(context.Background(), gptAutoLiveInput(gptAutoLiveChatBody))
			require.NoError(t, err)
			require.Equal(t, model, route.Model())
			require.Equal(t, GPTAutoEffortHigh, route.Effort())
			body := route.Body()
			require.Equal(t, model, gjson.GetBytes(body, "model").String())
			require.Equal(t, GPTAutoEffortHigh, gjson.GetBytes(body, "reasoning_effort").String())
			require.True(t, gjson.GetBytes(body, "stream").Bool(), "other fields are untouched")
			require.Equal(t, "gpt-auto", gjson.Get(gptAutoLiveChatBody, "model").String(), "the original buffer is not modified")

			route.Finish(context.Background(), GPTAutoRouteOutcome{HTTPStatus: 200, AccountID: 5})
			repo.waitRows(t, 1)
			row := repo.lastRow(t)
			require.Equal(t, GPTAutoModeLive, row.Mode)
			require.Equal(t, GPTAutoVirtualModel, row.ActualModel)
			require.Equal(t, model, row.JevModel)
			require.Equal(t, model, row.FinalModel)
			require.Equal(t, GPTAutoRouteClassified, row.RoutingReason)
			require.Equal(t, GPTAutoRequestSucceeded, row.RequestStatus)
			require.Equal(t, GPTAutoClassifierStateVersion, row.StateVersion)
			require.Equal(t, GPTAutoClassifierSchemaVersion, row.ClassifierSchema)
			require.NotEmpty(t, row.ClassifierConfigHash)
			require.NotNil(t, row.UpstreamAccountID)
			require.Equal(t, int64(5), *row.UpstreamAccountID)
			require.NotNil(t, row.BillingMultiplier)
			require.NotNil(t, row.GatewayLatencyMS)
			require.NotNil(t, row.TotalLatencyMS)
		})
	}
}

func TestGPTAutoLiveResponsesEffortField(t *testing.T) {
	svc, _, classifier := newGPTAutoLiveTestService(t, nil)
	gptAutoClassifyAs(classifier, GPTAutoModelGPT6Luna, GPTAutoEffortLow)
	input := gptAutoLiveInput(`{"model":"gpt-auto","reasoning_effort":"high","input":"Summarise the plan"}`)
	input.Endpoint = GPTAutoEndpointResponses
	route, err := svc.Route(context.Background(), input)
	require.NoError(t, err)
	body := route.Body()
	require.Equal(t, GPTAutoEffortLow, gjson.GetBytes(body, "reasoning.effort").String())
	require.False(t, gjson.GetBytes(body, "reasoning_effort").Exists(), "the chat-style field is removed on Responses")
}

func TestGPTAutoLiveRetiredJevAnswerNeverExecutes(t *testing.T) {
	svc, _, classifier := newGPTAutoLiveTestService(t, nil)
	classifier.result = func() (*GPTAutoClassification, error) {
		cls := gptAutoSampleClassification(GPTAutoModelGPT56Sol, GPTAutoEffortHigh)
		cls.ModelProbabilities = map[string]float64{GPTAutoModelGPT56Sol: 0.5, GPTAutoModelGPT6Luna: 0.3, GPTAutoModelGPT61Sol: 0.2}
		return cls, nil
	}
	route, err := svc.Route(context.Background(), gptAutoLiveInput(gptAutoLiveChatBody))
	require.NoError(t, err)
	require.Equal(t, GPTAutoModelGPT6Luna, route.Model())
	require.True(t, IsGPTAutoModel(route.Model()))
}

func TestGPTAutoLiveUnsupportedEffortIsNormalized(t *testing.T) {
	svc, repo, classifier := newGPTAutoLiveTestService(t, nil)
	gptAutoClassifyAs(classifier, GPTAutoModelGPT61Sol, GPTAutoEffortNone)
	route, err := svc.Route(context.Background(), gptAutoLiveInput(gptAutoLiveChatBody))
	require.NoError(t, err)
	require.Equal(t, GPTAutoModelGPT61Sol, route.Model())
	require.Equal(t, GPTAutoEffortLow, route.Effort(), "gpt-6.1-sol rejects none; raised to the nearest supported level")
	route.Finish(context.Background(), GPTAutoRouteOutcome{HTTPStatus: 200})
	repo.waitRows(t, 1)
	row := repo.lastRow(t)
	require.Equal(t, GPTAutoEffortNone, row.JevEffort)
	require.Equal(t, GPTAutoEffortLow, row.FinalEffort)
	require.Equal(t, GPTAutoDecisionAdjusted, row.DecisionStatus)
}

func TestGPTAutoLiveClientEffortPolicy(t *testing.T) {
	body := `{"model":"gpt-auto","reasoning_effort":"xhigh","messages":[{"role":"user","content":"Prove this lemma"}]}`
	t.Run("jev decides by default", func(t *testing.T) {
		svc, repo, classifier := newGPTAutoLiveTestService(t, nil)
		gptAutoClassifyAs(classifier, GPTAutoModelGPT6Luna, GPTAutoEffortLow)
		route, err := svc.Route(context.Background(), gptAutoLiveInput(body))
		require.NoError(t, err)
		require.Equal(t, GPTAutoEffortLow, route.Effort())
		route.Finish(context.Background(), GPTAutoRouteOutcome{HTTPStatus: 200})
		repo.waitRows(t, 1)
		row := repo.lastRow(t)
		require.Equal(t, GPTAutoEffortXHigh, row.ActualEffort, "the client's explicit effort is recorded")
		require.Equal(t, GPTAutoEffortLow, row.JevEffort)
	})
	t.Run("honor_client", func(t *testing.T) {
		svc, repo, classifier := newGPTAutoLiveTestService(t, func(c *GPTAutoShadowConfig) {
			c.Live.ClientEffortPolicy = GPTAutoClientEffortHonor
		})
		gptAutoClassifyAs(classifier, GPTAutoModelGPT6Luna, GPTAutoEffortLow)
		route, err := svc.Route(context.Background(), gptAutoLiveInput(body))
		require.NoError(t, err)
		require.Equal(t, GPTAutoModelGPT6Luna, route.Model(), "the model is still Jev's choice")
		require.Equal(t, GPTAutoEffortXHigh, route.Effort())
		route.Finish(context.Background(), GPTAutoRouteOutcome{HTTPStatus: 200})
		repo.waitRows(t, 1)
		row := repo.lastRow(t)
		require.Equal(t, GPTAutoEffortLow, row.JevEffort, "Jev's recommendation is still recorded")
		require.Equal(t, GPTAutoEffortXHigh, row.FinalEffort)
		require.Contains(t, gptAutoAdjustmentRules(row.Decision), GPTAutoRuleClientEffort)
	})
}

func gptAutoAdjustmentRules(decision *GPTAutoPolicyDecision) []string {
	if decision == nil {
		return nil
	}
	out := make([]string, 0, len(decision.Adjustments))
	for _, adjustment := range decision.Adjustments {
		out = append(out, adjustment.Rule)
	}
	return out
}

// ---- capability validation -------------------------------------------------

func TestGPTAutoLiveCapabilityExclusions(t *testing.T) {
	disable := func(model string, edit func(*GPTAutoModelCapabilities)) func(*GPTAutoShadowConfig) {
		return func(c *GPTAutoShadowConfig) {
			for i := range c.ModelProfiles {
				if c.ModelProfiles[i].Model == model {
					caps := *DefaultGPTAutoModelCapabilities()
					edit(&caps)
					c.ModelProfiles[i].Capabilities = &caps
				}
			}
		}
	}
	cases := map[string]struct {
		mutate func(*GPTAutoShadowConfig)
		body   string
		reason string
	}{
		"vision": {disable(GPTAutoModelGPT6Luna, func(c *GPTAutoModelCapabilities) { c.Vision = false }),
			`{"model":"gpt-auto","messages":[{"role":"user","content":[{"type":"text","text":"What is this?"},{"type":"image_url","image_url":{"url":"https://example.com/a.png"}}]}]}`,
			GPTAutoRuleModelVision},
		"structured output": {disable(GPTAutoModelGPT6Luna, func(c *GPTAutoModelCapabilities) { c.StructuredOutputs = false }),
			`{"model":"gpt-auto","response_format":{"type":"json_schema","json_schema":{"name":"x","schema":{}}},"messages":[{"role":"user","content":"List three fruits"}]}`,
			GPTAutoRuleModelStructured},
		"context window": {disable(GPTAutoModelGPT6Luna, func(c *GPTAutoModelCapabilities) { c.MaxContextTokens = 5 }),
			gptAutoLiveChatBody, GPTAutoRuleModelContext},
		"tools": {func(c *GPTAutoShadowConfig) {
			for i := range c.ModelProfiles {
				if c.ModelProfiles[i].Model == GPTAutoModelGPT6Luna {
					c.ModelProfiles[i].SupportsTools = false
				}
			}
		}, `{"model":"gpt-auto","tools":[{"type":"function","function":{"name":"f","parameters":{}}}],"messages":[{"role":"user","content":"Call the tool"}]}`,
			GPTAutoRuleModelTools},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			svc, repo, classifier := newGPTAutoLiveTestService(t, tc.mutate)
			gptAutoClassifyAs(classifier, GPTAutoModelGPT6Luna, GPTAutoEffortMedium)
			route, err := svc.Route(context.Background(), gptAutoLiveInput(tc.body))
			require.NoError(t, err)
			require.NotEqual(t, GPTAutoModelGPT6Luna, route.Model(), "an incompatible model is never forwarded")
			route.Finish(context.Background(), GPTAutoRouteOutcome{HTTPStatus: 200})
			repo.waitRows(t, 1)
			row := repo.lastRow(t)
			var reasons []string
			for _, candidate := range row.Decision.Candidates {
				if candidate.Model == GPTAutoModelGPT6Luna {
					reasons = candidate.Reasons
				}
			}
			require.Contains(t, reasons, tc.reason)
		})
	}
}

func TestGPTAutoLiveRespectsUnderlyingModelPermissions(t *testing.T) {
	svc, _, classifier := newGPTAutoLiveTestService(t, nil)
	gptAutoClassifyAs(classifier, GPTAutoModelGPT61Sol, GPTAutoEffortHigh)
	input := gptAutoLiveInput(gptAutoLiveChatBody)
	input.ModelPermitted = func(model string) bool { return model != GPTAutoModelGPT61Sol }
	route, err := svc.Route(context.Background(), input)
	require.NoError(t, err)
	require.NotEqual(t, GPTAutoModelGPT61Sol, route.Model(), "gpt-auto must not bypass a denied model")
}

func TestGPTAutoLiveNoCompatibleModelIsStructuredError(t *testing.T) {
	svc, repo, classifier := newGPTAutoLiveTestService(t, nil)
	input := gptAutoLiveInput(gptAutoLiveChatBody)
	// gpt-auto itself is permitted; every underlying model is denied.
	input.ModelPermitted = func(model string) bool { return model == GPTAutoVirtualModel }
	route, err := svc.Route(context.Background(), input)
	require.Nil(t, route)
	routeErr := gptAutoRouteErr(t, err)
	require.Equal(t, http.StatusBadRequest, routeErr.Status)
	require.Equal(t, "gpt_auto_no_compatible_model", routeErr.Code)
	require.Contains(t, routeErr.Message, GPTAutoRuleModelNotPermitted)
	require.Zero(t, classifier.calls.Load(), "no classifier spend when nothing could run")
	repo.waitRows(t, 1)
	row := repo.lastRow(t)
	require.Equal(t, GPTAutoRequestRejected, row.RequestStatus)
	require.Equal(t, GPTAutoFailureNoCompatible, row.ErrorCategory)
	require.Equal(t, GPTAutoEvaluationSkipped, row.EvaluationStatus)
}

// ---- failure isolation -----------------------------------------------------

func TestGPTAutoLiveClassifierFailurePolicies(t *testing.T) {
	failures := map[string]error{
		"timeout":      &GPTAutoClassifierError{Class: GPTAutoErrTimeout},
		"rate limited": &GPTAutoClassifierError{Class: GPTAutoErrRateLimited},
		"auth":         &GPTAutoClassifierError{Class: GPTAutoErrAuth},
		"malformed":    &GPTAutoClassifierError{Class: GPTAutoErrInvalidAnswer},
	}
	for name, failure := range failures {
		t.Run(name+"/compatible_fallback", func(t *testing.T) {
			svc, repo, classifier := newGPTAutoLiveTestService(t, func(c *GPTAutoShadowConfig) {
				c.Live.FallbackModel = GPTAutoModelGPT56Luna
				c.Live.FallbackEffort = GPTAutoEffortMedium
			})
			classifier.result = func() (*GPTAutoClassification, error) { return nil, failure }
			route, err := svc.Route(context.Background(), gptAutoLiveInput(gptAutoLiveChatBody))
			require.NoError(t, err)
			require.Equal(t, GPTAutoModelGPT56Luna, route.Model())
			require.Equal(t, GPTAutoEffortMedium, route.Effort())
			route.Finish(context.Background(), GPTAutoRouteOutcome{HTTPStatus: 200})
			repo.waitRows(t, 1)
			row := repo.lastRow(t)
			require.Equal(t, GPTAutoRouteFallback, row.RoutingReason)
			require.Equal(t, failure.(*GPTAutoClassifierError).Class, row.FallbackReason)
			require.Equal(t, GPTAutoEvaluationFailed, row.EvaluationStatus)
		})
		t.Run(name+"/reject_request", func(t *testing.T) {
			svc, repo, classifier := newGPTAutoLiveTestService(t, func(c *GPTAutoShadowConfig) {
				c.Live.ClassifierFailurePolicy = GPTAutoFailureRejectRequest
			})
			classifier.result = func() (*GPTAutoClassification, error) { return nil, failure }
			route, err := svc.Route(context.Background(), gptAutoLiveInput(gptAutoLiveChatBody))
			require.Nil(t, route)
			routeErr := gptAutoRouteErr(t, err)
			require.Equal(t, http.StatusServiceUnavailable, routeErr.Status, "never an unexplained 502")
			require.Equal(t, "gpt_auto_routing_unavailable", routeErr.Code)
			repo.waitRows(t, 1)
			require.Equal(t, GPTAutoRequestRejected, repo.lastRow(t).RequestStatus)
		})
	}
}

func TestGPTAutoLiveFallbackSkipsIncompatibleFallbackModel(t *testing.T) {
	svc, _, classifier := newGPTAutoLiveTestService(t, func(c *GPTAutoShadowConfig) {
		c.Live.FallbackModel = GPTAutoModelGPT61Sol
	})
	classifier.result = func() (*GPTAutoClassification, error) {
		return nil, &GPTAutoClassifierError{Class: GPTAutoErrTimeout}
	}
	input := gptAutoLiveInput(gptAutoLiveChatBody)
	input.ModelPermitted = func(model string) bool { return model != GPTAutoModelGPT61Sol }
	route, err := svc.Route(context.Background(), input)
	require.NoError(t, err)
	require.NotEqual(t, GPTAutoModelGPT61Sol, route.Model(), "fallback only selects authorized, compatible models")
}

func TestGPTAutoLiveTimeoutIsBounded(t *testing.T) {
	svc, repo, classifier := newGPTAutoLiveTestService(t, func(c *GPTAutoShadowConfig) {
		c.Live.ClassifierTimeoutMS = 200
	})
	classifier.block = make(chan struct{}) // never answers
	started := time.Now()
	route, err := svc.Route(context.Background(), gptAutoLiveInput(gptAutoLiveChatBody))
	require.NoError(t, err)
	require.Less(t, time.Since(started), 2*time.Second)
	require.Equal(t, GPTAutoModelGPT6Luna, route.Model(), "default fallback model")
	route.Finish(context.Background(), GPTAutoRouteOutcome{HTTPStatus: 200})
	repo.waitRows(t, 1)
	require.Equal(t, GPTAutoErrTimeout, repo.lastRow(t).FallbackReason)
}

func TestGPTAutoLiveClientCancellationPropagates(t *testing.T) {
	svc, repo, classifier := newGPTAutoLiveTestService(t, func(c *GPTAutoShadowConfig) {
		c.Live.ClassifierTimeoutMS = 5000
	})
	classifier.block = make(chan struct{})
	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(50 * time.Millisecond)
		cancel()
	}()
	started := time.Now()
	route, err := svc.Route(ctx, gptAutoLiveInput(gptAutoLiveChatBody))
	require.Nil(t, route)
	require.Equal(t, "client_cancelled", gptAutoRouteErr(t, err).Code)
	require.Less(t, time.Since(started), time.Second, "the classifier call is cancelled with the client")
	repo.waitRows(t, 1)
	require.Equal(t, GPTAutoRequestCancelled, repo.lastRow(t).RequestStatus)
	require.Equal(t, "closed", svc.live.breaker.state(time.Now()), "a cancelled client is not a classifier failure")
}

func TestGPTAutoLiveCircuitBreakerOpens(t *testing.T) {
	svc, _, classifier := newGPTAutoLiveTestService(t, nil)
	classifier.result = func() (*GPTAutoClassification, error) {
		return nil, &GPTAutoClassifierError{Class: GPTAutoErrUpstream}
	}
	for i := 0; i < gptAutoBreakerThreshold; i++ {
		_, err := svc.Route(context.Background(), gptAutoLiveInput(gptAutoLiveChatBody))
		require.NoError(t, err)
	}
	calls := classifier.calls.Load()
	route, err := svc.Route(context.Background(), gptAutoLiveInput(gptAutoLiveChatBody))
	require.NoError(t, err)
	require.Equal(t, calls, classifier.calls.Load(), "an open circuit makes no classifier call")
	route.Finish(context.Background(), GPTAutoRouteOutcome{HTTPStatus: 200})
	require.Equal(t, "open", svc.live.breaker.state(time.Now()))
}

// gptAutoPeakClassifier blocks every call and records peak concurrency.
type gptAutoPeakClassifier struct {
	mu       sync.Mutex
	inFlight int
	peak     int
	release  chan struct{}
}

func (c *gptAutoPeakClassifier) Classify(ctx context.Context, _ GPTAutoShadowConfig, _ GPTAutoRequestSnapshot) (*GPTAutoClassification, error) {
	c.mu.Lock()
	c.inFlight++
	if c.inFlight > c.peak {
		c.peak = c.inFlight
	}
	c.mu.Unlock()
	defer func() {
		c.mu.Lock()
		c.inFlight--
		c.mu.Unlock()
	}()
	select {
	case <-c.release:
		return gptAutoSampleClassification(GPTAutoModelGPT6Luna, GPTAutoEffortMedium), nil
	case <-ctx.Done():
		return nil, &GPTAutoClassifierError{Class: GPTAutoErrTimeout, Err: ctx.Err()}
	}
}

func TestGPTAutoLiveConcurrencyIsBounded(t *testing.T) {
	svc, _, _ := newGPTAutoLiveTestService(t, func(c *GPTAutoShadowConfig) {
		c.Live.ClassifierMaxConcurrency = 2
		c.Live.ClassifierTimeoutMS = 300
	})
	peak := &gptAutoPeakClassifier{release: make(chan struct{})}
	svc.classifier = peak
	var wg sync.WaitGroup
	results := make(chan *GPTAutoRoute, 8)
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			route, err := svc.Route(context.Background(), gptAutoLiveInput(gptAutoLiveChatBody))
			if err == nil {
				results <- route
			}
		}()
	}
	wg.Wait()
	close(peak.release)
	close(results)
	peak.mu.Lock()
	require.LessOrEqual(t, peak.peak, 2, "never more classifier calls in flight than the limit")
	require.Positive(t, peak.peak)
	peak.mu.Unlock()
	require.Zero(t, svc.live.active.Load(), "all slots are released")
	count := 0
	for route := range results {
		count++
		require.True(t, IsGPTAutoModel(route.Model()), "every request still gets a compatible model")
	}
	require.Equal(t, 8, count, "saturation falls back instead of failing")
}

func TestGPTAutoLiveDailyBudget(t *testing.T) {
	svc, repo, classifier := newGPTAutoLiveTestService(t, func(c *GPTAutoShadowConfig) {
		c.Live.DailyRequestBudget = 2
	})
	for i := 0; i < 2; i++ {
		route, err := svc.Route(context.Background(), gptAutoLiveInput(gptAutoLiveChatBody))
		require.NoError(t, err)
		route.Finish(context.Background(), GPTAutoRouteOutcome{HTTPStatus: 200})
	}
	require.Equal(t, int64(2), classifier.calls.Load())
	route, err := svc.Route(context.Background(), gptAutoLiveInput(gptAutoLiveChatBody))
	require.NoError(t, err)
	require.Equal(t, int64(2), classifier.calls.Load(), "no classifier call beyond the live budget")
	route.Finish(context.Background(), GPTAutoRouteOutcome{HTTPStatus: 200})
	repo.waitRows(t, 3)
	require.Equal(t, GPTAutoFailureLiveBudget, repo.lastRow(t).FallbackReason)
	used, err := svc.LiveBudgetUsage(context.Background())
	require.NoError(t, err)
	require.Equal(t, int64(2), used)
}

func TestGPTAutoLiveBudgetIsNotSpentWithoutAJevCall(t *testing.T) {
	svc, _, classifier := newGPTAutoLiveTestService(t, func(c *GPTAutoShadowConfig) {
		c.Live.DailyRequestBudget = 100
	})
	classifier.result = func() (*GPTAutoClassification, error) {
		return nil, &GPTAutoClassifierError{Class: GPTAutoErrUpstream}
	}
	for i := 0; i < gptAutoBreakerThreshold+3; i++ {
		_, err := svc.Route(context.Background(), gptAutoLiveInput(gptAutoLiveChatBody))
		require.NoError(t, err)
	}
	used, err := svc.LiveBudgetUsage(context.Background())
	require.NoError(t, err)
	require.Equal(t, classifier.calls.Load(), used, "open-circuit requests do not consume the live budget")
	require.Equal(t, int64(gptAutoBreakerThreshold), used)
}

func TestGPTAutoLiveUnconfiguredClassifierFallsBack(t *testing.T) {
	svc, _, classifier := newGPTAutoLiveTestService(t, func(c *GPTAutoShadowConfig) {
		c.ClassifierAPIKey = ""
	})
	route, err := svc.Route(context.Background(), gptAutoLiveInput(gptAutoLiveChatBody))
	require.NoError(t, err)
	require.Zero(t, classifier.calls.Load())
	require.True(t, IsGPTAutoModel(route.Model()))
}

// ---- session affinity ----------------------------------------------------

func TestGPTAutoLiveStatefulContinuationKeepsModel(t *testing.T) {
	svc, repo, classifier := newGPTAutoLiveTestService(t, nil)
	gptAutoClassifyAs(classifier, GPTAutoModelGPT61Sol, GPTAutoEffortHigh)
	first := gptAutoLiveInput(`{"model":"gpt-auto","input":"Plan a refactor"}`)
	first.Endpoint = GPTAutoEndpointResponses
	route, err := svc.Route(context.Background(), first)
	require.NoError(t, err)
	route.ObserveResult(&OpenAIForwardResult{ResponseID: "resp_abc"})
	route.Finish(context.Background(), GPTAutoRouteOutcome{HTTPStatus: 200, AccountID: 9})

	// Jev would now pick a different model; the continuation must not move.
	gptAutoClassifyAs(classifier, GPTAutoModelGPT56Luna, GPTAutoEffortLow)
	calls := classifier.calls.Load()
	next := gptAutoLiveInput(`{"model":"gpt-auto","previous_response_id":"resp_abc","input":[{"type":"function_call_output","call_id":"c1","output":"ok"}]}`)
	next.Endpoint = GPTAutoEndpointResponses
	route, err = svc.Route(context.Background(), next)
	require.NoError(t, err)
	require.Equal(t, GPTAutoModelGPT61Sol, route.Model())
	require.Equal(t, GPTAutoEffortHigh, route.Effort())
	require.Equal(t, calls, classifier.calls.Load(), "an affinity hit costs no Jev call")
	require.False(t, route.Reroute(GPTAutoFailureNoUpstream), "a stateful continuation never switches model")
	route.Finish(context.Background(), GPTAutoRouteOutcome{HTTPStatus: 200})
	repo.waitRows(t, 2)
	row := repo.lastRow(t)
	require.Equal(t, GPTAutoRouteAffinity, row.RoutingReason)
	require.Equal(t, GPTAutoAffinityHit, row.AffinityOutcome)

	// The same response id from another API key is a different scope.
	other := next
	other.APIKeyID = 99
	cfgWithOther := svc.settings.GetGPTAutoShadowConfigCached(context.Background())
	cfgWithOther.Live.AllowedAPIKeyIDs = append(cfgWithOther.Live.AllowedAPIKeyIDs, 99)
	require.NoError(t, svc.settings.SetGPTAutoShadowConfig(context.Background(), &cfgWithOther))
	route, err = svc.Route(context.Background(), other)
	require.NoError(t, err)
	require.Equal(t, GPTAutoModelGPT56Luna, route.Model(), "affinity never crosses API keys")
}

func TestGPTAutoLiveAffinityModelNoLongerPermittedIsRejected(t *testing.T) {
	svc, _, classifier := newGPTAutoLiveTestService(t, nil)
	gptAutoClassifyAs(classifier, GPTAutoModelGPT61Sol, GPTAutoEffortHigh)
	first := gptAutoLiveInput(`{"model":"gpt-auto","input":"Plan a refactor"}`)
	first.Endpoint = GPTAutoEndpointResponses
	route, err := svc.Route(context.Background(), first)
	require.NoError(t, err)
	route.ObserveResult(&OpenAIForwardResult{ResponseID: "resp_state"})
	route.Finish(context.Background(), GPTAutoRouteOutcome{HTTPStatus: 200})

	next := gptAutoLiveInput(`{"model":"gpt-auto","previous_response_id":"resp_state","input":"continue"}`)
	next.Endpoint = GPTAutoEndpointResponses
	next.ModelPermitted = func(model string) bool { return model != GPTAutoModelGPT61Sol }
	_, err = svc.Route(context.Background(), next)
	routeErr := gptAutoRouteErr(t, err)
	require.Equal(t, http.StatusConflict, routeErr.Status)
	require.Equal(t, "gpt_auto_session_model_unavailable", routeErr.Code)
}

func TestGPTAutoLiveSessionAffinityForChatSessions(t *testing.T) {
	svc, _, classifier := newGPTAutoLiveTestService(t, nil)
	gptAutoClassifyAs(classifier, GPTAutoModelGPT61Sol, GPTAutoEffortMedium)
	input := gptAutoLiveInput(gptAutoLiveChatBody)
	input.SessionKey = "agent-session-1"
	route, err := svc.Route(context.Background(), input)
	require.NoError(t, err)
	route.Finish(context.Background(), GPTAutoRouteOutcome{HTTPStatus: 200})

	gptAutoClassifyAs(classifier, GPTAutoModelGPT56Luna, GPTAutoEffortLow)
	route, err = svc.Route(context.Background(), input)
	require.NoError(t, err)
	require.Equal(t, GPTAutoModelGPT61Sol, route.Model(), "an agent session stays on one model")

	// Failed requests do not create affinity.
	input.SessionKey = "agent-session-2"
	route, err = svc.Route(context.Background(), input)
	require.NoError(t, err)
	route.Finish(context.Background(), GPTAutoRouteOutcome{HTTPStatus: 502})
	gptAutoClassifyAs(classifier, GPTAutoModelGPT6Luna, GPTAutoEffortLow)
	route, err = svc.Route(context.Background(), input)
	require.NoError(t, err)
	require.Equal(t, GPTAutoModelGPT6Luna, route.Model())
}

func TestGPTAutoLiveAffinityDisabled(t *testing.T) {
	svc, _, classifier := newGPTAutoLiveTestService(t, func(c *GPTAutoShadowConfig) {
		c.Live.SessionAffinityEnabled = false
	})
	input := gptAutoLiveInput(gptAutoLiveChatBody)
	input.SessionKey = "s"
	gptAutoClassifyAs(classifier, GPTAutoModelGPT61Sol, GPTAutoEffortMedium)
	route, _ := svc.Route(context.Background(), input)
	route.Finish(context.Background(), GPTAutoRouteOutcome{HTTPStatus: 200})
	gptAutoClassifyAs(classifier, GPTAutoModelGPT56Luna, GPTAutoEffortLow)
	route, _ = svc.Route(context.Background(), input)
	require.Equal(t, GPTAutoModelGPT56Luna, route.Model())
}

func TestGPTAutoAffinityKeyIsScopedAndOpaque(t *testing.T) {
	a := gptAutoAffinityKey(1, "resp", "resp_x")
	require.NotEqual(t, a, gptAutoAffinityKey(2, "resp", "resp_x"))
	require.NotEqual(t, a, gptAutoAffinityKey(1, "sess", "resp_x"))
	require.NotContains(t, a, "resp_x")
	require.Len(t, a, 64)
}

// ---- rerouting, margin guard, billing --------------------------------------

func TestGPTAutoLiveRerouteMovesToNextEligibleModel(t *testing.T) {
	svc, repo, classifier := newGPTAutoLiveTestService(t, nil)
	classifier.result = func() (*GPTAutoClassification, error) {
		cls := gptAutoSampleClassification(GPTAutoModelGPT61Sol, GPTAutoEffortHigh)
		cls.ModelProbabilities = map[string]float64{GPTAutoModelGPT61Sol: 0.6, GPTAutoModelGPT6Luna: 0.3, GPTAutoModelGPT56Luna: 0.1}
		return cls, nil
	}
	route, err := svc.Route(context.Background(), gptAutoLiveInput(gptAutoLiveChatBody))
	require.NoError(t, err)
	require.True(t, route.Reroute(GPTAutoFailureNoUpstream))
	require.Equal(t, GPTAutoModelGPT6Luna, route.Model())
	require.True(t, route.Reroute(GPTAutoFailureNoUpstream))
	require.Equal(t, GPTAutoModelGPT56Luna, route.Model())
	require.False(t, route.Reroute(GPTAutoFailureNoUpstream), "no candidates left")
	require.Equal(t, GPTAutoModelGPT56Luna, gjson.GetBytes(route.Body(), "model").String(), "the body follows the reroute")
	route.Finish(context.Background(), GPTAutoRouteOutcome{HTTPStatus: 200})
	repo.waitRows(t, 1)
	row := repo.lastRow(t)
	require.Equal(t, 2, row.RetryCount)
	require.Equal(t, GPTAutoModelGPT56Luna, row.FinalModel)
	require.Equal(t, GPTAutoFailureNoUpstream, row.FallbackReason)
}

func TestGPTAutoLiveMarginGuard(t *testing.T) {
	lossy := func(c *GPTAutoShadowConfig) {
		c.Live.MarginGuardEnabled = true
		for i := range c.ModelProfiles {
			if c.ModelProfiles[i].Model == GPTAutoModelGPT61Sol {
				// Procurement far above the $2/$10 list price.
				c.ModelProfiles[i].UpstreamCosts = []GPTAutoUpstreamCost{{Channel: "x", Enabled: true, InputPerM: 50, OutputPerM: 200}}
			}
		}
	}
	svc, repo, classifier := newGPTAutoLiveTestService(t, lossy)
	repo.history = []GPTAutoOutputHistory{
		{Model: GPTAutoModelGPT61Sol, Effort: GPTAutoEffortHigh, AvgOutput: 500, Samples: 100},
		{Model: GPTAutoModelGPT6Luna, Effort: GPTAutoEffortHigh, AvgOutput: 500, Samples: 100},
		{Model: GPTAutoModelGPT56Luna, Effort: GPTAutoEffortHigh, AvgOutput: 500, Samples: 100},
	}
	svc.refreshOutputHistory()
	classifier.result = func() (*GPTAutoClassification, error) {
		cls := gptAutoSampleClassification(GPTAutoModelGPT61Sol, GPTAutoEffortHigh)
		cls.ModelProbabilities = map[string]float64{GPTAutoModelGPT61Sol: 0.6, GPTAutoModelGPT6Luna: 0.4}
		return cls, nil
	}
	route, err := svc.Route(context.Background(), gptAutoLiveInput(gptAutoLiveChatBody))
	require.NoError(t, err)
	require.Equal(t, GPTAutoModelGPT6Luna, route.Model(), "a loss-making choice is replaced when another model is eligible")

	// Disabled guard: the margin is a warning only.
	svc2, repo2, classifier2 := newGPTAutoLiveTestService(t, func(c *GPTAutoShadowConfig) {
		lossy(c)
		c.Live.MarginGuardEnabled = false
	})
	repo2.history = repo.history
	svc2.refreshOutputHistory()
	classifier2.result = classifier.result
	route, err = svc2.Route(context.Background(), gptAutoLiveInput(gptAutoLiveChatBody))
	require.NoError(t, err)
	require.Equal(t, GPTAutoModelGPT61Sol, route.Model())
}

func TestGPTAutoLiveBillingMultiplier(t *testing.T) {
	svc, _, classifier := newGPTAutoLiveTestService(t, nil)
	gptAutoClassifyAs(classifier, GPTAutoModelGPT61Sol, GPTAutoEffortHigh)
	route, err := svc.Route(context.Background(), gptAutoLiveInput(gptAutoLiveChatBody))
	require.NoError(t, err)
	require.Equal(t, 1.5, route.BillingMultiplier(), "the executed model's profile multiplier")
	require.True(t, route.Reroute(GPTAutoFailureNoUpstream))
	current := svc.settings.GetGPTAutoShadowConfigCached(context.Background())
	profile, _ := current.ProfileFor(route.Model())
	require.Equal(t, profile.BillingMultiplier, route.BillingMultiplier(), "rerouting re-prices on the model that runs")

	svc2, _, _ := newGPTAutoLiveTestService(t, func(c *GPTAutoShadowConfig) { c.Live.ApplyBillingMultiplier = false })
	route, err = svc2.Route(context.Background(), gptAutoLiveInput(gptAutoLiveChatBody))
	require.NoError(t, err)
	require.Equal(t, 1.0, route.BillingMultiplier())

	var none *GPTAutoRoute
	require.Equal(t, 1.0, none.BillingMultiplier(), "ordinary requests are never scaled")

	require.Equal(t, 0.0001500000, applyGPTAutoBillingMultiplier(0.0001, 1.5))
	require.Equal(t, 0.3, applyGPTAutoBillingMultiplier(0.1, 3), "decimal math, no 0.30000000000000004")
	require.Equal(t, 0.0, applyGPTAutoBillingMultiplier(0, 1.5))
}

func TestGPTAutoLiveOutcomeStatuses(t *testing.T) {
	cases := []struct {
		outcome GPTAutoRouteOutcome
		status  string
	}{
		{GPTAutoRouteOutcome{HTTPStatus: 200}, GPTAutoRequestSucceeded},
		{GPTAutoRouteOutcome{HTTPStatus: 502}, GPTAutoRequestFailed},
		{GPTAutoRouteOutcome{HTTPStatus: 200, StreamStarted: true, UpstreamFailed: true}, GPTAutoRequestFailed},
		{GPTAutoRouteOutcome{HTTPStatus: 200, ClientGone: true}, GPTAutoRequestCancelled},
		{GPTAutoRouteOutcome{HTTPStatus: 499, ClientGone: true, StreamStarted: true}, GPTAutoRequestCancelled},
	}
	for _, tc := range cases {
		svc, repo, _ := newGPTAutoLiveTestService(t, nil)
		route, err := svc.Route(context.Background(), gptAutoLiveInput(gptAutoLiveChatBody))
		require.NoError(t, err)
		route.Finish(context.Background(), tc.outcome)
		route.Finish(context.Background(), tc.outcome) // idempotent
		repo.waitRows(t, 1)
		require.Equal(t, 1, repo.count())
		require.Equal(t, tc.status, repo.lastRow(t).RequestStatus)
	}
}

func TestGPTAutoLiveRowsNeverStorePrompts(t *testing.T) {
	svc, repo, _ := newGPTAutoLiveTestService(t, nil)
	marker := "UNIQUE-PROMPT-MARKER-7f3a"
	route, err := svc.Route(context.Background(), gptAutoLiveInput(`{"model":"gpt-auto","messages":[{"role":"user","content":"`+marker+` dev@example.com"}]}`))
	require.NoError(t, err)
	route.Finish(context.Background(), GPTAutoRouteOutcome{HTTPStatus: 200})
	repo.waitRows(t, 1)
	raw, err := json.Marshal(repo.lastRow(t))
	require.NoError(t, err)
	require.NotContains(t, string(raw), marker)
	require.NotContains(t, string(raw), "dev@example.com")
}

func TestGPTAutoLiveConfigValidation(t *testing.T) {
	for name, mutate := range map[string]func(*GPTAutoLiveConfig){
		"timeout":        func(c *GPTAutoLiveConfig) { c.ClassifierTimeoutMS = 10 },
		"concurrency":    func(c *GPTAutoLiveConfig) { c.ClassifierMaxConcurrency = 0 },
		"policy":         func(c *GPTAutoLiveConfig) { c.ClassifierFailurePolicy = "retry_forever" },
		"fallback model": func(c *GPTAutoLiveConfig) { c.FallbackModel = GPTAutoModelGPT56Sol },
		"effort policy":  func(c *GPTAutoLiveConfig) { c.ClientEffortPolicy = "client_wins" },
		"budget":         func(c *GPTAutoLiveConfig) { c.DailyRequestBudget = -1 },
		"ids":            func(c *GPTAutoLiveConfig) { c.AllowedAPIKeyIDs = []int64{0} },
		"endpoint":       func(c *GPTAutoLiveConfig) { c.Endpoints = []string{"messages"} },
	} {
		t.Run(name, func(t *testing.T) {
			cfg := DefaultGPTAutoLiveConfig()
			mutate(&cfg)
			require.Error(t, ValidateGPTAutoLiveConfig(cfg))
		})
	}
	require.NoError(t, ValidateGPTAutoLiveConfig(DefaultGPTAutoLiveConfig()))
}
