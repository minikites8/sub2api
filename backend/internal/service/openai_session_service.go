package service

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"time"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
)

const (
	chatGPTSessionsURL           = "https://chatgpt.com/backend-api/accounts/sessions"
	chatGPTSessionRevokeURL      = "https://chatgpt.com/backend-api/accounts/sessions/revoke"
	openAISessionUpstreamTimeout = 20 * time.Second
	openAISessionDeviceLimit     = 200
	openAIAppSessionLimit        = 50
	openAISessionIDMaxLength     = 500
)

// OpenAIAppSession identifies one ChatGPT client attached to a device session.
type OpenAIAppSession struct {
	ClientName string `json:"client_name,omitempty"`
}

// OpenAISessionDevice is the device projection exposed to administrators.
type OpenAISessionDevice struct {
	RenderID                    string             `json:"render_id,omitempty"`
	DisplayName                 string             `json:"display_name,omitempty"`
	HumanReadableDescription    string             `json:"human_readable_description,omitempty"`
	Platform                    string             `json:"platform,omitempty"`
	OSVersion                   string             `json:"os_version,omitempty"`
	DeviceModel                 string             `json:"device_model,omitempty"`
	IsTrustedDevice             bool               `json:"is_trusted_device"`
	IsCurrentDevice             bool               `json:"is_current_device"`
	CanUntrust                  bool               `json:"can_untrust"`
	HashedDeviceID              string             `json:"hashed_device_id,omitempty"`
	SessionID                   string             `json:"session_id,omitempty"`
	LastSignedInTimestampSecond int64              `json:"last_signed_in_timestamp_second"`
	LastSignedInCity            string             `json:"last_signed_in_city,omitempty"`
	LastSignedInRegionCode      string             `json:"last_signed_in_region_code,omitempty"`
	LastSignedInCountry         string             `json:"last_signed_in_country,omitempty"`
	AppSessions                 []OpenAIAppSession `json:"app_sessions,omitempty"`
}

// OpenAISessionsResponse contains the current ChatGPT device sessions.
type OpenAISessionsResponse struct {
	ShowSessionManager bool                  `json:"show_session_manager"`
	Devices            []OpenAISessionDevice `json:"devices"`
	FetchedAt          int64                 `json:"fetched_at"`
}

// OpenAISessionRevokeResult reports the session removed upstream.
type OpenAISessionRevokeResult struct {
	SessionID string `json:"session_id"`
	Revoked   bool   `json:"revoked"`
}

// OpenAISessionService lists and revokes ChatGPT device sessions for OpenAI
// OAuth accounts through the account's refreshed access token and proxy.
type OpenAISessionService struct {
	accountRepo          AccountRepository
	proxyRepo            ProxyRepository
	tokenProvider        *OpenAITokenProvider
	privacyClientFactory PrivacyClientFactory
}

func NewOpenAISessionService(
	accountRepo AccountRepository,
	proxyRepo ProxyRepository,
	tokenProvider *OpenAITokenProvider,
	privacyClientFactory PrivacyClientFactory,
) *OpenAISessionService {
	return &OpenAISessionService{
		accountRepo:          accountRepo,
		proxyRepo:            proxyRepo,
		tokenProvider:        tokenProvider,
		privacyClientFactory: privacyClientFactory,
	}
}

func (s *OpenAISessionService) ListSessions(ctx context.Context, accountID int64) (*OpenAISessionsResponse, error) {
	accessToken, proxyURL, err := s.prepareUpstreamCall(ctx, accountID)
	if err != nil {
		return nil, err
	}
	client, err := s.privacyClientFactory(proxyURL)
	if err != nil {
		return nil, infraerrors.Newf(http.StatusBadGateway, "OPENAI_SESSIONS_CLIENT_ERROR", "failed to build upstream client: %v", err)
	}

	callCtx, cancel := context.WithTimeout(ctx, openAISessionUpstreamTimeout)
	defer cancel()

	var payload OpenAISessionsResponse
	resp, err := client.R().
		SetContext(callCtx).
		SetHeaders(openAISessionHeaders(accessToken)).
		SetSuccessResult(&payload).
		Get(chatGPTSessionsURL)
	if err != nil {
		return nil, infraerrors.Newf(http.StatusBadGateway, "OPENAI_SESSIONS_REQUEST_FAILED", "upstream request failed: %v", err)
	}
	if !resp.IsSuccessState() {
		return nil, openAISessionUpstreamError(resp.StatusCode, resp.String(), "OPENAI_SESSIONS_UPSTREAM_ERROR")
	}

	sanitizeOpenAISessionsResponse(&payload)
	payload.FetchedAt = time.Now().Unix()
	return &payload, nil
}

func (s *OpenAISessionService) RevokeSession(ctx context.Context, accountID int64, sessionID string) (*OpenAISessionRevokeResult, error) {
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return nil, infraerrors.New(http.StatusBadRequest, "OPENAI_SESSION_ID_REQUIRED", "session_id is required")
	}
	if len(sessionID) > openAISessionIDMaxLength {
		return nil, infraerrors.New(http.StatusBadRequest, "OPENAI_SESSION_ID_INVALID", "session_id is too long")
	}

	accessToken, proxyURL, err := s.prepareUpstreamCall(ctx, accountID)
	if err != nil {
		return nil, err
	}
	client, err := s.privacyClientFactory(proxyURL)
	if err != nil {
		return nil, infraerrors.Newf(http.StatusBadGateway, "OPENAI_SESSIONS_CLIENT_ERROR", "failed to build upstream client: %v", err)
	}

	callCtx, cancel := context.WithTimeout(ctx, openAISessionUpstreamTimeout)
	defer cancel()

	resp, err := client.R().
		SetContext(callCtx).
		SetHeaders(openAISessionHeaders(accessToken)).
		SetBody(struct {
			SessionID string `json:"session_id"`
		}{SessionID: sessionID}).
		Post(chatGPTSessionRevokeURL)
	if err != nil {
		return nil, infraerrors.Newf(http.StatusBadGateway, "OPENAI_SESSION_REVOKE_REQUEST_FAILED", "upstream request failed: %v", err)
	}
	if !resp.IsSuccessState() {
		return nil, openAISessionUpstreamError(resp.StatusCode, resp.String(), "OPENAI_SESSION_REVOKE_UPSTREAM_ERROR")
	}

	return &OpenAISessionRevokeResult{SessionID: sessionID, Revoked: true}, nil
}

func (s *OpenAISessionService) prepareUpstreamCall(ctx context.Context, accountID int64) (accessToken, proxyURL string, err error) {
	if s == nil || s.accountRepo == nil || s.tokenProvider == nil || s.privacyClientFactory == nil {
		return "", "", infraerrors.New(http.StatusInternalServerError, "OPENAI_SESSIONS_NOT_CONFIGURED", "openai session service is not configured")
	}

	account, err := s.accountRepo.GetByID(ctx, accountID)
	if err != nil {
		return "", "", infraerrors.Newf(http.StatusNotFound, "OPENAI_SESSIONS_ACCOUNT_NOT_FOUND", "account not found: %v", err)
	}
	if account == nil {
		return "", "", infraerrors.New(http.StatusNotFound, "OPENAI_SESSIONS_ACCOUNT_NOT_FOUND", "account not found")
	}
	if account.Platform != PlatformOpenAI {
		return "", "", infraerrors.New(http.StatusBadRequest, "OPENAI_SESSIONS_INVALID_PLATFORM", "account is not an OpenAI account")
	}
	if account.Type != AccountTypeOAuth {
		return "", "", infraerrors.New(http.StatusBadRequest, "OPENAI_SESSIONS_INVALID_TYPE", "account is not an OAuth account")
	}
	if account.IsShadow() {
		account, err = resolveCredentialAccount(ctx, s.accountRepo, account)
		if err != nil {
			return "", "", infraerrors.Newf(http.StatusBadGateway, "OPENAI_SESSIONS_SHADOW_RESOLVE_FAILED", "failed to resolve shadow account: %v", err)
		}
	}

	accessToken, err = s.tokenProvider.GetAccessToken(ctx, account)
	if err != nil {
		return "", "", infraerrors.Newf(http.StatusBadGateway, "OPENAI_SESSIONS_TOKEN_UNAVAILABLE", "failed to acquire access token: %v", err)
	}
	accessToken = strings.TrimSpace(accessToken)
	if accessToken == "" {
		return "", "", infraerrors.New(http.StatusBadGateway, "OPENAI_SESSIONS_TOKEN_UNAVAILABLE", "access token is empty")
	}

	if account.ProxyID != nil {
		switch {
		case account.Proxy != nil:
			proxyURL = account.Proxy.URL()
		case s.proxyRepo != nil:
			if proxy, proxyErr := s.proxyRepo.GetByID(ctx, *account.ProxyID); proxyErr == nil && proxy != nil {
				proxyURL = proxy.URL()
			}
		}
	}
	return accessToken, proxyURL, nil
}

func openAISessionHeaders(accessToken string) map[string]string {
	return map[string]string{
		"authorization": "Bearer " + accessToken,
		"accept":        "application/json",
		"content-type":  "application/json",
		"cache-control": "no-store",
	}
}

func sanitizeOpenAISessionsResponse(payload *OpenAISessionsResponse) {
	if payload == nil {
		return
	}
	if len(payload.Devices) > openAISessionDeviceLimit {
		payload.Devices = payload.Devices[:openAISessionDeviceLimit]
	}
	for i := range payload.Devices {
		device := &payload.Devices[i]
		device.RenderID = truncateOpenAISessionField(device.RenderID, 500)
		device.DisplayName = truncateOpenAISessionField(device.DisplayName, 200)
		device.HumanReadableDescription = truncateOpenAISessionField(device.HumanReadableDescription, 500)
		device.Platform = truncateOpenAISessionField(device.Platform, 100)
		device.OSVersion = truncateOpenAISessionField(device.OSVersion, 100)
		device.DeviceModel = truncateOpenAISessionField(device.DeviceModel, 200)
		device.HashedDeviceID = truncateOpenAISessionField(device.HashedDeviceID, 500)
		device.SessionID = truncateOpenAISessionField(device.SessionID, openAISessionIDMaxLength)
		device.LastSignedInCity = truncateOpenAISessionField(device.LastSignedInCity, 200)
		device.LastSignedInRegionCode = truncateOpenAISessionField(device.LastSignedInRegionCode, 100)
		device.LastSignedInCountry = truncateOpenAISessionField(device.LastSignedInCountry, 100)
		if device.LastSignedInTimestampSecond < 0 {
			device.LastSignedInTimestampSecond = 0
		}
		if len(device.AppSessions) > openAIAppSessionLimit {
			device.AppSessions = device.AppSessions[:openAIAppSessionLimit]
		}
		for j := range device.AppSessions {
			device.AppSessions[j].ClientName = truncateOpenAISessionField(device.AppSessions[j].ClientName, 200)
		}
	}
}

func truncateOpenAISessionField(value string, limit int) string {
	runes := []rune(value)
	if len(runes) <= limit {
		return value
	}
	return string(runes[:limit])
}

func openAISessionUpstreamError(status int, body, code string) error {
	message := http.StatusText(status)
	var payload struct {
		Detail  json.RawMessage `json:"detail"`
		Message string          `json:"message"`
		Error   struct {
			Message string `json:"message"`
		} `json:"error"`
	}
	if json.Unmarshal([]byte(body), &payload) == nil {
		switch {
		case strings.TrimSpace(payload.Error.Message) != "":
			message = payload.Error.Message
		case strings.TrimSpace(payload.Message) != "":
			message = payload.Message
		case len(payload.Detail) > 0:
			var detail string
			if json.Unmarshal(payload.Detail, &detail) == nil && strings.TrimSpace(detail) != "" {
				message = detail
			} else {
				var detailObject struct {
					Message string `json:"message"`
				}
				if json.Unmarshal(payload.Detail, &detailObject) == nil && strings.TrimSpace(detailObject.Message) != "" {
					message = detailObject.Message
				}
			}
		}
	}
	message = truncateOpenAISessionField(sanitizeUpstreamErrorMessage(message), 240)
	return infraerrors.Newf(mapUpstreamStatus(status), code, "upstream returned %d: %s", status, message)
}
