package service

import (
	"context"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/ctxkey"
	"github.com/stretchr/testify/require"
)

func TestGPTQoSZeroBasicWaitGetsOneImmediateAtomicAttempt(t *testing.T) {
	for _, granted := range []bool{true, false} {
		t.Run(map[bool]string{true: "available", false: "still_full"}[granted], func(t *testing.T) {
			svc, req := qualityTestGateway(true)
			req.RequiredTransport = OpenAIUpstreamTransportAny
			accounts := qualityTestAccountRepo(t, svc).accounts
			cache := &qosTerminalStatusCache{}
			cache.poll = func(_ context.Context, e GPTQoSEntry) GPTQoSGrant {
				if len(cache.entries) == 1 {
					return GPTQoSGrant{Status: "timeout"}
				}
				require.Len(t, cache.entries, 2, "zero Basic wait permits only one final poll")
				require.Len(t, e.Candidates, 1)
				require.Equal(t, "basic", e.Candidates[0].Quality)
				require.Equal(t, 1, e.WaitMs)
				require.Zero(t, e.StrongWaitMs)
				if granted {
					return GPTQoSGrant{Status: "granted", Candidate: e.Candidates[0]}
				}
				return GPTQoSGrant{Status: "waiting"}
			}
			svc.concurrencyService = NewConcurrencyService(cache)
			group := DefaultGPTQoSGroupConfig("economy")
			group.AllowBasicFallback, group.BasicWaitMs = true, 0
			ctx := context.WithValue(context.Background(), ctxkey.UserID, int64(42))
			selection, _, _, _, err := (&defaultOpenAIAccountScheduler{service: svc}).selectGPTQoSQueue(ctx, req, DefaultGPTQoSConfig(), group, []openAIAccountCandidateScore{{account: &accounts[0]}}, []openAIAccountCandidateScore{{account: &accounts[1]}}, time.Now().Add(time.Second))
			require.Len(t, cache.entries, 2)
			require.NotEqual(t, cache.entries[0].ID, cache.entries[1].ID)
			require.Contains(t, cache.closed, cache.entries[0].ID)
			if granted {
				require.NoError(t, err)
				require.Equal(t, int64(2), selection.Account.ID)
				selection.ReleaseFunc()
			} else {
				require.Nil(t, selection)
				var qosErr *GPTQoSSelectionError
				require.ErrorAs(t, err, &qosErr)
				require.Equal(t, "queue_timeout", qosErr.Reason)
			}
			require.Contains(t, cache.closed, cache.entries[1].ID)
		})
	}
}
