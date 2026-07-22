package handler

import (
	"context"
	"errors"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
)

type QuotaLeaseDemoHandler struct {
	svc            *service.QuotaLeaseDemoService
	adminSvc       service.AdminService
	apiKeyService  *service.APIKeyService
	usageService   *service.UsageService
	opsService     *service.OpsService
	channelService *service.ChannelService
}

const (
	quotaLeaseDemoNodeOAuthLastSyncedAtKey             = "node_oauth_last_synced_at"
	quotaLeaseDemoNodeOAuthLastSyncedAtPersistInterval = 5 * time.Second
)

type quotaLeaseDemoNodeAssignedAccountAdminService interface {
	ListNodeAssignedAccounts(ctx context.Context, nodeID string, page, pageSize int) ([]service.Account, int64, error)
}

func NewQuotaLeaseDemoHandler(svc *service.QuotaLeaseDemoService, adminSvc ...service.AdminService) *QuotaLeaseDemoHandler {
	h := &QuotaLeaseDemoHandler{svc: svc}
	if len(adminSvc) > 0 {
		h.adminSvc = adminSvc[0]
	}
	return h
}

func (h *QuotaLeaseDemoHandler) SetAPIKeyService(apiKeyService *service.APIKeyService) {
	if h == nil {
		return
	}
	h.apiKeyService = apiKeyService
}

func (h *QuotaLeaseDemoHandler) SetUsageService(usageService *service.UsageService) {
	if h == nil {
		return
	}
	h.usageService = usageService
}

func (h *QuotaLeaseDemoHandler) SetOpsService(opsService *service.OpsService) {
	if h == nil {
		return
	}
	h.opsService = opsService
}

func (h *QuotaLeaseDemoHandler) SetChannelService(channelService *service.ChannelService) {
	if h == nil {
		return
	}
	h.channelService = channelService
}

func (h *QuotaLeaseDemoHandler) InjectControlSecret(c *gin.Context) {
	if h != nil && h.svc != nil {
		if secret := h.svc.NodeSecret(); secret != "" {
			c.Request.Header.Set("X-Node-Secret", secret)
		}
	}
	c.Next()
}

func (h *QuotaLeaseDemoHandler) RegisterNode(c *gin.Context) {
	if !h.requireEnabled(c) {
		return
	}
	var req service.QuotaLeaseDemoNodeRegistrationRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid_request", "message": err.Error()})
		return
	}
	if req.RegistrationToken == "" {
		req.RegistrationToken = strings.TrimSpace(c.Query("registration_token"))
	}
	if strings.TrimSpace(req.RegistrationToken) == "" && !h.requireControlSecret(c) {
		return
	}
	result, err := h.svc.RegisterNode(c.Request.Context(), req)
	if err != nil {
		h.writeError(c, err)
		return
	}
	c.JSON(http.StatusOK, result)
}

func (h *QuotaLeaseDemoHandler) CreateNodeRegistrationURL(c *gin.Context) {
	if !h.requireEnabled(c) || !h.requireControlSecret(c) {
		return
	}
	var req service.QuotaLeaseDemoNodeRegistrationURLRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid_request", "message": err.Error()})
		return
	}
	result, err := h.svc.CreateNodeRegistrationURL(c.Request.Context(), req, quotaLeaseDemoExternalBaseURL(c))
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

func (h *QuotaLeaseDemoHandler) Diagnostics(c *gin.Context) {
	if !h.requireEnabled(c) || !h.requireControlSecret(c) {
		return
	}
	var resolver service.QuotaLeaseDemoDiagnosticUserResolver
	if h.adminSvc != nil {
		resolver = h.resolveQuotaLeaseDemoDiagnosticUser
	}
	c.JSON(http.StatusOK, gin.H{"diagnostics": h.svc.Diagnostics(c.Request.Context(), resolver)})
}

func (h *QuotaLeaseDemoHandler) resolveQuotaLeaseDemoDiagnosticUser(ctx context.Context, userID int64) (service.QuotaLeaseDemoDiagnosticUserProfile, error) {
	profile := service.QuotaLeaseDemoDiagnosticUserProfile{UserID: userID}
	if h == nil || h.adminSvc == nil || userID <= 0 {
		return profile, nil
	}
	user, err := h.adminSvc.GetUserIncludeDeleted(ctx, userID)
	if err != nil {
		return profile, err
	}
	if user == nil {
		return profile, nil
	}
	balance := user.Balance
	frozenBalance := user.FrozenBalance
	return service.QuotaLeaseDemoDiagnosticUserProfile{
		UserID:        user.ID,
		Username:      strings.TrimSpace(user.Username),
		Email:         strings.TrimSpace(user.Email),
		Status:        strings.TrimSpace(user.Status),
		Balance:       &balance,
		FrozenBalance: &frozenBalance,
		Found:         true,
	}, nil
}

func (h *QuotaLeaseDemoHandler) UpdateNode(c *gin.Context) {
	if !h.requireEnabled(c) || !h.requireControlSecret(c) {
		return
	}
	var req service.QuotaLeaseDemoNodeUpdateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid_request", "message": err.Error()})
		return
	}
	node, err := h.svc.UpdateNode(c.Request.Context(), c.Param("node_id"), req)
	if err != nil {
		h.writeError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"node": node})
}

func (h *QuotaLeaseDemoHandler) GetSettings(c *gin.Context) {
	if !h.requireEnabled(c) {
		return
	}
	if _, ok := h.authenticateNodeOrControl(c, ""); !ok {
		return
	}
	settings, err := h.svc.GetSettings(c.Request.Context())
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, settings)
}

func (h *QuotaLeaseDemoHandler) UpdateSettings(c *gin.Context) {
	if !h.requireEnabled(c) || !h.requireControlSecret(c) {
		return
	}
	var req service.QuotaLeaseDemoSettingsPatch
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}
	settings, err := h.svc.UpdateSettings(c.Request.Context(), &req)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, settings)
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
			ProxyID:               quotaLeaseDemoHandlerCloneInt64Ptr(account.ProxyID),
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
		ProxyID:               quotaLeaseDemoHandlerCloneInt64Ptr(account.ProxyID),
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

func (h *QuotaLeaseDemoHandler) CreateUsageProbeTask(c *gin.Context) {
	if !h.requireEnabled(c) || !h.requireControlSecret(c) {
		return
	}
	var req service.QuotaLeaseDemoUsageProbeTaskCreateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid_request", "message": err.Error()})
		return
	}
	task, err := h.svc.CreateUsageProbeTask(c.Request.Context(), req)
	if err != nil {
		h.writeError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"task": task})
}

func (h *QuotaLeaseDemoHandler) ListUsageProbeTasks(c *gin.Context) {
	if !h.requireEnabled(c) {
		return
	}
	requestedNodeID := strings.TrimSpace(c.Query("node_id"))
	nodeID, ok := h.authenticateNodeOrControl(c, requestedNodeID)
	if !ok {
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"tasks": h.svc.ListUsageProbeTasks(c.Request.Context(), nodeID, c.Query("status")),
	})
}

func (h *QuotaLeaseDemoHandler) CompleteUsageProbeTask(c *gin.Context) {
	if !h.requireEnabled(c) {
		return
	}
	var req service.QuotaLeaseDemoUsageProbeTaskCompleteRequest
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
	task, err := h.svc.CompleteUsageProbeTask(c.Request.Context(), req)
	if err != nil {
		h.writeError(c, err)
		return
	}
	if err := h.syncUsageProbeTaskResult(c.Request.Context(), task); err != nil {
		h.writeError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"task": task})
}

func (h *QuotaLeaseDemoHandler) syncUsageProbeTaskResult(ctx context.Context, task *service.QuotaLeaseDemoUsageProbeTask) error {
	if h == nil || h.adminSvc == nil || task == nil || task.Status != service.QuotaLeaseDemoAccountTaskCompleted {
		return nil
	}
	if task.AccountID <= 0 || len(task.ExtraPatch) == 0 {
		return nil
	}
	return h.adminSvc.UpdateAccountExtra(ctx, task.AccountID, task.ExtraPatch)
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

func (h *QuotaLeaseDemoHandler) MirrorSnapshot(c *gin.Context) {
	if !h.requireEnabled(c) {
		return
	}
	requestedNodeID := strings.TrimSpace(c.Query("node_id"))
	nodeID, ok := h.authenticateNodeOrControl(c, requestedNodeID)
	if !ok {
		return
	}
	if h.adminSvc == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "admin_service_unavailable"})
		return
	}
	sinceVersion, err := quotaLeaseDemoHandlerInt64Query(c, "since_version")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid_request", "message": err.Error()})
		return
	}
	snapshot, err := h.buildMirrorSnapshot(c.Request.Context(), nodeID)
	if err != nil {
		h.writeError(c, err)
		return
	}
	snapshot = h.svc.PrepareMirrorSnapshot(snapshot, sinceVersion)
	c.JSON(http.StatusOK, gin.H{"snapshot": snapshot})
}

func (h *QuotaLeaseDemoHandler) buildMirrorSnapshot(ctx context.Context, nodeID string) (service.QuotaLeaseDemoMirrorSnapshot, error) {
	syncedAt := time.Now().UTC()
	snapshot := service.QuotaLeaseDemoMirrorSnapshot{
		NodeID:   strings.TrimSpace(nodeID),
		SyncedAt: syncedAt,
	}

	groups, err := h.adminSvc.GetAllGroupsIncludingInactive(ctx)
	if err != nil {
		return snapshot, err
	}
	snapshot.Groups = make([]service.QuotaLeaseDemoGroupSnapshot, 0, len(groups))
	for _, group := range groups {
		snapshot.Groups = append(snapshot.Groups, quotaLeaseDemoHandlerGroupSnapshot(group))
	}

	if h.channelService != nil {
		channels, err := h.channelService.ListAll(ctx)
		if err != nil {
			return snapshot, err
		}
		snapshot.Channels = make([]service.QuotaLeaseDemoChannelSnapshot, 0, len(channels))
		for _, channel := range channels {
			snapshot.Channels = append(snapshot.Channels, service.NewQuotaLeaseDemoChannelSnapshot(channel))
		}
	} else {
		snapshot.Channels = []service.QuotaLeaseDemoChannelSnapshot{}
	}

	proxies, err := h.adminSvc.GetAllProxies(ctx)
	if err != nil {
		return snapshot, err
	}
	snapshot.Proxies = make([]service.QuotaLeaseDemoProxySnapshot, 0, len(proxies))
	for _, proxy := range proxies {
		if proxySnapshot := quotaLeaseDemoHandlerProxySnapshot(&proxy); proxySnapshot != nil {
			snapshot.Proxies = append(snapshot.Proxies, *proxySnapshot)
		}
	}

	assigned, err := h.listAssignedAccounts(ctx, nodeID)
	if err != nil {
		return snapshot, err
	}
	accountGroups := make(map[[2]int64]service.QuotaLeaseDemoAccountGroupSnapshot)
	snapshot.Accounts = make([]service.QuotaLeaseDemoAccountSnapshot, 0, len(assigned))
	for _, item := range assigned {
		account := item.Account
		if quotaLeaseDemoHandlerString(account.Extra[quotaLeaseDemoNodeOAuthLastSyncedAtKey]) == "" {
			if account.Extra == nil {
				account.Extra = map[string]any{}
			}
			account.Extra[quotaLeaseDemoNodeOAuthLastSyncedAtKey] = syncedAt.Format(time.RFC3339Nano)
		}
		if len(account.AccountGroups) == 0 && len(account.GroupIDs) > 0 {
			account.AccountGroups = quotaLeaseDemoHandlerAccountGroupSnapshotsFromIDs(account.ID, account.GroupIDs, quotaLeaseDemoHandlerTimeOrNow(account.CreatedAt))
		}
		snapshot.Accounts = append(snapshot.Accounts, account)
		for _, accountGroup := range account.AccountGroups {
			if accountGroup.AccountID <= 0 || accountGroup.GroupID <= 0 {
				continue
			}
			key := [2]int64{accountGroup.AccountID, accountGroup.GroupID}
			accountGroups[key] = accountGroup
		}
	}
	snapshot.AccountGroups = make([]service.QuotaLeaseDemoAccountGroupSnapshot, 0, len(accountGroups))
	for _, accountGroup := range accountGroups {
		snapshot.AccountGroups = append(snapshot.AccountGroups, accountGroup)
	}
	sort.Slice(snapshot.AccountGroups, func(i, j int) bool {
		if snapshot.AccountGroups[i].AccountID == snapshot.AccountGroups[j].AccountID {
			return snapshot.AccountGroups[i].GroupID < snapshot.AccountGroups[j].GroupID
		}
		return snapshot.AccountGroups[i].AccountID < snapshot.AccountGroups[j].AccountID
	})
	apiKeys, err := h.listMirrorAPIKeySnapshots(ctx)
	if err != nil {
		return snapshot, err
	}
	snapshot.APIKeys = apiKeys
	return snapshot, nil
}

func (h *QuotaLeaseDemoHandler) listMirrorAPIKeySnapshots(ctx context.Context) ([]service.QuotaLeaseDemoAPIKeySnapshot, error) {
	if h == nil || h.adminSvc == nil || h.apiKeyService == nil {
		return nil, nil
	}
	const pageSize = 500
	includeSubscriptions := false
	out := make([]service.QuotaLeaseDemoAPIKeySnapshot, 0)
	seen := make(map[int64]struct{})
	for page := 1; ; page++ {
		users, total, err := h.adminSvc.ListUsers(ctx, page, pageSize, service.UserListFilters{
			IncludeSubscriptions: &includeSubscriptions,
		}, "id", "asc")
		if err != nil {
			return nil, err
		}
		for _, user := range users {
			keys, err := h.listMirrorAPIKeysByUser(ctx, user.ID)
			if err != nil {
				return nil, err
			}
			for _, key := range keys {
				if key.ID <= 0 {
					continue
				}
				if _, ok := seen[key.ID]; ok {
					continue
				}
				seen[key.ID] = struct{}{}
				out = append(out, key)
			}
		}
		if len(users) == 0 || int64(page*pageSize) >= total {
			break
		}
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].ID < out[j].ID
	})
	return out, nil
}

func (h *QuotaLeaseDemoHandler) listMirrorAPIKeysByUser(ctx context.Context, userID int64) ([]service.QuotaLeaseDemoAPIKeySnapshot, error) {
	const pageSize = 500
	out := make([]service.QuotaLeaseDemoAPIKeySnapshot, 0)
	for page := 1; ; page++ {
		keys, total, err := h.adminSvc.GetUserAPIKeys(ctx, userID, page, pageSize, "id", "asc")
		if err != nil {
			return nil, err
		}
		for _, key := range keys {
			snapshot, err := h.apiKeyService.AuthSnapshotForKey(ctx, key.Key)
			if err != nil {
				if errors.Is(err, service.ErrAPIKeyNotFound) {
					continue
				}
				return nil, err
			}
			item := service.NewQuotaLeaseDemoAPIKeySnapshot(key.Key, snapshot)
			item.CreatedAt = quotaLeaseDemoHandlerTimeOrNow(key.CreatedAt)
			item.UpdatedAt = quotaLeaseDemoHandlerTimeOrNow(key.UpdatedAt)
			out = append(out, item)
		}
		if len(keys) == 0 || int64(page*pageSize) >= total {
			break
		}
	}
	return out, nil
}

func (h *QuotaLeaseDemoHandler) listAssignedAccounts(ctx context.Context, nodeID string) ([]service.QuotaLeaseDemoAssignedAccount, error) {
	accounts := h.svc.ListAssignedAccounts(ctx, nodeID)
	if h.adminSvc == nil {
		return accounts, nil
	}

	byID := make(map[int64]service.QuotaLeaseDemoAssignedAccount)

	syncedAt := time.Now().UTC()
	const pageSize = 500
	nodeAssignedLister, hasNodeAssignedLister := h.adminSvc.(quotaLeaseDemoNodeAssignedAccountAdminService)
	for page := 1; ; page++ {
		var (
			items []service.Account
			total int64
			err   error
		)
		if hasNodeAssignedLister {
			items, total, err = nodeAssignedLister.ListNodeAssignedAccounts(ctx, nodeID, page, pageSize)
		} else {
			items, total, err = h.adminSvc.ListAccounts(ctx, page, pageSize, "", "", "", "", 0, "", "id", "asc")
		}
		if err != nil {
			return nil, err
		}
		for _, account := range items {
			if account.ID <= 0 {
				continue
			}
			assignedNodeID := service.QuotaLeaseDemoAssignedNodeID(account)
			if nodeID != "" {
				if !service.QuotaLeaseDemoAccountAssignedToNode(account, nodeID) {
					continue
				}
				assignedNodeID = nodeID
			}
			if assignedNodeID == "" {
				continue
			}
			if !quotaLeaseDemoHandlerPersistedAccountReady(account) {
				continue
			}
			snapshot := h.quotaLeaseDemoHandlerAccountSnapshot(ctx, account)
			snapshot.Extra = h.touchAssignedAccountSync(ctx, account.ID, account.Extra, snapshot.Extra, syncedAt)
			byID[account.ID] = service.QuotaLeaseDemoAssignedAccount{
				NodeID:    assignedNodeID,
				Account:   snapshot,
				CreatedAt: quotaLeaseDemoHandlerTimeOrNow(account.CreatedAt),
				UpdatedAt: quotaLeaseDemoHandlerTimeOrNow(account.UpdatedAt),
			}
		}
		if len(items) == 0 || int64(page*pageSize) >= total {
			break
		}
	}

	accounts = make([]service.QuotaLeaseDemoAssignedAccount, 0, len(byID))
	for _, assigned := range byID {
		accounts = append(accounts, assigned)
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
	if strings.TrimSpace(account.Platform) == "" || strings.TrimSpace(account.Type) == "" {
		return false
	}
	if account.Status != service.StatusActive || !account.Schedulable {
		return false
	}
	if quotaLeaseDemoHandlerBool(account.Credentials["node_oauth_pending"]) {
		return false
	}
	switch account.Type {
	case service.AccountTypeOAuth, service.AccountTypeSetupToken:
		status := strings.TrimSpace(quotaLeaseDemoHandlerString(account.Extra["node_oauth_status"]))
		return status == "" || status == service.QuotaLeaseDemoAccountTaskCompleted
	case service.AccountTypeAPIKey, service.AccountTypeUpstream, service.AccountTypeBedrock, service.AccountTypeServiceAccount:
		return len(account.Credentials) > 0
	default:
		return false
	}
}

func (h *QuotaLeaseDemoHandler) quotaLeaseDemoHandlerAccountSnapshot(ctx context.Context, account service.Account) service.QuotaLeaseDemoAccountSnapshot {
	snapshot := service.QuotaLeaseDemoAccountSnapshot{
		ID:                      account.ID,
		Name:                    account.Name,
		Notes:                   quotaLeaseDemoHandlerCloneStringPtr(account.Notes),
		Platform:                account.Platform,
		Type:                    account.Type,
		Credentials:             quotaLeaseDemoHandlerCloneAnyMap(account.Credentials),
		Extra:                   quotaLeaseDemoHandlerCloneAnyMap(account.Extra),
		ProxyID:                 quotaLeaseDemoHandlerCloneInt64Ptr(account.ProxyID),
		ProxyFallbackOriginID:   quotaLeaseDemoHandlerCloneInt64Ptr(account.ProxyFallbackOriginID),
		Proxy:                   quotaLeaseDemoHandlerProxySnapshot(h.quotaLeaseDemoHandlerAccountProxy(ctx, account)),
		Status:                  account.Status,
		ErrorMessage:            account.ErrorMessage,
		Schedulable:             account.Schedulable,
		Concurrency:             account.Concurrency,
		LoadFactor:              quotaLeaseDemoHandlerCloneIntPtr(account.LoadFactor),
		Priority:                account.Priority,
		RateMultiplier:          quotaLeaseDemoHandlerCloneFloat64Ptr(account.RateMultiplier),
		GroupIDs:                quotaLeaseDemoHandlerAccountGroupIDs(account),
		AccountGroups:           quotaLeaseDemoHandlerAccountGroupSnapshots(account),
		LastUsedAt:              quotaLeaseDemoHandlerCloneTime(account.LastUsedAt),
		ExpiresAt:               quotaLeaseDemoHandlerCloneTime(account.ExpiresAt),
		AutoPauseOnExpired:      account.AutoPauseOnExpired,
		RateLimitedAt:           quotaLeaseDemoHandlerCloneTime(account.RateLimitedAt),
		RateLimitResetAt:        quotaLeaseDemoHandlerCloneTime(account.RateLimitResetAt),
		OverloadUntil:           quotaLeaseDemoHandlerCloneTime(account.OverloadUntil),
		TempUnschedulableUntil:  quotaLeaseDemoHandlerCloneTime(account.TempUnschedulableUntil),
		TempUnschedulableReason: account.TempUnschedulableReason,
		SessionWindowStart:      quotaLeaseDemoHandlerCloneTime(account.SessionWindowStart),
		SessionWindowEnd:        quotaLeaseDemoHandlerCloneTime(account.SessionWindowEnd),
		SessionWindowStatus:     account.SessionWindowStatus,
		ParentAccountID:         quotaLeaseDemoHandlerCloneInt64Ptr(account.ParentAccountID),
		QuotaDimension:          account.QuotaDimensionOrDefault(),
		CreatedAt:               quotaLeaseDemoHandlerTimeOrNow(account.CreatedAt),
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
	if snapshot.ProxyID == nil && snapshot.Proxy != nil && snapshot.Proxy.ID > 0 {
		snapshot.ProxyID = &snapshot.Proxy.ID
	}
	if len(snapshot.AccountGroups) == 0 && len(snapshot.GroupIDs) > 0 {
		snapshot.AccountGroups = quotaLeaseDemoHandlerAccountGroupSnapshotsFromIDs(snapshot.ID, snapshot.GroupIDs, snapshot.CreatedAt)
	}
	return snapshot
}

func (h *QuotaLeaseDemoHandler) quotaLeaseDemoHandlerAccountProxy(ctx context.Context, account service.Account) *service.Proxy {
	if account.Proxy != nil {
		return account.Proxy
	}
	if h == nil || h.adminSvc == nil || account.ProxyID == nil || *account.ProxyID <= 0 {
		return nil
	}
	proxy, err := h.adminSvc.GetProxy(ctx, *account.ProxyID)
	if err != nil {
		return nil
	}
	return proxy
}

func quotaLeaseDemoHandlerProxySnapshot(proxy *service.Proxy) *service.QuotaLeaseDemoProxySnapshot {
	if proxy == nil {
		return nil
	}
	return &service.QuotaLeaseDemoProxySnapshot{
		ID:             proxy.ID,
		Name:           proxy.Name,
		Protocol:       proxy.Protocol,
		Host:           proxy.Host,
		Port:           proxy.Port,
		Username:       proxy.Username,
		Password:       proxy.Password,
		Status:         proxy.Status,
		ExpiresAt:      quotaLeaseDemoHandlerCloneTime(proxy.ExpiresAt),
		FallbackMode:   proxy.FallbackMode,
		BackupProxyID:  quotaLeaseDemoHandlerCloneInt64Ptr(proxy.BackupProxyID),
		ExpiryWarnDays: proxy.ExpiryWarnDays,
		CreatedAt:      quotaLeaseDemoHandlerTimeOrNow(proxy.CreatedAt),
		UpdatedAt:      quotaLeaseDemoHandlerTimeOrNow(proxy.UpdatedAt),
	}
}

func quotaLeaseDemoHandlerGroupSnapshot(group service.Group) service.QuotaLeaseDemoGroupSnapshot {
	return service.QuotaLeaseDemoGroupSnapshot{
		ID:                              group.ID,
		Name:                            group.Name,
		Description:                     group.Description,
		Platform:                        group.Platform,
		RateMultiplier:                  group.RateMultiplier,
		PeakRateEnabled:                 group.PeakRateEnabled,
		PeakStart:                       group.PeakStart,
		PeakEnd:                         group.PeakEnd,
		PeakRateMultiplier:              group.PeakRateMultiplier,
		IsExclusive:                     group.IsExclusive,
		Status:                          group.Status,
		SubscriptionType:                group.SubscriptionType,
		DailyLimitUSD:                   quotaLeaseDemoHandlerCloneFloat64Ptr(group.DailyLimitUSD),
		WeeklyLimitUSD:                  quotaLeaseDemoHandlerCloneFloat64Ptr(group.WeeklyLimitUSD),
		MonthlyLimitUSD:                 quotaLeaseDemoHandlerCloneFloat64Ptr(group.MonthlyLimitUSD),
		DefaultValidityDays:             group.DefaultValidityDays,
		AllowImageGeneration:            group.AllowImageGeneration,
		AllowBatchImageGeneration:       group.AllowBatchImageGeneration,
		ImageRateIndependent:            group.ImageRateIndependent,
		ImageRateMultiplier:             group.ImageRateMultiplier,
		ImagePrice1K:                    quotaLeaseDemoHandlerCloneFloat64Ptr(group.ImagePrice1K),
		ImagePrice2K:                    quotaLeaseDemoHandlerCloneFloat64Ptr(group.ImagePrice2K),
		ImagePrice4K:                    quotaLeaseDemoHandlerCloneFloat64Ptr(group.ImagePrice4K),
		BatchImageDiscountMultiplier:    group.BatchImageDiscountMultiplier,
		BatchImageHoldMultiplier:        group.BatchImageHoldMultiplier,
		VideoRateIndependent:            group.VideoRateIndependent,
		VideoRateMultiplier:             group.VideoRateMultiplier,
		VideoPrice480P:                  quotaLeaseDemoHandlerCloneFloat64Ptr(group.VideoPrice480P),
		VideoPrice720P:                  quotaLeaseDemoHandlerCloneFloat64Ptr(group.VideoPrice720P),
		VideoPrice1080P:                 quotaLeaseDemoHandlerCloneFloat64Ptr(group.VideoPrice1080P),
		WebSearchPricePerCall:           quotaLeaseDemoHandlerCloneFloat64Ptr(group.WebSearchPricePerCall),
		ClaudeCodeOnly:                  group.ClaudeCodeOnly,
		FallbackGroupID:                 quotaLeaseDemoHandlerCloneInt64Ptr(group.FallbackGroupID),
		FallbackGroupIDOnInvalidRequest: quotaLeaseDemoHandlerCloneInt64Ptr(group.FallbackGroupIDOnInvalidRequest),
		ModelRouting:                    quotaLeaseDemoHandlerCloneModelRouting(group.ModelRouting),
		ModelRoutingEnabled:             group.ModelRoutingEnabled,
		MCPXMLInject:                    group.MCPXMLInject,
		SupportedModelScopes:            quotaLeaseDemoHandlerCloneStringSlice(group.SupportedModelScopes),
		SortOrder:                       group.SortOrder,
		AllowMessagesDispatch:           group.AllowMessagesDispatch,
		RequireOAuthOnly:                group.RequireOAuthOnly,
		RequirePrivacySet:               group.RequirePrivacySet,
		DefaultMappedModel:              group.DefaultMappedModel,
		MessagesDispatchModelConfig:     group.MessagesDispatchModelConfig,
		ModelsListConfig:                group.ModelsListConfig,
		RPMLimit:                        group.RPMLimit,
		KiroCacheEmulationEnabled:       group.KiroCacheEmulationEnabled,
		KiroAutoStickyEnabled:           group.KiroAutoStickyEnabled,
		KiroStickySessionTTLSeconds:     group.KiroStickySessionTTLSeconds,
		KiroCacheEmulationRatio:         group.KiroCacheEmulationRatio,
		KiroEndpointMode:                group.KiroEndpointMode,
		CreatedAt:                       quotaLeaseDemoHandlerTimeOrNow(group.CreatedAt),
		UpdatedAt:                       quotaLeaseDemoHandlerTimeOrNow(group.UpdatedAt),
	}
}

func quotaLeaseDemoHandlerAccountGroupSnapshots(account service.Account) []service.QuotaLeaseDemoAccountGroupSnapshot {
	if len(account.AccountGroups) == 0 {
		return nil
	}
	out := make([]service.QuotaLeaseDemoAccountGroupSnapshot, 0, len(account.AccountGroups))
	for _, item := range account.AccountGroups {
		if item.AccountID <= 0 {
			item.AccountID = account.ID
		}
		if item.AccountID <= 0 || item.GroupID <= 0 {
			continue
		}
		createdAt := item.CreatedAt
		if createdAt.IsZero() {
			createdAt = quotaLeaseDemoHandlerTimeOrNow(account.CreatedAt)
		}
		out = append(out, service.QuotaLeaseDemoAccountGroupSnapshot{
			AccountID: item.AccountID,
			GroupID:   item.GroupID,
			Priority:  item.Priority,
			CreatedAt: createdAt,
		})
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func quotaLeaseDemoHandlerAccountGroupSnapshotsFromIDs(accountID int64, groupIDs []int64, createdAt time.Time) []service.QuotaLeaseDemoAccountGroupSnapshot {
	if accountID <= 0 || len(groupIDs) == 0 {
		return nil
	}
	seen := make(map[int64]struct{}, len(groupIDs))
	out := make([]service.QuotaLeaseDemoAccountGroupSnapshot, 0, len(groupIDs))
	for _, groupID := range groupIDs {
		if groupID <= 0 {
			continue
		}
		if _, exists := seen[groupID]; exists {
			continue
		}
		seen[groupID] = struct{}{}
		out = append(out, service.QuotaLeaseDemoAccountGroupSnapshot{
			AccountID: accountID,
			GroupID:   groupID,
			CreatedAt: quotaLeaseDemoHandlerTimeOrNow(createdAt),
		})
	}
	if len(out) == 0 {
		return nil
	}
	return out
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

func quotaLeaseDemoHandlerCloneStringSlice(src []string) []string {
	if len(src) == 0 {
		return nil
	}
	dst := make([]string, len(src))
	copy(dst, src)
	return dst
}

func quotaLeaseDemoHandlerCloneModelRouting(src map[string][]int64) map[string][]int64 {
	if len(src) == 0 {
		return nil
	}
	dst := make(map[string][]int64, len(src))
	for key, value := range src {
		key = strings.TrimSpace(key)
		if key == "" {
			continue
		}
		dst[key] = quotaLeaseDemoHandlerCloneInt64Slice(value)
	}
	if len(dst) == 0 {
		return nil
	}
	return dst
}

func quotaLeaseDemoHandlerAccountGroupIDs(account service.Account) []int64 {
	seen := map[int64]struct{}{}
	groupIDs := make([]int64, 0, len(account.GroupIDs)+len(account.AccountGroups)+len(account.Groups))
	add := func(id int64) {
		if id <= 0 {
			return
		}
		if _, exists := seen[id]; exists {
			return
		}
		seen[id] = struct{}{}
		groupIDs = append(groupIDs, id)
	}
	for _, id := range account.GroupIDs {
		add(id)
	}
	for _, accountGroup := range account.AccountGroups {
		add(accountGroup.GroupID)
	}
	for _, group := range account.Groups {
		if group != nil {
			add(group.ID)
		}
	}
	if len(groupIDs) == 0 {
		return nil
	}
	return groupIDs
}

func quotaLeaseDemoHandlerCloneInt64Ptr(src *int64) *int64 {
	if src == nil {
		return nil
	}
	value := *src
	return &value
}

func quotaLeaseDemoHandlerCloneIntPtr(src *int) *int {
	if src == nil {
		return nil
	}
	value := *src
	return &value
}

func quotaLeaseDemoHandlerCloneFloat64Ptr(src *float64) *float64 {
	if src == nil {
		return nil
	}
	value := *src
	return &value
}

func quotaLeaseDemoHandlerCloneStringPtr(src *string) *string {
	if src == nil {
		return nil
	}
	value := *src
	return &value
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

func quotaLeaseDemoHandlerInt64Query(c *gin.Context, key string) (int64, error) {
	raw := strings.TrimSpace(c.Query(key))
	if raw == "" {
		return 0, nil
	}
	value, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || value < 0 {
		return 0, service.ErrQuotaLeaseDemoInvalidInput
	}
	return value, nil
}

func quotaLeaseDemoExternalBaseURL(c *gin.Context) string {
	if c == nil || c.Request == nil {
		return ""
	}
	proto := strings.TrimSpace(c.GetHeader("X-Forwarded-Proto"))
	if proto == "" {
		proto = strings.TrimSpace(c.GetHeader("X-Forwarded-Protocol"))
	}
	if proto == "" {
		if c.Request.TLS != nil {
			proto = "https"
		} else {
			proto = "http"
		}
	}
	if comma := strings.Index(proto, ","); comma >= 0 {
		proto = strings.TrimSpace(proto[:comma])
	}
	host := strings.TrimSpace(c.GetHeader("X-Forwarded-Host"))
	if host == "" {
		host = strings.TrimSpace(c.Request.Host)
	}
	if comma := strings.Index(host, ","); comma >= 0 {
		host = strings.TrimSpace(host[:comma])
	}
	if host == "" {
		return ""
	}
	return proto + "://" + host
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
	if amount, err := h.quotaLeaseDemoRequestLeaseAmount(c.Request.Context(), req); err != nil {
		h.writeError(c, err)
		return
	} else {
		req.Amount = amount
	}
	lease, err := h.svc.RequestLease(c.Request.Context(), req)
	if err != nil {
		h.writeError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"lease": lease})
}

func (h *QuotaLeaseDemoHandler) AuthorizeClientKey(c *gin.Context) {
	if !h.requireEnabled(c) {
		return
	}
	var req service.QuotaLeaseDemoClientAuthRequest
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
	if h.apiKeyService == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "api_key_service_unavailable"})
		return
	}
	snapshot, err := h.apiKeyService.AuthSnapshotForKey(c.Request.Context(), req.APIKey)
	if err != nil {
		if errors.Is(err, service.ErrAPIKeyNotFound) {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid_api_key"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "api_key_auth_failed", "message": err.Error()})
		return
	}
	if !quotaLeaseDemoHandlerCanIssueClientLease(snapshot) {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "api_key_disabled"})
		return
	}
	amount := req.Amount
	if amount <= 0 {
		amount = h.svc.DefaultGrantAmount()
	}
	leaseReq := service.QuotaLeaseDemoLeaseRequest{
		NodeID:    req.NodeID,
		UserID:    snapshot.UserID,
		APIKeyID:  snapshot.APIKeyID,
		Amount:    amount,
		RequestID: req.RequestID,
		TraceID:   req.TraceID,
	}
	amount, err = h.quotaLeaseDemoRequestLeaseAmount(c.Request.Context(), leaseReq)
	if err != nil {
		h.writeError(c, err)
		return
	}
	leaseReq.Amount = amount
	lease, err := h.svc.RequestLease(c.Request.Context(), leaseReq)
	if err != nil {
		h.writeError(c, err)
		return
	}
	expiresAt := time.Now().UTC().Add(30 * time.Second)
	if lease != nil && !lease.ExpiresAt.IsZero() && lease.ExpiresAt.Before(expiresAt) {
		expiresAt = lease.ExpiresAt
	}
	traceID := strings.TrimSpace(req.TraceID)
	if traceID == "" && lease != nil {
		traceID = strings.TrimSpace(lease.TraceID)
	}
	c.JSON(http.StatusOK, gin.H{
		"snapshot":   snapshot,
		"lease":      lease,
		"trace_id":   traceID,
		"expires_at": expiresAt,
	})
}

func (h *QuotaLeaseDemoHandler) quotaLeaseDemoRequestLeaseAmount(ctx context.Context, req service.QuotaLeaseDemoLeaseRequest) (float64, error) {
	if h == nil || h.svc == nil {
		return req.Amount, nil
	}
	amount := req.Amount
	if amount <= 0 {
		amount = h.svc.DefaultGrantAmount()
	}
	if req.UserID <= 0 || req.APIKeyID <= 0 || h.apiKeyService == nil {
		return amount, nil
	}
	apiKey, err := h.apiKeyService.GetByID(ctx, req.APIKeyID)
	if err != nil {
		return 0, err
	}
	if apiKey == nil || apiKey.User == nil || apiKey.UserID != req.UserID {
		return 0, service.ErrQuotaLeaseDemoNoCapacity
	}
	if apiKey.User.Status != service.StatusActive {
		return 0, service.ErrQuotaLeaseDemoNoCapacity
	}
	if apiKey.Status != service.StatusActive &&
		apiKey.Status != service.StatusAPIKeyExpired &&
		apiKey.Status != service.StatusAPIKeyQuotaExhausted {
		return 0, service.ErrQuotaLeaseDemoNoCapacity
	}
	if apiKey.Group != nil && apiKey.Group.Status != service.StatusActive {
		return 0, service.ErrQuotaLeaseDemoNoCapacity
	}
	if apiKey.User.Balance <= 0 {
		return 0, service.ErrQuotaLeaseDemoNoCapacity
	}
	if amount > apiKey.User.Balance {
		return apiKey.User.Balance, nil
	}
	return amount, nil
}

func quotaLeaseDemoClientLeaseAmount(snapshot *service.APIKeyAuthSnapshot, requested float64) float64 {
	if snapshot == nil {
		return requested
	}
	balance := snapshot.User.Balance
	if balance <= 0 {
		return 0
	}
	if requested > balance {
		return balance
	}
	return requested
}

func quotaLeaseDemoHandlerCanIssueClientLease(snapshot *service.APIKeyAuthSnapshot) bool {
	if snapshot == nil {
		return false
	}
	if snapshot.User.Status != service.StatusActive {
		return false
	}
	if snapshot.Status != service.StatusActive &&
		snapshot.Status != service.StatusAPIKeyExpired &&
		snapshot.Status != service.StatusAPIKeyQuotaExhausted {
		return false
	}
	if snapshot.Group != nil && snapshot.Group.Status != service.StatusActive {
		return false
	}
	return true
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

func (h *QuotaLeaseDemoHandler) PostUsageLogBatch(c *gin.Context) {
	if !h.requireEnabled(c) {
		return
	}
	if h.usageService == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "usage_service_unavailable"})
		return
	}
	var req service.QuotaLeaseDemoUsageLogBatchRequest
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
		for _, item := range req.Logs {
			if strings.TrimSpace(item.NodeID) != "" && strings.TrimSpace(item.NodeID) != nodeID {
				c.JSON(http.StatusForbidden, gin.H{"error": "node_mismatch"})
				return
			}
		}
	}

	result := service.QuotaLeaseDemoUsageLogBatchResult{
		Results: make([]service.QuotaLeaseDemoUsageLogResult, 0, len(req.Logs)),
	}
	for _, item := range req.Logs {
		if strings.TrimSpace(item.NodeID) == "" {
			item.NodeID = strings.TrimSpace(req.NodeID)
		}
		log := item.ToUsageLog()
		row := service.QuotaLeaseDemoUsageLogResult{
			RequestID: log.RequestID,
			APIKeyID:  log.APIKeyID,
		}
		if strings.TrimSpace(log.RequestID) == "" || log.APIKeyID <= 0 {
			row.Error = "request_id and api_key_id are required"
			result.Results = append(result.Results, row)
			continue
		}
		inserted, err := h.usageService.CreateLogBestEffort(c.Request.Context(), log)
		if err != nil {
			row.Error = err.Error()
			result.Results = append(result.Results, row)
			continue
		}
		row.Applied = inserted
		row.Duplicate = !inserted
		result.Results = append(result.Results, row)
	}
	c.JSON(http.StatusOK, result)
}

func (h *QuotaLeaseDemoHandler) PostOpsErrorLogBatch(c *gin.Context) {
	if !h.requireEnabled(c) {
		return
	}
	if h.opsService == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "ops_service_unavailable"})
		return
	}
	var req service.QuotaLeaseDemoOpsErrorLogBatchRequest
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
		for _, item := range req.Logs {
			if strings.TrimSpace(item.NodeID) != "" && strings.TrimSpace(item.NodeID) != nodeID {
				c.JSON(http.StatusForbidden, gin.H{"error": "node_mismatch"})
				return
			}
		}
	}

	result := service.QuotaLeaseDemoOpsErrorLogBatchResult{
		Results: make([]service.QuotaLeaseDemoOpsErrorLogResult, 0, len(req.Logs)),
	}
	entries := make([]*service.OpsInsertErrorLogInput, 0, len(req.Logs))
	for _, item := range req.Logs {
		if strings.TrimSpace(item.NodeID) == "" {
			item.NodeID = strings.TrimSpace(req.NodeID)
		}
		key := item.Key()
		row := service.QuotaLeaseDemoOpsErrorLogResult{
			Key:             key,
			RequestID:       strings.TrimSpace(item.RequestID),
			ClientRequestID: strings.TrimSpace(item.ClientRequestID),
		}
		if key == "" {
			row.Error = "error log identity is required"
			result.Results = append(result.Results, row)
			continue
		}
		entries = append(entries, item.ToOpsInsertErrorLogInput())
		result.Results = append(result.Results, row)
	}
	if len(entries) > 0 {
		if err := h.opsService.RecordErrorBatch(c.Request.Context(), entries); err != nil {
			for i := range result.Results {
				if strings.TrimSpace(result.Results[i].Error) == "" {
					result.Results[i].Error = err.Error()
				}
			}
			c.JSON(http.StatusOK, result)
			return
		}
		for i := range result.Results {
			if strings.TrimSpace(result.Results[i].Error) == "" {
				result.Results[i].Applied = true
			}
		}
	}
	c.JSON(http.StatusOK, result)
}

func (h *QuotaLeaseDemoHandler) ReclaimExpired(c *gin.Context) {
	if !h.requireEnabled(c) || !h.requireControlSecret(c) {
		return
	}
	now := time.Now().UTC()
	reclaim := h.svc.ReclaimExpired(c.Request.Context(), now)
	cleanup, err := h.svc.CleanupRetainedRecords(c.Request.Context(), now)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"expired_count":   reclaim.ExpiredCount,
		"reclaimed_count": reclaim.ReclaimedCount,
		"reclaimed_total": reclaim.ReclaimedTotal,
		"cleanup":         cleanup,
	})
}

func (h *QuotaLeaseDemoHandler) ExportUsageLedgerEvents(c *gin.Context) {
	if !h.requireEnabled(c) {
		return
	}
	requestedNodeID := strings.TrimSpace(c.Query("node_id"))
	nodeID, ok := h.authenticateNodeOrControl(c, requestedNodeID)
	if !ok {
		return
	}
	if nodeID == "" {
		nodeID = requestedNodeID
	}
	result, err := h.svc.ExportUsageLedgerEvents(
		c.Request.Context(),
		nodeID,
		c.Query("after_event_id"),
		quotaLeaseDemoHandlerReconcileLimit(c.Query("limit")),
	)
	if err != nil {
		h.writeError(c, err)
		return
	}
	c.JSON(http.StatusOK, result)
}

func (h *QuotaLeaseDemoHandler) ReconcileUsageLedgers(c *gin.Context) {
	if !h.requireEnabled(c) || !h.requireControlSecret(c) {
		return
	}
	nodeID := strings.TrimSpace(c.Param("node_id"))
	if nodeID == "" {
		nodeID = strings.TrimSpace(c.Query("node_id"))
	}
	if nodeID != "" {
		c.JSON(http.StatusOK, gin.H{"result": h.svc.ReconcileNodeUsageLedger(c.Request.Context(), nodeID)})
		return
	}
	c.JSON(http.StatusOK, gin.H{"results": h.svc.ReconcileUsageLedgers(c.Request.Context())})
}

func (h *QuotaLeaseDemoHandler) Status(c *gin.Context) {
	if !h.requireEnabled(c) || !h.requireControlSecret(c) {
		return
	}
	c.JSON(http.StatusOK, h.svc.Snapshot())
}

func (h *QuotaLeaseDemoHandler) requireEnabled(c *gin.Context) bool {
	if h == nil || h.svc == nil || !h.svc.Enabled() {
		c.JSON(http.StatusNotFound, gin.H{"error": "quota_lease_disabled"})
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

func quotaLeaseDemoHandlerReconcileLimit(raw string) int {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return 0
	}
	value, err := strconv.Atoi(raw)
	if err != nil || value <= 0 {
		return 0
	}
	return value
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
		c.JSON(http.StatusNotFound, gin.H{"error": "quota_lease_disabled"})
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
