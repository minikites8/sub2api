package service

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/tlsfingerprint"
	"github.com/Wei-Shaw/sub2api/internal/pkg/yeteam"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type yeTeamAccountTestRepo struct {
	AccountRepository
	clearedErrorID     int64
	setErrorID         int64
	setErrorMsg        string
	updatedCredentials map[string]any
	updatedExtra       map[string]any
	setSchedulableID   int64
	setSchedulable     bool
	restoreOperations  []string
}

func (r *yeTeamAccountTestRepo) ClearError(_ context.Context, id int64) error {
	r.clearedErrorID = id
	r.restoreOperations = append(r.restoreOperations, "clear_error")
	return nil
}

func (r *yeTeamAccountTestRepo) SetError(_ context.Context, id int64, errorMsg string) error {
	r.setErrorID = id
	r.setErrorMsg = errorMsg
	return nil
}

func (r *yeTeamAccountTestRepo) UpdateCredentials(_ context.Context, _ int64, credentials map[string]any) error {
	r.updatedCredentials = shallowCopyMap(credentials)
	return nil
}

func (r *yeTeamAccountTestRepo) UpdateExtra(_ context.Context, _ int64, updates map[string]any) error {
	if r.updatedExtra == nil {
		r.updatedExtra = make(map[string]any, len(updates))
	}
	for key, value := range updates {
		r.updatedExtra[key] = value
	}
	return nil
}

func (r *yeTeamAccountTestRepo) SetSchedulable(_ context.Context, id int64, schedulable bool) error {
	r.setSchedulableID = id
	r.setSchedulable = schedulable
	r.restoreOperations = append(r.restoreOperations, "schedulable")
	return nil
}

type yeTeamAccountTestUpstream struct {
	responses []*http.Response
	requests  []*http.Request
	onRequest func(call int)
}

func (u *yeTeamAccountTestUpstream) Do(_ *http.Request, _ string, _ int64, _ int) (*http.Response, error) {
	return nil, fmt.Errorf("unexpected Do call")
}

func (u *yeTeamAccountTestUpstream) DoWithTLS(req *http.Request, _ string, _ int64, _ int, _ *tlsfingerprint.Profile) (*http.Response, error) {
	u.requests = append(u.requests, req)
	if u.onRequest != nil {
		u.onRequest(len(u.requests))
	}
	if len(u.responses) == 0 {
		return nil, fmt.Errorf("no mocked response")
	}
	resp := u.responses[0]
	u.responses = u.responses[1:]
	return resp, nil
}

type yeTeamAccountTestServerState struct {
	healthCalls   atomic.Int32
	batchCalls    atomic.Int32
	downloadCalls atomic.Int32
}

func newYeTeamAccountTestServer(t *testing.T) (*httptest.Server, *yeTeamAccountTestServerState) {
	t.Helper()
	state := &yeTeamAccountTestServerState{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/redeem/reclaim/health-check":
			state.healthCalls.Add(1)
			w.WriteHeader(http.StatusInternalServerError)
		case "/api/redeem/reclaim/batch-cards":
			call := state.batchCalls.Add(1)
			if call == 1 {
				_, _ = w.Write([]byte(`{"ok":true,"queued":0,"already_running":1,"cards":[{"card_code":"TEAM-TEST-401","tasks":[{"order_no":"ord-401","resource_uid":"acct-1","status":"pending"}]}]}`))
				return
			}
			_, _ = w.Write([]byte(`{"ok":true,"queued":0,"already_running":0,"done":1,"cards":[{"card_code":"TEAM-TEST-401","tasks":[{"order_no":"ord-401","resource_uid":"acct-1","status":"done","download_token":"tok-401"}]}]}`))
		case "/api/redeem/batch-download":
			state.downloadCalls.Add(1)
			_, _ = w.Write([]byte(`{"accounts":[{"name":"account@example.com","credentials":{"access_token":"new-token","chatgpt_account_id":"acct-1"}}]}`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	return server, state
}

func newYeTeamAccountTestContext() (*gin.Context, *httptest.ResponseRecorder) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/api/v1/admin/accounts/1/test", nil)
	return c, recorder
}

func newYeTeamAccountTestResponse(status int, body string) *http.Response {
	return &http.Response{StatusCode: status, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(body))}
}

func newYeTeamAccountTestService(t *testing.T, responses ...*http.Response) (*AccountTestService, *yeTeamAccountTestRepo, *yeTeamAccountTestUpstream, *yeTeamAccountTestServerState) {
	t.Helper()
	server, state := newYeTeamAccountTestServer(t)
	t.Cleanup(server.Close)
	repo := &yeTeamAccountTestRepo{}
	upstream := &yeTeamAccountTestUpstream{responses: responses}
	client := yeteam.NewClient(yeteam.Config{
		Enabled:         true,
		AutoRefresh401:  true,
		BaseURL:         server.URL,
		Timeout:         time.Second,
		PollInterval:    time.Millisecond,
		MaxPollDuration: time.Second,
	})
	gateway := &OpenAIGatewayService{accountRepo: repo}
	gateway.SetYeTeamClient(client)
	return &AccountTestService{accountRepo: repo, httpUpstream: upstream, openAIGatewayService: gateway}, repo, upstream, state
}

func newYeTeamRefreshAccount(id int64) *Account {
	return &Account{
		ID:           id,
		Name:         "account@example.com",
		Platform:     PlatformOpenAI,
		Type:         AccountTypeOAuth,
		Status:       StatusError,
		Schedulable:  false,
		Concurrency:  1,
		Credentials:  map[string]any{"access_token": "old-token", "chatgpt_account_id": "acct-1"},
		Extra:        map[string]any{"ye_team_card_code": "team-test-401"},
		ErrorMessage: "Authentication failed (401)",
	}
}

func TestAccountTestServiceYeTeam401RefreshesAndRetries(t *testing.T) {
	c, recorder := newYeTeamAccountTestContext()
	success := newYeTeamAccountTestResponse(http.StatusOK, "data: {\"type\":\"response.completed\"}\n\n")
	svc, repo, upstream, state := newYeTeamAccountTestService(t,
		newYeTeamAccountTestResponse(http.StatusUnauthorized, `{"error":"expired token"}`),
		success,
	)
	account := newYeTeamRefreshAccount(80)

	err := svc.testOpenAIAccountConnection(c, account, "gpt-5.4", "", "")
	require.NoError(t, err)
	require.Len(t, upstream.requests, 2)
	require.Equal(t, "Bearer old-token", upstream.requests[0].Header.Get("Authorization"))
	require.Equal(t, "Bearer new-token", upstream.requests[1].Header.Get("Authorization"))
	require.Zero(t, state.healthCalls.Load())
	require.Equal(t, int32(2), state.batchCalls.Load())
	require.Equal(t, int32(1), state.downloadCalls.Load())
	require.Equal(t, "new-token", repo.updatedCredentials["access_token"])
	require.Equal(t, yeTeamRefreshStatusSuccess, repo.updatedExtra[yeTeamLastRefreshStatusKey])
	require.NotEmpty(t, repo.updatedExtra[yeTeamLastRefreshAtKey])
	require.Empty(t, repo.updatedExtra[yeTeamLastRefreshErrorKey])
	require.Zero(t, repo.setErrorID)
	require.Equal(t, account.ID, repo.clearedErrorID)
	require.Equal(t, account.ID, repo.setSchedulableID)
	require.True(t, repo.setSchedulable)
	require.Equal(t, []string{"schedulable", "clear_error"}, repo.restoreOperations)
	require.Equal(t, StatusActive, account.Status)
	require.True(t, account.Schedulable)
	require.Empty(t, account.ErrorMessage)
	require.Contains(t, recorder.Body.String(), "ye.team 凭据刷新成功")
	require.Contains(t, recorder.Body.String(), `"success":true`)
}

func TestAccountTestServiceYeTeam401RefreshRetriesOnce(t *testing.T) {
	c, _ := newYeTeamAccountTestContext()
	svc, repo, upstream, state := newYeTeamAccountTestService(t,
		newYeTeamAccountTestResponse(http.StatusUnauthorized, `{"error":"expired token"}`),
		newYeTeamAccountTestResponse(http.StatusUnauthorized, `{"error":"replacement rejected"}`),
	)
	account := newYeTeamRefreshAccount(81)

	err := svc.testOpenAIAccountConnection(c, account, "gpt-5.4", "", "")
	require.Error(t, err)
	require.Len(t, upstream.requests, 2)
	require.Zero(t, state.healthCalls.Load())
	require.Equal(t, int32(2), state.batchCalls.Load())
	require.Equal(t, int32(1), state.downloadCalls.Load())
	require.Equal(t, account.ID, repo.setErrorID)
	require.Contains(t, repo.setErrorMsg, "replacement rejected")
}

func TestAccountTestServiceYeTeam401AcceptsHealthyUnchangedCredential(t *testing.T) {
	c, recorder := newYeTeamAccountTestContext()
	success := newYeTeamAccountTestResponse(http.StatusOK, "data: {\"type\":\"response.completed\"}\n\n")
	svc, repo, upstream, state := newYeTeamAccountTestService(t,
		newYeTeamAccountTestResponse(http.StatusUnauthorized, `{"error":"expired token"}`),
		success,
	)
	account := newYeTeamRefreshAccount(85)
	account.Credentials["access_token"] = "new-token"

	err := svc.testOpenAIAccountConnection(c, account, "gpt-5.4", "", "")
	require.NoError(t, err)
	require.Len(t, upstream.requests, 2)
	require.Equal(t, "Bearer new-token", upstream.requests[0].Header.Get("Authorization"))
	require.Equal(t, "Bearer new-token", upstream.requests[1].Header.Get("Authorization"))
	require.Zero(t, state.healthCalls.Load())
	require.Equal(t, int32(2), state.batchCalls.Load())
	require.Equal(t, int32(1), state.downloadCalls.Load())
	require.Equal(t, yeTeamRefreshStatusSuccess, repo.updatedExtra[yeTeamLastRefreshStatusKey])
	require.Empty(t, repo.updatedExtra[yeTeamLastRefreshErrorKey])
	require.Contains(t, recorder.Body.String(), `"success":true`)
}

func TestAccountTestServiceYeTeam401RefreshSurvivesRequestCancellation(t *testing.T) {
	c, _ := newYeTeamAccountTestContext()
	requestCtx, cancelRequest := context.WithCancel(c.Request.Context())
	c.Request = c.Request.WithContext(requestCtx)
	svc, repo, upstream, state := newYeTeamAccountTestService(t,
		newYeTeamAccountTestResponse(http.StatusUnauthorized, `{"error":"expired token"}`),
		newYeTeamAccountTestResponse(http.StatusOK, "data: {\"type\":\"response.completed\"}\n\n"),
	)
	upstream.onRequest = func(call int) {
		if call == 1 {
			cancelRequest()
		}
	}
	account := newYeTeamRefreshAccount(83)

	err := svc.testOpenAIAccountConnection(c, account, "gpt-5.4", "", "")
	require.NoError(t, err)
	require.ErrorIs(t, requestCtx.Err(), context.Canceled)
	require.Len(t, upstream.requests, 2)
	require.Equal(t, "Bearer old-token", upstream.requests[0].Header.Get("Authorization"))
	require.Equal(t, "Bearer new-token", upstream.requests[1].Header.Get("Authorization"))
	require.Equal(t, "new-token", repo.updatedCredentials["access_token"])
	require.Equal(t, yeTeamRefreshStatusSuccess, repo.updatedExtra[yeTeamLastRefreshStatusKey])
	require.Zero(t, state.healthCalls.Load())
	require.Equal(t, int32(1), state.downloadCalls.Load())
}
