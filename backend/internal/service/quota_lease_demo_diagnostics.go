package service

import (
	"context"
	"fmt"
	"math"
	"sort"
	"strings"
	"time"
)

const (
	QuotaLeaseDemoDiagnosticHealthOK       = "ok"
	QuotaLeaseDemoDiagnosticHealthWarning  = "warning"
	QuotaLeaseDemoDiagnosticHealthCritical = "critical"

	quotaLeaseDemoDiagnosticHeartbeatWarningAge = 90 * time.Second
	quotaLeaseDemoDiagnosticHeartbeatStaleAge   = 3 * time.Minute
	quotaLeaseDemoDiagnosticSyncWarningAge      = 90 * time.Second
	quotaLeaseDemoDiagnosticSyncStaleAge        = 5 * time.Minute
	quotaLeaseDemoDiagnosticPendingCritical     = 100
)

type QuotaLeaseDemoDiagnosticUserResolver func(context.Context, int64) (QuotaLeaseDemoDiagnosticUserProfile, error)

type QuotaLeaseDemoDiagnosticUserProfile struct {
	UserID        int64    `json:"user_id"`
	Username      string   `json:"username,omitempty"`
	Email         string   `json:"email,omitempty"`
	Status        string   `json:"status,omitempty"`
	Balance       *float64 `json:"balance,omitempty"`
	FrozenBalance *float64 `json:"frozen_balance,omitempty"`
	Found         bool     `json:"found"`
	Error         string   `json:"error,omitempty"`
}

type QuotaLeaseDemoDiagnostics struct {
	GeneratedAt            time.Time                       `json:"generated_at"`
	Enabled                bool                            `json:"enabled"`
	NodeID                 string                          `json:"node_id"`
	Health                 string                          `json:"health"`
	DefaultGrantAmount     float64                         `json:"default_grant_amount"`
	PreflightReserveAmount float64                         `json:"preflight_reserve_amount"`
	Stats                  QuotaLeaseDemoDiagnosticStats   `json:"stats"`
	Issues                 []QuotaLeaseDemoDiagnosticIssue `json:"issues"`
	Nodes                  []QuotaLeaseDemoNodeDiagnostic  `json:"nodes"`
	Users                  []QuotaLeaseDemoUserDiagnostic  `json:"users"`
	Leases                 []QuotaLeaseDemoLeaseDiagnostic `json:"leases"`
}

type QuotaLeaseDemoDiagnosticStats struct {
	NodeCount           int     `json:"node_count"`
	OnlineNodes         int     `json:"online_nodes"`
	UserCount           int     `json:"user_count"`
	LeaseCount          int     `json:"lease_count"`
	ActiveLeases        int     `json:"active_leases"`
	ExpiredLeases       int     `json:"expired_leases"`
	ClosedLeases        int     `json:"closed_leases"`
	ReclaimedLeases     int     `json:"reclaimed_leases"`
	OverdraftLeases     int     `json:"overdraft_leases"`
	LowCapacityLeases   int     `json:"low_capacity_leases"`
	GrantedTotal        float64 `json:"granted_total"`
	ConsumedTotal       float64 `json:"consumed_total"`
	ReclaimedTotal      float64 `json:"reclaimed_total"`
	RemainingTotal      float64 `json:"remaining_total"`
	OverdraftTotal      float64 `json:"overdraft_total"`
	EventCount          int     `json:"event_count"`
	PendingUsageEvents  int     `json:"pending_usage_events"`
	PendingUsageLogs    int     `json:"pending_usage_logs"`
	PendingOpsErrorLogs int     `json:"pending_ops_error_logs"`
	IssueCount          int     `json:"issue_count"`
	WarningCount        int     `json:"warning_count"`
	CriticalCount       int     `json:"critical_count"`
}

type QuotaLeaseDemoDiagnosticIssue struct {
	ID        string     `json:"id"`
	Level     string     `json:"level"`
	Scope     string     `json:"scope"`
	Code      string     `json:"code"`
	Message   string     `json:"message"`
	Detail    string     `json:"detail,omitempty"`
	NodeID    string     `json:"node_id,omitempty"`
	UserID    int64      `json:"user_id,omitempty"`
	APIKeyID  int64      `json:"api_key_id,omitempty"`
	LeaseID   string     `json:"lease_id,omitempty"`
	CreatedAt *time.Time `json:"created_at,omitempty"`
}

type QuotaLeaseDemoNodeDiagnostic struct {
	NodeID              string                        `json:"node_id"`
	Region              string                        `json:"region,omitempty"`
	BaseURL             string                        `json:"base_url,omitempty"`
	Status              string                        `json:"status"`
	Health              string                        `json:"health"`
	Issues              []string                      `json:"issues,omitempty"`
	LastHeartbeatAt     *time.Time                    `json:"last_heartbeat_at,omitempty"`
	HeartbeatAgeSeconds *int64                        `json:"heartbeat_age_seconds,omitempty"`
	ActiveLeaseCount    int                           `json:"active_lease_count"`
	ActiveRemaining     float64                       `json:"active_remaining"`
	OverdraftAmount     float64                       `json:"overdraft_amount"`
	LeaseCount          int                           `json:"lease_count"`
	SyncStatus          *QuotaLeaseDemoNodeSyncStatus `json:"sync_status,omitempty"`
	PendingUsageEvents  int                           `json:"pending_usage_events"`
	PendingUsageLogs    int                           `json:"pending_usage_logs"`
	PendingOpsErrorLogs int                           `json:"pending_ops_error_logs"`
}

type QuotaLeaseDemoUserDiagnostic struct {
	UserID           int64      `json:"user_id"`
	Username         string     `json:"username,omitempty"`
	Email            string     `json:"email,omitempty"`
	Status           string     `json:"status,omitempty"`
	Balance          *float64   `json:"balance,omitempty"`
	FrozenBalance    *float64   `json:"frozen_balance,omitempty"`
	ProfileError     string     `json:"profile_error,omitempty"`
	Health           string     `json:"health"`
	Issues           []string   `json:"issues,omitempty"`
	APIKeyIDs        []int64    `json:"api_key_ids,omitempty"`
	LeaseCount       int        `json:"lease_count"`
	ActiveLeaseCount int        `json:"active_lease_count"`
	ActiveRemaining  float64    `json:"active_remaining"`
	OverdraftAmount  float64    `json:"overdraft_amount"`
	GrantedTotal     float64    `json:"granted_total"`
	ConsumedTotal    float64    `json:"consumed_total"`
	ReclaimedTotal   float64    `json:"reclaimed_total"`
	LastLeaseAt      *time.Time `json:"last_lease_at,omitempty"`
	LastEventAt      *time.Time `json:"last_event_at,omitempty"`
}

type QuotaLeaseDemoLeaseDiagnostic struct {
	ID               string     `json:"id"`
	NodeID           string     `json:"node_id"`
	UserID           int64      `json:"user_id"`
	APIKeyID         int64      `json:"api_key_id"`
	Status           string     `json:"status"`
	Health           string     `json:"health"`
	Issues           []string   `json:"issues,omitempty"`
	Granted          float64    `json:"granted"`
	Consumed         float64    `json:"consumed"`
	Reclaimed        float64    `json:"reclaimed"`
	Remaining        float64    `json:"remaining"`
	EventCount       int        `json:"event_count"`
	UsageEventTotal  float64    `json:"usage_event_total"`
	LastEventAt      *time.Time `json:"last_event_at,omitempty"`
	ExpiresAt        time.Time  `json:"expires_at"`
	ReclaimAt        time.Time  `json:"reclaim_at"`
	CreatedAt        time.Time  `json:"created_at"`
	UpdatedAt        time.Time  `json:"updated_at"`
	ExpiresInSeconds int64      `json:"expires_in_seconds"`
	ReclaimInSeconds int64      `json:"reclaim_in_seconds"`
}

type quotaLeaseDemoDiagnosticEventStats struct {
	count      int
	usageTotal float64
	lastAt     *time.Time
}

func (s *QuotaLeaseDemoService) Diagnostics(ctx context.Context, resolveUser QuotaLeaseDemoDiagnosticUserResolver) QuotaLeaseDemoDiagnostics {
	if ctx == nil {
		ctx = context.Background()
	}
	snapshot := s.Snapshot()
	profiles := map[int64]QuotaLeaseDemoDiagnosticUserProfile{}
	if resolveUser != nil {
		for _, userID := range quotaLeaseDemoDiagnosticUserIDs(snapshot) {
			profile, err := resolveUser(ctx, userID)
			if profile.UserID <= 0 {
				profile.UserID = userID
			}
			if err != nil {
				profile.Found = false
				profile.Error = quotaLeaseDemoTrimDiagnosticText(err.Error(), 300)
			}
			profiles[userID] = profile
		}
	}
	defaultGrant := 0.0
	preflightReserve := 0.0
	if s != nil {
		defaultGrant = s.DefaultGrantAmount()
		preflightReserve = s.PreflightReserveAmount()
	}
	return buildQuotaLeaseDemoDiagnostics(snapshot, profiles, time.Now().UTC(), defaultGrant, preflightReserve)
}

func buildQuotaLeaseDemoDiagnostics(
	snapshot QuotaLeaseDemoSnapshot,
	profiles map[int64]QuotaLeaseDemoDiagnosticUserProfile,
	now time.Time,
	defaultGrant float64,
	preflightReserve float64,
) QuotaLeaseDemoDiagnostics {
	if now.IsZero() {
		now = time.Now().UTC()
	}
	diag := QuotaLeaseDemoDiagnostics{
		GeneratedAt:            now,
		Enabled:                snapshot.Enabled,
		NodeID:                 snapshot.NodeID,
		Health:                 QuotaLeaseDemoDiagnosticHealthOK,
		DefaultGrantAmount:     defaultGrant,
		PreflightReserveAmount: preflightReserve,
		Stats: QuotaLeaseDemoDiagnosticStats{
			NodeCount:       snapshot.Stats.NodeCount,
			OnlineNodes:     snapshot.Stats.OnlineNodes,
			LeaseCount:      len(snapshot.Leases),
			ActiveLeases:    snapshot.Stats.ActiveLeases,
			ExpiredLeases:   snapshot.Stats.ExpiredLeases,
			ClosedLeases:    snapshot.Stats.ClosedLeases,
			ReclaimedLeases: snapshot.Stats.ReclaimedLeases,
			GrantedTotal:    snapshot.Stats.GrantedTotal,
			ConsumedTotal:   snapshot.Stats.ConsumedTotal,
			ReclaimedTotal:  snapshot.Stats.ReclaimedTotal,
			RemainingTotal:  snapshot.Stats.RemainingTotal,
			EventCount:      snapshot.Stats.EventCount,
		},
	}
	eventStats := quotaLeaseDemoDiagnosticEventsByLease(snapshot.Events)
	if !snapshot.Enabled {
		quotaLeaseDemoAddDiagnosticIssue(&diag, nil, nil, nil, QuotaLeaseDemoDiagnosticHealthCritical, "system", "quota_lease_disabled", "节点租约功能已关闭", "", "", 0, 0, "", nil)
	} else if len(snapshot.Nodes) == 0 {
		quotaLeaseDemoAddDiagnosticIssue(&diag, nil, nil, nil, QuotaLeaseDemoDiagnosticHealthWarning, "system", "node_empty", "控制面还没有节点", "", "", 0, 0, "", nil)
	} else if snapshot.Stats.OnlineNodes == 0 {
		quotaLeaseDemoAddDiagnosticIssue(&diag, nil, nil, nil, QuotaLeaseDemoDiagnosticHealthCritical, "system", "node_online_empty", "当前没有在线节点", "", "", 0, 0, "", nil)
	}
	nodeDiagnostics := quotaLeaseDemoDiagnosticNodes(snapshot.Nodes, snapshot.Leases, now, &diag)
	userDiagnostics := quotaLeaseDemoDiagnosticUsers(snapshot.Leases, snapshot.Events, profiles, &diag)
	leaseDiagnostics := quotaLeaseDemoDiagnosticLeases(snapshot.Leases, eventStats, now, preflightReserve, &diag)

	diag.Nodes = nodeDiagnostics
	diag.Users = userDiagnostics
	diag.Leases = leaseDiagnostics
	diag.Stats.UserCount = len(userDiagnostics)
	sort.Slice(diag.Issues, func(i, j int) bool {
		if diag.Issues[i].Level == diag.Issues[j].Level {
			return diag.Issues[i].ID < diag.Issues[j].ID
		}
		return quotaLeaseDemoDiagnosticSeverity(diag.Issues[i].Level) > quotaLeaseDemoDiagnosticSeverity(diag.Issues[j].Level)
	})
	diag.Health = quotaLeaseDemoDiagnosticOverallHealth(diag.Stats.CriticalCount, diag.Stats.WarningCount)
	return diag
}

func quotaLeaseDemoDiagnosticUserIDs(snapshot QuotaLeaseDemoSnapshot) []int64 {
	seen := map[int64]struct{}{}
	for _, lease := range snapshot.Leases {
		if lease.UserID > 0 {
			seen[lease.UserID] = struct{}{}
		}
	}
	for _, event := range snapshot.Events {
		if event.UserID > 0 {
			seen[event.UserID] = struct{}{}
		}
	}
	ids := make([]int64, 0, len(seen))
	for id := range seen {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	return ids
}

func quotaLeaseDemoDiagnosticEventsByLease(events []QuotaLeaseDemoLedgerEvent) map[string]quotaLeaseDemoDiagnosticEventStats {
	out := make(map[string]quotaLeaseDemoDiagnosticEventStats)
	for _, event := range events {
		leaseID := strings.TrimSpace(event.LeaseID)
		if leaseID == "" {
			continue
		}
		stats := out[leaseID]
		stats.count++
		if event.EventType == QuotaLeaseDemoEventUsagePosted {
			stats.usageTotal += event.Amount
		}
		eventAt := event.CreatedAt
		if stats.lastAt == nil || eventAt.After(*stats.lastAt) {
			stats.lastAt = &eventAt
		}
		out[leaseID] = stats
	}
	return out
}

func quotaLeaseDemoDiagnosticNodes(
	nodes []QuotaLeaseDemoNode,
	leases []QuotaLeaseDemoLease,
	now time.Time,
	diag *QuotaLeaseDemoDiagnostics,
) []QuotaLeaseDemoNodeDiagnostic {
	byNode := make(map[string]*QuotaLeaseDemoNodeDiagnostic, len(nodes))
	for _, node := range nodes {
		item := QuotaLeaseDemoNodeDiagnostic{
			NodeID:          node.NodeID,
			Region:          node.Region,
			BaseURL:         node.BaseURL,
			Status:          strings.TrimSpace(node.Status),
			Health:          QuotaLeaseDemoDiagnosticHealthOK,
			LastHeartbeatAt: cloneQuotaLeaseDemoTimePtr(node.LastHeartbeatAt),
			SyncStatus:      cloneQuotaLeaseDemoNodeSyncStatus(node.SyncStatus),
		}
		if item.Status == "" {
			item.Status = QuotaLeaseDemoNodeStatusOffline
		}
		if node.LastHeartbeatAt != nil {
			age := int64(now.Sub(node.LastHeartbeatAt.UTC()).Seconds())
			if age < 0 {
				age = 0
			}
			item.HeartbeatAgeSeconds = &age
		}
		if node.SyncStatus != nil {
			item.PendingUsageEvents = node.SyncStatus.PendingUsageEvents
			item.PendingUsageLogs = node.SyncStatus.PendingUsageLogs
			item.PendingOpsErrorLogs = node.SyncStatus.PendingOpsErrorLogs
			diag.Stats.PendingUsageEvents += node.SyncStatus.PendingUsageEvents
			diag.Stats.PendingUsageLogs += node.SyncStatus.PendingUsageLogs
			diag.Stats.PendingOpsErrorLogs += node.SyncStatus.PendingOpsErrorLogs
		}
		byNode[item.NodeID] = &item
	}
	for _, lease := range leases {
		nodeID := strings.TrimSpace(lease.NodeID)
		if nodeID == "" {
			continue
		}
		item := byNode[nodeID]
		if item == nil {
			item = &QuotaLeaseDemoNodeDiagnostic{
				NodeID: nodeID,
				Status: QuotaLeaseDemoNodeStatusOffline,
				Health: QuotaLeaseDemoDiagnosticHealthOK,
			}
			byNode[nodeID] = item
		}
		item.LeaseCount++
		if lease.Status == QuotaLeaseDemoStatusActive {
			item.ActiveLeaseCount++
			remaining := lease.Remaining()
			item.ActiveRemaining += remaining
			if remaining < -1e-12 {
				item.OverdraftAmount += -remaining
			}
		}
	}

	out := make([]QuotaLeaseDemoNodeDiagnostic, 0, len(byNode))
	for _, item := range byNode {
		quotaLeaseDemoDiagnoseNode(item, now, diag)
		out = append(out, *item)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Health == out[j].Health {
			return out[i].NodeID < out[j].NodeID
		}
		return quotaLeaseDemoDiagnosticSeverity(out[i].Health) > quotaLeaseDemoDiagnosticSeverity(out[j].Health)
	})
	return out
}

func quotaLeaseDemoDiagnoseNode(item *QuotaLeaseDemoNodeDiagnostic, now time.Time, diag *QuotaLeaseDemoDiagnostics) {
	if item == nil {
		return
	}
	switch item.Status {
	case QuotaLeaseDemoNodeStatusDisabled:
		quotaLeaseDemoAddDiagnosticIssue(diag, item, nil, nil, QuotaLeaseDemoDiagnosticHealthCritical, "node", "node_disabled", "节点已禁用", "", item.NodeID, 0, 0, "", nil)
	case QuotaLeaseDemoNodeStatusOffline:
		quotaLeaseDemoAddDiagnosticIssue(diag, item, nil, nil, QuotaLeaseDemoDiagnosticHealthCritical, "node", "node_offline", "节点离线", "", item.NodeID, 0, 0, "", nil)
	}
	if item.LastHeartbeatAt == nil {
		quotaLeaseDemoAddDiagnosticIssue(diag, item, nil, nil, QuotaLeaseDemoDiagnosticHealthWarning, "node", "node_missing_heartbeat", "节点没有心跳", "", item.NodeID, 0, 0, "", nil)
	} else {
		age := now.Sub(item.LastHeartbeatAt.UTC())
		switch {
		case age >= quotaLeaseDemoDiagnosticHeartbeatStaleAge:
			quotaLeaseDemoAddDiagnosticIssue(diag, item, nil, nil, QuotaLeaseDemoDiagnosticHealthCritical, "node", "node_heartbeat_stale", "节点心跳超过 3 分钟", fmt.Sprintf("最近心跳距今 %s", quotaLeaseDemoDiagnosticDurationLabel(age)), item.NodeID, 0, 0, "", item.LastHeartbeatAt)
		case age >= quotaLeaseDemoDiagnosticHeartbeatWarningAge:
			quotaLeaseDemoAddDiagnosticIssue(diag, item, nil, nil, QuotaLeaseDemoDiagnosticHealthWarning, "node", "node_heartbeat_slow", "节点心跳超过 90 秒", fmt.Sprintf("最近心跳距今 %s", quotaLeaseDemoDiagnosticDurationLabel(age)), item.NodeID, 0, 0, "", item.LastHeartbeatAt)
		}
	}
	if item.SyncStatus == nil {
		quotaLeaseDemoAddDiagnosticIssue(diag, item, nil, nil, QuotaLeaseDemoDiagnosticHealthWarning, "node", "node_sync_status_missing", "节点没有上报同步状态", "", item.NodeID, 0, 0, "", nil)
		return
	}
	if strings.TrimSpace(item.SyncStatus.LastSyncError) != "" {
		quotaLeaseDemoAddDiagnosticIssue(diag, item, nil, nil, QuotaLeaseDemoDiagnosticHealthCritical, "node", "node_sync_failed", "节点同步失败", item.SyncStatus.LastSyncError, item.NodeID, 0, 0, "", item.SyncStatus.LastSyncFailedAt)
	}
	if !item.SyncStatus.MirrorReady {
		quotaLeaseDemoAddDiagnosticIssue(diag, item, nil, nil, QuotaLeaseDemoDiagnosticHealthWarning, "node", "node_mirror_pending", "节点镜像准备中", "", item.NodeID, 0, 0, "", item.SyncStatus.LastSyncStartedAt)
	}
	if item.SyncStatus.LastSyncSuccessAt != nil {
		age := now.Sub(item.SyncStatus.LastSyncSuccessAt.UTC())
		switch {
		case age >= quotaLeaseDemoDiagnosticSyncStaleAge:
			quotaLeaseDemoAddDiagnosticIssue(diag, item, nil, nil, QuotaLeaseDemoDiagnosticHealthCritical, "node", "node_sync_stale", "节点同步超过 5 分钟没有成功", fmt.Sprintf("最近同步距今 %s", quotaLeaseDemoDiagnosticDurationLabel(age)), item.NodeID, 0, 0, "", item.SyncStatus.LastSyncSuccessAt)
		case age >= quotaLeaseDemoDiagnosticSyncWarningAge:
			quotaLeaseDemoAddDiagnosticIssue(diag, item, nil, nil, QuotaLeaseDemoDiagnosticHealthWarning, "node", "node_sync_slow", "节点同步超过 90 秒", fmt.Sprintf("最近同步距今 %s", quotaLeaseDemoDiagnosticDurationLabel(age)), item.NodeID, 0, 0, "", item.SyncStatus.LastSyncSuccessAt)
		}
	}
	pending := item.PendingUsageEvents + item.PendingUsageLogs + item.PendingOpsErrorLogs
	if pending >= quotaLeaseDemoDiagnosticPendingCritical {
		quotaLeaseDemoAddDiagnosticIssue(diag, item, nil, nil, QuotaLeaseDemoDiagnosticHealthCritical, "node", "node_pending_uploads_high", "节点待上传数据过多", quotaLeaseDemoDiagnosticPendingDetail(item), item.NodeID, 0, 0, "", nil)
	} else if pending > 0 {
		quotaLeaseDemoAddDiagnosticIssue(diag, item, nil, nil, QuotaLeaseDemoDiagnosticHealthWarning, "node", "node_pending_uploads", "节点有待上传数据", quotaLeaseDemoDiagnosticPendingDetail(item), item.NodeID, 0, 0, "", nil)
	}
}

func quotaLeaseDemoDiagnosticUsers(
	leases []QuotaLeaseDemoLease,
	events []QuotaLeaseDemoLedgerEvent,
	profiles map[int64]QuotaLeaseDemoDiagnosticUserProfile,
	diag *QuotaLeaseDemoDiagnostics,
) []QuotaLeaseDemoUserDiagnostic {
	byUser := make(map[int64]*QuotaLeaseDemoUserDiagnostic)
	for _, lease := range leases {
		if lease.UserID <= 0 {
			continue
		}
		item := quotaLeaseDemoDiagnosticUserEntry(byUser, lease.UserID)
		item.LeaseCount++
		item.GrantedTotal += lease.Granted
		item.ConsumedTotal += lease.Consumed
		item.ReclaimedTotal += lease.Reclaimed
		item.APIKeyIDs = append(item.APIKeyIDs, lease.APIKeyID)
		if item.LastLeaseAt == nil || lease.CreatedAt.After(*item.LastLeaseAt) {
			createdAt := lease.CreatedAt
			item.LastLeaseAt = &createdAt
		}
		if lease.Status == QuotaLeaseDemoStatusActive {
			item.ActiveLeaseCount++
			remaining := lease.Remaining()
			item.ActiveRemaining += remaining
			if remaining < -1e-12 {
				item.OverdraftAmount += -remaining
			}
		}
	}
	for _, event := range events {
		if event.UserID <= 0 {
			continue
		}
		item := quotaLeaseDemoDiagnosticUserEntry(byUser, event.UserID)
		if item.LastEventAt == nil || event.CreatedAt.After(*item.LastEventAt) {
			createdAt := event.CreatedAt
			item.LastEventAt = &createdAt
		}
	}
	for userID, profile := range profiles {
		item := quotaLeaseDemoDiagnosticUserEntry(byUser, userID)
		item.Username = strings.TrimSpace(profile.Username)
		item.Email = strings.TrimSpace(profile.Email)
		item.Status = strings.TrimSpace(profile.Status)
		item.Balance = cloneQuotaLeaseDemoFloat64Ptr(profile.Balance)
		item.FrozenBalance = cloneQuotaLeaseDemoFloat64Ptr(profile.FrozenBalance)
		item.ProfileError = strings.TrimSpace(profile.Error)
	}

	out := make([]QuotaLeaseDemoUserDiagnostic, 0, len(byUser))
	for _, item := range byUser {
		item.APIKeyIDs = quotaLeaseDemoUniqueSortedInt64s(item.APIKeyIDs)
		quotaLeaseDemoDiagnoseUser(item, diag)
		out = append(out, *item)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Health == out[j].Health {
			if out[i].LastEventAt != nil && out[j].LastEventAt != nil && !out[i].LastEventAt.Equal(*out[j].LastEventAt) {
				return out[i].LastEventAt.After(*out[j].LastEventAt)
			}
			return out[i].UserID < out[j].UserID
		}
		return quotaLeaseDemoDiagnosticSeverity(out[i].Health) > quotaLeaseDemoDiagnosticSeverity(out[j].Health)
	})
	return out
}

func quotaLeaseDemoDiagnosticUserEntry(byUser map[int64]*QuotaLeaseDemoUserDiagnostic, userID int64) *QuotaLeaseDemoUserDiagnostic {
	item := byUser[userID]
	if item == nil {
		item = &QuotaLeaseDemoUserDiagnostic{
			UserID: userID,
			Health: QuotaLeaseDemoDiagnosticHealthOK,
		}
		byUser[userID] = item
	}
	return item
}

func quotaLeaseDemoDiagnoseUser(item *QuotaLeaseDemoUserDiagnostic, diag *QuotaLeaseDemoDiagnostics) {
	if item == nil {
		return
	}
	if item.ProfileError != "" {
		quotaLeaseDemoAddDiagnosticIssue(diag, nil, item, nil, QuotaLeaseDemoDiagnosticHealthWarning, "user", "user_profile_load_failed", "用户资料读取失败", item.ProfileError, "", item.UserID, 0, "", nil)
	}
	if item.OverdraftAmount > 1e-12 {
		level := QuotaLeaseDemoDiagnosticHealthWarning
		detail := fmt.Sprintf("透支 %.6fU", item.OverdraftAmount)
		if item.Balance != nil && *item.Balance <= 1e-12 {
			level = QuotaLeaseDemoDiagnosticHealthCritical
			detail = fmt.Sprintf("余额 %.6fU，透支 %.6fU", *item.Balance, item.OverdraftAmount)
		}
		quotaLeaseDemoAddDiagnosticIssue(diag, nil, item, nil, level, "user", "user_has_overdraft_lease", "用户有透支租约", detail, "", item.UserID, 0, "", nil)
	}
	if item.Balance != nil && *item.Balance <= 1e-12 && item.ActiveRemaining > 1e-12 {
		quotaLeaseDemoAddDiagnosticIssue(diag, nil, item, nil, QuotaLeaseDemoDiagnosticHealthCritical, "user", "user_zero_balance_active_lease", "用户余额耗尽且仍有活跃租约", fmt.Sprintf("活跃租约剩余 %.6fU", item.ActiveRemaining), "", item.UserID, 0, "", nil)
	}
}

func quotaLeaseDemoDiagnosticLeases(
	leases []QuotaLeaseDemoLease,
	eventStats map[string]quotaLeaseDemoDiagnosticEventStats,
	now time.Time,
	preflightReserve float64,
	diag *QuotaLeaseDemoDiagnostics,
) []QuotaLeaseDemoLeaseDiagnostic {
	out := make([]QuotaLeaseDemoLeaseDiagnostic, 0, len(leases))
	for _, lease := range leases {
		stats := eventStats[lease.ID]
		item := QuotaLeaseDemoLeaseDiagnostic{
			ID:               lease.ID,
			NodeID:           lease.NodeID,
			UserID:           lease.UserID,
			APIKeyID:         lease.APIKeyID,
			Status:           lease.Status,
			Health:           QuotaLeaseDemoDiagnosticHealthOK,
			Granted:          lease.Granted,
			Consumed:         lease.Consumed,
			Reclaimed:        lease.Reclaimed,
			Remaining:        lease.Remaining(),
			EventCount:       stats.count,
			UsageEventTotal:  stats.usageTotal,
			LastEventAt:      cloneQuotaLeaseDemoTimePtr(stats.lastAt),
			ExpiresAt:        lease.ExpiresAt,
			ReclaimAt:        lease.ReclaimAt,
			CreatedAt:        lease.CreatedAt,
			UpdatedAt:        lease.UpdatedAt,
			ExpiresInSeconds: int64(lease.ExpiresAt.Sub(now).Seconds()),
			ReclaimInSeconds: int64(lease.ReclaimAt.Sub(now).Seconds()),
		}
		quotaLeaseDemoDiagnoseLease(&item, preflightReserve, diag)
		out = append(out, item)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Health == out[j].Health {
			return out[i].UpdatedAt.After(out[j].UpdatedAt)
		}
		return quotaLeaseDemoDiagnosticSeverity(out[i].Health) > quotaLeaseDemoDiagnosticSeverity(out[j].Health)
	})
	return out
}

func quotaLeaseDemoDiagnoseLease(item *QuotaLeaseDemoLeaseDiagnostic, preflightReserve float64, diag *QuotaLeaseDemoDiagnostics) {
	if item == nil {
		return
	}
	if item.Remaining < -1e-12 {
		diag.Stats.OverdraftLeases++
		diag.Stats.OverdraftTotal += -item.Remaining
		quotaLeaseDemoAddDiagnosticIssue(diag, nil, nil, item, QuotaLeaseDemoDiagnosticHealthCritical, "lease", "lease_overdraft", "租约已透支", fmt.Sprintf("剩余 %.6fU", item.Remaining), item.NodeID, item.UserID, item.APIKeyID, item.ID, nil)
	}
	if item.Status == QuotaLeaseDemoStatusActive && preflightReserve > 0 && item.Remaining >= -1e-12 && item.Remaining < preflightReserve {
		diag.Stats.LowCapacityLeases++
		quotaLeaseDemoAddDiagnosticIssue(diag, nil, nil, item, QuotaLeaseDemoDiagnosticHealthWarning, "lease", "lease_low_capacity", "租约剩余额度偏低", fmt.Sprintf("剩余 %.6fU，预检需要 %.6fU", item.Remaining, preflightReserve), item.NodeID, item.UserID, item.APIKeyID, item.ID, nil)
	}
	if item.Status == QuotaLeaseDemoStatusExpired && item.Remaining > 1e-12 {
		quotaLeaseDemoAddDiagnosticIssue(diag, nil, nil, item, QuotaLeaseDemoDiagnosticHealthWarning, "lease", "lease_expired_reclaimable", "过期租约有可回收额度", fmt.Sprintf("可回收 %.6fU", item.Remaining), item.NodeID, item.UserID, item.APIKeyID, item.ID, &item.ExpiresAt)
	}
	if math.Abs(item.UsageEventTotal-item.Consumed) > 1e-9 {
		quotaLeaseDemoAddDiagnosticIssue(diag, nil, nil, item, QuotaLeaseDemoDiagnosticHealthCritical, "lease", "lease_usage_mismatch", "消费流水与租约记录不一致", fmt.Sprintf("流水 %.6fU，租约消费 %.6fU", item.UsageEventTotal, item.Consumed), item.NodeID, item.UserID, item.APIKeyID, item.ID, item.LastEventAt)
	}
}

func quotaLeaseDemoAddDiagnosticIssue(
	diag *QuotaLeaseDemoDiagnostics,
	node *QuotaLeaseDemoNodeDiagnostic,
	user *QuotaLeaseDemoUserDiagnostic,
	lease *QuotaLeaseDemoLeaseDiagnostic,
	level string,
	scope string,
	code string,
	message string,
	detail string,
	nodeID string,
	userID int64,
	apiKeyID int64,
	leaseID string,
	createdAt *time.Time,
) {
	if diag == nil {
		return
	}
	level = quotaLeaseDemoDiagnosticHealthLabel(level)
	issue := QuotaLeaseDemoDiagnosticIssue{
		ID:        quotaLeaseDemoDiagnosticIssueID(scope, code, nodeID, userID, apiKeyID, leaseID),
		Level:     level,
		Scope:     strings.TrimSpace(scope),
		Code:      strings.TrimSpace(code),
		Message:   strings.TrimSpace(message),
		Detail:    quotaLeaseDemoTrimDiagnosticText(detail, 500),
		NodeID:    strings.TrimSpace(nodeID),
		UserID:    userID,
		APIKeyID:  apiKeyID,
		LeaseID:   strings.TrimSpace(leaseID),
		CreatedAt: cloneQuotaLeaseDemoTimePtr(createdAt),
	}
	diag.Issues = append(diag.Issues, issue)
	diag.Stats.IssueCount++
	switch level {
	case QuotaLeaseDemoDiagnosticHealthCritical:
		diag.Stats.CriticalCount++
	case QuotaLeaseDemoDiagnosticHealthWarning:
		diag.Stats.WarningCount++
	}
	if node != nil {
		node.Health = quotaLeaseDemoDiagnosticMaxHealth(node.Health, level)
		node.Issues = append(node.Issues, issue.Message)
	}
	if user != nil {
		user.Health = quotaLeaseDemoDiagnosticMaxHealth(user.Health, level)
		user.Issues = append(user.Issues, issue.Message)
	}
	if lease != nil {
		lease.Health = quotaLeaseDemoDiagnosticMaxHealth(lease.Health, level)
		lease.Issues = append(lease.Issues, issue.Message)
	}
}

func quotaLeaseDemoDiagnosticIssueID(scope, code, nodeID string, userID, apiKeyID int64, leaseID string) string {
	raw := fmt.Sprintf("%s:%s:%s:%d:%d:%s", strings.TrimSpace(scope), strings.TrimSpace(code), strings.TrimSpace(nodeID), userID, apiKeyID, strings.TrimSpace(leaseID))
	raw = strings.ReplaceAll(raw, " ", "_")
	raw = strings.ReplaceAll(raw, "/", "_")
	return raw
}

func quotaLeaseDemoDiagnosticOverallHealth(criticalCount, warningCount int) string {
	if criticalCount > 0 {
		return QuotaLeaseDemoDiagnosticHealthCritical
	}
	if warningCount > 0 {
		return QuotaLeaseDemoDiagnosticHealthWarning
	}
	return QuotaLeaseDemoDiagnosticHealthOK
}

func quotaLeaseDemoDiagnosticMaxHealth(current, next string) string {
	if current == "" {
		current = QuotaLeaseDemoDiagnosticHealthOK
	}
	if quotaLeaseDemoDiagnosticSeverity(next) > quotaLeaseDemoDiagnosticSeverity(current) {
		return quotaLeaseDemoDiagnosticHealthLabel(next)
	}
	return quotaLeaseDemoDiagnosticHealthLabel(current)
}

func quotaLeaseDemoDiagnosticHealthLabel(value string) string {
	switch strings.TrimSpace(value) {
	case QuotaLeaseDemoDiagnosticHealthCritical:
		return QuotaLeaseDemoDiagnosticHealthCritical
	case QuotaLeaseDemoDiagnosticHealthWarning:
		return QuotaLeaseDemoDiagnosticHealthWarning
	default:
		return QuotaLeaseDemoDiagnosticHealthOK
	}
}

func quotaLeaseDemoDiagnosticSeverity(value string) int {
	switch strings.TrimSpace(value) {
	case QuotaLeaseDemoDiagnosticHealthCritical:
		return 2
	case QuotaLeaseDemoDiagnosticHealthWarning:
		return 1
	default:
		return 0
	}
}

func quotaLeaseDemoDiagnosticPendingDetail(item *QuotaLeaseDemoNodeDiagnostic) string {
	if item == nil {
		return ""
	}
	return fmt.Sprintf("扣费 %d，记录 %d，错误 %d", item.PendingUsageEvents, item.PendingUsageLogs, item.PendingOpsErrorLogs)
}

func quotaLeaseDemoDiagnosticDurationLabel(duration time.Duration) string {
	if duration < 0 {
		duration = 0
	}
	seconds := int64(duration.Round(time.Second).Seconds())
	if seconds < 60 {
		return fmt.Sprintf("%d 秒", seconds)
	}
	minutes := seconds / 60
	rest := seconds % 60
	if minutes < 60 {
		if rest == 0 {
			return fmt.Sprintf("%d 分钟", minutes)
		}
		return fmt.Sprintf("%d 分钟 %d 秒", minutes, rest)
	}
	hours := minutes / 60
	minuteRest := minutes % 60
	if minuteRest == 0 {
		return fmt.Sprintf("%d 小时", hours)
	}
	return fmt.Sprintf("%d 小时 %d 分钟", hours, minuteRest)
}

func quotaLeaseDemoTrimDiagnosticText(message string, limit int) string {
	message = strings.TrimSpace(message)
	if limit <= 0 || len(message) <= limit {
		return message
	}
	return message[:limit]
}
