package service

import (
	"context"
	"encoding/json"
	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
	"testing"
	"time"
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

func TestCodexTicketLogsExposePerAccountHistory(t *testing.T) {
	resetCodexHarvestFlow()
	t.Cleanup(resetCodexHarvestFlow)
	account := ticketTestAccount(4242)
	other := ticketTestAccount(4243)
	model := "gpt-6-astra"
	recordCodexHarvestProbe(account, model, "invalid_state", "relay-a", "", 200, 312, 11, 292, 10)
	recordCodexHarvestProbe(other, model, "success", "relay-b", "", 200, 292, 10, 292, 10)
	recordCodexHarvestProbe(account, model, "success", "relay-a", "", 200, 292, 10, 292, 10)

	s := &OpenAIGatewayService{cfg: &config.Config{}}
	s.cfg.Gateway.OpenAICodexTicket = config.OpenAICodexTicketConfig{Enabled: true, Models: []string{model}}
	logs, err := s.OpenAICodexTicketLogs(context.Background(), account, model, time.Now())
	require.NoError(t, err)
	require.Len(t, logs.Entries, 2)
	require.Equal(t, "acquired", logs.Entries[0].Event)
	require.Equal(t, "target_length_matched", logs.Entries[0].Reason)
	require.Equal(t, 292, *logs.Entries[0].TicketLength)
	require.Equal(t, "miss", logs.Entries[1].Event)
	require.Equal(t, "length_mismatch", logs.Entries[1].Reason)
	require.Equal(t, 312, *logs.Entries[1].TicketLength)
}
