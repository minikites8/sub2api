package service

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
)

func TestCodexHarvestFlowRecordsProbeTicketAndSelect(t *testing.T) {
	resetCodexHarvestFlow()
	t.Cleanup(resetCodexHarvestFlow)

	account := ticketTestAccount(2)
	account.Name = "20x"
	recordCodexHarvestNode("airport-node-1", "load-balance", 340)
	recordCodexHarvestProbe(account, "gpt-6-astra", "invalid_state", lastCodexHarvestNode(), "", 200, 312, 11, 292, 10)
	recordCodexHarvestTicketReject(account, "gpt-6-astra", "response_mismatch", 312, 11)
	recordCodexHarvestSelect(account, "gpt-6-astra", "skip", "ticket_unavailable", "")
	recordCodexHarvestSelect(account, "gpt-6-astra", "skip", "ticket_unavailable", "")

	events := listCodexHarvestFlowEvents()
	require.Len(t, events, 4)
	require.Equal(t, "node", events[0].Stage)
	require.Equal(t, "airport-node-1", events[0].Node)
	require.Equal(t, "probe_miss", events[1].Kind)
	require.Equal(t, 312, events[1].Length)
	require.Equal(t, 292, events[1].ExpectedLength)
	require.Equal(t, "reject", events[2].Kind)
	require.Equal(t, "skip", events[3].Kind)
}

func TestBuildCodexHarvestFlowSnapshotAndStages(t *testing.T) {
	resetCodexHarvestFlow()
	t.Cleanup(resetCodexHarvestFlow)

	account := ticketTestAccount(2)
	account.Name = "20x"
	account.Extra = map[string]any{
		openAICodexTicketExtraKey("gpt-6-astra"): &openAICodexTicket{
			AccountID:  2,
			Model:      "gpt-6-astra",
			State:      fakeCodexTicketState(292),
			Length:     292,
			Blocks:     10,
			ExpiresAt:  time.Now().Add(time.Hour),
			CapturedAt: time.Now(),
			Identity:   ticketIdentity(account),
		},
	}
	recordCodexHarvestProbe(account, "gpt-6-astra", "success", "airport-node-2", "", 200, 292, 10, 292, 10)
	recordCodexHarvestTicketStore(account, &openAICodexTicket{Model: "gpt-6-astra", Length: 292, Blocks: 10}, false)
	recordCodexHarvestSelect(account, "gpt-6-astra", "selected", "", "")

	snapshot := BuildCodexHarvestFlow(context.Background(), &config.Config{
		Gateway: config.GatewayConfig{OpenAICodexTicket: config.OpenAICodexTicketConfig{
			Enabled:      true,
			FailClosed:   true,
			TargetLength: 292,
			Models:       []string{"gpt-6-astra", "gpt-5.6-sol"},
		}},
	}, nil, []Account{*account})

	require.True(t, snapshot.Harvest.Enabled)
	require.True(t, snapshot.Harvest.FailClosed)
	require.Equal(t, 292, snapshot.Harvest.TargetLength)
	require.Len(t, snapshot.Accounts, 1)
	require.Equal(t, 1, snapshot.Accounts[0].ReadyCount)
	require.False(t, snapshot.Accounts[0].SkipHarvest)
	require.True(t, snapshot.Accounts[0].InScope)
	require.Equal(t, CodexAccountAvailabilityAvailable, snapshot.Accounts[0].Availability)
	require.Equal(t, 1, snapshot.Counts.TicketsReady)
	require.Equal(t, 1, snapshot.Counts.ProbeHit)
	require.Equal(t, 1, snapshot.Counts.TicketAccept)
	require.Zero(t, snapshot.Counts.SelectOK)
	require.Len(t, snapshot.Stages, 5)
	require.Equal(t, "probe", snapshot.Stages[1].ID)
	require.Equal(t, "ok", snapshot.Stages[1].Status)
	require.Equal(t, "shape", snapshot.Stages[2].ID)
	require.Equal(t, "ok", snapshot.Stages[2].Status)
	require.Equal(t, 292, snapshot.Stages[2].Length)
	require.Equal(t, 10, snapshot.Stages[2].Blocks)
	require.Equal(t, 292, snapshot.Stages[2].ExpectedLength)
	var kinds []string
	for _, event := range snapshot.Events {
		kinds = append(kinds, event.Kind)
	}
	require.NotContains(t, kinds, "selected")
}

func TestBuildCodexHarvestFlowSkipHarvestAccount(t *testing.T) {
	resetCodexHarvestFlow()
	t.Cleanup(resetCodexHarvestFlow)

	account := ticketTestAccount(5)
	account.Name = "5x"
	account.Extra = map[string]any{
		OpenAICodexSkipHarvestExtraKey: true,
		openAICodexTicketExtraKey("gpt-6-astra"): &openAICodexTicket{
			AccountID:  5,
			Model:      "gpt-6-astra",
			State:      fakeCodexTicketState(292),
			Length:     292,
			Blocks:     10,
			ExpiresAt:  time.Now().Add(time.Hour),
			CapturedAt: time.Now(),
			Identity:   ticketIdentity(account),
		},
	}
	snapshot := BuildCodexHarvestFlow(context.Background(), &config.Config{
		Gateway: config.GatewayConfig{OpenAICodexTicket: config.OpenAICodexTicketConfig{
			Enabled:      true,
			FailClosed:   true,
			TargetLength: 292,
			Models:       []string{"gpt-6-astra", "gpt-5.6-sol"},
		}},
	}, nil, []Account{*account})
	require.Len(t, snapshot.Accounts, 1)
	require.True(t, snapshot.Accounts[0].SkipHarvest)
	require.False(t, snapshot.Accounts[0].InScope)
	require.Equal(t, 1, snapshot.Accounts[0].ReadyCount)
	require.Equal(t, 0, snapshot.Accounts[0].BlockedCount)
	require.False(t, snapshot.Accounts[0].Tickets[0].Blocked)
	require.False(t, snapshot.Accounts[0].Tickets[1].Ready)
	require.False(t, snapshot.Accounts[0].Tickets[1].Blocked)
}

func TestClipFlowTextKeepsRunes(t *testing.T) {
	require.Equal(t, "账号名称", clipFlowText("账号名称", 80))
	require.Equal(t, "账号", clipFlowText("账号名称", 2))
	require.Empty(t, clipFlowText("abc", 0))
}

func TestBuildCodexHarvestFlowAppliesRuntimeEnabled(t *testing.T) {
	resetCodexHarvestFlow()
	t.Cleanup(resetCodexHarvestFlow)
	account := ticketTestAccount(2)
	account.Name = "20x"
	account.Extra = map[string]any{
		openAICodexTicketExtraKey("gpt-6-astra"): &openAICodexTicket{
			AccountID:  2,
			Model:      "gpt-6-astra",
			State:      fakeCodexTicketState(292),
			Length:     292,
			Blocks:     10,
			ExpiresAt:  time.Now().Add(time.Hour),
			CapturedAt: time.Now(),
			Identity:   ticketIdentity(account),
		},
	}
	disabled := BuildCodexHarvestFlow(context.Background(), &config.Config{
		Gateway: config.GatewayConfig{OpenAICodexTicket: config.OpenAICodexTicketConfig{
			Enabled:      false,
			FailClosed:   true,
			TargetLength: 292,
			Models:       []string{"gpt-6-astra", "gpt-5.6-sol"},
		}},
	}, nil, []Account{*account})
	require.True(t, disabled.Harvest.FailClosed)
	require.False(t, disabled.Harvest.Enabled)
	require.Len(t, disabled.Accounts, 1)
	require.NotNil(t, disabled.Accounts[0].Tickets)
	require.Empty(t, disabled.Accounts[0].Tickets)
	raw, err := json.Marshal(disabled)
	require.NoError(t, err)
	require.Contains(t, string(raw), `"tickets":[]`)
	require.NotContains(t, string(raw), `"tickets":null`)

	none := BuildCodexHarvestFlow(context.Background(), &config.Config{
		Gateway: config.GatewayConfig{OpenAICodexTicket: config.OpenAICodexTicketConfig{
			Enabled: true, FailClosed: true, TargetLength: 292, Models: []string{"gpt-6-astra"},
		}},
	}, nil, nil)
	require.NotNil(t, none.Accounts)
	require.Empty(t, none.Accounts)
	emptyRaw, err := json.Marshal(none)
	require.NoError(t, err)
	require.Contains(t, string(emptyRaw), `"accounts":[]`)
	require.NotContains(t, string(emptyRaw), `"accounts":null`)

	enabled := BuildCodexHarvestFlow(context.Background(), &config.Config{
		Gateway: config.GatewayConfig{OpenAICodexTicket: config.OpenAICodexTicketConfig{
			Enabled:      true,
			FailClosed:   true,
			TargetLength: 292,
			Models:       []string{"gpt-6-astra", "gpt-5.6-sol"},
		}},
	}, nil, []Account{*account})
	require.True(t, enabled.Harvest.Enabled)
	require.NotEmpty(t, enabled.Accounts[0].Tickets)
	require.Equal(t, 1, enabled.Accounts[0].ReadyCount)
}

func TestCodexHarvestSelectFailedDebounces(t *testing.T) {
	resetCodexHarvestFlow()
	t.Cleanup(resetCodexHarvestFlow)
	recordCodexHarvestSelect(nil, "gpt-6-astra", "failed", "unavailable", "no available OpenAI accounts")
	recordCodexHarvestSelect(nil, "gpt-6-astra", "failed", "unavailable", "no available OpenAI accounts")
	require.Len(t, listCodexHarvestFlowEvents(), 1)
}

func TestShapeStageKeeps292AfterUnauthorizedProbe(t *testing.T) {
	resetCodexHarvestFlow()
	t.Cleanup(resetCodexHarvestFlow)
	account := ticketTestAccount(2)
	account.Name = "20x"
	recordCodexHarvestProbe(account, "gpt-6-astra", "success", "japan-08", "", 200, 292, 10, 292, 10)
	dead := ticketTestAccount(3)
	dead.Name = "5x"
	recordCodexHarvestProbe(dead, "gpt-6-astra", "response_incomplete_or_error", "japan-08", "", 401, 0, 0, 292, 10)

	snapshot := BuildCodexHarvestFlow(context.Background(), &config.Config{
		Gateway: config.GatewayConfig{OpenAICodexTicket: config.OpenAICodexTicketConfig{
			Enabled:      true,
			FailClosed:   true,
			TargetLength: 292,
			Models:       []string{"gpt-6-astra", "gpt-5.6-sol"},
		}},
	}, nil, []Account{*account})
	require.Equal(t, "warn", snapshot.Stages[1].Status)
	require.Equal(t, "shape", snapshot.Stages[2].ID)
	require.Equal(t, "ok", snapshot.Stages[2].Status)
	require.Equal(t, 292, snapshot.Stages[2].Length)
}

func TestShapeStageFailsOnWrongTicketLength(t *testing.T) {
	resetCodexHarvestFlow()
	t.Cleanup(resetCodexHarvestFlow)
	account := ticketTestAccount(2)
	recordCodexHarvestProbe(account, "gpt-6-astra", "invalid_state", "japan-08", "", 200, 312, 11, 292, 10)
	stages := buildCodexHarvestFlowStages(CodexHarvestFlowSnapshot{Events: listCodexHarvestFlowEvents()})
	require.Equal(t, "fail", stages[2].Status)
	require.Contains(t, stages[2].Detail, "312")
}

func TestRecordCodexHarvestNodeReusesPoolSize(t *testing.T) {
	resetCodexHarvestFlow()
	t.Cleanup(resetCodexHarvestFlow)
	recordCodexHarvestNode("japan-07", "LoadBalance", 340)
	recordCodexHarvestNode("japan-08", "", 0)
	events := listCodexHarvestFlowEvents()
	require.Len(t, events, 2)
	require.Equal(t, "pool=340", events[1].Detail)
	require.Equal(t, "LoadBalance", events[1].Result)
}

func TestBuildCodexHarvestFlowAvailabilityWindows(t *testing.T) {
	resetCodexHarvestFlow()
	t.Cleanup(resetCodexHarvestFlow)
	account := ticketTestAccount(2)
	until := time.Now().Add(2 * time.Hour)
	account.RateLimitResetAt = &until
	snapshot := BuildCodexHarvestFlow(context.Background(), &config.Config{
		Gateway: config.GatewayConfig{OpenAICodexTicket: config.OpenAICodexTicketConfig{
			Enabled: true, Models: []string{"gpt-6-astra"},
		}},
	}, nil, []Account{*account})
	require.Equal(t, CodexAccountAvailabilityRateLimited, snapshot.Accounts[0].Availability)
	require.NotNil(t, snapshot.Accounts[0].RecoverAt)
}

func TestRecordCodexHarvestProbeHumanizesMiss(t *testing.T) {
	resetCodexHarvestFlow()
	t.Cleanup(resetCodexHarvestFlow)
	account := ticketTestAccount(2)
	recordCodexHarvestProbe(account, "gpt-6-astra", "invalid_state", "node-a", "", 200, 312, 11, 292, 10)
	events := listCodexHarvestFlowEvents()
	require.NotEmpty(t, events)
	require.Contains(t, events[len(events)-1].Detail, "降智")
}

type memoryHarvestFlowStore struct {
	mu     sync.Mutex
	events []CodexHarvestFlowEvent
	fail   error
}

func (s *memoryHarvestFlowStore) List(_ context.Context, limit int) ([]CodexHarvestFlowEvent, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.fail != nil {
		return nil, s.fail
	}
	if limit < 1 || limit > len(s.events) {
		limit = len(s.events)
	}
	out := make([]CodexHarvestFlowEvent, limit)
	copy(out, s.events[len(s.events)-limit:])
	return out, nil
}

func (s *memoryHarvestFlowStore) Append(_ context.Context, event CodexHarvestFlowEvent) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.fail != nil {
		return s.fail
	}
	s.events = append(s.events, event)
	if overflow := len(s.events) - 200; overflow > 0 {
		s.events = append([]CodexHarvestFlowEvent(nil), s.events[overflow:]...)
	}
	return nil
}

func TestHarvestFlowPersistsAcrossRestart(t *testing.T) {
	resetCodexHarvestFlow()
	store := &memoryHarvestFlowStore{}
	bindCodexHarvestFlowStore(store)
	t.Cleanup(func() {
		bindCodexHarvestFlowStore(nil)
		resetCodexHarvestFlow()
	})
	account := ticketTestAccount(2)
	account.Name = "20x"
	recordCodexHarvestProbe(account, "gpt-6-astra", "success", "japan-08", "", 200, 292, 10, 292, 10)
	require.Len(t, store.events, 1)
	resetCodexHarvestFlow()
	require.Empty(t, listCodexHarvestFlowEvents())
	bindCodexHarvestFlowStore(store)
	events := listCodexHarvestFlowEvents()
	require.Len(t, events, 1)
	require.Equal(t, "probe_hit", events[0].Kind)
	require.Equal(t, "japan-08", events[0].Node)
}

func TestHarvestFlowPersistFailureKeepsMemory(t *testing.T) {
	resetCodexHarvestFlow()
	store := &memoryHarvestFlowStore{fail: errors.New("db down")}
	bindCodexHarvestFlowStore(store)
	t.Cleanup(func() {
		bindCodexHarvestFlowStore(nil)
		resetCodexHarvestFlow()
	})
	recordCodexHarvestSelect(nil, "gpt-6-astra", "failed", "unavailable", "no available OpenAI accounts")
	events := listCodexHarvestFlowEvents()
	require.Len(t, events, 1)
	require.Empty(t, store.events)
}

func TestExternalHarvestProxyStatus(t *testing.T) {
	resetCodexHarvestFlow()
	t.Cleanup(resetCodexHarvestFlow)
	recordCodexHarvestNode("prior-node", "LoadBalance", 1)
	for _, proxy := range []string{"http://user:password@residential.example:8080", ""} {
		snapshot := BuildCodexHarvestFlow(context.Background(), &config.Config{Gateway: config.GatewayConfig{OpenAICodexTicket: config.OpenAICodexTicketConfig{HarvestProxyURL: proxy}}}, nil, nil)
		require.Empty(t, snapshot.Sidecar.Error)
		require.Empty(t, snapshot.Sidecar.Now)
		require.Empty(t, snapshot.Stages[0].Node)
		require.False(t, snapshot.Sidecar.Reachable)
		if proxy == "" {
			require.Equal(t, "unconfigured", snapshot.Sidecar.Mode)
		} else {
			require.Equal(t, "external", snapshot.Sidecar.Mode)
		}
	}
}
