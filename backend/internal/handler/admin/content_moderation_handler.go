package admin

import (
	"encoding/json"
	"fmt"
	"io"
	"strconv"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
)

type ContentModerationHandler struct {
	service  *service.ContentModerationService
	settings *service.SettingService
}

func NewContentModerationHandler(svc *service.ContentModerationService, settings *service.SettingService) *ContentModerationHandler {
	return &ContentModerationHandler{service: svc, settings: settings}
}

type contentModerationConfigRequest struct {
	Enabled             *bool               `json:"enabled"`
	Mode                *string             `json:"mode"`
	BaseURL             *string             `json:"base_url"`
	Model               *string             `json:"model"`
	GroupModelOverrides *map[int64]string   `json:"group_model_overrides"`
	GroupModelFilters   *map[int64][]string `json:"group_model_filters"`
	// 审计请求使用的代理服务器：null 不修改；0 清除（直连）；>0 指定代理。
	ProxyID              *int64              `json:"proxy_id"`
	APIKey               *string             `json:"api_key"`
	APIKeys              *[]string           `json:"api_keys"`
	APIKeysMode          string              `json:"api_keys_mode"`
	DeleteAPIKeyHashes   *[]string           `json:"delete_api_key_hashes"`
	ClearAPIKey          bool                `json:"clear_api_key"`
	TimeoutMS            *int                `json:"timeout_ms"`
	SampleRate           *int                `json:"sample_rate"`
	AllGroups            *bool               `json:"all_groups"`
	GroupIDs             *[]int64            `json:"group_ids"`
	RecordNonHits        *bool               `json:"record_non_hits"`
	Thresholds           *map[string]float64 `json:"thresholds"`
	WorkerCount          *int                `json:"worker_count"`
	QueueSize            *int                `json:"queue_size"`
	BlockStatus          *int                `json:"block_status"`
	BlockMessage         *string             `json:"block_message"`
	EmailOnHit           *bool               `json:"email_on_hit"`
	AutoBanEnabled       *bool               `json:"auto_ban_enabled"`
	BanType              *string             `json:"ban_type"`
	BanDurationHours     *int                `json:"ban_duration_hours"`
	BanThreshold         *int                `json:"ban_threshold"`
	ViolationWindowHours *int                `json:"violation_window_hours"`
	// cyber_policy 命中是否排除出自动封号计数；前端 RiskControlView 已发送该字段，
	// service.UpdateContentModerationConfigInput 已支持，此前 handler 层缺透传导致开关静默失效。
	CyberPolicyExcludeFromBanCount *bool                                 `json:"cyber_policy_exclude_from_ban_count"`
	CyberPolicyGroupBanEnabled     *bool                                 `json:"cyber_policy_group_ban_enabled"`
	CyberPolicyTriggerGroupIDs     *[]int64                              `json:"cyber_policy_trigger_group_ids"`
	CyberPolicyTargetGroupIDs      *[]int64                              `json:"cyber_policy_target_group_ids"`
	RetryCount                     *int                                  `json:"retry_count"`
	HitRetentionDays               *int                                  `json:"hit_retention_days"`
	NonHitRetentionDays            *int                                  `json:"non_hit_retention_days"`
	PreHashCheckEnabled            *bool                                 `json:"pre_hash_check_enabled"`
	BlockedKeywords                *[]string                             `json:"blocked_keywords"`
	KeywordBlockingMode            *string                               `json:"keyword_blocking_mode"`
	ModelFilter                    *service.ContentModerationModelFilter `json:"model_filter"`
}

type contentModerationAPIKeyTestRequest struct {
	APIKeys   []string `json:"api_keys"`
	BaseURL   string   `json:"base_url"`
	Model     string   `json:"model"`
	TimeoutMS int      `json:"timeout_ms"`
	ProxyID   *int64   `json:"proxy_id"`
	Prompt    string   `json:"prompt"`
	Images    []string `json:"images"`
}

type contentModerationHashRequest struct {
	InputHash string `json:"input_hash"`
}

const maxRiskControlJSONBodyBytes = 2 << 20

// bindRiskControlJSON tolerates legacy clients that submitted an unescaped
// control byte inside a text field while retaining standard JSON decoding.
func bindRiskControlJSON(c *gin.Context, target any) error {
	if c == nil || c.Request == nil || c.Request.Body == nil {
		return io.EOF
	}
	raw, err := io.ReadAll(io.LimitReader(c.Request.Body, maxRiskControlJSONBodyBytes+1))
	if err != nil {
		return err
	}
	if len(raw) > maxRiskControlJSONBodyBytes {
		return fmt.Errorf("request body exceeds %d bytes", maxRiskControlJSONBodyBytes)
	}
	for i, char := range raw {
		if char < 0x20 && char != '\t' && char != '\r' && char != '\n' {
			raw[i] = ' '
		}
	}
	return json.Unmarshal(raw, target)
}

func (h *ContentModerationHandler) GetConfig(c *gin.Context) {
	cfg, err := h.service.GetConfig(c.Request.Context())
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, cfg)
}

func (h *ContentModerationHandler) GetAntiAbuseConfig(c *gin.Context) {
	if h.settings == nil {
		response.Success(c, service.AntiAbuseConfigView{})
		return
	}
	response.Success(c, h.settings.GetAntiAbuseConfig(c.Request.Context()))
}

type antiAbuseConfigRequest struct {
	Enabled                             bool    `json:"enabled"`
	ScoreThreshold                      int     `json:"score_threshold"`
	FingerprintWeight                   int     `json:"fingerprint_weight"`
	IPWeight                            int     `json:"ip_weight"`
	EmailWeight                         int     `json:"email_weight"`
	UserAgentWeight                     int     `json:"user_agent_weight"`
	TLSFingerprintWeight                int     `json:"tls_fingerprint_weight"`
	SignupIPRiskControlThreshold        *int    `json:"signup_ip_risk_control_threshold"`
	SignupIPDisablePreviousAccounts     *bool   `json:"signup_ip_disable_previous_accounts"`
	SignupIPKeepPreviousAccounts        *int    `json:"signup_ip_keep_previous_accounts"`
	APIUsageIPUARiskControlThreshold    *int    `json:"api_usage_ip_ua_risk_control_threshold"`
	APIUsageIPUADisablePreviousAccounts *bool   `json:"api_usage_ip_ua_disable_previous_accounts"`
	APIUsageIPUAKeepPreviousAccounts    *int    `json:"api_usage_ip_ua_keep_previous_accounts"`
	IPReputationEndpoint                string  `json:"ip_reputation_endpoint"`
	IPReputationAPIKey                  *string `json:"ip_reputation_api_key"`
}

func (h *ContentModerationHandler) UpdateAntiAbuseConfig(c *gin.Context) {
	if h.settings == nil {
		response.InternalError(c, "setting service not configured")
		return
	}
	var req antiAbuseConfigRequest
	if err := bindRiskControlJSON(c, &req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}
	endpoint := strings.TrimSpace(req.IPReputationEndpoint)
	if endpoint != "" {
		if err := config.ValidateAbsoluteHTTPURL(endpoint); err != nil {
			response.BadRequest(c, "ip_reputation_endpoint must be an absolute HTTP(S) URL")
			return
		}
	}
	current := h.settings.GetAntiAbuseConfig(c.Request.Context())
	signupThreshold := current.SignupIPRiskControlThreshold
	if req.SignupIPRiskControlThreshold != nil {
		signupThreshold = *req.SignupIPRiskControlThreshold
	}
	signupDisablePrevious := current.SignupIPDisablePreviousAccounts
	if req.SignupIPDisablePreviousAccounts != nil {
		signupDisablePrevious = *req.SignupIPDisablePreviousAccounts
	}
	signupKeepPrevious := current.SignupIPKeepPreviousAccounts
	if req.SignupIPKeepPreviousAccounts != nil {
		signupKeepPrevious = *req.SignupIPKeepPreviousAccounts
	}
	apiThreshold := current.APIUsageIPUARiskControlThreshold
	if req.APIUsageIPUARiskControlThreshold != nil {
		apiThreshold = *req.APIUsageIPUARiskControlThreshold
	}
	apiDisablePrevious := current.APIUsageIPUADisablePreviousAccounts
	if req.APIUsageIPUADisablePreviousAccounts != nil {
		apiDisablePrevious = *req.APIUsageIPUADisablePreviousAccounts
	}
	apiKeepPrevious := current.APIUsageIPUAKeepPreviousAccounts
	if req.APIUsageIPUAKeepPreviousAccounts != nil {
		apiKeepPrevious = *req.APIUsageIPUAKeepPreviousAccounts
	}
	view, err := h.settings.UpdateAntiAbuseConfig(c.Request.Context(), service.UpdateAntiAbuseConfigInput{
		Enabled: req.Enabled, ScoreThreshold: req.ScoreThreshold, FingerprintWeight: req.FingerprintWeight,
		IPWeight: req.IPWeight, EmailWeight: req.EmailWeight, UserAgentWeight: req.UserAgentWeight,
		TLSFingerprintWeight: req.TLSFingerprintWeight, IPReputationEndpoint: endpoint, IPReputationAPIKey: req.IPReputationAPIKey,
		SignupIPRiskControlThreshold:        signupThreshold,
		SignupIPDisablePreviousAccounts:     signupDisablePrevious,
		SignupIPKeepPreviousAccounts:        signupKeepPrevious,
		APIUsageIPUARiskControlThreshold:    apiThreshold,
		APIUsageIPUADisablePreviousAccounts: apiDisablePrevious,
		APIUsageIPUAKeepPreviousAccounts:    apiKeepPrevious,
	})
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, view)
}

func (h *ContentModerationHandler) UpdateConfig(c *gin.Context) {
	var req contentModerationConfigRequest
	if err := bindRiskControlJSON(c, &req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}
	cfg, err := h.service.UpdateConfig(c.Request.Context(), service.UpdateContentModerationConfigInput{
		Enabled:                        req.Enabled,
		Mode:                           req.Mode,
		BaseURL:                        req.BaseURL,
		Model:                          req.Model,
		GroupModelOverrides:            req.GroupModelOverrides,
		GroupModelFilters:              req.GroupModelFilters,
		ProxyID:                        req.ProxyID,
		APIKey:                         req.APIKey,
		APIKeys:                        req.APIKeys,
		APIKeysMode:                    req.APIKeysMode,
		DeleteAPIKeyHashes:             req.DeleteAPIKeyHashes,
		ClearAPIKey:                    req.ClearAPIKey,
		TimeoutMS:                      req.TimeoutMS,
		SampleRate:                     req.SampleRate,
		AllGroups:                      req.AllGroups,
		GroupIDs:                       req.GroupIDs,
		RecordNonHits:                  req.RecordNonHits,
		Thresholds:                     req.Thresholds,
		WorkerCount:                    req.WorkerCount,
		QueueSize:                      req.QueueSize,
		BlockStatus:                    req.BlockStatus,
		BlockMessage:                   req.BlockMessage,
		EmailOnHit:                     req.EmailOnHit,
		AutoBanEnabled:                 req.AutoBanEnabled,
		BanType:                        req.BanType,
		BanDurationHours:               req.BanDurationHours,
		BanThreshold:                   req.BanThreshold,
		ViolationWindowHours:           req.ViolationWindowHours,
		CyberPolicyExcludeFromBanCount: req.CyberPolicyExcludeFromBanCount,
		CyberPolicyGroupBanEnabled:     req.CyberPolicyGroupBanEnabled,
		CyberPolicyTriggerGroupIDs:     req.CyberPolicyTriggerGroupIDs,
		CyberPolicyTargetGroupIDs:      req.CyberPolicyTargetGroupIDs,
		RetryCount:                     req.RetryCount,
		HitRetentionDays:               req.HitRetentionDays,
		NonHitRetentionDays:            req.NonHitRetentionDays,
		PreHashCheckEnabled:            req.PreHashCheckEnabled,
		BlockedKeywords:                req.BlockedKeywords,
		KeywordBlockingMode:            req.KeywordBlockingMode,
		ModelFilter:                    req.ModelFilter,
	})
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, cfg)
}

func (h *ContentModerationHandler) TestAPIKeys(c *gin.Context) {
	var req contentModerationAPIKeyTestRequest
	if err := bindRiskControlJSON(c, &req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}
	result, err := h.service.TestAPIKeys(c.Request.Context(), service.TestContentModerationAPIKeysInput{
		APIKeys:   req.APIKeys,
		BaseURL:   req.BaseURL,
		Model:     req.Model,
		TimeoutMS: req.TimeoutMS,
		ProxyID:   req.ProxyID,
		Prompt:    req.Prompt,
		Images:    req.Images,
	})
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, result)
}

func (h *ContentModerationHandler) GetStatus(c *gin.Context) {
	status, err := h.service.GetStatus(c.Request.Context())
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, status)
}

func (h *ContentModerationHandler) ListLogs(c *gin.Context) {
	page, pageSize := response.ParsePagination(c)
	filter := service.ContentModerationLogFilter{
		Pagination: pagination.PaginationParams{
			Page:      page,
			PageSize:  pageSize,
			SortOrder: pagination.SortOrderDesc,
		},
		Result:   c.Query("result"),
		Endpoint: c.Query("endpoint"),
		Search:   c.Query("search"),
	}
	if raw := strings.TrimSpace(c.Query("group_id")); raw != "" {
		groupID, err := strconv.ParseInt(raw, 10, 64)
		if err != nil || groupID <= 0 {
			response.BadRequest(c, "Invalid group_id")
			return
		}
		filter.GroupID = &groupID
	}
	if raw := strings.TrimSpace(c.Query("from")); raw != "" {
		t, _, err := parseContentModerationDate(raw)
		if err != nil {
			response.BadRequest(c, "Invalid from")
			return
		}
		filter.From = &t
	}
	if raw := strings.TrimSpace(c.Query("to")); raw != "" {
		t, dateOnly, err := parseContentModerationDate(raw)
		if err != nil {
			response.BadRequest(c, "Invalid to")
			return
		}
		if dateOnly {
			t = t.Add(24*time.Hour - time.Nanosecond)
		}
		filter.To = &t
	}
	items, pageResult, err := h.service.ListLogs(c.Request.Context(), filter)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Paginated(c, items, pageResult.Total, pageResult.Page, pageResult.PageSize)
}

func (h *ContentModerationHandler) ListAntiAbuseEvents(c *gin.Context) {
	c.Header("Cache-Control", "no-store, no-cache, must-revalidate, max-age=0")
	c.Header("Pragma", "no-cache")
	c.Header("Expires", "0")
	page, pageSize := response.ParsePagination(c)
	filter := service.AntiAbuseEventFilter{
		Pagination: pagination.PaginationParams{Page: page, PageSize: pageSize, SortOrder: pagination.SortOrderDesc},
		EventType:  c.Query("event_type"), Action: c.Query("action"), Search: c.Query("search"),
		DeductionsOnly: c.Query("deductions_only") == "true",
	}
	if raw := strings.TrimSpace(c.Query("from")); raw != "" {
		t, _, err := parseContentModerationDate(raw)
		if err != nil {
			response.BadRequest(c, "Invalid from")
			return
		}
		filter.From = &t
	}
	if raw := strings.TrimSpace(c.Query("to")); raw != "" {
		t, dateOnly, err := parseContentModerationDate(raw)
		if err != nil {
			response.BadRequest(c, "Invalid to")
			return
		}
		if dateOnly {
			t = t.Add(24*time.Hour - time.Nanosecond)
		}
		filter.To = &t
	}
	items, total, err := h.service.ListAntiAbuseEvents(c.Request.Context(), filter)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Paginated(c, items, total, page, pageSize)
}

func (h *ContentModerationHandler) UnbanUser(c *gin.Context) {
	userID, err := strconv.ParseInt(strings.TrimSpace(c.Param("user_id")), 10, 64)
	if err != nil || userID <= 0 {
		response.BadRequest(c, "Invalid user_id")
		return
	}
	if raw := strings.TrimSpace(c.Query("group_id")); raw != "" {
		groupID, parseErr := strconv.ParseInt(raw, 10, 64)
		if parseErr != nil || groupID <= 0 {
			response.BadRequest(c, "Invalid group_id")
			return
		}
		result, unbanErr := h.service.UnbanGroup(c.Request.Context(), userID, groupID)
		if unbanErr != nil {
			response.ErrorFrom(c, unbanErr)
			return
		}
		response.Success(c, result)
		return
	}
	result, err := h.service.UnbanUser(c.Request.Context(), userID)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, result)
}

func (h *ContentModerationHandler) DeleteFlaggedHash(c *gin.Context) {
	var req contentModerationHashRequest
	if err := bindRiskControlJSON(c, &req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}
	result, err := h.service.DeleteFlaggedInputHash(c.Request.Context(), req.InputHash)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, result)
}

func (h *ContentModerationHandler) ClearFlaggedHashes(c *gin.Context) {
	result, err := h.service.ClearFlaggedInputHashes(c.Request.Context())
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, result)
}

func parseContentModerationDate(raw string) (time.Time, bool, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return time.Time{}, false, nil
	}
	if t, err := time.Parse(time.RFC3339, raw); err == nil {
		return t, false, nil
	}
	t, err := time.Parse("2006-01-02", raw)
	return t, err == nil, err
}
