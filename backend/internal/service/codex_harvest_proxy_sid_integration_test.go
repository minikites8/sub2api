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
	s := ticketTestService(t, config.OpenAICodexTicketConfig{Enabled: true, TargetLength: 780, TTLSeconds: 240, HarvestProxyURL: harvestSIDTestProxy, Models: []string{"gpt-6-astra"}}, nil)
	s.accountRepo = &manualHarvestAccountRepo{account: account, persist: func(context.Context) error { return nil }}
	var proxies, sessions []string
	state := mint780State(time.Now().Add(-time.Second))
	pair := mint780Pair(time.Now().Add(time.Hour), "unified-88")
	s.httpUpstream = &harvestProxyUpstream{do: func(req *http.Request, proxy string) (*http.Response, error) {
		proxies = append(proxies, proxy)
		sessions = append(sessions, req.Header.Get("session_id"))
		require.NotEqual(t, "OldSID12", codexHarvestProxySID(proxy))
		h := http.Header{}
		body := harvestProbeCompleteFixture("gpt-6-astra")
		switch len(proxies) {
		case 1:
			require.Empty(t, req.Header.Get("Cookie"))
			body = "data: {\"type\":\"response.created\"}\n\n"
		case 2:
			require.Empty(t, req.Header.Get("Cookie"))
			h["Set-Cookie"] = pair
			h.Set(openAICodexTurnStateHeader, state)
		case 3:
			require.Equal(t, strings.Join(pair, "; "), req.Header.Get("Cookie"))
			require.Equal(t, state, req.Header.Get(openAICodexTurnStateHeader))
			require.Equal(t, proxies[1], proxy)
			require.Equal(t, sessions[1], sessions[2])
		default:
			t.Fatalf("unexpected call %d", len(proxies))
		}
		return &http.Response{StatusCode: 200, Header: h, Body: io.NopCloser(strings.NewReader(body))}, nil
	}}
	var events []ManualHarvestProgress
	require.NoError(t, s.ExecuteManualHarvest(t.Context(), ManualHarvestRequest{AccountID: account.ID, Models: []string{"gpt-6-astra"}, MaxAttempts: 3, ProbeIntervalSeconds: 1, StopOnSuccess: true, NodeSwitchRule: ManualHarvestNodeSwitchNever}, func(p ManualHarvestProgress) { events = append(events, p) }))
	require.Len(t, proxies, 3)
	require.NotEqual(t, proxies[0], proxies[1])
	require.NotEqual(t, sessions[0], sessions[1])
	require.Equal(t, harvestSIDTestProxy, s.cfg.Gateway.OpenAICodexTicket.HarvestProxyURL)
	ticket := s.lookupOpenAICodexTicket(account, "gpt-6-astra")
	require.NotNil(t, ticket)
	require.Equal(t, proxies[1], ticket.HarvestProxyURL)
	require.Equal(t, sessions[1], ticket.HarvestSessionID)
	require.Empty(t, ticket.HarvestNodeName)
	var displayed []string
	for _, event := range events {
		if event.Message == "已轮换代理 SID。" {
			displayed = append(displayed, event.Node)
		}
		encoded, err := json.Marshal(event)
		require.NoError(t, err)
		require.NotContains(t, string(encoded), "secret")
		require.NotContains(t, string(encoded), "user-region")
	}
	require.Equal(t, []string{"SID " + codexHarvestProxySID(proxies[0]), "SID " + codexHarvestProxySID(proxies[1])}, displayed)
	for range 2 {
		proxy, release, err := s.pinStoredCodexTicketEgress(t.Context(), ticket, "http://fallback.invalid:8080")
		require.NoError(t, err)
		release()
		require.Equal(t, proxies[1], proxy)
	}
}

func TestCodexHarvestProxySIDConfiguredNodeMemorySkipsSidecar(t *testing.T) {
	s := ticketTestService(t, config.OpenAICodexTicketConfig{Enabled: true, HarvestProxyURL: harvestSIDTestProxy}, nil)
	s.codexHarvest = NewCodexHarvestService(nil, nil, s.cfg)
	controls := CodexHarvestControls{NodeMemoryEnabled: true}
	a, ok := s.prepareHarvestAttempt(t.Context(), ticketTestAccount(1), "gpt-6-astra", harvestSIDTestProxy, map[string]bool{}, controls)
	require.True(t, ok)
	require.Nil(t, a.sidecar)
	require.Equal(t, harvestSIDTestProxy, a.proxy)
	a.release()
	b, err := s.acquireManualHarvestNode(t.Context(), ticketTestAccount(1), "gpt-6-astra", harvestSIDTestProxy, "", map[string]bool{}, true)
	require.NoError(t, err)
	require.Nil(t, b.sidecar)
	require.Equal(t, harvestSIDTestProxy, b.proxy)
	b.release()
	require.Empty(t, s.codexHarvest.Runtime().DegradedReason)
}

func TestCodexHarvestProxySIDAutomaticRetriesWithoutNodeMemory(t *testing.T) {
	resetCodexHarvestFlow()
	t.Cleanup(resetCodexHarvestFlow)
	s, _, _ := learningHarvestFixture(t, 4)
	s.cfg.Gateway.OpenAICodexTicket.TargetLength = 780
	s.cfg.Gateway.OpenAICodexTicket.HarvestProxyURL = harvestSIDTestProxy
	s.codexHarvest.current.NodeMemoryEnabled = false
	s.codexHarvest.current.Speed.MaxNodeAttempts = 2
	s.codexHarvest.current.Speed.MaxRequestsPerRound = 4
	state := mint780State(time.Now().Add(-time.Second))
	pair := mint780Pair(time.Now().Add(time.Hour), "unified-88")
	var proxies []string
	s.httpUpstream = &harvestProxyUpstream{do: func(req *http.Request, proxy string) (*http.Response, error) {
		proxies = append(proxies, proxy)
		h := http.Header{}
		status := 200
		if len(proxies) == 1 {
			status = 503
		} else if len(proxies) == 2 {
			require.Empty(t, req.Header.Get("Cookie"))
			h["Set-Cookie"] = pair
			h.Set(openAICodexTurnStateHeader, state)
		} else {
			require.Equal(t, proxies[1], proxy)
			require.Equal(t, strings.Join(pair, "; "), req.Header.Get("Cookie"))
		}
		return &http.Response{StatusCode: status, Header: h, Body: io.NopCloser(strings.NewReader(harvestProbeCompleteFixture("gpt-6-astra")))}, nil
	}}
	s.probeOnceOpenAICodexTicket(t.Context(), ticketTestAccount(1), "gpt-6-astra")
	require.Len(t, proxies, 3)
	require.NotEqual(t, proxies[0], proxies[1])
	ticket := s.lookupOpenAICodexTicket(ticketTestAccount(1), "gpt-6-astra")
	require.NotNil(t, ticket)
	require.Equal(t, proxies[1], ticket.HarvestProxyURL)
	require.Equal(t, harvestSIDTestProxy, s.cfg.Gateway.OpenAICodexTicket.HarvestProxyURL)
}

func TestCodexHarvestProxySIDBorrowedTicketKeepsOriginalSession(t *testing.T) {
	s, repo, seed := unifiedAstraFixture(t)
	seed.HarvestProxyURL = harvestSIDTestProxy
	account, err := repo.GetByID(t.Context(), 1)
	require.NoError(t, err)
	require.NoError(t, s.storeOpenAICodexTicket(t.Context(), account, seed))
	targetState := mint780State(time.Now().Add(-time.Second))
	calls := 0
	s.httpUpstream = &harvestProxyUpstream{do: func(req *http.Request, proxy string) (*http.Response, error) {
		calls++
		require.Equal(t, harvestSIDTestProxy, proxy)
		require.Equal(t, strings.Join(seed.HarvestCookies, "; "), req.Header.Get("Cookie"))
		h := http.Header{}
		if calls == 1 {
			require.Equal(t, seed.State, req.Header.Get(openAICodexTurnStateHeader))
			h.Set(openAICodexTurnStateHeader, targetState)
		} else {
			require.Equal(t, targetState, req.Header.Get(openAICodexTurnStateHeader))
		}
		return &http.Response{StatusCode: 200, Header: h, Body: io.NopCloser(strings.NewReader(harvestProbeCompleteFixture("gpt-6-astra")))}, nil
	}}
	require.NoError(t, s.VerifyStoredAstraTarget(t.Context(), 2))
	require.Equal(t, 2, calls)
	h := targetTicketHeaders()
	proxy, release, err := s.ApplyStoredAstraTarget(t.Context(), h, 2, "sse")
	require.NoError(t, err)
	release()
	require.Equal(t, harvestSIDTestProxy, proxy)
}

func TestCodexHarvestProxySIDNative292RotatesEachProbe(t *testing.T) {
	s := ticketTestService(t, config.OpenAICodexTicketConfig{Enabled: true, TargetLength: 292}, nil)
	var proxies []string
	s.httpUpstream = &harvestProxyUpstream{do: func(_ *http.Request, proxy string) (*http.Response, error) {
		proxies = append(proxies, proxy)
		return codexTicketResponse(), nil
	}}
	for range 2 {
		r := s.executeCodexHarvestProbe(t.Context(), ticketTestAccount(1), "fixture", "gpt-6-astra", harvestSIDTestProxy, time.Second, nil, "")
		require.Equal(t, "success", r.Kind)
		require.Equal(t, proxies[len(proxies)-1], r.ProxyURL)
		require.Equal(t, codexHarvestProxySID(r.ProxyURL), r.ProxySID)
	}
	require.NotEqual(t, proxies[0], proxies[1])
}

func TestCodexHarvestProxySIDPlainProxyRetainsConfiguration(t *testing.T) {
	const proxy = "http://plain.example:8080"
	s := ticketTestService(t, config.OpenAICodexTicketConfig{Enabled: true, TargetLength: 292}, nil)
	s.httpUpstream = &harvestProxyUpstream{do: func(_ *http.Request, actual string) (*http.Response, error) {
		require.Equal(t, proxy, actual)
		return codexTicketResponse(), nil
	}}
	r := s.executeCodexHarvestProbe(t.Context(), ticketTestAccount(1), "fixture", "gpt-6-astra", proxy, time.Second, nil, "")
	require.Equal(t, "success", r.Kind)
	require.Empty(t, r.ProxySID)
}
