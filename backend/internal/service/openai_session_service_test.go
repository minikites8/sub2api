package service

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/stretchr/testify/require"
)

func newOpenAISessionTestService(t *testing.T, handler http.Handler) *OpenAISessionService {
	t.Helper()
	account := &Account{
		ID:       42,
		Platform: PlatformOpenAI,
		Type:     AccountTypeOAuth,
		Status:   StatusActive,
		Credentials: map[string]any{
			"access_token": "cached-token",
		},
	}
	repo := &stubQuotaAccountRepo{accounts: map[int64]*Account{account.ID: account}}
	tokenCache := &stubQuotaTokenCache{tokens: map[string]string{
		OpenAITokenCacheKey(account): "cached-token",
	}}
	tokenProvider := NewOpenAITokenProvider(repo, tokenCache, nil)
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	return NewOpenAISessionService(repo, nil, tokenProvider, newQuotaRedirectingFactory(server))
}

func TestOpenAISessionServiceListSessionsUsesBearerTokenAndSanitizesDevices(t *testing.T) {
	var gotAuthorization string
	svc := newOpenAISessionTestService(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuthorization = r.Header.Get("Authorization")
		require.Equal(t, http.MethodGet, r.Method)
		require.Equal(t, "/backend-api/accounts/sessions", r.URL.Path)
		_, _ = io.WriteString(w, `{"show_session_manager":true,"devices":[{"display_name":"Desktop","session_id":"session-1","last_signed_in_timestamp_second":-4,"app_sessions":[{"client_name":"Codex"}]}]}`)
	}))

	result, err := svc.ListSessions(context.Background(), 42)
	require.NoError(t, err)
	require.Equal(t, "Bearer cached-token", gotAuthorization)
	require.True(t, result.ShowSessionManager)
	require.Len(t, result.Devices, 1)
	require.Equal(t, "session-1", result.Devices[0].SessionID)
	require.Zero(t, result.Devices[0].LastSignedInTimestampSecond)
	require.Equal(t, "Codex", result.Devices[0].AppSessions[0].ClientName)
	require.Positive(t, result.FetchedAt)
}

func TestOpenAISessionServiceRevokeSessionSendsSessionID(t *testing.T) {
	var gotAuthorization string
	var gotSessionID string
	svc := newOpenAISessionTestService(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuthorization = r.Header.Get("Authorization")
		require.Equal(t, http.MethodPost, r.Method)
		require.Equal(t, "/backend-api/accounts/sessions/revoke", r.URL.Path)
		body, readErr := io.ReadAll(r.Body)
		require.NoError(t, readErr)
		var payload struct {
			SessionID string `json:"session_id"`
		}
		require.NoError(t, json.Unmarshal(body, &payload))
		gotSessionID = payload.SessionID
		_, _ = io.WriteString(w, `{"success":true}`)
	}))

	result, err := svc.RevokeSession(context.Background(), 42, " session-2 ")
	require.NoError(t, err)
	require.Equal(t, "Bearer cached-token", gotAuthorization)
	require.Equal(t, "session-2", gotSessionID)
	require.Equal(t, "session-2", result.SessionID)
	require.True(t, result.Revoked)
}

func TestOpenAISessionServiceRejectsInvalidSessionID(t *testing.T) {
	svc := newOpenAISessionTestService(t, http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		t.Fatal("upstream should not be called")
	}))

	_, err := svc.RevokeSession(context.Background(), 42, strings.Repeat("x", openAISessionIDMaxLength+1))
	require.Error(t, err)
	status, reason := infraerrorsForTest(err)
	require.Equal(t, http.StatusBadRequest, status)
	require.Equal(t, "OPENAI_SESSION_ID_INVALID", reason)
}

func infraerrorsForTest(err error) (int, string) {
	status, apiStatus := infraerrors.ToHTTP(err)
	return status, apiStatus.Reason
}
