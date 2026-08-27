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
}
