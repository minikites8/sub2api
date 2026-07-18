package service

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"
)

const (
	quotaLeaseDemoControlPlanePrefix = "/api/v1/node-leases/demo"
	quotaLeaseDemoRemoteTimeout      = 5 * time.Second
)

type quotaLeaseDemoRemoteHTTPError struct {
	StatusCode int
	Body       string
}

func (e *quotaLeaseDemoRemoteHTTPError) Error() string {
	body := strings.TrimSpace(e.Body)
	if body == "" {
		return fmt.Sprintf("quota lease demo control plane returned status %d", e.StatusCode)
	}
	return fmt.Sprintf("quota lease demo control plane returned status %d: %s", e.StatusCode, body)
}

func (s *QuotaLeaseDemoService) remoteMode() bool {
	return s != nil && s.Enabled() && (strings.TrimSpace(s.ControlPlaneBaseURL()) != "" || strings.TrimSpace(s.RegistrationURL()) != "")
}

func (s *QuotaLeaseDemoService) ensureCapacity(ctx context.Context, nodeID string, userID, apiKeyID int64, amount float64) bool {
	if s == nil || !s.Enabled() || userID <= 0 || apiKeyID <= 0 || !finitePositive(amount) {
		return false
	}
	nodeID = strings.TrimSpace(nodeID)
	if nodeID == "" {
		nodeID = s.NodeID()
	}
	if s.hasCapacity(nodeID, userID, apiKeyID, amount, time.Now().UTC()) {
		return true
	}
	if !s.remoteMode() {
		return false
	}

	_, err := s.RequestLease(ctx, QuotaLeaseDemoLeaseRequest{
		NodeID:   "",
		UserID:   userID,
		APIKeyID: apiKeyID,
		Amount:   amount,
	})
	if err != nil {
		return false
	}
	return s.hasCapacity(nodeID, userID, apiKeyID, amount, time.Now().UTC())
}

func (s *QuotaLeaseDemoService) registerRemoteNode(ctx context.Context, req QuotaLeaseDemoNodeRegistrationRequest) (*QuotaLeaseDemoNodeRegistrationResult, error) {
	if s == nil || !s.Enabled() {
		return nil, ErrQuotaLeaseDemoDisabled
	}

	s.remoteMu.Lock()
	if s.remoteNodeID != "" && s.remoteNodeSecret != "" {
		node := &QuotaLeaseDemoNode{
			NodeID: s.remoteNodeID,
			Secret: s.remoteNodeSecret,
			Status: QuotaLeaseDemoNodeStatusOnline,
		}
		s.remoteMu.Unlock()
		return &QuotaLeaseDemoNodeRegistrationResult{Node: cloneQuotaLeaseDemoNode(node), NodeSecret: s.remoteNodeSecret}, nil
	}
	if strings.TrimSpace(req.NodeSecret) == "" {
		if strings.TrimSpace(s.remoteNodeSecret) == "" {
			generated, err := generateQuotaLeaseDemoNodeSecret()
			if err != nil {
				s.remoteMu.Unlock()
				return nil, err
			}
			s.remoteNodeSecret = generated
		}
		req.NodeSecret = s.remoteNodeSecret
	}
	s.remoteMu.Unlock()

	if strings.TrimSpace(req.NodeID) == "" {
		req.NodeID = s.NodeID()
	}
	var result QuotaLeaseDemoNodeRegistrationResult
	if registrationURL := strings.TrimSpace(s.RegistrationURL()); registrationURL != "" {
		if err := s.registerRemoteNodeWithURL(ctx, registrationURL, req, &result); err != nil {
			return nil, err
		}
	} else {
		if err := s.doRemoteJSON(ctx, http.MethodPost, "/nodes/register", "", s.ControlPlaneKey(), req, &result); err != nil {
			return nil, err
		}
	}
	if result.Node == nil || strings.TrimSpace(result.Node.NodeID) == "" || strings.TrimSpace(result.NodeSecret) == "" {
		return nil, fmt.Errorf("%w: invalid node registration response", ErrQuotaLeaseDemoInvalidInput)
	}
	s.remoteMu.Lock()
	s.remoteNodeID = strings.TrimSpace(result.Node.NodeID)
	s.remoteNodeSecret = strings.TrimSpace(result.NodeSecret)
	s.remoteMu.Unlock()
	result.Node.Secret = s.remoteNodeSecret
	s.cacheRemoteNode(result.Node)
	return &result, nil
}

func (s *QuotaLeaseDemoService) registerRemoteNodeWithURL(ctx context.Context, registrationURL string, req QuotaLeaseDemoNodeRegistrationRequest, result *QuotaLeaseDemoNodeRegistrationResult) error {
	endpoint, token, controlBaseURL, err := quotaLeaseDemoParseRegistrationURL(registrationURL)
	if err != nil {
		return err
	}
	if strings.TrimSpace(req.RegistrationToken) == "" {
		req.RegistrationToken = token
	}
	s.remoteMu.Lock()
	s.remoteControlURL = controlBaseURL
	s.remoteMu.Unlock()
	return s.doRemoteJSONToURL(ctx, http.MethodPost, endpoint, "", "", req, result)
}

func (s *QuotaLeaseDemoService) remoteNodeAuth(ctx context.Context) (string, string, error) {
	result, err := s.registerRemoteNode(ctx, QuotaLeaseDemoNodeRegistrationRequest{})
	if err != nil {
		return "", "", err
	}
	if result == nil || result.Node == nil {
		return "", "", fmt.Errorf("%w: node registration missing result", ErrQuotaLeaseDemoInvalidInput)
	}
	return strings.TrimSpace(result.Node.NodeID), strings.TrimSpace(result.NodeSecret), nil
}

func (s *QuotaLeaseDemoService) heartbeatRemoteNode(ctx context.Context, req QuotaLeaseDemoNodeHeartbeatRequest) (*QuotaLeaseDemoNode, error) {
	nodeID, secret, err := s.remoteNodeAuth(ctx)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(req.NodeID) == "" {
		req.NodeID = nodeID
	}
	var result struct {
		Node *QuotaLeaseDemoNode `json:"node"`
	}
	if err := s.doRemoteJSON(ctx, http.MethodPost, "/nodes/heartbeat", nodeID, secret, req, &result); err != nil {
		return nil, err
	}
	if result.Node != nil {
		result.Node.Secret = secret
		s.cacheRemoteNode(result.Node)
	}
	return cloneQuotaLeaseDemoNode(result.Node), nil
}

func (s *QuotaLeaseDemoService) requestRemoteLease(ctx context.Context, req QuotaLeaseDemoLeaseRequest) (*QuotaLeaseDemoLease, error) {
	nodeID, secret, err := s.remoteNodeAuth(ctx)
	if err != nil {
		return nil, err
	}
	req.NodeID = nodeID
	var result struct {
		Lease *QuotaLeaseDemoLease `json:"lease"`
	}
	if err := s.doRemoteJSON(ctx, http.MethodPost, "/leases/request", nodeID, secret, req, &result); err != nil {
		return nil, err
	}
	if result.Lease == nil {
		return nil, fmt.Errorf("%w: lease response missing lease", ErrQuotaLeaseDemoInvalidInput)
	}
	s.cacheRemoteLease(result.Lease)
	return cloneQuotaLeaseDemoLease(result.Lease), nil
}

func (s *QuotaLeaseDemoService) postRemoteUsageBatch(ctx context.Context, req QuotaLeaseDemoUsageBatchRequest) (QuotaLeaseDemoUsageBatchResult, error) {
	nodeID, secret, err := s.remoteNodeAuth(ctx)
	if err != nil {
		return QuotaLeaseDemoUsageBatchResult{}, err
	}
	if strings.TrimSpace(req.NodeID) == "" {
		req.NodeID = nodeID
	}
	for i := range req.Events {
		if strings.TrimSpace(req.Events[i].NodeID) == "" {
			req.Events[i].NodeID = req.NodeID
		}
	}
	var result QuotaLeaseDemoUsageBatchResult
	if err := s.doRemoteJSON(ctx, http.MethodPost, "/usage/batch", nodeID, secret, req, &result); err != nil {
		return QuotaLeaseDemoUsageBatchResult{}, err
	}
	return result, nil
}

func (s *QuotaLeaseDemoService) postRemoteUsageLogBatch(ctx context.Context, req QuotaLeaseDemoUsageLogBatchRequest) (QuotaLeaseDemoUsageLogBatchResult, error) {
	nodeID, secret, err := s.remoteNodeAuth(ctx)
	if err != nil {
		return QuotaLeaseDemoUsageLogBatchResult{}, err
	}
	if strings.TrimSpace(req.NodeID) == "" {
		req.NodeID = nodeID
	}
	for i := range req.Logs {
		if strings.TrimSpace(req.Logs[i].NodeID) == "" {
			req.Logs[i].NodeID = req.NodeID
		}
	}
	var result QuotaLeaseDemoUsageLogBatchResult
	if err := s.doRemoteJSON(ctx, http.MethodPost, "/usage-logs/batch", nodeID, secret, req, &result); err != nil {
		return QuotaLeaseDemoUsageLogBatchResult{}, err
	}
	return result, nil
}

type quotaLeaseDemoRemoteSettingsResponse struct {
	Data *QuotaLeaseDemoSettings `json:"data"`

	PrefetchLowWatermarkAmount *float64 `json:"prefetch_low_watermark_amount"`
	PrefetchAverageWindow      *int     `json:"prefetch_average_window"`
	PrefetchAverageMultiplier  *float64 `json:"prefetch_average_multiplier"`
	PrefetchDebounceSeconds    *int     `json:"prefetch_debounce_seconds"`
}

func (r quotaLeaseDemoRemoteSettingsResponse) settings() (*QuotaLeaseDemoSettings, error) {
	if r.Data != nil {
		return validateQuotaLeaseDemoSettings(r.Data)
	}
	patch := &QuotaLeaseDemoSettingsPatch{
		PrefetchLowWatermarkAmount: r.PrefetchLowWatermarkAmount,
		PrefetchAverageWindow:      r.PrefetchAverageWindow,
		PrefetchAverageMultiplier:  r.PrefetchAverageMultiplier,
		PrefetchDebounceSeconds:    r.PrefetchDebounceSeconds,
	}
	return validateQuotaLeaseDemoSettings(applyQuotaLeaseDemoSettingsPatch(defaultQuotaLeaseDemoSettings(), patch))
}

func (s *QuotaLeaseDemoService) fetchRemoteSettings(ctx context.Context) (*QuotaLeaseDemoSettings, error) {
	nodeID, secret, err := s.remoteNodeAuth(ctx)
	if err != nil {
		return nil, err
	}
	var result quotaLeaseDemoRemoteSettingsResponse
	if err := s.doRemoteJSON(ctx, http.MethodGet, "/settings", nodeID, secret, nil, &result); err != nil {
		return nil, err
	}
	return result.settings()
}

func (s *QuotaLeaseDemoService) createRemoteAccountLoginTask(ctx context.Context, req QuotaLeaseDemoAccountLoginTaskCreateRequest) (*QuotaLeaseDemoAccountLoginTask, error) {
	var result struct {
		Task *QuotaLeaseDemoAccountLoginTask `json:"task"`
	}
	if err := s.doRemoteJSON(ctx, http.MethodPost, "/accounts/login-tasks", "", s.ControlPlaneKey(), req, &result); err != nil {
		return nil, err
	}
	if result.Task == nil {
		return nil, fmt.Errorf("%w: account login task response missing task", ErrQuotaLeaseDemoInvalidInput)
	}
	return result.Task, nil
}

func (s *QuotaLeaseDemoService) fetchRemoteAccountLoginTasks(ctx context.Context, status string) ([]QuotaLeaseDemoAccountLoginTask, error) {
	nodeID, secret, err := s.remoteNodeAuth(ctx)
	if err != nil {
		return nil, err
	}
	endpoint := "/accounts/login-tasks"
	if strings.TrimSpace(status) != "" {
		endpoint += "?status=" + url.QueryEscape(strings.TrimSpace(status))
	}
	var result struct {
		Tasks []QuotaLeaseDemoAccountLoginTask `json:"tasks"`
	}
	if err := s.doRemoteJSON(ctx, http.MethodGet, endpoint, nodeID, secret, nil, &result); err != nil {
		return nil, err
	}
	return result.Tasks, nil
}

func (s *QuotaLeaseDemoService) completeRemoteAccountLoginTask(ctx context.Context, req QuotaLeaseDemoAccountLoginTaskCompleteRequest) (*QuotaLeaseDemoAccountLoginTask, error) {
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
		Task *QuotaLeaseDemoAccountLoginTask `json:"task"`
	}
	endpoint := "/accounts/login-tasks/" + url.PathEscape(strings.TrimSpace(req.TaskID)) + "/complete"
	if err := s.doRemoteJSON(ctx, http.MethodPost, endpoint, nodeID, secret, req, &result); err != nil {
		return nil, err
	}
	if result.Task == nil {
		return nil, fmt.Errorf("%w: account login completion response missing task", ErrQuotaLeaseDemoInvalidInput)
	}
	if result.Task.Account != nil {
		s.upsertRemoteMirrorAccountBestEffort(ctx, *result.Task.Account, "account_login_complete")
	}
	return result.Task, nil
}

func (s *QuotaLeaseDemoService) reportRemoteAccountLoginTaskProgress(ctx context.Context, req QuotaLeaseDemoAccountLoginTaskProgressRequest) (*QuotaLeaseDemoAccountLoginTask, error) {
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
		Task *QuotaLeaseDemoAccountLoginTask `json:"task"`
	}
	endpoint := "/accounts/login-tasks/" + url.PathEscape(strings.TrimSpace(req.TaskID)) + "/progress"
	if err := s.doRemoteJSON(ctx, http.MethodPost, endpoint, nodeID, secret, req, &result); err != nil {
		return nil, err
	}
	if result.Task == nil {
		return nil, fmt.Errorf("%w: account login progress response missing task", ErrQuotaLeaseDemoInvalidInput)
	}
	return result.Task, nil
}

func (s *QuotaLeaseDemoService) submitRemoteAccountLoginTaskCallback(ctx context.Context, req QuotaLeaseDemoAccountLoginTaskCallbackRequest) (*QuotaLeaseDemoAccountLoginTask, error) {
	if strings.TrimSpace(req.TaskID) == "" {
		return nil, fmt.Errorf("%w: task_id is required", ErrQuotaLeaseDemoInvalidInput)
	}
	var result struct {
		Task *QuotaLeaseDemoAccountLoginTask `json:"task"`
	}
	endpoint := "/accounts/login-tasks/" + url.PathEscape(strings.TrimSpace(req.TaskID)) + "/callback"
	if err := s.doRemoteJSON(ctx, http.MethodPost, endpoint, "", s.ControlPlaneKey(), req, &result); err != nil {
		return nil, err
	}
	if result.Task == nil {
		return nil, fmt.Errorf("%w: account login callback response missing task", ErrQuotaLeaseDemoInvalidInput)
	}
	return result.Task, nil
}

func (s *QuotaLeaseDemoService) reportRemoteAccountStatus(ctx context.Context, req QuotaLeaseDemoAccountStatusReportRequest) (*QuotaLeaseDemoAssignedAccount, error) {
	nodeID, secret, err := s.remoteNodeAuth(ctx)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(req.NodeID) == "" {
		req.NodeID = nodeID
	}
	var result struct {
		Account *QuotaLeaseDemoAssignedAccount `json:"account"`
	}
	if err := s.doRemoteJSON(ctx, http.MethodPost, "/accounts/status", nodeID, secret, req, &result); err != nil {
		return nil, err
	}
	if result.Account == nil {
		return nil, fmt.Errorf("%w: account status response missing account", ErrQuotaLeaseDemoInvalidInput)
	}
	s.cacheRemoteAssignedAccount(nodeID, result.Account)
	s.upsertRemoteMirrorAccountBestEffort(ctx, result.Account.Account, "account_status")
	return cloneQuotaLeaseDemoAssignedAccount(result.Account), nil
}

func (s *QuotaLeaseDemoService) SyncAssignedAccounts(ctx context.Context) error {
	if s == nil || !s.remoteMode() {
		return nil
	}
	if s.quotaLeaseDemoMirrorStore() != nil {
		return s.SyncMirrorSnapshot(ctx)
	}
	nodeID, secret, err := s.remoteNodeAuth(ctx)
	if err != nil {
		return err
	}
	var result struct {
		Accounts []QuotaLeaseDemoAssignedAccount `json:"accounts"`
	}
	if err := s.doRemoteJSON(ctx, http.MethodGet, "/accounts/assignments", nodeID, secret, nil, &result); err != nil {
		return err
	}
	s.cacheRemoteAssignedAccounts(nodeID, result.Accounts)
	return nil
}

func (s *QuotaLeaseDemoService) upsertRemoteMirrorAccountBestEffort(ctx context.Context, account QuotaLeaseDemoAccountSnapshot, source string) {
	store := s.quotaLeaseDemoMirrorStore()
	if store == nil || account.ID <= 0 {
		return
	}
	if nodeID := s.activeNodeID(); strings.TrimSpace(nodeID) != "" {
		if account.Extra == nil {
			account.Extra = make(map[string]any)
		}
		if strings.TrimSpace(quotaLeaseDemoStringFromPayload(account.Extra["node_oauth_assigned_node_id"])) == "" {
			account.Extra["node_oauth_assigned_node_id"] = strings.TrimSpace(nodeID)
		}
	}
	if err := store.UpsertAccount(ctx, account); err != nil {
		slog.Warn("quota_lease_demo.mirror_account_upsert_failed",
			"account_id", account.ID,
			"source", strings.TrimSpace(source),
			"error", err,
		)
		return
	}
	s.markMirrorSynced(time.Now().UTC())
}

func (s *QuotaLeaseDemoService) activeNodeID() string {
	if s == nil {
		return ""
	}
	s.remoteMu.Lock()
	remoteNodeID := strings.TrimSpace(s.remoteNodeID)
	s.remoteMu.Unlock()
	if remoteNodeID != "" {
		return remoteNodeID
	}
	return s.NodeID()
}

func (s *QuotaLeaseDemoService) FlushPendingUsage(ctx context.Context) error {
	if s == nil || !s.remoteMode() {
		return nil
	}
	events := s.pendingUsageEvents()
	if len(events) == 0 {
		return nil
	}
	nodeID, _, err := s.remoteNodeAuth(ctx)
	if err != nil {
		return err
	}
	result, err := s.postRemoteUsageBatch(ctx, QuotaLeaseDemoUsageBatchRequest{
		NodeID: nodeID,
		Events: events,
	})
	if err != nil {
		return err
	}
	s.removePendingUsageResults(result)
	return nil
}

func (s *QuotaLeaseDemoService) FlushPendingUsageLogs(ctx context.Context) error {
	if s == nil || !s.remoteMode() {
		return nil
	}
	logs := s.pendingUsageLogSnapshots()
	if len(logs) == 0 {
		return nil
	}
	nodeID, _, err := s.remoteNodeAuth(ctx)
	if err != nil {
		return err
	}
	result, err := s.postRemoteUsageLogBatch(ctx, QuotaLeaseDemoUsageLogBatchRequest{
		NodeID: nodeID,
		Logs:   logs,
	})
	if err != nil {
		return err
	}
	s.removePendingUsageLogResults(result)
	return nil
}

func (s *QuotaLeaseDemoService) flushPendingUsageAsync() {
	if s == nil || !s.remoteMode() {
		return
	}
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), quotaLeaseDemoRemoteTimeout)
		defer cancel()
		_ = s.FlushPendingUsage(ctx)
	}()
}

func (s *QuotaLeaseDemoService) flushPendingUsageLogsAsync() {
	if s == nil || !s.remoteMode() {
		return
	}
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), quotaLeaseDemoRemoteTimeout)
		defer cancel()
		_ = s.FlushPendingUsageLogs(ctx)
	}()
}

func (s *QuotaLeaseDemoService) enqueuePendingUsageEvent(event QuotaLeaseDemoUsageEvent) {
	if s == nil {
		return
	}
	event.EventID = strings.TrimSpace(event.EventID)
	event.LeaseID = strings.TrimSpace(event.LeaseID)
	event.NodeID = strings.TrimSpace(event.NodeID)
	event.RequestID = strings.TrimSpace(event.RequestID)
	event.EventType = strings.TrimSpace(event.EventType)
	if event.EventID == "" || event.LeaseID == "" {
		return
	}
	if event.NodeID == "" {
		event.NodeID = s.NodeID()
	}
	if event.EventType == "" {
		event.EventType = QuotaLeaseDemoEventUsagePosted
	}
	if event.CreatedAt.IsZero() {
		event.CreatedAt = time.Now().UTC()
	}

	s.mu.Lock()
	if s.pendingEvents == nil {
		s.pendingEvents = make(map[string]QuotaLeaseDemoUsageEvent)
	}
	s.pendingEvents[event.EventID] = event
	s.mu.Unlock()
}

func (s *QuotaLeaseDemoService) pendingUsageEvents() []QuotaLeaseDemoUsageEvent {
	if s == nil {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	events := make([]QuotaLeaseDemoUsageEvent, 0, len(s.pendingEvents))
	for _, event := range s.pendingEvents {
		events = append(events, event)
	}
	sort.Slice(events, func(i, j int) bool {
		return events[i].CreatedAt.Before(events[j].CreatedAt)
	})
	return events
}

func (s *QuotaLeaseDemoService) removePendingUsageResults(result QuotaLeaseDemoUsageBatchResult) {
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
			delete(s.pendingEvents, strings.TrimSpace(item.EventID))
		}
	}
}

func (s *QuotaLeaseDemoService) cacheRemoteLease(lease *QuotaLeaseDemoLease) {
	if s == nil || lease == nil {
		return
	}
	copy := *lease
	s.mu.Lock()
	if s.leases == nil {
		s.leases = make(map[string]*QuotaLeaseDemoLease)
	}
	if s.events == nil {
		s.events = make(map[string]*QuotaLeaseDemoLedgerEvent)
	}
	if existing := s.leases[copy.ID]; existing != nil {
		if existing.Consumed > copy.Consumed {
			copy.Consumed = existing.Consumed
		}
		if existing.Reclaimed > copy.Reclaimed {
			copy.Reclaimed = existing.Reclaimed
		}
	}
	s.leases[copy.ID] = &copy
	eventID := "lease:" + copy.ID
	if s.events[eventID] == nil {
		s.events[eventID] = &QuotaLeaseDemoLedgerEvent{
			EventID:     eventID,
			LeaseID:     copy.ID,
			NodeID:      copy.NodeID,
			UserID:      copy.UserID,
			APIKeyID:    copy.APIKeyID,
			Amount:      copy.Granted,
			EventType:   QuotaLeaseDemoEventLeaseGranted,
			PayloadHash: quotaLeaseDemoPayloadHash(copy.ID, copy.NodeID, copy.UserID, copy.APIKeyID, "", copy.Granted, QuotaLeaseDemoEventLeaseGranted),
			CreatedAt:   copy.CreatedAt,
		}
	}
	s.mu.Unlock()
}

func (s *QuotaLeaseDemoService) cacheRemoteNode(node *QuotaLeaseDemoNode) {
	if s == nil || node == nil {
		return
	}
	copy := cloneQuotaLeaseDemoNode(node)
	if copy == nil {
		return
	}
	s.mu.Lock()
	if s.nodes == nil {
		s.nodes = make(map[string]*QuotaLeaseDemoNode)
	}
	s.nodes[copy.NodeID] = copy
	s.mu.Unlock()
}

func (s *QuotaLeaseDemoService) cacheRemoteAssignedAccounts(nodeID string, accounts []QuotaLeaseDemoAssignedAccount) {
	if s == nil {
		return
	}
	nodeID = strings.TrimSpace(nodeID)
	s.mu.Lock()
	if s.assignedAccounts == nil {
		s.assignedAccounts = make(map[int64]*QuotaLeaseDemoAssignedAccount)
	}
	for id, assigned := range s.assignedAccounts {
		if nodeID == "" || assigned == nil || assigned.NodeID == nodeID {
			delete(s.assignedAccounts, id)
		}
	}
	for _, account := range accounts {
		cloned := cloneQuotaLeaseDemoAssignedAccount(&account)
		if cloned == nil || cloned.Account.ID <= 0 {
			continue
		}
		if cloned.NodeID == "" {
			cloned.NodeID = nodeID
		}
		s.assignedAccounts[cloned.Account.ID] = cloned
	}
	s.mu.Unlock()
}

func (s *QuotaLeaseDemoService) cacheRemoteAssignedAccount(nodeID string, account *QuotaLeaseDemoAssignedAccount) {
	if s == nil || account == nil {
		return
	}
	cloned := cloneQuotaLeaseDemoAssignedAccount(account)
	if cloned == nil || cloned.Account.ID <= 0 {
		return
	}
	nodeID = strings.TrimSpace(nodeID)
	if cloned.NodeID == "" {
		cloned.NodeID = nodeID
	}
	s.mu.Lock()
	if s.assignedAccounts == nil {
		s.assignedAccounts = make(map[int64]*QuotaLeaseDemoAssignedAccount)
	}
	s.assignedAccounts[cloned.Account.ID] = cloned
	s.mu.Unlock()
}

func (s *QuotaLeaseDemoService) doRemoteJSON(ctx context.Context, method, endpoint, nodeID, secret string, input any, output any) error {
	fullURL, err := quotaLeaseDemoRemoteEndpointURL(s.ControlPlaneBaseURL(), endpoint)
	if err != nil {
		return err
	}
	return s.doRemoteJSONToURL(ctx, method, fullURL, nodeID, secret, input, output)
}

func (s *QuotaLeaseDemoService) doRemoteJSONToURL(ctx context.Context, method, fullURL, nodeID, secret string, input any, output any) error {
	if s == nil || !s.Enabled() {
		return ErrQuotaLeaseDemoDisabled
	}
	if ctx == nil {
		ctx = context.Background()
	}
	reqCtx, cancel := context.WithTimeout(ctx, quotaLeaseDemoRemoteTimeout)
	defer cancel()

	var reqBody io.Reader
	if input != nil {
		payload, err := json.Marshal(input)
		if err != nil {
			return err
		}
		reqBody = bytes.NewReader(payload)
	}
	req, err := http.NewRequestWithContext(reqCtx, method, fullURL, reqBody)
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	if strings.TrimSpace(nodeID) != "" {
		req.Header.Set("X-Node-ID", strings.TrimSpace(nodeID))
	}
	if strings.TrimSpace(secret) != "" {
		req.Header.Set("X-Node-Secret", strings.TrimSpace(secret))
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return err
	}
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return &quotaLeaseDemoRemoteHTTPError{StatusCode: resp.StatusCode, Body: string(body)}
	}
	if output == nil || len(body) == 0 {
		return nil
	}
	if err := json.Unmarshal(body, output); err != nil {
		return err
	}
	return nil
}

func quotaLeaseDemoRemoteEndpointURL(baseURL, endpoint string) (string, error) {
	base := strings.TrimRight(strings.TrimSpace(baseURL), "/")
	endpoint = "/" + strings.TrimLeft(strings.TrimSpace(endpoint), "/")
	if base == "" {
		return "", fmt.Errorf("%w: control_plane_base_url is required", ErrQuotaLeaseDemoInvalidInput)
	}
	if strings.HasSuffix(base, quotaLeaseDemoControlPlanePrefix) || strings.HasSuffix(base, "/node-leases/demo") {
		return base + endpoint, nil
	}
	return base + quotaLeaseDemoControlPlanePrefix + endpoint, nil
}

func quotaLeaseDemoBuildRegistrationURL(externalBaseURL, token string) (string, error) {
	token = strings.TrimSpace(token)
	if token == "" {
		return "", fmt.Errorf("%w: registration token is required", ErrQuotaLeaseDemoInvalidInput)
	}
	endpoint, err := quotaLeaseDemoRemoteEndpointURL(externalBaseURL, "/nodes/register")
	if err != nil {
		return "", err
	}
	parsed, err := url.Parse(endpoint)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return "", fmt.Errorf("%w: registration url base is invalid", ErrQuotaLeaseDemoInvalidInput)
	}
	query := parsed.Query()
	query.Set("registration_token", token)
	parsed.RawQuery = query.Encode()
	return parsed.String(), nil
}

func quotaLeaseDemoParseRegistrationURL(registrationURL string) (endpointURL string, token string, controlBaseURL string, err error) {
	parsed, err := url.Parse(strings.TrimSpace(registrationURL))
	if err != nil || parsed.Scheme == "" || parsed.Host == "" || (parsed.Scheme != "http" && parsed.Scheme != "https") {
		return "", "", "", fmt.Errorf("%w: registration_url must be a valid http(s) URL", ErrQuotaLeaseDemoInvalidInput)
	}
	token = strings.TrimSpace(parsed.Query().Get("registration_token"))
	if token == "" {
		return "", "", "", fmt.Errorf("%w: registration_url missing registration_token", ErrQuotaLeaseDemoInvalidInput)
	}
	endpointURL = parsed.String()
	basePath := strings.TrimRight(parsed.Path, "/")
	if strings.HasSuffix(basePath, "/nodes/register") {
		basePath = strings.TrimSuffix(basePath, "/nodes/register")
	}
	control := url.URL{
		Scheme: parsed.Scheme,
		Host:   parsed.Host,
		Path:   basePath,
	}
	return endpointURL, token, control.String(), nil
}
