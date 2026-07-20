package service

import (
	"context"
	"log/slog"
	"strings"
	"time"
)

type QuotaLeaseDemoPersistedState struct {
	Nodes              []QuotaLeaseDemoNode
	Leases             []QuotaLeaseDemoLease
	Events             []QuotaLeaseDemoLedgerEvent
	PendingUsageEvents []QuotaLeaseDemoUsageEvent
}

type QuotaLeaseDemoPersistenceStore interface {
	LoadQuotaLeaseDemoState(ctx context.Context) (QuotaLeaseDemoPersistedState, error)
	SaveQuotaLeaseDemoNode(ctx context.Context, node QuotaLeaseDemoNode) error
	SaveQuotaLeaseDemoLease(ctx context.Context, lease QuotaLeaseDemoLease) error
	SaveQuotaLeaseDemoLedgerEvent(ctx context.Context, event QuotaLeaseDemoLedgerEvent) error
	SaveQuotaLeaseDemoPendingUsageEvent(ctx context.Context, event QuotaLeaseDemoUsageEvent) error
	DeleteQuotaLeaseDemoPendingUsageEvent(ctx context.Context, eventID string) error
	CleanupQuotaLeaseDemoRecords(ctx context.Context, cutoff time.Time, limit int) (QuotaLeaseDemoCleanupResult, error)
}

type QuotaLeaseDemoUsageBillingCommand struct {
	Billing *UsageBillingCommand
	Lease   QuotaLeaseDemoLease
	Event   QuotaLeaseDemoLedgerEvent
}

type QuotaLeaseDemoUsageBillingRepository interface {
	ApplyQuotaLeaseUsage(ctx context.Context, cmd *QuotaLeaseDemoUsageBillingCommand) (*UsageBillingApplyResult, error)
}

func (s *QuotaLeaseDemoService) SetPersistenceStore(ctx context.Context, store QuotaLeaseDemoPersistenceStore) error {
	if s == nil {
		return nil
	}
	s.cfgMu.Lock()
	s.persistenceStore = store
	s.cfgMu.Unlock()
	if store == nil {
		return nil
	}
	if ctx == nil {
		ctx = context.Background()
	}
	state, err := store.LoadQuotaLeaseDemoState(ctx)
	if err != nil {
		return err
	}
	s.restorePersistedState(state, time.Now().UTC())
	return nil
}

func (s *QuotaLeaseDemoService) quotaLeaseDemoPersistenceStore() QuotaLeaseDemoPersistenceStore {
	if s == nil {
		return nil
	}
	s.cfgMu.RLock()
	defer s.cfgMu.RUnlock()
	return s.persistenceStore
}

func (s *QuotaLeaseDemoService) restorePersistedState(state QuotaLeaseDemoPersistedState, now time.Time) {
	if s == nil {
		return
	}
	if now.IsZero() {
		now = time.Now().UTC()
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.nodes == nil {
		s.nodes = make(map[string]*QuotaLeaseDemoNode)
	}
	for _, node := range state.Nodes {
		node.NodeID = strings.TrimSpace(node.NodeID)
		if node.NodeID == "" {
			continue
		}
		if node.Status == "" {
			node.Status = QuotaLeaseDemoNodeStatusOffline
		}
		if node.RegisteredAt.IsZero() {
			node.RegisteredAt = now
		}
		if node.UpdatedAt.IsZero() {
			node.UpdatedAt = node.RegisteredAt
		}
		copy := node
		s.nodes[node.NodeID] = &copy
	}

	if s.leases == nil {
		s.leases = make(map[string]*QuotaLeaseDemoLease)
	}
	for _, lease := range state.Leases {
		lease.ID = strings.TrimSpace(lease.ID)
		if lease.ID == "" {
			continue
		}
		lease.NodeID = strings.TrimSpace(lease.NodeID)
		if lease.Status == "" {
			lease.Status = QuotaLeaseDemoStatusActive
		}
		if lease.CreatedAt.IsZero() {
			lease.CreatedAt = now
		}
		if lease.UpdatedAt.IsZero() {
			lease.UpdatedAt = lease.CreatedAt
		}
		if lease.ExpiresAt.IsZero() {
			lease.ExpiresAt = quotaLeaseDemoIdleExpiresAt(now)
		}
		if lease.ReclaimAt.IsZero() {
			lease.ReclaimAt = quotaLeaseDemoReclaimAt(lease.ExpiresAt, 3600)
		}
		convergeQuotaLeaseDemoLeaseState(&lease, now)
		copy := lease
		s.leases[lease.ID] = &copy
	}

	if s.events == nil {
		s.events = make(map[string]*QuotaLeaseDemoLedgerEvent)
	}
	for _, event := range state.Events {
		event.EventID = strings.TrimSpace(event.EventID)
		if event.EventID == "" {
			continue
		}
		if event.EventType == "" {
			event.EventType = QuotaLeaseDemoEventUsagePosted
		}
		if event.CreatedAt.IsZero() {
			event.CreatedAt = now
		}
		copy := event
		s.events[event.EventID] = &copy
	}

	if s.pendingEvents == nil {
		s.pendingEvents = make(map[string]QuotaLeaseDemoUsageEvent)
	}
	for _, event := range state.PendingUsageEvents {
		event.EventID = strings.TrimSpace(event.EventID)
		if event.EventID == "" {
			continue
		}
		if event.EventType == "" {
			event.EventType = QuotaLeaseDemoEventUsagePosted
		}
		if event.CreatedAt.IsZero() {
			event.CreatedAt = now
		}
		s.pendingEvents[event.EventID] = event
	}

	s.restoreRemoteNodeAuthLocked()
}

func (s *QuotaLeaseDemoService) restoreRemoteNodeAuthLocked() {
	if s == nil || !s.remoteMode() || len(s.nodes) == 0 {
		return
	}
	s.remoteMu.Lock()
	if s.remoteNodeID != "" && s.remoteNodeSecret != "" {
		s.remoteMu.Unlock()
		return
	}
	s.remoteMu.Unlock()

	configuredNodeID := strings.TrimSpace(s.NodeID())
	var selected *QuotaLeaseDemoNode
	for _, node := range s.nodes {
		if node == nil || strings.TrimSpace(node.Secret) == "" || node.Status == QuotaLeaseDemoNodeStatusDisabled {
			continue
		}
		if configuredNodeID != "" && node.NodeID == configuredNodeID {
			selected = node
			break
		}
		if selected == nil || node.UpdatedAt.After(selected.UpdatedAt) {
			selected = node
		}
	}
	if selected == nil {
		return
	}
	s.remoteMu.Lock()
	s.remoteNodeID = strings.TrimSpace(selected.NodeID)
	s.remoteNodeSecret = strings.TrimSpace(selected.Secret)
	s.remoteMu.Unlock()
}

func (s *QuotaLeaseDemoService) persistQuotaLeaseDemoNode(ctx context.Context, node *QuotaLeaseDemoNode) error {
	store := s.quotaLeaseDemoPersistenceStore()
	if store == nil || node == nil {
		return nil
	}
	return store.SaveQuotaLeaseDemoNode(ctx, *cloneQuotaLeaseDemoNode(node))
}

func (s *QuotaLeaseDemoService) persistQuotaLeaseDemoLease(ctx context.Context, lease *QuotaLeaseDemoLease) error {
	store := s.quotaLeaseDemoPersistenceStore()
	if store == nil || lease == nil {
		return nil
	}
	return store.SaveQuotaLeaseDemoLease(ctx, *cloneQuotaLeaseDemoLease(lease))
}

func (s *QuotaLeaseDemoService) persistQuotaLeaseDemoLedgerEvent(ctx context.Context, event *QuotaLeaseDemoLedgerEvent) error {
	store := s.quotaLeaseDemoPersistenceStore()
	if store == nil || event == nil {
		return nil
	}
	value := *event
	return store.SaveQuotaLeaseDemoLedgerEvent(ctx, value)
}

func (s *QuotaLeaseDemoService) persistQuotaLeaseDemoLeaseAndEvent(ctx context.Context, lease *QuotaLeaseDemoLease, event *QuotaLeaseDemoLedgerEvent) error {
	if err := s.persistQuotaLeaseDemoLease(ctx, lease); err != nil {
		return err
	}
	if err := s.persistQuotaLeaseDemoLedgerEvent(ctx, event); err != nil {
		return err
	}
	return nil
}

func (s *QuotaLeaseDemoService) persistQuotaLeaseDemoPendingUsageEvent(ctx context.Context, event QuotaLeaseDemoUsageEvent) error {
	store := s.quotaLeaseDemoPersistenceStore()
	if store == nil {
		return nil
	}
	return store.SaveQuotaLeaseDemoPendingUsageEvent(ctx, event)
}

func (s *QuotaLeaseDemoService) deleteQuotaLeaseDemoPendingUsageEvent(ctx context.Context, eventID string) error {
	store := s.quotaLeaseDemoPersistenceStore()
	if store == nil {
		return nil
	}
	return store.DeleteQuotaLeaseDemoPendingUsageEvent(ctx, strings.TrimSpace(eventID))
}

func (s *QuotaLeaseDemoService) persistQuotaLeaseDemoLeaseBestEffort(lease *QuotaLeaseDemoLease, reason string) {
	if s == nil || lease == nil || s.quotaLeaseDemoPersistenceStore() == nil {
		return
	}
	if err := s.persistQuotaLeaseDemoLease(context.Background(), lease); err != nil {
		slog.Warn("quota_lease_demo.persist_lease_failed",
			"lease_id", lease.ID,
			"reason", strings.TrimSpace(reason),
			"error", err,
		)
	}
}

func (s *QuotaLeaseDemoService) CleanupRetainedRecords(ctx context.Context, now time.Time) (QuotaLeaseDemoCleanupResult, error) {
	result := QuotaLeaseDemoCleanupResult{}
	if s == nil || !s.Enabled() {
		return result, nil
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if now.IsZero() {
		now = time.Now().UTC()
	}
	cutoff := now.UTC().Add(-quotaLeaseDemoRecordRetention)
	if store := s.quotaLeaseDemoPersistenceStore(); store != nil {
		persisted, err := store.CleanupQuotaLeaseDemoRecords(ctx, cutoff, quotaLeaseDemoCleanupBatchSize)
		if err != nil {
			return result, err
		}
		result = persisted
	}
	memoryResult := s.cleanupQuotaLeaseDemoMemoryRecords(cutoff, quotaLeaseDemoCleanupBatchSize)
	if result.LeaseCount == 0 && result.LedgerEventCount == 0 {
		result = memoryResult
	}
	return result, nil
}

func (s *QuotaLeaseDemoService) cleanupQuotaLeaseDemoMemoryRecords(cutoff time.Time, limit int) QuotaLeaseDemoCleanupResult {
	result := QuotaLeaseDemoCleanupResult{}
	if s == nil {
		return result
	}
	if limit <= 0 {
		limit = quotaLeaseDemoCleanupBatchSize
	}
	deletedLeaseIDs := make(map[string]struct{})
	s.mu.Lock()
	for id, lease := range s.leases {
		if result.LeaseCount >= int64(limit) {
			break
		}
		if !quotaLeaseDemoLeaseCleanupEligible(lease, cutoff) {
			continue
		}
		deletedLeaseIDs[id] = struct{}{}
		delete(s.leases, id)
		result.LeaseCount++
	}
	for id, event := range s.events {
		if _, ok := deletedLeaseIDs[strings.TrimSpace(event.LeaseID)]; ok {
			delete(s.events, id)
			result.LedgerEventCount++
		}
	}
	s.mu.Unlock()
	return result
}

func quotaLeaseDemoLeaseCleanupEligible(lease *QuotaLeaseDemoLease, cutoff time.Time) bool {
	if lease == nil || cutoff.IsZero() {
		return false
	}
	status := strings.TrimSpace(lease.Status)
	if status != QuotaLeaseDemoStatusClosed && status != QuotaLeaseDemoStatusReclaimed {
		return false
	}
	updatedAt := lease.UpdatedAt
	if updatedAt.IsZero() {
		updatedAt = lease.CreatedAt
	}
	return !updatedAt.IsZero() && updatedAt.Before(cutoff)
}

func convergeQuotaLeaseDemoLeaseState(lease *QuotaLeaseDemoLease, now time.Time) bool {
	if lease == nil {
		return false
	}
	if now.IsZero() {
		now = time.Now().UTC()
	}
	beforeStatus := lease.Status
	beforeUpdatedAt := lease.UpdatedAt
	if lease.Status == "" {
		lease.Status = QuotaLeaseDemoStatusActive
	}
	remaining := lease.Remaining()
	if lease.Status == QuotaLeaseDemoStatusActive && remaining < -1e-12 {
		return lease.Status != beforeStatus || !lease.UpdatedAt.Equal(beforeUpdatedAt)
	}
	if lease.Status == QuotaLeaseDemoStatusActive && remaining <= 1e-12 && remaining >= -1e-12 {
		lease.Status = QuotaLeaseDemoStatusClosed
		lease.UpdatedAt = now
	}
	if lease.Status == QuotaLeaseDemoStatusActive && now.After(lease.ExpiresAt) {
		lease.Status = QuotaLeaseDemoStatusExpired
		lease.UpdatedAt = now
	}
	return lease.Status != beforeStatus || !lease.UpdatedAt.Equal(beforeUpdatedAt)
}
