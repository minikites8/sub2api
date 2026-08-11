package service

import (
	"context"
	"sort"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/google/uuid"
)

type QuotaLeaseDemoModelAvailabilityEvent struct {
	EventID   string    `json:"event_id"`
	NodeID    string    `json:"node_id,omitempty"`
	RequestID string    `json:"request_id,omitempty"`
	Model     string    `json:"model"`
	Success   bool      `json:"success"`
	CheckedAt time.Time `json:"checked_at"`
}

type QuotaLeaseDemoModelAvailabilityBatchRequest struct {
	NodeID string                                 `json:"node_id"`
	Events []QuotaLeaseDemoModelAvailabilityEvent `json:"events"`
}

type QuotaLeaseDemoModelAvailabilityResult struct {
	EventID   string `json:"event_id"`
	Applied   bool   `json:"applied"`
	Duplicate bool   `json:"duplicate"`
	Error     string `json:"error,omitempty"`
}

type QuotaLeaseDemoModelAvailabilityBatchResult struct {
	Results []QuotaLeaseDemoModelAvailabilityResult `json:"results"`
}

func NewQuotaLeaseDemoModelAvailabilityEvent(nodeID, requestID, model string, success bool, checkedAt time.Time) QuotaLeaseDemoModelAvailabilityEvent {
	if checkedAt.IsZero() {
		checkedAt = time.Now().UTC()
	}
	return QuotaLeaseDemoModelAvailabilityEvent{
		EventID:   "availability:" + uuid.NewString(),
		NodeID:    strings.TrimSpace(nodeID),
		RequestID: strings.TrimSpace(requestID),
		Model:     strings.TrimSpace(model),
		Success:   success,
		CheckedAt: checkedAt.UTC(),
	}
}

// EnqueueQuotaLeaseDemoModelAvailability forwards one completed gateway call
// from a node to the control plane through the existing node sync transport.
func EnqueueQuotaLeaseDemoModelAvailability(ctx context.Context, cfg *config.Config, model string, success bool, checkedAt time.Time) {
	if cfg == nil || !cfg.IsNodeRole() || !QuotaLeaseDemoEnabled(cfg) || strings.TrimSpace(model) == "" {
		return
	}
	svc := GetQuotaLeaseDemoService(cfg)
	if svc == nil || !svc.remoteMode() {
		return
	}
	event := NewQuotaLeaseDemoModelAvailabilityEvent(
		svc.activeNodeID(),
		quotaLeaseDemoContextRequestID(ctx),
		model,
		success,
		checkedAt,
	)
	svc.enqueuePendingModelAvailabilityEvent(event)
	svc.flushPendingModelAvailabilityAsync()
}

func (s *QuotaLeaseDemoService) enqueuePendingModelAvailabilityEvent(event QuotaLeaseDemoModelAvailabilityEvent) {
	if s == nil {
		return
	}
	event.EventID = strings.TrimSpace(event.EventID)
	event.NodeID = strings.TrimSpace(event.NodeID)
	event.RequestID = strings.TrimSpace(event.RequestID)
	event.Model = strings.TrimSpace(event.Model)
	if event.EventID == "" || event.Model == "" {
		return
	}
	if event.NodeID == "" {
		event.NodeID = s.activeNodeID()
	}
	if event.CheckedAt.IsZero() {
		event.CheckedAt = time.Now().UTC()
	} else {
		event.CheckedAt = event.CheckedAt.UTC()
	}
	s.mu.Lock()
	if s.pendingModelAvailability == nil {
		s.pendingModelAvailability = make(map[string]QuotaLeaseDemoModelAvailabilityEvent)
	}
	s.pendingModelAvailability[event.EventID] = event
	s.mu.Unlock()
}

func (s *QuotaLeaseDemoService) pendingModelAvailabilityEvents() []QuotaLeaseDemoModelAvailabilityEvent {
	if s == nil {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	events := make([]QuotaLeaseDemoModelAvailabilityEvent, 0, len(s.pendingModelAvailability))
	for _, event := range s.pendingModelAvailability {
		events = append(events, event)
	}
	sort.Slice(events, func(i, j int) bool {
		return events[i].CheckedAt.Before(events[j].CheckedAt)
	})
	return events
}

func (s *QuotaLeaseDemoService) removePendingModelAvailabilityResults(result QuotaLeaseDemoModelAvailabilityBatchResult) {
	if s == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, item := range result.Results {
		if strings.TrimSpace(item.Error) == "" && (item.Applied || item.Duplicate) {
			delete(s.pendingModelAvailability, strings.TrimSpace(item.EventID))
		}
	}
}
