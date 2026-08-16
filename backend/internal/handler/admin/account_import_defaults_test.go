package admin

import (
	"context"
	"errors"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

type sessionCleanupStub struct {
	list       *service.OpenAISessionsResponse
	listErr    error
	revokeErrs map[string]error
	revoked    []string
}

func (s *sessionCleanupStub) ListSessions(context.Context, int64) (*service.OpenAISessionsResponse, error) {
	return s.list, s.listErr
}

func (s *sessionCleanupStub) RevokeSession(_ context.Context, _ int64, sessionID string) (*service.OpenAISessionRevokeResult, error) {
	if err := s.revokeErrs[sessionID]; err != nil {
		return nil, err
	}
	s.revoked = append(s.revoked, sessionID)
	return &service.OpenAISessionRevokeResult{SessionID: sessionID, Revoked: true}, nil
}

func TestRevokeOtherOpenAISessionsPreservesCurrentDeviceAndContinuesAfterFailure(t *testing.T) {
	stub := &sessionCleanupStub{
		list: &service.OpenAISessionsResponse{Devices: []service.OpenAISessionDevice{
			{SessionID: "current", IsCurrentDevice: true},
			{SessionID: "remote-1"},
			{SessionID: "remote-2"},
			{SessionID: ""},
		}},
		revokeErrs: map[string]error{"remote-1": errors.New("upstream unavailable")},
	}

	revoked, err := revokeOtherOpenAISessions(context.Background(), stub, 42)

	require.Equal(t, 1, revoked)
	require.Error(t, err)
	require.Equal(t, []string{"remote-2"}, stub.revoked)
}

func TestRevokeOtherOpenAISessionsReturnsListError(t *testing.T) {
	listErr := errors.New("list failed")
	stub := &sessionCleanupStub{listErr: listErr}

	revoked, err := revokeOtherOpenAISessions(context.Background(), stub, 42)

	require.Zero(t, revoked)
	require.ErrorIs(t, err, listErr)
}
