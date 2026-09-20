package service

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func ticketFixture() (*OpenAIGatewayService, *Account) {
	cfg := &config.Config{}
	cfg.Gateway.OpenAICodexTicket = normalizeCodexTicketConfig(config.OpenAICodexTicketConfig{Enabled: true, FailClosed: true, HarvestProxyURL: "http://user:secret@proxy.example:8080", Models: []string{"ticket-model"}})
	account := &Account{ID: 71, Platform: PlatformOpenAI, Type: AccountTypeOAuth, Status: StatusActive, Schedulable: true, Concurrency: 1, Credentials: map[string]any{"access_token": "test-access-token", "chatgpt_account_id": "test-workspace", "plan_type": "plus"}}
	return &OpenAIGatewayService{cfg: cfg}, account
}
func ticketBlob(n int) string { return "gAAAAA" + strings.Repeat("X", n-6) }
func installTicket(s *OpenAIGatewayService, a *Account, model string) *codexTurnTicket {
	ticket := &codexTurnTicket{state: ticketBlob(codexTicketLength(a)), expires: time.Now().Add(time.Hour), credential: codexTicketCredential(a)}
	s.codexTickets.Store(codexTicketKey(a.ID, model), ticket)
	return ticket
}

func TestCodexTicketPlanAndValidation(t *testing.T) {
	for _, tc := range []struct {
		plan   string
		length int
	}{{"plus", 292}, {"team", 332}, {" Team ", 332}, {"self_serve-business_pro_lite", 332}, {"business", 332}} {
		t.Run(tc.plan, func(t *testing.T) {
			s, a := ticketFixture()
			a.Credentials["plan_type"] = tc.plan
			require.Equal(t, tc.length, codexTicketLength(a))
			v := installTicket(s, a, "ticket-model")
			require.True(t, v.valid(a, time.Now()))
			v.state = strings.Repeat("X", tc.length)
			require.False(t, v.valid(a, time.Now()))
		})
	}
	s, a := ticketFixture()
	v := installTicket(s, a, "ticket-model")
	require.False(t, v.valid(a, time.Now().Add(2*time.Hour)))
	a.Credentials["access_token"] = "replacement"
	require.False(t, v.valid(a, time.Now()))
}
func TestCodexTicketInjectionIsolationAndGate(t *testing.T) {
	s, a := ticketFixture()
	h := http.Header{}
	require.ErrorIs(t, s.applyOpenAICodexTicket(context.Background(), a, "ticket-model", h), ErrOpenAICodexTicketUnavailable)
	v := installTicket(s, a, "ticket-model")
	require.NoError(t, s.applyOpenAICodexTicket(context.Background(), a, "ticket-model", h))
	require.Equal(t, v.state, h.Get(openAICodexTurnStateHeader))
	other := *a
	other.ID++
	require.ErrorIs(t, s.applyOpenAICodexTicket(context.Background(), &other, "ticket-model", http.Header{}), ErrOpenAICodexTicketUnavailable)
	s.cfg.Gateway.OpenAICodexTicket.Models = append(s.cfg.Gateway.OpenAICodexTicket.Models, "other-model")
	require.ErrorIs(t, s.applyOpenAICodexTicket(context.Background(), a, "other-model", http.Header{}), ErrOpenAICodexTicketUnavailable)
	require.NoError(t, s.applyOpenAICodexTicket(context.Background(), a, "ungated", http.Header{}))
	s.cfg.Gateway.OpenAICodexTicket.Enabled = false
	require.NoError(t, s.applyOpenAICodexTicket(context.Background(), &other, "ticket-model", http.Header{}))
	s.cfg.Gateway.OpenAICodexTicket.Enabled = true
	s.cfg.Gateway.OpenAICodexTicket.FailClosed = false
	require.NoError(t, s.applyOpenAICodexTicket(context.Background(), &other, "ticket-model", http.Header{}))
	s.cfg.Gateway.OpenAICodexTicket.FailClosed = true
	other.Type = AccountTypeAPIKey
	require.NoError(t, s.applyOpenAICodexTicket(context.Background(), &other, "ticket-model", http.Header{}))
	other = *a
	parent := int64(9)
	other.ParentAccountID = &parent
	require.NoError(t, s.applyOpenAICodexTicket(context.Background(), &other, "ticket-model", http.Header{}))
}
func TestCodexTicketRequestBuilders(t *testing.T) {
	s, a := ticketFixture()
	ticket := installTicket(s, a, "ticket-model")
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest("POST", "/v1/responses", nil)
	body := []byte(`{"model":"ticket-model","input":"ping"}`)
	req, err := s.buildUpstreamRequest(context.Background(), c, a, body, "test-access-token", true, "", true)
	require.NoError(t, err)
	require.Equal(t, ticket.state, req.Header.Get(openAICodexTurnStateHeader))
	req, err = s.buildUpstreamRequestOpenAIPassthrough(context.Background(), c, a, body, "test-access-token")
	require.NoError(t, err)
	require.Equal(t, ticket.state, req.Header.Get(openAICodexTurnStateHeader))
	h, _, err := s.buildOpenAIWSHeaders(context.Background(), c, a, "test-access-token", OpenAIWSProtocolDecision{}, true, "client-state", "", "", "ticket-model", "")
	require.NoError(t, err)
	require.Equal(t, ticket.state, h.Get(openAICodexTurnStateHeader))
	s.codexTickets.Delete(codexTicketKey(a.ID, "ticket-model"))
	_, err = s.buildUpstreamRequest(context.Background(), c, a, body, "test-access-token", true, "", true)
	require.ErrorIs(t, err, ErrOpenAICodexTicketUnavailable)
	_, err = s.buildUpstreamRequestOpenAIPassthrough(context.Background(), c, a, body, "test-access-token")
	require.ErrorIs(t, err, ErrOpenAICodexTicketUnavailable)
	_, _, err = s.buildOpenAIWSHeaders(context.Background(), c, a, "test-access-token", OpenAIWSProtocolDecision{}, true, "", "", "", "ticket-model", "")
	require.ErrorIs(t, err, ErrOpenAICodexTicketUnavailable)
}
func TestCodexTicketSchedulingMapping(t *testing.T) {
	s, a := ticketFixture()
	a.Credentials["model_mapping"] = map[string]any{"public-name": "ticket-model"}
	require.True(t, s.codexTicketBlocksAccount(context.Background(), a, "public-name", false))
	installTicket(s, a, "ticket-model")
	require.False(t, s.codexTicketBlocksAccount(context.Background(), a, "public-name", false))
	s.codexTickets.Delete(codexTicketKey(a.ID, "ticket-model"))
	s.cfg.Gateway.OpenAICompactModel = "compact-model"
	require.False(t, s.codexTicketBlocksAccount(context.Background(), a, "public-name", true))
}

type codexTicketMockUpstream struct {
	HTTPUpstream
	calls atomic.Int32
	do    func(*http.Request, string) (*http.Response, error)
}

func (u *codexTicketMockUpstream) Do(r *http.Request, p string, _ int64, _ int) (*http.Response, error) {
	u.calls.Add(1)
	return u.do(r, p)
}
func ticketResponse(code, n int) *http.Response {
	h := http.Header{}
	if n > 0 {
		h.Set(openAICodexTurnStateHeader, ticketBlob(n))
	}
	return &http.Response{StatusCode: code, Header: h, Body: io.NopCloser(strings.NewReader(""))}
}

func TestCodexTicketProbeAndPause(t *testing.T) {
	for _, tc := range []struct {
		name      string
		status, n int
		ready     bool
	}{{"personal", 200, 292, true}, {"wrong-length", 200, 312, false}, {"unauthorized", 401, 292, false}, {"limited", 429, 292, false}, {"upstream-error", 503, 292, false}} {
		t.Run(tc.name, func(t *testing.T) {
			s, a := ticketFixture()
			u := &codexTicketMockUpstream{do: func(r *http.Request, p string) (*http.Response, error) {
				require.Equal(t, s.cfg.Gateway.OpenAICodexTicket.HarvestProxyURL, p)
				require.Equal(t, "Bearer test-access-token", r.Header.Get("Authorization"))
				require.Equal(t, "test-workspace", r.Header.Get("chatgpt-account-id"))
				require.True(t, r.Close)
				require.True(t, HTTPUpstreamRedirectsDisabled(r.Context()))
				require.Equal(t, HTTPUpstreamProfileOpenAIHarvest, HTTPUpstreamProfileFromContext(r.Context()))
				require.Empty(t, r.Header.Get(openAICodexTurnStateHeader))
				return ticketResponse(tc.status, tc.n), nil
			}}
			s.httpUpstream = u
			s.probeCodexTicket(context.Background(), a, "ticket-model", s.codexTicketConfig(context.Background()))
			require.Equal(t, tc.ready, s.cachedCodexTicket(a, "ticket-model").valid(a, time.Now()))
			if !tc.ready {
				s.probeCodexTicket(context.Background(), a, "ticket-model", s.codexTicketConfig(context.Background()))
				require.EqualValues(t, 1, u.calls.Load())
			}
		})
	}
}
func TestCodexTicketUnauthorizedRecovery(t *testing.T) {
	s, a := ticketFixture()
	u := &codexTicketMockUpstream{do: func(*http.Request, string) (*http.Response, error) { return ticketResponse(401, 0), nil }}
	s.httpUpstream = u
	installTicket(s, a, "ticket-model")
	s.probeCodexTicket(context.Background(), a, "ticket-model", s.codexTicketConfig(context.Background()))
	require.Nil(t, s.cachedCodexTicket(a, "ticket-model"))
	s.probeCodexTicket(context.Background(), a, "ticket-model", s.codexTicketConfig(context.Background()))
	require.EqualValues(t, 1, u.calls.Load())
	a.Credentials["access_token"] = "renewed-token"
	u.do = func(*http.Request, string) (*http.Response, error) { return ticketResponse(200, 292), nil }
	s.probeCodexTicket(context.Background(), a, "ticket-model", s.codexTicketConfig(context.Background()))
	require.EqualValues(t, 2, u.calls.Load())
	require.True(t, s.cachedCodexTicket(a, "ticket-model").valid(a, time.Now()))
}
func TestCodexTicketRetryAfter(t *testing.T) {
	s, a := ticketFixture()
	h := http.Header{}
	h.Set("Retry-After", "1800")
	start := time.Now()
	s.pauseCodexTicket(a, 429, h)
	v, _ := s.codexTicketBackoffs.Load(a.ID)
	require.True(t, v.(codexTicketBackoff).until.After(start.Add(1799*time.Second)))
	h.Set("Retry-After", time.Now().Add(time.Hour).UTC().Format(http.TimeFormat))
	s.pauseCodexTicket(a, 429, h)
	v, _ = s.codexTicketBackoffs.Load(a.ID)
	require.True(t, v.(codexTicketBackoff).until.After(start.Add(3598*time.Second)))
}
func TestCodexTicketProxyAndMask(t *testing.T) {
	for _, raw := range []string{"", "http://u:p@proxy.example:8080", "socks5h://[::1]:1080", "https://proxy.example"} {
		require.NoError(t, ValidateOpenAICodexTicketHarvestProxyURL(raw))
	}
	for _, raw := range []string{"file:///etc/passwd", "http://proxy/path", "http://proxy:99999", "http://proxy?token=secret", "broken", "http://proxy/#fragment"} {
		require.Error(t, ValidateOpenAICodexTicketHarvestProxyURL(raw))
	}
	masked := MaskCodexTicketProxyURL("http://user:password@proxy.example:8080")
	require.NotContains(t, masked, "password")
	require.True(t, IsMaskedCodexTicketProxyURL(masked))
	require.False(t, IsMaskedCodexTicketProxyURL(""))
}
func TestCodexTicketRuntimeSettings(t *testing.T) {
	repo := &codexVersionSettingRepoStub{values: map[string]string{SettingKeyOpenAICodexTicketEnabled: "true", SettingKeyOpenAICodexTicketHarvestProxyURL: "socks5://proxy:1080", SettingKeyOpenAICodexTicketModels: "a, b,a\nc"}}
	s := &SettingService{settingRepo: repo}
	cfg := s.codexTicketRuntimeConfig(context.Background(), config.OpenAICodexTicketConfig{})
	require.True(t, cfg.Enabled)
	require.Equal(t, "socks5://proxy:1080", cfg.HarvestProxyURL)
	require.Equal(t, []string{"a", "b", "c"}, cfg.Models)
	s.codexTicketSettingsCache.Store(&codexTicketSettingsSnapshot{})
	repo.err = errors.New("db failure")
	require.False(t, s.codexTicketRuntimeConfig(context.Background(), config.OpenAICodexTicketConfig{Enabled: true}).Enabled)
}

type codexTicketAccountRepo struct {
	AccountRepository
	accounts []Account
}

func (r *codexTicketAccountRepo) ListByPlatform(context.Context, string) ([]Account, error) {
	return r.accounts, nil
}
func TestCodexTicketLifecycleCancellation(t *testing.T) {
	s, a := ticketFixture()
	s.accountRepo = &codexTicketAccountRepo{accounts: []Account{*a}}
	entered := make(chan struct{})
	s.httpUpstream = &codexTicketMockUpstream{do: func(r *http.Request, _ string) (*http.Response, error) {
		close(entered)
		<-r.Context().Done()
		return nil, r.Context().Err()
	}}
	s.StartOpenAICodexTicketHarvester()
	done := s.codexTicketDone
	s.StartOpenAICodexTicketHarvester()
	require.Equal(t, done, s.codexTicketDone)
	select {
	case <-entered:
	case <-time.After(4 * time.Second):
		t.Fatal("harvester did not start")
	}
	stopped := make(chan struct{})
	go func() { s.StopOpenAICodexTicketHarvester(); close(stopped) }()
	select {
	case <-stopped:
	case <-time.After(time.Second):
		t.Fatal("shutdown did not cancel probe")
	}
	s.StopOpenAICodexTicketHarvester()
	s.StartOpenAICodexTicketHarvester()
	require.Equal(t, done, s.codexTicketDone)
}

func TestCodexTicketMissKeepsOtherModelsAvailable(t *testing.T) {
	s, a := ticketFixture()
	s.cfg.Gateway.OpenAICodexTicket.Models = []string{"ticket-model", "other-model"}
	u := &codexTicketMockUpstream{do: func(*http.Request, string) (*http.Response, error) { return ticketResponse(200, 312), nil }}
	s.httpUpstream = u
	cfg := s.codexTicketConfig(context.Background())
	s.probeCodexTicket(context.Background(), a, "ticket-model", cfg)
	u.do = func(*http.Request, string) (*http.Response, error) { return ticketResponse(200, 292), nil }
	s.probeCodexTicket(context.Background(), a, "other-model", cfg)
	require.EqualValues(t, 2, u.calls.Load())
	require.True(t, s.cachedCodexTicket(a, "other-model").valid(a, time.Now()))
}
func TestCodexTicketQuotaPauseSurvivesCredentialRefresh(t *testing.T) {
	s, a := ticketFixture()
	s.pauseCodexTicket(a, 429, nil)
	a.Credentials["access_token"] = "refreshed"
	u := &codexTicketMockUpstream{do: func(*http.Request, string) (*http.Response, error) {
		t.Fatal("quota cooldown must hold")
		return nil, nil
	}}
	s.httpUpstream = u
	s.probeCodexTicket(context.Background(), a, "ticket-model", s.codexTicketConfig(context.Background()))
	require.Zero(t, u.calls.Load())
}

func TestCodexTicketDisabledAndProxyRequired(t *testing.T) {
	for _, disabled := range []bool{true, false} {
		s, a := ticketFixture()
		u := &codexTicketMockUpstream{do: func(*http.Request, string) (*http.Response, error) { t.Fatal("unexpected probe"); return nil, nil }}
		s.httpUpstream = u
		cfg := s.codexTicketConfig(context.Background())
		if disabled {
			cfg.Enabled = false
		} else {
			cfg.HarvestProxyURL = ""
		}
		s.probeCodexTicket(context.Background(), a, "ticket-model", cfg)
		require.Zero(t, u.calls.Load())
	}
}
func TestCodexTicketConcurrentProbeDeduplication(t *testing.T) {
	s, a := ticketFixture()
	entered, release, done := make(chan struct{}), make(chan struct{}), make(chan struct{})
	s.httpUpstream = &codexTicketMockUpstream{do: func(*http.Request, string) (*http.Response, error) {
		close(entered)
		<-release
		return ticketResponse(200, 292), nil
	}}
	cfg := s.codexTicketConfig(context.Background())
	go func() { s.probeCodexTicket(context.Background(), a, "ticket-model", cfg); close(done) }()
	<-entered
	s.probeCodexTicket(context.Background(), a, "ticket-model", cfg)
	close(release)
	<-done
	require.EqualValues(t, 1, s.httpUpstream.(*codexTicketMockUpstream).calls.Load())
}
func TestCodexTicketSchedulerCompactOverride(t *testing.T) {
	s, a := ticketFixture()
	s.cfg.RunMode = config.RunModeSimple
	s.cfg.Gateway.OpenAICompactModel = "compact-model"
	require.Nil(t, s.recheckSelectedOpenAIAccountFromDB(context.Background(), a, nil, PlatformOpenAI, "ticket-model", false, OpenAIEndpointCapability("")))
	require.NotNil(t, s.recheckSelectedOpenAIAccountFromDB(context.Background(), a, nil, PlatformOpenAI, "ticket-model", false, OpenAIEndpointCapability(""), true))
}
