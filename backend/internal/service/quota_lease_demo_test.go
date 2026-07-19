package service

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
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

type quotaLeaseDemoSettingRepo struct {
	mu     sync.Mutex
	values map[string]string
}

func newQuotaLeaseDemoSettingRepo() *quotaLeaseDemoSettingRepo {
	return &quotaLeaseDemoSettingRepo{values: make(map[string]string)}
}

func (r *quotaLeaseDemoSettingRepo) Get(ctx context.Context, key string) (*Setting, error) {
	value, err := r.GetValue(ctx, key)
	if err != nil {
		return nil, err
	}
	return &Setting{Key: key, Value: value, UpdatedAt: time.Now().UTC()}, nil
}

func (r *quotaLeaseDemoSettingRepo) GetValue(ctx context.Context, key string) (string, error) {
	_ = ctx
	r.mu.Lock()
	defer r.mu.Unlock()
	value, ok := r.values[key]
	if !ok {
		return "", ErrSettingNotFound
	}
	return value, nil
}

func (r *quotaLeaseDemoSettingRepo) Set(ctx context.Context, key, value string) error {
	_ = ctx
	r.mu.Lock()
	defer r.mu.Unlock()
	r.values[key] = value
	return nil
}

func (r *quotaLeaseDemoSettingRepo) GetMultiple(ctx context.Context, keys []string) (map[string]string, error) {
	_ = ctx
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make(map[string]string, len(keys))
	for _, key := range keys {
		if value, ok := r.values[key]; ok {
			out[key] = value
		}
	}
	return out, nil
}

func (r *quotaLeaseDemoSettingRepo) SetMultiple(ctx context.Context, settings map[string]string) error {
	_ = ctx
	r.mu.Lock()
	defer r.mu.Unlock()
	for key, value := range settings {
		r.values[key] = value
	}
	return nil
}

func (r *quotaLeaseDemoSettingRepo) GetAll(ctx context.Context) (map[string]string, error) {
	_ = ctx
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make(map[string]string, len(r.values))
	for key, value := range r.values {
		out[key] = value
	}
	return out, nil
}

func (r *quotaLeaseDemoSettingRepo) Delete(ctx context.Context, key string) error {
	_ = ctx
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.values, key)
	return nil
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

type quotaLeaseDemoBillingRepo struct {
	applies    []*UsageBillingCommand
	reserves   []*BalanceHoldCommand
	captures   []*BalanceHoldCommand
	releases   []*BalanceHoldCommand
	seen       map[string]struct{}
	applyErr   error
	reserveErr error
	captureErr error
	releaseErr error
}

func (r *quotaLeaseDemoBillingRepo) Apply(_ context.Context, cmd *UsageBillingCommand) (*UsageBillingApplyResult, error) {
	if r.applyErr != nil {
		r.applies = append(r.applies, cmd)
		return nil, r.applyErr
	}
	if r.seen == nil {
		r.seen = make(map[string]struct{})
	}
	if cmd != nil {
		cmd.Normalize()
		if _, ok := r.seen[cmd.RequestID]; ok {
			r.applies = append(r.applies, cmd)
			return &UsageBillingApplyResult{Applied: false}, nil
		}
		r.seen[cmd.RequestID] = struct{}{}
	}
	r.applies = append(r.applies, cmd)
	return &UsageBillingApplyResult{Applied: true}, nil
}

func (r *quotaLeaseDemoBillingRepo) ReserveBalanceHold(_ context.Context, cmd *BalanceHoldCommand) (*BalanceHoldResult, error) {
	if r.reserveErr != nil {
		r.reserves = append(r.reserves, cmd)
		return nil, r.reserveErr
	}
	return r.applyHold(cmd, &r.reserves)
}

func (r *quotaLeaseDemoBillingRepo) CaptureBalanceHold(_ context.Context, cmd *BalanceHoldCommand) (*BalanceHoldResult, error) {
	if r.captureErr != nil {
		r.captures = append(r.captures, cmd)
		return nil, r.captureErr
	}
	return r.applyHold(cmd, &r.captures)
}

func (r *quotaLeaseDemoBillingRepo) ReleaseBalanceHold(_ context.Context, cmd *BalanceHoldCommand) (*BalanceHoldResult, error) {
	if r.releaseErr != nil {
		r.releases = append(r.releases, cmd)
		return nil, r.releaseErr
	}
	return r.applyHold(cmd, &r.releases)
}

func (r *quotaLeaseDemoBillingRepo) ReserveBatchImageBalance(_ context.Context, cmd *BatchImageBalanceHoldCommand) (*BatchImageBalanceHoldResult, error) {
	return &BatchImageBalanceHoldResult{Applied: true}, nil
}

func (r *quotaLeaseDemoBillingRepo) CaptureBatchImageBalance(_ context.Context, cmd *BatchImageBalanceHoldCommand) (*BatchImageBalanceHoldResult, error) {
	return &BatchImageBalanceHoldResult{Applied: true}, nil
}

func (r *quotaLeaseDemoBillingRepo) ReleaseBatchImageBalance(_ context.Context, cmd *BatchImageBalanceHoldCommand) (*BatchImageBalanceHoldResult, error) {
	return &BatchImageBalanceHoldResult{Applied: true}, nil
}

func (r *quotaLeaseDemoBillingRepo) applyHold(cmd *BalanceHoldCommand, calls *[]*BalanceHoldCommand) (*BalanceHoldResult, error) {
	if r.seen == nil {
		r.seen = make(map[string]struct{})
	}
	if cmd != nil {
		cmd.Normalize()
		if _, ok := r.seen[cmd.RequestID]; ok {
			*calls = append(*calls, cmd)
			return &BalanceHoldResult{Applied: false}, nil
		}
		r.seen[cmd.RequestID] = struct{}{}
	}
	*calls = append(*calls, cmd)
	return &BalanceHoldResult{Applied: true}, nil
}

var _ UsageBillingRepository = (*quotaLeaseDemoBillingRepo)(nil)

type quotaLeaseDemoStrictBalanceRejectingBillingRepo struct {
	quotaLeaseDemoBillingRepo
}

func (r *quotaLeaseDemoStrictBalanceRejectingBillingRepo) Apply(_ context.Context, cmd *UsageBillingCommand) (*UsageBillingApplyResult, error) {
	if cmd != nil && cmd.StrictBalance {
		r.applies = append(r.applies, cmd)
		return nil, ErrBalanceHoldInsufficientBalance
	}
	r.applies = append(r.applies, cmd)
	return &UsageBillingApplyResult{Applied: true, BalanceOverdrafted: true}, nil
}

var _ UsageBillingRepository = (*quotaLeaseDemoStrictBalanceRejectingBillingRepo)(nil)

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
		case "/api/v1/node-leases/demo/settings":
			if !control.AuthenticateNode(r.Header.Get("X-Node-ID"), r.Header.Get("X-Node-Secret")) {
				w.WriteHeader(http.StatusUnauthorized)
				_ = json.NewEncoder(w).Encode(map[string]string{"error": "invalid_node_secret"})
				return
			}
			settings, err := control.GetSettings(r.Context())
			require.NoError(t, err)
			require.NoError(t, json.NewEncoder(w).Encode(map[string]any{
				"code":    0,
				"message": "success",
				"data":    settings,
			}))
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

func TestQuotaLeaseDemoSettingsServiceDefaultsUpdatesAndValidates(t *testing.T) {
	repo := newQuotaLeaseDemoSettingRepo()
	settingSvc := NewSettingService(repo, nil)
	ctx := context.Background()

	defaults, err := settingSvc.GetQuotaLeaseDemoSettings(ctx)
	require.NoError(t, err)
	require.InDelta(t, 0.2, defaults.PrefetchLowWatermarkAmount, 1e-12)
	require.Equal(t, 5, defaults.PrefetchAverageWindow)
	require.InDelta(t, 3.0, defaults.PrefetchAverageMultiplier, 1e-12)
	require.Equal(t, 10, defaults.PrefetchDebounceSeconds)

	lowWatermark := 0.45
	window := 8
	multiplier := 2.5
	debounce := 4
	updated, err := settingSvc.SetQuotaLeaseDemoSettings(ctx, &QuotaLeaseDemoSettingsPatch{
		PrefetchLowWatermarkAmount: &lowWatermark,
		PrefetchAverageWindow:      &window,
		PrefetchAverageMultiplier:  &multiplier,
		PrefetchDebounceSeconds:    &debounce,
	})
	require.NoError(t, err)
	require.InDelta(t, lowWatermark, updated.PrefetchLowWatermarkAmount, 1e-12)
	require.Equal(t, window, updated.PrefetchAverageWindow)
	require.InDelta(t, multiplier, updated.PrefetchAverageMultiplier, 1e-12)
	require.Equal(t, debounce, updated.PrefetchDebounceSeconds)

	raw, err := repo.GetValue(ctx, SettingKeyQuotaLeaseDemoSettings)
	require.NoError(t, err)
	var saved QuotaLeaseDemoSettings
	require.NoError(t, json.Unmarshal([]byte(raw), &saved))
	require.InDelta(t, lowWatermark, saved.PrefetchLowWatermarkAmount, 1e-12)
	require.Equal(t, window, saved.PrefetchAverageWindow)

	invalid := -0.1
	_, err = settingSvc.SetQuotaLeaseDemoSettings(ctx, &QuotaLeaseDemoSettingsPatch{
		PrefetchLowWatermarkAmount: &invalid,
	})
	require.Error(t, err)
}

func TestQuotaLeaseDemoRemoteNodeReadsPrefetchSettingsFromControlPlane(t *testing.T) {
	control := newQuotaLeaseDemoTestService()
	settingSvc := NewSettingService(newQuotaLeaseDemoSettingRepo(), nil)
	lowWatermark := 0.35
	window := 2
	multiplier := 4.0
	debounce := 1
	_, err := settingSvc.SetQuotaLeaseDemoSettings(context.Background(), &QuotaLeaseDemoSettingsPatch{
		PrefetchLowWatermarkAmount: &lowWatermark,
		PrefetchAverageWindow:      &window,
		PrefetchAverageMultiplier:  &multiplier,
		PrefetchDebounceSeconds:    &debounce,
	})
	require.NoError(t, err)
	control.SetSettingService(settingSvc)

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

	settings, err := node.GetSettings(context.Background())
	require.NoError(t, err)
	require.InDelta(t, lowWatermark, settings.PrefetchLowWatermarkAmount, 1e-12)
	require.Equal(t, window, settings.PrefetchAverageWindow)
	require.InDelta(t, multiplier, settings.PrefetchAverageMultiplier, 1e-12)
	require.Equal(t, debounce, settings.PrefetchDebounceSeconds)
}

func TestQuotaLeaseDemoRemotePrefetchExpandsActiveLeaseFromControlSettings(t *testing.T) {
	control := newQuotaLeaseDemoTestService()
	settingSvc := NewSettingService(newQuotaLeaseDemoSettingRepo(), nil)
	lowWatermark := 0.2
	window := 0
	multiplier := 0.0
	debounce := 0
	_, err := settingSvc.SetQuotaLeaseDemoSettings(context.Background(), &QuotaLeaseDemoSettingsPatch{
		PrefetchLowWatermarkAmount: &lowWatermark,
		PrefetchAverageWindow:      &window,
		PrefetchAverageMultiplier:  &multiplier,
		PrefetchDebounceSeconds:    &debounce,
	})
	require.NoError(t, err)
	control.SetSettingService(settingSvc)

	server := newQuotaLeaseDemoControlPlaneTestServer(t, control, "control-secret")
	defer server.Close()

	node := NewQuotaLeaseDemoService(&config.Config{
		Gateway: config.GatewayConfig{
			QuotaLeaseDemo: config.GatewayQuotaLeaseDemoConfig{
				Enabled:                true,
				ControlPlaneBaseURL:    server.URL,
				ControlPlaneKey:        "control-secret",
				DefaultGrantAmount:     1,
				LeaseTTLSeconds:        600,
				ReclaimGraceSeconds:    3600,
				PreflightReserveAmount: 0.000001,
			},
		},
	})
	ctx := context.Background()

	lease, err := node.RequestLease(ctx, QuotaLeaseDemoLeaseRequest{
		UserID:   10,
		APIKeyID: 20,
		Amount:   1,
	})
	require.NoError(t, err)
	require.InDelta(t, 1, lease.Granted, 1e-12)
	initialLeaseID := lease.ID

	handled, applied, err := node.ApplyUsageBilling(ctx, &UsageBillingCommand{
		RequestID:   "prefetch-req-1",
		UserID:      10,
		APIKeyID:    20,
		BalanceCost: 0.9,
	})
	require.NoError(t, err)
	require.True(t, handled)
	require.True(t, applied)

	require.Eventually(t, func() bool {
		snapshot := control.Snapshot()
		if len(snapshot.Leases) != 1 {
			return false
		}
		remoteLease := snapshot.Leases[0]
		return remoteLease.ID == initialLeaseID &&
			remoteLease.Granted >= 2.0-1e-9 &&
			remoteLease.Consumed >= 0.9-1e-9 &&
			remoteLease.Remaining() >= 1.1-1e-9
	}, 2*time.Second, 20*time.Millisecond)

	nodeSnapshot := node.Snapshot()
	require.Len(t, nodeSnapshot.Leases, 1)
	require.Equal(t, initialLeaseID, nodeSnapshot.Leases[0].ID)
	require.GreaterOrEqual(t, nodeSnapshot.Leases[0].Granted, 2.0-1e-9)
	require.GreaterOrEqual(t, nodeSnapshot.Leases[0].Consumed, 0.9-1e-9)
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

func TestQuotaLeaseDemoConsumeUsageAllowsNegativeRemaining(t *testing.T) {
	svc := newQuotaLeaseDemoTestService()
	ctx := context.Background()

	lease, err := svc.RequestLease(ctx, QuotaLeaseDemoLeaseRequest{
		UserID:   10,
		APIKeyID: 20,
		Amount:   0.02,
	})
	require.NoError(t, err)

	result, err := svc.ConsumeUsage(ctx, QuotaLeaseDemoUsageEvent{
		EventID:   "event-overdraft-1",
		LeaseID:   lease.ID,
		NodeID:    "node-1",
		UserID:    10,
		APIKeyID:  20,
		RequestID: "req-overdraft-1",
		Amount:    1,
	})
	require.NoError(t, err)
	require.True(t, result.Applied)
	require.InDelta(t, -0.98, result.Lease.Remaining(), 1e-9)
	require.Equal(t, QuotaLeaseDemoStatusActive, result.Lease.Status)
	require.False(t, svc.HasCapacity("node-1", 10, 20, 0.000001))
}

func TestQuotaLeaseDemoUsageBillingChargesBalanceOnConsumption(t *testing.T) {
	svc := newQuotaLeaseDemoTestService()
	billing := &quotaLeaseDemoBillingRepo{}
	svc.SetUsageBillingRepository(billing)
	ctx := context.Background()

	lease, err := svc.RequestLease(ctx, QuotaLeaseDemoLeaseRequest{
		NodeID:   "node-1",
		UserID:   10,
		APIKeyID: 20,
		Amount:   1,
	})
	require.NoError(t, err)
	require.Empty(t, billing.reserves)

	event := QuotaLeaseDemoUsageEvent{
		EventID:   "event-hold-1",
		LeaseID:   lease.ID,
		NodeID:    "node-1",
		UserID:    10,
		APIKeyID:  20,
		RequestID: "req-hold-1",
		Amount:    0.4,
	}
	first, err := svc.ConsumeUsage(ctx, event)
	require.NoError(t, err)
	require.True(t, first.Applied)
	require.Len(t, billing.applies, 1)
	require.Equal(t, quotaLeaseDemoUsageBillingRequestID(event.NodeID, event.APIKeyID, event.RequestID), billing.applies[0].RequestID)
	require.Equal(t, int64(10), billing.applies[0].UserID)
	require.Equal(t, int64(20), billing.applies[0].APIKeyID)
	require.InDelta(t, 0.4, billing.applies[0].BalanceCost, 1e-12)
	require.False(t, billing.applies[0].StrictBalance)
	require.Empty(t, billing.captures)

	duplicate, err := svc.ConsumeUsage(ctx, event)
	require.NoError(t, err)
	require.True(t, duplicate.Duplicate)
	require.Len(t, billing.applies, 1)

	reclaimAt := first.Lease.ExpiresAt.Add(time.Second)
	require.True(t, reclaimAt.Before(first.Lease.ReclaimAt))
	reclaimed := svc.ReclaimExpired(ctx, reclaimAt)
	require.Equal(t, 1, reclaimed.ReclaimedCount)
	require.InDelta(t, 0.6, reclaimed.ReclaimedTotal, 1e-12)
	require.Empty(t, billing.releases)
}

func TestQuotaLeaseDemoUsageBillingAllowsControlPlaneBalanceOverdraft(t *testing.T) {
	svc := newQuotaLeaseDemoTestService()
	billing := &quotaLeaseDemoStrictBalanceRejectingBillingRepo{}
	svc.SetUsageBillingRepository(billing)
	ctx := context.Background()

	lease, err := svc.RequestLease(ctx, QuotaLeaseDemoLeaseRequest{
		NodeID:   "node-1",
		UserID:   10,
		APIKeyID: 20,
		Amount:   0.02,
	})
	require.NoError(t, err)

	result, err := svc.ConsumeUsage(ctx, QuotaLeaseDemoUsageEvent{
		EventID:   "event-control-overdraft-1",
		LeaseID:   lease.ID,
		NodeID:    "node-1",
		UserID:    10,
		APIKeyID:  20,
		RequestID: "req-control-overdraft-1",
		Amount:    0.05,
	})
	require.NoError(t, err)
	require.True(t, result.Applied)
	require.Len(t, billing.applies, 1)
	require.False(t, billing.applies[0].StrictBalance)
	require.InDelta(t, -0.03, result.Lease.Remaining(), 1e-9)
}

func TestQuotaLeaseDemoLeaseUsesIdleExpiryWindow(t *testing.T) {
	svc := newQuotaLeaseDemoTestService()
	ctx := context.Background()
	now := time.Now().UTC()

	lease, err := svc.RequestLease(ctx, QuotaLeaseDemoLeaseRequest{
		NodeID:   "node-1",
		UserID:   10,
		APIKeyID: 20,
		Amount:   1,
	})
	require.NoError(t, err)
	require.WithinDuration(t, now.Add(5*time.Minute), lease.ExpiresAt, 2*time.Second)
	require.WithinDuration(t, lease.ExpiresAt.Add(1*time.Hour), lease.ReclaimAt, 2*time.Second)
}

func TestQuotaLeaseDemoUsageConsumptionRefreshesExpiryWindow(t *testing.T) {
	svc := newQuotaLeaseDemoTestService()
	billing := &quotaLeaseDemoBillingRepo{}
	svc.SetUsageBillingRepository(billing)
	ctx := context.Background()

	lease, err := svc.RequestLease(ctx, QuotaLeaseDemoLeaseRequest{
		NodeID:   "node-1",
		UserID:   10,
		APIKeyID: 20,
		Amount:   1,
	})
	require.NoError(t, err)

	time.Sleep(10 * time.Millisecond)
	before := lease.ExpiresAt
	_, err = svc.ConsumeUsage(ctx, QuotaLeaseDemoUsageEvent{
		EventID:   "event-refresh-1",
		LeaseID:   lease.ID,
		NodeID:    "node-1",
		UserID:    10,
		APIKeyID:  20,
		RequestID: "req-refresh-1",
		Amount:    0.25,
	})
	require.NoError(t, err)

	refreshed := svc.Snapshot().Leases[0]
	require.True(t, refreshed.ExpiresAt.After(before))
	require.WithinDuration(t, time.Now().UTC().Add(5*time.Minute), refreshed.ExpiresAt, 2*time.Second)
	require.WithinDuration(t, refreshed.ExpiresAt.Add(1*time.Hour), refreshed.ReclaimAt, 2*time.Second)
}

func TestQuotaLeaseDemoLeaseTopUpExtendsGrantWithoutBalanceHold(t *testing.T) {
	svc := newQuotaLeaseDemoTestService()
	billing := &quotaLeaseDemoBillingRepo{}
	svc.SetUsageBillingRepository(billing)
	ctx := context.Background()

	lease, err := svc.RequestLease(ctx, QuotaLeaseDemoLeaseRequest{
		NodeID:   "node-1",
		UserID:   10,
		APIKeyID: 20,
		Amount:   0.2,
	})
	require.NoError(t, err)

	toppedUp, err := svc.RequestLease(ctx, QuotaLeaseDemoLeaseRequest{
		NodeID:   "node-1",
		UserID:   10,
		APIKeyID: 20,
		Amount:   1,
	})
	require.NoError(t, err)
	require.Equal(t, lease.ID, toppedUp.ID)
	require.Empty(t, billing.reserves)
	require.InDelta(t, 1, toppedUp.Granted, 1e-12)
}

func TestQuotaLeaseDemoLeaseGrantSkipsBalanceHoldReserve(t *testing.T) {
	svc := newQuotaLeaseDemoTestService()
	billing := &quotaLeaseDemoBillingRepo{reserveErr: ErrBalanceHoldInsufficientBalance}
	svc.SetUsageBillingRepository(billing)

	lease, err := svc.RequestLease(context.Background(), QuotaLeaseDemoLeaseRequest{
		NodeID:   "node-1",
		UserID:   10,
		APIKeyID: 20,
		Amount:   1,
	})
	require.NoError(t, err)
	require.Equal(t, "node-1", lease.NodeID)
	require.Empty(t, billing.reserves)
	require.Len(t, svc.Snapshot().Leases, 1)
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

func TestQuotaLeaseDemoInspectCapacityPrefersActiveLease(t *testing.T) {
	svc := newQuotaLeaseDemoTestService()
	now := time.Now().UTC()
	svc.leases["expired-high"] = &QuotaLeaseDemoLease{
		ID:        "expired-high",
		NodeID:    "node-1",
		UserID:    10,
		APIKeyID:  20,
		Granted:   0.5,
		Consumed:  0.1,
		Status:    QuotaLeaseDemoStatusActive,
		ExpiresAt: now.Add(-time.Minute),
		ReclaimAt: now.Add(time.Hour),
		CreatedAt: now.Add(-10 * time.Minute),
		UpdatedAt: now.Add(-10 * time.Minute),
	}
	svc.leases["active-low"] = &QuotaLeaseDemoLease{
		ID:        "active-low",
		NodeID:    "node-1",
		UserID:    10,
		APIKeyID:  20,
		Granted:   0.03,
		Status:    QuotaLeaseDemoStatusActive,
		ExpiresAt: now.Add(time.Minute),
		ReclaimAt: now.Add(time.Hour),
		CreatedAt: now.Add(-time.Minute),
		UpdatedAt: now.Add(-time.Minute),
	}

	ok, probe := svc.inspectCapacity("node-1", 10, 20, 0.02, now)
	require.True(t, ok)
	require.Equal(t, "active-low", probe.BestLeaseID)
	require.Equal(t, QuotaLeaseDemoStatusActive, probe.BestLeaseStatus)
	require.InDelta(t, 0.03, probe.BestLeaseRemaining, 1e-12)
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

func TestQuotaLeaseDemoRemoteOverdraftBlocksWhenUsageFlushFails(t *testing.T) {
	ctx := context.Background()
	now := time.Now().UTC()
	leaseRequests := 0
	usageBatchCalls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/v1/node-leases/demo/nodes/register":
			require.NoError(t, json.NewEncoder(w).Encode(QuotaLeaseDemoNodeRegistrationResult{
				Node: &QuotaLeaseDemoNode{
					NodeID:       "node-us",
					Secret:       "node-secret",
					Status:       QuotaLeaseDemoNodeStatusOnline,
					RegisteredAt: now,
					UpdatedAt:    now,
				},
				NodeSecret: "node-secret",
			}))
		case "/api/v1/node-leases/demo/leases/request":
			leaseRequests++
			require.NoError(t, json.NewEncoder(w).Encode(map[string]any{
				"lease": &QuotaLeaseDemoLease{
					ID:        "lease-small",
					NodeID:    "node-us",
					UserID:    10,
					APIKeyID:  20,
					Granted:   0.02,
					Status:    QuotaLeaseDemoStatusActive,
					ExpiresAt: now.Add(time.Hour),
					ReclaimAt: now.Add(2 * time.Hour),
					CreatedAt: now,
					UpdatedAt: now,
				},
			}))
		case "/api/v1/node-leases/demo/usage/batch":
			usageBatchCalls++
			var req QuotaLeaseDemoUsageBatchRequest
			require.NoError(t, json.NewDecoder(r.Body).Decode(&req))
			results := make([]QuotaLeaseDemoUsageResult, 0, len(req.Events))
			for _, event := range req.Events {
				results = append(results, QuotaLeaseDemoUsageResult{
					EventID: strings.TrimSpace(event.EventID),
					LeaseID: strings.TrimSpace(event.LeaseID),
					Error:   "insufficient balance",
				})
			}
			require.NoError(t, json.NewEncoder(w).Encode(QuotaLeaseDemoUsageBatchResult{Results: results}))
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
				DefaultGrantAmount:  1,
				LeaseTTLSeconds:     600,
				ReclaimGraceSeconds: 3600,
			},
		},
	})
	node.cacheRemoteLease(&QuotaLeaseDemoLease{
		ID:        "lease-small",
		NodeID:    "node-us",
		UserID:    10,
		APIKeyID:  20,
		Granted:   0.02,
		Status:    QuotaLeaseDemoStatusActive,
		ExpiresAt: now.Add(time.Hour),
		ReclaimAt: now.Add(2 * time.Hour),
		CreatedAt: now,
		UpdatedAt: now,
	})

	handled, applied, err := node.ApplyUsageBilling(ctx, &UsageBillingCommand{
		RequestID:   "req-overdraft-remote",
		UserID:      10,
		APIKeyID:    20,
		BalanceCost: 1,
	})
	require.ErrorIs(t, err, ErrQuotaLeaseDemoNoCapacity)
	require.True(t, handled)
	require.True(t, applied)

	snapshot := node.Snapshot()
	require.Len(t, snapshot.Leases, 1)
	require.InDelta(t, -0.98, snapshot.Leases[0].Remaining(), 1e-9)
	require.Len(t, node.pendingUsageEvents(), 1)

	require.False(t, node.CanAuthorizeRequest(ctx, &APIKey{
		ID:     20,
		UserID: 10,
		User: &User{
			ID:      10,
			Status:  StatusActive,
			Balance: 0.02,
		},
	}, nil))
	require.GreaterOrEqual(t, usageBatchCalls, 1)
	require.GreaterOrEqual(t, leaseRequests, 1)
}

func TestQuotaLeaseDemoRemoteOverdraftSettlementSucceedsWhenRenewalDenied(t *testing.T) {
	ctx := context.Background()
	now := time.Now().UTC()
	leaseRequests := 0
	usageBatchCalls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/v1/node-leases/demo/nodes/register":
			require.NoError(t, json.NewEncoder(w).Encode(QuotaLeaseDemoNodeRegistrationResult{
				Node: &QuotaLeaseDemoNode{
					NodeID:       "node-us",
					Secret:       "node-secret",
					Status:       QuotaLeaseDemoNodeStatusOnline,
					RegisteredAt: now,
					UpdatedAt:    now,
				},
				NodeSecret: "node-secret",
			}))
		case "/api/v1/node-leases/demo/leases/request":
			leaseRequests++
			w.WriteHeader(http.StatusForbidden)
			require.NoError(t, json.NewEncoder(w).Encode(map[string]string{
				"error": "no_capacity",
			}))
		case "/api/v1/node-leases/demo/usage/batch":
			usageBatchCalls++
			var req QuotaLeaseDemoUsageBatchRequest
			require.NoError(t, json.NewDecoder(r.Body).Decode(&req))
			results := make([]QuotaLeaseDemoUsageResult, 0, len(req.Events))
			for _, event := range req.Events {
				results = append(results, QuotaLeaseDemoUsageResult{
					EventID: strings.TrimSpace(event.EventID),
					LeaseID: strings.TrimSpace(event.LeaseID),
					Applied: true,
				})
			}
			require.NoError(t, json.NewEncoder(w).Encode(QuotaLeaseDemoUsageBatchResult{Results: results}))
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
				DefaultGrantAmount:  1,
				LeaseTTLSeconds:     600,
				ReclaimGraceSeconds: 3600,
			},
		},
	})
	node.cacheRemoteLease(&QuotaLeaseDemoLease{
		ID:        "lease-small",
		NodeID:    "node-us",
		UserID:    10,
		APIKeyID:  20,
		Granted:   0.02,
		Status:    QuotaLeaseDemoStatusActive,
		ExpiresAt: now.Add(time.Hour),
		ReclaimAt: now.Add(2 * time.Hour),
		CreatedAt: now,
		UpdatedAt: now,
	})

	handled, applied, err := node.ApplyUsageBilling(ctx, &UsageBillingCommand{
		RequestID:   "req-overdraft-renew-denied",
		UserID:      10,
		APIKeyID:    20,
		BalanceCost: 0.05,
	})
	require.NoError(t, err)
	require.True(t, handled)
	require.True(t, applied)
	require.Equal(t, 0, len(node.pendingUsageEvents()))
	require.GreaterOrEqual(t, usageBatchCalls, 1)
	require.GreaterOrEqual(t, leaseRequests, 2)

	snapshot := node.Snapshot()
	require.Len(t, snapshot.Leases, 1)
	require.InDelta(t, -0.03, snapshot.Leases[0].Remaining(), 1e-9)
	require.False(t, node.CanAuthorizeRequest(ctx, &APIKey{
		ID:     20,
		UserID: 10,
		User: &User{
			ID:      10,
			Status:  StatusActive,
			Balance: 0.02,
		},
	}, nil))
}

func TestQuotaLeaseDemoNodeWorkerReportsRuntimeHeartbeat(t *testing.T) {
	control := newQuotaLeaseDemoTestService()
	server := newQuotaLeaseDemoControlPlaneTestServer(t, control, "control-secret")
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

	lease, err := node.RequestLease(ctx, QuotaLeaseDemoLeaseRequest{
		UserID:   10,
		APIKeyID: 20,
		Amount:   1,
	})
	require.NoError(t, err)
	require.Equal(t, "node-us", lease.NodeID)

	handled, applied, err := node.ApplyUsageBilling(ctx, &UsageBillingCommand{
		RequestID:   "remote-heartbeat-req-1",
		UserID:      10,
		APIKeyID:    20,
		BalanceCost: 0.25,
	})
	require.NoError(t, err)
	require.True(t, handled)
	require.True(t, applied)

	worker := NewQuotaLeaseDemoNodeWorker(node, NewQuotaLeaseDemoPayloadAccountTaskExecutor(), time.Hour)
	require.NoError(t, worker.RunOnce(ctx))

	nodes := control.ListNodes()
	require.Len(t, nodes, 1)
	require.Equal(t, "node-us", nodes[0].NodeID)
	require.NotNil(t, nodes[0].LastHeartbeatAt)
	require.InDelta(t, 0.75, nodes[0].LeaseRemaining, 1e-9)
	require.Equal(t, 1.0, nodes[0].Metrics["active_leases"])

	controlSnapshot := control.Snapshot()
	require.Len(t, controlSnapshot.Leases, 1)
	require.InDelta(t, 0.25, controlSnapshot.Leases[0].Consumed, 1e-9)
	require.InDelta(t, 0.75, controlSnapshot.Nodes[0].LeaseRemaining, 1e-9)
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

func TestQuotaLeaseDemoUsageLogSnapshotPreservesNodeID(t *testing.T) {
	createdAt := time.Date(2026, 7, 18, 12, 45, 0, 0, time.UTC)
	snapshot := NewQuotaLeaseDemoUsageLogSnapshot("", &UsageLog{
		NodeID:    " node-us ",
		UserID:    10,
		APIKeyID:  20,
		AccountID: 30,
		RequestID: "usage-log-node-id",
		Model:     "gpt-5",
		CreatedAt: createdAt,
	})

	require.Equal(t, "node-us", snapshot.NodeID)
	log := snapshot.ToUsageLog()
	require.Equal(t, "node-us", log.NodeID)
	require.Equal(t, "usage-log-node-id", log.RequestID)
}

func TestQuotaLeaseDemoRemoteNodeAuthorizesClientKeyViaControlPlane(t *testing.T) {
	ctx := context.Background()
	control := newQuotaLeaseDemoTestService()
	authCalls := 0
	var authAmount float64
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
			authAmount = req.Amount
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
	require.InDelta(t, node.DefaultGrantAmount(), authAmount, 1e-12)
	require.InDelta(t, node.DefaultGrantAmount(), result.Lease.Granted, 1e-12)
	require.True(t, node.hasCapacity("node-us", 10, 20, node.DefaultGrantAmount(), time.Now().UTC()))

	cached, err := node.AuthorizeClientKeyViaControlPlane(ctx, "sk-live-user", 0)
	require.NoError(t, err)
	require.Equal(t, result.Lease.ID, cached.Lease.ID)
	require.Equal(t, 1, authCalls)
}

func TestQuotaLeaseDemoRemoteClientAuthCapsCapacityToSnapshotBalance(t *testing.T) {
	ctx := context.Background()
	control := newQuotaLeaseDemoTestService()
	authCalls := 0
	authAmount := 0.0
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
			authAmount = req.Amount
			grantAmount := req.Amount
			if grantAmount > 0.5 {
				grantAmount = 0.5
			}
			lease, err := control.RequestLease(r.Context(), QuotaLeaseDemoLeaseRequest{
				NodeID:   req.NodeID,
				UserID:   10,
				APIKeyID: 20,
				Amount:   grantAmount,
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
						Balance:     0.5,
						Concurrency: 2,
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
				Enabled:             true,
				NodeID:              "node-us",
				ControlPlaneBaseURL: server.URL,
				ControlPlaneKey:     "control-secret",
				DefaultGrantAmount:  1,
				LeaseTTLSeconds:     600,
				ReclaimGraceSeconds: 3600,
			},
		},
	})

	result, err := node.AuthorizeClientKeyViaControlPlane(ctx, "sk-live-user", 0)
	require.NoError(t, err)
	require.NotNil(t, result.Lease)
	require.InDelta(t, node.DefaultGrantAmount(), authAmount, 1e-12)
	require.InDelta(t, 0.5, result.Lease.Granted, 1e-12)
	require.True(t, node.hasCapacity("node-us", 10, 20, 0.5, time.Now().UTC()))
	require.False(t, node.hasCapacity("node-us", 10, 20, 1, time.Now().UTC()))

	cached, err := node.AuthorizeClientKeyViaControlPlane(ctx, "sk-live-user", 0)
	require.NoError(t, err)
	require.Equal(t, result.Lease.ID, cached.Lease.ID)
	require.Equal(t, 1, authCalls)
}

func TestQuotaLeaseDemoRemoteClientAuthFlushesPendingUsageBeforeCapacityRequest(t *testing.T) {
	ctx := context.Background()
	control := newQuotaLeaseDemoTestService()
	authCalls := 0
	leaseCalls := 0
	usageBatchCalls := 0
	groupID := int64(30)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/v1/node-leases/demo/nodes/register":
			var req QuotaLeaseDemoNodeRegistrationRequest
			require.NoError(t, json.NewDecoder(r.Body).Decode(&req))
			result, err := control.RegisterNode(r.Context(), req)
			require.NoError(t, err)
			require.NoError(t, json.NewEncoder(w).Encode(result))
		case "/api/v1/node-leases/demo/auth/client-key":
			authCalls++
			var req QuotaLeaseDemoClientAuthRequest
			require.NoError(t, json.NewDecoder(r.Body).Decode(&req))
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
						Balance:     1,
						Concurrency: 2,
					},
				},
				Lease:     lease,
				ExpiresAt: time.Now().UTC().Add(30 * time.Second),
			}))
		case "/api/v1/node-leases/demo/leases/request":
			leaseCalls++
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
			usageBatchCalls++
			if !control.AuthenticateNode(r.Header.Get("X-Node-ID"), r.Header.Get("X-Node-Secret")) {
				w.WriteHeader(http.StatusUnauthorized)
				_ = json.NewEncoder(w).Encode(map[string]string{"error": "invalid_node_secret"})
				return
			}
			var req QuotaLeaseDemoUsageBatchRequest
			require.NoError(t, json.NewDecoder(r.Body).Decode(&req))
			require.NoError(t, json.NewEncoder(w).Encode(control.PostUsageBatch(r.Context(), req)))
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
				PreflightReserveAmount: 0.7,
			},
		},
	})

	result, err := node.AuthorizeClientKeyViaControlPlane(ctx, "sk-live-user", 0)
	require.NoError(t, err)
	require.NotNil(t, result.Lease)
	require.True(t, node.hasCapacity("node-us", 10, 20, 1, time.Now().UTC()))

	usageEvent := QuotaLeaseDemoUsageEvent{
		EventID:   "evt-drain-lease",
		LeaseID:   result.Lease.ID,
		NodeID:    "node-us",
		UserID:    10,
		APIKeyID:  20,
		RequestID: "req-drain-lease",
		Amount:    0.4,
		EventType: QuotaLeaseDemoEventUsagePosted,
		CreatedAt: time.Now().UTC(),
	}
	_, err = node.consumeUsageLocal(ctx, usageEvent)
	require.NoError(t, err)
	node.enqueuePendingUsageEvent(usageEvent)
	require.True(t, node.hasCapacity("node-us", 10, 20, 0.6, time.Now().UTC()))
	require.False(t, node.hasCapacity("node-us", 10, 20, 1, time.Now().UTC()))

	cached, err := node.AuthorizeClientKeyViaControlPlane(ctx, "sk-live-user", 0)
	require.NoError(t, err)
	require.Equal(t, result.Lease.ID, cached.Lease.ID)
	require.Equal(t, 1, authCalls)
	require.Equal(t, 1, leaseCalls)
	require.Equal(t, 1, usageBatchCalls)
	require.True(t, node.hasCapacity("node-us", 10, 20, 1, time.Now().UTC()))

	controlLease := control.Snapshot().Leases[0]
	require.InDelta(t, 1.4, controlLease.Granted, 1e-12)
	require.InDelta(t, 0.4, controlLease.Consumed, 1e-12)
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
		User: &User{ID: 10, Balance: 0.5},
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

func TestQuotaLeaseDemoRemotePreflightCapsToUserBalance(t *testing.T) {
	control := NewQuotaLeaseDemoService(&config.Config{
		Gateway: config.GatewayConfig{
			QuotaLeaseDemo: config.GatewayQuotaLeaseDemoConfig{
				Enabled:                true,
				NodeID:                 "control-node",
				DefaultGrantAmount:     1,
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
				DefaultGrantAmount:     1,
				LeaseTTLSeconds:        600,
				ReclaimGraceSeconds:    3600,
				PreflightReserveAmount: 0.002,
			},
		},
	})
	node.SetSettingService(NewSettingService(newQuotaLeaseDemoSettingRepo(), &config.Config{
		Gateway: config.GatewayConfig{
			QuotaLeaseDemo: config.GatewayQuotaLeaseDemoConfig{
				Enabled:                true,
				NodeID:                 "configured-node-lease",
				ControlPlaneBaseURL:    server.URL,
				ControlPlaneKey:        "control-secret",
				DefaultGrantAmount:     1,
				LeaseTTLSeconds:        600,
				ReclaimGraceSeconds:    3600,
				PreflightReserveAmount: 0.002,
			},
		},
	}))
	node.remoteNodeID = registered.Node.NodeID
	node.remoteNodeSecret = registered.NodeSecret

	lease, err := control.RequestLease(ctx, QuotaLeaseDemoLeaseRequest{
		NodeID:   registered.Node.NodeID,
		UserID:   10,
		APIKeyID: 20,
		Amount:   0.00144,
	})
	require.NoError(t, err)
	require.Equal(t, registered.Node.NodeID, lease.NodeID)
	node.cacheRemoteLease(lease)

	require.True(t, node.CanAuthorizeRequest(ctx, &APIKey{
		ID:   20,
		User: &User{ID: 10, Balance: 0.5},
	}, nil))

	snapshot := control.Snapshot()
	require.Len(t, snapshot.Leases, 1)
	require.InDelta(t, 0.5, snapshot.Leases[0].Granted, 1e-12)
	require.InDelta(t, 0.5, snapshot.Leases[0].Remaining(), 1e-12)
}

func TestQuotaLeaseDemoRemotePreflightAcceptsPartialLeaseGrant(t *testing.T) {
	ctx := context.Background()
	now := time.Now().UTC()
	leaseRequests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/v1/node-leases/demo/nodes/register":
			require.NoError(t, json.NewEncoder(w).Encode(QuotaLeaseDemoNodeRegistrationResult{
				Node: &QuotaLeaseDemoNode{
					NodeID:       "node-us",
					Secret:       "node-secret",
					Status:       QuotaLeaseDemoNodeStatusOnline,
					RegisteredAt: now,
					UpdatedAt:    now,
				},
				NodeSecret: "node-secret",
			}))
		case "/api/v1/node-leases/demo/leases/request":
			leaseRequests++
			var req QuotaLeaseDemoLeaseRequest
			require.NoError(t, json.NewDecoder(r.Body).Decode(&req))
			require.InDelta(t, 1, req.Amount, 1e-12)
			require.NoError(t, json.NewEncoder(w).Encode(map[string]any{
				"lease": &QuotaLeaseDemoLease{
					ID:        "lease-partial",
					NodeID:    "node-us",
					UserID:    req.UserID,
					APIKeyID:  req.APIKeyID,
					Granted:   0.5,
					Status:    QuotaLeaseDemoStatusActive,
					ExpiresAt: now.Add(time.Hour),
					ReclaimAt: now.Add(2 * time.Hour),
					CreatedAt: now,
					UpdatedAt: now,
				},
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

	require.True(t, node.CanAuthorizeRequest(ctx, &APIKey{
		ID:   20,
		User: &User{ID: 10, Balance: 1},
	}, nil))
	require.Equal(t, 1, leaseRequests)
	require.True(t, node.hasCapacity("node-us", 10, 20, 0.5, time.Now().UTC()))
	require.False(t, node.hasCapacity("node-us", 10, 20, 1, time.Now().UTC()))
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

	result := svc.ReclaimExpired(ctx, lease.ExpiresAt.Add(time.Second))
	require.Equal(t, 1, result.ExpiredCount)
	require.Equal(t, 1, result.ReclaimedCount)
	require.InDelta(t, 1, result.ReclaimedTotal, 1e-9)

	snapshot := svc.Snapshot()
	require.Equal(t, QuotaLeaseDemoStatusReclaimed, snapshot.Leases[0].Status)
	require.InDelta(t, 0, snapshot.Leases[0].Remaining(), 1e-9)
}

func TestQuotaLeaseDemoReclaimWorkerMarksExpiredUnusedLease(t *testing.T) {
	svc := newQuotaLeaseDemoTestService()
	billing := &quotaLeaseDemoBillingRepo{}
	svc.SetUsageBillingRepository(billing)
	ctx := context.Background()

	lease, err := svc.RequestLease(ctx, QuotaLeaseDemoLeaseRequest{
		NodeID:   "node-1",
		UserID:   10,
		APIKeyID: 20,
		Amount:   1,
	})
	require.NoError(t, err)

	svc.mu.Lock()
	internalLease := svc.leases[lease.ID]
	internalLease.ExpiresAt = time.Now().UTC().Add(-time.Second)
	internalLease.ReclaimAt = time.Now().UTC().Add(time.Hour)
	svc.mu.Unlock()

	worker := NewQuotaLeaseDemoReclaimWorker(svc, time.Hour)
	require.NoError(t, worker.RunOnce(ctx))

	require.Empty(t, billing.releases)

	snapshot := svc.Snapshot()
	require.Equal(t, QuotaLeaseDemoStatusReclaimed, snapshot.Leases[0].Status)
	require.InDelta(t, 0, snapshot.Leases[0].Remaining(), 1e-9)
}
