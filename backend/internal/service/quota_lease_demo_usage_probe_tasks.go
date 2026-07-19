package service

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/google/uuid"
)

const (
	QuotaLeaseDemoUsageProbeKindAccountUsage = "account_usage"
	QuotaLeaseDemoUsageProbeKindOpenAIQuota  = "openai_quota"
	QuotaLeaseDemoUsageProbeKindGrokQuota    = "grok_quota"

	quotaLeaseDemoUsageProbeWaitTimeout  = 60 * time.Second
	quotaLeaseDemoUsageProbePollInterval = 500 * time.Millisecond
)

type QuotaLeaseDemoUsageProbeTaskCreateRequest struct {
	AccountID      int64             `json:"account_id"`
	AssignedNodeID string            `json:"assigned_node_id"`
	Platform       string            `json:"platform,omitempty"`
	Source         string            `json:"source,omitempty"`
	Force          bool              `json:"force"`
	ProbeKind      string            `json:"probe_kind,omitempty"`
	Metadata       map[string]string `json:"metadata,omitempty"`
}

type QuotaLeaseDemoUsageProbeTaskCompleteRequest struct {
	TaskID      string                `json:"task_id"`
	NodeID      string                `json:"node_id,omitempty"`
	Error       string                `json:"error,omitempty"`
	Usage       *UsageInfo            `json:"usage,omitempty"`
	OpenAIQuota *OpenAIQuotaUsage     `json:"openai_quota,omitempty"`
	GrokQuota   *GrokQuotaProbeResult `json:"grok_quota,omitempty"`
	ExtraPatch  map[string]any        `json:"extra_patch,omitempty"`
}

type QuotaLeaseDemoUsageProbeTask struct {
	ID             string                `json:"id"`
	AccountID      int64                 `json:"account_id"`
	AssignedNodeID string                `json:"assigned_node_id"`
	Platform       string                `json:"platform,omitempty"`
	Source         string                `json:"source,omitempty"`
	Force          bool                  `json:"force"`
	ProbeKind      string                `json:"probe_kind"`
	Metadata       map[string]string     `json:"metadata,omitempty"`
	Status         string                `json:"status"`
	Error          string                `json:"error,omitempty"`
	Usage          *UsageInfo            `json:"usage,omitempty"`
	OpenAIQuota    *OpenAIQuotaUsage     `json:"openai_quota,omitempty"`
	GrokQuota      *GrokQuotaProbeResult `json:"grok_quota,omitempty"`
	ExtraPatch     map[string]any        `json:"extra_patch,omitempty"`
	CreatedAt      time.Time             `json:"created_at"`
	UpdatedAt      time.Time             `json:"updated_at"`
	CompletedAt    *time.Time            `json:"completed_at,omitempty"`
}

type QuotaLeaseDemoUsageProbeResult struct {
	Usage       *UsageInfo
	OpenAIQuota *OpenAIQuotaUsage
	GrokQuota   *GrokQuotaProbeResult
	ExtraPatch  map[string]any
}

func (s *QuotaLeaseDemoService) CreateUsageProbeTask(ctx context.Context, req QuotaLeaseDemoUsageProbeTaskCreateRequest) (*QuotaLeaseDemoUsageProbeTask, error) {
	if s.remoteMode() {
		return s.createRemoteUsageProbeTask(ctx, req)
	}
	if s == nil || !s.Enabled() {
		return nil, ErrQuotaLeaseDemoDisabled
	}
	nodeID := strings.TrimSpace(req.AssignedNodeID)
	if nodeID == "" {
		return nil, fmt.Errorf("%w: assigned_node_id is required", ErrQuotaLeaseDemoInvalidInput)
	}
	if req.AccountID <= 0 {
		return nil, fmt.Errorf("%w: account_id is required", ErrQuotaLeaseDemoInvalidInput)
	}
	kind, err := normalizeQuotaLeaseDemoUsageProbeKind(req.ProbeKind)
	if err != nil {
		return nil, err
	}
	source := strings.TrimSpace(req.Source)
	if source == "" {
		source = "active"
	}
	now := time.Now().UTC()
	task := &QuotaLeaseDemoUsageProbeTask{
		ID:             "ql_usage_probe_" + uuidStringWithoutDash(),
		AccountID:      req.AccountID,
		AssignedNodeID: nodeID,
		Platform:       strings.TrimSpace(req.Platform),
		Source:         source,
		Force:          req.Force,
		ProbeKind:      kind,
		Metadata:       cloneQuotaLeaseDemoStringMap(req.Metadata),
		Status:         QuotaLeaseDemoAccountTaskPending,
		CreatedAt:      now,
		UpdatedAt:      now,
	}

	s.mu.Lock()
	if s.usageProbeTasks == nil {
		s.usageProbeTasks = make(map[string]*QuotaLeaseDemoUsageProbeTask)
	}
	s.usageProbeTasks[task.ID] = task
	s.mu.Unlock()
	_ = ctx
	return cloneQuotaLeaseDemoUsageProbeTask(task), nil
}

func (s *QuotaLeaseDemoService) ListUsageProbeTasks(ctx context.Context, nodeID, status string) []QuotaLeaseDemoUsageProbeTask {
	if s.remoteMode() {
		tasks, err := s.fetchRemoteUsageProbeTasks(ctx, status)
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

	tasks := make([]QuotaLeaseDemoUsageProbeTask, 0, len(s.usageProbeTasks))
	for _, task := range s.usageProbeTasks {
		if task == nil {
			continue
		}
		if nodeID != "" && task.AssignedNodeID != nodeID {
			continue
		}
		if status != "" && task.Status != status {
			continue
		}
		if cloned := cloneQuotaLeaseDemoUsageProbeTask(task); cloned != nil {
			tasks = append(tasks, *cloned)
		}
	}
	sort.Slice(tasks, func(i, j int) bool {
		return tasks[i].CreatedAt.Before(tasks[j].CreatedAt)
	})
	_ = ctx
	return tasks
}

func (s *QuotaLeaseDemoService) CompleteUsageProbeTask(ctx context.Context, req QuotaLeaseDemoUsageProbeTaskCompleteRequest) (*QuotaLeaseDemoUsageProbeTask, error) {
	if s.remoteMode() {
		return s.completeRemoteUsageProbeTask(ctx, req)
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

	task := s.usageProbeTasks[taskID]
	if task == nil {
		return nil, fmt.Errorf("%w: usage probe task not found", ErrQuotaLeaseDemoInvalidInput)
	}
	if nodeID == "" {
		nodeID = task.AssignedNodeID
	}
	if task.AssignedNodeID != nodeID {
		return nil, fmt.Errorf("%w: usage probe task node mismatch", ErrQuotaLeaseDemoInvalidInput)
	}

	task.Error = strings.TrimSpace(req.Error)
	task.UpdatedAt = now
	task.CompletedAt = &now
	if task.Error != "" {
		task.Status = QuotaLeaseDemoAccountTaskFailed
		return cloneQuotaLeaseDemoUsageProbeTask(task), nil
	}

	task.Status = QuotaLeaseDemoAccountTaskCompleted
	task.Usage = cloneQuotaLeaseDemoUsageInfo(req.Usage)
	task.OpenAIQuota = cloneQuotaLeaseDemoOpenAIQuotaUsage(req.OpenAIQuota)
	task.GrokQuota = cloneQuotaLeaseDemoGrokQuotaProbeResult(req.GrokQuota)
	task.ExtraPatch = quotaLeaseDemoAllowedUsageProbeExtraPatch(req.ExtraPatch)
	s.mergeUsageProbeExtraPatchIntoAssignedAccountLocked(task, now)
	return cloneQuotaLeaseDemoUsageProbeTask(task), nil
}

func (s *QuotaLeaseDemoService) WaitUsageProbeTask(ctx context.Context, taskID string, timeout, pollInterval time.Duration) (*QuotaLeaseDemoUsageProbeTask, error) {
	if s == nil || !s.Enabled() {
		return nil, ErrQuotaLeaseDemoDisabled
	}
	taskID = strings.TrimSpace(taskID)
	if taskID == "" {
		return nil, fmt.Errorf("%w: task_id is required", ErrQuotaLeaseDemoInvalidInput)
	}
	if timeout <= 0 {
		timeout = quotaLeaseDemoUsageProbeWaitTimeout
	}
	if pollInterval <= 0 {
		pollInterval = quotaLeaseDemoUsageProbePollInterval
	}
	if ctx == nil {
		ctx = context.Background()
	}
	waitCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	ticker := time.NewTicker(pollInterval)
	defer ticker.Stop()
	for {
		task, err := s.getUsageProbeTask(waitCtx, taskID)
		if err != nil {
			return nil, err
		}
		switch task.Status {
		case QuotaLeaseDemoAccountTaskCompleted:
			return task, nil
		case QuotaLeaseDemoAccountTaskFailed:
			message := strings.TrimSpace(task.Error)
			if message == "" {
				message = "usage probe task failed"
			}
			return task, infraerrors.New(http.StatusBadGateway, "QUOTA_LEASE_DEMO_USAGE_PROBE_FAILED", message)
		}

		select {
		case <-waitCtx.Done():
			return task, infraerrors.New(http.StatusGatewayTimeout, "QUOTA_LEASE_DEMO_USAGE_PROBE_TIMEOUT", "usage probe task timed out")
		case <-ticker.C:
		}
	}
}

func (s *QuotaLeaseDemoService) createRemoteUsageProbeTask(ctx context.Context, req QuotaLeaseDemoUsageProbeTaskCreateRequest) (*QuotaLeaseDemoUsageProbeTask, error) {
	var result struct {
		Task *QuotaLeaseDemoUsageProbeTask `json:"task"`
	}
	if err := s.doRemoteJSON(ctx, http.MethodPost, "/accounts/usage-probe-tasks", "", s.ControlPlaneKey(), req, &result); err != nil {
		return nil, err
	}
	if result.Task == nil {
		return nil, fmt.Errorf("%w: usage probe task response missing task", ErrQuotaLeaseDemoInvalidInput)
	}
	return result.Task, nil
}

func (s *QuotaLeaseDemoService) fetchRemoteUsageProbeTasks(ctx context.Context, status string) ([]QuotaLeaseDemoUsageProbeTask, error) {
	nodeID, secret, err := s.remoteNodeAuth(ctx)
	if err != nil {
		return nil, err
	}
	endpoint := "/accounts/usage-probe-tasks"
	if strings.TrimSpace(status) != "" {
		endpoint += "?status=" + url.QueryEscape(strings.TrimSpace(status))
	}
	var result struct {
		Tasks []QuotaLeaseDemoUsageProbeTask `json:"tasks"`
	}
	if err := s.doRemoteJSON(ctx, http.MethodGet, endpoint, nodeID, secret, nil, &result); err != nil {
		return nil, err
	}
	return result.Tasks, nil
}

func (s *QuotaLeaseDemoService) completeRemoteUsageProbeTask(ctx context.Context, req QuotaLeaseDemoUsageProbeTaskCompleteRequest) (*QuotaLeaseDemoUsageProbeTask, error) {
	nodeID, secret, err := s.remoteNodeAuth(ctx)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(req.NodeID) == "" {
		req.NodeID = nodeID
	}
	if strings.TrimSpace(req.TaskID) == "" {
		return nil, fmt.Errorf("%w: task_id is required", ErrQuotaLeaseDemoInvalidInput)
	}
	var result struct {
		Task *QuotaLeaseDemoUsageProbeTask `json:"task"`
	}
	endpoint := "/accounts/usage-probe-tasks/" + url.PathEscape(strings.TrimSpace(req.TaskID)) + "/complete"
	if err := s.doRemoteJSON(ctx, http.MethodPost, endpoint, nodeID, secret, req, &result); err != nil {
		return nil, err
	}
	if result.Task == nil {
		return nil, fmt.Errorf("%w: usage probe completion response missing task", ErrQuotaLeaseDemoInvalidInput)
	}
	return result.Task, nil
}

func (s *QuotaLeaseDemoService) getUsageProbeTask(ctx context.Context, taskID string) (*QuotaLeaseDemoUsageProbeTask, error) {
	if s == nil || !s.Enabled() {
		return nil, ErrQuotaLeaseDemoDisabled
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	task := s.usageProbeTasks[strings.TrimSpace(taskID)]
	if task == nil {
		return nil, fmt.Errorf("%w: usage probe task not found", ErrQuotaLeaseDemoInvalidInput)
	}
	_ = ctx
	return cloneQuotaLeaseDemoUsageProbeTask(task), nil
}

func (s *QuotaLeaseDemoService) mergeUsageProbeExtraPatchIntoAssignedAccountLocked(task *QuotaLeaseDemoUsageProbeTask, now time.Time) {
	if s == nil || task == nil || len(task.ExtraPatch) == 0 {
		return
	}
	assigned := s.assignedAccounts[task.AccountID]
	if assigned == nil || assigned.NodeID != task.AssignedNodeID {
		return
	}
	account := assigned.Account
	account.Extra = mergeQuotaLeaseDemoAnyPatch(account.Extra, task.ExtraPatch)
	account.UpdatedAt = now
	assigned.Account = account
	assigned.UpdatedAt = now
}

func normalizeQuotaLeaseDemoUsageProbeKind(kind string) (string, error) {
	kind = strings.TrimSpace(kind)
	if kind == "" {
		return QuotaLeaseDemoUsageProbeKindAccountUsage, nil
	}
	switch kind {
	case QuotaLeaseDemoUsageProbeKindAccountUsage, QuotaLeaseDemoUsageProbeKindOpenAIQuota, QuotaLeaseDemoUsageProbeKindGrokQuota:
		return kind, nil
	default:
		return "", fmt.Errorf("%w: unsupported usage probe kind", ErrQuotaLeaseDemoInvalidInput)
	}
}

func cloneQuotaLeaseDemoUsageProbeTask(task *QuotaLeaseDemoUsageProbeTask) *QuotaLeaseDemoUsageProbeTask {
	if task == nil {
		return nil
	}
	value := *task
	value.Metadata = cloneQuotaLeaseDemoStringMap(task.Metadata)
	value.Usage = cloneQuotaLeaseDemoUsageInfo(task.Usage)
	value.OpenAIQuota = cloneQuotaLeaseDemoOpenAIQuotaUsage(task.OpenAIQuota)
	value.GrokQuota = cloneQuotaLeaseDemoGrokQuotaProbeResult(task.GrokQuota)
	value.ExtraPatch = cloneQuotaLeaseDemoAnyMap(task.ExtraPatch)
	if task.CompletedAt != nil {
		completedAt := *task.CompletedAt
		value.CompletedAt = &completedAt
	}
	return &value
}

func quotaLeaseDemoAllowedUsageProbeExtraPatch(input map[string]any) map[string]any {
	if len(input) == 0 {
		return nil
	}
	allowed := map[string]struct{}{
		"codex_usage_updated_at":          {},
		"codex_5h_used_percent":           {},
		"codex_5h_reset_after_seconds":    {},
		"codex_5h_window_minutes":         {},
		"codex_5h_reset_at":               {},
		"codex_7d_used_percent":           {},
		"codex_7d_reset_after_seconds":    {},
		"codex_7d_window_minutes":         {},
		"codex_7d_reset_at":               {},
		"session_window_utilization":      {},
		"passive_usage_7d_utilization":    {},
		"passive_usage_7d_reset":          {},
		"passive_usage_7d_oi_utilization": {},
		"passive_usage_7d_oi_reset":       {},
		"passive_usage_sampled_at":        {},
		"grok_usage_snapshot":             {},
		"grok_billing_snapshot":           {},
		"antigravity_quota_snapshot":      {},
		"antigravity_quota_snapshot_at":   {},
		"antigravity_credits_snapshot":    {},
		"antigravity_credits_snapshot_at": {},
		"kiro_usage_snapshot":             {},
		"kiro_usage_snapshot_at":          {},
	}
	out := make(map[string]any, len(input))
	for key, value := range input {
		key = strings.TrimSpace(key)
		if _, ok := allowed[key]; !ok {
			continue
		}
		out[key] = value
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func quotaLeaseDemoUsageProbeExtraPatchFromAccountUsage(account *Account, usage *UsageInfo, now time.Time) map[string]any {
	if usage == nil {
		if account == nil {
			return nil
		}
		return quotaLeaseDemoAllowedUsageProbeExtraPatch(account.Extra)
	}
	patch := map[string]any{}
	if account != nil {
		patch = mergeQuotaLeaseDemoAnyPatch(patch, quotaLeaseDemoAllowedUsageProbeExtraPatch(account.Extra))
	}

	platform := ""
	if account != nil {
		platform = strings.TrimSpace(account.Platform)
	}
	switch platform {
	case PlatformOpenAI:
		stamp := now
		if usage.UpdatedAt != nil && !usage.UpdatedAt.IsZero() {
			stamp = usage.UpdatedAt.UTC()
		}
		if updates := quotaLeaseDemoCodexExtraPatchFromUsage(usage, stamp, now); len(updates) > 0 {
			patch = mergeQuotaLeaseDemoAnyPatch(patch, updates)
		}
	case PlatformGrok:
		if usage.GrokBilling != nil {
			if patch == nil {
				patch = map[string]any{}
			}
			patch[grokBillingExtraKey] = usage.GrokBilling
		}
	default:
		if updates := quotaLeaseDemoPassiveExtraPatchFromUsage(usage, now); len(updates) > 0 {
			patch = mergeQuotaLeaseDemoAnyPatch(patch, updates)
		}
	}
	return quotaLeaseDemoAllowedUsageProbeExtraPatch(patch)
}

func quotaLeaseDemoCodexExtraPatchFromUsage(usage *UsageInfo, stamp time.Time, now time.Time) map[string]any {
	if usage == nil {
		return nil
	}
	patch := map[string]any{}
	if usage.FiveHour != nil {
		patch["codex_5h_used_percent"] = usage.FiveHour.Utilization
		patch["codex_5h_window_minutes"] = 5 * 60
		quotaLeaseDemoSetResetFieldsFromUsageProgress(patch, "codex_5h", usage.FiveHour, now)
	}
	if usage.SevenDay != nil {
		patch["codex_7d_used_percent"] = usage.SevenDay.Utilization
		patch["codex_7d_window_minutes"] = 7 * 24 * 60
		quotaLeaseDemoSetResetFieldsFromUsageProgress(patch, "codex_7d", usage.SevenDay, now)
	}
	if len(patch) > 0 {
		if stamp.IsZero() {
			stamp = now
		}
		patch["codex_usage_updated_at"] = stamp.UTC().Format(time.RFC3339)
	}
	return patch
}

func quotaLeaseDemoSetResetFieldsFromUsageProgress(patch map[string]any, prefix string, progress *UsageProgress, now time.Time) {
	if patch == nil || progress == nil {
		return
	}
	if progress.ResetsAt != nil && !progress.ResetsAt.IsZero() {
		resetAt := progress.ResetsAt.UTC()
		patch[prefix+"_reset_at"] = resetAt.Format(time.RFC3339)
		remaining := int(resetAt.Sub(now).Seconds())
		if remaining < 0 {
			remaining = 0
		}
		patch[prefix+"_reset_after_seconds"] = remaining
		return
	}
	if progress.RemainingSeconds > 0 {
		patch[prefix+"_reset_after_seconds"] = progress.RemainingSeconds
	}
}

func quotaLeaseDemoPassiveExtraPatchFromUsage(usage *UsageInfo, now time.Time) map[string]any {
	if usage == nil {
		return nil
	}
	patch := map[string]any{}
	if usage.FiveHour != nil {
		patch["session_window_utilization"] = usage.FiveHour.Utilization / 100
	}
	if usage.SevenDay != nil {
		patch["passive_usage_7d_utilization"] = usage.SevenDay.Utilization / 100
		if usage.SevenDay.ResetsAt != nil && !usage.SevenDay.ResetsAt.IsZero() {
			patch["passive_usage_7d_reset"] = usage.SevenDay.ResetsAt.Unix()
		}
	}
	if usage.SevenDayFable != nil {
		patch["passive_usage_7d_oi_utilization"] = usage.SevenDayFable.Utilization / 100
		if usage.SevenDayFable.ResetsAt != nil && !usage.SevenDayFable.ResetsAt.IsZero() {
			patch["passive_usage_7d_oi_reset"] = usage.SevenDayFable.ResetsAt.Unix()
		}
	}
	if len(patch) > 0 {
		stamp := now.UTC()
		if usage.UpdatedAt != nil && !usage.UpdatedAt.IsZero() {
			stamp = usage.UpdatedAt.UTC()
		}
		patch["passive_usage_sampled_at"] = stamp.Format(time.RFC3339)
	}
	return patch
}

func quotaLeaseDemoUsageProbeExtraPatchFromOpenAIQuota(result *OpenAIQuotaUsage, now time.Time) map[string]any {
	return quotaLeaseDemoAllowedUsageProbeExtraPatch(buildCodexSparkWindowExtraUpdates(result, now))
}

func quotaLeaseDemoUsageProbeExtraPatchFromGrokQuota(result *GrokQuotaProbeResult) map[string]any {
	if result == nil {
		return nil
	}
	patch := map[string]any{}
	if result.Snapshot != nil {
		patch[grokQuotaSnapshotExtraKey] = result.Snapshot
	}
	if result.Billing != nil {
		patch[grokBillingExtraKey] = result.Billing
	}
	return quotaLeaseDemoAllowedUsageProbeExtraPatch(patch)
}

func cloneQuotaLeaseDemoUsageInfo(src *UsageInfo) *UsageInfo {
	if src == nil {
		return nil
	}
	var dst UsageInfo
	if cloneJSONSerializableValue(src, &dst) {
		return &dst
	}
	value := *src
	return &value
}

func cloneQuotaLeaseDemoOpenAIQuotaUsage(src *OpenAIQuotaUsage) *OpenAIQuotaUsage {
	if src == nil {
		return nil
	}
	var dst OpenAIQuotaUsage
	if cloneJSONSerializableValue(src, &dst) {
		return &dst
	}
	value := *src
	return &value
}

func cloneQuotaLeaseDemoGrokQuotaProbeResult(src *GrokQuotaProbeResult) *GrokQuotaProbeResult {
	if src == nil {
		return nil
	}
	var dst GrokQuotaProbeResult
	if cloneJSONSerializableValue(src, &dst) {
		return &dst
	}
	value := *src
	return &value
}

func cloneJSONSerializableValue(src any, dst any) bool {
	payload, err := json.Marshal(src)
	if err != nil {
		return false
	}
	return json.Unmarshal(payload, dst) == nil
}

func uuidStringWithoutDash() string {
	return strings.ReplaceAll(uuid.NewString(), "-", "")
}
