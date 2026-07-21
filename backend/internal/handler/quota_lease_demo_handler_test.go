package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sort"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type quotaLeaseDemoUsageRepoStub struct {
	service.UsageLogRepository
	mu       sync.Mutex
	inserted map[string]bool
	calls    []service.UsageLog
}

func (r *quotaLeaseDemoUsageRepoStub) Create(_ context.Context, log *service.UsageLog) (bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.inserted == nil {
		r.inserted = make(map[string]bool)
	}
	if log == nil {
		return false, nil
	}
	key := strings.TrimSpace(log.RequestID) + "\x1f" + strconv.FormatInt(log.APIKeyID, 10)
	r.calls = append(r.calls, *log)
	if r.inserted[key] {
		return false, nil
	}
	r.inserted[key] = true
	return true, nil
}

type quotaLeaseDemoOpsRepoStub struct {
	service.OpsRepository
	mu      sync.Mutex
	entries []*service.OpsInsertErrorLogInput
}

func (r *quotaLeaseDemoOpsRepoStub) InsertErrorLog(_ context.Context, input *service.OpsInsertErrorLogInput) (int64, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.entries = append(r.entries, input)
	return int64(len(r.entries)), nil
}

func (r *quotaLeaseDemoOpsRepoStub) BatchInsertErrorLogs(_ context.Context, inputs []*service.OpsInsertErrorLogInput) (int64, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.entries = append(r.entries, inputs...)
	return int64(len(inputs)), nil
}

type quotaLeaseDemoUserRepoStub struct {
	service.UserRepository
	mu             sync.Mutex
	user           *service.User
	balanceUpdates []quotaLeaseDemoBalanceUpdate
}

type quotaLeaseDemoBalanceUpdate struct {
	userID int64
	amount float64
}

func (r *quotaLeaseDemoUserRepoStub) GetByID(_ context.Context, id int64) (*service.User, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.user != nil {
		user := *r.user
		user.ID = id
		return &user, nil
	}
	return &service.User{ID: id, Status: service.StatusActive}, nil
}

func (r *quotaLeaseDemoUserRepoStub) UpdateBalance(_ context.Context, id int64, amount float64) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.balanceUpdates = append(r.balanceUpdates, quotaLeaseDemoBalanceUpdate{userID: id, amount: amount})
	return nil
}

type quotaLeaseDemoSyncAdminService struct {
	service.AdminService
	updatedID        int64
	input            *service.UpdateAccountInput
	updatedExtraID   int64
	updatedExtra     map[string]any
	clearedAccountID int64
	listedAccounts   []service.Account
	listedNodeIDs    []string
	groups           []service.Group
	allProxies       []service.Proxy
	proxies          map[int64]*service.Proxy
	users            map[int64]*service.User
}

func (s *quotaLeaseDemoSyncAdminService) UpdateAccount(_ context.Context, id int64, input *service.UpdateAccountInput) (*service.Account, error) {
	s.updatedID = id
	s.input = input
	return &service.Account{ID: id, Status: input.Status, Credentials: input.Credentials, Extra: input.Extra}, nil
}

func (s *quotaLeaseDemoSyncAdminService) UpdateAccountExtra(_ context.Context, id int64, updates map[string]any) error {
	s.updatedExtraID = id
	s.updatedExtra = updates
	return nil
}

func (s *quotaLeaseDemoSyncAdminService) ClearAccountError(_ context.Context, id int64) (*service.Account, error) {
	s.clearedAccountID = id
	return &service.Account{ID: id, Status: service.StatusActive}, nil
}

func (s *quotaLeaseDemoSyncAdminService) ListAccounts(_ context.Context, page, pageSize int, platform, accountType, status, search string, groupID int64, privacyMode string, sortBy, sortOrder string) ([]service.Account, int64, error) {
	start := (page - 1) * pageSize
	if start >= len(s.listedAccounts) {
		return nil, int64(len(s.listedAccounts)), nil
	}
	end := start + pageSize
	if end > len(s.listedAccounts) {
		end = len(s.listedAccounts)
	}
	return append([]service.Account(nil), s.listedAccounts[start:end]...), int64(len(s.listedAccounts)), nil
}

func (s *quotaLeaseDemoSyncAdminService) ListNodeAssignedAccounts(_ context.Context, nodeID string, page, pageSize int) ([]service.Account, int64, error) {
	nodeID = strings.TrimSpace(nodeID)
	s.listedNodeIDs = append(s.listedNodeIDs, nodeID)
	filtered := make([]service.Account, 0, len(s.listedAccounts))
	for _, account := range s.listedAccounts {
		if service.QuotaLeaseDemoAccountAssignedToNode(account, nodeID) {
			filtered = append(filtered, account)
		}
	}
	start := (page - 1) * pageSize
	if start >= len(filtered) {
		return nil, int64(len(filtered)), nil
	}
	end := start + pageSize
	if end > len(filtered) {
		end = len(filtered)
	}
	return append([]service.Account(nil), filtered[start:end]...), int64(len(filtered)), nil
}

func (s *quotaLeaseDemoSyncAdminService) GetAllGroupsIncludingInactive(context.Context) ([]service.Group, error) {
	return append([]service.Group(nil), s.groups...), nil
}

func (s *quotaLeaseDemoSyncAdminService) GetAllProxies(context.Context) ([]service.Proxy, error) {
	if s.allProxies != nil {
		return append([]service.Proxy(nil), s.allProxies...), nil
	}
	out := make([]service.Proxy, 0, len(s.proxies))
	for _, proxy := range s.proxies {
		if proxy != nil {
			out = append(out, *proxy)
		}
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].ID < out[j].ID
	})
	return out, nil
}

func (s *quotaLeaseDemoSyncAdminService) GetProxy(_ context.Context, id int64) (*service.Proxy, error) {
	if s.proxies == nil {
		return nil, nil
	}
	return s.proxies[id], nil
}

func (s *quotaLeaseDemoSyncAdminService) GetUserIncludeDeleted(_ context.Context, id int64) (*service.User, error) {
	if s.users == nil {
		return nil, nil
	}
	user := s.users[id]
	if user == nil {
		return nil, nil
	}
	value := *user
	return &value, nil
}

type quotaLeaseDemoBillingRepoStub struct {
	service.UsageBillingRepository
	reserveCalls int
	reserveErr   error
}

func (r *quotaLeaseDemoBillingRepoStub) ReserveBalanceHold(context.Context, *service.BalanceHoldCommand) (*service.BalanceHoldResult, error) {
	r.reserveCalls++
	if r.reserveErr != nil {
		return nil, r.reserveErr
	}
	return &service.BalanceHoldResult{Applied: true}, nil
}

func (r *quotaLeaseDemoBillingRepoStub) CaptureBalanceHold(context.Context, *service.BalanceHoldCommand) (*service.BalanceHoldResult, error) {
	return &service.BalanceHoldResult{Applied: true}, nil
}

func (r *quotaLeaseDemoBillingRepoStub) ReleaseBalanceHold(context.Context, *service.BalanceHoldCommand) (*service.BalanceHoldResult, error) {
	return &service.BalanceHoldResult{Applied: true}, nil
}

func (r *quotaLeaseDemoBillingRepoStub) ReserveBatchImageBalance(context.Context, *service.BatchImageBalanceHoldCommand) (*service.BatchImageBalanceHoldResult, error) {
	return &service.BatchImageBalanceHoldResult{Applied: true}, nil
}

func (r *quotaLeaseDemoBillingRepoStub) CaptureBatchImageBalance(context.Context, *service.BatchImageBalanceHoldCommand) (*service.BatchImageBalanceHoldResult, error) {
	return &service.BatchImageBalanceHoldResult{Applied: true}, nil
}

func (r *quotaLeaseDemoBillingRepoStub) ReleaseBatchImageBalance(context.Context, *service.BatchImageBalanceHoldCommand) (*service.BatchImageBalanceHoldResult, error) {
	return &service.BatchImageBalanceHoldResult{Applied: true}, nil
}

type quotaLeaseDemoAPIKeyRepoStub struct {
	service.APIKeyRepository
	apiKey     *service.APIKey
	apiKeyByID *service.APIKey
}

func (r *quotaLeaseDemoAPIKeyRepoStub) Create(context.Context, *service.APIKey) error {
	panic("unexpected Create call")
}

func (r *quotaLeaseDemoAPIKeyRepoStub) GetByID(context.Context, int64) (*service.APIKey, error) {
	if r.apiKeyByID != nil {
		return r.apiKeyByID, nil
	}
	return r.apiKey, nil
}

func (r *quotaLeaseDemoAPIKeyRepoStub) GetKeyAndOwnerID(context.Context, int64) (string, int64, error) {
	panic("unexpected GetKeyAndOwnerID call")
}

func (r *quotaLeaseDemoAPIKeyRepoStub) GetByKey(context.Context, string) (*service.APIKey, error) {
	return r.apiKey, nil
}

func (r *quotaLeaseDemoAPIKeyRepoStub) GetByKeyForAuth(context.Context, string) (*service.APIKey, error) {
	return r.apiKey, nil
}

func (r *quotaLeaseDemoAPIKeyRepoStub) Update(context.Context, *service.APIKey) error {
	panic("unexpected Update call")
}

func (r *quotaLeaseDemoAPIKeyRepoStub) Delete(context.Context, int64) error {
	panic("unexpected Delete call")
}

func (r *quotaLeaseDemoAPIKeyRepoStub) DeleteWithAudit(context.Context, int64) error {
	panic("unexpected DeleteWithAudit call")
}

func (r *quotaLeaseDemoAPIKeyRepoStub) ListByUserID(context.Context, int64, pagination.PaginationParams, service.APIKeyListFilters) ([]service.APIKey, *pagination.PaginationResult, error) {
	panic("unexpected ListByUserID call")
}

func (r *quotaLeaseDemoAPIKeyRepoStub) VerifyOwnership(context.Context, int64, []int64) ([]int64, error) {
	panic("unexpected VerifyOwnership call")
}

func (r *quotaLeaseDemoAPIKeyRepoStub) CountByUserID(context.Context, int64) (int64, error) {
	panic("unexpected CountByUserID call")
}

func (r *quotaLeaseDemoAPIKeyRepoStub) ExistsByKey(context.Context, string) (bool, error) {
	panic("unexpected ExistsByKey call")
}

func (r *quotaLeaseDemoAPIKeyRepoStub) ListByGroupID(context.Context, int64, pagination.PaginationParams) ([]service.APIKey, *pagination.PaginationResult, error) {
	panic("unexpected ListByGroupID call")
}

func (r *quotaLeaseDemoAPIKeyRepoStub) SearchAPIKeys(context.Context, int64, string, int) ([]service.APIKey, error) {
	panic("unexpected SearchAPIKeys call")
}

func (r *quotaLeaseDemoAPIKeyRepoStub) ClearGroupIDByGroupID(context.Context, int64) (int64, error) {
	panic("unexpected ClearGroupIDByGroupID call")
}

func (r *quotaLeaseDemoAPIKeyRepoStub) UpdateGroupIDByUserAndGroup(context.Context, int64, int64, int64) (int64, error) {
	panic("unexpected UpdateGroupIDByUserAndGroup call")
}

func (r *quotaLeaseDemoAPIKeyRepoStub) CountByGroupID(context.Context, int64) (int64, error) {
	panic("unexpected CountByGroupID call")
}

func (r *quotaLeaseDemoAPIKeyRepoStub) ListKeysByUserID(context.Context, int64) ([]string, error) {
	panic("unexpected ListKeysByUserID call")
}

func (r *quotaLeaseDemoAPIKeyRepoStub) ListKeysByGroupID(context.Context, int64) ([]string, error) {
	panic("unexpected ListKeysByGroupID call")
}

func (r *quotaLeaseDemoAPIKeyRepoStub) IncrementQuotaUsed(context.Context, int64, float64) (float64, error) {
	panic("unexpected IncrementQuotaUsed call")
}

func (r *quotaLeaseDemoAPIKeyRepoStub) UpdateLastUsed(context.Context, int64, time.Time) error {
	panic("unexpected UpdateLastUsed call")
}

func (r *quotaLeaseDemoAPIKeyRepoStub) IncrementRateLimitUsage(context.Context, int64, float64) error {
	panic("unexpected IncrementRateLimitUsage call")
}

func (r *quotaLeaseDemoAPIKeyRepoStub) ResetRateLimitWindows(context.Context, int64) error {
	panic("unexpected ResetRateLimitWindows call")
}

func (r *quotaLeaseDemoAPIKeyRepoStub) GetRateLimitData(context.Context, int64) (*service.APIKeyRateLimitData, error) {
	panic("unexpected GetRateLimitData call")
}

func newQuotaLeaseDemoHandlerTestRouter(t *testing.T) (*gin.Engine, *service.QuotaLeaseDemoService) {
	t.Helper()
	gin.SetMode(gin.TestMode)

	svc := service.NewQuotaLeaseDemoService(&config.Config{
		Gateway: config.GatewayConfig{
			QuotaLeaseDemo: config.GatewayQuotaLeaseDemoConfig{
				Enabled:                true,
				NodeID:                 "control-node",
				NodeSecret:             "control-secret",
				DefaultGrantAmount:     1,
				LeaseTTLSeconds:        600,
				ReclaimGraceSeconds:    3600,
				PreflightReserveAmount: 0.000001,
			},
		},
	})
	h := NewQuotaLeaseDemoHandler(svc)
	router := gin.New()
	group := router.Group("/api/v1/node-leases/demo")
	{
		group.POST("/nodes/registration-urls", h.CreateNodeRegistrationURL)
		group.POST("/nodes/register", h.RegisterNode)
		group.POST("/nodes/heartbeat", h.HeartbeatNode)
		group.GET("/nodes", h.ListNodes)
		group.PUT("/nodes/:node_id", h.UpdateNode)
		group.POST("/accounts/login-tasks", h.CreateAccountLoginTask)
		group.GET("/accounts/login-tasks", h.ListAccountLoginTasks)
		group.POST("/accounts/login-tasks/:task_id/complete", h.CompleteAccountLoginTask)
		group.POST("/accounts/login-tasks/:task_id/progress", h.ReportAccountLoginTaskProgress)
		group.POST("/accounts/login-tasks/:task_id/callback", h.SubmitAccountLoginTaskCallback)
		group.POST("/accounts/usage-probe-tasks", h.CreateUsageProbeTask)
		group.GET("/accounts/usage-probe-tasks", h.ListUsageProbeTasks)
		group.POST("/accounts/usage-probe-tasks/:task_id/complete", h.CompleteUsageProbeTask)
		group.POST("/accounts/status", h.ReportAccountStatus)
		group.GET("/accounts/assignments", h.ListAssignedAccounts)
		group.POST("/leases/request", h.RequestLease)
		group.POST("/usage/batch", h.PostUsageBatch)
		group.POST("/ops-error-logs/batch", h.PostOpsErrorLogBatch)
		group.GET("/diagnostics", h.Diagnostics)
		group.GET("/status", h.Status)
	}
	return router, svc
}

func quotaLeaseDemoJSONRequest(t *testing.T, method, path string, body any) *http.Request {
	t.Helper()
	payload, err := json.Marshal(body)
	require.NoError(t, err)
	req := httptest.NewRequest(method, path, bytes.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")
	return req
}

func TestQuotaLeaseDemoHandlerSyncsCompletedAccountToAdminService(t *testing.T) {
	adminSvc := &quotaLeaseDemoSyncAdminService{}
	h := NewQuotaLeaseDemoHandler(nil, adminSvc)
	task := &service.QuotaLeaseDemoAccountLoginTask{
		ID:             "task-1",
		AccountID:      707,
		Name:           "node-openai",
		Type:           service.AccountTypeOAuth,
		AssignedNodeID: "foreign-1",
		Concurrency:    2,
		Priority:       5,
		Status:         service.QuotaLeaseDemoAccountTaskCompleted,
		Account: &service.QuotaLeaseDemoAccountSnapshot{
			ID:          707,
			Name:        "node-openai",
			Platform:    service.PlatformOpenAI,
			Type:        service.AccountTypeOAuth,
			Credentials: map[string]any{"access_token": "node-access"},
			Extra:       map[string]any{"openai_long_context_billing_enabled": false},
			Concurrency: 3,
			Priority:    9,
			GroupIDs:    []int64{10, 20},
		},
	}

	require.NoError(t, h.syncCompletedAccount(context.Background(), task))

	require.Equal(t, int64(707), adminSvc.updatedID)
	require.NotNil(t, adminSvc.input)
	require.Equal(t, service.StatusActive, adminSvc.input.Status)
	require.Equal(t, service.AccountTypeOAuth, adminSvc.input.Type)
	require.Equal(t, "node-access", adminSvc.input.Credentials["access_token"])
	require.Equal(t, false, adminSvc.input.Extra["openai_long_context_billing_enabled"])
	require.Equal(t, service.QuotaLeaseDemoAccountTaskCompleted, adminSvc.input.Extra["node_oauth_status"])
	require.Equal(t, "foreign-1", adminSvc.input.Extra["node_oauth_assigned_node_id"])
	require.NotNil(t, adminSvc.input.Concurrency)
	require.Equal(t, 3, *adminSvc.input.Concurrency)
	require.NotNil(t, adminSvc.input.Priority)
	require.Equal(t, 9, *adminSvc.input.Priority)
	require.NotNil(t, adminSvc.input.GroupIDs)
	require.Equal(t, []int64{10, 20}, *adminSvc.input.GroupIDs)
	require.True(t, adminSvc.input.SkipMixedChannelCheck)
}

func TestQuotaLeaseDemoHandlerSyncsReauthTaskWithExtraMerge(t *testing.T) {
	adminSvc := &quotaLeaseDemoSyncAdminService{}
	h := NewQuotaLeaseDemoHandler(nil, adminSvc)
	task := &service.QuotaLeaseDemoAccountLoginTask{
		ID:             "task-reauth",
		AccountID:      808,
		Name:           "existing-openai",
		Type:           service.AccountTypeOAuth,
		AssignedNodeID: "foreign-1",
		Concurrency:    4,
		Priority:       6,
		Status:         service.QuotaLeaseDemoAccountTaskCompleted,
		Metadata:       map[string]string{"source": "account_reauth_modal"},
		Account: &service.QuotaLeaseDemoAccountSnapshot{
			ID:          808,
			Name:        "existing-openai",
			Platform:    service.PlatformOpenAI,
			Type:        service.AccountTypeOAuth,
			Credentials: map[string]any{"access_token": "fresh-access", "model_mapping": map[string]any{"mode": "allow"}},
			Extra:       map[string]any{"account_uuid": "acct-new"},
			Concurrency: 4,
			Priority:    6,
			GroupIDs:    []int64{30},
		},
	}

	require.NoError(t, h.syncCompletedAccount(context.Background(), task))

	require.Equal(t, int64(808), adminSvc.updatedID)
	require.NotNil(t, adminSvc.input)
	require.Nil(t, adminSvc.input.Extra)
	require.Equal(t, "fresh-access", adminSvc.input.Credentials["access_token"])
	require.Equal(t, int64(808), adminSvc.updatedExtraID)
	require.Equal(t, map[string]any{
		"account_uuid":                "acct-new",
		"node_oauth_status":           service.QuotaLeaseDemoAccountTaskCompleted,
		"node_oauth_assigned_node_id": "foreign-1",
	}, adminSvc.updatedExtra)
	require.Equal(t, int64(808), adminSvc.clearedAccountID)
}

func TestQuotaLeaseDemoHandlerListsAssignedAccountsFromPersistedAdminAccounts(t *testing.T) {
	now := time.Now().UTC()
	proxyID := int64(88)
	adminSvc := &quotaLeaseDemoSyncAdminService{
		proxies: map[int64]*service.Proxy{
			proxyID: {
				ID:       proxyID,
				Name:     "foreign-egress",
				Protocol: "http",
				Host:     "127.0.0.1",
				Port:     18080,
				Username: "node-user",
				Password: "node-pass",
				Status:   service.StatusActive,
			},
		},
		listedAccounts: []service.Account{
			{
				ID:          901,
				Name:        "persisted-openai",
				Platform:    service.PlatformOpenAI,
				Type:        service.AccountTypeOAuth,
				Credentials: map[string]any{"access_token": "node-token"},
				Extra: map[string]any{
					"node_oauth_assigned_node_id": "foreign-1",
					"node_oauth_status":           service.QuotaLeaseDemoAccountTaskCompleted,
				},
				Status:      service.StatusActive,
				Schedulable: true,
				Concurrency: 2,
				Priority:    7,
				ProxyID:     &proxyID,
				Groups:      []*service.Group{{ID: 2, Name: "gpt"}},
				CreatedAt:   now.Add(-time.Hour),
				UpdatedAt:   now,
			},
			{
				ID:       902,
				Name:     "pending-openai",
				Platform: service.PlatformOpenAI,
				Type:     service.AccountTypeOAuth,
				Credentials: map[string]any{
					"node_oauth_pending": true,
				},
				Extra: map[string]any{
					"node_oauth_assigned_node_id": "foreign-1",
					"node_oauth_status":           service.QuotaLeaseDemoAccountTaskPending,
				},
				Status:      service.StatusActive,
				Schedulable: true,
			},
			{
				ID:          903,
				Name:        "other-node-openai",
				Platform:    service.PlatformOpenAI,
				Type:        service.AccountTypeOAuth,
				Credentials: map[string]any{"access_token": "other-token"},
				Extra: map[string]any{
					"node_oauth_assigned_node_id": "foreign-2",
					"node_oauth_status":           service.QuotaLeaseDemoAccountTaskCompleted,
				},
				Status:      service.StatusActive,
				Schedulable: true,
			},
		},
	}
	svc := service.NewQuotaLeaseDemoService(&config.Config{
		Gateway: config.GatewayConfig{
			QuotaLeaseDemo: config.GatewayQuotaLeaseDemoConfig{
				Enabled:            true,
				NodeID:             "control-node",
				NodeSecret:         "control-secret",
				DefaultGrantAmount: 1,
				LeaseTTLSeconds:    600,
			},
		},
	})
	h := NewQuotaLeaseDemoHandler(svc, adminSvc)

	accounts, err := h.listAssignedAccounts(context.Background(), "foreign-1")
	require.NoError(t, err)
	require.Len(t, accounts, 1)
	require.Equal(t, "foreign-1", accounts[0].NodeID)
	require.Equal(t, int64(901), accounts[0].Account.ID)
	require.Equal(t, "node-token", accounts[0].Account.Credentials["access_token"])
	require.Equal(t, []int64{2}, accounts[0].Account.GroupIDs)
	require.NotNil(t, accounts[0].Account.ProxyID)
	require.Equal(t, proxyID, *accounts[0].Account.ProxyID)
	require.NotNil(t, accounts[0].Account.Proxy)
	require.Equal(t, "foreign-egress", accounts[0].Account.Proxy.Name)
	proxySnapshot := &service.Proxy{
		Protocol: accounts[0].Account.Proxy.Protocol,
		Host:     accounts[0].Account.Proxy.Host,
		Port:     accounts[0].Account.Proxy.Port,
		Username: accounts[0].Account.Proxy.Username,
		Password: accounts[0].Account.Proxy.Password,
	}
	require.Equal(t, "http://node-user:node-pass@127.0.0.1:18080", proxySnapshot.URL())
	require.Equal(t, service.QuotaLeaseDemoAccountTaskCompleted, accounts[0].Account.Extra["node_oauth_status"])
	lastSyncedAt, ok := accounts[0].Account.Extra["node_oauth_last_synced_at"].(string)
	require.True(t, ok)
	_, err = time.Parse(time.RFC3339Nano, lastSyncedAt)
	require.NoError(t, err)
	require.Equal(t, int64(901), adminSvc.updatedExtraID)
	require.Equal(t, lastSyncedAt, adminSvc.updatedExtra["node_oauth_last_synced_at"])
	require.Equal(t, []string{"foreign-1"}, adminSvc.listedNodeIDs)
}

func TestQuotaLeaseDemoHandlerListsAssignedAPIKeyAccounts(t *testing.T) {
	now := time.Now().UTC()
	adminSvc := &quotaLeaseDemoSyncAdminService{
		listedAccounts: []service.Account{
			{
				ID:          904,
				Name:        "persisted-openai-key",
				Platform:    service.PlatformOpenAI,
				Type:        service.AccountTypeAPIKey,
				Credentials: map[string]any{"api_key": "sk-node"},
				Extra: map[string]any{
					"node_oauth_assigned_node_id": "foreign-1",
				},
				Status:      service.StatusActive,
				Schedulable: true,
				Concurrency: 1,
				GroupIDs:    []int64{2},
				CreatedAt:   now.Add(-time.Hour),
				UpdatedAt:   now,
			},
		},
	}
	svc := service.NewQuotaLeaseDemoService(&config.Config{
		Gateway: config.GatewayConfig{
			QuotaLeaseDemo: config.GatewayQuotaLeaseDemoConfig{
				Enabled: true,
			},
		},
	})
	h := NewQuotaLeaseDemoHandler(svc, adminSvc)

	accounts, err := h.listAssignedAccounts(context.Background(), "foreign-1")
	require.NoError(t, err)
	require.Len(t, accounts, 1)
	require.Equal(t, int64(904), accounts[0].Account.ID)
	require.Equal(t, service.AccountTypeAPIKey, accounts[0].Account.Type)
	require.Equal(t, "sk-node", accounts[0].Account.Credentials["api_key"])
	require.Equal(t, "foreign-1", accounts[0].NodeID)
}

func TestQuotaLeaseDemoHandlerListsAssignedAPIKeyAccountsForMultipleNodes(t *testing.T) {
	now := time.Now().UTC()
	adminSvc := &quotaLeaseDemoSyncAdminService{
		listedAccounts: []service.Account{
			{
				ID:          905,
				Name:        "shared-openai-key",
				Platform:    service.PlatformOpenAI,
				Type:        service.AccountTypeAPIKey,
				Credentials: map[string]any{"api_key": "sk-shared"},
				Extra: map[string]any{
					service.QuotaLeaseDemoAssignedNodeIDsExtraKey: []any{"foreign-1", "foreign-2"},
				},
				Status:      service.StatusActive,
				Schedulable: true,
				Concurrency: 1,
				GroupIDs:    []int64{2},
				CreatedAt:   now.Add(-time.Hour),
				UpdatedAt:   now,
			},
		},
	}
	svc := service.NewQuotaLeaseDemoService(&config.Config{
		Gateway: config.GatewayConfig{
			QuotaLeaseDemo: config.GatewayQuotaLeaseDemoConfig{
				Enabled: true,
			},
		},
	})
	h := NewQuotaLeaseDemoHandler(svc, adminSvc)

	nodeOneAccounts, err := h.listAssignedAccounts(context.Background(), "foreign-1")
	require.NoError(t, err)
	require.Len(t, nodeOneAccounts, 1)
	require.Equal(t, int64(905), nodeOneAccounts[0].Account.ID)
	require.Equal(t, "foreign-1", nodeOneAccounts[0].NodeID)

	nodeTwoAccounts, err := h.listAssignedAccounts(context.Background(), "foreign-2")
	require.NoError(t, err)
	require.Len(t, nodeTwoAccounts, 1)
	require.Equal(t, int64(905), nodeTwoAccounts[0].Account.ID)
	require.Equal(t, "foreign-2", nodeTwoAccounts[0].NodeID)

	otherNodeAccounts, err := h.listAssignedAccounts(context.Background(), "foreign-3")
	require.NoError(t, err)
	require.Empty(t, otherNodeAccounts)
}

func TestQuotaLeaseDemoHandlerMirrorSnapshotMovesPersistedAccountBetweenNodes(t *testing.T) {
	ctx := context.Background()
	now := time.Now().UTC()
	adminSvc := &quotaLeaseDemoSyncAdminService{
		listedAccounts: []service.Account{
			{
				ID:          1001,
				Name:        "migrating-openai",
				Platform:    service.PlatformOpenAI,
				Type:        service.AccountTypeOAuth,
				Credentials: map[string]any{"access_token": "node-token-a"},
				Extra: map[string]any{
					"node_oauth_assigned_node_id": "node-a",
					"node_oauth_status":           service.QuotaLeaseDemoAccountTaskCompleted,
				},
				Status:      service.StatusActive,
				Schedulable: true,
				Concurrency: 1,
				CreatedAt:   now.Add(-time.Hour),
				UpdatedAt:   now,
			},
		},
	}
	svc := service.NewQuotaLeaseDemoService(&config.Config{
		Gateway: config.GatewayConfig{
			QuotaLeaseDemo: config.GatewayQuotaLeaseDemoConfig{
				Enabled: true,
			},
		},
	})
	h := NewQuotaLeaseDemoHandler(svc, adminSvc)

	firstNodeASnapshot, err := h.buildMirrorSnapshot(ctx, "node-a")
	require.NoError(t, err)
	firstNodeASnapshot = svc.PrepareMirrorSnapshot(firstNodeASnapshot, 0)
	require.Len(t, firstNodeASnapshot.Accounts, 1)
	require.Equal(t, int64(1001), firstNodeASnapshot.Accounts[0].ID)

	adminSvc.listedAccounts[0].Extra["node_oauth_assigned_node_id"] = "node-b"
	adminSvc.listedAccounts[0].UpdatedAt = now.Add(time.Minute)

	secondNodeASnapshot, err := h.buildMirrorSnapshot(ctx, "node-a")
	require.NoError(t, err)
	secondNodeASnapshot = svc.PrepareMirrorSnapshot(secondNodeASnapshot, firstNodeASnapshot.Version)
	require.True(t, secondNodeASnapshot.Delta)
	require.Equal(t, []int64{1001}, secondNodeASnapshot.DeletedAccountIDs)
	require.Empty(t, secondNodeASnapshot.Accounts)
	require.Equal(t, 0, secondNodeASnapshot.TotalAccountCount)

	nodeBSnapshot, err := h.buildMirrorSnapshot(ctx, "node-b")
	require.NoError(t, err)
	nodeBSnapshot = svc.PrepareMirrorSnapshot(nodeBSnapshot, 0)
	require.Len(t, nodeBSnapshot.Accounts, 1)
	require.Equal(t, int64(1001), nodeBSnapshot.Accounts[0].ID)
	require.Equal(t, "node-b", nodeBSnapshot.Accounts[0].Extra["node_oauth_assigned_node_id"])
}

func TestQuotaLeaseDemoHandlerRegistersNodeAndUsesNodeSecret(t *testing.T) {
	router, _ := newQuotaLeaseDemoHandlerTestRouter(t)

	unauthorized := httptest.NewRecorder()
	router.ServeHTTP(unauthorized, quotaLeaseDemoJSONRequest(t, http.MethodPost, "/api/v1/node-leases/demo/nodes/register", map[string]any{
		"node_id": "foreign-1",
	}))
	require.Equal(t, http.StatusUnauthorized, unauthorized.Code)

	registerReq := quotaLeaseDemoJSONRequest(t, http.MethodPost, "/api/v1/node-leases/demo/nodes/register", map[string]any{
		"node_id":  "foreign-1",
		"region":   "sg",
		"base_url": "https://foreign-1.example",
	})
	registerReq.Header.Set("X-Node-Secret", "control-secret")
	registerRec := httptest.NewRecorder()
	router.ServeHTTP(registerRec, registerReq)
	require.Equal(t, http.StatusOK, registerRec.Code)

	var registerBody struct {
		Node       service.QuotaLeaseDemoNode `json:"node"`
		NodeSecret string                     `json:"node_secret"`
	}
	require.NoError(t, json.Unmarshal(registerRec.Body.Bytes(), &registerBody))
	require.Equal(t, "foreign-1", registerBody.Node.NodeID)
	require.NotEmpty(t, registerBody.NodeSecret)

	leaseReq := quotaLeaseDemoJSONRequest(t, http.MethodPost, "/api/v1/node-leases/demo/leases/request", map[string]any{
		"user_id":    10,
		"api_key_id": 20,
		"amount":     1,
	})
	leaseReq.Header.Set("X-Node-ID", "foreign-1")
	leaseReq.Header.Set("X-Node-Secret", registerBody.NodeSecret)
	leaseRec := httptest.NewRecorder()
	router.ServeHTTP(leaseRec, leaseReq)
	require.Equal(t, http.StatusOK, leaseRec.Code)

	var leaseBody struct {
		Lease service.QuotaLeaseDemoLease `json:"lease"`
	}
	require.NoError(t, json.Unmarshal(leaseRec.Body.Bytes(), &leaseBody))
	require.Equal(t, "foreign-1", leaseBody.Lease.NodeID)

	heartbeatReq := quotaLeaseDemoJSONRequest(t, http.MethodPost, "/api/v1/node-leases/demo/nodes/heartbeat", map[string]any{
		"node_id":           "foreign-1",
		"inflight_requests": 2,
		"lease_remaining":   0.5,
		"metrics":           map[string]float64{"rps": 3},
	})
	heartbeatReq.Header.Set("X-Node-ID", "foreign-1")
	heartbeatReq.Header.Set("X-Node-Secret", registerBody.NodeSecret)
	heartbeatRec := httptest.NewRecorder()
	router.ServeHTTP(heartbeatRec, heartbeatReq)
	require.Equal(t, http.StatusOK, heartbeatRec.Code)
}

func TestQuotaLeaseDemoHandlerUpdatesNode(t *testing.T) {
	router, _ := newQuotaLeaseDemoHandlerTestRouter(t)

	registerReq := quotaLeaseDemoJSONRequest(t, http.MethodPost, "/api/v1/node-leases/demo/nodes/register", map[string]any{
		"node_id":  "foreign-edit-1",
		"region":   "sg",
		"base_url": "https://old-node.example",
	})
	registerReq.Header.Set("X-Node-Secret", "control-secret")
	registerRec := httptest.NewRecorder()
	router.ServeHTTP(registerRec, registerReq)
	require.Equal(t, http.StatusOK, registerRec.Code)

	unauthorizedReq := quotaLeaseDemoJSONRequest(t, http.MethodPut, "/api/v1/node-leases/demo/nodes/foreign-edit-1", map[string]any{
		"region": "us-west",
	})
	unauthorizedRec := httptest.NewRecorder()
	router.ServeHTTP(unauthorizedRec, unauthorizedReq)
	require.Equal(t, http.StatusUnauthorized, unauthorizedRec.Code)

	updateReq := quotaLeaseDemoJSONRequest(t, http.MethodPut, "/api/v1/node-leases/demo/nodes/foreign-edit-1", map[string]any{
		"region":     "us-west",
		"base_url":   "https://new-node.example",
		"public_key": "node-public-key",
		"metadata":   map[string]string{"pool": "west"},
		"status":     service.QuotaLeaseDemoNodeStatusDisabled,
	})
	updateReq.Header.Set("X-Node-Secret", "control-secret")
	updateRec := httptest.NewRecorder()
	router.ServeHTTP(updateRec, updateReq)
	require.Equal(t, http.StatusOK, updateRec.Code)

	var updateBody struct {
		Node service.QuotaLeaseDemoNode `json:"node"`
	}
	require.NoError(t, json.Unmarshal(updateRec.Body.Bytes(), &updateBody))
	require.Equal(t, "foreign-edit-1", updateBody.Node.NodeID)
	require.Equal(t, "us-west", updateBody.Node.Region)
	require.Equal(t, "https://new-node.example", updateBody.Node.BaseURL)
	require.Equal(t, "node-public-key", updateBody.Node.PublicKey)
	require.Equal(t, "west", updateBody.Node.Metadata["pool"])
	require.Equal(t, service.QuotaLeaseDemoNodeStatusDisabled, updateBody.Node.Status)
}

func TestQuotaLeaseDemoHandlerInjectsControlSecretForAdminRoute(t *testing.T) {
	router, svc := newQuotaLeaseDemoHandlerTestRouter(t)
	h := NewQuotaLeaseDemoHandler(svc)

	adminGroup := router.Group("/api/v1/admin/node-leases/demo")
	adminGroup.Use(h.InjectControlSecret)
	adminGroup.GET("/status", h.Status)

	publicRec := httptest.NewRecorder()
	router.ServeHTTP(publicRec, httptest.NewRequest(http.MethodGet, "/api/v1/node-leases/demo/status", nil))
	require.Equal(t, http.StatusUnauthorized, publicRec.Code)

	adminRec := httptest.NewRecorder()
	router.ServeHTTP(adminRec, httptest.NewRequest(http.MethodGet, "/api/v1/admin/node-leases/demo/status", nil))
	require.Equal(t, http.StatusOK, adminRec.Code)
}

func TestQuotaLeaseDemoHandlerCreatesRegistrationURLAndStoresNodeSecret(t *testing.T) {
	router, _ := newQuotaLeaseDemoHandlerTestRouter(t)

	unauthorized := httptest.NewRecorder()
	router.ServeHTTP(unauthorized, quotaLeaseDemoJSONRequest(t, http.MethodPost, "/api/v1/node-leases/demo/nodes/registration-urls", map[string]any{
		"node_id": "foreign-url-1",
	}))
	require.Equal(t, http.StatusUnauthorized, unauthorized.Code)

	createReq := quotaLeaseDemoJSONRequest(t, http.MethodPost, "/api/v1/node-leases/demo/nodes/registration-urls", map[string]any{
		"node_id":     "foreign-url-1",
		"region":      "us-west",
		"base_url":    "https://foreign-url-1.example",
		"metadata":    map[string]string{"pool": "us"},
		"ttl_seconds": 120,
	})
	createReq.Header.Set("X-Node-Secret", "control-secret")
	createReq.Header.Set("X-Forwarded-Proto", "https")
	createReq.Header.Set("X-Forwarded-Host", "control.example.test")
	createRec := httptest.NewRecorder()
	router.ServeHTTP(createRec, createReq)
	require.Equal(t, http.StatusOK, createRec.Code)

	var createBody struct {
		RegistrationURL string `json:"registration_url"`
		NodeID          string `json:"node_id"`
		ExpiresAt       string `json:"expires_at"`
	}
	require.NoError(t, json.Unmarshal(createRec.Body.Bytes(), &createBody))
	require.Equal(t, "foreign-url-1", createBody.NodeID)
	require.NotEmpty(t, createBody.ExpiresAt)
	require.Contains(t, createBody.RegistrationURL, "https://control.example.test/api/v1/node-leases/demo/nodes/register")
	require.Contains(t, createBody.RegistrationURL, "registration_token=")

	registerReq := quotaLeaseDemoJSONRequest(t, http.MethodPost, createBody.RegistrationURL, map[string]any{
		"node_secret": "node-generated-secret",
	})
	registerRec := httptest.NewRecorder()
	router.ServeHTTP(registerRec, registerReq)
	require.Equal(t, http.StatusOK, registerRec.Code)

	var registerBody struct {
		Node       service.QuotaLeaseDemoNode `json:"node"`
		NodeSecret string                     `json:"node_secret"`
	}
	require.NoError(t, json.Unmarshal(registerRec.Body.Bytes(), &registerBody))
	require.Equal(t, "foreign-url-1", registerBody.Node.NodeID)
	require.Equal(t, "node-generated-secret", registerBody.NodeSecret)

	leaseReq := quotaLeaseDemoJSONRequest(t, http.MethodPost, "/api/v1/node-leases/demo/leases/request", map[string]any{
		"user_id":    10,
		"api_key_id": 20,
		"amount":     1,
	})
	leaseReq.Header.Set("X-Node-ID", "foreign-url-1")
	leaseReq.Header.Set("X-Node-Secret", "node-generated-secret")
	leaseRec := httptest.NewRecorder()
	router.ServeHTTP(leaseRec, leaseReq)
	require.Equal(t, http.StatusOK, leaseRec.Code)
}

func TestQuotaLeaseDemoHandlerDiagnosticsIncludesUserProfile(t *testing.T) {
	gin.SetMode(gin.TestMode)
	svc := service.NewQuotaLeaseDemoService(&config.Config{
		Gateway: config.GatewayConfig{
			QuotaLeaseDemo: config.GatewayQuotaLeaseDemoConfig{
				Enabled:                true,
				NodeID:                 "control-node",
				NodeSecret:             "control-secret",
				DefaultGrantAmount:     1,
				ReclaimGraceSeconds:    3600,
				PreflightReserveAmount: 0.000001,
			},
		},
	})
	adminSvc := &quotaLeaseDemoSyncAdminService{
		users: map[int64]*service.User{
			7: {
				ID:       7,
				Username: "alice",
				Email:    "alice@example.test",
				Status:   service.StatusActive,
				Balance:  0,
			},
		},
	}
	_, err := svc.RegisterNode(context.Background(), service.QuotaLeaseDemoNodeRegistrationRequest{
		NodeID:     "foreign-1",
		NodeSecret: "node-secret",
	})
	require.NoError(t, err)
	lease, err := svc.RequestLease(context.Background(), service.QuotaLeaseDemoLeaseRequest{
		NodeID:   "foreign-1",
		UserID:   7,
		APIKeyID: 8,
		Amount:   0.5,
	})
	require.NoError(t, err)
	_, err = svc.ConsumeUsage(context.Background(), service.QuotaLeaseDemoUsageEvent{
		EventID:   "evt-diagnostics",
		LeaseID:   lease.ID,
		NodeID:    "foreign-1",
		UserID:    7,
		APIKeyID:  8,
		RequestID: "req-diagnostics",
		Amount:    0.75,
	})
	require.NoError(t, err)

	h := NewQuotaLeaseDemoHandler(svc, adminSvc)
	router := gin.New()
	router.GET("/api/v1/node-leases/demo/diagnostics", h.Diagnostics)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/node-leases/demo/diagnostics", nil)
	req.Header.Set("X-Node-Secret", "control-secret")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	var body struct {
		Diagnostics service.QuotaLeaseDemoDiagnostics `json:"diagnostics"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	require.Equal(t, service.QuotaLeaseDemoDiagnosticHealthCritical, body.Diagnostics.Health)
	require.Len(t, body.Diagnostics.Users, 1)
	require.Equal(t, "alice", body.Diagnostics.Users[0].Username)
	require.NotNil(t, body.Diagnostics.Users[0].Balance)
	require.Contains(t, quotaLeaseDemoHandlerDiagnosticIssueCodes(body.Diagnostics.Issues), "lease_overdraft")
	require.Contains(t, quotaLeaseDemoHandlerDiagnosticIssueCodes(body.Diagnostics.Issues), "user_has_overdraft_lease")
}

func quotaLeaseDemoHandlerDiagnosticIssueCodes(issues []service.QuotaLeaseDemoDiagnosticIssue) []string {
	codes := make([]string, 0, len(issues))
	for _, issue := range issues {
		codes = append(codes, issue.Code)
	}
	return codes
}

func TestQuotaLeaseDemoHandlerAuthorizeClientKeyUsesDefaultGrantAmountByDefault(t *testing.T) {
	router, svc := newQuotaLeaseDemoHandlerTestRouter(t)
	apiKeySvc := service.NewAPIKeyService(
		&quotaLeaseDemoAPIKeyRepoStub{
			apiKey: &service.APIKey{
				ID:     20,
				UserID: 10,
				Key:    "sk-live-user",
				Status: service.StatusAPIKeyActive,
				User: &service.User{
					ID:      10,
					Status:  service.StatusActive,
					Balance: 0.6,
				},
			},
		},
		nil,
		nil,
		nil,
		nil,
		nil,
		&config.Config{},
	)
	h := NewQuotaLeaseDemoHandler(svc)
	h.SetAPIKeyService(apiKeySvc)
	router.POST("/api/v1/node-leases/demo/auth/client-key", h.AuthorizeClientKey)

	req := quotaLeaseDemoJSONRequest(t, http.MethodPost, "/api/v1/node-leases/demo/auth/client-key", map[string]any{
		"api_key": "sk-live-user",
	})
	req.Header.Set("X-Node-Secret", "control-secret")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	require.Equal(t, http.StatusOK, rec.Code)

	var body struct {
		Lease service.QuotaLeaseDemoLease `json:"lease"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	require.InDelta(t, 0.6, body.Lease.Granted, 1e-12)
}

func TestQuotaLeaseDemoHandlerAuthorizeClientKeyCapsExplicitAmountToUserBalance(t *testing.T) {
	router, svc := newQuotaLeaseDemoHandlerTestRouter(t)
	apiKeySvc := service.NewAPIKeyService(
		&quotaLeaseDemoAPIKeyRepoStub{
			apiKey: &service.APIKey{
				ID:     20,
				UserID: 10,
				Key:    "sk-live-user",
				Status: service.StatusAPIKeyActive,
				User: &service.User{
					ID:      10,
					Status:  service.StatusActive,
					Balance: 0.6,
				},
			},
		},
		nil,
		nil,
		nil,
		nil,
		nil,
		&config.Config{},
	)
	h := NewQuotaLeaseDemoHandler(svc)
	h.SetAPIKeyService(apiKeySvc)
	router.POST("/api/v1/node-leases/demo/auth/client-key/explicit", h.AuthorizeClientKey)

	req := quotaLeaseDemoJSONRequest(t, http.MethodPost, "/api/v1/node-leases/demo/auth/client-key/explicit", map[string]any{
		"api_key": "sk-live-user",
		"amount":  1,
	})
	req.Header.Set("X-Node-Secret", "control-secret")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	require.Equal(t, http.StatusOK, rec.Code)

	var body struct {
		Lease service.QuotaLeaseDemoLease `json:"lease"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	require.InDelta(t, 0.6, body.Lease.Granted, 1e-12)
}

func TestQuotaLeaseDemoHandlerAuthorizeClientKeyUsesFreshBalanceForLease(t *testing.T) {
	router, svc := newQuotaLeaseDemoHandlerTestRouter(t)
	cached := &service.APIKey{
		ID:     20,
		UserID: 10,
		Key:    "sk-live-user",
		Status: service.StatusAPIKeyActive,
		User: &service.User{
			ID:      10,
			Status:  service.StatusActive,
			Balance: 0.6,
		},
	}
	fresh := &service.APIKey{
		ID:     20,
		UserID: 10,
		Key:    "sk-live-user",
		Status: service.StatusAPIKeyActive,
		User: &service.User{
			ID:      10,
			Status:  service.StatusActive,
			Balance: -0.001,
		},
	}
	apiKeySvc := service.NewAPIKeyService(
		&quotaLeaseDemoAPIKeyRepoStub{
			apiKey:     cached,
			apiKeyByID: fresh,
		},
		nil,
		nil,
		nil,
		nil,
		nil,
		&config.Config{},
	)
	h := NewQuotaLeaseDemoHandler(svc)
	h.SetAPIKeyService(apiKeySvc)
	router.POST("/api/v1/node-leases/demo/auth/client-key/fresh", h.AuthorizeClientKey)

	req := quotaLeaseDemoJSONRequest(t, http.MethodPost, "/api/v1/node-leases/demo/auth/client-key/fresh", map[string]any{
		"api_key": "sk-live-user",
	})
	req.Header.Set("X-Node-Secret", "control-secret")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	require.Equal(t, http.StatusForbidden, rec.Code)
	require.Contains(t, rec.Body.String(), "no_capacity")
	require.Empty(t, svc.Snapshot().Leases)
}

func TestQuotaLeaseDemoHandlerAuthorizeClientKeyRejectsZeroBalanceWithExistingLease(t *testing.T) {
	router, svc := newQuotaLeaseDemoHandlerTestRouter(t)
	existing, err := svc.RequestLease(context.Background(), service.QuotaLeaseDemoLeaseRequest{
		NodeID:   "foreign-1",
		UserID:   10,
		APIKeyID: 20,
		Amount:   0.5,
	})
	require.NoError(t, err)

	billing := &quotaLeaseDemoBillingRepoStub{reserveErr: service.ErrBalanceHoldInsufficientBalance}
	svc.SetUsageBillingRepository(billing)
	apiKeySvc := service.NewAPIKeyService(
		&quotaLeaseDemoAPIKeyRepoStub{
			apiKey: &service.APIKey{
				ID:     20,
				UserID: 10,
				Key:    "sk-live-user",
				Status: service.StatusAPIKeyActive,
				User: &service.User{
					ID:      10,
					Status:  service.StatusActive,
					Balance: 0,
				},
			},
		},
		nil,
		nil,
		nil,
		nil,
		nil,
		&config.Config{},
	)
	h := NewQuotaLeaseDemoHandler(svc)
	h.SetAPIKeyService(apiKeySvc)
	router.POST("/api/v1/node-leases/demo/auth/client-key/reuse", h.AuthorizeClientKey)

	req := quotaLeaseDemoJSONRequest(t, http.MethodPost, "/api/v1/node-leases/demo/auth/client-key/reuse", map[string]any{
		"api_key": "sk-live-user",
		"node_id": "foreign-1",
	})
	req.Header.Set("X-Node-Secret", "control-secret")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	require.Equal(t, http.StatusForbidden, rec.Code)
	require.Contains(t, rec.Body.String(), "no_capacity")
	snapshot := svc.Snapshot()
	require.Len(t, snapshot.Leases, 1)
	require.Equal(t, existing.ID, snapshot.Leases[0].ID)
	require.InDelta(t, 0, float64(billing.reserveCalls), 1e-12)
}

func TestQuotaLeaseDemoHandlerAuthorizeClientKeyRejectsZeroBalance(t *testing.T) {
	router, svc := newQuotaLeaseDemoHandlerTestRouter(t)
	svc.SetUsageBillingRepository(&quotaLeaseDemoBillingRepoStub{reserveErr: service.ErrBalanceHoldInsufficientBalance})
	apiKeySvc := service.NewAPIKeyService(
		&quotaLeaseDemoAPIKeyRepoStub{
			apiKey: &service.APIKey{
				ID:     20,
				UserID: 10,
				Key:    "sk-live-user",
				Status: service.StatusAPIKeyActive,
				User: &service.User{
					ID:      10,
					Status:  service.StatusActive,
					Balance: 0,
				},
			},
		},
		nil,
		nil,
		nil,
		nil,
		nil,
		&config.Config{},
	)
	h := NewQuotaLeaseDemoHandler(svc)
	h.svc.SetConfig(&config.Config{
		Gateway: config.GatewayConfig{
			QuotaLeaseDemo: config.GatewayQuotaLeaseDemoConfig{
				Enabled:                true,
				NodeID:                 "control-node",
				NodeSecret:             "control-secret",
				DefaultGrantAmount:     1,
				LeaseTTLSeconds:        600,
				ReclaimGraceSeconds:    3600,
				PreflightReserveAmount: 0.000001,
			},
		},
	})
	h.SetAPIKeyService(apiKeySvc)
	router.POST("/api/v1/node-leases/demo/auth/client-key/zero", h.AuthorizeClientKey)

	req := quotaLeaseDemoJSONRequest(t, http.MethodPost, "/api/v1/node-leases/demo/auth/client-key/zero", map[string]any{
		"api_key": "sk-live-user",
	})
	req.Header.Set("X-Node-Secret", "control-secret")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	require.Equal(t, http.StatusForbidden, rec.Code)
	require.Contains(t, rec.Body.String(), "no_capacity")
}

func TestQuotaLeaseDemoHandlerRejectsNodeMismatch(t *testing.T) {
	router, _ := newQuotaLeaseDemoHandlerTestRouter(t)

	registerReq := quotaLeaseDemoJSONRequest(t, http.MethodPost, "/api/v1/node-leases/demo/nodes/register", map[string]any{
		"node_id": "foreign-1",
	})
	registerReq.Header.Set("X-Node-Secret", "control-secret")
	registerRec := httptest.NewRecorder()
	router.ServeHTTP(registerRec, registerReq)
	require.Equal(t, http.StatusOK, registerRec.Code)

	var registerBody struct {
		NodeSecret string `json:"node_secret"`
	}
	require.NoError(t, json.Unmarshal(registerRec.Body.Bytes(), &registerBody))

	leaseReq := quotaLeaseDemoJSONRequest(t, http.MethodPost, "/api/v1/node-leases/demo/leases/request", map[string]any{
		"node_id":    "foreign-2",
		"user_id":    10,
		"api_key_id": 20,
		"amount":     1,
	})
	leaseReq.Header.Set("X-Node-ID", "foreign-1")
	leaseReq.Header.Set("X-Node-Secret", registerBody.NodeSecret)
	leaseRec := httptest.NewRecorder()
	router.ServeHTTP(leaseRec, leaseReq)
	require.Equal(t, http.StatusForbidden, leaseRec.Code)
}

func TestQuotaLeaseDemoHandlerAccountLoginTaskFlow(t *testing.T) {
	router, _ := newQuotaLeaseDemoHandlerTestRouter(t)

	registerReq := quotaLeaseDemoJSONRequest(t, http.MethodPost, "/api/v1/node-leases/demo/nodes/register", map[string]any{
		"node_id": "foreign-1",
	})
	registerReq.Header.Set("X-Node-Secret", "control-secret")
	registerRec := httptest.NewRecorder()
	router.ServeHTTP(registerRec, registerReq)
	require.Equal(t, http.StatusOK, registerRec.Code)

	var registerBody struct {
		NodeSecret string `json:"node_secret"`
	}
	require.NoError(t, json.Unmarshal(registerRec.Body.Bytes(), &registerBody))

	createReq := quotaLeaseDemoJSONRequest(t, http.MethodPost, "/api/v1/node-leases/demo/accounts/login-tasks", map[string]any{
		"account_id":       101,
		"name":             "gpt-oauth-1",
		"platform":         service.PlatformOpenAI,
		"type":             service.AccountTypeOAuth,
		"assigned_node_id": "foreign-1",
		"login_payload": map[string]any{
			"auth_url": "https://auth.example/start",
		},
	})
	createReq.Header.Set("X-Node-Secret", "control-secret")
	createRec := httptest.NewRecorder()
	router.ServeHTTP(createRec, createReq)
	require.Equal(t, http.StatusOK, createRec.Code)

	var createBody struct {
		Task service.QuotaLeaseDemoAccountLoginTask `json:"task"`
	}
	require.NoError(t, json.Unmarshal(createRec.Body.Bytes(), &createBody))
	require.Equal(t, "foreign-1", createBody.Task.AssignedNodeID)

	listReq := httptest.NewRequest(http.MethodGet, "/api/v1/node-leases/demo/accounts/login-tasks?status=pending", nil)
	listReq.Header.Set("X-Node-ID", "foreign-1")
	listReq.Header.Set("X-Node-Secret", registerBody.NodeSecret)
	listRec := httptest.NewRecorder()
	router.ServeHTTP(listRec, listReq)
	require.Equal(t, http.StatusOK, listRec.Code)

	var listBody struct {
		Tasks []service.QuotaLeaseDemoAccountLoginTask `json:"tasks"`
	}
	require.NoError(t, json.Unmarshal(listRec.Body.Bytes(), &listBody))
	require.Len(t, listBody.Tasks, 1)
	require.Equal(t, createBody.Task.ID, listBody.Tasks[0].ID)

	completeReq := quotaLeaseDemoJSONRequest(t, http.MethodPost, "/api/v1/node-leases/demo/accounts/login-tasks/"+createBody.Task.ID+"/complete", map[string]any{
		"account": map[string]any{
			"credentials": map[string]any{
				"access_token": "node-access-token",
			},
		},
	})
	completeReq.Header.Set("X-Node-ID", "foreign-1")
	completeReq.Header.Set("X-Node-Secret", registerBody.NodeSecret)
	completeRec := httptest.NewRecorder()
	router.ServeHTTP(completeRec, completeReq)
	require.Equal(t, http.StatusOK, completeRec.Code)

	assignmentsReq := httptest.NewRequest(http.MethodGet, "/api/v1/node-leases/demo/accounts/assignments", nil)
	assignmentsReq.Header.Set("X-Node-ID", "foreign-1")
	assignmentsReq.Header.Set("X-Node-Secret", registerBody.NodeSecret)
	assignmentsRec := httptest.NewRecorder()
	router.ServeHTTP(assignmentsRec, assignmentsReq)
	require.Equal(t, http.StatusOK, assignmentsRec.Code)

	var assignmentsBody struct {
		Accounts []service.QuotaLeaseDemoAssignedAccount `json:"accounts"`
	}
	require.NoError(t, json.Unmarshal(assignmentsRec.Body.Bytes(), &assignmentsBody))
	require.Len(t, assignmentsBody.Accounts, 1)
	require.Equal(t, int64(101), assignmentsBody.Accounts[0].Account.ID)
	require.Equal(t, "node-access-token", assignmentsBody.Accounts[0].Account.Credentials["access_token"])

	statusReq := quotaLeaseDemoJSONRequest(t, http.MethodPost, "/api/v1/node-leases/demo/accounts/status", map[string]any{
		"account_id":    101,
		"status":        service.StatusActive,
		"schedulable":   false,
		"error_message": "oauth cooling down",
		"credentials_patch": map[string]any{
			"access_token": "node-access-token-2",
		},
	})
	statusReq.Header.Set("X-Node-ID", "foreign-1")
	statusReq.Header.Set("X-Node-Secret", registerBody.NodeSecret)
	statusRec := httptest.NewRecorder()
	router.ServeHTTP(statusRec, statusReq)
	require.Equal(t, http.StatusOK, statusRec.Code)

	var statusBody struct {
		Account service.QuotaLeaseDemoAssignedAccount `json:"account"`
	}
	require.NoError(t, json.Unmarshal(statusRec.Body.Bytes(), &statusBody))
	require.Equal(t, int64(101), statusBody.Account.Account.ID)
	require.False(t, statusBody.Account.Account.Schedulable)
	require.Equal(t, "oauth cooling down", statusBody.Account.Account.ErrorMessage)
	require.Equal(t, "node-access-token-2", statusBody.Account.Account.Credentials["access_token"])
}

func TestQuotaLeaseDemoHandlerUsageProbeTaskSyncsExtraToAdminService(t *testing.T) {
	gin.SetMode(gin.TestMode)
	svc := service.NewQuotaLeaseDemoService(&config.Config{
		Gateway: config.GatewayConfig{
			QuotaLeaseDemo: config.GatewayQuotaLeaseDemoConfig{
				Enabled:    true,
				NodeSecret: "control-secret",
			},
		},
	})
	adminSvc := &quotaLeaseDemoSyncAdminService{}
	h := NewQuotaLeaseDemoHandler(svc, adminSvc)
	router := gin.New()
	group := router.Group("/api/v1/node-leases/demo")
	group.POST("/nodes/register", h.RegisterNode)
	group.POST("/accounts/usage-probe-tasks", h.CreateUsageProbeTask)
	group.GET("/accounts/usage-probe-tasks", h.ListUsageProbeTasks)
	group.POST("/accounts/usage-probe-tasks/:task_id/complete", h.CompleteUsageProbeTask)

	registerReq := quotaLeaseDemoJSONRequest(t, http.MethodPost, "/api/v1/node-leases/demo/nodes/register", map[string]any{
		"node_id": "foreign-usage-1",
	})
	registerReq.Header.Set("X-Node-Secret", "control-secret")
	registerRec := httptest.NewRecorder()
	router.ServeHTTP(registerRec, registerReq)
	require.Equal(t, http.StatusOK, registerRec.Code)

	var registerBody struct {
		NodeSecret string `json:"node_secret"`
	}
	require.NoError(t, json.Unmarshal(registerRec.Body.Bytes(), &registerBody))

	createReq := quotaLeaseDemoJSONRequest(t, http.MethodPost, "/api/v1/node-leases/demo/accounts/usage-probe-tasks", map[string]any{
		"account_id":       202,
		"assigned_node_id": "foreign-usage-1",
		"platform":         service.PlatformOpenAI,
		"source":           "active",
		"force":            true,
		"probe_kind":       service.QuotaLeaseDemoUsageProbeKindAccountUsage,
	})
	createReq.Header.Set("X-Node-Secret", "control-secret")
	createRec := httptest.NewRecorder()
	router.ServeHTTP(createRec, createReq)
	require.Equal(t, http.StatusOK, createRec.Code)

	var createBody struct {
		Task service.QuotaLeaseDemoUsageProbeTask `json:"task"`
	}
	require.NoError(t, json.Unmarshal(createRec.Body.Bytes(), &createBody))

	listReq := httptest.NewRequest(http.MethodGet, "/api/v1/node-leases/demo/accounts/usage-probe-tasks?status=pending", nil)
	listReq.Header.Set("X-Node-ID", "foreign-usage-1")
	listReq.Header.Set("X-Node-Secret", registerBody.NodeSecret)
	listRec := httptest.NewRecorder()
	router.ServeHTTP(listRec, listReq)
	require.Equal(t, http.StatusOK, listRec.Code)

	var listBody struct {
		Tasks []service.QuotaLeaseDemoUsageProbeTask `json:"tasks"`
	}
	require.NoError(t, json.Unmarshal(listRec.Body.Bytes(), &listBody))
	require.Len(t, listBody.Tasks, 1)

	now := time.Now().UTC()
	completeReq := quotaLeaseDemoJSONRequest(t, http.MethodPost, "/api/v1/node-leases/demo/accounts/usage-probe-tasks/"+createBody.Task.ID+"/complete", map[string]any{
		"usage": map[string]any{
			"source":     "active",
			"updated_at": now.Format(time.RFC3339),
			"five_hour": map[string]any{
				"utilization": 19.5,
			},
		},
		"extra_patch": map[string]any{
			"codex_5h_used_percent": 19.5,
			"unsafe_key":            "ignored",
		},
	})
	completeReq.Header.Set("X-Node-ID", "foreign-usage-1")
	completeReq.Header.Set("X-Node-Secret", registerBody.NodeSecret)
	completeRec := httptest.NewRecorder()
	router.ServeHTTP(completeRec, completeReq)
	require.Equal(t, http.StatusOK, completeRec.Code)
	require.Equal(t, int64(202), adminSvc.updatedExtraID)
	require.Equal(t, 19.5, adminSvc.updatedExtra["codex_5h_used_percent"])
	require.NotContains(t, adminSvc.updatedExtra, "unsafe_key")
}

func TestQuotaLeaseDemoHandlerPostsUsageLogBatchWithoutBalanceDeduction(t *testing.T) {
	router, svc := newQuotaLeaseDemoHandlerTestRouter(t)
	usageRepo := &quotaLeaseDemoUsageRepoStub{}
	userRepo := &quotaLeaseDemoUserRepoStub{
		user: &service.User{
			ID:     1,
			Status: service.StatusActive,
		},
	}
	usageSvc := service.NewUsageService(usageRepo, userRepo, nil, nil)
	h := NewQuotaLeaseDemoHandler(svc)
	h.SetUsageService(usageSvc)

	group := router.Group("/api/v1/node-leases/demo")
	group.POST("/usage/logs/batch", h.PostUsageLogBatch)

	req := quotaLeaseDemoJSONRequest(t, http.MethodPost, "/api/v1/node-leases/demo/usage/logs/batch", map[string]any{
		"logs": []map[string]any{
			{
				"user_id":     1,
				"api_key_id":  2,
				"request_id":  "req-1",
				"actual_cost": 1.25,
			},
			{
				"user_id":     1,
				"api_key_id":  2,
				"request_id":  "req-1",
				"actual_cost": 1.25,
			},
		},
	})
	req.Header.Set("X-Node-Secret", "control-secret")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	require.Equal(t, http.StatusOK, rec.Code)

	var body service.QuotaLeaseDemoUsageLogBatchResult
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	require.Len(t, body.Results, 2)
	require.True(t, body.Results[0].Applied)
	require.False(t, body.Results[0].Duplicate)
	require.False(t, body.Results[1].Applied)
	require.True(t, body.Results[1].Duplicate)
	require.Empty(t, userRepo.balanceUpdates)
}

func TestQuotaLeaseDemoHandlerPostsOpsErrorLogBatchWithNodeID(t *testing.T) {
	gin.SetMode(gin.TestMode)
	svc := service.NewQuotaLeaseDemoService(&config.Config{
		Gateway: config.GatewayConfig{
			QuotaLeaseDemo: config.GatewayQuotaLeaseDemoConfig{
				Enabled:    true,
				NodeID:     "control-node",
				NodeSecret: "control-secret",
			},
		},
	})
	_, err := svc.RegisterNode(context.Background(), service.QuotaLeaseDemoNodeRegistrationRequest{
		NodeID:     "foreign-1",
		NodeSecret: "node-secret",
	})
	require.NoError(t, err)
	opsRepo := &quotaLeaseDemoOpsRepoStub{}
	opsSvc := service.NewOpsService(opsRepo, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil)
	h := NewQuotaLeaseDemoHandler(svc)
	h.SetOpsService(opsSvc)

	router := gin.New()
	group := router.Group("/api/v1/node-leases/demo")
	group.POST("/ops-error-logs/batch", h.PostOpsErrorLogBatch)

	req := quotaLeaseDemoJSONRequest(t, http.MethodPost, "/api/v1/node-leases/demo/ops-error-logs/batch", map[string]any{
		"node_id": "foreign-1",
		"logs": []map[string]any{
			{
				"request_id":    "err-req-1",
				"user_id":       1,
				"api_key_id":    2,
				"platform":      service.PlatformOpenAI,
				"model":         "gpt-5",
				"error_phase":   "upstream",
				"error_type":    "upstream_error",
				"status_code":   503,
				"error_message": "upstream failed",
				"created_at":    time.Now().UTC(),
			},
		},
	})
	req.Header.Set("X-Node-ID", "foreign-1")
	req.Header.Set("X-Node-Secret", "node-secret")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	require.Equal(t, http.StatusOK, rec.Code)

	var body service.QuotaLeaseDemoOpsErrorLogBatchResult
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	require.Len(t, body.Results, 1)
	require.True(t, body.Results[0].Applied)
	require.Empty(t, body.Results[0].Error)
	require.Len(t, opsRepo.entries, 1)
	require.Equal(t, "foreign-1", opsRepo.entries[0].NodeID)
	require.Equal(t, "err-req-1", opsRepo.entries[0].RequestID)
}
