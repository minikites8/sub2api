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

func TestCodexTicketAdminAdapterNoSecrets(t *testing.T) {
	account := ticketTestAccount(4242)
	s := &OpenAIGatewayService{cfg: &config.Config{}}
	s.cfg.Gateway.OpenAICodexTicket = config.OpenAICodexTicketConfig{Enabled: true, Models: []string{"gpt-6-astra"}}
	now := time.Now()
	statuses := s.OpenAICodexTicketStatuses(context.Background(), account, now)
	require.NotNil(t, statuses)
	_, err := s.OpenAICodexTicketLogs(context.Background(), account, "unknown-model", now)
	require.ErrorIs(t, err, ErrOpenAICodexTicketLogModel)
	for _, status := range statuses {
		logs, err := s.OpenAICodexTicketLogs(context.Background(), account, status.Model, now)
		require.NoError(t, err)
		encoded, err := json.Marshal(logs)
		require.NoError(t, err)
		require.NotContains(t, string(encoded), account.GetCredential("access_token"))
	}
}

func TestOpenAICodexTicketStatusesUIState(t *testing.T) {
	now := time.Now()
	future, past := now.Add(time.Minute), now.Add(-time.Minute)
	for _, tc := range []struct {
		name, state string
		ready, skip bool
		probe       *CodexProbeSummary
	}{
		{name: "waiting", state: "waiting"},
		{name: "ready", state: "ready", ready: true},
		{name: "paused", state: "paused", skip: true},
		{name: "ready while paused", state: "ready", ready: true, skip: true},
		{name: "token error", state: "token_invalid", probe: &CodexProbeSummary{Result: "token_error"}},
		{name: "unauthorized", state: "token_invalid", probe: &CodexProbeSummary{HTTPStatus: 401}},
		{name: "forbidden", state: "token_invalid", probe: &CodexProbeSummary{HTTPStatus: 403}},
		{name: "cooldown", state: "cooldown", probe: &CodexProbeSummary{NextProbeAt: &future}},
		{name: "cooldown elapsed", state: "waiting", probe: &CodexProbeSummary{NextProbeAt: &past}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			resetCodexHarvestFlow()
			t.Cleanup(resetCodexHarvestFlow)
			account := ticketTestAccount(4242)
			model := "gpt-6-astra"
			account.Extra = map[string]any{OpenAICodexSkipHarvestExtraKey: tc.skip, codexProbeSummaryKey(model): tc.probe}
			if tc.ready {
				attachReadyCodexTicket(account, model)
			}
			statuses := OpenAICodexTicketStatuses(account, config.OpenAICodexTicketConfig{Enabled: true, Models: []string{model}, TargetLength: 292}, now)
			require.Len(t, statuses, 1)
			require.Equal(t, tc.state, statuses[0].State)
			encoded, err := json.Marshal(statuses[0])
			require.NoError(t, err)
			var payload map[string]any
			require.NoError(t, json.Unmarshal(encoded, &payload))
			require.Equal(t, tc.state, payload["state"])
			require.Equal(t, float64(0), payload["attempts"])
			require.Equal(t, float64(292), payload["target_length"])
		})
	}
}

func TestOpenAICodexTicketStatusesUIAttempts(t *testing.T) {
	resetCodexHarvestFlow()
	t.Cleanup(resetCodexHarvestFlow)
	account := ticketTestAccount(4242)
	model := "gpt-6-astra"
	attachReadyCodexTicket(account, model)
	ticket := account.Extra[openAICodexTicketExtraKey(model)].(openAICodexTicket)
	ticket.Attempts = 7
	account.Extra[openAICodexTicketExtraKey(model)] = ticket
	cfg := config.OpenAICodexTicketConfig{Enabled: true, Models: []string{model, "gpt-5.6-sol"}, TargetLength: 292}
	statuses := OpenAICodexTicketStatuses(account, cfg, time.Now())
	require.Equal(t, 7, statuses[0].Attempts)
	require.Zero(t, statuses[1].Attempts)
	for i := 0; i < 2; i++ {
		recordCodexHarvestProbe(account, model, "success", "relay-a", "unified-88", "", 200, 292, 10, 292, 10)
	}
	recordCodexHarvestProbe(account, "gpt-5.6-sol", "invalid_state", "relay-a", "unified-88", "", 200, 312, 11, 292, 10)
	recordCodexHarvestProbe(ticketTestAccount(4243), model, "success", "relay-a", "unified-88", "", 200, 292, 10, 292, 10)
	statuses = OpenAICodexTicketStatuses(account, cfg, time.Now())
	require.Equal(t, 2, statuses[0].Attempts)
	require.Equal(t, 1, statuses[1].Attempts)
}

func TestCodexTicketLogsExposePerAccountHistory(t *testing.T) {
	resetCodexHarvestFlow()
	t.Cleanup(resetCodexHarvestFlow)
	account := ticketTestAccount(4242)
	other := ticketTestAccount(4243)
	model := "gpt-6-astra"
	recordCodexHarvestProbe(account, model, "invalid_state", "relay-a", "unified-84", "", 200, 312, 11, 292, 10)
	recordCodexHarvestProbe(other, model, "success", "relay-b", "unified-95", "", 200, 292, 10, 292, 10)
	recordCodexHarvestProbe(account, model, "success", "relay-a", "unified-88", "", 200, 292, 10, 292, 10)

	s := &OpenAIGatewayService{cfg: &config.Config{}}
	s.cfg.Gateway.OpenAICodexTicket = config.OpenAICodexTicketConfig{Enabled: true, Models: []string{model}}
	logs, err := s.OpenAICodexTicketLogs(context.Background(), account, model, time.Now())
	require.NoError(t, err)
	require.Len(t, logs.Entries, 2)
	require.Equal(t, "acquired", logs.Entries[0].Event)
	require.Equal(t, "target_length_matched", logs.Entries[0].Reason)
	require.Equal(t, 292, *logs.Entries[0].TicketLength)
	require.Equal(t, "unified-88", logs.Entries[0].Gateway)
	require.Equal(t, "miss", logs.Entries[1].Event)
	require.Equal(t, "length_mismatch", logs.Entries[1].Reason)
	require.Equal(t, 312, *logs.Entries[1].TicketLength)
	require.Equal(t, "unified-84", logs.Entries[1].Gateway)
	encoded, err := json.Marshal(logs.Entries[0])
	require.NoError(t, err)
	require.Contains(t, string(encoded), `"gateway":"unified-88"`)
}

func TestCodexTicketLogsLeaveUnobservedGatewayEmpty(t *testing.T) {
	resetCodexHarvestFlow()
	t.Cleanup(resetCodexHarvestFlow)
	account := ticketTestAccount(4242)
	for _, gateway := range []string{"", "any"} {
		recordCodexHarvestProbe(account, "gpt-6-astra", "network_error", "relay-a", gateway, "", 0, 0, 0, 780, 33)
	}
	entries := listCodexTicketHistory(account.ID, "gpt-6-astra")
	require.Len(t, entries, 2)
	for _, entry := range entries {
		require.Empty(t, entry.Gateway)
	}
}

func TestCodexTicketLogsCaptureRelayGateway(t *testing.T) {
	for _, tc := range []struct {
		name, servedModel, event string
	}{
		{name: "acquired", servedModel: "gpt-6-astra", event: "acquired"},
		{name: "model mismatch", servedModel: "gpt-5.6-luna", event: "miss"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			resetCodexHarvestFlow()
			t.Cleanup(resetCodexHarvestFlow)
			account := ticketTestAccount(4242)
			model := "gpt-6-astra"
			now := time.Now().Truncate(time.Second)
			expires := now.Add(4 * time.Minute)
			pair := mint780Pair(expires, "unified-88")
			cfg := config.OpenAICodexTicketConfig{
				Enabled: true, Models: []string{model}, TargetLength: 780, TTLSeconds: 240,
				CloudMint: config.OpenAICodexCloudMintConfig{
					Enabled: true, URL: "https://relay.example/", Key: "relay-secret", Gateway: "any",
				},
			}
			s := ticketTestService(t, cfg, nil)
			s.httpUpstream = &harvestProxyUpstream{do: func(req *http.Request, _ string) (*http.Response, error) {
				require.Equal(t, "any", req.Header.Get("X-Mint-Gateway"))
				body := cloudMintResponseBody(t, model, tc.servedModel, mint780State(now), now, expires, pair)
				return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(body))}, nil
			}}
			s.probeOnceOpenAICodexTicket(context.Background(), account, model)
			logs, err := s.OpenAICodexTicketLogs(context.Background(), account, model, time.Now())
			require.NoError(t, err)
			require.Len(t, logs.Entries, 1)
			require.Equal(t, tc.event, logs.Entries[0].Event)
			require.Equal(t, "unified-88", logs.Entries[0].Gateway)
		})
	}
}
