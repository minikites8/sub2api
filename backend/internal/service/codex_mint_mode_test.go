package service

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
)

func setTestCodexMintMode(t *testing.T, s *OpenAIGatewayService, mode string) {
	t.Helper()
	s.codexHarvest = NewCodexHarvestService(nil, &harvestControlSettingsRepo{}, s.cfg)
	v, _, err := s.codexHarvest.Controls(context.Background())
	require.NoError(t, err)
	v.MintMode = mode
	require.NoError(t, s.codexHarvest.SaveControls(context.Background(), v))
}

func TestCodexMintModePersistsAndOverridesDeployment(t *testing.T) {
	for _, enabled := range []bool{false, true} {
		cfg := &config.Config{Gateway: config.GatewayConfig{OpenAICodexTicket: config.OpenAICodexTicketConfig{
			CloudMint: config.OpenAICodexCloudMintConfig{Enabled: enabled},
		}}}
		repo := &harvestControlSettingsRepo{}
		for _, mode := range []string{"", "remote", "local"} {
			s := NewCodexHarvestService(nil, repo, cfg)
			v, _, err := s.Controls(context.Background())
			require.NoError(t, err)
			v.MintMode = mode
			require.NoError(t, s.SaveControls(context.Background(), v))
			restarted := NewCodexHarvestService(nil, repo, cfg)
			loaded, saved, err := restarted.Controls(context.Background())
			require.NoError(t, err)
			require.True(t, saved)
			require.Equal(t, mode, loaded.MintMode)
			gateway := &OpenAIGatewayService{cfg: cfg, codexHarvest: restarted}
			want := mode == "remote" || (mode == "" && enabled)
			require.Equal(t, want, gateway.usesRemoteCodexMint(context.Background()))
		}
	}
	v := CodexHarvestControls{Version: 1, MintMode: "unknown", Speed: CodexHarvestSpeedPresets()["standard"]}
	require.Error(t, ValidateCodexHarvestControls(v))
}

func TestCodexLocalMintOverridesConfiguredRelay(t *testing.T) {
	cfg := config.OpenAICodexTicketConfig{TargetLength: 780, CloudMint: config.OpenAICodexCloudMintConfig{
		Enabled: true, URL: "https://relay.example", Key: "relay-secret",
	}}
	s := ticketTestService(t, cfg, nil)
	setTestCodexMintMode(t, s, "local")
	s.httpUpstream = &harvestProxyUpstream{do: func(req *http.Request, proxy string) (*http.Response, error) {
		require.Equal(t, "chatgpt.com", req.URL.Hostname())
		require.Equal(t, "socks5://127.0.0.1:1080", proxy)
		require.Empty(t, req.Header.Get("X-Relay-Key"))
		h := http.Header{"Set-Cookie": mint780Pair(time.Now().Add(time.Hour), "unified-88")}
		h.Set(openAICodexTurnStateHeader, mint780State(time.Now()))
		return &http.Response{StatusCode: 200, Header: h, Body: io.NopCloser(strings.NewReader(
			"data: {\"type\":\"response.created\",\"response\":{\"id\":\"r\",\"model\":\"gpt-6-astra\"}}\n\n"))}, nil
	}}
	out := s.requestCodexHarvestProbe(context.Background(), ticketTestAccount(1), "token", "gpt-6-astra", "socks5://127.0.0.1:1080", nil, "session")
	require.NoError(t, out.Err)
	require.Len(t, out.State, 780)
	require.Equal(t, "unified-88", out.Gateway)
}

func TestCodexRemoteMintOverridesDisabledRelay(t *testing.T) {
	cfg := config.OpenAICodexTicketConfig{TargetLength: 780, CloudMint: config.OpenAICodexCloudMintConfig{
		URL: "https://relay.example", Key: "relay-secret", Gateway: "unified-88",
	}}
	s := ticketTestService(t, cfg, nil)
	setTestCodexMintMode(t, s, "remote")
	now := time.Now().Truncate(time.Second)
	s.httpUpstream = &harvestProxyUpstream{do: func(req *http.Request, proxy string) (*http.Response, error) {
		require.Equal(t, "relay.example", req.URL.Hostname())
		require.Equal(t, "relay-secret", req.Header.Get("X-Relay-Key"))
		require.Equal(t, "0", req.Header.Get("X-Mint-TTL"))
		require.Empty(t, proxy)
		return &http.Response{StatusCode: 200, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(
			cloudMintResponseBody(t, "gpt-6-astra", "gpt-6-astra", mint780State(now), now, now.Add(time.Minute), mint780Pair(now.Add(time.Minute), "unified-88"))))}, nil
	}}
	out := s.requestCodexHarvestProbe(context.Background(), ticketTestAccount(1), "token", "gpt-6-astra", "", nil, "session")
	require.NoError(t, out.Err)
	require.Len(t, out.State, 780)
}

func TestLegacyRelayTicketRetainsEgressAfterSwitchToLocal(t *testing.T) {
	s := ticketTestService(t, config.OpenAICodexTicketConfig{Enabled: true, TargetLength: 780}, nil)
	setTestCodexMintMode(t, s, "local")
	for _, provider := range []string{"", "relay"} {
		now := time.Now()
		ticket := &openAICodexTicket{AccountID: 1, Model: "gpt-6-astra", Length: 780,
			State: mint780State(now), IssuedAt: now, ExpiresAt: now.Add(time.Minute),
			Transport: "sse", HarvestNodeProvider: provider, Gateway: "unified-88", HarvestCookies: mint780Pair(now.Add(time.Minute), "unified-88")}
		s.openaiCodexTickets.Store(openAICodexTicketKey(1, "gpt-6-astra"), ticket)
		header := http.Header{}
		header.Set(openAICodexTurnStateHeader, ticket.State)
		proxy, release, err := s.pinCodexTicketEgressFromHeader(context.Background(), header, ticketTestAccount(1), "http://production-proxy:8080")
		require.NoError(t, err)
		require.Equal(t, "http://production-proxy:8080", proxy)
		release()
	}
}

func TestCodexMintAvailabilityUsesSavedRuntimeSettings(t *testing.T) {
	ctx := context.Background()
	repo := &codexPolicyMigrationRepoStub{values: map[string]string{
		SettingKeyOpenAICodexRelayURL:              "https://relay.example",
		SettingKeyOpenAICodexRelayKey:              "relay-secret",
		SettingKeyOpenAICodexTicketHarvestProxyURL: "socks5h://harvest.example:1080",
	}}
	cfg := &config.Config{}
	settings := NewSettingService(repo, cfg)
	s := NewCodexHarvestService(nil, repo, cfg)
	require.True(t, s.SnapshotWithSettings(ctx, cfg, settings).Available)
	v, _, err := s.Controls(ctx)
	require.NoError(t, err)
	v.MintMode = "local"
	v.NodeMemoryEnabled = false
	require.NoError(t, s.SaveControls(ctx, v))
	require.True(t, s.SnapshotWithSettings(ctx, cfg, settings).Available)
	v.MintMode = "remote"
	require.NoError(t, s.SaveControls(ctx, v))
	require.True(t, s.SnapshotWithSettings(ctx, cfg, settings).Available)
}
