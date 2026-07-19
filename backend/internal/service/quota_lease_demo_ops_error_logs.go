package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"sort"
	"strconv"
	"strings"
	"time"
)

type QuotaLeaseDemoOpsErrorLogSnapshot struct {
	NodeID          string `json:"node_id,omitempty"`
	RequestID       string `json:"request_id,omitempty"`
	ClientRequestID string `json:"client_request_id,omitempty"`

	UserID    *int64  `json:"user_id,omitempty"`
	APIKeyID  *int64  `json:"api_key_id,omitempty"`
	AccountID *int64  `json:"account_id,omitempty"`
	GroupID   *int64  `json:"group_id,omitempty"`
	ClientIP  *string `json:"client_ip,omitempty"`

	Platform         string `json:"platform,omitempty"`
	Model            string `json:"model,omitempty"`
	RequestPath      string `json:"request_path,omitempty"`
	Stream           bool   `json:"stream"`
	InboundEndpoint  string `json:"inbound_endpoint,omitempty"`
	UpstreamEndpoint string `json:"upstream_endpoint,omitempty"`
	RequestedModel   string `json:"requested_model,omitempty"`
	UpstreamModel    string `json:"upstream_model,omitempty"`
	RequestType      *int16 `json:"request_type,omitempty"`
	UserAgent        string `json:"user_agent,omitempty"`

	ErrorPhase        string `json:"error_phase,omitempty"`
	ErrorType         string `json:"error_type,omitempty"`
	Severity          string `json:"severity,omitempty"`
	StatusCode        int    `json:"status_code"`
	IsBusinessLimited bool   `json:"is_business_limited"`
	IsCountTokens     bool   `json:"is_count_tokens"`

	ErrorMessage string `json:"error_message,omitempty"`
	ErrorBody    string `json:"error_body,omitempty"`
	ErrorSource  string `json:"error_source,omitempty"`
	ErrorOwner   string `json:"error_owner,omitempty"`

	UpstreamStatusCode   *int    `json:"upstream_status_code,omitempty"`
	UpstreamErrorMessage *string `json:"upstream_error_message,omitempty"`
	UpstreamErrorDetail  *string `json:"upstream_error_detail,omitempty"`
	UpstreamErrorsJSON   *string `json:"upstream_errors_json,omitempty"`

	AuthLatencyMs      *int64 `json:"auth_latency_ms,omitempty"`
	RoutingLatencyMs   *int64 `json:"routing_latency_ms,omitempty"`
	UpstreamLatencyMs  *int64 `json:"upstream_latency_ms,omitempty"`
	ResponseLatencyMs  *int64 `json:"response_latency_ms,omitempty"`
	TimeToFirstTokenMs *int64 `json:"time_to_first_token_ms,omitempty"`

	AttemptedKeyPrefix    string `json:"attempted_key_prefix,omitempty"`
	DeletedKeyOwnerUserID *int64 `json:"deleted_key_owner_user_id,omitempty"`
	DeletedKeyName        string `json:"deleted_key_name,omitempty"`
	APIKeyPrefix          string `json:"api_key_prefix,omitempty"`

	CreatedAt time.Time `json:"created_at"`
}

type QuotaLeaseDemoOpsErrorLogBatchRequest struct {
	NodeID string                              `json:"node_id"`
	Logs   []QuotaLeaseDemoOpsErrorLogSnapshot `json:"logs"`
}

type QuotaLeaseDemoOpsErrorLogResult struct {
	Key             string `json:"key,omitempty"`
	RequestID       string `json:"request_id,omitempty"`
	ClientRequestID string `json:"client_request_id,omitempty"`
	Applied         bool   `json:"applied"`
	Duplicate       bool   `json:"duplicate"`
	Error           string `json:"error,omitempty"`
}

type QuotaLeaseDemoOpsErrorLogBatchResult struct {
	Results []QuotaLeaseDemoOpsErrorLogResult `json:"results"`
}

func NewQuotaLeaseDemoOpsErrorLogSnapshot(nodeID string, entry *OpsInsertErrorLogInput) QuotaLeaseDemoOpsErrorLogSnapshot {
	if entry == nil {
		return QuotaLeaseDemoOpsErrorLogSnapshot{}
	}
	nodeID = strings.TrimSpace(nodeID)
	if nodeID == "" {
		nodeID = strings.TrimSpace(entry.NodeID)
	}
	snapshot := QuotaLeaseDemoOpsErrorLogSnapshot{
		NodeID:          nodeID,
		RequestID:       strings.TrimSpace(entry.RequestID),
		ClientRequestID: strings.TrimSpace(entry.ClientRequestID),
		UserID:          cloneQuotaLeaseDemoInt64Ptr(entry.UserID),
		APIKeyID:        cloneQuotaLeaseDemoInt64Ptr(entry.APIKeyID),
		AccountID:       cloneQuotaLeaseDemoInt64Ptr(entry.AccountID),
		GroupID:         cloneQuotaLeaseDemoInt64Ptr(entry.GroupID),
		ClientIP:        cloneQuotaLeaseDemoStringPtr(entry.ClientIP),

		Platform:         strings.TrimSpace(entry.Platform),
		Model:            strings.TrimSpace(entry.Model),
		RequestPath:      strings.TrimSpace(entry.RequestPath),
		Stream:           entry.Stream,
		InboundEndpoint:  strings.TrimSpace(entry.InboundEndpoint),
		UpstreamEndpoint: strings.TrimSpace(entry.UpstreamEndpoint),
		RequestedModel:   strings.TrimSpace(entry.RequestedModel),
		UpstreamModel:    strings.TrimSpace(entry.UpstreamModel),
		RequestType:      cloneQuotaLeaseDemoInt16Ptr(entry.RequestType),
		UserAgent:        strings.TrimSpace(entry.UserAgent),

		ErrorPhase:        strings.TrimSpace(entry.ErrorPhase),
		ErrorType:         strings.TrimSpace(entry.ErrorType),
		Severity:          strings.TrimSpace(entry.Severity),
		StatusCode:        entry.StatusCode,
		IsBusinessLimited: entry.IsBusinessLimited,
		IsCountTokens:     entry.IsCountTokens,

		ErrorMessage: strings.TrimSpace(entry.ErrorMessage),
		ErrorBody:    strings.TrimSpace(entry.ErrorBody),
		ErrorSource:  strings.TrimSpace(entry.ErrorSource),
		ErrorOwner:   strings.TrimSpace(entry.ErrorOwner),

		UpstreamStatusCode:   cloneQuotaLeaseDemoIntPtr(entry.UpstreamStatusCode),
		UpstreamErrorMessage: cloneQuotaLeaseDemoStringPtr(entry.UpstreamErrorMessage),
		UpstreamErrorDetail:  cloneQuotaLeaseDemoStringPtr(entry.UpstreamErrorDetail),
		UpstreamErrorsJSON:   cloneQuotaLeaseDemoStringPtr(entry.UpstreamErrorsJSON),

		AuthLatencyMs:      cloneQuotaLeaseDemoInt64Ptr(entry.AuthLatencyMs),
		RoutingLatencyMs:   cloneQuotaLeaseDemoInt64Ptr(entry.RoutingLatencyMs),
		UpstreamLatencyMs:  cloneQuotaLeaseDemoInt64Ptr(entry.UpstreamLatencyMs),
		ResponseLatencyMs:  cloneQuotaLeaseDemoInt64Ptr(entry.ResponseLatencyMs),
		TimeToFirstTokenMs: cloneQuotaLeaseDemoInt64Ptr(entry.TimeToFirstTokenMs),

		AttemptedKeyPrefix:    strings.TrimSpace(entry.AttemptedKeyPrefix),
		DeletedKeyOwnerUserID: cloneQuotaLeaseDemoInt64Ptr(entry.DeletedKeyOwnerUserID),
		DeletedKeyName:        strings.TrimSpace(entry.DeletedKeyName),
		APIKeyPrefix:          strings.TrimSpace(entry.APIKeyPrefix),
		CreatedAt:             entry.CreatedAt,
	}
	if snapshot.CreatedAt.IsZero() {
		snapshot.CreatedAt = time.Now().UTC()
	}
	return snapshot
}

func (s QuotaLeaseDemoOpsErrorLogSnapshot) ToOpsInsertErrorLogInput() *OpsInsertErrorLogInput {
	entry := &OpsInsertErrorLogInput{
		NodeID:          strings.TrimSpace(s.NodeID),
		RequestID:       strings.TrimSpace(s.RequestID),
		ClientRequestID: strings.TrimSpace(s.ClientRequestID),
		UserID:          cloneQuotaLeaseDemoInt64Ptr(s.UserID),
		APIKeyID:        cloneQuotaLeaseDemoInt64Ptr(s.APIKeyID),
		AccountID:       cloneQuotaLeaseDemoInt64Ptr(s.AccountID),
		GroupID:         cloneQuotaLeaseDemoInt64Ptr(s.GroupID),
		ClientIP:        cloneQuotaLeaseDemoStringPtr(s.ClientIP),

		Platform:         strings.TrimSpace(s.Platform),
		Model:            strings.TrimSpace(s.Model),
		RequestPath:      strings.TrimSpace(s.RequestPath),
		Stream:           s.Stream,
		InboundEndpoint:  strings.TrimSpace(s.InboundEndpoint),
		UpstreamEndpoint: strings.TrimSpace(s.UpstreamEndpoint),
		RequestedModel:   strings.TrimSpace(s.RequestedModel),
		UpstreamModel:    strings.TrimSpace(s.UpstreamModel),
		RequestType:      cloneQuotaLeaseDemoInt16Ptr(s.RequestType),
		UserAgent:        strings.TrimSpace(s.UserAgent),

		ErrorPhase:        strings.TrimSpace(s.ErrorPhase),
		ErrorType:         strings.TrimSpace(s.ErrorType),
		Severity:          strings.TrimSpace(s.Severity),
		StatusCode:        s.StatusCode,
		IsBusinessLimited: s.IsBusinessLimited,
		IsCountTokens:     s.IsCountTokens,

		ErrorMessage: strings.TrimSpace(s.ErrorMessage),
		ErrorBody:    strings.TrimSpace(s.ErrorBody),
		ErrorSource:  strings.TrimSpace(s.ErrorSource),
		ErrorOwner:   strings.TrimSpace(s.ErrorOwner),

		UpstreamStatusCode:   cloneQuotaLeaseDemoIntPtr(s.UpstreamStatusCode),
		UpstreamErrorMessage: cloneQuotaLeaseDemoStringPtr(s.UpstreamErrorMessage),
		UpstreamErrorDetail:  cloneQuotaLeaseDemoStringPtr(s.UpstreamErrorDetail),
		UpstreamErrorsJSON:   cloneQuotaLeaseDemoStringPtr(s.UpstreamErrorsJSON),

		AuthLatencyMs:      cloneQuotaLeaseDemoInt64Ptr(s.AuthLatencyMs),
		RoutingLatencyMs:   cloneQuotaLeaseDemoInt64Ptr(s.RoutingLatencyMs),
		UpstreamLatencyMs:  cloneQuotaLeaseDemoInt64Ptr(s.UpstreamLatencyMs),
		ResponseLatencyMs:  cloneQuotaLeaseDemoInt64Ptr(s.ResponseLatencyMs),
		TimeToFirstTokenMs: cloneQuotaLeaseDemoInt64Ptr(s.TimeToFirstTokenMs),

		AttemptedKeyPrefix:    strings.TrimSpace(s.AttemptedKeyPrefix),
		DeletedKeyOwnerUserID: cloneQuotaLeaseDemoInt64Ptr(s.DeletedKeyOwnerUserID),
		DeletedKeyName:        strings.TrimSpace(s.DeletedKeyName),
		APIKeyPrefix:          strings.TrimSpace(s.APIKeyPrefix),
		CreatedAt:             s.CreatedAt,
	}
	if entry.CreatedAt.IsZero() {
		entry.CreatedAt = time.Now().UTC()
	}
	return entry
}

func (s QuotaLeaseDemoOpsErrorLogSnapshot) Key() string {
	return quotaLeaseDemoOpsErrorLogSnapshotKey(
		s.NodeID,
		s.RequestID,
		s.ClientRequestID,
		s.APIKeyID,
		s.AccountID,
		s.ErrorPhase,
		s.ErrorType,
		s.StatusCode,
		s.CreatedAt,
	)
}

func (s *QuotaLeaseDemoService) enqueuePendingOpsErrorLogSnapshot(snapshot QuotaLeaseDemoOpsErrorLogSnapshot) bool {
	if s == nil {
		return false
	}
	snapshot.NodeID = strings.TrimSpace(snapshot.NodeID)
	if snapshot.NodeID == "" {
		snapshot.NodeID = s.NodeID()
	}
	if snapshot.CreatedAt.IsZero() {
		snapshot.CreatedAt = time.Now().UTC()
	}
	key := snapshot.Key()
	if key == "" {
		return false
	}
	s.mu.Lock()
	if s.pendingOpsErrorLogs == nil {
		s.pendingOpsErrorLogs = make(map[string]QuotaLeaseDemoOpsErrorLogSnapshot)
	}
	s.pendingOpsErrorLogs[key] = snapshot
	s.mu.Unlock()
	return true
}

func (s *QuotaLeaseDemoService) pendingOpsErrorLogSnapshots() []QuotaLeaseDemoOpsErrorLogSnapshot {
	if s == nil {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	logs := make([]QuotaLeaseDemoOpsErrorLogSnapshot, 0, len(s.pendingOpsErrorLogs))
	for _, log := range s.pendingOpsErrorLogs {
		logs = append(logs, log)
	}
	sort.Slice(logs, func(i, j int) bool {
		return logs[i].CreatedAt.Before(logs[j].CreatedAt)
	})
	return logs
}

func (s *QuotaLeaseDemoService) removePendingOpsErrorLogResults(result QuotaLeaseDemoOpsErrorLogBatchResult) {
	if s == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	for _, item := range result.Results {
		if strings.TrimSpace(item.Error) != "" {
			continue
		}
		if item.Applied || item.Duplicate {
			delete(s.pendingOpsErrorLogs, strings.TrimSpace(item.Key))
		}
	}
}

func (s *OpsService) forwardQuotaLeaseDemoNodeErrorLogs(ctx context.Context, entries []*OpsInsertErrorLogInput) bool {
	if s == nil || s.cfg == nil || !s.cfg.IsNodeRole() || !QuotaLeaseDemoEnabled(s.cfg) {
		return false
	}
	leaseSvc := GetQuotaLeaseDemoService(s.cfg)
	if leaseSvc == nil || !leaseSvc.remoteMode() {
		return false
	}
	nodeID := leaseSvc.activeNodeID()
	enqueued := 0
	for _, entry := range entries {
		snapshot := NewQuotaLeaseDemoOpsErrorLogSnapshot(nodeID, entry)
		if leaseSvc.enqueuePendingOpsErrorLogSnapshot(snapshot) {
			enqueued++
		}
	}
	if enqueued == 0 {
		return false
	}
	leaseSvc.flushPendingOpsErrorLogsAsync()
	_ = ctx
	return true
}

func quotaLeaseDemoOpsErrorLogSnapshotKey(nodeID, requestID, clientRequestID string, apiKeyID, accountID *int64, phase, errorType string, statusCode int, createdAt time.Time) string {
	parts := []string{
		strings.TrimSpace(nodeID),
		strings.TrimSpace(requestID),
		strings.TrimSpace(clientRequestID),
		quotaLeaseDemoInt64PtrKey(apiKeyID),
		quotaLeaseDemoInt64PtrKey(accountID),
		strings.TrimSpace(phase),
		strings.TrimSpace(errorType),
		strconv.Itoa(statusCode),
	}
	if !createdAt.IsZero() {
		parts = append(parts, strconv.FormatInt(createdAt.UTC().UnixNano(), 10))
	}
	raw := strings.Join(parts, "\x1f")
	if strings.Trim(raw, "\x1f") == "" {
		return ""
	}
	sum := sha256.Sum256([]byte(raw))
	return "ops_error:" + hex.EncodeToString(sum[:])
}

func quotaLeaseDemoInt64PtrKey(value *int64) string {
	if value == nil {
		return ""
	}
	return strconv.FormatInt(*value, 10)
}

func cloneQuotaLeaseDemoInt16Ptr(src *int16) *int16 {
	if src == nil {
		return nil
	}
	value := *src
	return &value
}
