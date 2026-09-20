package service

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestCodexTicketTelemetryAttemptsAndReadOnlyLogs(t *testing.T) {
	s, a := ticketFixture()
	cfg := s.codexTicketConfig(context.Background())
	u := &codexTicketMockUpstream{do: func(r *http.Request, _ string) (*http.Response, error) {
		observer := OpenAICodexTicketEgressObserverFromContext(r.Context())
		require.NotNil(t, observer)
		observer(OpenAICodexTicketEgressResult{IP: "103.131.213.7", CountryCode: "PK"})
		return ticketResponse(200, 356), nil
	}}
	s.httpUpstream = u
	s.probeCodexTicket(context.Background(), a, "ticket-model", cfg)
	statuses := s.OpenAICodexTicketStatuses(context.Background(), a, time.Now())
	require.Equal(t, 1, statuses[0].Attempts)
	require.Equal(t, "cooldown", statuses[0].State)
	s.probeCodexTicket(context.Background(), a, "ticket-model", cfg)
	require.EqualValues(t, 1, u.calls.Load())
	s.codexTicketMissBackoffs.Delete(codexTicketKey(a.ID, "ticket-model"))
	u.do = func(r *http.Request, _ string) (*http.Response, error) {
		OpenAICodexTicketEgressObserverFromContext(r.Context())(OpenAICodexTicketEgressResult{IP: "103.131.213.7", CountryCode: "PK"})
		return ticketResponse(200, 292), nil
	}
	s.probeCodexTicket(context.Background(), a, "ticket-model", cfg)
	for i := 0; i < 3; i++ {
		logs, err := s.OpenAICodexTicketLogs(context.Background(), a, "ticket-model", time.Now())
		require.NoError(t, err)
		require.Len(t, logs.Entries, 4)
		require.Equal(t, 2, logs.Status.Attempts)
		require.True(t, logs.Status.Ready)
		require.False(t, logs.Status.Harvesting)
		require.Equal(t, "acquired", logs.Entries[3].Event)
		require.Equal(t, "103.131.213.7", logs.Entries[2].EgressIP)
		require.Equal(t, "PK", logs.Entries[3].EgressCountryCode)
		require.Equal(t, 356, *logs.Entries[1].TicketLength)
		require.NotNil(t, logs.Entries[3].DurationMS)
		encoded, err := json.Marshal(logs)
		require.NoError(t, err)
		for _, secret := range []string{"test-access-token", "proxy.example", "gAAAAA", "test-workspace"} {
			require.NotContains(t, string(encoded), secret)
		}
	}
	require.EqualValues(t, 2, u.calls.Load())
	s.probeCodexTicket(context.Background(), a, "ticket-model", cfg)
	status := s.OpenAICodexTicketStatuses(context.Background(), a, time.Now())[0]
	require.Equal(t, 1, status.Attempts)
}
func TestCodexTicketTelemetryStatusScopeAndPause(t *testing.T) {
	s, a := ticketFixture()
	installTicket(s, a, "ticket-model")
	status := s.OpenAICodexTicketStatuses(context.Background(), a, time.Now())[0]
	require.Greater(t, status.RemainingSeconds, int64(3500))
	other := *a
	other.ID++
	logs, err := s.OpenAICodexTicketLogs(context.Background(), &other, "ticket-model", time.Now())
	require.NoError(t, err)
	require.Empty(t, logs.Entries)
	require.False(t, logs.Status.Ready)
	_, err = s.OpenAICodexTicketLogs(context.Background(), a, "unknown-model", time.Now())
	require.ErrorIs(t, err, ErrOpenAICodexTicketLogModel)
	s.pauseCodexTicket(a, 401, nil)
	require.Equal(t, "token_invalid", s.OpenAICodexTicketStatuses(context.Background(), a, time.Now())[0].State)
	a.Credentials["access_token"] = "updated"
	require.NotEqual(t, "token_invalid", s.OpenAICodexTicketStatuses(context.Background(), a, time.Now())[0].State)
	s.pauseCodexTicket(a, 429, nil)
	status = s.OpenAICodexTicketStatuses(context.Background(), a, time.Now())[0]
	require.Equal(t, "cooldown", status.State)
	require.NotNil(t, status.NextHarvestAt)
	s.cfg.Gateway.OpenAICodexTicket.Enabled = false
	require.Empty(t, s.OpenAICodexTicketStatuses(context.Background(), a, time.Now()))
}
func TestCodexTicketTelemetryBoundedAndCopied(t *testing.T) {
	var store codexTicketTelemetryStore
	_, a := ticketFixture()
	for i := 0; i < 150; i++ {
		attempt, _ := store.begin(a, "model")
		length := i
		store.finish(a.ID, "model", OpenAICodexTicketLogEntry{Attempt: attempt, Event: "miss", TicketLength: &length})
	}
	logs, attempts, harvesting, _ := store.snapshot(a.ID, "model")
	require.Len(t, logs, 200)
	require.Equal(t, 150, attempts)
	require.False(t, harvesting)
	*logs[len(logs)-1].TicketLength = -1
	again, _, _, _ := store.snapshot(a.ID, "model")
	require.Equal(t, 149, *again[len(again)-1].TicketLength)
	for i := 0; i < 600; i++ {
		store.begin(a, fmt.Sprint(i))
	}
	require.Len(t, store.streams, 512)
	var wg sync.WaitGroup
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 20; j++ {
				store.snapshot(a.ID, "599")
				store.progress(a.ID, "599")
			}
		}()
	}
	wg.Wait()
}

type codexTicketTelemetryRepo struct {
	AccountRepository
	accounts []*Account
	calls    int
}

func (r *codexTicketTelemetryRepo) GetByIDs(context.Context, []int64) ([]*Account, error) {
	r.calls++
	return r.accounts, nil
}
func TestCodexTicketTelemetryBatchUsesOneLocalLookup(t *testing.T) {
	s, a := ticketFixture()
	repo := &codexTicketTelemetryRepo{accounts: []*Account{a}}
	s.accountRepo = repo
	result, err := s.OpenAICodexTicketStatusBatch(context.Background(), []int64{a.ID, 999})
	require.NoError(t, err)
	require.Len(t, result[a.ID], 1)
	require.Empty(t, result[999])
	require.Equal(t, 1, repo.calls)
}
func TestCodexTicketEgressSanitizesDiagnostics(t *testing.T) {
	got := normalizeOpenAICodexTicketEgressResult(OpenAICodexTicketEgressResult{IP: "103.131.213.7", CountryCode: "pk"})
	require.Equal(t, "PK", got.CountryCode)
	got = normalizeOpenAICodexTicketEgressResult(OpenAICodexTicketEgressResult{IP: "127.0.0.1", CountryCode: "PK"})
	require.Empty(t, got.IP)
	require.Empty(t, got.CountryCode)
	got = normalizeOpenAICodexTicketEgressResult(OpenAICodexTicketEgressResult{Error: &OpenAICodexTicketEgressError{Reason: strings.Repeat("secret", 10)}})
	require.Equal(t, "unknown", got.Error.Reason)
}
