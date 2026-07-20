package service

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"sort"
	"strings"
)

const quotaLeaseDemoReconcileBatchSize = 200

type QuotaLeaseDemoLedgerExportResult struct {
	NodeID      string                     `json:"node_id"`
	Events      []QuotaLeaseDemoUsageEvent `json:"events"`
	NextEventID string                     `json:"next_event_id,omitempty"`
	HasMore     bool                       `json:"has_more"`
}

type QuotaLeaseDemoReconcileResult struct {
	NodeID         string `json:"node_id"`
	FetchedCount   int    `json:"fetched_count"`
	AppliedCount   int    `json:"applied_count"`
	DuplicateCount int    `json:"duplicate_count"`
	FailedCount    int    `json:"failed_count"`
	NextEventID    string `json:"next_event_id,omitempty"`
	HasMore        bool   `json:"has_more"`
	Error          string `json:"error,omitempty"`
}

func (s *QuotaLeaseDemoService) ExportUsageLedgerEvents(ctx context.Context, nodeID, afterEventID string, limit int) (QuotaLeaseDemoLedgerExportResult, error) {
	_ = ctx
	result := QuotaLeaseDemoLedgerExportResult{NodeID: strings.TrimSpace(nodeID)}
	if s == nil || !s.Enabled() {
		return result, ErrQuotaLeaseDemoDisabled
	}
	if result.NodeID == "" {
		result.NodeID = s.activeNodeID()
	}
	if limit <= 0 || limit > quotaLeaseDemoReconcileBatchSize {
		limit = quotaLeaseDemoReconcileBatchSize
	}
	afterEventID = strings.TrimSpace(afterEventID)

	s.mu.Lock()
	events := make([]QuotaLeaseDemoLedgerEvent, 0, len(s.events))
	for _, event := range s.events {
		if event == nil {
			continue
		}
		if strings.TrimSpace(event.NodeID) != result.NodeID {
			continue
		}
		if strings.TrimSpace(event.EventType) != QuotaLeaseDemoEventUsagePosted {
			continue
		}
		events = append(events, *event)
	}
	s.mu.Unlock()

	sort.Slice(events, func(i, j int) bool {
		if events[i].CreatedAt.Equal(events[j].CreatedAt) {
			return events[i].EventID < events[j].EventID
		}
		return events[i].CreatedAt.Before(events[j].CreatedAt)
	})

	passedWatermark := afterEventID == ""
	if afterEventID != "" {
		found := false
		for _, event := range events {
			if strings.TrimSpace(event.EventID) == afterEventID {
				found = true
				break
			}
		}
		if !found {
			passedWatermark = true
		}
	}
	for _, event := range events {
		if !passedWatermark {
			if strings.TrimSpace(event.EventID) == afterEventID {
				passedWatermark = true
			}
			continue
		}
		if len(result.Events) >= limit {
			result.HasMore = true
			break
		}
		result.Events = append(result.Events, QuotaLeaseDemoUsageEvent{
			EventID:   event.EventID,
			LeaseID:   event.LeaseID,
			NodeID:    event.NodeID,
			UserID:    event.UserID,
			APIKeyID:  event.APIKeyID,
			RequestID: event.RequestID,
			TraceID:   event.TraceID,
			Amount:    event.Amount,
			EventType: event.EventType,
			CreatedAt: event.CreatedAt,
		})
		result.NextEventID = event.EventID
	}
	return result, nil
}

func (s *QuotaLeaseDemoService) ReconcileUsageLedgers(ctx context.Context) []QuotaLeaseDemoReconcileResult {
	if s == nil || !s.Enabled() || s.remoteMode() {
		return nil
	}
	nodes := s.reconcileCandidateNodes()
	results := make([]QuotaLeaseDemoReconcileResult, 0, len(nodes))
	for _, node := range nodes {
		results = append(results, s.ReconcileNodeUsageLedger(ctx, node.NodeID))
	}
	return results
}

func (s *QuotaLeaseDemoService) ReconcileNodeUsageLedger(ctx context.Context, nodeID string) QuotaLeaseDemoReconcileResult {
	result := QuotaLeaseDemoReconcileResult{NodeID: strings.TrimSpace(nodeID)}
	if s == nil || !s.Enabled() || s.remoteMode() {
		return result
	}
	if ctx == nil {
		ctx = context.Background()
	}

	node := s.reconcileNodeSnapshot(result.NodeID)
	if node == nil {
		result.Error = ErrQuotaLeaseDemoNodeNotFound.Error()
		return result
	}
	baseURL := strings.TrimSpace(node.BaseURL)
	if baseURL == "" {
		result.Error = "node base_url is empty"
		return result
	}
	secret := strings.TrimSpace(node.Secret)
	if secret == "" {
		result.Error = "node secret is empty"
		return result
	}

	watermark := s.reconcileWatermark(result.NodeID)
	endpoint := "/reconcile/ledger-events?limit=" + url.QueryEscape(fmt.Sprintf("%d", quotaLeaseDemoReconcileBatchSize))
	if watermark != "" {
		endpoint += "&after_event_id=" + url.QueryEscape(watermark)
	}
	fullURL, err := quotaLeaseDemoRemoteEndpointURL(baseURL, endpoint)
	if err != nil {
		result.Error = err.Error()
		return result
	}

	var exported QuotaLeaseDemoLedgerExportResult
	if err := s.doRemoteJSONToURL(ctx, http.MethodGet, fullURL, result.NodeID, secret, nil, &exported); err != nil {
		result.Error = err.Error()
		return result
	}
	result.FetchedCount = len(exported.Events)
	result.NextEventID = strings.TrimSpace(exported.NextEventID)
	result.HasMore = exported.HasMore
	if len(exported.Events) == 0 {
		if result.NextEventID != "" {
			s.setReconcileWatermark(result.NodeID, result.NextEventID)
		}
		return result
	}

	batch := s.postUsageBatchLocal(ctx, QuotaLeaseDemoUsageBatchRequest{
		NodeID: result.NodeID,
		Events: exported.Events,
	})
	for _, row := range batch.Results {
		switch {
		case strings.TrimSpace(row.Error) != "":
			result.FailedCount++
		case row.Duplicate:
			result.DuplicateCount++
		case row.Applied:
			result.AppliedCount++
		}
	}
	if result.FailedCount == 0 && result.NextEventID != "" {
		s.setReconcileWatermark(result.NodeID, result.NextEventID)
	}
	if result.FailedCount > 0 {
		result.Error = fmt.Sprintf("%d ledger events failed", result.FailedCount)
	}
	slog.Info("quota_lease_demo.reconcile_usage_ledger",
		"node_id", result.NodeID,
		"fetched_count", result.FetchedCount,
		"applied_count", result.AppliedCount,
		"duplicate_count", result.DuplicateCount,
		"failed_count", result.FailedCount,
		"next_event_id", result.NextEventID,
		"has_more", result.HasMore,
		"error", result.Error,
	)
	return result
}

func (s *QuotaLeaseDemoService) reconcileCandidateNodes() []QuotaLeaseDemoNode {
	if s == nil {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	nodes := make([]QuotaLeaseDemoNode, 0, len(s.nodes))
	for _, node := range s.nodes {
		if node == nil || strings.TrimSpace(node.BaseURL) == "" || strings.TrimSpace(node.Secret) == "" {
			continue
		}
		if node.Status == QuotaLeaseDemoNodeStatusDisabled {
			continue
		}
		nodes = append(nodes, *cloneQuotaLeaseDemoNode(node))
	}
	sort.Slice(nodes, func(i, j int) bool {
		if nodes[i].UpdatedAt.Equal(nodes[j].UpdatedAt) {
			return nodes[i].NodeID < nodes[j].NodeID
		}
		return nodes[i].UpdatedAt.Before(nodes[j].UpdatedAt)
	})
	return nodes
}

func (s *QuotaLeaseDemoService) reconcileNodeSnapshot(nodeID string) *QuotaLeaseDemoNode {
	if s == nil {
		return nil
	}
	nodeID = strings.TrimSpace(nodeID)
	if nodeID == "" {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return cloneQuotaLeaseDemoNode(s.nodes[nodeID])
}

func (s *QuotaLeaseDemoService) reconcileWatermark(nodeID string) string {
	if s == nil {
		return ""
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return strings.TrimSpace(s.reconcileWatermarks[strings.TrimSpace(nodeID)])
}

func (s *QuotaLeaseDemoService) setReconcileWatermark(nodeID, eventID string) {
	if s == nil {
		return
	}
	nodeID = strings.TrimSpace(nodeID)
	eventID = strings.TrimSpace(eventID)
	if nodeID == "" || eventID == "" {
		return
	}
	s.mu.Lock()
	if s.reconcileWatermarks == nil {
		s.reconcileWatermarks = make(map[string]string)
	}
	s.reconcileWatermarks[nodeID] = eventID
	s.mu.Unlock()
}

func (s *QuotaLeaseDemoService) DrainNodeRuntime(ctx context.Context) error {
	if s == nil || !s.remoteMode() {
		return nil
	}
	if ctx == nil {
		ctx = context.Background()
	}
	var combined error
	if err := s.FlushPendingUsage(ctx); err != nil {
		combined = errors.Join(combined, err)
	}
	if err := s.FlushPendingUsageLogs(ctx); err != nil {
		combined = errors.Join(combined, err)
	}
	if err := s.FlushPendingOpsErrorLogs(ctx); err != nil {
		combined = errors.Join(combined, err)
	}
	if _, err := s.HeartbeatNode(ctx, QuotaLeaseDemoNodeHeartbeatRequest{
		NodeID: s.activeNodeID(),
		Status: QuotaLeaseDemoNodeStatusOffline,
	}); err != nil {
		combined = errors.Join(combined, err)
	}
	return combined
}

func parseQuotaLeaseDemoReconcileLimit(raw string) int {
	if strings.TrimSpace(raw) == "" {
		return quotaLeaseDemoReconcileBatchSize
	}
	var value int
	if _, err := fmt.Sscanf(strings.TrimSpace(raw), "%d", &value); err != nil || value <= 0 {
		return quotaLeaseDemoReconcileBatchSize
	}
	if value > quotaLeaseDemoReconcileBatchSize {
		return quotaLeaseDemoReconcileBatchSize
	}
	return value
}
