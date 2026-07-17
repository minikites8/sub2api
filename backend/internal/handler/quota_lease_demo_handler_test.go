package handler

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

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
		group.POST("/nodes/register", h.RegisterNode)
		group.POST("/nodes/heartbeat", h.HeartbeatNode)
		group.GET("/nodes", h.ListNodes)
		group.POST("/accounts/login-tasks", h.CreateAccountLoginTask)
		group.GET("/accounts/login-tasks", h.ListAccountLoginTasks)
		group.POST("/accounts/login-tasks/:task_id/complete", h.CompleteAccountLoginTask)
		group.POST("/accounts/login-tasks/:task_id/progress", h.ReportAccountLoginTaskProgress)
		group.POST("/accounts/login-tasks/:task_id/callback", h.SubmitAccountLoginTaskCallback)
		group.POST("/accounts/status", h.ReportAccountStatus)
		group.GET("/accounts/assignments", h.ListAssignedAccounts)
		group.POST("/leases/request", h.RequestLease)
		group.POST("/usage/batch", h.PostUsageBatch)
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
