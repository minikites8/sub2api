package service

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

type openAIAccountGuardRepoStub struct {
	accounts []Account
	updates  map[int64]map[string]any
}

func (r *openAIAccountGuardRepoStub) FindByExtraField(context.Context, string, any) ([]Account, error) {
	return r.accounts, nil
}

func (r *openAIAccountGuardRepoStub) UpdateExtra(_ context.Context, id int64, updates map[string]any) error {
	if r.updates == nil {
		r.updates = make(map[int64]map[string]any)
	}
	r.updates[id] = updates
	return nil
}

type openAIAccountGuardSessionsStub struct {
	mu         sync.Mutex
	devices    map[int64][]OpenAISessionDevice
	revokeErrs map[string]error
	revoked    []string
}

func (s *openAIAccountGuardSessionsStub) ListSessions(_ context.Context, accountID int64) (*OpenAISessionsResponse, error) {
	return &OpenAISessionsResponse{Devices: s.devices[accountID]}, nil
}

func (s *openAIAccountGuardSessionsStub) RevokeSession(_ context.Context, _ int64, sessionID string) (*OpenAISessionRevokeResult, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.revokeErrs[sessionID]; err != nil {
		return nil, err
	}
	s.revoked = append(s.revoked, sessionID)
	return &OpenAISessionRevokeResult{SessionID: sessionID, Revoked: true}, nil
}

func TestOpenAIAccountGuardRunDuePreservesCurrentDeviceAndContinues(t *testing.T) {
	now := time.Date(2026, 8, 17, 8, 30, 0, 0, time.UTC)
	repo := &openAIAccountGuardRepoStub{accounts: []Account{
		{
			ID:       1,
			Platform: PlatformOpenAI,
			Type:     AccountTypeOAuth,
			Extra: map[string]any{
				OpenAIAccountGuardEnabledExtraKey:         true,
				OpenAIAccountGuardIntervalMinutesExtraKey: 10,
			},
		},
		{
			ID:       2,
			Platform: PlatformOpenAI,
			Type:     AccountTypeOAuth,
			Extra: map[string]any{
				OpenAIAccountGuardEnabledExtraKey:         true,
				OpenAIAccountGuardIntervalMinutesExtraKey: 10,
				OpenAIAccountGuardLastRunAtExtraKey:       now.Add(-5 * time.Minute).Format(time.RFC3339Nano),
			},
		},
		{ID: 3, Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Extra: map[string]any{OpenAIAccountGuardEnabledExtraKey: true}},
	}}
	sessions := &openAIAccountGuardSessionsStub{
		devices: map[int64][]OpenAISessionDevice{
			1: {
				{SessionID: "current", IsCurrentDevice: true},
				{SessionID: "remote-1"},
				{SessionID: "remote-1"},
				{SessionID: "remote-2"},
			},
		},
		revokeErrs: map[string]error{"remote-1": errors.New("upstream unavailable")},
	}
	guard := NewOpenAIAccountGuardService(repo, sessions)
	guard.now = func() time.Time { return now }

	err := guard.RunDue(context.Background())

	require.Error(t, err)
	require.Equal(t, []string{"remote-2"}, sessions.revoked)
	require.Contains(t, repo.updates, int64(1))
	require.Equal(t, now.Format(time.RFC3339Nano), repo.updates[1][OpenAIAccountGuardLastRunAtExtraKey])
	require.NotContains(t, repo.updates, int64(2))
	require.NotContains(t, repo.updates, int64(3))
}

func TestNormalizeOpenAIAccountGuardExtra(t *testing.T) {
	normalized, err := normalizeOpenAIAccountGuardExtra(PlatformOpenAI, AccountTypeOAuth, map[string]any{
		OpenAIAccountGuardEnabledExtraKey: true,
	})
	require.NoError(t, err)
	require.Equal(t, OpenAIAccountGuardDefaultIntervalMinutes, normalized[OpenAIAccountGuardIntervalMinutesExtraKey])

	_, err = normalizeOpenAIAccountGuardExtra(PlatformOpenAI, AccountTypeAPIKey, map[string]any{
		OpenAIAccountGuardEnabledExtraKey: true,
	})
	require.Error(t, err)

	_, err = normalizeOpenAIAccountGuardExtra(PlatformOpenAI, AccountTypeOAuth, map[string]any{
		OpenAIAccountGuardEnabledExtraKey:         true,
		OpenAIAccountGuardIntervalMinutesExtraKey: 1,
	})
	require.Error(t, err)
}
