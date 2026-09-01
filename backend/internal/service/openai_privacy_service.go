package service

import (
	"context"
	"fmt"
	"log/slog"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/imroc/req/v3"
)

// PrivacyClientFactory creates an HTTP client for privacy API calls.
// Injected from repository layer to avoid import cycles.
type PrivacyClientFactory func(proxyURL string) (*req.Client, error)

const (
	openAISettingsURL = "https://chatgpt.com/backend-api/settings/account_user_setting"

	PrivacyModeTrainingOff = "training_off"
	PrivacyModeFailed      = "training_set_failed"
	PrivacyModeCFBlocked   = "training_set_cf_blocked"
)

func shouldSkipOpenAIPrivacyEnsure(extra map[string]any) bool {
	if extra == nil {
		return false
	}
	raw, ok := extra["privacy_mode"]
	if !ok {
		return false
	}
	mode, _ := raw.(string)
	mode = strings.TrimSpace(mode)
	return mode != PrivacyModeFailed && mode != PrivacyModeCFBlocked
}

// disableOpenAITraining calls ChatGPT settings API to turn off "Improve the model for everyone".
// Returns privacy_mode value: "training_off" on success, "cf_blocked" / "failed" on failure.
func disableOpenAITraining(ctx context.Context, clientFactory PrivacyClientFactory, accessToken, proxyURL string) string {
	if accessToken == "" || clientFactory == nil {
		return ""
	}

	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()

	client, err := clientFactory(proxyURL)
	if err != nil {
		slog.Warn("openai_privacy_client_error", "error", err.Error())
		return PrivacyModeFailed
	}

	resp, err := client.R().
		SetContext(ctx).
		SetHeader("Authorization", "Bearer "+accessToken).
		SetHeader("Origin", "https://chatgpt.com").
		SetHeader("Referer", "https://chatgpt.com/").
		SetHeader("Accept", "application/json").
		SetHeader("sec-fetch-mode", "cors").
		SetHeader("sec-fetch-site", "same-origin").
		SetHeader("sec-fetch-dest", "empty").
		SetQueryParam("feature", "training_allowed").
		SetQueryParam("value", "false").
		Patch(openAISettingsURL)

	if err != nil {
		slog.Warn("openai_privacy_request_error", "error", err.Error())
		return PrivacyModeFailed
	}

	if resp.StatusCode == 403 || resp.StatusCode == 503 {
		body := resp.String()
		if strings.Contains(body, "cloudflare") || strings.Contains(body, "cf-") || strings.Contains(body, "Just a moment") {
			slog.Warn("openai_privacy_cf_blocked", "status", resp.StatusCode)
			return PrivacyModeCFBlocked
		}
	}

	if !resp.IsSuccessState() {
		slog.Warn("openai_privacy_failed", "status", resp.StatusCode, "body", truncate(resp.String(), 200))
		return PrivacyModeFailed
	}

	slog.Info("openai_privacy_training_disabled")
	return PrivacyModeTrainingOff
}

// ChatGPTAccountInfo 从 chatgpt.com/backend-api/accounts/check 获取的账号信息
type ChatGPTAccountInfo struct {
	PlanType string
	Email    string
	// AccountID 是本条信息所属账号的标识（优先取 account.account_id，否则取 accounts
	// 的 map key）。accounts/check 是多账号/工作区端点，调用方需要据此判断拿到的
	// plan_type / expires_at 到底属于个人账号还是某个 workspace。
	AccountID                   string
	SubscriptionExpiresAt       string // entitlement.expires_at (RFC3339)
	WorkspaceName               string
	WorkspaceCreatedTime        string
	WorkspaceOrganizationID     string
	WorkspaceType               string
	IsTeamWorkspace             bool
	HasSelfServeBusinessProlite bool
}

var (
	chatGPTAccountsCheckURL    = "https://chatgpt.com/backend-api/accounts/check/v4-2023-04-27"
	chatGPTSubscriptionsURL    = "https://chatgpt.com/backend-api/subscriptions"
	chatGPTSeatTypeCountsURL   = "https://chatgpt.com/backend-api/accounts/%s/users/seat_type_counts"
	chatGPTWorkspaceInvitesURL = "https://chatgpt.com/backend-api/accounts/%s/invites"
	chatGPTWorkspaceUsersURL   = "https://chatgpt.com/backend-api/accounts/%s/users"
)

const chatGPTAccountsCheckTimezoneOffsetMin = "-540"

func setChatGPTBackendRequestHeaders(request *req.Request, accessToken, accountID, targetPath string) {
	request.
		SetHeader("Authorization", "Bearer "+accessToken).
		SetHeader("Origin", "https://chatgpt.com").
		SetHeader("Referer", "https://chatgpt.com/").
		SetHeader("Accept", "application/json").
		SetHeader("sec-fetch-mode", "cors").
		SetHeader("sec-fetch-site", "same-origin").
		SetHeader("sec-fetch-dest", "empty")
	if accountID = strings.TrimSpace(accountID); accountID != "" {
		request.SetHeader("chatgpt-account-id", accountID)
	}
	if targetPath = strings.TrimSpace(targetPath); targetPath != "" {
		request.SetHeader("x-openai-target-path", targetPath)
	}
}

// fetchChatGPTAccountInfo calls ChatGPT backend-api to get account info (plan_type, etc.).
// Used as fallback when id_token doesn't contain these fields (e.g., Mobile RT).
// orgID is used to match the correct account when multiple accounts exist (e.g., personal + team).
// Returns nil on any failure (best-effort, non-blocking).
func fetchChatGPTAccountInfo(ctx context.Context, clientFactory PrivacyClientFactory, accessToken, proxyURL, orgID string) *ChatGPTAccountInfo {
	if accessToken == "" || clientFactory == nil {
		return nil
	}

	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()

	client, err := clientFactory(proxyURL)
	if err != nil {
		slog.Debug("chatgpt_account_check_client_error", "error", err.Error())
		return nil
	}

	var result map[string]any
	request := client.R().SetContext(ctx)
	setChatGPTBackendRequestHeaders(request, accessToken, orgID, "/backend-api/accounts/check/v4-2023-04-27")
	resp, err := request.
		SetQueryParam("timezone_offset_min", chatGPTAccountsCheckTimezoneOffsetMin).
		SetSuccessResult(&result).
		Get(chatGPTAccountsCheckURL)

	if err != nil {
		slog.Debug("chatgpt_account_check_request_error", "error", err.Error())
		return nil
	}

	if !resp.IsSuccessState() {
		slog.Debug("chatgpt_account_check_failed", "status", resp.StatusCode, "body", truncate(resp.String(), 200))
		return nil
	}

	info := &ChatGPTAccountInfo{}

	accounts, ok := result["accounts"].(map[string]any)
	if !ok {
		slog.Debug("chatgpt_account_check_no_accounts", "body", truncate(resp.String(), 300))
		return nil
	}

	// 优先匹配 orgID 对应的账号（access_token JWT 中的 poid）
	if acct, key, ok := findChatGPTAccount(accounts, orgID); ok && isUsableChatGPTAccountCandidate(acct, time.Now()) {
		fillAccountInfo(info, acct, key)
	}

	// 未匹配到时，遍历所有账号：优先 is_default，次选非 free
	if info.PlanType == "" {
		type candidate struct {
			account    map[string]any
			accountKey string
			planType   string
			expiresAt  string
			accountID  string
		}
		var defaultC, paidC, anyC candidate
		for key, acctRaw := range accounts {
			acct, ok := acctRaw.(map[string]any)
			if !ok {
				continue
			}
			if !isUsableChatGPTAccountCandidate(acct, time.Now()) {
				continue
			}
			planType := extractPlanType(acct)
			if planType == "" {
				continue
			}
			ea := extractEntitlementExpiresAt(acct)
			id := chatGPTAccountObjectID(acct, key)
			if anyC.planType == "" {
				anyC = candidate{account: acct, accountKey: key, planType: planType, expiresAt: ea, accountID: id}
			}
			if account, ok := acct["account"].(map[string]any); ok {
				if isDefault, _ := account["is_default"].(bool); isDefault {
					defaultC = candidate{account: acct, accountKey: key, planType: planType, expiresAt: ea, accountID: id}
				}
			}
			if !strings.EqualFold(planType, "free") && paidC.planType == "" {
				paidC = candidate{account: acct, accountKey: key, planType: planType, expiresAt: ea, accountID: id}
			}
		}
		// 优先级：default > 非 free > 任意
		var selected candidate
		switch {
		case defaultC.planType != "":
			selected = defaultC
		case paidC.planType != "":
			selected = paidC
		default:
			selected = anyC
		}
		if selected.account != nil {
			fillAccountInfo(info, selected.account, selected.accountKey)
		}
	}

	if info.PlanType == "" {
		slog.Debug("chatgpt_account_check_no_plan_type", "body", truncate(resp.String(), 300))
		return nil
	}

	slog.Info("chatgpt_account_check_success", "plan_type", info.PlanType, "subscription_expires_at", info.SubscriptionExpiresAt, "org_id", orgID)
	return info
}

func findChatGPTAccount(accounts map[string]any, hint string) (map[string]any, string, bool) {
	hint = strings.TrimSpace(hint)
	if hint == "" {
		return nil, "", false
	}
	if raw, ok := accounts[hint]; ok {
		if acct, ok := raw.(map[string]any); ok {
			return acct, hint, true
		}
	}
	for key, raw := range accounts {
		acct, ok := raw.(map[string]any)
		if ok && chatGPTAccountMatchesHint(acct, hint) {
			return acct, key, true
		}
	}
	return nil, "", false
}

func chatGPTAccountMatchesHint(acct map[string]any, hint string) bool {
	account, ok := acct["account"].(map[string]any)
	if !ok {
		return false
	}
	for _, key := range []string{"account_id", "organization_id"} {
		value, _ := account[key].(string)
		if strings.EqualFold(strings.TrimSpace(value), hint) {
			return true
		}
	}
	return false
}

// fetchChatGPTSubscriptionExpiresAt reads the lightweight subscription endpoint used by
// ChatGPT/Codex clients. Some Plus accounts no longer expose entitlement.expires_at in
// accounts/check, but this endpoint still returns active_until.
func fetchChatGPTSubscriptionExpiresAt(ctx context.Context, clientFactory PrivacyClientFactory, accessToken, proxyURL, accountID string) string {
	accountID = strings.TrimSpace(accountID)
	if accessToken == "" || accountID == "" || clientFactory == nil {
		return ""
	}

	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()

	client, err := clientFactory(proxyURL)
	if err != nil {
		slog.Debug("chatgpt_subscription_client_error", "error", err.Error())
		return ""
	}

	var result struct {
		PlanType    string `json:"plan_type"`
		ActiveUntil string `json:"active_until"`
		WillRenew   bool   `json:"will_renew"`
		ID          string `json:"id"`
	}
	resp, err := client.R().
		SetContext(ctx).
		SetHeader("Authorization", "Bearer "+accessToken).
		SetHeader("Origin", "https://chatgpt.com").
		SetHeader("Referer", "https://chatgpt.com/").
		SetHeader("Accept", "application/json").
		SetSuccessResult(&result).
		SetQueryParam("account_id", accountID).
		Get(chatGPTSubscriptionsURL)
	if err != nil {
		slog.Debug("chatgpt_subscription_request_error", "error", err.Error())
		return ""
	}
	if !resp.IsSuccessState() {
		slog.Debug("chatgpt_subscription_failed", "status", resp.StatusCode, "body", truncate(resp.String(), 200))
		return ""
	}

	activeUntil := strings.TrimSpace(result.ActiveUntil)
	if activeUntil == "" {
		slog.Debug("chatgpt_subscription_no_active_until", "plan_type", result.PlanType, "has_subscription_id", strings.TrimSpace(result.ID) != "", "will_renew", result.WillRenew)
		return ""
	}
	if _, err := time.Parse(time.RFC3339, activeUntil); err != nil {
		slog.Debug("chatgpt_subscription_bad_active_until", "active_until", activeUntil, "error", err.Error())
		return ""
	}

	slog.Info("chatgpt_subscription_success", "plan_type", result.PlanType, "subscription_expires_at", activeUntil, "account_id", accountID)
	return activeUntil
}

// fillAccountInfo 从单个 account 对象中提取 plan_type 和 subscription_expires_at。
// fallbackID 是该对象在 accounts 里的 map key，用于 account.account_id 缺失时兜底。
func fillAccountInfo(info *ChatGPTAccountInfo, acct map[string]any, fallbackID string) {
	info.PlanType = extractPlanType(acct)
	info.SubscriptionExpiresAt = extractEntitlementExpiresAt(acct)
	info.AccountID = chatGPTAccountObjectID(acct, fallbackID)
	if !isChatGPTTeamWorkspaceAccount(acct) {
		return
	}

	account, ok := acct["account"].(map[string]any)
	if !ok {
		return
	}
	info.IsTeamWorkspace = true
	info.WorkspaceName = chatGPTAccountString(account, "name")
	info.WorkspaceCreatedTime = chatGPTAccountString(account, "created_time")
	info.WorkspaceOrganizationID = chatGPTAccountString(account, "organization_id")
	info.WorkspaceType = chatGPTAccountString(account, "workspace_type")
	info.HasSelfServeBusinessProlite = isChatGPTTeamPlanType(info.PlanType) &&
		strings.EqualFold(strings.TrimSpace(info.PlanType), "self_serve_business_prolite")
	info.HasSelfServeBusinessProlite = info.HasSelfServeBusinessProlite ||
		chatGPTAccountHasFeature(acct, "self_serve_business_prolite")
}

func isChatGPTTeamWorkspaceAccount(acct map[string]any) bool {
	account, ok := acct["account"].(map[string]any)
	if !ok {
		return false
	}
	planType := strings.ToLower(strings.TrimSpace(extractPlanType(acct)))
	if isChatGPTTeamPlanType(planType) {
		return true
	}
	if strings.EqualFold(chatGPTAccountString(account, "structure"), "workspace") {
		return true
	}
	return chatGPTAccountHasFeature(acct, "self_serve_business_prolite") &&
		strings.TrimSpace(chatGPTAccountString(account, "organization_id")) != "" &&
		!strings.EqualFold(chatGPTAccountString(account, "structure"), "personal")
}

func chatGPTAccountHasFeature(acct map[string]any, wanted string) bool {
	switch features := acct["features"].(type) {
	case []any:
		for _, raw := range features {
			feature, _ := raw.(string)
			if strings.EqualFold(strings.TrimSpace(feature), wanted) {
				return true
			}
		}
	case []string:
		for _, feature := range features {
			if strings.EqualFold(strings.TrimSpace(feature), wanted) {
				return true
			}
		}
	}
	return false
}

func chatGPTAccountString(account map[string]any, key string) string {
	value, _ := account[key].(string)
	return strings.TrimSpace(value)
}

type OpenAIWorkspaceInfo struct {
	AccountID      string         `json:"account_id"`
	Name           string         `json:"name"`
	CreatedTime    string         `json:"created_time"`
	OrganizationID string         `json:"organization_id"`
	PlanType       string         `json:"plan_type"`
	WorkspaceType  string         `json:"workspace_type,omitempty"`
	SeatTypeCounts map[string]int `json:"seat_type_counts"`
	MaximumSeats   int            `json:"maximum_seats"`
	FetchedAt      int64          `json:"fetched_at"`
}

type OpenAIWorkspaceInvite struct {
	ID             string `json:"id"`
	EmailAddress   string `json:"email_address"`
	Role           string `json:"role"`
	Status         int    `json:"status"`
	SeatType       string `json:"seat_type"`
	CreatedTime    string `json:"created_time"`
	IsSCIMManaged  bool   `json:"is_scim_managed"`
	CreationSource any    `json:"creation_source"`
}

type OpenAIWorkspaceInviteResult struct {
	AccountInvites []OpenAIWorkspaceInvite `json:"account_invites"`
	ErroredEmails  []string                `json:"errored_emails"`
}

type OpenAIWorkspaceInviteListResult struct {
	Items  []OpenAIWorkspaceInvite `json:"items"`
	Total  int                     `json:"total"`
	Limit  int                     `json:"limit"`
	Offset int                     `json:"offset"`
}

type OpenAIWorkspaceUser struct {
	ID                  string `json:"id"`
	AccountUserID       string `json:"account_user_id"`
	Email               string `json:"email"`
	VerifiedEmail       any    `json:"verified_email"`
	Role                string `json:"role"`
	SeatType            string `json:"seat_type"`
	CreditLimits        any    `json:"credit_limits"`
	Name                string `json:"name"`
	CreatedTime         string `json:"created_time"`
	IsSCIMManaged       bool   `json:"is_scim_managed"`
	CreationSource      any    `json:"creation_source"`
	DeactivatedTime     any    `json:"deactivated_time"`
	PendingSeatType     any    `json:"pending_seat_type"`
	ReclaimableSeatType any    `json:"reclaimable_seat_type"`
}

type OpenAIWorkspaceUserListResult struct {
	Items  []OpenAIWorkspaceUser `json:"items"`
	Total  int                   `json:"total"`
	Limit  int                   `json:"limit"`
	Offset int                   `json:"offset"`
}

func fetchChatGPTSeatTypeCounts(ctx context.Context, clientFactory PrivacyClientFactory, accessToken, proxyURL, accountID string) (*OpenAIWorkspaceInfo, error) {
	accountID = strings.TrimSpace(accountID)
	if accessToken == "" {
		return nil, fmt.Errorf("access token is required")
	}
	if accountID == "" {
		return nil, fmt.Errorf("workspace account id is required")
	}
	if clientFactory == nil {
		return nil, fmt.Errorf("openai workspace client is not configured")
	}

	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()

	client, err := clientFactory(proxyURL)
	if err != nil {
		return nil, fmt.Errorf("create workspace client: %w", err)
	}

	var result struct {
		SeatTypeCounts map[string]int `json:"seat_type_counts"`
		MaximumSeats   int            `json:"maximum_seats"`
	}
	request := client.R().SetContext(ctx)
	setChatGPTBackendRequestHeaders(request, accessToken, accountID, "/backend-api/accounts/"+accountID+"/users/seat_type_counts")
	resp, err := request.
		SetSuccessResult(&result).
		Get(fmt.Sprintf(chatGPTSeatTypeCountsURL, url.PathEscape(accountID)))
	if err != nil {
		return nil, fmt.Errorf("request workspace seat counts: %w", err)
	}
	if !resp.IsSuccessState() {
		return nil, fmt.Errorf("workspace seat counts request failed with status %d: %s", resp.StatusCode, truncate(resp.String(), 200))
	}
	if result.SeatTypeCounts == nil {
		return nil, fmt.Errorf("workspace seat counts response is missing seat_type_counts")
	}

	return &OpenAIWorkspaceInfo{
		AccountID:      accountID,
		SeatTypeCounts: result.SeatTypeCounts,
		MaximumSeats:   result.MaximumSeats,
		FetchedAt:      time.Now().Unix(),
	}, nil
}

func fetchChatGPTWorkspaceInvites(ctx context.Context, clientFactory PrivacyClientFactory, accessToken, proxyURL, accountID string, emailAddresses []string, role, seatType string, resendEmails bool) (*OpenAIWorkspaceInviteResult, error) {
	accountID = strings.TrimSpace(accountID)
	if accessToken == "" {
		return nil, fmt.Errorf("access token is required")
	}
	if accountID == "" {
		return nil, fmt.Errorf("workspace account id is required")
	}
	if len(emailAddresses) == 0 {
		return nil, fmt.Errorf("at least one email address is required")
	}
	if clientFactory == nil {
		return nil, fmt.Errorf("openai workspace client is not configured")
	}

	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	client, err := clientFactory(proxyURL)
	if err != nil {
		return nil, fmt.Errorf("create workspace client: %w", err)
	}
	payload := struct {
		EmailAddresses []string `json:"email_addresses"`
		FlowID         string   `json:"flow_id"`
		Role           string   `json:"role"`
		SeatType       string   `json:"seat_type"`
		ResendEmails   bool     `json:"resend_emails"`
		SubmissionID   string   `json:"submission_id"`
	}{
		EmailAddresses: emailAddresses,
		FlowID:         uuid.NewString(),
		Role:           role,
		SeatType:       seatType,
		ResendEmails:   resendEmails,
		SubmissionID:   uuid.NewString(),
	}
	var result OpenAIWorkspaceInviteResult
	request := client.R().SetContext(ctx)
	setChatGPTBackendRequestHeaders(request, accessToken, accountID, "/backend-api/accounts/"+accountID+"/invites")
	resp, err := request.
		SetBody(payload).
		SetSuccessResult(&result).
		Post(fmt.Sprintf(chatGPTWorkspaceInvitesURL, url.PathEscape(accountID)))
	if err != nil {
		return nil, fmt.Errorf("request workspace invites: %w", err)
	}
	if !resp.IsSuccessState() {
		return nil, fmt.Errorf("workspace invite request failed with status %d: %s", resp.StatusCode, truncate(resp.String(), 300))
	}
	return &result, nil
}

func fetchChatGPTWorkspaceInviteList(ctx context.Context, clientFactory PrivacyClientFactory, accessToken, proxyURL, accountID string, offset, limit int, query string) (*OpenAIWorkspaceInviteListResult, error) {
	accountID = strings.TrimSpace(accountID)
	if accessToken == "" {
		return nil, fmt.Errorf("access token is required")
	}
	if accountID == "" {
		return nil, fmt.Errorf("workspace account id is required")
	}
	if clientFactory == nil {
		return nil, fmt.Errorf("openai workspace client is not configured")
	}

	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	client, err := clientFactory(proxyURL)
	if err != nil {
		return nil, fmt.Errorf("create workspace client: %w", err)
	}
	var result OpenAIWorkspaceInviteListResult
	request := client.R().SetContext(ctx)
	setChatGPTBackendRequestHeaders(request, accessToken, accountID, "/backend-api/accounts/"+accountID+"/invites")
	resp, err := request.
		SetQueryParam("offset", strconv.Itoa(offset)).
		SetQueryParam("limit", strconv.Itoa(limit)).
		SetQueryParam("query", query).
		SetSuccessResult(&result).
		Get(fmt.Sprintf(chatGPTWorkspaceInvitesURL, url.PathEscape(accountID)))
	if err != nil {
		return nil, fmt.Errorf("request workspace invite list: %w", err)
	}
	if !resp.IsSuccessState() {
		return nil, fmt.Errorf("workspace invite list request failed with status %d: %s", resp.StatusCode, truncate(resp.String(), 300))
	}
	return &result, nil
}

func fetchChatGPTWorkspaceUserList(ctx context.Context, clientFactory PrivacyClientFactory, accessToken, proxyURL, accountID string, offset, limit int, query string) (*OpenAIWorkspaceUserListResult, error) {
	accountID = strings.TrimSpace(accountID)
	if accessToken == "" {
		return nil, fmt.Errorf("access token is required")
	}
	if accountID == "" {
		return nil, fmt.Errorf("workspace account id is required")
	}
	if clientFactory == nil {
		return nil, fmt.Errorf("openai workspace client is not configured")
	}

	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	client, err := clientFactory(proxyURL)
	if err != nil {
		return nil, fmt.Errorf("create workspace client: %w", err)
	}
	var result OpenAIWorkspaceUserListResult
	request := client.R().SetContext(ctx)
	setChatGPTBackendRequestHeaders(request, accessToken, accountID, "/backend-api/accounts/"+accountID+"/users")
	resp, err := request.
		SetQueryParam("offset", strconv.Itoa(offset)).
		SetQueryParam("limit", strconv.Itoa(limit)).
		SetQueryParam("query", query).
		SetSuccessResult(&result).
		Get(fmt.Sprintf(chatGPTWorkspaceUsersURL, url.PathEscape(accountID)))
	if err != nil {
		return nil, fmt.Errorf("request workspace user list: %w", err)
	}
	if !resp.IsSuccessState() {
		return nil, fmt.Errorf("workspace user list request failed with status %d: %s", resp.StatusCode, truncate(resp.String(), 300))
	}
	return &result, nil
}

// chatGPTAccountObjectID 取单个 account 对象的账号标识。
// accounts 的 map key 有时是 "default" 这类别名，所以优先读 account.account_id。
func chatGPTAccountObjectID(acct map[string]any, fallbackID string) string {
	if account, ok := acct["account"].(map[string]any); ok {
		if id, ok := account["account_id"].(string); ok && strings.TrimSpace(id) != "" {
			return strings.TrimSpace(id)
		}
	}
	return strings.TrimSpace(fallbackID)
}

// extractPlanType 从单个 account 对象中提取 plan_type
func extractPlanType(acct map[string]any) string {
	if account, ok := acct["account"].(map[string]any); ok {
		if planType, ok := account["plan_type"].(string); ok && planType != "" {
			return planType
		}
	}
	if entitlement, ok := acct["entitlement"].(map[string]any); ok {
		if subPlan, ok := entitlement["subscription_plan"].(string); ok && subPlan != "" {
			return subPlan
		}
	}
	return ""
}

func isChatGPTTeamPlanType(planType string) bool {
	switch strings.ToLower(strings.TrimSpace(planType)) {
	case "team", "self_serve_business_prolite":
		return true
	default:
		return false
	}
}

func isUsableChatGPTAccountCandidate(acct map[string]any, now time.Time) bool {
	if acct == nil || hasChatGPTAccountDeactivatedMarker(acct) {
		return false
	}
	if account, ok := acct["account"].(map[string]any); ok && hasChatGPTAccountDeactivatedMarker(account) {
		return false
	}

	expiresAt := extractEntitlementExpiresAt(acct)
	if expiresAt == "" {
		return true
	}
	expiry, err := time.Parse(time.RFC3339, expiresAt)
	if err != nil {
		return true
	}
	return expiry.After(now)
}

func hasChatGPTAccountDeactivatedMarker(obj map[string]any) bool {
	for _, key := range []string{"deactivated", "is_deactivated", "disabled", "is_disabled"} {
		if value, ok := obj[key].(bool); ok && value {
			return true
		}
	}
	for _, key := range []string{"deactivated_at", "disabled_at", "deleted_at"} {
		if value, ok := obj[key].(string); ok && strings.TrimSpace(value) != "" {
			return true
		}
	}
	for _, key := range []string{"status", "state"} {
		value, _ := obj[key].(string)
		switch strings.ToLower(strings.TrimSpace(value)) {
		case "deactivated", "disabled", "deleted", "inactive", "suspended":
			return true
		}
	}
	return false
}

// extractEntitlementExpiresAt 从 entitlement 中提取 expires_at。
// 预期为 RFC3339 字符串格式，如 "2026-05-02T20:32:12+00:00"。
func extractEntitlementExpiresAt(acct map[string]any) string {
	entitlement, ok := acct["entitlement"].(map[string]any)
	if !ok {
		return ""
	}
	ea, _ := entitlement["expires_at"].(string)
	return ea
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + fmt.Sprintf("...(%d more)", len(s)-n)
}
