package service

import (
	"container/list"
	"context"
	"errors"
	"net"
	"strings"
	"sync"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
)

const OpenAICodexTicketLogLimit = 200
const codexTicketTelemetryMaxStreams = 512

var ErrOpenAICodexTicketLogModel = errors.New("model is not configured for Codex tickets")

// These summaries deliberately exclude bearer tokens, opaque tickets and proxy credentials.
type OpenAICodexTicketStatus struct {
	Model            string     `json:"model"`
	State            string     `json:"state"`
	Ready            bool       `json:"ready"`
	RemainingSeconds int64      `json:"remaining_seconds"`
	ExpiresAt        *time.Time `json:"expires_at,omitempty"`
	Length           int        `json:"length,omitempty"`
	TargetLength     int        `json:"target_length"`
	Attempts         int        `json:"attempts"`
	Harvesting       bool       `json:"harvesting"`
	HarvestEnabled   bool       `json:"harvest_enabled"`
	NextHarvestAt    *time.Time `json:"next_harvest_at,omitempty"`
}
type OpenAICodexTicketLogEntry struct {
	ID                int64                         `json:"id"`
	Time              time.Time                     `json:"time"`
	Attempt           int                           `json:"attempt"`
	Event             string                        `json:"event"`
	Reason            string                        `json:"reason"`
	HTTPStatus        int                           `json:"http_status,omitempty"`
	TicketLength      *int                          `json:"ticket_length,omitempty"`
	TargetLength      int                           `json:"target_length"`
	DurationMS        *int64                        `json:"duration_ms,omitempty"`
	EgressIP          string                        `json:"egress_ip,omitempty"`
	EgressCountryCode string                        `json:"egress_country_code,omitempty"`
	EgressError       *OpenAICodexTicketEgressError `json:"egress_error,omitempty"`
}
type OpenAICodexTicketLogs struct {
	Model     string                      `json:"model"`
	Entries   []OpenAICodexTicketLogEntry `json:"entries"`
	Status    *OpenAICodexTicketStatus    `json:"status"`
	Limit     int                         `json:"limit"`
	FetchedAt time.Time                   `json:"fetched_at"`
}
type codexTicketTelemetryStream struct {
	key        string
	credential [32]byte
	entries    []OpenAICodexTicketLogEntry
	attempts   int
	harvesting bool
	completed  bool
}
type codexTicketTelemetryStore struct {
	mu      sync.Mutex
	streams map[string]*list.Element
	lru     list.List
	nextID  int64
}

func cloneCodexTicketLog(entry OpenAICodexTicketLogEntry) OpenAICodexTicketLogEntry {
	if entry.TicketLength != nil {
		v := *entry.TicketLength
		entry.TicketLength = &v
	}
	if entry.DurationMS != nil {
		v := *entry.DurationMS
		entry.DurationMS = &v
	}
	if entry.EgressError != nil {
		v := *entry.EgressError
		entry.EgressError = &v
	}
	return entry
}
func (s *codexTicketTelemetryStore) streamLocked(key string) *codexTicketTelemetryStream {
	if s.streams == nil {
		s.streams = make(map[string]*list.Element)
	}
	if e := s.streams[key]; e != nil {
		s.lru.MoveToFront(e)
		return e.Value.(*codexTicketTelemetryStream)
	}
	if len(s.streams) >= codexTicketTelemetryMaxStreams {
		oldest := s.lru.Back()
		delete(s.streams, oldest.Value.(*codexTicketTelemetryStream).key)
		s.lru.Remove(oldest)
	}
	stream := &codexTicketTelemetryStream{key: key}
	s.streams[key] = s.lru.PushFront(stream)
	return stream
}
func (s *codexTicketTelemetryStore) appendLocked(stream *codexTicketTelemetryStream, entry OpenAICodexTicketLogEntry) int64 {
	s.nextID++
	entry.ID = s.nextID
	entry.Time = time.Now().UTC()
	if len(stream.entries) == OpenAICodexTicketLogLimit {
		copy(stream.entries, stream.entries[1:])
		stream.entries = stream.entries[:OpenAICodexTicketLogLimit-1]
	}
	stream.entries = append(stream.entries, cloneCodexTicketLog(entry))
	return entry.ID
}
func (s *codexTicketTelemetryStore) begin(account *Account, model string) (int, int64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	stream := s.streamLocked(codexTicketKey(account.ID, model))
	credential := codexTicketCredential(account)
	if stream.completed || stream.credential != credential {
		stream.attempts = 0
		stream.completed = false
	}
	stream.credential = credential
	stream.attempts++
	stream.harvesting = true
	id := s.appendLocked(stream, OpenAICodexTicketLogEntry{Attempt: stream.attempts, Event: "started", Reason: "request_started", TargetLength: codexTicketLength(account)})
	return stream.attempts, id
}
func (s *codexTicketTelemetryStore) finish(accountID int64, model string, entry OpenAICodexTicketLogEntry) {
	s.mu.Lock()
	defer s.mu.Unlock()
	stream := s.streamLocked(codexTicketKey(accountID, model))
	stream.harvesting = false
	stream.completed = entry.Event == "acquired"
	s.appendLocked(stream, entry)
}
func (s *codexTicketTelemetryStore) setEgress(accountID int64, model string, id int64, result OpenAICodexTicketEgressResult) {
	s.mu.Lock()
	defer s.mu.Unlock()
	e := s.streams[codexTicketKey(accountID, model)]
	if e == nil {
		return
	}
	stream := e.Value.(*codexTicketTelemetryStream)
	for i := range stream.entries {
		if stream.entries[i].ID == id {
			stream.entries[i].EgressIP = result.IP
			stream.entries[i].EgressCountryCode = result.CountryCode
			stream.entries[i].EgressError = result.Error
			return
		}
	}
}
func (s *codexTicketTelemetryStore) snapshot(accountID int64, model string) ([]OpenAICodexTicketLogEntry, int, bool, [32]byte) {
	s.mu.Lock()
	defer s.mu.Unlock()
	entries := []OpenAICodexTicketLogEntry{}
	e := s.streams[codexTicketKey(accountID, model)]
	if e == nil {
		return entries, 0, false, [32]byte{}
	}
	stream := e.Value.(*codexTicketTelemetryStream)
	for _, entry := range stream.entries {
		entries = append(entries, cloneCodexTicketLog(entry))
	}
	return entries, stream.attempts, stream.harvesting, stream.credential
}
func codexTicketProbeErrorReason(err error) string {
	if errors.Is(err, context.Canceled) {
		return "canceled"
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return "timeout"
	}
	var ne net.Error
	if errors.As(err, &ne) && ne.Timeout() {
		return "timeout"
	}
	if err != nil {
		return "network_error"
	}
	return "empty_response"
}
func (s *OpenAIGatewayService) codexTicketStatus(account *Account, model string, cfg config.OpenAICodexTicketConfig, now time.Time) OpenAICodexTicketStatus {
	status := OpenAICodexTicketStatus{Model: model, State: "waiting", TargetLength: codexTicketLength(account)}
	status.HarvestEnabled = cfg.Enabled && cfg.HarvestProxyURL != "" && ValidateOpenAICodexTicketHarvestProxyURL(cfg.HarvestProxyURL) == nil
	attempts, harvesting, credential := s.codexTicketTelemetry.progress(account.ID, model)
	if credential == codexTicketCredential(account) {
		status.Attempts = attempts
		status.Harvesting = harvesting
	}
	if ticket := s.cachedCodexTicket(account, model); ticket.valid(account, now) {
		status.Ready = true
		status.State = "ready"
		status.ExpiresAt = &ticket.expires
		status.Length = len(ticket.state)
		status.RemainingSeconds = int64(ticket.expires.Sub(now) / time.Second)
		if status.Attempts == 0 {
			status.Attempts = ticket.attempts
		}
	}
	next := time.Time{}
	if deadline := s.codexTicketNextHarvest.Load(); deadline > 0 {
		next = time.Unix(0, deadline)
	}
	if raw, ok := s.codexTicketMissBackoffs.Load(codexTicketKey(account.ID, model)); ok {
		if b := raw.(codexTicketBackoff); b.until.After(next) {
			next = b.until
		}
	}
	tokenInvalid := false
	if raw, ok := s.codexTicketBackoffs.Load(account.ID); ok {
		b := raw.(codexTicketBackoff)
		tokenInvalid = b.unauthorized && b.credential == codexTicketCredential(account)
		if !b.unauthorized && b.until.After(next) {
			next = b.until
		}
	}
	switch {
	case !cfg.Enabled:
		status.State = "disabled"
	case tokenInvalid:
		status.State = "token_invalid"
	case !status.HarvestEnabled:
		status.State = "proxy_missing"
	case account.IsRateLimited():
		status.State = "cooldown"
		if account.RateLimitResetAt != nil && account.RateLimitResetAt.After(next) {
			next = *account.RateLimitResetAt
		}
	case !account.IsSchedulable():
		status.State = "paused"
	case status.Harvesting:
		if !status.Ready {
			status.State = "harvesting"
		}
	case status.Ready:
	default:
		if next.After(now) {
			status.State = "cooldown"
		}
	}
	if paused, _ := shouldAutoPauseOpenAIAccountByQuota(context.Background(), account); cfg.Enabled && paused && !status.Harvesting {
		status.State = "paused"
	}
	if status.State == "cooldown" && next.After(now) {
		status.NextHarvestAt = &next
	}
	return status
}
func (s *OpenAIGatewayService) OpenAICodexTicketStatuses(ctx context.Context, account *Account, now time.Time) []OpenAICodexTicketStatus {
	if s == nil || !codexTicketAccount(account) {
		return nil
	}
	cfg := s.codexTicketConfig(ctx)
	if !cfg.Enabled {
		return nil
	}
	statuses := make([]OpenAICodexTicketStatus, 0, len(cfg.Models))
	for _, model := range cfg.Models {
		statuses = append(statuses, s.codexTicketStatus(account, model, cfg, now))
	}
	return statuses
}
func (s *OpenAIGatewayService) OpenAICodexTicketStatusBatch(ctx context.Context, ids []int64) (map[int64][]OpenAICodexTicketStatus, error) {
	result := make(map[int64][]OpenAICodexTicketStatus, len(ids))
	for _, id := range ids {
		result[id] = []OpenAICodexTicketStatus{}
	}
	if s == nil || s.accountRepo == nil {
		return result, nil
	}
	cfg := s.codexTicketConfig(ctx)
	if !cfg.Enabled {
		return result, nil
	}
	accounts, err := s.accountRepo.GetByIDs(ctx, ids)
	if err != nil {
		return nil, err
	}
	now := time.Now()
	for _, account := range accounts {
		if codexTicketAccount(account) {
			for _, model := range cfg.Models {
				result[account.ID] = append(result[account.ID], s.codexTicketStatus(account, model, cfg, now))
			}
		}
	}
	return result, nil
}
func (s *OpenAIGatewayService) OpenAICodexTicketLogs(ctx context.Context, account *Account, model string, now time.Time) (*OpenAICodexTicketLogs, error) {
	model = strings.TrimSpace(model)
	cfg := s.codexTicketConfig(ctx)
	configured := false
	for _, item := range cfg.Models {
		if item == model {
			configured = true
			break
		}
	}
	if !configured {
		return nil, ErrOpenAICodexTicketLogModel
	}
	out := &OpenAICodexTicketLogs{Model: model, Entries: []OpenAICodexTicketLogEntry{}, Limit: OpenAICodexTicketLogLimit, FetchedAt: now.UTC()}
	if s == nil || !codexTicketAccount(account) {
		return out, nil
	}
	out.Entries, _, _, _ = s.codexTicketTelemetry.snapshot(account.ID, model)
	status := s.codexTicketStatus(account, model, cfg, now)
	out.Status = &status
	return out, nil
}

func (s *codexTicketTelemetryStore) progress(accountID int64, model string) (int, bool, [32]byte) {
	s.mu.Lock()
	defer s.mu.Unlock()
	e := s.streams[codexTicketKey(accountID, model)]
	if e == nil {
		return 0, false, [32]byte{}
	}
	stream := e.Value.(*codexTicketTelemetryStream)
	return stream.attempts, stream.harvesting, stream.credential
}
