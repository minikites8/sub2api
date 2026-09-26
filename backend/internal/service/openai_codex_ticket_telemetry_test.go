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
