//go:build unit

package service

import (
	"context"
	"io"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
)

// This fixture uses the existing probe API so the same input runs on both
// the pristine upstream implementation and the SID-enabled implementation.
func TestCodexHarvestProxySIDRegression(t *testing.T) {
	const proxy = "socks5://user-region-Rand-sid-OldSID12-t-5:secret@proxy.example:3000"
	account := ticketTestAccount(1)
	s := ticketTestService(t, config.OpenAICodexTicketConfig{Enabled: true, TargetLength: 780, TTLSeconds: 240, HarvestProxyURL: proxy}, nil)
	pair := mint780Pair(time.Now().Add(time.Hour), "unified-88")
	state := mint780State(time.Now().Add(-time.Second))
	var proxies []string
	s.httpUpstream = &harvestProxyUpstream{do: func(req *http.Request, actual string) (*http.Response, error) {
		proxies = append(proxies, actual)
		h := http.Header{}
		if len(proxies)%2 == 1 {
			require.Empty(t, req.Header.Get("Cookie"))
			require.Empty(t, req.Header.Get(openAICodexTurnStateHeader))
			h["Set-Cookie"] = pair
			h.Set(openAICodexTurnStateHeader, state)
		} else {
			require.Equal(t, state, req.Header.Get(openAICodexTurnStateHeader))
			require.Equal(t, strings.Join(pair, "; "), req.Header.Get("Cookie"))
		}
		return &http.Response{StatusCode: 200, Header: h, Body: io.NopCloser(strings.NewReader(harvestProbeCompleteFixture("gpt-6-astra")))}, nil
	}}
	var ticket *openAICodexTicket
	for range 2 {
		result := s.executeCodexHarvestProbe(t.Context(), account, "fixture", "gpt-6-astra", proxy, time.Second, nil, "session")
		require.Equal(t, "success", result.Kind)
		ticket = codexHarvestTicket(account, "gpt-6-astra", result, s.openAICodexTicketConfig(), 1)
		bindCodexHarvestEgress(ticket, codexHarvestAttempt{proxy: proxy}, "session")
	}
	require.Len(t, proxies, 4)
	rotated := proxies[0] != proxy && proxies[2] != proxy
	retryNew := proxies[0] != proxies[2]
	confirmationSame := proxies[0] == proxies[1] && proxies[2] == proxies[3]
	pinned := ticket.HarvestProxyURL == proxies[2] && ticket.HarvestProxyURL != proxy
	templateSame := s.cfg.Gateway.OpenAICodexTicket.HarvestProxyURL == proxy
	t.Logf("SID_rotated=%t retry_new_SID=%t confirmation_same_SID=%t ticket_egress_pinned=%t template_unchanged=%t", rotated, retryNew, confirmationSame, pinned, templateSame)
	require.True(t, rotated, "self collection rotates the proxy SID")
	require.True(t, retryNew)
	require.True(t, confirmationSame)
	require.True(t, pinned)
	require.True(t, templateSame)
	original, err := url.Parse(proxy)
	require.NoError(t, err)
	actual, err := url.Parse(proxies[0])
	require.NoError(t, err)
	require.Equal(t, original.Host, actual.Host)
	password, _ := actual.User.Password()
	require.Equal(t, "secret", password)
	ctx := context.Background()
	first, release, err := s.pinStoredCodexTicketEgress(ctx, ticket, "http://fallback.invalid:8080")
	require.NoError(t, err)
	release()
	require.Equal(t, proxies[2], first)
}
