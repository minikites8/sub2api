package service

import (
	"context"
	"crypto/subtle"
	"log/slog"
	"net/http"
	"strings"
	"time"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/Wei-Shaw/sub2api/internal/pkg/openai"
)

// OpenAIOAuthService handles OpenAI OAuth authentication flows
type OpenAIOAuthService struct {
	sessionStore         *openai.SessionStore
	proxyRepo            ProxyRepository
	oauthClient          OpenAIOAuthClient
	privacyClientFactory PrivacyClientFactory // 用于调用 chatgpt.com/backend-api（ImpersonateChrome）
}

// NewOpenAIOAuthService creates a new OpenAI OAuth service
func NewOpenAIOAuthService(proxyRepo ProxyRepository, oauthClient OpenAIOAuthClient) *OpenAIOAuthService {
	return &OpenAIOAuthService{
		sessionStore: openai.NewSessionStore(),
		proxyRepo:    proxyRepo,
		oauthClient:  oauthClient,
	}
}

// SetPrivacyClientFactory 注入 ImpersonateChrome 客户端工厂，
// 用于调用 chatgpt.com/backend-api 获取账号信息（plan_type 等）。
func (s *OpenAIOAuthService) SetPrivacyClientFactory(factory PrivacyClientFactory) {
	s.privacyClientFactory = factory
}

// OpenAIAuthURLResult contains the authorization URL and session info
type OpenAIAuthURLResult struct {
	AuthURL   string `json:"auth_url"`
	SessionID string `json:"session_id"`
}

// GenerateAuthURL generates an OpenAI OAuth authorization URL
func (s *OpenAIOAuthService) GenerateAuthURL(ctx context.Context, proxyID *int64, redirectURI, platform string) (*OpenAIAuthURLResult, error) {
	// Generate PKCE values
	state, err := openai.GenerateState()
	if err != nil {
		return nil, infraerrors.Newf(http.StatusInternalServerError, "OPENAI_OAUTH_STATE_FAILED", "failed to generate state: %v", err)
	}

	codeVerifier, err := openai.GenerateCodeVerifier()
	if err != nil {
		return nil, infraerrors.Newf(http.StatusInternalServerError, "OPENAI_OAUTH_VERIFIER_FAILED", "failed to generate code verifier: %v", err)
	}

	codeChallenge := openai.GenerateCodeChallenge(codeVerifier)

	// Generate session ID
	sessionID, err := openai.GenerateSessionID()
	if err != nil {
		return nil, infraerrors.Newf(http.StatusInternalServerError, "OPENAI_OAUTH_SESSION_FAILED", "failed to generate session ID: %v", err)
	}

	// Get proxy URL if specified
	var proxyURL string
	if proxyID != nil {
		proxy, err := s.proxyRepo.GetByID(ctx, *proxyID)
		if err != nil {
			return nil, infraerrors.Newf(http.StatusBadRequest, "OPENAI_OAUTH_PROXY_NOT_FOUND", "proxy not found: %v", err)
		}
		if proxy != nil {
			proxyURL = proxy.URL()
		}
	}

	// Use default redirect URI if not specified
	if redirectURI == "" {
		redirectURI = openai.DefaultRedirectURI
	}
	normalizedPlatform := normalizeOpenAIOAuthPlatform(platform)
	clientID, _ := openai.OAuthClientConfigByPlatform(normalizedPlatform)

	// Store session
	session := &openai.OAuthSession{
		State:        state,
		CodeVerifier: codeVerifier,
		ClientID:     clientID,
		RedirectURI:  redirectURI,
		ProxyURL:     proxyURL,
		CreatedAt:    time.Now(),
	}
	s.sessionStore.Set(sessionID, session)

	// Build authorization URL
	authURL := openai.BuildAuthorizationURLForPlatform(state, codeChallenge, redirectURI, normalizedPlatform)

	return &OpenAIAuthURLResult{
		AuthURL:   authURL,
		SessionID: sessionID,
	}, nil
}

// OpenAIExchangeCodeInput represents the input for code exchange
type OpenAIExchangeCodeInput struct {
	SessionID   string
	Code        string
	State       string
	RedirectURI string
	ProxyID     *int64
}

// OpenAITokenInfo represents the token information for OpenAI
type OpenAITokenInfo struct {
	AccessToken                  string `json:"access_token"`
	RefreshToken                 string `json:"refresh_token"`
	IDToken                      string `json:"id_token,omitempty"`
	ExpiresIn                    int64  `json:"expires_in"`
	ExpiresAt                    int64  `json:"expires_at"`
	ClientID                     string `json:"client_id,omitempty"`
	AuthMode                     string `json:"auth_mode,omitempty"`
	Email                        string `json:"email,omitempty"`
	Name                         string `json:"name,omitempty"`
	CreatedTime                  string `json:"created_time,omitempty"`
	ChatGPTAccountID             string `json:"chatgpt_account_id,omitempty"`
	ChatGPTUserID                string `json:"chatgpt_user_id,omitempty"`
	ChatGPTAccountFedRAMP        bool   `json:"chatgpt_account_is_fedramp,omitempty"`
	OrganizationID               string `json:"organization_id,omitempty"`
	TeamName                     string `json:"team_name,omitempty"`
	TeamCreatedTime              string `json:"team_created_time,omitempty"`
	TeamOrganizationID           string `json:"team_organization_id,omitempty"`
	TeamAccountID                string `json:"team_account_id,omitempty"`
	TeamPlanType                 string `json:"team_plan_type,omitempty"`
	TeamWorkspaceType            string `json:"team_workspace_type,omitempty"`
	TeamSelfServeBusinessProlite bool   `json:"team_self_serve_business_prolite,omitempty"`
	PlanType                     string `json:"plan_type,omitempty"`
	SubscriptionExpiresAt        string `json:"subscription_expires_at,omitempty"`
	PrivacyMode                  string `json:"privacy_mode,omitempty"`
}

// ExchangeCode exchanges authorization code for tokens
func (s *OpenAIOAuthService) ExchangeCode(ctx context.Context, input *OpenAIExchangeCodeInput) (*OpenAITokenInfo, error) {
	// Get session
	session, ok := s.sessionStore.Get(input.SessionID)
	if !ok {
		return nil, infraerrors.New(http.StatusBadRequest, "OPENAI_OAUTH_SESSION_NOT_FOUND", "session not found or expired")
	}
	if input.State == "" {
		return nil, infraerrors.New(http.StatusBadRequest, "OPENAI_OAUTH_STATE_REQUIRED", "oauth state is required")
	}
	if subtle.ConstantTimeCompare([]byte(input.State), []byte(session.State)) != 1 {
		return nil, infraerrors.New(http.StatusBadRequest, "OPENAI_OAUTH_INVALID_STATE", "invalid oauth state")
	}

	// Get proxy URL: prefer input.ProxyID, fallback to session.ProxyURL
	proxyURL := session.ProxyURL
	if input.ProxyID != nil {
		proxy, err := s.proxyRepo.GetByID(ctx, *input.ProxyID)
		if err != nil {
			return nil, infraerrors.Newf(http.StatusBadRequest, "OPENAI_OAUTH_PROXY_NOT_FOUND", "proxy not found: %v", err)
		}
		if proxy != nil {
			proxyURL = proxy.URL()
		}
	}

	// Use redirect URI from session or input
	redirectURI := session.RedirectURI
	if input.RedirectURI != "" {
		redirectURI = input.RedirectURI
	}
	clientID := strings.TrimSpace(session.ClientID)
	if clientID == "" {
		clientID = openai.ClientID
	}

	// Exchange code for token
	tokenResp, err := s.oauthClient.ExchangeCode(ctx, input.Code, session.CodeVerifier, redirectURI, proxyURL, clientID)
	if err != nil {
		return nil, err
	}

	// Parse ID token to get user info
	var userInfo *openai.UserInfo
	if tokenResp.IDToken != "" {
		claims, parseErr := openai.ParseIDToken(tokenResp.IDToken)
		if parseErr != nil {
			slog.Warn("openai_oauth_id_token_parse_failed", "error", parseErr)
		} else {
			userInfo = claims.GetUserInfo()
		}
	}

	// Delete session after successful exchange
	s.sessionStore.Delete(input.SessionID)

	tokenInfo := &OpenAITokenInfo{
		AccessToken:  tokenResp.AccessToken,
		RefreshToken: tokenResp.RefreshToken,
		IDToken:      tokenResp.IDToken,
		ExpiresIn:    int64(tokenResp.ExpiresIn),
		ExpiresAt:    time.Now().Unix() + int64(tokenResp.ExpiresIn),
		ClientID:     clientID,
	}

	if userInfo != nil {
		tokenInfo.Email = userInfo.Email
		tokenInfo.ChatGPTAccountID = userInfo.ChatGPTAccountID
		tokenInfo.ChatGPTUserID = userInfo.ChatGPTUserID
		tokenInfo.OrganizationID = userInfo.OrganizationID
		tokenInfo.PlanType = userInfo.PlanType
	}

	s.enrichTokenInfo(ctx, tokenInfo, proxyURL)

	return tokenInfo, nil
}

// RefreshToken refreshes an OpenAI OAuth token
func (s *OpenAIOAuthService) RefreshToken(ctx context.Context, refreshToken string, proxyURL string) (*OpenAITokenInfo, error) {
	return s.RefreshTokenWithClientID(ctx, refreshToken, proxyURL, "")
}

// RefreshTokenWithClientID refreshes an OpenAI OAuth token with optional client_id.
func (s *OpenAIOAuthService) RefreshTokenWithClientID(ctx context.Context, refreshToken string, proxyURL string, clientID string) (*OpenAITokenInfo, error) {
	tokenResp, err := s.oauthClient.RefreshTokenWithClientID(ctx, refreshToken, proxyURL, clientID)
	if err != nil {
		return nil, err
	}

	// Parse ID token to get user info
	var userInfo *openai.UserInfo
	if tokenResp.IDToken != "" {
		claims, parseErr := openai.ParseIDToken(tokenResp.IDToken)
		if parseErr != nil {
			slog.Warn("openai_oauth_id_token_parse_failed", "error", parseErr)
		} else {
			userInfo = claims.GetUserInfo()
		}
	}

	tokenInfo := &OpenAITokenInfo{
		AccessToken:  tokenResp.AccessToken,
		RefreshToken: tokenResp.RefreshToken,
		IDToken:      tokenResp.IDToken,
		ExpiresIn:    int64(tokenResp.ExpiresIn),
		ExpiresAt:    time.Now().Unix() + int64(tokenResp.ExpiresIn),
	}
	if trimmed := strings.TrimSpace(clientID); trimmed != "" {
		tokenInfo.ClientID = trimmed
	}

	if userInfo != nil {
		tokenInfo.Email = userInfo.Email
		tokenInfo.ChatGPTAccountID = userInfo.ChatGPTAccountID
		tokenInfo.ChatGPTUserID = userInfo.ChatGPTUserID
		tokenInfo.OrganizationID = userInfo.OrganizationID
		tokenInfo.PlanType = userInfo.PlanType
	}

	s.enrichTokenInfo(ctx, tokenInfo, proxyURL)

	return tokenInfo, nil
}

// enrichTokenInfo 通过 ChatGPT backend-api 补全 tokenInfo 并设置隐私（best-effort）。
// 从 accounts/check 获取最新 plan_type、subscription_expires_at、email，
// 然后尝试关闭训练数据共享。适用于所有获取/刷新 token 的路径。
func (s *OpenAIOAuthService) enrichTokenInfo(ctx context.Context, tokenInfo *OpenAITokenInfo, proxyURL string) {
	if tokenInfo.AccessToken == "" || s.privacyClientFactory == nil {
		return
	}

	// 从 access_token JWT 中提取 orgID（poid），用于匹配正确的账号
	orgID := tokenInfo.OrganizationID
	if orgID == "" {
		if atClaims, err := openai.DecodeIDToken(tokenInfo.AccessToken); err == nil && atClaims.OpenAIAuth != nil {
			orgID = atClaims.OpenAIAuth.POID
		}
	}
	// accounts/check 命中的记录不属于个人账号时，必须改用个人订阅端点拿到期时间，
	// 否则会把 workspace 权益的 expires_at 当成个人订阅到期日展示。
	forcePersonalSubscriptionLookup := false
	if info := fetchChatGPTAccountInfo(ctx, s.privacyClientFactory, tokenInfo.AccessToken, proxyURL, orgID); info != nil {
		// chatgpt_plan_type from the ID token is the canonical personal-plan value.
		// accounts/check is a multi-account/workspace endpoint; inactive team or
		// business workspaces can otherwise overwrite Pro/Free with internal
		// workspace billing plan names such as self_serve_business_usage_based.
		appliedAccountInfoPlanType := shouldApplyChatGPTAccountInfoPlanType(tokenInfo.PlanType, info.PlanType)
		if appliedAccountInfoPlanType {
			tokenInfo.PlanType = info.PlanType
		}
		// plan_type 与 subscription_expires_at 必须描述同一份订阅。套餐取自
		// accounts/check 时，到期时间跟着取同一条记录；套餐保留了 JWT 里的个人值时，
		// 只有该记录确实就是个人账号才能用它的 entitlement.expires_at——poid 指向的
		// 默认 Personal workspace 与 chatgpt_account_id 可以是两个不同的标识，
		// 混用会显示成「个人 Pro + workspace 到期时间」。
		if info.SubscriptionExpiresAt != "" {
			if appliedAccountInfoPlanType || chatGPTAccountInfoBelongsToTokenAccount(tokenInfo, info) ||
				chatGPTTeamAccountInfoMatchesToken(tokenInfo, info, orgID) {
				tokenInfo.SubscriptionExpiresAt = info.SubscriptionExpiresAt
			} else {
				forcePersonalSubscriptionLookup = true
			}
		}
		if tokenInfo.Email == "" && info.Email != "" {
			tokenInfo.Email = info.Email
		}
		if info.IsTeamWorkspace {
			tokenInfo.Name = info.WorkspaceName
			tokenInfo.CreatedTime = info.WorkspaceCreatedTime
			tokenInfo.TeamName = info.WorkspaceName
			tokenInfo.TeamCreatedTime = info.WorkspaceCreatedTime
			tokenInfo.TeamOrganizationID = info.WorkspaceOrganizationID
			tokenInfo.TeamAccountID = info.AccountID
			tokenInfo.TeamPlanType = info.PlanType
			tokenInfo.TeamWorkspaceType = info.WorkspaceType
			tokenInfo.TeamSelfServeBusinessProlite = info.HasSelfServeBusinessProlite
			if info.WorkspaceOrganizationID != "" {
				tokenInfo.OrganizationID = info.WorkspaceOrganizationID
			}
		}
	}
	if forcePersonalSubscriptionLookup || strings.TrimSpace(tokenInfo.SubscriptionExpiresAt) == "" {
		if expiresAt := fetchChatGPTSubscriptionExpiresAt(ctx, s.privacyClientFactory, tokenInfo.AccessToken, proxyURL, resolveChatGPTSubscriptionAccountID(tokenInfo, orgID)); expiresAt != "" {
			tokenInfo.SubscriptionExpiresAt = expiresAt
		}
	}

	// 尝试设置隐私（关闭训练数据共享），best-effort
	tokenInfo.PrivacyMode = disableOpenAITraining(ctx, s.privacyClientFactory, tokenInfo.AccessToken, proxyURL)
}

func (s *OpenAIOAuthService) GetWorkspaceInfo(ctx context.Context, account *Account) (*OpenAIWorkspaceInfo, error) {
	if account == nil || !account.IsOpenAIOAuth() {
		return nil, infraerrors.New(http.StatusBadRequest, "OPENAI_WORKSPACE_INVALID_ACCOUNT", "account is not an OpenAI OAuth account")
	}
	accessToken := strings.TrimSpace(account.GetCredential("access_token"))
	if accessToken == "" {
		return nil, infraerrors.New(http.StatusBadRequest, "OPENAI_WORKSPACE_ACCESS_TOKEN_REQUIRED", "access token is required")
	}
	if s.privacyClientFactory == nil {
		return nil, infraerrors.New(http.StatusInternalServerError, "OPENAI_WORKSPACE_CLIENT_UNAVAILABLE", "OpenAI workspace client is not configured")
	}

	var proxyURL string
	if account.ProxyID != nil && s.proxyRepo != nil {
		proxy, err := s.proxyRepo.GetByID(ctx, *account.ProxyID)
		if err == nil && proxy != nil {
			proxyURL = proxy.URL()
		}
	}

	workspace := &OpenAIWorkspaceInfo{
		AccountID:      strings.TrimSpace(account.GetCredential("team_account_id")),
		Name:           strings.TrimSpace(account.GetCredential("team_name")),
		CreatedTime:    strings.TrimSpace(account.GetCredential("team_created_time")),
		OrganizationID: strings.TrimSpace(account.GetCredential("team_organization_id")),
		PlanType:       strings.TrimSpace(account.GetCredential("team_plan_type")),
		WorkspaceType:  strings.TrimSpace(account.GetCredential("team_workspace_type")),
	}
	// Older Team imports stored the accounts/check fields under their original
	// names. Keep those imports immediately displayable while the fresh lookup
	// fills the normalized team_* keys.
	if workspace.Name == "" {
		workspace.Name = strings.TrimSpace(account.GetCredential("name"))
	}
	if workspace.CreatedTime == "" {
		workspace.CreatedTime = strings.TrimSpace(account.GetCredential("created_time"))
	}
	if workspace.AccountID == "" {
		workspace.AccountID = strings.TrimSpace(account.GetCredential("chatgpt_account_id"))
	}
	if workspace.OrganizationID == "" {
		workspace.OrganizationID = strings.TrimSpace(account.GetCredential("organization_id"))
	}
	if workspace.PlanType == "" {
		workspace.PlanType = strings.TrimSpace(account.GetCredential("plan_type"))
	}

	needsAccountLookup := strings.TrimSpace(account.GetCredential("team_account_id")) == "" ||
		strings.TrimSpace(account.GetCredential("team_name")) == "" ||
		strings.TrimSpace(account.GetCredential("team_created_time")) == "" ||
		strings.TrimSpace(account.GetCredential("team_organization_id")) == "" ||
		strings.TrimSpace(account.GetCredential("team_plan_type")) == ""
	if needsAccountLookup {
		hint := workspace.AccountID
		if hint == "" {
			hint = workspace.OrganizationID
		}
		if hint == "" {
			hint = strings.TrimSpace(account.GetCredential("chatgpt_account_id"))
		}
		if info := fetchChatGPTAccountInfo(ctx, s.privacyClientFactory, accessToken, proxyURL, hint); info != nil && info.IsTeamWorkspace {
			workspace.AccountID = info.AccountID
			workspace.Name = info.WorkspaceName
			workspace.CreatedTime = info.WorkspaceCreatedTime
			workspace.OrganizationID = info.WorkspaceOrganizationID
			workspace.PlanType = info.PlanType
			workspace.WorkspaceType = info.WorkspaceType
		}
	}
	if !isChatGPTTeamPlanType(workspace.PlanType) && workspace.Name == "" && workspace.OrganizationID == "" {
		return nil, infraerrors.New(http.StatusBadRequest, "OPENAI_WORKSPACE_TEAM_REQUIRED", "account is not a Team workspace account")
	}

	seatInfo, err := fetchChatGPTSeatTypeCounts(ctx, s.privacyClientFactory, accessToken, proxyURL, workspace.AccountID)
	if err != nil {
		return nil, infraerrors.Newf(http.StatusBadGateway, "OPENAI_WORKSPACE_SEAT_COUNTS_FAILED", "%v", err)
	}
	seatInfo.Name = workspace.Name
	seatInfo.CreatedTime = workspace.CreatedTime
	seatInfo.OrganizationID = workspace.OrganizationID
	seatInfo.PlanType = workspace.PlanType
	seatInfo.WorkspaceType = workspace.WorkspaceType
	return seatInfo, nil
}

func (s *OpenAIOAuthService) InviteWorkspaceMembers(ctx context.Context, account *Account, workspaceAccountID string, emailAddresses []string, role, seatType string, resendEmails bool) (*OpenAIWorkspaceInviteResult, error) {
	if account == nil || !account.IsOpenAIOAuth() {
		return nil, infraerrors.New(http.StatusBadRequest, "OPENAI_WORKSPACE_INVALID_ACCOUNT", "account is not an OpenAI OAuth account")
	}
	accessToken := strings.TrimSpace(account.GetCredential("access_token"))
	if accessToken == "" {
		return nil, infraerrors.New(http.StatusBadRequest, "OPENAI_WORKSPACE_ACCESS_TOKEN_REQUIRED", "access token is required")
	}
	role = strings.TrimSpace(role)
	if role == "" {
		role = "standard-user"
	}
	if role != "standard-user" {
		return nil, infraerrors.New(http.StatusBadRequest, "OPENAI_WORKSPACE_ROLE_INVALID", "only standard-user invitations are supported")
	}
	seatType = strings.TrimSpace(strings.ToLower(seatType))
	if seatType == "" {
		seatType = "default"
	}
	switch seatType {
	case "default", "usage_based", "automation", "prolite":
	default:
		return nil, infraerrors.New(http.StatusBadRequest, "OPENAI_WORKSPACE_SEAT_TYPE_INVALID", "unsupported workspace seat type")
	}
	normalizedEmails := make([]string, 0, len(emailAddresses))
	seen := make(map[string]struct{}, len(emailAddresses))
	for _, email := range emailAddresses {
		email = strings.TrimSpace(email)
		if email == "" {
			continue
		}
		key := strings.ToLower(email)
		if !strings.Contains(email, "@") || strings.HasPrefix(email, "@") || strings.HasSuffix(email, "@") {
			return nil, infraerrors.Newf(http.StatusBadRequest, "OPENAI_WORKSPACE_EMAIL_INVALID", "invalid email address: %s", email)
		}
		if _, exists := seen[key]; exists {
			continue
		}
		seen[key] = struct{}{}
		normalizedEmails = append(normalizedEmails, email)
	}
	if len(normalizedEmails) == 0 {
		return nil, infraerrors.New(http.StatusBadRequest, "OPENAI_WORKSPACE_EMAIL_REQUIRED", "at least one email address is required")
	}

	var proxyURL string
	if account.ProxyID != nil && s.proxyRepo != nil {
		proxy, err := s.proxyRepo.GetByID(ctx, *account.ProxyID)
		if err == nil && proxy != nil {
			proxyURL = proxy.URL()
		}
	}
	result, err := fetchChatGPTWorkspaceInvites(ctx, s.privacyClientFactory, accessToken, proxyURL, workspaceAccountID, normalizedEmails, role, seatType, resendEmails)
	if err != nil {
		return nil, infraerrors.Newf(http.StatusBadGateway, "OPENAI_WORKSPACE_INVITE_FAILED", "%v", err)
	}
	return result, nil
}

func (s *OpenAIOAuthService) ListWorkspaceInvites(ctx context.Context, account *Account, workspaceAccountID string, offset, limit int, query string) (*OpenAIWorkspaceInviteListResult, error) {
	if account == nil || !account.IsOpenAIOAuth() {
		return nil, infraerrors.New(http.StatusBadRequest, "OPENAI_WORKSPACE_INVALID_ACCOUNT", "account is not an OpenAI OAuth account")
	}
	accessToken := strings.TrimSpace(account.GetCredential("access_token"))
	if accessToken == "" {
		return nil, infraerrors.New(http.StatusBadRequest, "OPENAI_WORKSPACE_ACCESS_TOKEN_REQUIRED", "access token is required")
	}
	if offset < 0 {
		return nil, infraerrors.New(http.StatusBadRequest, "OPENAI_WORKSPACE_INVITE_OFFSET_INVALID", "offset must be non-negative")
	}
	if limit <= 0 || limit > 100 {
		return nil, infraerrors.New(http.StatusBadRequest, "OPENAI_WORKSPACE_INVITE_LIMIT_INVALID", "limit must be between 1 and 100")
	}
	query = strings.TrimSpace(query)
	if len(query) > 100 {
		query = query[:100]
	}

	var proxyURL string
	if account.ProxyID != nil && s.proxyRepo != nil {
		proxy, err := s.proxyRepo.GetByID(ctx, *account.ProxyID)
		if err == nil && proxy != nil {
			proxyURL = proxy.URL()
		}
	}
	result, err := fetchChatGPTWorkspaceInviteList(ctx, s.privacyClientFactory, accessToken, proxyURL, workspaceAccountID, offset, limit, query)
	if err != nil {
		return nil, infraerrors.Newf(http.StatusBadGateway, "OPENAI_WORKSPACE_INVITE_LIST_FAILED", "%v", err)
	}
	return result, nil
}

func (s *OpenAIOAuthService) ListWorkspaceUsers(ctx context.Context, account *Account, workspaceAccountID string, offset, limit int, query string) (*OpenAIWorkspaceUserListResult, error) {
	if account == nil || !account.IsOpenAIOAuth() {
		return nil, infraerrors.New(http.StatusBadRequest, "OPENAI_WORKSPACE_INVALID_ACCOUNT", "account is not an OpenAI OAuth account")
	}
	accessToken := strings.TrimSpace(account.GetCredential("access_token"))
	if accessToken == "" {
		return nil, infraerrors.New(http.StatusBadRequest, "OPENAI_WORKSPACE_ACCESS_TOKEN_REQUIRED", "access token is required")
	}
	if offset < 0 {
		return nil, infraerrors.New(http.StatusBadRequest, "OPENAI_WORKSPACE_USER_OFFSET_INVALID", "offset must be non-negative")
	}
	if limit <= 0 || limit > 100 {
		return nil, infraerrors.New(http.StatusBadRequest, "OPENAI_WORKSPACE_USER_LIMIT_INVALID", "limit must be between 1 and 100")
	}
	query = strings.TrimSpace(query)
	if len(query) > 100 {
		query = query[:100]
	}

	var proxyURL string
	if account.ProxyID != nil && s.proxyRepo != nil {
		proxy, err := s.proxyRepo.GetByID(ctx, *account.ProxyID)
		if err == nil && proxy != nil {
			proxyURL = proxy.URL()
		}
	}
	result, err := fetchChatGPTWorkspaceUserList(ctx, s.privacyClientFactory, accessToken, proxyURL, workspaceAccountID, offset, limit, query)
	if err != nil {
		return nil, infraerrors.Newf(http.StatusBadGateway, "OPENAI_WORKSPACE_USER_LIST_FAILED", "%v", err)
	}
	return result, nil
}

func shouldApplyChatGPTAccountInfoPlanType(current, candidate string) bool {
	return strings.TrimSpace(candidate) != "" && strings.TrimSpace(current) == ""
}

// chatGPTAccountInfoBelongsToTokenAccount 判断 accounts/check 命中的那条记录是不是
// token 自己的个人 ChatGPT 账号。两侧任一缺 ID 时无法区分，返回 true 保持既有行为。
func chatGPTAccountInfoBelongsToTokenAccount(tokenInfo *OpenAITokenInfo, info *ChatGPTAccountInfo) bool {
	personalID := strings.TrimSpace(tokenInfo.ChatGPTAccountID)
	sourceID := strings.TrimSpace(info.AccountID)
	if personalID == "" || sourceID == "" {
		return true
	}
	return strings.EqualFold(personalID, sourceID)
}

func chatGPTTeamAccountInfoMatchesToken(tokenInfo *OpenAITokenInfo, info *ChatGPTAccountInfo, orgID string) bool {
	if tokenInfo == nil || info == nil || !info.IsTeamWorkspace {
		return false
	}
	if isChatGPTTeamPlanType(tokenInfo.PlanType) {
		return true
	}
	for _, candidate := range []string{orgID, tokenInfo.ChatGPTAccountID, tokenInfo.OrganizationID} {
		candidate = strings.TrimSpace(candidate)
		if candidate == "" {
			continue
		}
		if strings.EqualFold(candidate, info.AccountID) || strings.EqualFold(candidate, info.WorkspaceOrganizationID) {
			return true
		}
	}
	return false
}

func resolveChatGPTSubscriptionAccountID(tokenInfo *OpenAITokenInfo, orgID string) string {
	for _, candidate := range []string{
		tokenInfo.ChatGPTAccountID,
		tokenInfo.OrganizationID,
		orgID,
	} {
		if trimmed := strings.TrimSpace(candidate); trimmed != "" {
			return trimmed
		}
	}
	return ""
}

// RefreshAccountToken refreshes token for an OpenAI OAuth account
func (s *OpenAIOAuthService) RefreshAccountToken(ctx context.Context, account *Account) (*OpenAITokenInfo, error) {
	if account.Platform != PlatformOpenAI {
		return nil, infraerrors.New(http.StatusBadRequest, "OPENAI_OAUTH_INVALID_ACCOUNT", "account is not an OpenAI account")
	}
	if account.Type != AccountTypeOAuth {
		return nil, infraerrors.New(http.StatusBadRequest, "OPENAI_OAUTH_INVALID_ACCOUNT_TYPE", "account is not an OAuth account")
	}

	var proxyURL string
	if account.ProxyID != nil && s.proxyRepo != nil {
		proxy, err := s.proxyRepo.GetByID(ctx, *account.ProxyID)
		if err == nil && proxy != nil {
			proxyURL = proxy.URL()
		}
	}

	accessToken := account.GetCredential("access_token")
	if account.IsOpenAIPersonalAccessToken() {
		if accessToken == "" {
			return nil, infraerrors.New(http.StatusBadRequest, "OPENAI_CODEX_PAT_REQUIRED", "access token is required")
		}
		return s.ValidateCodexPersonalAccessToken(ctx, accessToken, proxyURL)
	}

	refreshToken := account.GetCredential("refresh_token")
	if refreshToken == "" {
		if accessToken != "" {
			tokenInfo := &OpenAITokenInfo{
				AccessToken:                  accessToken,
				RefreshToken:                 "",
				IDToken:                      account.GetCredential("id_token"),
				ClientID:                     account.GetCredential("client_id"),
				Email:                        account.GetCredential("email"),
				Name:                         account.GetCredential("name"),
				CreatedTime:                  account.GetCredential("created_time"),
				ChatGPTAccountID:             account.GetCredential("chatgpt_account_id"),
				ChatGPTUserID:                account.GetCredential("chatgpt_user_id"),
				OrganizationID:               account.GetCredential("organization_id"),
				TeamName:                     account.GetCredential("team_name"),
				TeamCreatedTime:              account.GetCredential("team_created_time"),
				TeamOrganizationID:           account.GetCredential("team_organization_id"),
				TeamAccountID:                account.GetCredential("team_account_id"),
				TeamPlanType:                 account.GetCredential("team_plan_type"),
				TeamWorkspaceType:            account.GetCredential("team_workspace_type"),
				TeamSelfServeBusinessProlite: strings.EqualFold(account.GetCredential("team_self_serve_business_prolite"), "true"),
				PlanType:                     account.GetCredential("plan_type"),
				SubscriptionExpiresAt:        account.GetCredential("subscription_expires_at"),
			}
			if expiresAt := account.GetCredentialAsTime("expires_at"); expiresAt != nil {
				tokenInfo.ExpiresAt = expiresAt.Unix()
				tokenInfo.ExpiresIn = int64(time.Until(*expiresAt).Seconds())
			}
			s.enrichTokenInfo(ctx, tokenInfo, proxyURL)
			return tokenInfo, nil
		}
		return nil, infraerrors.New(http.StatusBadRequest, "OPENAI_OAUTH_NO_REFRESH_TOKEN", "no refresh token available")
	}

	clientID := account.GetCredential("client_id")
	return s.RefreshTokenWithClientID(ctx, refreshToken, proxyURL, clientID)
}

// BuildAccountCredentials builds credentials map from token info
func (s *OpenAIOAuthService) BuildAccountCredentials(tokenInfo *OpenAITokenInfo) map[string]any {
	creds := map[string]any{
		"access_token": tokenInfo.AccessToken,
	}
	if tokenInfo.ExpiresAt > 0 {
		creds["expires_at"] = time.Unix(tokenInfo.ExpiresAt, 0).Format(time.RFC3339)
	}
	// 仅在刷新响应返回了新的 refresh_token 时才更新，防止用空值覆盖已有令牌
	if strings.TrimSpace(tokenInfo.RefreshToken) != "" {
		creds["refresh_token"] = tokenInfo.RefreshToken
	}

	if tokenInfo.IDToken != "" {
		creds["id_token"] = tokenInfo.IDToken
	}
	if tokenInfo.Email != "" {
		creds["email"] = tokenInfo.Email
	}
	if tokenInfo.Name != "" {
		creds["name"] = tokenInfo.Name
	}
	if tokenInfo.CreatedTime != "" {
		creds["created_time"] = tokenInfo.CreatedTime
	}
	if tokenInfo.ChatGPTAccountID != "" {
		creds["chatgpt_account_id"] = tokenInfo.ChatGPTAccountID
	}
	if tokenInfo.ChatGPTUserID != "" {
		creds["chatgpt_user_id"] = tokenInfo.ChatGPTUserID
	}
	if tokenInfo.OrganizationID != "" {
		creds["organization_id"] = tokenInfo.OrganizationID
	}
	if tokenInfo.TeamName != "" {
		creds["team_name"] = tokenInfo.TeamName
	}
	if tokenInfo.TeamCreatedTime != "" {
		creds["team_created_time"] = tokenInfo.TeamCreatedTime
	}
	if tokenInfo.TeamOrganizationID != "" {
		creds["team_organization_id"] = tokenInfo.TeamOrganizationID
	}
	if tokenInfo.TeamAccountID != "" {
		creds["team_account_id"] = tokenInfo.TeamAccountID
	}
	if tokenInfo.TeamPlanType != "" {
		creds["team_plan_type"] = tokenInfo.TeamPlanType
	}
	if tokenInfo.TeamWorkspaceType != "" {
		creds["team_workspace_type"] = tokenInfo.TeamWorkspaceType
	}
	if tokenInfo.TeamName != "" || tokenInfo.TeamCreatedTime != "" || tokenInfo.TeamOrganizationID != "" || tokenInfo.TeamAccountID != "" {
		creds["team_self_serve_business_prolite"] = tokenInfo.TeamSelfServeBusinessProlite
	}
	if tokenInfo.PlanType != "" {
		creds["plan_type"] = tokenInfo.PlanType
	}
	if tokenInfo.SubscriptionExpiresAt != "" {
		creds["subscription_expires_at"] = tokenInfo.SubscriptionExpiresAt
	}
	if strings.TrimSpace(tokenInfo.ClientID) != "" {
		creds["client_id"] = strings.TrimSpace(tokenInfo.ClientID)
	}
	if tokenInfo.AuthMode == OpenAIAuthModePersonalAccessToken {
		creds[openAIAuthModeCredentialKey] = OpenAIAuthModePersonalAccessToken
		creds[openAIAuthModeLegacyCredentialKey] = "personal_access_token"
		creds["token_type"] = "Bearer"
		creds["chatgpt_account_is_fedramp"] = tokenInfo.ChatGPTAccountFedRAMP
	} else if tokenInfo.ChatGPTAccountFedRAMP {
		creds["chatgpt_account_is_fedramp"] = true
	}

	return NormalizeOpenAIPersonalAccessTokenCredentials(nil, tokenInfo, creds)
}

// Stop stops the session store cleanup goroutine
func (s *OpenAIOAuthService) Stop() {
	s.sessionStore.Stop()
}

func normalizeOpenAIOAuthPlatform(platform string) string {
	return openai.OAuthPlatformOpenAI
}
