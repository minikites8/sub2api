package handler

import (
	"context"
	"errors"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
)

type QuotaLeaseDemoHandler struct {
	svc      *service.QuotaLeaseDemoService
	adminSvc service.AdminService
}

const (
	quotaLeaseDemoNodeOAuthLastSyncedAtKey             = "node_oauth_last_synced_at"
	quotaLeaseDemoNodeOAuthLastSyncedAtPersistInterval = 5 * time.Second
)

func NewQuotaLeaseDemoHandler(svc *service.QuotaLeaseDemoService, adminSvc ...service.AdminService) *QuotaLeaseDemoHandler {
	h := &QuotaLeaseDemoHandler{svc: svc}
	if len(adminSvc) > 0 {
		h.adminSvc = adminSvc[0]
	}
	return h
}

func (h *QuotaLeaseDemoHandler) RegisterNode(c *gin.Context) {
	if !h.requireEnabled(c) || !h.requireControlSecret(c) {
		return
	}
	var req service.QuotaLeaseDemoNodeRegistrationRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid_request", "message": err.Error()})
		return
	}
	result, err := h.svc.RegisterNode(c.Request.Context(), req)
	if err != nil {
		h.writeError(c, err)
		return
	}
	c.JSON(http.StatusOK, result)
}

func (h *QuotaLeaseDemoHandler) HeartbeatNode(c *gin.Context) {
	if !h.requireEnabled(c) {
		return
	}
	var req service.QuotaLeaseDemoNodeHeartbeatRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid_request", "message": err.Error()})
		return
	}
	nodeID, ok := h.authenticateNodeOrControl(c, req.NodeID)
	if !ok {
		return
	}
	if req.NodeID == "" {
		req.NodeID = nodeID
	}
	node, err := h.svc.HeartbeatNode(c.Request.Context(), req)
	if err != nil {
		h.writeError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"node": node})
}

func (h *QuotaLeaseDemoHandler) ListNodes(c *gin.Context) {
	if !h.requireEnabled(c) || !h.requireControlSecret(c) {
		return
	}
	c.JSON(http.StatusOK, gin.H{"nodes": h.svc.ListNodes()})
}

func (h *QuotaLeaseDemoHandler) CreateAccountLoginTask(c *gin.Context) {
	if !h.requireEnabled(c) || !h.requireControlSecret(c) {
		return
	}
	var req service.QuotaLeaseDemoAccountLoginTaskCreateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid_request", "message": err.Error()})
		return
	}
	task, err := h.svc.CreateAccountLoginTask(c.Request.Context(), req)
	if err != nil {
		h.writeError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"task": task})
}

func (h *QuotaLeaseDemoHandler) ListAccountLoginTasks(c *gin.Context) {
	if !h.requireEnabled(c) {
		return
	}
	requestedNodeID := strings.TrimSpace(c.Query("node_id"))
	nodeID, ok := h.authenticateNodeOrControl(c, requestedNodeID)
	if !ok {
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"tasks": h.svc.ListAccountLoginTasks(c.Request.Context(), nodeID, c.Query("status")),
	})
}

func (h *QuotaLeaseDemoHandler) CompleteAccountLoginTask(c *gin.Context) {
	if !h.requireEnabled(c) {
		return
	}
	var req service.QuotaLeaseDemoAccountLoginTaskCompleteRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid_request", "message": err.Error()})
		return
	}
	if req.TaskID == "" {
		req.TaskID = c.Param("task_id")
	}
	nodeID, ok := h.authenticateNodeOrControl(c, req.NodeID)
	if !ok {
		return
	}
	if req.NodeID == "" {
		req.NodeID = nodeID
	}
	task, err := h.svc.CompleteAccountLoginTask(c.Request.Context(), req)
	if err != nil {
		h.writeError(c, err)
		return
	}
	if err := h.syncCompletedAccount(c.Request.Context(), task); err != nil {
		h.writeError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"task": task})
}

func (h *QuotaLeaseDemoHandler) syncCompletedAccount(ctx context.Context, task *service.QuotaLeaseDemoAccountLoginTask) error {
	if h == nil || h.adminSvc == nil || task == nil || task.Status != service.QuotaLeaseDemoAccountTaskCompleted || task.Account == nil {
		return nil
	}
	account := task.Account
	if account.ID <= 0 {
		return nil
	}
	concurrency := account.Concurrency
	if concurrency <= 0 {
		concurrency = task.Concurrency
	}
	priority := account.Priority
	groupIDs := append([]int64(nil), account.GroupIDs...)
	extra := quotaLeaseDemoHandlerCompletedAccountExtra(account.Extra, task.AssignedNodeID)
	if strings.EqualFold(strings.TrimSpace(task.Metadata["source"]), "account_reauth_modal") {
		input := &service.UpdateAccountInput{
			Name:                  account.Name,
			Type:                  account.Type,
			Credentials:           account.Credentials,
			Concurrency:           &concurrency,
			Priority:              &priority,
			Status:                service.StatusActive,
			SkipMixedChannelCheck: true,
		}
		if len(groupIDs) > 0 {
			input.GroupIDs = &groupIDs
		}
		if _, err := h.adminSvc.UpdateAccount(ctx, account.ID, input); err != nil {
			return err
		}
		if len(extra) > 0 {
			if err := h.adminSvc.UpdateAccountExtra(ctx, account.ID, extra); err != nil {
				return err
			}
		}
		if _, err := h.adminSvc.ClearAccountError(ctx, account.ID); err != nil {
			return err
		}
		return nil
	}
	input := &service.UpdateAccountInput{
		Name:                  account.Name,
		Type:                  account.Type,
		Credentials:           account.Credentials,
		Extra:                 extra,
		Concurrency:           &concurrency,
		Priority:              &priority,
		Status:                service.StatusActive,
		SkipMixedChannelCheck: true,
	}
	if len(groupIDs) > 0 {
		input.GroupIDs = &groupIDs
	}
	_, err := h.adminSvc.UpdateAccount(ctx, account.ID, input)
	return err
}

func quotaLeaseDemoHandlerCompletedAccountExtra(extra map[string]any, nodeID string) map[string]any {
	out := quotaLeaseDemoHandlerCloneAnyMap(extra)
	if out == nil {
		out = map[string]any{}
	}
	out["node_oauth_status"] = service.QuotaLeaseDemoAccountTaskCompleted
	if nodeID = strings.TrimSpace(nodeID); nodeID != "" {
		out["node_oauth_assigned_node_id"] = nodeID
	}
	return out
}

func (h *QuotaLeaseDemoHandler) ReportAccountLoginTaskProgress(c *gin.Context) {
	if !h.requireEnabled(c) {
		return
	}
	var req service.QuotaLeaseDemoAccountLoginTaskProgressRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid_request", "message": err.Error()})
		return
	}
	if req.TaskID == "" {
		req.TaskID = c.Param("task_id")
	}
	nodeID, ok := h.authenticateNodeOrControl(c, req.NodeID)
	if !ok {
		return
	}
	if req.NodeID == "" {
		req.NodeID = nodeID
	}
	task, err := h.svc.ReportAccountLoginTaskProgress(c.Request.Context(), req)
	if err != nil {
		h.writeError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"task": task})
}

func (h *QuotaLeaseDemoHandler) SubmitAccountLoginTaskCallback(c *gin.Context) {
	if !h.requireEnabled(c) || !h.requireControlSecret(c) {
		return
	}
	var req service.QuotaLeaseDemoAccountLoginTaskCallbackRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid_request", "message": err.Error()})
		return
	}
	if req.TaskID == "" {
		req.TaskID = c.Param("task_id")
	}
	task, err := h.svc.SubmitAccountLoginTaskCallback(c.Request.Context(), req)
	if err != nil {
		h.writeError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"task": task})
}

func (h *QuotaLeaseDemoHandler) ReportAccountStatus(c *gin.Context) {
	if !h.requireEnabled(c) {
		return
	}
	var req service.QuotaLeaseDemoAccountStatusReportRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid_request", "message": err.Error()})
		return
	}
	nodeID, ok := h.authenticateNodeOrControl(c, req.NodeID)
	if !ok {
		return
	}
	if req.NodeID == "" {
		req.NodeID = nodeID
	}
	account, err := h.svc.ReportAccountStatus(c.Request.Context(), req)
	if err != nil {
		h.writeError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"account": account})
}

func (h *QuotaLeaseDemoHandler) ListAssignedAccounts(c *gin.Context) {
	if !h.requireEnabled(c) {
		return
	}
	requestedNodeID := strings.TrimSpace(c.Query("node_id"))
	nodeID, ok := h.authenticateNodeOrControl(c, requestedNodeID)
	if !ok {
		return
	}
	accounts, err := h.listAssignedAccounts(c.Request.Context(), nodeID)
	if err != nil {
		h.writeError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"accounts": accounts,
	})
}

func (h *QuotaLeaseDemoHandler) listAssignedAccounts(ctx context.Context, nodeID string) ([]service.QuotaLeaseDemoAssignedAccount, error) {
	accounts := h.svc.ListAssignedAccounts(ctx, nodeID)
	if h.adminSvc == nil {
		return accounts, nil
	}

	seen := make(map[int64]int, len(accounts))
	for index, assigned := range accounts {
		if assigned.Account.ID > 0 {
			seen[assigned.Account.ID] = index
		}
	}

	syncedAt := time.Now().UTC()
	const pageSize = 500
	for page := 1; ; page++ {
		items, total, err := h.adminSvc.ListAccounts(ctx, page, pageSize, "", service.AccountTypeOAuth, service.StatusActive, "", 0, "", "id", "asc")
		if err != nil {
			return nil, err
		}
		for _, account := range items {
			if account.ID <= 0 {
				continue
			}
			assignedNodeID := quotaLeaseDemoHandlerString(account.Extra["node_oauth_assigned_node_id"])
			if assignedNodeID == "" || (nodeID != "" && assignedNodeID != nodeID) {
				continue
			}
			if !quotaLeaseDemoHandlerPersistedAccountReady(account) {
				continue
			}
			snapshot := quotaLeaseDemoHandlerAccountSnapshot(account)
			snapshot.Extra = h.touchAssignedAccountSync(ctx, account.ID, account.Extra, snapshot.Extra, syncedAt)
			if existingIndex, ok := seen[account.ID]; ok {
				accounts[existingIndex].Account.Extra = snapshot.Extra
				continue
			}
			accounts = append(accounts, service.QuotaLeaseDemoAssignedAccount{
				NodeID:    assignedNodeID,
				Account:   snapshot,
				CreatedAt: quotaLeaseDemoHandlerTimeOrNow(account.CreatedAt),
				UpdatedAt: quotaLeaseDemoHandlerTimeOrNow(account.UpdatedAt),
			})
			seen[account.ID] = len(accounts) - 1
		}
		if len(items) == 0 || int64(page*pageSize) >= total {
			break
		}
	}

	sort.Slice(accounts, func(i, j int) bool {
		return accounts[i].Account.ID < accounts[j].Account.ID
	})
	return accounts, nil
}

func (h *QuotaLeaseDemoHandler) touchAssignedAccountSync(ctx context.Context, accountID int64, currentExtra, snapshotExtra map[string]any, syncedAt time.Time) map[string]any {
	if snapshotExtra == nil {
		snapshotExtra = map[string]any{}
	}
	stamp := syncedAt.Format(time.RFC3339Nano)
	previous := currentExtra[quotaLeaseDemoNodeOAuthLastSyncedAtKey]
	snapshotExtra[quotaLeaseDemoNodeOAuthLastSyncedAtKey] = stamp
	if h.adminSvc != nil && accountID > 0 && quotaLeaseDemoHandlerShouldPersistLastSyncedAt(previous, syncedAt) {
		_ = h.adminSvc.UpdateAccountExtra(ctx, accountID, map[string]any{
			quotaLeaseDemoNodeOAuthLastSyncedAtKey: stamp,
		})
	}
	return snapshotExtra
}

func quotaLeaseDemoHandlerShouldPersistLastSyncedAt(value any, syncedAt time.Time) bool {
	last, ok := quotaLeaseDemoHandlerTime(value)
	if !ok {
		return true
	}
	return syncedAt.Sub(last) >= quotaLeaseDemoNodeOAuthLastSyncedAtPersistInterval
}

func quotaLeaseDemoHandlerTime(value any) (time.Time, bool) {
	switch v := value.(type) {
	case time.Time:
		if v.IsZero() {
			return time.Time{}, false
		}
		return v, true
	case string:
		raw := strings.TrimSpace(v)
		if raw == "" {
			return time.Time{}, false
		}
		if parsed, err := time.Parse(time.RFC3339Nano, raw); err == nil {
			return parsed, true
		}
		if parsed, err := time.Parse(time.RFC3339, raw); err == nil {
			return parsed, true
		}
	}
	return time.Time{}, false
}

func quotaLeaseDemoHandlerPersistedAccountReady(account service.Account) bool {
	if account.Platform != service.PlatformOpenAI && account.Platform != service.PlatformGrok {
		return false
	}
	status := strings.TrimSpace(quotaLeaseDemoHandlerString(account.Extra["node_oauth_status"]))
	if status != "" && status != service.QuotaLeaseDemoAccountTaskCompleted {
		return false
	}
	if quotaLeaseDemoHandlerBool(account.Credentials["node_oauth_pending"]) {
		return false
	}
	return true
}

func quotaLeaseDemoHandlerAccountSnapshot(account service.Account) service.QuotaLeaseDemoAccountSnapshot {
	snapshot := service.QuotaLeaseDemoAccountSnapshot{
		ID:                      account.ID,
		Name:                    account.Name,
		Platform:                account.Platform,
		Type:                    account.Type,
		Credentials:             quotaLeaseDemoHandlerCloneAnyMap(account.Credentials),
		Extra:                   quotaLeaseDemoHandlerCloneAnyMap(account.Extra),
		Status:                  account.Status,
		ErrorMessage:            account.ErrorMessage,
		Schedulable:             account.Schedulable,
		Concurrency:             account.Concurrency,
		Priority:                account.Priority,
		GroupIDs:                quotaLeaseDemoHandlerCloneInt64Slice(account.GroupIDs),
		ExpiresAt:               quotaLeaseDemoHandlerCloneTime(account.ExpiresAt),
		RateLimitResetAt:        quotaLeaseDemoHandlerCloneTime(account.RateLimitResetAt),
		TempUnschedulableUntil:  quotaLeaseDemoHandlerCloneTime(account.TempUnschedulableUntil),
		TempUnschedulableReason: account.TempUnschedulableReason,
		UpdatedAt:               quotaLeaseDemoHandlerTimeOrNow(account.UpdatedAt),
	}
	if snapshot.Status == "" {
		snapshot.Status = service.StatusActive
	}
	if snapshot.Concurrency <= 0 {
		snapshot.Concurrency = 1
	}
	if !snapshot.Schedulable && snapshot.Status == service.StatusActive {
		snapshot.Schedulable = true
	}
	return snapshot
}

func quotaLeaseDemoHandlerCloneAnyMap(src map[string]any) map[string]any {
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

func quotaLeaseDemoHandlerCloneInt64Slice(src []int64) []int64 {
	if len(src) == 0 {
		return nil
	}
	dst := make([]int64, len(src))
	copy(dst, src)
	return dst
}

func quotaLeaseDemoHandlerCloneTime(src *time.Time) *time.Time {
	if src == nil {
		return nil
	}
	value := *src
	return &value
}

func quotaLeaseDemoHandlerTimeOrNow(value time.Time) time.Time {
	if value.IsZero() {
		return time.Now().UTC()
	}
	return value
}

func quotaLeaseDemoHandlerString(value any) string {
	switch v := value.(type) {
	case string:
		return strings.TrimSpace(v)
	default:
		return ""
	}
}

func quotaLeaseDemoHandlerBool(value any) bool {
	v, ok := value.(bool)
	return ok && v
}

func (h *QuotaLeaseDemoHandler) RequestLease(c *gin.Context) {
	if !h.requireEnabled(c) {
		return
	}
	var req service.QuotaLeaseDemoLeaseRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid_request", "message": err.Error()})
		return
	}
	nodeID, ok := h.authenticateNodeOrControl(c, req.NodeID)
	if !ok {
		return
	}
	if req.NodeID == "" {
		req.NodeID = nodeID
	}
	lease, err := h.svc.RequestLease(c.Request.Context(), req)
	if err != nil {
		h.writeError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"lease": lease})
}

func (h *QuotaLeaseDemoHandler) PostUsageBatch(c *gin.Context) {
	if !h.requireEnabled(c) {
		return
	}
	var req service.QuotaLeaseDemoUsageBatchRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid_request", "message": err.Error()})
		return
	}
	nodeID, ok := h.authenticateNodeOrControl(c, req.NodeID)
	if !ok {
		return
	}
	if nodeID != "" {
		if req.NodeID == "" {
			req.NodeID = nodeID
		}
		if req.NodeID != nodeID {
			c.JSON(http.StatusForbidden, gin.H{"error": "node_mismatch"})
			return
		}
		for _, event := range req.Events {
			if strings.TrimSpace(event.NodeID) != "" && strings.TrimSpace(event.NodeID) != nodeID {
				c.JSON(http.StatusForbidden, gin.H{"error": "node_mismatch"})
				return
			}
		}
	}
	c.JSON(http.StatusOK, h.svc.PostUsageBatch(c.Request.Context(), req))
}

func (h *QuotaLeaseDemoHandler) ReclaimExpired(c *gin.Context) {
	if !h.requireEnabled(c) || !h.requireControlSecret(c) {
		return
	}
	c.JSON(http.StatusOK, h.svc.ReclaimExpired(c.Request.Context(), time.Now().UTC()))
}

func (h *QuotaLeaseDemoHandler) Status(c *gin.Context) {
	if !h.requireEnabled(c) || !h.requireControlSecret(c) {
		return
	}
	c.JSON(http.StatusOK, h.svc.Snapshot())
}

func (h *QuotaLeaseDemoHandler) requireEnabled(c *gin.Context) bool {
	if h == nil || h.svc == nil || !h.svc.Enabled() {
		c.JSON(http.StatusNotFound, gin.H{"error": "quota_lease_demo_disabled"})
		return false
	}
	return true
}

func (h *QuotaLeaseDemoHandler) requireControlSecret(c *gin.Context) bool {
	if h.controlSecretOK(c) {
		return true
	}
	c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid_node_secret"})
	return false
}

func (h *QuotaLeaseDemoHandler) controlSecretOK(c *gin.Context) bool {
	secret := h.svc.NodeSecret()
	if secret == "" {
		return true
	}
	return strings.TrimSpace(c.GetHeader("X-Node-Secret")) == secret
}

func (h *QuotaLeaseDemoHandler) authenticateNodeOrControl(c *gin.Context, requestedNodeID string) (string, bool) {
	requestedNodeID = strings.TrimSpace(requestedNodeID)
	headerNodeID := strings.TrimSpace(c.GetHeader("X-Node-ID"))
	nodeID := headerNodeID
	if nodeID == "" {
		nodeID = requestedNodeID
	}
	nodeSecret := strings.TrimSpace(c.GetHeader("X-Node-Secret"))
	if nodeID != "" && h.svc.AuthenticateNode(nodeID, nodeSecret) {
		if requestedNodeID != "" && requestedNodeID != nodeID {
			c.JSON(http.StatusForbidden, gin.H{"error": "node_mismatch"})
			return "", false
		}
		return nodeID, true
	}
	if h.controlSecretOK(c) {
		return requestedNodeID, true
	}
	c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid_node_secret"})
	return "", false
}

func (h *QuotaLeaseDemoHandler) writeError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, service.ErrQuotaLeaseDemoDisabled):
		c.JSON(http.StatusNotFound, gin.H{"error": "quota_lease_demo_disabled"})
	case errors.Is(err, service.ErrQuotaLeaseDemoInvalidInput):
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid_request", "message": err.Error()})
	case errors.Is(err, service.ErrQuotaLeaseDemoConflict):
		c.JSON(http.StatusConflict, gin.H{"error": "event_conflict", "message": err.Error()})
	case errors.Is(err, service.ErrQuotaLeaseDemoNodeNotFound):
		c.JSON(http.StatusNotFound, gin.H{"error": "node_not_found", "message": err.Error()})
	case errors.Is(err, service.ErrQuotaLeaseDemoNoCapacity):
		c.JSON(http.StatusForbidden, gin.H{"error": "no_capacity", "message": err.Error()})
	default:
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal_error", "message": err.Error()})
	}
}
