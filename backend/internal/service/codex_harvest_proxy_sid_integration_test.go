//go:build unit

package service

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
)

const harvestSIDTestProxy = "socks5://user-region-Rand-sid-OldSID12-t-5:secret@proxy.example:3000"

func TestCodexHarvestProxySIDManualRetryStoresAndPinsSuccessfulSession(t *testing.T) {
	resetCodexHarvestFlow()
	t.Cleanup(resetCodexHarvestFlow)
	account := ticketTestAccount(41)
	s := ticketTestService(t, config.OpenAICodexTicketConfig{
		Enabled: true, TargetLength: 780, TTLSeconds: 240, HarvestProxyURL: harvestSIDTestProxy,
		Models: []string{"gpt-6-astra"},
	}, nil)
	setTestCodexMintMode(t, s, "local")
	controls, _, err := s.codexHarvest.Controls(context.Background())
	require.NoError(t, err)
	controls.NodeMemoryEnabled = true
	require.NoError(t, s.codexHarvest.SaveControls(context.Background(), controls))
	s.accountRepo = &manualHarvestAccountRepo{account: account, persist: func(context.Context) error { return nil }}
	var proxies, sessions []string
	s.httpUpstream = &harvestProxyUpstream{do: func(req *http.Request, proxy string) (*http.Response, error) {
		proxies = append(proxies, proxy)
		sessions = append(sessions, req.Header.Get("session-id"))
		require.NotEqual(t, "OldSID12", codexHarvestProxySID(proxy))
		require.Empty(t, req.Header.Get("Cookie"), "a new SID gets a fresh route")
		gateway := "unified-88"
		if len(proxies) == 1 {
			gateway = "unified-79"
		}
		h := http.Header{"Set-Cookie": mint780Pair(time.Now().Add(time.Hour), gateway)}
		h.Set(openAICodexTurnStateHeader, mint780State(time.Now()))
		return &http.Response{StatusCode: 200, Header: h, Body: io.NopCloser(strings.NewReader(
			"data: {\"type\":\"response.created\",\"response\":{\"id\":\"r\",\"model\":\"gpt-6-astra\"}}\n\n"))}, nil
	}}
	var events []ManualHarvestProgress
	require.NoError(t, s.ExecuteManualHarvest(context.Background(), ManualHarvestRequest{
		AccountID: account.ID, Models: []string{"gpt-6-astra"}, MaxAttempts: 2,
		ProbeIntervalSeconds: 1, StopOnSuccess: true, NodeSwitchRule: ManualHarvestNodeSwitchNever,
	}, func(p ManualHarvestProgress) { events = append(events, p) }))
	require.Len(t, proxies, 2)
	require.NotEqual(t, proxies[0], proxies[1])
	require.NotEqual(t, sessions[0], sessions[1])
	require.Equal(t, harvestSIDTestProxy, s.cfg.Gateway.OpenAICodexTicket.HarvestProxyURL)
	ticket := s.lookupOpenAICodexTicket(account, "gpt-6-astra")
	require.NotNil(t, ticket)
	require.Equal(t, proxies[1], ticket.HarvestProxyURL)
	require.Equal(t, sessions[1], ticket.HarvestSessionID)
	require.Empty(t, ticket.HarvestNodeName, "SID labels are display data; external tickets bind a URL")
	require.True(t, events[len(events)-1].Done)
	require.Equal(t, 1, events[len(events)-1].TicketsStored)
	var displayed []string
	for _, event := range events {
		if event.Result == "node_switch" && event.Message == "已轮换代理 SID。" {
			displayed = append(displayed, event.Node)
		}
		encoded, err := json.Marshal(event)
		require.NoError(t, err)
		require.NotContains(t, string(encoded), "secret")
		require.NotContains(t, string(encoded), "user-region")
	}
	require.Equal(t, []string{"SID " + codexHarvestProxySID(proxies[0]), "SID " + codexHarvestProxySID(proxies[1])}, displayed)

	// A later mint rotates again while the accepted ticket keeps its original egress.
	r := s.executeCodexHarvestProbe(context.Background(), account, "tok", "gpt-6-astra", harvestSIDTestProxy, time.Second, nil, "next-session")
	require.Equal(t, "success", r.Kind)
	require.Equal(t, proxies[2], r.ProxyURL)
	require.NotEqual(t, ticket.HarvestProxyURL, r.ProxyURL)
	h := http.Header{}
	h.Set(openAICodexTurnStateHeader, ticket.State)
	for range 2 {
		proxy, release, err := s.pinCodexTicketEgressFromHeader(context.Background(), h, account, "http://business.example:8080")
		require.NoError(t, err)
		require.Equal(t, proxies[1], proxy)
		release()
	}
	require.Empty(t, s.codexHarvest.Runtime().DegradedReason)
	require.True(t, s.codexHarvest.Snapshot(context.Background(), harvestSIDTestProxy).Available)
}

func TestCodexHarvestProxySIDRouteCacheAndTicketSeedsStayWithinSession(t *testing.T) {
	account := ticketTestAccount(1)
	s := ticketTestService(t, config.OpenAICodexTicketConfig{Enabled: true, TargetLength: 780, TTLSeconds: 240}, nil)
	now := time.Now()
	pair := mint780Pair(now.Add(time.Hour), "unified-88")
	oldProxy := harvestSIDTestProxy
	newProxy := strings.Replace(oldProxy, "OldSID12", "NewSID34", 1)
	s.openaiCodexTickets.Store(openAICodexTicketKey(account.ID, "gpt-6-astra"), &openAICodexTicket{
		AccountID: account.ID, Model: "gpt-6-astra", State: mint780State(now), Length: 780,
		IssuedAt: now, ExpiresAt: now.Add(time.Minute), HarvestCookies: pair, HarvestProxyURL: oldProxy,
		Gateway: "unified-88", Transport: "sse",
	})
	calls := 0
	s.httpUpstream = &harvestProxyUpstream{do: func(req *http.Request, proxy string) (*http.Response, error) {
		calls++
		if calls == 1 || calls == 3 {
			require.Equal(t, strings.Join(pair, "; "), req.Header.Get("Cookie"))
		} else {
			require.Empty(t, req.Header.Get("Cookie"), "SID change isolates both cached and persisted seeds")
		}
		h := http.Header{"Set-Cookie": pair}
		h.Set(openAICodexTurnStateHeader, mint780State(time.Now()))
		return &http.Response{StatusCode: 200, Header: h, Body: io.NopCloser(strings.NewReader(
			"data: {\"type\":\"response.created\",\"response\":{\"id\":\"r\",\"model\":\"gpt-6-astra\"}}\n\n"))}, nil
	}}
	for _, proxy := range []string{oldProxy, newProxy, newProxy} {
		r := s.requestLocalCodex780Probe(context.Background(), account, "tok", "gpt-6-astra", proxy, nil, "session")
		require.NoError(t, r.Err)
	}
	require.Len(t, s.codex780Routes.entries, 2)
}

func TestCodexHarvestProxySIDRemoteRelayKeepsExistingContract(t *testing.T) {
	s := ticketTestService(t, config.OpenAICodexTicketConfig{TargetLength: 780, HarvestProxyURL: harvestSIDTestProxy,
		CloudMint: config.OpenAICodexCloudMintConfig{Enabled: true, URL: "https://relay.example", Key: "relay-secret", Gateway: "unified-88"},
	}, nil)
	now := time.Now().Truncate(time.Second)
	s.httpUpstream = &harvestProxyUpstream{do: func(req *http.Request, proxy string) (*http.Response, error) {
		require.Empty(t, proxy)
		require.Equal(t, "relay.example", req.URL.Hostname())
		require.Equal(t, "relay-secret", req.Header.Get("X-Relay-Key"))
		require.Equal(t, "0", req.Header.Get("X-Mint-TTL"))
		return &http.Response{StatusCode: 200, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(
			cloudMintResponseBody(t, "gpt-6-astra", "gpt-6-astra", mint780State(now), now, now.Add(time.Minute), mint780Pair(now.Add(time.Minute), "unified-88"))))}, nil
	}}
	r := s.executeCodexHarvestProbe(context.Background(), ticketTestAccount(1), "tok", "gpt-6-astra", harvestSIDTestProxy, time.Second, nil, "session")
	require.Equal(t, "success", r.Kind)
	require.Empty(t, r.ProxyURL)
	require.Empty(t, r.ProxySID)
	require.Equal(t, harvestSIDTestProxy, s.cfg.Gateway.OpenAICodexTicket.HarvestProxyURL)
}

func TestCodexHarvestProxySIDNative292RotatesEachProbe(t *testing.T) {
	s := ticketTestService(t, config.OpenAICodexTicketConfig{Enabled: true, TargetLength: 292}, nil)
	var proxies []string
	s.httpUpstream = &harvestProxyUpstream{do: func(req *http.Request, proxy string) (*http.Response, error) {
		proxies = append(proxies, proxy)
		return codexTicketResponse(), nil
	}}
	for range 2 {
		r := s.executeCodexHarvestProbe(context.Background(), ticketTestAccount(1), "tok", "gpt-6-astra", harvestSIDTestProxy, time.Second, nil, "")
		require.Equal(t, "success", r.Kind)
		require.Equal(t, proxies[len(proxies)-1], r.ProxyURL)
		require.Equal(t, codexHarvestProxySID(r.ProxyURL), r.ProxySID)
	}
	require.NotEqual(t, proxies[0], proxies[1])
}
