package service

import (
	"context"
	"errors"
	"strings"
	"time"
)

// The administrator endpoint exposes a bounded summary from the current
// per-account probe. The previous in-memory telemetry was tied to a replaced
// harvester and must not be treated as a source of live attempt history.
const OpenAICodexTicketLogLimit = 200

var ErrOpenAICodexTicketLogModel = errors.New("model is not configured for Codex tickets")

type OpenAICodexTicketLogEntry struct {
	ID           int64     `json:"id"`
	Time         time.Time `json:"time"`
	Attempt      int       `json:"attempt"`
	Event        string    `json:"event"`
	Reason       string    `json:"reason"`
	HTTPStatus   int       `json:"http_status,omitempty"`
	TicketLength *int      `json:"ticket_length,omitempty"`
	TargetLength int       `json:"target_length"`
}
type OpenAICodexTicketLogs struct {
	Model     string                      `json:"model"`
	Entries   []OpenAICodexTicketLogEntry `json:"entries"`
	Status    *OpenAICodexTicketStatus    `json:"status"`
	Limit     int                         `json:"limit"`
	FetchedAt time.Time                   `json:"fetched_at"`
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
	out := &OpenAICodexTicketLogs{Model: model, Entries: []OpenAICodexTicketLogEntry{}, Limit: OpenAICodexTicketLogLimit, FetchedAt: now.UTC()}
	for _, status := range s.OpenAICodexTicketStatuses(ctx, account, now) {
		if status.Model != model {
			continue
		}
		out.Status = &status
		if status.Probe != nil && !status.Probe.CheckedAt.IsZero() {
			out.Entries = append(out.Entries, OpenAICodexTicketLogEntry{ID: status.Probe.CheckedAt.UnixNano(), Time: status.Probe.CheckedAt, Event: "probe", Reason: status.Probe.Result, HTTPStatus: status.Probe.HTTPStatus, TargetLength: openAICodexTicketTargetLength(account, s.openAICodexTicketConfig())})
		}
		break
	}
	return out, nil
}
