package service

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/yeteam"
	"github.com/stretchr/testify/require"
)

type yeTeamTokenCacheStub struct {
	tokens     map[string]string
	deletedKey string
}

type yeTeamTokenAccountRepo struct {
	yeTeamAccountTestRepo
	account *Account
}

func (r *yeTeamTokenAccountRepo) GetByID(_ context.Context, _ int64) (*Account, error) {
	return r.account, nil
}

type yeTeamDiscardingCredentialRepo struct {
	yeTeamTokenAccountRepo
}

func (r *yeTeamDiscardingCredentialRepo) UpdateCredentials(_ context.Context, _ int64, _ map[string]any) error {
	return nil
}

func (s *yeTeamTokenCacheStub) GetAccessToken(_ context.Context, cacheKey string) (string, error) {
	return s.tokens[cacheKey], nil
}

func (s *yeTeamTokenCacheStub) SetAccessToken(_ context.Context, cacheKey, token string, _ time.Duration) error {
	s.tokens[cacheKey] = token
	return nil
}

func (s *yeTeamTokenCacheStub) DeleteAccessToken(_ context.Context, cacheKey string) error {
	s.deletedKey = cacheKey
	delete(s.tokens, cacheKey)
	return nil
}

func (s *yeTeamTokenCacheStub) AcquireRefreshLock(_ context.Context, _ string, _ time.Duration) (bool, error) {
	return true, nil
}

func (s *yeTeamTokenCacheStub) ReleaseRefreshLock(_ context.Context, _ string) error {
	return nil
}

func TestYeTeamCardCodeSupportsMetadataFormats(t *testing.T) {
	for _, tc := range []struct {
		name string
		want string
	}{
		{name: "RCL metadata", want: "RCL-ABCD-1234"},
		{name: "team metadata", want: "TEAM-C6F73D-UVJS-04D34DE66753"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			account := &Account{Extra: map[string]any{"ye_team_card_code": tc.want}}
			if got := yeTeamCardCode(account); got != tc.want {
				t.Fatalf("yeTeamCardCode() = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestYeTeamCardCodeNameFallbackRemainsRCLOnly(t *testing.T) {
	account := &Account{Name: "team-c6f73d-UVJS-04D34DE66753"}
	if got := yeTeamCardCode(account); got != "" {
		t.Fatalf("yeTeamCardCode() = %q for a name-only team identifier", got)
	}
	account.Name = "account RCL-ABCD-1234"
	if got := yeTeamCardCode(account); got != "RCL-ABCD-1234" {
		t.Fatalf("yeTeamCardCode() = %q, want RCL-ABCD-1234", got)
	}
}

func TestYeTeamReclaimInvalidatesCachedOpenAIToken(t *testing.T) {
	server, state := newYeTeamAccountTestServer(t)
	t.Cleanup(server.Close)

	ctx := context.Background()
	account := newYeTeamRefreshAccount(82)
	staleAccount := *account
	staleAccount.Credentials = shallowCopyMap(account.Credentials)
	repo := &yeTeamTokenAccountRepo{account: account}
	cacheKey := OpenAITokenCacheKey(account)
	cache := &yeTeamTokenCacheStub{tokens: map[string]string{cacheKey: "cached-old-token"}}
	provider := NewOpenAITokenProvider(repo, cache, nil)
	gateway := &OpenAIGatewayService{accountRepo: repo, openAITokenProvider: provider}
	gateway.SetYeTeamClient(yeteam.NewClient(yeteam.Config{
		Enabled:         true,
		AutoRefresh401:  true,
		BaseURL:         server.URL,
		Timeout:         time.Second,
		PollInterval:    time.Millisecond,
		MaxPollDuration: time.Second,
	}))

	before, err := provider.GetAccessToken(ctx, account)
	require.NoError(t, err)
	require.Equal(t, "cached-old-token", before)
	require.True(t, gateway.reclaimOpenAIAccount401(ctx, account))
	require.Equal(t, cacheKey, cache.deletedKey)

	after, err := provider.GetAccessToken(ctx, &staleAccount)
	require.NoError(t, err)
	require.Equal(t, "new-token", after)
	require.Empty(t, cache.tokens[cacheKey])
	require.Positive(t, account.GetCredentialAsInt64("_token_version"))

	after, err = provider.GetAccessToken(ctx, account)
	require.NoError(t, err)
	require.Equal(t, "new-token", after)
	require.Equal(t, "new-token", cache.tokens[cacheKey])
	require.Zero(t, state.healthCalls.Load())
	require.Equal(t, int32(1), state.downloadCalls.Load())
	flow, ok := repo.updatedExtra[yeTeamLastRefreshFlowKey].(yeteam.ReclaimFlow)
	require.True(t, ok)
	require.Equal(t, yeTeamRefreshStatusSuccess, flow.Status)
	require.Contains(t, flowStageNames(flow), "match_credentials")
	require.Contains(t, flowStageNames(flow), "persist_credentials")
	require.Contains(t, flowStageNames(flow), "cache_invalidate")
}

func TestYeTeamReclaimPersistsFailureStatus(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, `{"error":"reclaim service unavailable"}`, http.StatusServiceUnavailable)
	}))
	t.Cleanup(server.Close)

	account := newYeTeamRefreshAccount(84)
	repo := &yeTeamTokenAccountRepo{account: account}
	gateway := &OpenAIGatewayService{accountRepo: repo}
	gateway.SetYeTeamClient(yeteam.NewClient(yeteam.Config{
		Enabled:        true,
		AutoRefresh401: true,
		BaseURL:        server.URL,
		Timeout:        time.Second,
	}))

	require.False(t, gateway.reclaimOpenAIAccount401(context.Background(), account))
	require.Equal(t, yeTeamRefreshStatusFailed, repo.updatedExtra[yeTeamLastRefreshStatusKey])
	require.NotEmpty(t, repo.updatedExtra[yeTeamLastRefreshAtKey])
	require.Contains(t, repo.updatedExtra[yeTeamLastRefreshErrorKey], "reclaim service unavailable")
	require.Equal(t, yeTeamRefreshStatusFailed, account.Extra[yeTeamLastRefreshStatusKey])
	flow, ok := repo.updatedExtra[yeTeamLastRefreshFlowKey].(yeteam.ReclaimFlow)
	require.True(t, ok)
	require.Equal(t, yeTeamRefreshStatusFailed, flow.Status)
	require.Contains(t, flowStageNames(flow), "batch_reclaim")
}

func TestYeTeamReclaimPersistsUnreclaimableTaskReason(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"cards":[{"card_code":"TEAM-TEST","tasks":[{"error_code":"account_deactivated","failure_class":"account_dead","message":"账号已被 ChatGPT 官方删除或停用（403 account_deactivated），请联系售后。","order_no":"ord-dead","permanent":true,"provider_status":403,"status":"unreclaimable"}],"unreclaimable":1}],"ok":true,"unreclaimable":1}`))
	}))
	t.Cleanup(server.Close)

	account := newYeTeamRefreshAccount(87)
	repo := &yeTeamTokenAccountRepo{account: account}
	gateway := &OpenAIGatewayService{accountRepo: repo}
	gateway.SetYeTeamClient(yeteam.NewClient(yeteam.Config{
		Enabled:        true,
		AutoRefresh401: true,
		BaseURL:        server.URL,
		Timeout:        time.Second,
	}))

	require.False(t, gateway.reclaimOpenAIAccount401(context.Background(), account))
	require.Equal(t, yeTeamRefreshStatusFailed, repo.updatedExtra[yeTeamLastRefreshStatusKey])
	require.Contains(t, repo.updatedExtra[yeTeamLastRefreshErrorKey], "账号已被 ChatGPT 官方删除或停用（403 account_deactivated），请联系售后。")
	require.Equal(t, repo.updatedExtra[yeTeamLastRefreshErrorKey], account.Extra[yeTeamLastRefreshErrorKey])
	flow, ok := repo.updatedExtra[yeTeamLastRefreshFlowKey].(yeteam.ReclaimFlow)
	require.True(t, ok)
	require.Equal(t, yeTeamRefreshStatusFailed, flow.Status)
	require.True(t, flow.FallbackUsed)
	require.Contains(t, flowStageNames(flow), "refresh_bound")
}

func TestYeTeamReclaimFallbackRefreshesBoundAccount(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/redeem/reclaim/batch-cards":
			_, _ = w.Write([]byte(`{"cards":[{"card_code":"TEAM-TEST","tasks":[{"error_code":"account_deactivated","failure_class":"account_dead","message":"account deactivated","order_no":"ord-dead","permanent":true,"provider_status":403,"status":"unreclaimable"}],"unreclaimable":1}],"ok":true,"unreclaimable":1}`))
		case "/api/redeem/orders":
			_, _ = w.Write([]byte(`{"download_token":"tok-refresh","order_no":"ord-refresh","status":"pending"}`))
		case "/api/redeem/orders/ord-refresh":
			_, _ = w.Write([]byte(`{"order":{"order_no":"ord-refresh","status":"completed"}}`))
		case "/api/redeem/orders/ord-refresh/download":
			_, _ = w.Write([]byte(`{"accounts":[{"name":"account@example.com","credentials":{"access_token":"new-token"}}]}`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(server.Close)

	account := newYeTeamRefreshAccount(89)
	repo := &yeTeamTokenAccountRepo{account: account}
	gateway := &OpenAIGatewayService{accountRepo: repo}
	gateway.SetYeTeamClient(yeteam.NewClient(yeteam.Config{
		Enabled:         true,
		AutoRefresh401:  true,
		BaseURL:         server.URL,
		Timeout:         time.Second,
		PollInterval:    time.Millisecond,
		MaxPollDuration: time.Second,
	}))

	require.True(t, gateway.reclaimOpenAIAccount401(context.Background(), account))
	require.Equal(t, "new-token", repo.updatedCredentials["access_token"])
	require.Equal(t, yeTeamRefreshStatusSuccess, repo.updatedExtra[yeTeamLastRefreshStatusKey])
	require.Empty(t, repo.updatedExtra[yeTeamLastRefreshErrorKey])
	flow, ok := repo.updatedExtra[yeTeamLastRefreshFlowKey].(yeteam.ReclaimFlow)
	require.True(t, ok)
	require.True(t, flow.FallbackUsed)
	require.Equal(t, "ord-refresh", flow.OrderNo)
	require.Contains(t, flowStageNames(flow), "refresh_bound_order")
	require.Contains(t, flowStageNames(flow), "download")
}

func TestYeTeamReclaimDownloadsHealthyNoActionPackage(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/redeem/reclaim/batch-cards":
			_, _ = w.Write([]byte(`{"ok":true,"queued":0,"already_running":0,"done":1,"cards":[{"card_code":"TEAM-TEST","tasks":[{"order_no":"ord-1","status":"done","no_action":true,"message":"credential healthy"}]}]}`))
		case "/api/redeem/batch-download":
			_, _ = w.Write([]byte(`{"accounts":[{"name":"account@example.com","credentials":{"access_token":"new-token"}}]}`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(server.Close)

	account := newYeTeamRefreshAccount(86)
	repo := &yeTeamTokenAccountRepo{account: account}
	gateway := &OpenAIGatewayService{accountRepo: repo}
	gateway.SetYeTeamClient(yeteam.NewClient(yeteam.Config{
		Enabled:         true,
		AutoRefresh401:  false,
		BaseURL:         server.URL,
		Timeout:         time.Second,
		MaxPollDuration: time.Second,
	}))

	require.NoError(t, gateway.ReclaimOpenAIAccount(context.Background(), account))
	require.Equal(t, "new-token", repo.updatedCredentials["access_token"])
	require.Equal(t, yeTeamRefreshStatusSuccess, repo.updatedExtra[yeTeamLastRefreshStatusKey])
	require.Empty(t, repo.updatedExtra[yeTeamLastRefreshErrorKey])
	require.Equal(t, yeTeamRefreshStatusSuccess, account.Extra[yeTeamLastRefreshStatusKey])
	flow, ok := repo.updatedExtra[yeTeamLastRefreshFlowKey].(yeteam.ReclaimFlow)
	require.True(t, ok)
	require.Contains(t, flowStageNames(flow), "batch_download")
}

func TestYeTeamReclaimPersistsCompletePackageWhenPrimaryCredentialIsUnchanged(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/redeem/reclaim/batch-cards":
			_, _ = w.Write([]byte(`{"ok":true,"done":1,"cards":[{"card_code":"TEAM-TEST","tasks":[{"order_no":"ord-1","status":"done","download_token":"tok-1"}]}]}`))
		case "/api/redeem/batch-download":
			_, _ = w.Write([]byte(`{"accounts":[{"name":"account@example.com","credentials":{"access_token":"old-token","refresh_token":"new-refresh-token","expires_at":1800000000,"chatgpt_account_id":"acct-1"}}]}`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(server.Close)

	account := newYeTeamRefreshAccount(90)
	account.Credentials["refresh_token"] = "old-refresh-token"
	repo := &yeTeamTokenAccountRepo{account: account}
	gateway := &OpenAIGatewayService{accountRepo: repo}
	gateway.SetYeTeamClient(yeteam.NewClient(yeteam.Config{
		Enabled: true, AutoRefresh401: true, BaseURL: server.URL, Timeout: time.Second,
	}))

	require.True(t, gateway.reclaimOpenAIAccount401(context.Background(), account))
	require.Equal(t, "old-token", repo.updatedCredentials["access_token"])
	require.Equal(t, "new-refresh-token", repo.updatedCredentials["refresh_token"])
	require.Equal(t, "new-refresh-token", account.GetCredential("refresh_token"))
	flow, ok := repo.updatedExtra[yeTeamLastRefreshFlowKey].(yeteam.ReclaimFlow)
	require.True(t, ok)
	require.True(t, *flow.CredentialChanged)
	require.Equal(t, yeTeamRefreshStatusSuccess, flow.Status)
}

func TestYeTeamReclaimFailsWhenCredentialReadbackDoesNotMatch(t *testing.T) {
	server, _ := newYeTeamAccountTestServer(t)
	t.Cleanup(server.Close)

	account := newYeTeamRefreshAccount(91)
	persisted := *account
	persisted.Credentials = shallowCopyMap(account.Credentials)
	repo := &yeTeamDiscardingCredentialRepo{yeTeamTokenAccountRepo: yeTeamTokenAccountRepo{account: &persisted}}
	gateway := &OpenAIGatewayService{accountRepo: repo}
	gateway.SetYeTeamClient(yeteam.NewClient(yeteam.Config{
		Enabled: true, AutoRefresh401: true, BaseURL: server.URL, Timeout: time.Second,
		PollInterval: time.Millisecond, MaxPollDuration: time.Second,
	}))

	require.False(t, gateway.reclaimOpenAIAccount401(context.Background(), account))
	require.Equal(t, "old-token", persisted.GetCredential("access_token"))
	require.Equal(t, yeTeamRefreshStatusFailed, repo.updatedExtra[yeTeamLastRefreshStatusKey])
	require.Contains(t, repo.updatedExtra[yeTeamLastRefreshErrorKey], "did not match database readback")
	flow, ok := repo.updatedExtra[yeTeamLastRefreshFlowKey].(yeteam.ReclaimFlow)
	require.True(t, ok)
	require.Equal(t, yeTeamRefreshStatusFailed, flow.Status)
}

func flowStageNames(flow yeteam.ReclaimFlow) []string {
	names := make([]string, 0, len(flow.Stages))
	for _, stage := range flow.Stages {
		names = append(names, stage.Name)
	}
	return names
}
