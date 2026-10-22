package admin

import (
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/pkg/typesafe"
	"github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"

	"github.com/gin-gonic/gin"
)

// GPTAutoHandler serves the GPT-Auto shadow routing admin APIs. Phase 1 is
// observation only: none of these endpoints can change live routing or
// billing.
type GPTAutoHandler struct {
	settingService *service.SettingService
	shadowService  *service.GPTAutoShadowService
}

// NewGPTAutoHandler creates the handler.
func NewGPTAutoHandler(settingService *service.SettingService, shadowService *service.GPTAutoShadowService) *GPTAutoHandler {
	return &GPTAutoHandler{settingService: settingService, shadowService: shadowService}
}

// gptAutoConfigResponse never carries the classifier API key.
type gptAutoConfigResponse struct {
	service.GPTAutoShadowConfig
	ClassifierAPIKeyConfigured bool   `json:"classifier_api_key_configured"`
	ClassifierAPIKeyMasked     string `json:"classifier_api_key_masked"`
}

func toGPTAutoConfigResponse(cfg *service.GPTAutoShadowConfig) gptAutoConfigResponse {
	out := gptAutoConfigResponse{GPTAutoShadowConfig: *cfg}
	key := strings.TrimSpace(cfg.ClassifierAPIKey)
	out.ClassifierAPIKeyConfigured = key != ""
	out.ClassifierAPIKeyMasked = maskGPTAutoKey(key)
	out.ClassifierAPIKey = ""
	return out
}

const gptAutoKeyMaskMarker = "****"

func maskGPTAutoKey(key string) string {
	if key == "" {
		return ""
	}
	if len(key) <= 8 {
		return gptAutoKeyMaskMarker
	}
	return key[:4] + gptAutoKeyMaskMarker + key[len(key)-4:]
}

// sameGPTAutoClassifierHost compares scheme and host; an empty URL means the
// default TypeSafe endpoint.
func sameGPTAutoClassifierHost(a, b string) bool {
	normalize := func(raw string) string {
		raw = strings.TrimSpace(raw)
		if raw == "" {
			raw = typesafe.DefaultBaseURL
		}
		parsed, err := url.Parse(raw)
		if err != nil {
			return raw
		}
		return strings.ToLower(parsed.Scheme + "://" + parsed.Host)
	}
	return normalize(a) == normalize(b)
}

// gptAutoURLHost returns the lower-cased hostname (no port) of a URL.
func gptAutoURLHost(raw string) string {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return ""
	}
	return strings.ToLower(parsed.Hostname())
}

// GetConfig returns the shadow configuration.
// GET /api/v1/admin/gpt-auto/config
func (h *GPTAutoHandler) GetConfig(c *gin.Context) {
	cfg, err := h.settingService.GetGPTAutoShadowConfig(c.Request.Context())
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, toGPTAutoConfigResponse(cfg))
}

// updateGPTAutoConfigRequest mirrors the config. An empty classifier_api_key
// keeps the stored key; clear_classifier_api_key removes it.
type updateGPTAutoConfigRequest struct {
	service.GPTAutoShadowConfig
	ClearClassifierAPIKey bool `json:"clear_classifier_api_key"`
}

// UpdateConfig validates and persists the configuration, then re-reads it.
// PUT /api/v1/admin/gpt-auto/config
func (h *GPTAutoHandler) UpdateConfig(c *gin.Context) {
	var req updateGPTAutoConfigRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}
	ctx := c.Request.Context()
	current, err := h.settingService.GetGPTAutoShadowConfig(ctx)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	next := req.GPTAutoShadowConfig
	submitted := strings.TrimSpace(next.ClassifierAPIKey)
	switch {
	case req.ClearClassifierAPIKey:
		next.ClassifierAPIKey = ""
	case submitted == "" || (current.ClassifierAPIKey != "" && submitted == maskGPTAutoKey(current.ClassifierAPIKey)):
		// Empty or the masked hint echoed back: keep the stored key.
		next.ClassifierAPIKey = current.ClassifierAPIKey
		submitted = ""
	case strings.Contains(submitted, gptAutoKeyMaskMarker):
		response.BadRequest(c, "classifier_api_key looks like a masked value; enter the full key or leave it empty")
		return
	}
	// The stored key must never follow a base-URL change to a new host: an
	// edited URL could otherwise exfiltrate the credential.
	if submitted == "" && next.ClassifierAPIKey != "" && !req.ClearClassifierAPIKey &&
		!sameGPTAutoClassifierHost(current.ClassifierBaseURL, next.ClassifierBaseURL) {
		response.BadRequest(c, "re-enter classifier_api_key when changing the classifier host")
		return
	}
	// Jev must be called directly, never through this gateway's own public,
	// customer-billable endpoints.
	ownURLs, err := h.settingService.GatewayPublicURLs(ctx)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	for _, own := range ownURLs {
		if host := gptAutoURLHost(own); host != "" && host == gptAutoURLHost(next.ClassifierBaseURL) {
			response.BadRequest(c, "classifier_base_url must not point at this gateway")
			return
		}
	}
	// Classifier instructions have their own versioned, audited endpoint; a
	// general config save (possibly from a stale form) never changes them.
	next.ClassifierInstructions = current.ClassifierInstructions
	if err := h.settingService.SetGPTAutoShadowConfig(ctx, &next); err != nil {
		response.ErrorFrom(c, err)
		return
	}
	saved, err := h.settingService.GetGPTAutoShadowConfig(ctx)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	// Model descriptions are part of the Jev questions: record the resulting
	// classifier version with its author.
	h.shadowService.RecordClassifierConfig(ctx, gptAutoAdminID(c))
	response.Success(c, toGPTAutoConfigResponse(saved))
}

// gptAutoAdminID returns the authenticated admin's user id, if any.
func gptAutoAdminID(c *gin.Context) *int64 {
	subject, ok := middleware.GetAuthSubjectFromContext(c)
	if !ok || subject.UserID <= 0 {
		return nil
	}
	id := subject.UserID
	return &id
}

// GetClassifier returns the Jev classification instructions, defaults,
// effective questions and version history.
// GET /api/v1/admin/gpt-auto/classifier
func (h *GPTAutoHandler) GetClassifier(c *gin.Context) {
	view, err := h.shadowService.ClassifierView(c.Request.Context())
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, view)
}

// UpdateClassifier saves new instructions (validated, versioned, audited).
// PUT /api/v1/admin/gpt-auto/classifier
func (h *GPTAutoHandler) UpdateClassifier(c *gin.Context) {
	var req service.GPTAutoClassifierInstructions
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}
	view, err := h.shadowService.SaveClassifierInstructions(c.Request.Context(), req, gptAutoAdminID(c))
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, view)
}

// PreviewClassifier returns the effective questions for a draft without
// calling Jev or saving anything.
// POST /api/v1/admin/gpt-auto/classifier/preview
func (h *GPTAutoHandler) PreviewClassifier(c *gin.Context) {
	var req service.GPTAutoClassifierInstructions
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}
	preview, err := h.shadowService.PreviewClassifier(c.Request.Context(), req)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, preview)
}

// DebugState shows the exact Jev state and questions for an administrator's
// synthetic request, with truncation metadata. No Jev call, nothing stored.
// POST /api/v1/admin/gpt-auto/debug/state
func (h *GPTAutoHandler) DebugState(c *gin.Context) {
	var req service.GPTAutoDebugInput
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}
	result, err := h.shadowService.DebugState(c.Request.Context(), req)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, result)
}

// CompareClassifier runs one sample through the saved and draft instructions
// (two separate, budgeted Jev calls). The draft is not saved.
// POST /api/v1/admin/gpt-auto/test/compare
func (h *GPTAutoHandler) CompareClassifier(c *gin.Context) {
	var req service.GPTAutoCompareInput
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}
	result, err := h.shadowService.CompareClassifierInstructions(c.Request.Context(), req)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, result)
}

type gptAutoModelMeta struct {
	Model                  string   `json:"model"`
	NativeSupportedEfforts []string `json:"native_supported_efforts"`
}

// Meta returns the fixed Phase 1 vocabularies so the UI never hard-codes them.
// GET /api/v1/admin/gpt-auto/meta
func (h *GPTAutoHandler) Meta(c *gin.Context) {
	models := make([]gptAutoModelMeta, 0, len(service.GPTAutoModelPool()))
	for _, profile := range service.DefaultGPTAutoModelProfiles() {
		models = append(models, gptAutoModelMeta{Model: profile.Model, NativeSupportedEfforts: profile.SupportedEfforts})
	}
	response.Success(c, gin.H{
		"models":              models,
		"efforts":             service.GPTAutoEffortLadder(),
		"categories":          service.GPTAutoCategories(),
		"endpoints":           service.GPTAutoSupportedEndpoints(),
		"default_profiles":    service.DefaultGPTAutoModelProfiles(),
		"policy_version":      service.GPTAutoPolicyVersionDefault,
		"classifier_schema":   service.GPTAutoClassifierSchemaVersion,
		"state_version":       service.GPTAutoClassifierStateVersion,
		"diagnostics_version": service.GPTAutoContextDiagnosticsVersion,
		"context_strategies":  service.GPTAutoContextStrategies(),
		"benchmark_samples":   service.GPTAutoBenchmarkSamples(),
		"phase":               "shadow",
		"live_routing_active": false,
	})
}

func parseGPTAutoTime(c *gin.Context, key string) (*time.Time, bool) {
	raw := strings.TrimSpace(c.Query(key))
	if raw == "" {
		return nil, true
	}
	parsed, err := time.Parse(time.RFC3339, raw)
	if err != nil {
		response.BadRequest(c, "Invalid "+key+", expect RFC3339")
		return nil, false
	}
	return &parsed, true
}

func parseGPTAutoGroupID(c *gin.Context) (*int64, bool) {
	raw := strings.TrimSpace(c.Query("group_id"))
	if raw == "" {
		return nil, true
	}
	id, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || id <= 0 {
		response.BadRequest(c, "Invalid group_id")
		return nil, false
	}
	return &id, true
}

const gptAutoMaxStatsWindow = 90 * 24 * time.Hour

// Overview returns dashboard statistics built from stored decisions plus this
// node's runtime counters.
// GET /api/v1/admin/gpt-auto/overview
func (h *GPTAutoHandler) Overview(c *gin.Context) {
	start, ok := parseGPTAutoTime(c, "start_time")
	if !ok {
		return
	}
	end, ok := parseGPTAutoTime(c, "end_time")
	if !ok {
		return
	}
	groupID, ok := parseGPTAutoGroupID(c)
	if !ok {
		return
	}
	filter := service.GPTAutoStatsFilter{EndTime: time.Now().UTC(), GroupID: groupID}
	if end != nil {
		filter.EndTime = end.UTC()
	}
	filter.StartTime = filter.EndTime.Add(-24 * time.Hour)
	if start != nil {
		filter.StartTime = start.UTC()
	}
	if !filter.StartTime.Before(filter.EndTime) || filter.EndTime.Sub(filter.StartTime) > gptAutoMaxStatsWindow {
		response.BadRequest(c, "Invalid time range (max 90 days)")
		return
	}
	overview, err := h.shadowService.Overview(c.Request.Context(), filter)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, overview)
}

// ListDecisions pages the shadow decision log.
// GET /api/v1/admin/gpt-auto/decisions
func (h *GPTAutoHandler) ListDecisions(c *gin.Context) {
	page, pageSize := response.ParsePagination(c)
	if pageSize > 200 {
		pageSize = 200
	}
	start, ok := parseGPTAutoTime(c, "start_time")
	if !ok {
		return
	}
	end, ok := parseGPTAutoTime(c, "end_time")
	if !ok {
		return
	}
	groupID, ok := parseGPTAutoGroupID(c)
	if !ok {
		return
	}
	filter := service.GPTAutoDecisionFilter{
		Pagination:       pagination.PaginationParams{Page: page, PageSize: pageSize},
		StartTime:        start,
		EndTime:          end,
		ActualModel:      strings.TrimSpace(c.Query("actual_model")),
		FinalModel:       strings.TrimSpace(c.Query("final_model")),
		FinalEffort:      strings.TrimSpace(c.Query("final_effort")),
		EvaluationStatus: strings.TrimSpace(c.Query("evaluation_status")),
		DecisionStatus:   strings.TrimSpace(c.Query("decision_status")),
		GroupID:          groupID,
		RequestID:        strings.TrimSpace(c.Query("request_id")),
	}
	for _, value := range []string{filter.ActualModel, filter.FinalModel, filter.RequestID} {
		if len(value) > 128 {
			response.BadRequest(c, "Filter value too long")
			return
		}
	}
	if filter.FinalEffort != "" && !service.IsGPTAutoEffort(filter.FinalEffort) {
		response.BadRequest(c, "Invalid final_effort")
		return
	}
	switch filter.EvaluationStatus {
	case "", service.GPTAutoEvaluationSuccess, service.GPTAutoEvaluationFailed:
	default:
		response.BadRequest(c, "Invalid evaluation_status")
		return
	}
	switch filter.DecisionStatus {
	case "", service.GPTAutoDecisionAccepted, service.GPTAutoDecisionAdjusted, service.GPTAutoDecisionNoEligible:
	default:
		response.BadRequest(c, "Invalid decision_status")
		return
	}
	items, result, err := h.shadowService.ListDecisions(c.Request.Context(), filter)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Paginated(c, items, result.Total, result.Page, result.PageSize)
}

func parseGPTAutoDecisionID(c *gin.Context) (int64, bool) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		response.BadRequest(c, "Invalid decision id")
		return 0, false
	}
	return id, true
}

// GetDecision returns one decision including Jev output and policy details.
// GET /api/v1/admin/gpt-auto/decisions/:id
func (h *GPTAutoHandler) GetDecision(c *gin.Context) {
	id, ok := parseGPTAutoDecisionID(c)
	if !ok {
		return
	}
	decision, err := h.shadowService.GetDecision(c.Request.Context(), id)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, decision)
}

// ReplayDecision re-runs the policy on a stored Jev result without calling Jev.
// POST /api/v1/admin/gpt-auto/decisions/:id/replay
func (h *GPTAutoHandler) ReplayDecision(c *gin.Context) {
	id, ok := parseGPTAutoDecisionID(c)
	if !ok {
		return
	}
	var req service.GPTAutoReplayInput
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}
	result, err := h.shadowService.Replay(c.Request.Context(), id, req)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, result)
}

// RunTest classifies an admin-supplied sample with Jev. It never calls a GPT
// model and never stores the sample.
// POST /api/v1/admin/gpt-auto/test
func (h *GPTAutoHandler) RunTest(c *gin.Context) {
	var req service.GPTAutoTestInput
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}
	result, err := h.shadowService.RunTest(c.Request.Context(), req)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, result)
}

// Simulate computes hypothetical economics for unsaved profile edits.
// POST /api/v1/admin/gpt-auto/simulate
func (h *GPTAutoHandler) Simulate(c *gin.Context) {
	var req service.GPTAutoSimulationInput
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}
	result, err := h.shadowService.Simulate(c.Request.Context(), req)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, result)
}
