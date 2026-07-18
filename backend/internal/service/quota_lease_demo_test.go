package service

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/openai"
	"github.com/Wei-Shaw/sub2api/internal/pkg/xai"
	"github.com/stretchr/testify/require"
)

type quotaLeaseDemoOpenAIOAuthClientStub struct {
	exchangeCode string
}

func (s *quotaLeaseDemoOpenAIOAuthClientStub) ExchangeCode(ctx context.Context, code, codeVerifier, redirectURI, proxyURL, clientID string) (*openai.TokenResponse, error) {
	_ = ctx
	_ = codeVerifier
	_ = redirectURI
	_ = proxyURL
	_ = clientID
	s.exchangeCode = code
	return &openai.TokenResponse{
		AccessToken:  "openai-access",
		RefreshToken: "openai-refresh",
		TokenType:    "Bearer",
		ExpiresIn:    3600,
	}, nil
}

func (s *quotaLeaseDemoOpenAIOAuthClientStub) RefreshToken(ctx context.Context, refreshToken, proxyURL string) (*openai.TokenResponse, error) {
	return s.RefreshTokenWithClientID(ctx, refreshToken, proxyURL, "")
}

func (s *quotaLeaseDemoOpenAIOAuthClientStub) RefreshTokenWithClientID(ctx context.Context, refreshToken, proxyURL string, clientID string) (*openai.TokenResponse, error) {
	_ = ctx
	_ = refreshToken
	_ = proxyURL
	_ = clientID
	return &openai.TokenResponse{AccessToken: "openai-refreshed", ExpiresIn: 3600}, nil
}

type quotaLeaseDemoGrokOAuthClientStub struct {
	exchangeCode string
}

func (s *quotaLeaseDemoGrokOAuthClientStub) ExchangeCode(ctx context.Context, code, codeVerifier, redirectURI, proxyURL, clientID string) (*xai.TokenResponse, error) {
	_ = ctx
	_ = codeVerifier
	_ = redirectURI
	_ = proxyURL
	_ = clientID
	s.exchangeCode = code
	return &xai.TokenResponse{
		AccessToken:  "grok-access",
		RefreshToken: "grok-refresh",
		TokenType:    "Bearer",
		ExpiresIn:    3600,
	}, nil
}

func (s *quotaLeaseDemoGrokOAuthClientStub) RefreshToken(ctx context.Context, refreshToken, proxyURL, clientID string) (*xai.TokenResponse, error) {
	_ = ctx
	_ = refreshToken
	_ = proxyURL
	_ = clientID
	return &xai.TokenResponse{AccessToken: "grok-refreshed", ExpiresIn: 3600}, nil
}

func (s *quotaLeaseDemoGrokOAuthClientStub) ConvertSSOToBuild(ctx context.Context, ssoToken, proxyURL string) (*xai.TokenResponse, error) {
	_ = ctx
	_ = ssoToken
	_ = proxyURL
	return &xai.TokenResponse{AccessToken: "grok-sso-access", RefreshToken: "grok-sso-refresh", ExpiresIn: 3600}, nil
}

func newQuotaLeaseDemoTestService() *QuotaLeaseDemoService {
	return NewQuotaLeaseDemoService(&config.Config{
		Gateway: config.GatewayConfig{
			QuotaLeaseDemo: config.GatewayQuotaLeaseDemoConfig{
				Enabled:                true,
				NodeID:                 "node-1",
				DefaultGrantAmount:     1,
				LeaseTTLSeconds:        600,
				ReclaimGraceSeconds:    3600,
				PreflightReserveAmount: 0.000001,
			},
		},
	})
}

func newQuotaLeaseDemoControlPlaneTestServer(t *testing.T, control *QuotaLeaseDemoService, controlSecret string) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/v1/node-leases/demo/nodes/register":
			var req QuotaLeaseDemoNodeRegistrationRequest
			require.NoError(t, json.NewDecoder(r.Body).Decode(&req))
			if req.RegistrationToken == "" {
				req.RegistrationToken = strings.TrimSpace(r.URL.Query().Get("registration_token"))
			}
			if req.RegistrationToken == "" && r.Header.Get("X-Node-Secret") != controlSecret {
				w.WriteHeader(http.StatusUnauthorized)
				_ = json.NewEncoder(w).Encode(map[string]string{"error": "invalid_node_secret"})
				return
			}
			result, err := control.RegisterNode(r.Context(), req)
			require.NoError(t, err)
			require.NoError(t, json.NewEncoder(w).Encode(result))
		case "/api/v1/node-leases/demo/nodes/heartbeat":
			if !control.AuthenticateNode(r.Header.Get("X-Node-ID"), r.Header.Get("X-Node-Secret")) {
				w.WriteHeader(http.StatusUnauthorized)
				_ = json.NewEncoder(w).Encode(map[string]string{"error": "invalid_node_secret"})
				return
			}
			var req QuotaLeaseDemoNodeHeartbeatRequest
			require.NoError(t, json.NewDecoder(r.Body).Decode(&req))
			node, err := control.HeartbeatNode(r.Context(), req)
			require.NoError(t, err)
			require.NoError(t, json.NewEncoder(w).Encode(map[string]any{"node": node}))
		case "/api/v1/node-leases/demo/leases/request":
			if !control.AuthenticateNode(r.Header.Get("X-Node-ID"), r.Header.Get("X-Node-Secret")) {
				w.WriteHeader(http.StatusUnauthorized)
				_ = json.NewEncoder(w).Encode(map[string]string{"error": "invalid_node_secret"})
				return
			}
			var req QuotaLeaseDemoLeaseRequest
			require.NoError(t, json.NewDecoder(r.Body).Decode(&req))
			lease, err := control.RequestLease(r.Context(), req)
			require.NoError(t, err)
			require.NoError(t, json.NewEncoder(w).Encode(map[string]any{"lease": lease}))
		case "/api/v1/node-leases/demo/usage/batch":
			if !control.AuthenticateNode(r.Header.Get("X-Node-ID"), r.Header.Get("X-Node-Secret")) {
				w.WriteHeader(http.StatusUnauthorized)
				_ = json.NewEncoder(w).Encode(map[string]string{"error": "invalid_node_secret"})
				return
			}
			var req QuotaLeaseDemoUsageBatchRequest
			require.NoError(t, json.NewDecoder(r.Body).Decode(&req))
			require.NoError(t, json.NewEncoder(w).Encode(control.PostUsageBatch(r.Context(), req)))
		case "/api/v1/node-leases/demo/usage-logs/batch":
			if !control.AuthenticateNode(r.Header.Get("X-Node-ID"), r.Header.Get("X-Node-Secret")) {
				w.WriteHeader(http.StatusUnauthorized)
				_ = json.NewEncoder(w).Encode(map[string]string{"error": "invalid_node_secret"})
				return
			}
			var req QuotaLeaseDemoUsageLogBatchRequest
			require.NoError(t, json.NewDecoder(r.Body).Decode(&req))
			result := QuotaLeaseDemoUsageLogBatchResult{Results: make([]QuotaLeaseDemoUsageLogResult, 0, len(req.Logs))}
			for _, item := range req.Logs {
				result.Results = append(result.Results, QuotaLeaseDemoUsageLogResult{
					RequestID: strings.TrimSpace(item.RequestID),
					APIKeyID:  item.APIKeyID,
					Applied:   true,
				})
			}
			require.NoError(t, json.NewEncoder(w).Encode(result))
		case "/api/v1/node-leases/demo/accounts/login-tasks":
			if r.Method == http.MethodPost {
				if r.Header.Get("X-Node-Secret") != controlSecret {
					w.WriteHeader(http.StatusUnauthorized)
					_ = json.NewEncoder(w).Encode(map[string]string{"error": "invalid_node_secret"})
					return
				}
				var req QuotaLeaseDemoAccountLoginTaskCreateRequest
				require.NoError(t, json.NewDecoder(r.Body).Decode(&req))
				task, err := control.CreateAccountLoginTask(r.Context(), req)
				require.NoError(t, err)
				require.NoError(t, json.NewEncoder(w).Encode(map[string]any{"task": task}))
				return
			}
			if !control.AuthenticateNode(r.Header.Get("X-Node-ID"), r.Header.Get("X-Node-Secret")) {
				w.WriteHeader(http.StatusUnauthorized)
				_ = json.NewEncoder(w).Encode(map[string]string{"error": "invalid_node_secret"})
				return
			}
			require.NoError(t, json.NewEncoder(w).Encode(map[string]any{
				"tasks": control.ListAccountLoginTasks(r.Context(), r.Header.Get("X-Node-ID"), r.URL.Query().Get("status")),
			}))
		case "/api/v1/node-leases/demo/accounts/assignments":
			if !control.AuthenticateNode(r.Header.Get("X-Node-ID"), r.Header.Get("X-Node-Secret")) {
				w.WriteHeader(http.StatusUnauthorized)
				_ = json.NewEncoder(w).Encode(map[string]string{"error": "invalid_node_secret"})
				return
			}
			require.NoError(t, json.NewEncoder(w).Encode(map[string]any{
				"accounts": control.ListAssignedAccounts(r.Context(), r.Header.Get("X-Node-ID")),
			}))
		case "/api/v1/node-leases/demo/accounts/status":
			if !control.AuthenticateNode(r.Header.Get("X-Node-ID"), r.Header.Get("X-Node-Secret")) {
				w.WriteHeader(http.StatusUnauthorized)
				_ = json.NewEncoder(w).Encode(map[string]string{"error": "invalid_node_secret"})
				return
			}
			var req QuotaLeaseDemoAccountStatusReportRequest
			require.NoError(t, json.NewDecoder(r.Body).Decode(&req))
			if req.NodeID == "" {
				req.NodeID = r.Header.Get("X-Node-ID")
			}
			account, err := control.ReportAccountStatus(r.Context(), req)
			require.NoError(t, err)
			require.NoError(t, json.NewEncoder(w).Encode(map[string]any{"account": account}))
		default:
			if strings.HasPrefix(r.URL.Path, "/api/v1/node-leases/demo/accounts/login-tasks/") &&
				strings.HasSuffix(r.URL.Path, "/complete") {
				if !control.AuthenticateNode(r.Header.Get("X-Node-ID"), r.Header.Get("X-Node-Secret")) {
					w.WriteHeader(http.StatusUnauthorized)
					_ = json.NewEncoder(w).Encode(map[string]string{"error": "invalid_node_secret"})
					return
				}
				var req QuotaLeaseDemoAccountLoginTaskCompleteRequest
				require.NoError(t, json.NewDecoder(r.Body).Decode(&req))
				task, err := control.CompleteAccountLoginTask(r.Context(), req)
				require.NoError(t, err)
				require.NoError(t, json.NewEncoder(w).Encode(map[string]any{"task": task}))
				return
			}
			if strings.HasPrefix(r.URL.Path, "/api/v1/node-leases/demo/accounts/login-tasks/") &&
				strings.HasSuffix(r.URL.Path, "/progress") {
				if !control.AuthenticateNode(r.Header.Get("X-Node-ID"), r.Header.Get("X-Node-Secret")) {
					w.WriteHeader(http.StatusUnauthorized)
					_ = json.NewEncoder(w).Encode(map[string]string{"error": "invalid_node_secret"})
					return
				}
				var req QuotaLeaseDemoAccountLoginTaskProgressRequest
				require.NoError(t, json.NewDecoder(r.Body).Decode(&req))
				task, err := control.ReportAccountLoginTaskProgress(r.Context(), req)
				require.NoError(t, err)
				require.NoError(t, json.NewEncoder(w).Encode(map[string]any{"task": task}))
				return
			}
			if strings.HasPrefix(r.URL.Path, "/api/v1/node-leases/demo/accounts/login-tasks/") &&
				strings.HasSuffix(r.URL.Path, "/callback") {
				if r.Header.Get("X-Node-Secret") != controlSecret {
					w.WriteHeader(http.StatusUnauthorized)
					_ = json.NewEncoder(w).Encode(map[string]string{"error": "invalid_node_secret"})
					return
				}
				var req QuotaLeaseDemoAccountLoginTaskCallbackRequest
				require.NoError(t, json.NewDecoder(r.Body).Decode(&req))
				task, err := control.SubmitAccountLoginTaskCallback(r.Context(), req)
				require.NoError(t, err)
				require.NoError(t, json.NewEncoder(w).Encode(map[string]any{"task": task}))
				return
			}
			w.WriteHeader(http.StatusNotFound)
			_ = json.NewEncoder(w).Encode(map[string]string{"error": "not_found"})
		}
	}))
}

func TestQuotaLeaseDemoConsumeUsageIsIdempotent(t *testing.T) {
	svc := newQuotaLeaseDemoTestService()
	ctx := context.Background()

	lease, err := svc.RequestLease(ctx, QuotaLeaseDemoLeaseRequest{
		UserID:   10,
		APIKeyID: 20,
		Amount:   1,
	})
	require.NoError(t, err)

	event := QuotaLeaseDemoUsageEvent{
		EventID:   "event-1",
		LeaseID:   lease.ID,
		NodeID:    "node-1",
		UserID:    10,
		APIKeyID:  20,
		RequestID: "req-1",
		Amount:    0.4,
	}
	first, err := svc.ConsumeUsage(ctx, event)
	require.NoError(t, err)
	require.True(t, first.Applied)
	require.False(t, first.Duplicate)
	require.InDelta(t, 0.6, first.Lease.Remaining(), 1e-9)

	second, err := svc.ConsumeUsage(ctx, event)
	require.NoError(t, err)
	require.False(t, second.Applied)
	require.True(t, second.Duplicate)
	require.InDelta(t, 0.6, second.Lease.Remaining(), 1e-9)
}

func TestQuotaLeaseDemoRequestLeaseReusesActiveCapacity(t *testing.T) {
	svc := newQuotaLeaseDemoTestService()
	ctx := context.Background()

	lease, err := svc.RequestLease(ctx, QuotaLeaseDemoLeaseRequest{
		NodeID:   "node-1",
		UserID:   10,
		APIKeyID: 20,
		Amount:   1,
	})
	require.NoError(t, err)

	reused, err := svc.RequestLease(ctx, QuotaLeaseDemoLeaseRequest{
		NodeID:   "node-1",
		UserID:   10,
		APIKeyID: 20,
		Amount:   svc.PreflightReserveAmount(),
	})
	require.NoError(t, err)
	require.Equal(t, lease.ID, reused.ID)

	snapshot := svc.Snapshot()
	require.Len(t, snapshot.Leases, 1)
	require.InDelta(t, 1, snapshot.Leases[0].Remaining(), 1e-9)
}

func TestQuotaLeaseDemoRequestLeaseTopsUpActivePreflightLease(t *testing.T) {
	svc := NewQuotaLeaseDemoService(&config.Config{
		Gateway: config.GatewayConfig{
			QuotaLeaseDemo: config.GatewayQuotaLeaseDemoConfig{
				Enabled:                true,
				NodeID:                 "node-1",
				DefaultGrantAmount:     0.000001,
				LeaseTTLSeconds:        600,
				ReclaimGraceSeconds:    3600,
				PreflightReserveAmount: 0.000001,
			},
		},
	})
	ctx := context.Background()

	preflight, err := svc.RequestLease(ctx, QuotaLeaseDemoLeaseRequest{
		NodeID:   "node-1",
		UserID:   10,
		APIKeyID: 20,
		Amount:   svc.PreflightReserveAmount(),
	})
	require.NoError(t, err)
	require.InDelta(t, 0.000001, preflight.Granted, 1e-12)

	toppedUp, err := svc.RequestLease(ctx, QuotaLeaseDemoLeaseRequest{
		NodeID:   "node-1",
		UserID:   10,
		APIKeyID: 20,
		Amount:   0.005715,
	})
	require.NoError(t, err)
	require.Equal(t, preflight.ID, toppedUp.ID)
	require.InDelta(t, 0.005715, toppedUp.Granted, 1e-12)
	require.InDelta(t, 0.005715, toppedUp.Remaining(), 1e-12)

	snapshot := svc.Snapshot()
	require.Len(t, snapshot.Leases, 1)
	require.Equal(t, preflight.ID, snapshot.Leases[0].ID)
	require.InDelta(t, 0.005715, snapshot.Leases[0].Granted, 1e-12)
}

func TestQuotaLeaseDemoRegisterNodeAndHeartbeat(t *testing.T) {
	svc := newQuotaLeaseDemoTestService()
	ctx := context.Background()

	result, err := svc.RegisterNode(ctx, QuotaLeaseDemoNodeRegistrationRequest{
		NodeID:  "foreign-1",
		Region:  "sg",
		BaseURL: "https://foreign-1.example",
		Metadata: map[string]string{
			"zone": "a",
		},
	})
	require.NoError(t, err)
	require.Equal(t, "foreign-1", result.Node.NodeID)
	require.Equal(t, "sg", result.Node.Region)
	require.True(t, strings.HasPrefix(result.NodeSecret, "qln_"))
	require.True(t, svc.AuthenticateNode("foreign-1", result.NodeSecret))

	node, err := svc.HeartbeatNode(ctx, QuotaLeaseDemoNodeHeartbeatRequest{
		NodeID:           "foreign-1",
		InflightRequests: 7,
		LeaseRemaining:   0.75,
		Metrics: map[string]float64{
			"rps": 12,
		},
	})
	require.NoError(t, err)
	require.Equal(t, QuotaLeaseDemoNodeStatusOnline, node.Status)
	require.Equal(t, 7, node.InflightRequests)
	require.InDelta(t, 0.75, node.LeaseRemaining, 1e-9)
	require.Equal(t, 12.0, node.Metrics["rps"])

	snapshot := svc.Snapshot()
	require.Equal(t, 1, snapshot.Stats.NodeCount)
	require.Equal(t, 1, snapshot.Stats.OnlineNodes)
	require.Len(t, snapshot.Nodes, 1)
	require.Equal(t, "foreign-1", snapshot.Nodes[0].NodeID)
}

func TestQuotaLeaseDemoApplyUsageBillingConsumesLocalLease(t *testing.T) {
	svc := newQuotaLeaseDemoTestService()
	ctx := context.Background()
	_, err := svc.RequestLease(ctx, QuotaLeaseDemoLeaseRequest{
		UserID:   10,
		APIKeyID: 20,
		Amount:   1,
	})
	require.NoError(t, err)

	handled, applied, err := svc.ApplyUsageBilling(ctx, &UsageBillingCommand{
		RequestID:   "req-1",
		UserID:      10,
		APIKeyID:    20,
		BalanceCost: 0.25,
	})
	require.NoError(t, err)
	require.True(t, handled)
	require.True(t, applied)

	snapshot := svc.Snapshot()
	require.Len(t, snapshot.Leases, 1)
	require.InDelta(t, 0.75, snapshot.Leases[0].Remaining(), 1e-9)

	handled, applied, err = svc.ApplyUsageBilling(ctx, &UsageBillingCommand{
		RequestID:   "req-1",
		UserID:      10,
		APIKeyID:    20,
		BalanceCost: 0.25,
	})
	require.NoError(t, err)
	require.True(t, handled)
	require.False(t, applied)
	require.InDelta(t, 0.75, svc.Snapshot().Leases[0].Remaining(), 1e-9)
}

func TestQuotaLeaseDemoRemoteNodeFetchesLeaseAndFlushesUsage(t *testing.T) {
	control := newQuotaLeaseDemoTestService()
	server := newQuotaLeaseDemoControlPlaneTestServer(t, control, "control-secret")
	defer server.Close()

	node := NewQuotaLeaseDemoService(&config.Config{
		Gateway: config.GatewayConfig{
			QuotaLeaseDemo: config.GatewayQuotaLeaseDemoConfig{
				Enabled:             true,
				ControlPlaneBaseURL: server.URL,
				ControlPlaneKey:     "control-secret",
			},
		},
	})
	ctx := context.Background()

	lease, err := node.RequestLease(ctx, QuotaLeaseDemoLeaseRequest{
		UserID:   10,
		APIKeyID: 20,
	})
	require.NoError(t, err)
	require.NotEmpty(t, lease.NodeID)
	require.InDelta(t, 1, lease.Granted, 1e-9)
	require.Len(t, node.Snapshot().Leases, 1)
	require.Len(t, control.Snapshot().Leases, 1)

	heartbeat, err := node.HeartbeatNode(ctx, QuotaLeaseDemoNodeHeartbeatRequest{
		InflightRequests: 3,
		LeaseRemaining:   0.9,
	})
	require.NoError(t, err)
	require.Equal(t, lease.NodeID, heartbeat.NodeID)
	require.Equal(t, 3, heartbeat.InflightRequests)

	handled, applied, err := node.ApplyUsageBilling(ctx, &UsageBillingCommand{
		RequestID:   "remote-req-1",
		UserID:      10,
		APIKeyID:    20,
		BalanceCost: 0.25,
	})
	require.NoError(t, err)
	require.True(t, handled)
	require.True(t, applied)
	require.InDelta(t, 0.75, node.Snapshot().Leases[0].Remaining(), 1e-9)

	require.NoError(t, node.FlushPendingUsage(ctx))
	controlSnapshot := control.Snapshot()
	require.Len(t, controlSnapshot.Leases, 1)
	require.InDelta(t, 0.25, controlSnapshot.Leases[0].Consumed, 1e-9)
	require.Len(t, node.pendingUsageEvents(), 0)
}

func TestQuotaLeaseDemoRemoteNodeFlushesUsageLogs(t *testing.T) {
	control := newQuotaLeaseDemoTestService()
	var received []QuotaLeaseDemoUsageLogSnapshot
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/v1/node-leases/demo/nodes/register":
			if r.Header.Get("X-Node-Secret") != "control-secret" {
				w.WriteHeader(http.StatusUnauthorized)
				_ = json.NewEncoder(w).Encode(map[string]string{"error": "invalid_node_secret"})
				return
			}
			var req QuotaLeaseDemoNodeRegistrationRequest
			require.NoError(t, json.NewDecoder(r.Body).Decode(&req))
			result, err := control.RegisterNode(r.Context(), req)
			require.NoError(t, err)
			require.NoError(t, json.NewEncoder(w).Encode(result))
		case "/api/v1/node-leases/demo/usage-logs/batch":
			if !control.AuthenticateNode(r.Header.Get("X-Node-ID"), r.Header.Get("X-Node-Secret")) {
				w.WriteHeader(http.StatusUnauthorized)
				_ = json.NewEncoder(w).Encode(map[string]string{"error": "invalid_node_secret"})
				return
			}
			var req QuotaLeaseDemoUsageLogBatchRequest
			require.NoError(t, json.NewDecoder(r.Body).Decode(&req))
			received = append(received, req.Logs...)
			result := QuotaLeaseDemoUsageLogBatchResult{Results: make([]QuotaLeaseDemoUsageLogResult, 0, len(req.Logs))}
			for _, item := range req.Logs {
				result.Results = append(result.Results, QuotaLeaseDemoUsageLogResult{
					RequestID: item.RequestID,
					APIKeyID:  item.APIKeyID,
					Applied:   true,
				})
			}
			require.NoError(t, json.NewEncoder(w).Encode(result))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	node := NewQuotaLeaseDemoService(&config.Config{
		Gateway: config.GatewayConfig{
			QuotaLeaseDemo: config.GatewayQuotaLeaseDemoConfig{
				Enabled:             true,
				NodeID:              "node-us",
				ControlPlaneBaseURL: server.URL,
				ControlPlaneKey:     "control-secret",
			},
		},
	})
	ctx := context.Background()
	serviceTier := "priority"
	durationMs := 321
	accountRate := 1.2
	snapshot := NewQuotaLeaseDemoUsageLogSnapshot("", &UsageLog{
		UserID:                10,
		APIKeyID:              20,
		AccountID:             30,
		RequestID:             "usage-log-req-1",
		Model:                 "gpt-5",
		RequestedModel:        "gpt-5",
		ServiceTier:           &serviceTier,
		InputTokens:           11,
		OutputTokens:          7,
		CacheCreationTokens:   3,
		CacheReadTokens:       5,
		TotalCost:             0.45,
		ActualCost:            0.4,
		RateMultiplier:        1.1,
		AccountRateMultiplier: &accountRate,
		BillingType:           BillingTypeBalance,
		RequestType:           RequestTypeStream,
		DurationMs:            &durationMs,
		CreatedAt:             time.Now().UTC(),
	})
	node.enqueuePendingUsageLogSnapshot(snapshot)

	require.NoError(t, node.FlushPendingUsageLogs(ctx))
	require.Len(t, received, 1)
	require.Equal(t, "node-us", received[0].NodeID)
	require.Equal(t, int64(20), received[0].APIKeyID)
	require.Equal(t, "usage-log-req-1", received[0].RequestID)
	require.Equal(t, RequestTypeStream, received[0].RequestType)
	require.Equal(t, 11, received[0].InputTokens)
	require.InDelta(t, 0.4, received[0].ActualCost, 1e-9)
	require.Len(t, node.pendingUsageLogSnapshots(), 0)
}

func TestQuotaLeaseDemoRemoteNodeAuthorizesClientKeyViaControlPlane(t *testing.T) {
	ctx := context.Background()
	control := newQuotaLeaseDemoTestService()
	authCalls := 0
	groupID := int64(30)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/v1/node-leases/demo/nodes/register":
			if r.Header.Get("X-Node-Secret") != "control-secret" {
				w.WriteHeader(http.StatusUnauthorized)
				_ = json.NewEncoder(w).Encode(map[string]string{"error": "invalid_node_secret"})
				return
			}
			var req QuotaLeaseDemoNodeRegistrationRequest
			require.NoError(t, json.NewDecoder(r.Body).Decode(&req))
			result, err := control.RegisterNode(r.Context(), req)
			require.NoError(t, err)
			require.NoError(t, json.NewEncoder(w).Encode(result))
		case "/api/v1/node-leases/demo/auth/client-key":
			if !control.AuthenticateNode(r.Header.Get("X-Node-ID"), r.Header.Get("X-Node-Secret")) {
				w.WriteHeader(http.StatusUnauthorized)
				_ = json.NewEncoder(w).Encode(map[string]string{"error": "invalid_node_secret"})
				return
			}
			authCalls++
			var req QuotaLeaseDemoClientAuthRequest
			require.NoError(t, json.NewDecoder(r.Body).Decode(&req))
			if req.APIKey != "sk-live-user" {
				w.WriteHeader(http.StatusUnauthorized)
				_ = json.NewEncoder(w).Encode(map[string]string{"error": "invalid_api_key"})
				return
			}
			lease, err := control.RequestLease(r.Context(), QuotaLeaseDemoLeaseRequest{
				NodeID:   req.NodeID,
				UserID:   10,
				APIKeyID: 20,
				Amount:   req.Amount,
			})
			require.NoError(t, err)
			require.NoError(t, json.NewEncoder(w).Encode(QuotaLeaseDemoClientAuthResult{
				Snapshot: &APIKeyAuthSnapshot{
					Version:  apiKeyAuthSnapshotVersion,
					APIKeyID: 20,
					UserID:   10,
					GroupID:  &groupID,
					Name:     "client",
					Status:   StatusActive,
					User: APIKeyAuthUserSnapshot{
						ID:          10,
						Status:      StatusActive,
						Role:        RoleUser,
						Balance:     5,
						Concurrency: 2,
					},
					Group: &APIKeyAuthGroupSnapshot{
						ID:             groupID,
						Name:           "openai",
						Platform:       PlatformOpenAI,
						Status:         StatusActive,
						RateMultiplier: 1,
					},
				},
				Lease:     lease,
				ExpiresAt: time.Now().UTC().Add(30 * time.Second),
			}))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	node := NewQuotaLeaseDemoService(&config.Config{
		Gateway: config.GatewayConfig{
			QuotaLeaseDemo: config.GatewayQuotaLeaseDemoConfig{
				Enabled:                true,
				NodeID:                 "node-us",
				ControlPlaneBaseURL:    server.URL,
				ControlPlaneKey:        "control-secret",
				DefaultGrantAmount:     1,
				LeaseTTLSeconds:        600,
				ReclaimGraceSeconds:    3600,
				PreflightReserveAmount: 0.000001,
			},
		},
	})

	result, err := node.AuthorizeClientKeyViaControlPlane(ctx, "sk-live-user", 0)
	require.NoError(t, err)
	require.NotNil(t, result.Snapshot)
	require.Equal(t, int64(20), result.Snapshot.APIKeyID)
	require.NotNil(t, result.Lease)
	require.True(t, node.hasCapacity("node-us", 10, 20, node.PreflightReserveAmount(), time.Now().UTC()))

	cached, err := node.AuthorizeClientKeyViaControlPlane(ctx, "sk-live-user", 0)
	require.NoError(t, err)
	require.Equal(t, result.Lease.ID, cached.Lease.ID)
	require.Equal(t, 1, authCalls)
}

func TestQuotaLeaseDemoAccountLoginTaskAssignsNodeAccount(t *testing.T) {
	svc := newQuotaLeaseDemoTestService()
	ctx := context.Background()

	task, err := svc.CreateAccountLoginTask(ctx, QuotaLeaseDemoAccountLoginTaskCreateRequest{
		AccountID:      101,
		Name:           "gpt-oauth-1",
		Platform:       PlatformOpenAI,
		Type:           AccountTypeOAuth,
		AssignedNodeID: "foreign-1",
		GroupIDs:       []int64{7},
		LoginPayload: map[string]any{
			"auth_url": "https://auth.example/start",
		},
	})
	require.NoError(t, err)
	require.Equal(t, QuotaLeaseDemoAccountTaskPending, task.Status)

	tasks := svc.ListAccountLoginTasks(ctx, "foreign-1", QuotaLeaseDemoAccountTaskPending)
	require.Len(t, tasks, 1)
	require.Equal(t, task.ID, tasks[0].ID)

	completed, err := svc.CompleteAccountLoginTask(ctx, QuotaLeaseDemoAccountLoginTaskCompleteRequest{
		TaskID: task.ID,
		NodeID: "foreign-1",
		Account: QuotaLeaseDemoAccountSnapshot{
			Credentials: map[string]any{
				"access_token":  "node-access-token",
				"refresh_token": "node-refresh-token",
			},
		},
	})
	require.NoError(t, err)
	require.Equal(t, QuotaLeaseDemoAccountTaskCompleted, completed.Status)

	assigned := svc.ListAssignedAccounts(ctx, "foreign-1")
	require.Len(t, assigned, 1)
	require.Equal(t, int64(101), assigned[0].Account.ID)
	require.Equal(t, "node-access-token", assigned[0].Account.Credentials["access_token"])
	require.Equal(t, []int64{7}, assigned[0].Account.GroupIDs)
}

func TestQuotaLeaseDemoRemoteNodeSyncsAssignedAccountsForScheduling(t *testing.T) {
	control := newQuotaLeaseDemoTestService()
	server := newQuotaLeaseDemoControlPlaneTestServer(t, control, "control-secret")
	defer server.Close()

	node := NewQuotaLeaseDemoService(&config.Config{
		Gateway: config.GatewayConfig{
			QuotaLeaseDemo: config.GatewayQuotaLeaseDemoConfig{
				Enabled:             true,
				ControlPlaneBaseURL: server.URL,
				ControlPlaneKey:     "control-secret",
			},
		},
	})
	ctx := context.Background()
	register, err := node.RegisterNode(ctx, QuotaLeaseDemoNodeRegistrationRequest{})
	require.NoError(t, err)
	nodeID := register.Node.NodeID

	task, err := control.CreateAccountLoginTask(ctx, QuotaLeaseDemoAccountLoginTaskCreateRequest{
		AccountID:      202,
		Name:           "grok-oauth-1",
		Platform:       PlatformGrok,
		Type:           AccountTypeOAuth,
		AssignedNodeID: nodeID,
	})
	require.NoError(t, err)

	tasks := node.ListAccountLoginTasks(ctx, "", QuotaLeaseDemoAccountTaskPending)
	require.Len(t, tasks, 1)
	require.Equal(t, task.ID, tasks[0].ID)

	proxyID := int64(303)
	groupID := int64(7)
	_, err = node.CompleteAccountLoginTask(ctx, QuotaLeaseDemoAccountLoginTaskCompleteRequest{
		TaskID: task.ID,
		Account: QuotaLeaseDemoAccountSnapshot{
			ID:       202,
			Platform: PlatformGrok,
			Type:     AccountTypeOAuth,
			Credentials: map[string]any{
				"access_token": "grok-node-access",
			},
			ProxyID: &proxyID,
			Proxy: &QuotaLeaseDemoProxySnapshot{
				ID:       proxyID,
				Protocol: "socks5",
				Host:     "127.0.0.1",
				Port:     19090,
				Username: "grok-user",
				Password: "grok-pass",
				Status:   StatusActive,
			},
			Status:      StatusActive,
			Schedulable: true,
			Concurrency: 2,
			GroupIDs:    []int64{groupID},
		},
	})
	require.NoError(t, err)

	require.NoError(t, node.SyncAssignedAccounts(ctx))
	accounts, handled := node.AssignedAccountsForScheduling(ctx, &groupID, PlatformGrok)
	require.True(t, handled)
	require.Len(t, accounts, 1)
	require.Equal(t, int64(202), accounts[0].ID)
	require.Equal(t, "grok-node-access", accounts[0].Credentials["access_token"])
	require.NotNil(t, accounts[0].ProxyID)
	require.Equal(t, proxyID, *accounts[0].ProxyID)
	require.NotNil(t, accounts[0].Proxy)
	require.Equal(t, "socks5://grok-user:grok-pass@127.0.0.1:19090", accounts[0].Proxy.URL())

	control.mu.Lock()
	delete(control.assignedAccounts, 202)
	control.mu.Unlock()

	require.NoError(t, node.SyncAssignedAccounts(ctx))
	accounts, handled = node.AssignedAccountsForScheduling(ctx, &groupID, PlatformGrok)
	require.True(t, handled)
	require.Empty(t, accounts)
}

func TestQuotaLeaseDemoRemoteNodeRegistersWithRegistrationURLAndNodeSecret(t *testing.T) {
	control := newQuotaLeaseDemoTestService()
	server := newQuotaLeaseDemoControlPlaneTestServer(t, control, "control-secret")
	defer server.Close()

	ctx := context.Background()
	created, err := control.CreateNodeRegistrationURL(ctx, QuotaLeaseDemoNodeRegistrationURLRequest{
		NodeID:     "foreign-url-1",
		Region:     "us-west",
		BaseURL:    "https://foreign-url-1.example",
		Metadata:   map[string]string{"pool": "us"},
		TTLSeconds: 600,
	}, server.URL)
	require.NoError(t, err)
	require.Contains(t, created.RegistrationURL, "registration_token=")

	node := NewQuotaLeaseDemoService(&config.Config{
		Gateway: config.GatewayConfig{
			QuotaLeaseDemo: config.GatewayQuotaLeaseDemoConfig{
				Enabled:                true,
				RegistrationURL:        created.RegistrationURL,
				DefaultGrantAmount:     1,
				LeaseTTLSeconds:        600,
				ReclaimGraceSeconds:    3600,
				PreflightReserveAmount: 0.000001,
			},
		},
	})

	registered, err := node.RegisterNode(ctx, QuotaLeaseDemoNodeRegistrationRequest{})
	require.NoError(t, err)
	require.Equal(t, "foreign-url-1", registered.Node.NodeID)
	require.Equal(t, "us-west", registered.Node.Region)
	require.Equal(t, "https://foreign-url-1.example", registered.Node.BaseURL)
	require.Equal(t, "us", registered.Node.Metadata["pool"])
	require.NotEmpty(t, registered.NodeSecret)
	require.True(t, control.AuthenticateNode("foreign-url-1", registered.NodeSecret))

	heartbeat, err := node.HeartbeatNode(ctx, QuotaLeaseDemoNodeHeartbeatRequest{
		InflightRequests: 1,
		LeaseRemaining:   0.75,
	})
	require.NoError(t, err)
	require.Equal(t, "foreign-url-1", heartbeat.NodeID)
	require.Equal(t, 1, heartbeat.InflightRequests)
}

func TestQuotaLeaseDemoRemoteNodeUsesRegisteredNodeIDForSchedulingCache(t *testing.T) {
	control := newQuotaLeaseDemoTestService()
	server := newQuotaLeaseDemoControlPlaneTestServer(t, control, "control-secret")
	defer server.Close()

	ctx := context.Background()
	registered, err := control.RegisterNode(ctx, QuotaLeaseDemoNodeRegistrationRequest{
		NodeID: "registered-node-1",
	})
	require.NoError(t, err)

	node := NewQuotaLeaseDemoService(&config.Config{
		Gateway: config.GatewayConfig{
			QuotaLeaseDemo: config.GatewayQuotaLeaseDemoConfig{
				Enabled:             true,
				NodeID:              "configured-node-1",
				ControlPlaneBaseURL: server.URL,
				ControlPlaneKey:     "control-secret",
			},
		},
	})
	node.remoteNodeID = registered.Node.NodeID
	node.remoteNodeSecret = registered.NodeSecret

	groupID := int64(12)
	task, err := control.CreateAccountLoginTask(ctx, QuotaLeaseDemoAccountLoginTaskCreateRequest{
		AccountID:      606,
		Name:           "gpt-oauth-registered-node",
		Platform:       PlatformOpenAI,
		Type:           AccountTypeOAuth,
		AssignedNodeID: registered.Node.NodeID,
		GroupIDs:       []int64{groupID},
	})
	require.NoError(t, err)
	_, err = control.CompleteAccountLoginTask(ctx, QuotaLeaseDemoAccountLoginTaskCompleteRequest{
		TaskID: task.ID,
		NodeID: registered.Node.NodeID,
		Account: QuotaLeaseDemoAccountSnapshot{
			ID:       606,
			Platform: PlatformOpenAI,
			Type:     AccountTypeOAuth,
			Credentials: map[string]any{
				"access_token": "node-access-token",
			},
			Status:      StatusActive,
			Schedulable: true,
			Concurrency: 1,
			GroupIDs:    []int64{groupID},
		},
	})
	require.NoError(t, err)

	accounts, handled := node.AssignedAccountsForScheduling(ctx, &groupID, PlatformOpenAI)
	require.True(t, handled)
	require.Len(t, accounts, 1)
	require.Equal(t, int64(606), accounts[0].ID)
	require.Equal(t, "registered-node-1", node.activeNodeID())
	require.Equal(t, "configured-node-1", node.NodeID())
}

func TestQuotaLeaseDemoRemotePreflightUsesRegisteredNodeLease(t *testing.T) {
	control := NewQuotaLeaseDemoService(&config.Config{
		Gateway: config.GatewayConfig{
			QuotaLeaseDemo: config.GatewayQuotaLeaseDemoConfig{
				Enabled:                true,
				NodeID:                 "control-node",
				DefaultGrantAmount:     0.000001,
				LeaseTTLSeconds:        600,
				ReclaimGraceSeconds:    3600,
				PreflightReserveAmount: 0.000001,
			},
		},
	})
	server := newQuotaLeaseDemoControlPlaneTestServer(t, control, "control-secret")
	defer server.Close()

	ctx := context.Background()
	registered, err := control.RegisterNode(ctx, QuotaLeaseDemoNodeRegistrationRequest{
		NodeID: "registered-node-lease",
	})
	require.NoError(t, err)

	node := NewQuotaLeaseDemoService(&config.Config{
		Gateway: config.GatewayConfig{
			QuotaLeaseDemo: config.GatewayQuotaLeaseDemoConfig{
				Enabled:                true,
				NodeID:                 "configured-node-lease",
				ControlPlaneBaseURL:    server.URL,
				ControlPlaneKey:        "control-secret",
				DefaultGrantAmount:     0.000001,
				LeaseTTLSeconds:        600,
				ReclaimGraceSeconds:    3600,
				PreflightReserveAmount: 0.000001,
			},
		},
	})
	node.remoteNodeID = registered.Node.NodeID
	node.remoteNodeSecret = registered.NodeSecret

	require.True(t, node.CanAuthorizeRequest(ctx, &APIKey{
		ID:   20,
		User: &User{ID: 10},
	}, nil))

	preflightSnapshot := control.Snapshot()
	require.Len(t, preflightSnapshot.Leases, 1)
	preflightLease := preflightSnapshot.Leases[0]
	require.Equal(t, registered.Node.NodeID, preflightLease.NodeID)
	require.InDelta(t, 0.000001, preflightLease.Granted, 1e-12)

	handled, applied, err := node.ApplyUsageBilling(ctx, &UsageBillingCommand{
		RequestID:   "remote-preflight-billing-1",
		UserID:      10,
		APIKeyID:    20,
		BalanceCost: 0.005715,
	})
	require.NoError(t, err)
	require.True(t, handled)
	require.True(t, applied)

	controlSnapshot := control.Snapshot()
	require.Len(t, controlSnapshot.Leases, 1)
	require.Equal(t, preflightLease.ID, controlSnapshot.Leases[0].ID)
	require.InDelta(t, 0.005715, controlSnapshot.Leases[0].Granted, 1e-12)
	nodeSnapshot := node.Snapshot()
	require.Len(t, nodeSnapshot.Leases, 1)
	require.Equal(t, preflightLease.ID, nodeSnapshot.Leases[0].ID)
}

func TestQuotaLeaseDemoNodeWorkerExecutesPendingAccountTask(t *testing.T) {
	control := newQuotaLeaseDemoTestService()
	server := newQuotaLeaseDemoControlPlaneTestServer(t, control, "control-secret")
	defer server.Close()

	node := NewQuotaLeaseDemoService(&config.Config{
		Gateway: config.GatewayConfig{
			QuotaLeaseDemo: config.GatewayQuotaLeaseDemoConfig{
				Enabled:             true,
				ControlPlaneBaseURL: server.URL,
				ControlPlaneKey:     "control-secret",
			},
		},
	})
	ctx := context.Background()
	register, err := node.RegisterNode(ctx, QuotaLeaseDemoNodeRegistrationRequest{})
	require.NoError(t, err)
	nodeID := register.Node.NodeID

	task, err := control.CreateAccountLoginTask(ctx, QuotaLeaseDemoAccountLoginTaskCreateRequest{
		AccountID:      303,
		Name:           "gpt-oauth-worker",
		Platform:       PlatformOpenAI,
		Type:           AccountTypeOAuth,
		AssignedNodeID: nodeID,
		LoginPayload: map[string]any{
			"credentials": map[string]any{
				"access_token":  "worker-access-token",
				"refresh_token": "worker-refresh-token",
			},
			"extra": map[string]any{
				"source": "node-worker",
			},
		},
		Concurrency: 2,
	})
	require.NoError(t, err)

	worker := NewQuotaLeaseDemoNodeWorker(node, NewQuotaLeaseDemoPayloadAccountTaskExecutor(), time.Millisecond)
	require.NoError(t, worker.RunOnce(ctx))

	tasks := control.ListAccountLoginTasks(ctx, nodeID, QuotaLeaseDemoAccountTaskCompleted)
	require.Len(t, tasks, 1)
	require.Equal(t, task.ID, tasks[0].ID)

	assigned := control.ListAssignedAccounts(ctx, nodeID)
	require.Len(t, assigned, 1)
	require.Equal(t, int64(303), assigned[0].Account.ID)
	require.Equal(t, "worker-access-token", assigned[0].Account.Credentials["access_token"])
	require.Equal(t, "node-worker", assigned[0].Account.Extra["source"])
	require.True(t, assigned[0].Account.Schedulable)
	require.Equal(t, 2, assigned[0].Account.Concurrency)

	cached := node.ListAssignedAccounts(ctx, nodeID)
	require.Len(t, cached, 1)
	require.Equal(t, int64(303), cached[0].Account.ID)
}

func TestQuotaLeaseDemoOAuthExecutorGeneratesOpenAIURLAndExchangesCode(t *testing.T) {
	client := &quotaLeaseDemoOpenAIOAuthClientStub{}
	openaiSvc := NewOpenAIOAuthService(nil, client)
	defer openaiSvc.Stop()
	executor := NewQuotaLeaseDemoOAuthAccountTaskExecutor(openaiSvc, nil)
	ctx := context.Background()
	task := QuotaLeaseDemoAccountLoginTask{
		ID:             "task-openai",
		AccountID:      505,
		Name:           "openai-real-oauth",
		Platform:       PlatformOpenAI,
		Type:           AccountTypeOAuth,
		AssignedNodeID: "node-1",
		LoginPayload:   map[string]any{},
		Concurrency:    1,
		Status:         QuotaLeaseDemoAccountTaskPending,
	}

	_, err := executor.ExecuteAccountLoginTask(ctx, task)
	var progressErr *QuotaLeaseDemoAccountLoginProgressError
	require.ErrorAs(t, err, &progressErr)
	require.Equal(t, QuotaLeaseDemoAccountTaskWaiting, progressErr.Status)
	require.NotEmpty(t, progressErr.LoginPayloadPatch["auth_url"])
	require.NotEmpty(t, progressErr.LoginPayloadPatch["session_id"])
	require.NotEmpty(t, progressErr.LoginPayloadPatch["state"])

	task.LoginPayload = mergeQuotaLeaseDemoAnyPatch(task.LoginPayload, progressErr.LoginPayloadPatch)
	task.LoginPayload["code"] = "openai-code"
	task.LoginPayload["credential_overrides"] = map[string]any{
		"model_mapping": map[string]any{"gpt-5": "gpt-5-node"},
	}
	account, err := executor.ExecuteAccountLoginTask(ctx, task)
	require.NoError(t, err)
	require.Equal(t, "openai-code", client.exchangeCode)
	require.Equal(t, int64(505), account.ID)
	require.Equal(t, PlatformOpenAI, account.Platform)
	require.Equal(t, "openai-access", account.Credentials["access_token"])
	require.Equal(t, "openai-refresh", account.Credentials["refresh_token"])
	require.Equal(t, map[string]any{"gpt-5": "gpt-5-node"}, account.Credentials["model_mapping"])
	require.True(t, account.Schedulable)
}

func TestQuotaLeaseDemoOAuthExecutorGeneratesGrokURLAndExchangesCallbackURL(t *testing.T) {
	client := &quotaLeaseDemoGrokOAuthClientStub{}
	grokSvc := NewGrokOAuthService(nil, client)
	defer grokSvc.Stop()
	executor := NewQuotaLeaseDemoOAuthAccountTaskExecutor(nil, grokSvc)
	ctx := context.Background()
	task := QuotaLeaseDemoAccountLoginTask{
		ID:             "task-grok",
		AccountID:      506,
		Name:           "grok-real-oauth",
		Platform:       PlatformGrok,
		Type:           AccountTypeOAuth,
		AssignedNodeID: "node-1",
		LoginPayload:   map[string]any{},
		Concurrency:    1,
		Status:         QuotaLeaseDemoAccountTaskPending,
	}

	_, err := executor.ExecuteAccountLoginTask(ctx, task)
	var progressErr *QuotaLeaseDemoAccountLoginProgressError
	require.ErrorAs(t, err, &progressErr)
	require.Equal(t, QuotaLeaseDemoAccountTaskWaiting, progressErr.Status)
	require.NotEmpty(t, progressErr.LoginPayloadPatch["auth_url"])
	require.NotEmpty(t, progressErr.LoginPayloadPatch["session_id"])
	require.NotEmpty(t, progressErr.LoginPayloadPatch["state"])

	task.LoginPayload = mergeQuotaLeaseDemoAnyPatch(task.LoginPayload, progressErr.LoginPayloadPatch)
	task.LoginPayload["callback_url"] = "http://127.0.0.1:56121/callback?code=grok-code&state=" + task.LoginPayload["state"].(string)
	account, err := executor.ExecuteAccountLoginTask(ctx, task)
	require.NoError(t, err)
	require.Equal(t, "grok-code", client.exchangeCode)
	require.Equal(t, int64(506), account.ID)
	require.Equal(t, PlatformGrok, account.Platform)
	require.Equal(t, "grok-access", account.Credentials["access_token"])
	require.Equal(t, "grok-refresh", account.Credentials["refresh_token"])
	require.Equal(t, xai.DefaultCLIBaseURL, account.Credentials["base_url"])
	require.True(t, account.Schedulable)
}

func TestQuotaLeaseDemoRemoteNodeReportsAccountStatus(t *testing.T) {
	control := newQuotaLeaseDemoTestService()
	server := newQuotaLeaseDemoControlPlaneTestServer(t, control, "control-secret")
	defer server.Close()

	node := NewQuotaLeaseDemoService(&config.Config{
		Gateway: config.GatewayConfig{
			QuotaLeaseDemo: config.GatewayQuotaLeaseDemoConfig{
				Enabled:             true,
				ControlPlaneBaseURL: server.URL,
				ControlPlaneKey:     "control-secret",
			},
		},
	})
	ctx := context.Background()
	register, err := node.RegisterNode(ctx, QuotaLeaseDemoNodeRegistrationRequest{})
	require.NoError(t, err)
	nodeID := register.Node.NodeID

	task, err := control.CreateAccountLoginTask(ctx, QuotaLeaseDemoAccountLoginTaskCreateRequest{
		AccountID:      404,
		Name:           "grok-oauth-status",
		Platform:       PlatformGrok,
		Type:           AccountTypeOAuth,
		AssignedNodeID: nodeID,
	})
	require.NoError(t, err)
	_, err = control.CompleteAccountLoginTask(ctx, QuotaLeaseDemoAccountLoginTaskCompleteRequest{
		TaskID: task.ID,
		NodeID: nodeID,
		Account: QuotaLeaseDemoAccountSnapshot{
			ID:       404,
			Platform: PlatformGrok,
			Type:     AccountTypeOAuth,
			Credentials: map[string]any{
				"access_token":  "grok-access-old",
				"refresh_token": "grok-refresh",
			},
			Status:      StatusActive,
			Schedulable: true,
			Concurrency: 1,
		},
	})
	require.NoError(t, err)

	schedulable := false
	errorMessage := "oauth token cooling down"
	resetAt := time.Now().UTC().Add(time.Minute)
	tempUntil := time.Now().UTC().Add(2 * time.Minute)
	tempReason := "node runtime 401"
	updated, err := node.ReportAccountStatus(ctx, QuotaLeaseDemoAccountStatusReportRequest{
		AccountID:               404,
		Schedulable:             &schedulable,
		ErrorMessage:            &errorMessage,
		CredentialsPatch:        map[string]any{"access_token": "grok-access-new"},
		ExtraPatch:              map[string]any{"last_node_error": "401"},
		RateLimitResetAt:        &resetAt,
		TempUnschedulableUntil:  &tempUntil,
		TempUnschedulableReason: &tempReason,
	})
	require.NoError(t, err)
	require.Equal(t, int64(404), updated.Account.ID)
	require.False(t, updated.Account.Schedulable)

	assigned := control.ListAssignedAccounts(ctx, nodeID)
	require.Len(t, assigned, 1)
	require.Equal(t, "oauth token cooling down", assigned[0].Account.ErrorMessage)
	require.Equal(t, "grok-access-new", assigned[0].Account.Credentials["access_token"])
	require.Equal(t, "grok-refresh", assigned[0].Account.Credentials["refresh_token"])
	require.Equal(t, "401", assigned[0].Account.Extra["last_node_error"])
	require.NotNil(t, assigned[0].Account.RateLimitResetAt)
	require.NotNil(t, assigned[0].Account.TempUnschedulableUntil)

	_, handled := node.AssignedAccountByID(ctx, 404)
	require.True(t, handled)
}

func TestQuotaLeaseDemoReclaimExpiredLease(t *testing.T) {
	svc := NewQuotaLeaseDemoService(&config.Config{
		Gateway: config.GatewayConfig{
			QuotaLeaseDemo: config.GatewayQuotaLeaseDemoConfig{
				Enabled:                true,
				NodeID:                 "node-1",
				DefaultGrantAmount:     1,
				LeaseTTLSeconds:        1,
				ReclaimGraceSeconds:    1,
				PreflightReserveAmount: 0.000001,
			},
		},
	})
	ctx := context.Background()
	lease, err := svc.RequestLease(ctx, QuotaLeaseDemoLeaseRequest{
		UserID:   10,
		APIKeyID: 20,
		Amount:   1,
	})
	require.NoError(t, err)

	result := svc.ReclaimExpired(ctx, lease.ReclaimAt.Add(time.Second))
	require.Equal(t, 1, result.ExpiredCount)
	require.Equal(t, 1, result.ReclaimedCount)
	require.InDelta(t, 1, result.ReclaimedTotal, 1e-9)

	snapshot := svc.Snapshot()
	require.Equal(t, QuotaLeaseDemoStatusReclaimed, snapshot.Leases[0].Status)
	require.InDelta(t, 0, snapshot.Leases[0].Remaining(), 1e-9)
}
