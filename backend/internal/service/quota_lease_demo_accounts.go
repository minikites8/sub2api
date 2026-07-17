package service

import (
	"context"
	"fmt"
	"log/slog"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
)

const (
	QuotaLeaseDemoAccountTaskPending   = "pending"
	QuotaLeaseDemoAccountTaskWaiting   = "waiting_callback"
	QuotaLeaseDemoAccountTaskReady     = "callback_ready"
	QuotaLeaseDemoAccountTaskCompleted = "completed"
	QuotaLeaseDemoAccountTaskFailed    = "failed"
)

type QuotaLeaseDemoAccountSnapshot struct {
	ID                      int64                        `json:"id"`
	Name                    string                       `json:"name"`
	Platform                string                       `json:"platform"`
	Type                    string                       `json:"type"`
	Credentials             map[string]any               `json:"credentials,omitempty"`
	Extra                   map[string]any               `json:"extra,omitempty"`
	ProxyID                 *int64                       `json:"proxy_id,omitempty"`
	Proxy                   *QuotaLeaseDemoProxySnapshot `json:"proxy,omitempty"`
	Status                  string                       `json:"status"`
	ErrorMessage            string                       `json:"error_message,omitempty"`
	Schedulable             bool                         `json:"schedulable"`
	Concurrency             int                          `json:"concurrency"`
	Priority                int                          `json:"priority"`
	GroupIDs                []int64                      `json:"group_ids,omitempty"`
	ExpiresAt               *time.Time                   `json:"expires_at,omitempty"`
	RateLimitResetAt        *time.Time                   `json:"rate_limit_reset_at,omitempty"`
	TempUnschedulableUntil  *time.Time                   `json:"temp_unschedulable_until,omitempty"`
	TempUnschedulableReason string                       `json:"temp_unschedulable_reason,omitempty"`
	UpdatedAt               time.Time                    `json:"updated_at"`
}

type QuotaLeaseDemoProxySnapshot struct {
	ID             int64      `json:"id"`
	Name           string     `json:"name,omitempty"`
	Protocol       string     `json:"protocol"`
	Host           string     `json:"host"`
	Port           int        `json:"port"`
	Username       string     `json:"username,omitempty"`
	Password       string     `json:"password,omitempty"`
	Status         string     `json:"status,omitempty"`
	ExpiresAt      *time.Time `json:"expires_at,omitempty"`
	FallbackMode   string     `json:"fallback_mode,omitempty"`
	BackupProxyID  *int64     `json:"backup_proxy_id,omitempty"`
	ExpiryWarnDays int        `json:"expiry_warn_days,omitempty"`
}

type QuotaLeaseDemoAccountLoginTaskCreateRequest struct {
	AccountID      int64             `json:"account_id"`
	Name           string            `json:"name"`
	Platform       string            `json:"platform"`
	Type           string            `json:"type"`
	AssignedNodeID string            `json:"assigned_node_id"`
	LoginPayload   map[string]any    `json:"login_payload,omitempty"`
	Metadata       map[string]string `json:"metadata,omitempty"`
	GroupIDs       []int64           `json:"group_ids,omitempty"`
	Concurrency    int               `json:"concurrency"`
	Priority       int               `json:"priority"`
}

type QuotaLeaseDemoAccountLoginTaskCompleteRequest struct {
	TaskID  string                        `json:"task_id"`
	NodeID  string                        `json:"node_id"`
	Error   string                        `json:"error,omitempty"`
	Account QuotaLeaseDemoAccountSnapshot `json:"account"`
}

type QuotaLeaseDemoAccountLoginTaskProgressRequest struct {
	TaskID            string            `json:"task_id"`
	NodeID            string            `json:"node_id"`
	Status            string            `json:"status"`
	Error             string            `json:"error,omitempty"`
	LoginPayloadPatch map[string]any    `json:"login_payload_patch,omitempty"`
	MetadataPatch     map[string]string `json:"metadata_patch,omitempty"`
}

type QuotaLeaseDemoAccountLoginTaskCallbackRequest struct {
	TaskID      string         `json:"task_id"`
	Code        string         `json:"code,omitempty"`
	State       string         `json:"state,omitempty"`
	SessionID   string         `json:"session_id,omitempty"`
	RedirectURI string         `json:"redirect_uri,omitempty"`
	CallbackURL string         `json:"callback_url,omitempty"`
	ProxyID     *int64         `json:"proxy_id,omitempty"`
	Payload     map[string]any `json:"payload,omitempty"`
}

type QuotaLeaseDemoAccountLoginTask struct {
	ID             string                         `json:"id"`
	AccountID      int64                          `json:"account_id"`
	Name           string                         `json:"name"`
	Platform       string                         `json:"platform"`
	Type           string                         `json:"type"`
	AssignedNodeID string                         `json:"assigned_node_id"`
	LoginPayload   map[string]any                 `json:"login_payload,omitempty"`
	Metadata       map[string]string              `json:"metadata,omitempty"`
	GroupIDs       []int64                        `json:"group_ids,omitempty"`
	Concurrency    int                            `json:"concurrency"`
	Priority       int                            `json:"priority"`
	Status         string                         `json:"status"`
	Error          string                         `json:"error,omitempty"`
	Account        *QuotaLeaseDemoAccountSnapshot `json:"account,omitempty"`
	CreatedAt      time.Time                      `json:"created_at"`
	UpdatedAt      time.Time                      `json:"updated_at"`
	CompletedAt    *time.Time                     `json:"completed_at,omitempty"`
}

type QuotaLeaseDemoAssignedAccount struct {
	NodeID    string                        `json:"node_id"`
	TaskID    string                        `json:"task_id,omitempty"`
	Account   QuotaLeaseDemoAccountSnapshot `json:"account"`
	CreatedAt time.Time                     `json:"created_at"`
	UpdatedAt time.Time                     `json:"updated_at"`
}

type QuotaLeaseDemoAccountStatusReportRequest struct {
	NodeID                  string         `json:"node_id"`
	AccountID               int64          `json:"account_id"`
	Status                  string         `json:"status,omitempty"`
	Schedulable             *bool          `json:"schedulable,omitempty"`
	ErrorMessage            *string        `json:"error_message,omitempty"`
	CredentialsPatch        map[string]any `json:"credentials_patch,omitempty"`
	ExtraPatch              map[string]any `json:"extra_patch,omitempty"`
	RateLimitResetAt        *time.Time     `json:"rate_limit_reset_at,omitempty"`
	ClearRateLimitResetAt   bool           `json:"clear_rate_limit_reset_at,omitempty"`
	TempUnschedulableUntil  *time.Time     `json:"temp_unschedulable_until,omitempty"`
	TempUnschedulableReason *string        `json:"temp_unschedulable_reason,omitempty"`
	ClearTempUnschedulable  bool           `json:"clear_temp_unschedulable,omitempty"`
	ReportedAt              time.Time      `json:"reported_at,omitempty"`
}

func (s *QuotaLeaseDemoService) CreateAccountLoginTask(ctx context.Context, req QuotaLeaseDemoAccountLoginTaskCreateRequest) (*QuotaLeaseDemoAccountLoginTask, error) {
	if s.remoteMode() {
		return s.createRemoteAccountLoginTask(ctx, req)
	}
	if s == nil || !s.Enabled() {
		return nil, ErrQuotaLeaseDemoDisabled
	}
	nodeID := strings.TrimSpace(req.AssignedNodeID)
	if nodeID == "" {
		return nil, fmt.Errorf("%w: assigned_node_id is required", ErrQuotaLeaseDemoInvalidInput)
	}
	platform := strings.TrimSpace(req.Platform)
	accountType := strings.TrimSpace(req.Type)
	if req.AccountID <= 0 || platform == "" || accountType == "" {
		return nil, fmt.Errorf("%w: account_id, platform and type are required", ErrQuotaLeaseDemoInvalidInput)
	}
	now := time.Now().UTC()
	task := &QuotaLeaseDemoAccountLoginTask{
		ID:             "ql_account_task_" + uuid.NewString(),
		AccountID:      req.AccountID,
		Name:           strings.TrimSpace(req.Name),
		Platform:       platform,
		Type:           accountType,
		AssignedNodeID: nodeID,
		LoginPayload:   cloneQuotaLeaseDemoAnyMap(req.LoginPayload),
		Metadata:       cloneQuotaLeaseDemoStringMap(req.Metadata),
		GroupIDs:       cloneQuotaLeaseDemoInt64Slice(req.GroupIDs),
		Concurrency:    req.Concurrency,
		Priority:       req.Priority,
		Status:         QuotaLeaseDemoAccountTaskPending,
		CreatedAt:      now,
		UpdatedAt:      now,
	}
	if task.Concurrency <= 0 {
		task.Concurrency = 1
	}
	s.mu.Lock()
	if s.accountTasks == nil {
		s.accountTasks = make(map[string]*QuotaLeaseDemoAccountLoginTask)
	}
	s.accountTasks[task.ID] = task
	s.mu.Unlock()
	_ = ctx
	return cloneQuotaLeaseDemoAccountLoginTask(task), nil
}

func (s *QuotaLeaseDemoService) ListAccountLoginTasks(ctx context.Context, nodeID, status string) []QuotaLeaseDemoAccountLoginTask {
	if s.remoteMode() {
		tasks, err := s.fetchRemoteAccountLoginTasks(ctx, status)
		if err == nil {
			return tasks
		}
		return nil
	}
	if s == nil || !s.Enabled() {
		return nil
	}
	nodeID = strings.TrimSpace(nodeID)
	status = strings.TrimSpace(status)
	s.mu.Lock()
	defer s.mu.Unlock()

	tasks := make([]QuotaLeaseDemoAccountLoginTask, 0, len(s.accountTasks))
	for _, task := range s.accountTasks {
		if task == nil {
			continue
		}
		if nodeID != "" && task.AssignedNodeID != nodeID {
			continue
		}
		if status != "" && task.Status != status {
			continue
		}
		if cloned := cloneQuotaLeaseDemoAccountLoginTask(task); cloned != nil {
			tasks = append(tasks, *cloned)
		}
	}
	sort.Slice(tasks, func(i, j int) bool {
		return tasks[i].CreatedAt.Before(tasks[j].CreatedAt)
	})
	_ = ctx
	return tasks
}

func (s *QuotaLeaseDemoService) CompleteAccountLoginTask(ctx context.Context, req QuotaLeaseDemoAccountLoginTaskCompleteRequest) (*QuotaLeaseDemoAccountLoginTask, error) {
	if s.remoteMode() {
		return s.completeRemoteAccountLoginTask(ctx, req)
	}
	if s == nil || !s.Enabled() {
		return nil, ErrQuotaLeaseDemoDisabled
	}
	taskID := strings.TrimSpace(req.TaskID)
	nodeID := strings.TrimSpace(req.NodeID)
	if taskID == "" {
		return nil, fmt.Errorf("%w: task_id is required", ErrQuotaLeaseDemoInvalidInput)
	}
	now := time.Now().UTC()

	s.mu.Lock()
	defer s.mu.Unlock()

	task := s.accountTasks[taskID]
	if task == nil {
		return nil, fmt.Errorf("%w: account login task not found", ErrQuotaLeaseDemoInvalidInput)
	}
	if nodeID == "" {
		nodeID = task.AssignedNodeID
	}
	if task.AssignedNodeID != nodeID {
		return nil, fmt.Errorf("%w: account login task node mismatch", ErrQuotaLeaseDemoInvalidInput)
	}

	task.Error = strings.TrimSpace(req.Error)
	if task.Error != "" {
		task.Status = QuotaLeaseDemoAccountTaskFailed
		task.UpdatedAt = now
		return cloneQuotaLeaseDemoAccountLoginTask(task), nil
	}

	account := normalizeQuotaLeaseDemoAccountSnapshot(req.Account, task, now)
	if account.ID <= 0 || account.Platform == "" || account.Type == "" {
		return nil, fmt.Errorf("%w: account snapshot is incomplete", ErrQuotaLeaseDemoInvalidInput)
	}
	task.Account = &account
	task.Status = QuotaLeaseDemoAccountTaskCompleted
	task.UpdatedAt = now
	task.CompletedAt = &now

	if s.assignedAccounts == nil {
		s.assignedAccounts = make(map[int64]*QuotaLeaseDemoAssignedAccount)
	}
	existing := s.assignedAccounts[account.ID]
	createdAt := now
	if existing != nil {
		createdAt = existing.CreatedAt
	}
	s.assignedAccounts[account.ID] = &QuotaLeaseDemoAssignedAccount{
		NodeID:    task.AssignedNodeID,
		TaskID:    task.ID,
		Account:   account,
		CreatedAt: createdAt,
		UpdatedAt: now,
	}
	_ = ctx
	return cloneQuotaLeaseDemoAccountLoginTask(task), nil
}

func (s *QuotaLeaseDemoService) ReportAccountLoginTaskProgress(ctx context.Context, req QuotaLeaseDemoAccountLoginTaskProgressRequest) (*QuotaLeaseDemoAccountLoginTask, error) {
	if s.remoteMode() {
		return s.reportRemoteAccountLoginTaskProgress(ctx, req)
	}
	return s.reportAccountLoginTaskProgressLocal(ctx, req)
}

func (s *QuotaLeaseDemoService) reportAccountLoginTaskProgressLocal(ctx context.Context, req QuotaLeaseDemoAccountLoginTaskProgressRequest) (*QuotaLeaseDemoAccountLoginTask, error) {
	if s == nil || !s.Enabled() {
		return nil, ErrQuotaLeaseDemoDisabled
	}
	taskID := strings.TrimSpace(req.TaskID)
	nodeID := strings.TrimSpace(req.NodeID)
	status := strings.TrimSpace(req.Status)
	if taskID == "" || status == "" {
		return nil, fmt.Errorf("%w: task_id and status are required", ErrQuotaLeaseDemoInvalidInput)
	}
	if !quotaLeaseDemoAccountTaskProgressStatus(status) {
		return nil, fmt.Errorf("%w: unsupported account login task progress status", ErrQuotaLeaseDemoInvalidInput)
	}
	now := time.Now().UTC()

	s.mu.Lock()
	defer s.mu.Unlock()

	task := s.accountTasks[taskID]
	if task == nil {
		return nil, fmt.Errorf("%w: account login task not found", ErrQuotaLeaseDemoInvalidInput)
	}
	if nodeID == "" {
		nodeID = task.AssignedNodeID
	}
	if task.AssignedNodeID != nodeID {
		return nil, fmt.Errorf("%w: account login task node mismatch", ErrQuotaLeaseDemoInvalidInput)
	}
	task.Status = status
	task.Error = strings.TrimSpace(req.Error)
	task.LoginPayload = mergeQuotaLeaseDemoAnyPatch(task.LoginPayload, req.LoginPayloadPatch)
	task.Metadata = mergeQuotaLeaseDemoStringPatch(task.Metadata, req.MetadataPatch)
	task.UpdatedAt = now

	_ = ctx
	return cloneQuotaLeaseDemoAccountLoginTask(task), nil
}

func (s *QuotaLeaseDemoService) SubmitAccountLoginTaskCallback(ctx context.Context, req QuotaLeaseDemoAccountLoginTaskCallbackRequest) (*QuotaLeaseDemoAccountLoginTask, error) {
	if s.remoteMode() {
		return s.submitRemoteAccountLoginTaskCallback(ctx, req)
	}
	return s.submitAccountLoginTaskCallbackLocal(ctx, req)
}

func (s *QuotaLeaseDemoService) submitAccountLoginTaskCallbackLocal(ctx context.Context, req QuotaLeaseDemoAccountLoginTaskCallbackRequest) (*QuotaLeaseDemoAccountLoginTask, error) {
	if s == nil || !s.Enabled() {
		return nil, ErrQuotaLeaseDemoDisabled
	}
	taskID := strings.TrimSpace(req.TaskID)
	if taskID == "" {
		return nil, fmt.Errorf("%w: task_id is required", ErrQuotaLeaseDemoInvalidInput)
	}
	patch := cloneQuotaLeaseDemoAnyMap(req.Payload)
	if patch == nil {
		patch = make(map[string]any)
	}
	if code := strings.TrimSpace(req.Code); code != "" {
		patch["code"] = code
	}
	if state := strings.TrimSpace(req.State); state != "" {
		patch["state"] = state
	}
	if sessionID := strings.TrimSpace(req.SessionID); sessionID != "" {
		patch["session_id"] = sessionID
	}
	if redirectURI := strings.TrimSpace(req.RedirectURI); redirectURI != "" {
		patch["redirect_uri"] = redirectURI
	}
	if callbackURL := strings.TrimSpace(req.CallbackURL); callbackURL != "" {
		patch["callback_url"] = callbackURL
	}
	if req.ProxyID != nil {
		patch["proxy_id"] = *req.ProxyID
	}
	if len(patch) == 0 {
		return nil, fmt.Errorf("%w: callback payload is required", ErrQuotaLeaseDemoInvalidInput)
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	task := s.accountTasks[taskID]
	if task == nil {
		return nil, fmt.Errorf("%w: account login task not found", ErrQuotaLeaseDemoInvalidInput)
	}
	task.LoginPayload = mergeQuotaLeaseDemoAnyPatch(task.LoginPayload, patch)
	task.Status = QuotaLeaseDemoAccountTaskReady
	task.Error = ""
	task.UpdatedAt = time.Now().UTC()

	_ = ctx
	return cloneQuotaLeaseDemoAccountLoginTask(task), nil
}

func (s *QuotaLeaseDemoService) ReportAccountStatus(ctx context.Context, req QuotaLeaseDemoAccountStatusReportRequest) (*QuotaLeaseDemoAssignedAccount, error) {
	if s.remoteMode() {
		return s.reportRemoteAccountStatus(ctx, req)
	}
	return s.reportAccountStatusLocal(ctx, req)
}

func (s *QuotaLeaseDemoService) reportAccountStatusLocal(ctx context.Context, req QuotaLeaseDemoAccountStatusReportRequest) (*QuotaLeaseDemoAssignedAccount, error) {
	if s == nil || !s.Enabled() {
		return nil, ErrQuotaLeaseDemoDisabled
	}
	nodeID := strings.TrimSpace(req.NodeID)
	if req.AccountID <= 0 {
		return nil, fmt.Errorf("%w: account_id is required", ErrQuotaLeaseDemoInvalidInput)
	}
	reportedAt := req.ReportedAt
	if reportedAt.IsZero() {
		reportedAt = time.Now().UTC()
	} else {
		reportedAt = reportedAt.UTC()
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	assigned := s.assignedAccounts[req.AccountID]
	if assigned == nil {
		return nil, fmt.Errorf("%w: assigned account not found", ErrQuotaLeaseDemoInvalidInput)
	}
	if nodeID != "" && assigned.NodeID != nodeID {
		return nil, fmt.Errorf("%w: account node mismatch", ErrQuotaLeaseDemoInvalidInput)
	}

	account := assigned.Account
	if status := strings.TrimSpace(req.Status); status != "" {
		account.Status = status
	}
	if req.Schedulable != nil {
		account.Schedulable = *req.Schedulable
	}
	if req.ErrorMessage != nil {
		account.ErrorMessage = strings.TrimSpace(*req.ErrorMessage)
	}
	account.Credentials = mergeQuotaLeaseDemoAnyPatch(account.Credentials, req.CredentialsPatch)
	account.Extra = mergeQuotaLeaseDemoAnyPatch(account.Extra, req.ExtraPatch)
	if req.ClearRateLimitResetAt {
		account.RateLimitResetAt = nil
	} else if req.RateLimitResetAt != nil {
		resetAt := req.RateLimitResetAt.UTC()
		account.RateLimitResetAt = &resetAt
	}
	if req.ClearTempUnschedulable {
		account.TempUnschedulableUntil = nil
		account.TempUnschedulableReason = ""
	} else if req.TempUnschedulableUntil != nil {
		until := req.TempUnschedulableUntil.UTC()
		account.TempUnschedulableUntil = &until
		if req.TempUnschedulableReason != nil {
			account.TempUnschedulableReason = strings.TrimSpace(*req.TempUnschedulableReason)
		}
	} else if req.TempUnschedulableReason != nil {
		account.TempUnschedulableReason = strings.TrimSpace(*req.TempUnschedulableReason)
	}
	account.UpdatedAt = reportedAt

	assigned.Account = account
	assigned.UpdatedAt = reportedAt
	if assigned.TaskID != "" {
		if task := s.accountTasks[assigned.TaskID]; task != nil && task.Account != nil {
			task.Account = &account
			task.UpdatedAt = reportedAt
		}
	}

	_ = ctx
	return cloneQuotaLeaseDemoAssignedAccount(assigned), nil
}

func (s *QuotaLeaseDemoService) ListAssignedAccounts(ctx context.Context, nodeID string) []QuotaLeaseDemoAssignedAccount {
	if s == nil || !s.Enabled() {
		return nil
	}
	nodeID = strings.TrimSpace(nodeID)
	s.mu.Lock()
	defer s.mu.Unlock()

	accounts := make([]QuotaLeaseDemoAssignedAccount, 0, len(s.assignedAccounts))
	for _, assigned := range s.assignedAccounts {
		if assigned == nil {
			continue
		}
		if nodeID != "" && assigned.NodeID != nodeID {
			continue
		}
		if cloned := cloneQuotaLeaseDemoAssignedAccount(assigned); cloned != nil {
			accounts = append(accounts, *cloned)
		}
	}
	sort.Slice(accounts, func(i, j int) bool {
		return accounts[i].Account.ID < accounts[j].Account.ID
	})
	_ = ctx
	return accounts
}

func (s *QuotaLeaseDemoService) AssignedAccountsForScheduling(ctx context.Context, groupID *int64, platform string) ([]Account, bool) {
	if s == nil || !s.remoteMode() {
		return nil, false
	}
	if err := s.SyncAssignedAccounts(ctx); err != nil {
		slog.Warn("quota_lease_demo.assigned_accounts_sync_failed",
			"node_id", s.activeNodeID(),
			"error", err,
		)
	}
	nodeID := s.activeNodeID()
	assigned := s.ListAssignedAccounts(ctx, nodeID)
	accounts := make([]Account, 0, len(assigned))
	skippedPlatform := 0
	skippedUnschedulable := 0
	skippedGroup := 0
	for _, item := range assigned {
		account := quotaLeaseDemoAccountSnapshotToAccount(item.Account)
		if reason := quotaLeaseDemoAccountSchedulingSkipReason(account, groupID, platform); reason != "" {
			switch reason {
			case "platform":
				skippedPlatform++
			case "schedulable":
				skippedUnschedulable++
			case "group":
				skippedGroup++
			}
			continue
		}
		accounts = append(accounts, account)
	}
	if len(assigned) == 0 {
		slog.Warn("quota_lease_demo.assigned_accounts_empty",
			"node_id", nodeID,
			"configured_node_id", s.NodeID(),
			"platform", strings.TrimSpace(platform),
			"group_id", quotaLeaseDemoLogGroupID(groupID),
		)
	}
	if len(assigned) > 0 && len(accounts) == 0 {
		slog.Warn("quota_lease_demo.assigned_accounts_filtered",
			"node_id", nodeID,
			"configured_node_id", s.NodeID(),
			"platform", strings.TrimSpace(platform),
			"group_id", quotaLeaseDemoLogGroupID(groupID),
			"assigned_count", len(assigned),
			"skipped_platform", skippedPlatform,
			"skipped_unschedulable", skippedUnschedulable,
			"skipped_group", skippedGroup,
		)
	}
	return accounts, true
}

func (s *QuotaLeaseDemoService) AssignedAccountByID(ctx context.Context, accountID int64) (*Account, bool) {
	if s == nil || !s.remoteMode() || accountID <= 0 {
		return nil, false
	}
	_ = s.SyncAssignedAccounts(ctx)
	nodeID := s.activeNodeID()
	s.mu.Lock()
	defer s.mu.Unlock()
	assigned := s.assignedAccounts[accountID]
	if assigned == nil || assigned.NodeID != nodeID {
		return nil, true
	}
	account := quotaLeaseDemoAccountSnapshotToAccount(assigned.Account)
	return &account, true
}

func normalizeQuotaLeaseDemoAccountSnapshot(account QuotaLeaseDemoAccountSnapshot, task *QuotaLeaseDemoAccountLoginTask, now time.Time) QuotaLeaseDemoAccountSnapshot {
	if account.ID <= 0 && task != nil {
		account.ID = task.AccountID
	}
	if strings.TrimSpace(account.Name) == "" && task != nil {
		account.Name = task.Name
	}
	account.Name = strings.TrimSpace(account.Name)
	if strings.TrimSpace(account.Platform) == "" && task != nil {
		account.Platform = task.Platform
	}
	account.Platform = strings.TrimSpace(account.Platform)
	if strings.TrimSpace(account.Type) == "" && task != nil {
		account.Type = task.Type
	}
	account.Type = strings.TrimSpace(account.Type)
	if strings.TrimSpace(account.Status) == "" {
		account.Status = StatusActive
	}
	if account.Concurrency <= 0 {
		if task != nil && task.Concurrency > 0 {
			account.Concurrency = task.Concurrency
		} else {
			account.Concurrency = 1
		}
	}
	if account.GroupIDs == nil && task != nil {
		account.GroupIDs = cloneQuotaLeaseDemoInt64Slice(task.GroupIDs)
	} else {
		account.GroupIDs = cloneQuotaLeaseDemoInt64Slice(account.GroupIDs)
	}
	if account.Priority == 0 && task != nil {
		account.Priority = task.Priority
	}
	if !account.Schedulable && account.Status == StatusActive {
		account.Schedulable = true
	}
	account.Credentials = cloneQuotaLeaseDemoAnyMap(account.Credentials)
	account.Extra = cloneQuotaLeaseDemoAnyMap(account.Extra)
	account.ProxyID = cloneQuotaLeaseDemoInt64Ptr(account.ProxyID)
	account.Proxy = cloneQuotaLeaseDemoProxySnapshot(account.Proxy)
	if account.ProxyID == nil && account.Proxy != nil && account.Proxy.ID > 0 {
		account.ProxyID = &account.Proxy.ID
	}
	if account.UpdatedAt.IsZero() {
		account.UpdatedAt = now
	}
	return account
}

func quotaLeaseDemoAccountSnapshotToAccount(snapshot QuotaLeaseDemoAccountSnapshot) Account {
	account := Account{
		ID:                      snapshot.ID,
		Name:                    snapshot.Name,
		Platform:                snapshot.Platform,
		Type:                    snapshot.Type,
		Credentials:             cloneQuotaLeaseDemoAnyMap(snapshot.Credentials),
		Extra:                   cloneQuotaLeaseDemoAnyMap(snapshot.Extra),
		ProxyID:                 cloneQuotaLeaseDemoInt64Ptr(snapshot.ProxyID),
		Proxy:                   quotaLeaseDemoProxySnapshotToProxy(snapshot.Proxy),
		Status:                  snapshot.Status,
		ErrorMessage:            snapshot.ErrorMessage,
		Schedulable:             snapshot.Schedulable,
		Concurrency:             snapshot.Concurrency,
		Priority:                snapshot.Priority,
		GroupIDs:                cloneQuotaLeaseDemoInt64Slice(snapshot.GroupIDs),
		ExpiresAt:               snapshot.ExpiresAt,
		RateLimitResetAt:        snapshot.RateLimitResetAt,
		TempUnschedulableUntil:  snapshot.TempUnschedulableUntil,
		TempUnschedulableReason: snapshot.TempUnschedulableReason,
		UpdatedAt:               snapshot.UpdatedAt,
	}
	if account.Status == "" {
		account.Status = StatusActive
	}
	if account.Concurrency <= 0 {
		account.Concurrency = 1
	}
	if !account.Schedulable && account.Status == StatusActive {
		account.Schedulable = true
	}
	if account.ProxyID == nil && account.Proxy != nil && account.Proxy.ID > 0 {
		account.ProxyID = &account.Proxy.ID
	}
	return account
}

func quotaLeaseDemoAccountMatchesScheduling(account Account, groupID *int64, platform string) bool {
	return quotaLeaseDemoAccountSchedulingSkipReason(account, groupID, platform) == ""
}

func quotaLeaseDemoAccountSchedulingSkipReason(account Account, groupID *int64, platform string) string {
	platform = strings.TrimSpace(platform)
	if platform != "" && account.Platform != platform {
		return "platform"
	}
	if !account.IsSchedulable() {
		return "schedulable"
	}
	if groupID == nil {
		if len(account.GroupIDs) == 0 {
			return ""
		}
		return "group"
	}
	for _, id := range account.GroupIDs {
		if id == *groupID {
			return ""
		}
	}
	return "group"
}

func quotaLeaseDemoLogGroupID(groupID *int64) int64 {
	if groupID == nil {
		return 0
	}
	return *groupID
}

func cloneQuotaLeaseDemoAccountLoginTask(task *QuotaLeaseDemoAccountLoginTask) *QuotaLeaseDemoAccountLoginTask {
	if task == nil {
		return nil
	}
	value := *task
	value.LoginPayload = cloneQuotaLeaseDemoAnyMap(task.LoginPayload)
	value.Metadata = cloneQuotaLeaseDemoStringMap(task.Metadata)
	value.GroupIDs = cloneQuotaLeaseDemoInt64Slice(task.GroupIDs)
	if task.Account != nil {
		account := cloneQuotaLeaseDemoAccountSnapshot(*task.Account)
		value.Account = &account
	}
	if task.CompletedAt != nil {
		completedAt := *task.CompletedAt
		value.CompletedAt = &completedAt
	}
	return &value
}

func cloneQuotaLeaseDemoAssignedAccount(assigned *QuotaLeaseDemoAssignedAccount) *QuotaLeaseDemoAssignedAccount {
	if assigned == nil {
		return nil
	}
	value := *assigned
	value.Account = cloneQuotaLeaseDemoAccountSnapshot(assigned.Account)
	return &value
}

func cloneQuotaLeaseDemoAccountSnapshot(account QuotaLeaseDemoAccountSnapshot) QuotaLeaseDemoAccountSnapshot {
	account.Credentials = cloneQuotaLeaseDemoAnyMap(account.Credentials)
	account.Extra = cloneQuotaLeaseDemoAnyMap(account.Extra)
	account.ProxyID = cloneQuotaLeaseDemoInt64Ptr(account.ProxyID)
	account.Proxy = cloneQuotaLeaseDemoProxySnapshot(account.Proxy)
	account.GroupIDs = cloneQuotaLeaseDemoInt64Slice(account.GroupIDs)
	if account.ExpiresAt != nil {
		expiresAt := *account.ExpiresAt
		account.ExpiresAt = &expiresAt
	}
	if account.RateLimitResetAt != nil {
		rateLimitResetAt := *account.RateLimitResetAt
		account.RateLimitResetAt = &rateLimitResetAt
	}
	if account.TempUnschedulableUntil != nil {
		tempUnschedulableUntil := *account.TempUnschedulableUntil
		account.TempUnschedulableUntil = &tempUnschedulableUntil
	}
	return account
}

func cloneQuotaLeaseDemoProxySnapshot(proxy *QuotaLeaseDemoProxySnapshot) *QuotaLeaseDemoProxySnapshot {
	if proxy == nil {
		return nil
	}
	value := *proxy
	value.BackupProxyID = cloneQuotaLeaseDemoInt64Ptr(proxy.BackupProxyID)
	if proxy.ExpiresAt != nil {
		expiresAt := *proxy.ExpiresAt
		value.ExpiresAt = &expiresAt
	}
	return &value
}

func quotaLeaseDemoProxySnapshotToProxy(snapshot *QuotaLeaseDemoProxySnapshot) *Proxy {
	if snapshot == nil {
		return nil
	}
	return &Proxy{
		ID:             snapshot.ID,
		Name:           strings.TrimSpace(snapshot.Name),
		Protocol:       strings.TrimSpace(snapshot.Protocol),
		Host:           strings.TrimSpace(snapshot.Host),
		Port:           snapshot.Port,
		Username:       strings.TrimSpace(snapshot.Username),
		Password:       snapshot.Password,
		Status:         strings.TrimSpace(snapshot.Status),
		ExpiresAt:      cloneQuotaLeaseDemoTimePtr(snapshot.ExpiresAt),
		FallbackMode:   strings.TrimSpace(snapshot.FallbackMode),
		BackupProxyID:  cloneQuotaLeaseDemoInt64Ptr(snapshot.BackupProxyID),
		ExpiryWarnDays: snapshot.ExpiryWarnDays,
	}
}

func cloneQuotaLeaseDemoAnyMap(src map[string]any) map[string]any {
	if len(src) == 0 {
		return nil
	}
	dst := make(map[string]any, len(src))
	for k, v := range src {
		key := strings.TrimSpace(k)
		if key == "" {
			continue
		}
		dst[key] = v
	}
	if len(dst) == 0 {
		return nil
	}
	return dst
}

func mergeQuotaLeaseDemoAnyPatch(dst map[string]any, patch map[string]any) map[string]any {
	if len(patch) == 0 {
		return cloneQuotaLeaseDemoAnyMap(dst)
	}
	merged := cloneQuotaLeaseDemoAnyMap(dst)
	if merged == nil {
		merged = make(map[string]any)
	}
	for k, v := range patch {
		key := strings.TrimSpace(k)
		if key == "" {
			continue
		}
		if v == nil {
			delete(merged, key)
			continue
		}
		merged[key] = v
	}
	if len(merged) == 0 {
		return nil
	}
	return merged
}

func mergeQuotaLeaseDemoStringPatch(dst map[string]string, patch map[string]string) map[string]string {
	if len(patch) == 0 {
		return cloneQuotaLeaseDemoStringMap(dst)
	}
	merged := cloneQuotaLeaseDemoStringMap(dst)
	if merged == nil {
		merged = make(map[string]string)
	}
	for k, v := range patch {
		key := strings.TrimSpace(k)
		if key == "" {
			continue
		}
		value := strings.TrimSpace(v)
		if value == "" {
			delete(merged, key)
			continue
		}
		merged[key] = value
	}
	if len(merged) == 0 {
		return nil
	}
	return merged
}

func quotaLeaseDemoAccountTaskProgressStatus(status string) bool {
	switch strings.TrimSpace(status) {
	case QuotaLeaseDemoAccountTaskPending, QuotaLeaseDemoAccountTaskWaiting, QuotaLeaseDemoAccountTaskReady, QuotaLeaseDemoAccountTaskFailed:
		return true
	default:
		return false
	}
}

func cloneQuotaLeaseDemoInt64Slice(src []int64) []int64 {
	if len(src) == 0 {
		return nil
	}
	dst := make([]int64, 0, len(src))
	seen := make(map[int64]struct{}, len(src))
	for _, value := range src {
		if value <= 0 {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		dst = append(dst, value)
	}
	if len(dst) == 0 {
		return nil
	}
	sort.Slice(dst, func(i, j int) bool {
		return dst[i] < dst[j]
	})
	return dst
}

func cloneQuotaLeaseDemoInt64Ptr(src *int64) *int64 {
	if src == nil {
		return nil
	}
	value := *src
	return &value
}

func cloneQuotaLeaseDemoTimePtr(src *time.Time) *time.Time {
	if src == nil {
		return nil
	}
	value := *src
	return &value
}
