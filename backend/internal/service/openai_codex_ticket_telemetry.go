package service

import (
	"context"
	"errors"
	"sort"
	"strings"
	"time"
)

// The administrator endpoint exposes a bounded per-account/model history
// backed by the harvest-flow ring and the latest live ticket status.
const OpenAICodexTicketLogLimit = 200

var ErrOpenAICodexTicketLogModel = errors.New("model is not configured for Codex tickets")

type OpenAICodexTicketLogEntry struct {
	ID                int64     `json:"id"`
	Time              time.Time `json:"time"`
	Attempt           int       `json:"attempt"`
	Event             string    `json:"event"`
	Reason            string    `json:"reason"`
	HTTPStatus        int       `json:"http_status,omitempty"`
	TicketLength      *int      `json:"ticket_length,omitempty"`
	TargetLength      int       `json:"target_length"`
	DurationMS        int64     `json:"duration_ms,omitempty"`
	Gateway           string    `json:"gateway,omitempty"`
	EdgeIP            string    `json:"edge_ip,omitempty"`
	EgressIP          string    `json:"egress_ip,omitempty"`
	EgressCountryCode string    `json:"egress_country_code,omitempty"`
}
type OpenAICodexTicketLogs struct {
	GatewayBlacklist []string                    `json:"gateway_blacklist"`
	Model            string                      `json:"model"`
	Entries          []OpenAICodexTicketLogEntry `json:"entries"`
	Status           *OpenAICodexTicketStatus    `json:"status"`
	Limit            int                         `json:"limit"`
	FetchedAt        time.Time                   `json:"fetched_at"`
}

func codexTicketHistoryReason(event CodexHarvestFlowEvent) string {
	result := strings.ToLower(strings.TrimSpace(event.Result))
	switch result {
	case "success":
		return "target_length_matched"
	case "missing_state", "ticket_missing":
		return "missing_state"
	case "invalid_prefix":
		return "invalid_prefix"
	case "timeout", "deadline_exceeded":
		return "timeout"
	case "canceled", "cancelled", "context_canceled":
		return "canceled"
	case "network_error", "connection_error":
		return "network_error"
	case "empty_response":
		return "empty_response"
	case "expired_ticket", "invalid_route", "model_mismatch", "ticket_timestamp_mismatch", "ticket_length_mismatch":
		return result
	case "invalid_state", "response_mismatch":
		if event.Length > 0 && event.ExpectedLength > 0 && event.Length != event.ExpectedLength {
			return "length_mismatch"
		}
		if result == "invalid_state" {
			return "invalid_state"
		}
	}
	if event.HTTPStatus >= 400 {
		return "http_error"
	}
	return "request_error"
}

func codexTicketHistoryEvent(event CodexHarvestFlowEvent) string {
	if event.Kind == "probe_hit" {
		return "acquired"
	}
	result := strings.ToLower(strings.TrimSpace(event.Result))
	switch result {
	case "account_error", "token_error", "network_error", "timeout", "deadline_exceeded", "canceled", "cancelled", "context_canceled", "request_error", "cloud_mint_auth_error", "cloud_mint_unavailable":
		return "error"
	}
	if event.HTTPStatus >= 400 {
		return "error"
	}
	return "miss"
}

// appendCodexTicketHistoryLocked keeps a per-account/model bounded history while
// the harvest-flow ring records the same event for the global diagnostic view.
// The caller holds defaultCodexHarvestFlow.mu.
func appendCodexTicketHistoryLocked(event CodexHarvestFlowEvent) {
	if event.AccountID <= 0 || strings.TrimSpace(event.Model) == "" {
		return
	}
	if event.Stage != "probe" && !(event.Stage == "ticket" && event.Kind == "reject") {
		return
	}
	if event.At.IsZero() {
		event.At = time.Now().UTC()
	}
	key := openAICodexTicketKey(event.AccountID, event.Model)
	sequence := defaultCodexHarvestFlow.ticketLogSequence[key] + 1
	defaultCodexHarvestFlow.ticketLogSequence[key] = sequence
	entry := OpenAICodexTicketLogEntry{
		ID:           event.At.UnixNano() + int64(sequence),
		Time:         event.At,
		Attempt:      sequence,
		Event:        codexTicketHistoryEvent(event),
		Reason:       codexTicketHistoryReason(event),
		HTTPStatus:   event.HTTPStatus,
		TargetLength: event.ExpectedLength,
		Gateway:      event.Gateway,
		EdgeIP:       event.EdgeIP,
	}
	if event.Length > 0 {
		length := event.Length
		entry.TicketLength = &length
	}
	entries := append(defaultCodexHarvestFlow.ticketLogs[key], entry)
	if overflow := len(entries) - OpenAICodexTicketLogLimit; overflow > 0 {
		entries = append([]OpenAICodexTicketLogEntry(nil), entries[overflow:]...)
	}
	defaultCodexHarvestFlow.ticketLogs[key] = entries
}

func codexTicketHistoryAttempts(accountID int64, model string) int {
	defaultCodexHarvestFlow.mu.Lock()
	defer defaultCodexHarvestFlow.mu.Unlock()
	return defaultCodexHarvestFlow.ticketLogSequence[openAICodexTicketKey(accountID, model)]
}

func listCodexTicketHistory(accountID int64, model string) []OpenAICodexTicketLogEntry {
	if accountID <= 0 || strings.TrimSpace(model) == "" {
		return []OpenAICodexTicketLogEntry{}
	}
	key := openAICodexTicketKey(accountID, model)
	defaultCodexHarvestFlow.mu.Lock()
	entries := append([]OpenAICodexTicketLogEntry(nil), defaultCodexHarvestFlow.ticketLogs[key]...)
	defaultCodexHarvestFlow.mu.Unlock()
	sort.SliceStable(entries, func(i, j int) bool { return entries[i].ID > entries[j].ID })
	return entries
}

func (s *OpenAIGatewayService) OpenAICodexTicketStatuses(ctx context.Context, account *Account, now time.Time) []OpenAICodexTicketStatus {
	if s == nil || account == nil || !s.openAICodexTicketEnabledContext(ctx) {
		return []OpenAICodexTicketStatus{}
	}
	return OpenAICodexTicketStatuses(account, s.openAICodexTicketConfig(), now)
}
func (s *OpenAIGatewayService) OpenAICodexTicketStatusBatch(ctx context.Context, ids []int64) (map[int64][]OpenAICodexTicketStatus, error) {
	out := make(map[int64][]OpenAICodexTicketStatus, len(ids))
	for _, id := range ids {
		out[id] = []OpenAICodexTicketStatus{}
	}
	if s == nil || s.accountRepo == nil {
		return out, nil
	}
	accounts, err := s.accountRepo.GetByIDs(ctx, ids)
	if err != nil {
		return nil, err
	}
	now := time.Now()
	for _, account := range accounts {
		out[account.ID] = s.OpenAICodexTicketStatuses(ctx, account, now)
	}
	return out, nil
}
func (s *OpenAIGatewayService) OpenAICodexTicketLogs(ctx context.Context, account *Account, model string, now time.Time) (*OpenAICodexTicketLogs, error) {
	model = normalizeOpenAICodexTicketModel(strings.TrimSpace(model))
	if s == nil || model == "" || !s.openAICodexTicketGatedModel(model) {
		return nil, ErrOpenAICodexTicketLogModel
	}
	entries := []OpenAICodexTicketLogEntry{}
	if account != nil {
		entries = listCodexTicketHistory(account.ID, model)
	}
	out := &OpenAICodexTicketLogs{Model: model, Entries: entries, Limit: OpenAICodexTicketLogLimit, FetchedAt: now.UTC(), GatewayBlacklist: OpenAICodexTicketGatewayBlacklist(account)}
	for _, status := range s.OpenAICodexTicketStatuses(ctx, account, now) {
		if status.Model != model {
			continue
		}
		out.Status = &status
		if len(out.Entries) == 0 && status.Probe != nil && !status.Probe.CheckedAt.IsZero() {
			result := CodexHarvestFlowEvent{Result: status.Probe.Result, HTTPStatus: status.Probe.HTTPStatus}
			event := "miss"
			if status.Probe.Result == "success" {
				event = "acquired"
			}
			out.Entries = append(out.Entries, OpenAICodexTicketLogEntry{ID: status.Probe.CheckedAt.UnixNano(), Time: status.Probe.CheckedAt, Attempt: 1, Event: event, Reason: codexTicketHistoryReason(result), HTTPStatus: status.Probe.HTTPStatus, TargetLength: openAICodexTicketTargetLength(account, s.openAICodexTicketConfig())})
		}
		break
	}
	return out, nil
}
