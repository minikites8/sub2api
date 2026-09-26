package service

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
)

func TestExternalHarvestAttemptKeepsConfiguredProxy(t *testing.T) {
	proxy := "http://proxy.example:8080"
	s := ticketTestService(t, config.OpenAICodexTicketConfig{Enabled: true, HarvestProxyURL: proxy}, nil)
	attempt, ok := s.prepareHarvestAttempt(context.Background(), ticketTestAccount(1), "gpt-6-astra", proxy, map[string]bool{}, CodexHarvestControls{NodeMemoryEnabled: true})
	require.True(t, ok)
	require.Equal(t, proxy, attempt.proxy)
	require.Empty(t, attempt.node.ID)
	require.NotNil(t, attempt.release)
	attempt.release()
}

type harvestProxyUpstream struct {
	HTTPUpstream
	do func(*http.Request, string) (*http.Response, error)
}

func (u *harvestProxyUpstream) Do(req *http.Request, proxy string, _ int64, _ int) (*http.Response, error) {
	return u.do(req, proxy)
}

func TestLegacyNodeBoundTicketFailsClosed(t *testing.T) {
	account := ticketTestAccount(1)
	s := ticketTestService(t, config.OpenAICodexTicketConfig{Enabled: true, Models: []string{"gpt-6-astra"}, TargetLength: 292}, nil)
	now := time.Now()
	state := fakeCodexTicketState(292)
	ticket := &openAICodexTicket{AccountID: account.ID, Model: "gpt-6-astra", State: state, Length: 292, IssuedAt: now, ExpiresAt: now.Add(time.Hour), HarvestNodeID: "legacy-node", HarvestProxyURL: "http://legacy.example:8080"}
	s.openaiCodexTickets.Store(openAICodexTicketKey(account.ID, ticket.Model), ticket)
	h := http.Header{}
	h.Set(openAICodexTurnStateHeader, state)
	proxy, release, err := s.pinCodexTicketEgressFromHeader(context.Background(), h, account, "http://account.example:8080")
	require.ErrorIs(t, err, ErrOpenAICodexTicketUnavailable)
	require.Empty(t, proxy)
	release()
}
