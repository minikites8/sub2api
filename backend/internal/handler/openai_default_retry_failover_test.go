//go:build unit

package handler

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"context"
	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/openai"
	middleware "github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

// Exercise the complete HTTP handler, production selector, account admission,
// native transport and parser. A helper calling a closure twice cannot prove
// that real native cooldown/admission gates allow a second submission.
func TestOpenAIDefaultRetryActuallySubmitsTwicePerAccount(t *testing.T) {
	for _, tc := range []struct {
		name, kind string
		status     int
		pool       bool
	}{
		{"oauth generic502", service.AccountTypeOAuth, 502, false},
		{"oauth generic503", service.AccountTypeOAuth, 503, false},
		{"oauth capacity503", service.AccountTypeOAuth, 503, false},
		{"api key default pool503", service.AccountTypeAPIKey, 503, true},
		{"api key ordinary503", service.AccountTypeAPIKey, 503, false},
	} {
		for _, stream := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/stream=%t", tc.name, stream), func(t *testing.T) {
				var accounts []service.Account
				for _, id := range []int64{801, 802} {
					credentials := map[string]any{"access_token": "test-access", "expires_at": time.Now().Add(time.Hour).UTC().Format(time.RFC3339)}
					extra := map[string]any{"openai_passthrough": true}
					if tc.kind == service.AccountTypeAPIKey {
						credentials = map[string]any{"api_key": "test-key", "base_url": "https://api.openai.com", "pool_mode": tc.pool}
						extra = map[string]any{"openai_passthrough": false}
					}
					accounts = append(accounts, service.Account{ID: id, Name: fmt.Sprint(id), Platform: service.PlatformOpenAI, Type: tc.kind, Status: service.StatusActive, Schedulable: true, Concurrency: 1, Priority: int(id), Credentials: credentials, Extra: extra})
				}
				failureBody := `{"error":{"type":"server_error","message":"Upstream request failed"}}`
				if strings.Contains(tc.name, "capacity") {
					failureBody = `{"error":{"type":"server_error","code":"server_is_overloaded","message":"Our servers are currently overloaded. Please try again later."}}`
				}
				upstream := &excelBPSFailoverUpstream{answer: func(call int) *http.Response {
					if call < 3 {
						return &http.Response{StatusCode: tc.status, Header: http.Header{"Content-Type": {"application/json"}}, Body: io.NopCloser(strings.NewReader(failureBody))}
					}
					return excelBPSCompleted()
				}}
				router := newOpenAI502503RouterWithAccounts(t, upstream, accounts)
				body := fmt.Sprintf(`{"model":"gpt-5.2","input":[{"role":"user","content":"hello"}],"stream":%t}`, stream)
				rec := httptest.NewRecorder()
				req := httptest.NewRequest("POST", "/v1/responses", strings.NewReader(body))
				req.Header.Set("Content-Type", "application/json")
				router.ServeHTTP(rec, req)
				require.Equal(t, []int64{801, 801, 802, 802}, upstream.calls(), "a native runtime cooldown must not silently turn the promised same-account retry into re-selection: %s", rec.Body.String())
				require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
				require.Contains(t, rec.Body.String(), "done")
				for i, url := range upstream.urls {
					require.NotContains(t, url, "/basispoints/")
					require.Equal(t, gjson.GetBytes(upstream.bodies[0], "input").Raw, gjson.GetBytes(upstream.bodies[i], "input").Raw, "history must remain unchanged")
				}
			})
		}
	}
}

func TestOpenAIDefaultRetryPreservesNativeAgentNoReplay(t *testing.T) {
	for _, toolResult := range []bool{false, true} {
		t.Run(fmt.Sprint(toolResult), func(t *testing.T) {
			account := service.Account{ID: 801, Name: "native", Platform: service.PlatformOpenAI, Type: service.AccountTypeOAuth, Status: service.StatusActive, Schedulable: true, Concurrency: 1, Credentials: map[string]any{"access_token": "test-access", "expires_at": time.Now().Add(time.Hour).UTC().Format(time.RFC3339)}, Extra: map[string]any{"openai_passthrough": false}}
			cfg := &config.Config{RunMode: config.RunModeSimple}
			cfg.Gateway.DisableEncryptedAgentOwnerBinding = true
			upstream := &excelBPSFailoverUpstream{answer: func(int) *http.Response {
				return &http.Response{StatusCode: 503, Header: http.Header{"Content-Type": {"application/json"}}, Body: io.NopCloser(strings.NewReader(`{"error":{"type":"server_error","message":"Upstream request failed"}}`))}
			}}
			router := newOpenAIDefaultRetryRouter(t, upstream, []service.Account{account}, cfg)
			input := `[{"type":"agent_message","author":"fixture-parent","recipient":"fixture-child","content":[{"type":"input_text","text":"fixture envelope"},{"type":"encrypted_content","encrypted_content":"fixture-opaque-reference"}]}]`
			if toolResult {
				input = strings.TrimSuffix(input, "]") + `,{"type":"function_call_output","call_id":"call_fixture","output":"tool finished"},{"type":"item_reference","id":"call_fixture"}]`
			}
			body := `{"model":"gpt-5.2","input":` + input + `,"stream":false}`
			req := httptest.NewRequest("POST", "/v1/responses", strings.NewReader(body))
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("Session-Id", "native-no-replay")
			rec := httptest.NewRecorder()
			router.ServeHTTP(rec, req)
			if toolResult {
				// The existing native normalizer rejects this first-seen opaque
				// tool-result shape before dispatch. The retry wrapper must not
				// bypass that wire-integrity gate to make the fixture send.
				require.Empty(t, upstream.calls())
				require.Equal(t, 409, rec.Code)
				require.Equal(t, "encrypted_agent_recovery_wire_changed", gjson.GetBytes(rec.Body.Bytes(), "error.code").String())
				return
			}
			require.Equal(t, []int64{801}, upstream.calls(), "an opaque native continuation permits one intact attempt, never a wrapper retry: %s", rec.Body.String())
			require.Equal(t, 502, rec.Code, rec.Body.String())
			require.Len(t, upstream.bodies, 1)
			require.JSONEq(t, gjson.Get(body, "input").Raw, gjson.GetBytes(upstream.bodies[0], "input").Raw, "opaque/task/tool result input must remain intact")
		})
	}
}

func newOpenAIDefaultRetryRouter(t *testing.T, upstream service.HTTPUpstream, accounts []service.Account, cfg *config.Config, options ...service.OpenAIGatewayOption) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)
	repo := &openAI502503AdmissionRepo{grokCredentialHandlerRepo: &grokCredentialHandlerRepo{accounts: accounts}}
	cfg.Gateway.MaxAccountSwitches = 3
	billing := service.NewBillingCacheService(nil, nil, nil, nil, nil, nil, cfg, nil)
	t.Cleanup(billing.Stop)
	gateway := service.NewOpenAIGatewayService(repo, nil, nil, nil, nil, nil, nil, nil, cfg, nil, nil,
		service.NewBillingService(cfg, nil), nil, billing, upstream, &service.DeferredService{}, nil, nil, nil, nil, nil, nil, nil, nil, options...)
	cache := &concurrencyCacheMock{
		acquireUserSlotFn:    func(context.Context, int64, int, string) (bool, error) { return true, nil },
		acquireAccountSlotFn: func(context.Context, int64, int, string) (bool, error) { return true, nil },
	}
	h := NewOpenAIGatewayHandler(gateway, service.NewConcurrencyService(cache), billing, &service.APIKeyService{}, nil, nil, nil, nil, nil, cfg)
	groupID := int64(901)
	key := &service.APIKey{AllowLongContext: true, ID: 902, GroupID: &groupID, User: &service.User{ID: 903, Status: service.StatusActive},
		Group: &service.Group{ID: groupID, Platform: service.PlatformOpenAI, Status: service.StatusActive, AllowMessagesDispatch: true}}
	r := gin.New()
	r.Use(func(c *gin.Context) {
		c.Set(string(middleware.ContextKeyAPIKey), key)
		c.Set(string(middleware.ContextKeyUser), middleware.AuthSubject{UserID: key.User.ID, Concurrency: 1})
		c.Next()
		if c.Writer.Status() >= 400 {
			if raw, exists := c.Get(service.OpsUpstreamErrorsKey); exists {
				if events, ok := raw.([]*service.OpsUpstreamErrorEvent); ok {
					for _, event := range events {
						t.Logf("native upstream error: %+v", *event)
					}
				}
			}
		}
	})
	r.POST("/v1/responses", h.Responses)
	r.POST("/v1/messages", h.Messages)
	r.POST("/v1/chat/completions", h.ChatCompletions)
	return r
}

type nativeRetryConnectionRepo struct {
	service.OpenAIOAuthReauthRepository
	service.OpenAIAuthConnectionRepository
	rows             map[int64]*service.OpenAIAuthConnection
	alternate        *service.OpenAIAuthConnection
	affinityOverride string
	writes           int
}

func (r *nativeRetryConnectionRepo) AuthConnectionsManaged(context.Context, int64) (bool, error) {
	return true, nil
}
func (r *nativeRetryConnectionRepo) ListAuthConnections(_ context.Context, id int64) ([]*service.OpenAIAuthConnection, error) {
	row := *r.rows[id]
	result := []*service.OpenAIAuthConnection{&row}
	if r.alternate != nil && r.alternate.AccountID == id {
		copied := *r.alternate
		result = append(result, &copied)
	}
	return result, nil
}
func (r *nativeRetryConnectionRepo) ResolveAuthConnectionAffinity(_ context.Context, _ int64, _ int64, _ string, id string) (string, error) {
	if r.affinityOverride != "" {
		return r.affinityOverride, nil
	}
	return id, nil
}
func (r *nativeRetryConnectionRepo) AcquireAuthConnectionLeaseProof(_ context.Context, id int64, _ string, owner string) (*service.OpenAIAuthConnectionLease, error) {
	row := r.rows[id]
	row.LeaseOwner = owner
	row.LeaseEpoch++
	return &service.OpenAIAuthConnectionLease{Owner: owner, Epoch: row.LeaseEpoch}, nil
}
func (r *nativeRetryConnectionRepo) ReleaseAuthConnectionLease(context.Context, int64, string, string) error {
	return nil
}
func (r *nativeRetryConnectionRepo) CASAuthConnection(_ context.Context, row *service.OpenAIAuthConnection, revision int64) (bool, error) {
	if r.rows[row.AccountID].Revision != revision {
		return false, nil
	}
	copied := *row
	copied.Revision++
	r.rows[row.AccountID] = &copied
	r.writes++
	return true, nil
}

type nativeRetryAccountReader struct{ accounts map[int64]*service.Account }

func (r nativeRetryAccountReader) GetAccount(_ context.Context, id int64) (*service.Account, error) {
	return r.accounts[id], nil
}
func (r nativeRetryAccountReader) GetProxy(context.Context, int64) (*service.Proxy, error) {
	return nil, nil
}

type nativeRetryEncryptor struct{}

func (nativeRetryEncryptor) Encrypt(value string) (string, error) { return value, nil }
func (nativeRetryEncryptor) Decrypt(value string) (string, error) { return value, nil }

func TestOpenAIDefaultRetryManagedConnectionDefersCASUntilSecondFailure(t *testing.T) {
	repo, accounts, reauth := nativeRetryManagedFixture(t)
	upstream := &excelBPSFailoverUpstream{answer: func(call int) *http.Response {
		// The second real send must still pass the unchanged connection revision
		// and cooldown gate. The first rejection cannot CAS this row prematurely.
		if call == 1 {
			require.Equal(t, int64(1), repo.rows[801].Revision)
			require.Nil(t, repo.rows[801].CooldownUntil)
			require.Zero(t, repo.writes)
		}
		if call == 3 {
			require.Equal(t, int64(1), repo.rows[802].Revision)
			require.Nil(t, repo.rows[802].CooldownUntil)
			require.Equal(t, 1, repo.writes)
		}
		if call < 3 {
			return &http.Response{StatusCode: 503, Header: http.Header{"Content-Type": {"application/json"}}, Body: io.NopCloser(strings.NewReader(`{"error":{"type":"server_error","message":"Upstream request failed"}}`))}
		}
		return excelBPSCompleted()
	}}
	router := newOpenAIDefaultRetryRouter(t, upstream, accounts, &config.Config{RunMode: config.RunModeSimple}, service.WithOpenAIAuthConnections(reauth))
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/v1/responses", strings.NewReader(`{"model":"gpt-5.2","input":"hello","stream":false}`))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(rec, req)
	require.Equal(t, []int64{801, 801, 802, 802}, upstream.calls(), rec.Body.String())
	require.Equal(t, 200, rec.Code, rec.Body.String())
	require.Equal(t, 1, repo.writes)
	require.Equal(t, int64(2), repo.rows[801].Revision)
	require.NotNil(t, repo.rows[801].CooldownUntil)
	require.Equal(t, int64(1), repo.rows[802].Revision)
	require.Nil(t, repo.rows[802].CooldownUntil)
}

func nativeRetryManagedFixture(t *testing.T) (*nativeRetryConnectionRepo, []service.Account, *service.OpenAIOAuthReauthService) {
	t.Helper()
	repo := &nativeRetryConnectionRepo{rows: make(map[int64]*service.OpenAIAuthConnection)}
	reader := nativeRetryAccountReader{accounts: make(map[int64]*service.Account)}
	var accounts []service.Account
	for _, id := range []int64{801, 802} {
		connectionID := fmt.Sprintf("00000000-0000-4000-8000-%012d", id)
		ciphertext := fmt.Sprintf(`{"_connection_id":%q,"_account_id":%q,"_connection_kind":"codex","_connection_subject":"","client_id":%q,"access_token":%q,"expires_at":%q}`, connectionID, fmt.Sprint(id), openai.ClientID, fmt.Sprintf("connection-token-%d", id), fmt.Sprint(time.Now().Add(time.Hour).Unix()))
		row := &service.OpenAIAuthConnection{ID: connectionID, AccountID: id, Kind: "codex", ClientID: openai.ClientID, Status: "active", RefreshState: "idle", Revision: 1, Preferred: true, Capabilities: []string{"responses", "chat_completions"}, CredentialsCiphertext: ciphertext}
		repo.rows[id] = row
		account := service.Account{ID: id, Name: fmt.Sprint(id), Platform: service.PlatformOpenAI, Type: service.AccountTypeOAuth, Status: service.StatusActive, Schedulable: true, Concurrency: 1, Priority: int(id), Credentials: map[string]any{"access_token": "legacy-must-not-send"}, Extra: map[string]any{"openai_passthrough": true, "openai_connections_managed": true, "openai_auth_connections": []*service.OpenAIAuthConnection{row}}}
		accounts = append(accounts, account)
		copied := account
		reader.accounts[id] = &copied
	}
	reauth := service.NewOpenAIOAuthReauthService(repo, reader, nil, nil, nativeRetryEncryptor{}, true, nil, nil)

	for _, account := range accounts {
		token, err := reauth.ConnectionAccessToken(context.Background(), account.ID, repo.rows[account.ID].ID)
		require.NoError(t, err)
		require.Equal(t, fmt.Sprintf("connection-token-%d", account.ID), token)
	}
	return repo, accounts, reauth
}

type nativeRetryObservedUpstream struct {
	*excelBPSFailoverUpstream
	tokens []string
}

func (u *nativeRetryObservedUpstream) Do(req *http.Request, proxy string, id int64, concurrency int) (*http.Response, error) {
	u.tokens = append(u.tokens, req.Header.Get("Authorization"))
	return u.excelBPSFailoverUpstream.Do(req, proxy, id, concurrency)
}

func TestOpenAIDefaultRetryManagedPinsGrantAcrossPreferenceChange(t *testing.T) {
	repo, accounts, reauth := nativeRetryManagedFixture(t)
	alternate := *repo.rows[801]
	alternate.ID = "00000000-0000-4000-8000-000000009999"
	alternate.Preferred = false
	alternate.CredentialsCiphertext = fmt.Sprintf(`{"_connection_id":%q,"_account_id":"801","_connection_kind":"codex","_connection_subject":"","client_id":%q,"access_token":"alternate-token","expires_at":%q}`, alternate.ID, openai.ClientID, fmt.Sprint(time.Now().Add(time.Hour).Unix()))
	repo.alternate = &alternate
	upstream := &nativeRetryObservedUpstream{excelBPSFailoverUpstream: &excelBPSFailoverUpstream{answer: func(call int) *http.Response {
		if call == 0 {
			repo.rows[801].Preferred = false
			repo.alternate.Preferred = true
			return &http.Response{StatusCode: 503, Header: http.Header{"Content-Type": {"application/json"}}, Body: io.NopCloser(strings.NewReader(`{"error":{"type":"server_error","message":"Upstream request failed"}}`))}
		}
		return excelBPSCompleted()
	}}}
	router := newOpenAIDefaultRetryRouter(t, upstream, accounts[:1], &config.Config{RunMode: config.RunModeSimple}, service.WithOpenAIAuthConnections(reauth))
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/v1/responses", strings.NewReader(`{"model":"gpt-5.2","input":"hello","stream":false}`))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(rec, req)
	require.Equal(t, []int64{801, 801}, upstream.calls(), rec.Body.String())
	require.Equal(t, []string{"Bearer connection-token-801", "Bearer connection-token-801"}, upstream.tokens)
	require.Equal(t, 200, rec.Code, rec.Body.String())
	require.Zero(t, repo.writes)
	require.Nil(t, repo.rows[801].CooldownUntil)
	require.Nil(t, repo.alternate.CooldownUntil)
}

func TestOpenAIDefaultRetryCancellationDuringDelayPersistsConfirmedFailure(t *testing.T) {
	repo, accounts, reauth := nativeRetryManagedFixture(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	upstream := &excelBPSFailoverUpstream{answer: func(int) *http.Response {
		time.AfterFunc(100*time.Millisecond, cancel)
		return &http.Response{StatusCode: 503, Header: http.Header{"Content-Type": {"application/json"}, "Retry-After": {"5"}}, Body: io.NopCloser(strings.NewReader(`{"error":{"type":"server_error","message":"Upstream request failed"}}`))}
	}}
	router := newOpenAIDefaultRetryRouter(t, upstream, accounts[:1], &config.Config{RunMode: config.RunModeSimple}, service.WithOpenAIAuthConnections(reauth))
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/v1/responses", strings.NewReader(`{"model":"gpt-5.2","input":"hello","stream":false}`)).WithContext(ctx)
	req.Header.Set("Content-Type", "application/json")
	started := time.Now()
	router.ServeHTTP(rec, req)
	require.Equal(t, []int64{801}, upstream.calls(), "canceling backoff must not send again")
	require.Less(t, time.Since(started), 2*time.Second)
	require.Equal(t, 1, repo.writes, "persist the observed failure exactly once with a detached context")
	require.Equal(t, int64(2), repo.rows[801].Revision)
	require.NotNil(t, repo.rows[801].CooldownUntil)
}

type nativeRetryEOFUpstream struct {
	service.HTTPUpstream
	hits []int64
}

func (u *nativeRetryEOFUpstream) Do(req *http.Request, _ string, id int64, _ int) (*http.Response, error) {
	_, _ = io.ReadAll(req.Body)
	u.hits = append(u.hits, id)
	return nil, io.EOF
}
func TestOpenAIDefaultRetryDoesNotReplayUncertainTransportSubmission(t *testing.T) {
	upstream := &nativeRetryEOFUpstream{}
	account := service.Account{ID: 801, Platform: service.PlatformOpenAI, Type: service.AccountTypeOAuth, Status: service.StatusActive, Schedulable: true, Concurrency: 1, Credentials: map[string]any{"access_token": "test-access", "expires_at": time.Now().Add(time.Hour).UTC().Format(time.RFC3339)}, Extra: map[string]any{"openai_passthrough": true}}
	router := newOpenAIDefaultRetryRouter(t, upstream, []service.Account{account}, &config.Config{RunMode: config.RunModeSimple})
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/v1/responses", strings.NewReader(`{"model":"gpt-5.2","input":"hello","stream":false}`))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(rec, req)
	require.Equal(t, []int64{801}, upstream.hits, "a read EOF after accepting the request is not an explicit HTTP rejection and cannot trigger the default same-account resubmission")
	require.GreaterOrEqual(t, rec.Code, 400)
}

func TestOpenAIDefaultRetryAffinityChangeCannotReplaceGrantOrLoseFailure(t *testing.T) {
	repo, accounts, reauth := nativeRetryManagedFixture(t)
	alternate := *repo.rows[801]
	alternate.ID = "00000000-0000-4000-8000-000000009999"
	alternate.Preferred = false
	alternate.CredentialsCiphertext = fmt.Sprintf(`{"_connection_id":%q,"_account_id":"801","_connection_kind":"codex","_connection_subject":"","client_id":%q,"access_token":"alternate-token","expires_at":%q}`, alternate.ID, openai.ClientID, fmt.Sprint(time.Now().Add(time.Hour).Unix()))
	repo.alternate = &alternate
	upstream := &nativeRetryObservedUpstream{excelBPSFailoverUpstream: &excelBPSFailoverUpstream{answer: func(int) *http.Response {
		repo.affinityOverride = alternate.ID
		return &http.Response{StatusCode: 503, Header: http.Header{"Content-Type": {"application/json"}}, Body: io.NopCloser(strings.NewReader(`{"error":{"type":"server_error","message":"Upstream request failed"}}`))}
	}}}
	router := newOpenAIDefaultRetryRouter(t, upstream, accounts[:1], &config.Config{RunMode: config.RunModeSimple}, service.WithOpenAIAuthConnections(reauth))
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/v1/responses", strings.NewReader(`{"model":"gpt-5.2","input":"hello","stream":false}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Session-Id", "native-retry-affinity")
	router.ServeHTTP(rec, req)
	require.Equal(t, []int64{801}, upstream.calls(), "same-revision affinity to another grant must fail before a second HTTP send: %s", rec.Body.String())
	require.Equal(t, []string{"Bearer connection-token-801"}, upstream.tokens)
	require.GreaterOrEqual(t, rec.Code, 400)
	require.Equal(t, 1, repo.writes, "the original confirmed 503 survives retry admission failure")
	require.Equal(t, int64(2), repo.rows[801].Revision)
	require.NotNil(t, repo.rows[801].CooldownUntil)
	require.Equal(t, int64(1), repo.alternate.Revision)
	require.Nil(t, repo.alternate.CooldownUntil)
}
