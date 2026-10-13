package service

import (
	"context"
	"errors"
	"net/http"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
)

type scopedHarvestTag struct{}

func TestScopedHarvestDifferentAccountsOverlap(t *testing.T) {
	scopedHarvestOverlap(t, 42, "gpt-6-astra")
}

func TestScopedHarvestDifferentModelsOverlap(t *testing.T) {
	scopedHarvestOverlap(t, 41, "gpt-6-sol")
}

func scopedHarvestOverlap(t *testing.T, secondID int64, secondModel string) {
	t.Helper()
	resetCodexHarvestFlow()
	t.Cleanup(resetCodexHarvestFlow)
	ctx, cancel := context.WithTimeout(t.Context(), 3*time.Second)
	defer cancel()
	started := make(chan int, 2)
	proceed := make(chan struct{})
	s := ticketTestService(t, config.OpenAICodexTicketConfig{TTLSeconds: 3600}, &codexTicketFuncUpstream{do: func(req *http.Request) (*http.Response, error) {
		tag := req.Context().Value(scopedHarvestTag{}).(int)
		started <- tag
		select {
		case <-proceed:
		case <-req.Context().Done():
			return nil, req.Context().Err()
		}
		return nil, errors.New("fixture finished")
	}})
	s.accountRepo = &astraTicketRepo{accounts: map[int64]*Account{41: ticketTestAccount(41), 42: ticketTestAccount(42)}}
	done := make(chan error, 2)
	run := func(id int64, model string, tag int) {
		done <- s.ExecuteManualHarvest(context.WithValue(ctx, scopedHarvestTag{}, tag), ManualHarvestRequest{AccountID: id, Models: []string{model}, MaxAttempts: 1, ProbeIntervalSeconds: 1}, nil)
	}
	go run(41, "gpt-6-astra", 1)
	select {
	case <-started:
	case <-ctx.Done():
		t.Fatal("first request did not start")
	}
	go run(secondID, secondModel, 2)
	overlapped := false
	remaining := 2
	select {
	case tag := <-started:
		overlapped = tag == 2
	case e := <-done:
		remaining--
		t.Logf("second admission: %v", e)
	case <-time.After(300 * time.Millisecond):
	}
	close(proceed)
	cancel()
	for range remaining {
		<-done
	}
	t.Logf("different scope overlap=%t", overlapped)
	require.True(t, overlapped)
}

func TestScopedHarvestSameModelResumesAfterRelease(t *testing.T) {
	resetCodexHarvestFlow()
	t.Cleanup(resetCodexHarvestFlow)
	ctx, cancel := context.WithTimeout(t.Context(), 3*time.Second)
	defer cancel()
	started := make(chan int, 2)
	proceed := make(chan struct{})
	s := ticketTestService(t, config.OpenAICodexTicketConfig{TTLSeconds: 3600}, &codexTicketFuncUpstream{do: func(req *http.Request) (*http.Response, error) {
		tag := req.Context().Value(scopedHarvestTag{}).(int)
		started <- tag
		if tag == 1 {
			select {
			case <-proceed:
			case <-req.Context().Done():
				return nil, req.Context().Err()
			}
		}
		return nil, errors.New("fixture finished")
	}})
	s.accountRepo = &astraTicketRepo{accounts: map[int64]*Account{41: ticketTestAccount(41)}}
	done := make(chan error, 2)
	for _, tag := range []int{1, 2} {
		go func(tag int) {
			done <- s.ExecuteManualHarvest(context.WithValue(ctx, scopedHarvestTag{}, tag), ManualHarvestRequest{AccountID: 41, Models: []string{"gpt-6-astra"}, MaxAttempts: 1, ProbeIntervalSeconds: 1}, nil)
		}(tag)
		if tag == 1 {
			<-started
		}
	}
	premature := false
	select {
	case <-started:
		premature = true
	case <-time.After(20 * time.Millisecond):
	}
	close(proceed)
	resumed := false
	select {
	case tag := <-started:
		resumed = tag == 2
	case <-time.After(1500 * time.Millisecond):
	}
	cancel()
	<-done
	<-done
	t.Logf("same model premature=%t resumed=%t", premature, resumed)
	require.False(t, premature)
	require.True(t, resumed)
}

func TestScopedHarvestSameModelQueuesAndCancels(t *testing.T) {
	resetCodexHarvestFlow()
	t.Cleanup(resetCodexHarvestFlow)
	started := make(chan struct{}, 1)
	proceed := make(chan struct{})
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	s := ticketTestService(t, config.OpenAICodexTicketConfig{TTLSeconds: 3600}, &codexTicketFuncUpstream{do: func(req *http.Request) (*http.Response, error) {
		started <- struct{}{}
		select {
		case <-proceed:
		case <-req.Context().Done():
			return nil, req.Context().Err()
		}
		return nil, errors.New("fixture finished")
	}})
	s.accountRepo = &astraTicketRepo{accounts: map[int64]*Account{41: ticketTestAccount(41)}}
	req := ManualHarvestRequest{AccountID: 41, Models: []string{"gpt-6-astra"}, MaxAttempts: 1, ProbeIntervalSeconds: 1}
	done := make(chan error, 1)
	go func() { done <- s.ExecuteManualHarvest(ctx, req, nil) }()
	<-started
	wait, stop := context.WithTimeout(ctx, 50*time.Millisecond)
	err := s.ExecuteManualHarvest(wait, req, nil)
	stop()
	close(proceed)
	cancel()
	<-done
	t.Logf("same scope waiter: %v", err)
	require.ErrorIs(t, err, context.DeadlineExceeded)
}
