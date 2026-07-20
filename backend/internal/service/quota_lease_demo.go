package service

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"math"
	"os"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/ctxkey"
	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/google/uuid"
	"golang.org/x/sync/singleflight"
)

const (
	QuotaLeaseDemoStatusActive    = "active"
	QuotaLeaseDemoStatusExpired   = "expired"
	QuotaLeaseDemoStatusReclaimed = "reclaimed"
	QuotaLeaseDemoStatusClosed    = "closed"

	QuotaLeaseDemoEventLeaseGranted = "lease_granted"
	QuotaLeaseDemoEventUsagePosted  = "usage_posted"
	QuotaLeaseDemoEventReclaimed    = "lease_reclaimed"

	QuotaLeaseDemoNodeStatusOnline   = "online"
	QuotaLeaseDemoNodeStatusOffline  = "offline"
	QuotaLeaseDemoNodeStatusDisabled = "disabled"

	quotaLeaseDemoIdleLeaseTTL          = 5 * time.Minute
	quotaLeaseDemoReclaimWorkerInterval = 10 * time.Second
	quotaLeaseDemoRecordRetention       = 7 * 24 * time.Hour
	quotaLeaseDemoCleanupBatchSize      = 1000
)

var (
	ErrQuotaLeaseDemoDisabled     = errors.New("quota lease demo disabled")
	ErrQuotaLeaseDemoInvalidInput = errors.New("quota lease demo invalid input")
	ErrQuotaLeaseDemoConflict     = errors.New("quota lease demo event conflict")
	ErrQuotaLeaseDemoNodeNotFound = errors.New("quota lease demo node not found")
	ErrQuotaLeaseDemoNoCapacity   = infraerrors.Forbidden("QUOTA_LEASE_DEMO_NO_CAPACITY", "No local quota lease capacity available")
)

type quotaLeaseDemoGlobalState struct {
	mu  sync.Mutex
	svc *QuotaLeaseDemoService
}

var globalQuotaLeaseDemo quotaLeaseDemoGlobalState

// GetQuotaLeaseDemoService returns the process-local demo service shared by
// handlers and billing code.
func GetQuotaLeaseDemoService(cfg *config.Config) *QuotaLeaseDemoService {
	globalQuotaLeaseDemo.mu.Lock()
	defer globalQuotaLeaseDemo.mu.Unlock()

	if globalQuotaLeaseDemo.svc == nil {
		globalQuotaLeaseDemo.svc = NewQuotaLeaseDemoService(cfg)
		return globalQuotaLeaseDemo.svc
	}
	globalQuotaLeaseDemo.svc.SetConfig(cfg)
	return globalQuotaLeaseDemo.svc
}

func QuotaLeaseDemoEnabled(cfg *config.Config) bool {
	return cfg != nil && cfg.Gateway.QuotaLeaseDemo.Enabled
}

type QuotaLeaseDemoService struct {
	mu                       sync.Mutex
	cfgMu                    sync.RWMutex
	settingsMu               sync.RWMutex
	remoteMu                 sync.Mutex
	cfg                      *config.Config
	settingService           *SettingService
	billingRepo              UsageBillingRepository
	persistenceStore         QuotaLeaseDemoPersistenceStore
	runtimeSettings          *QuotaLeaseDemoSettings
	runtimeSettingsExpiresAt time.Time
	leases                   map[string]*QuotaLeaseDemoLease
	leaseIndex               map[string]map[string]struct{}
	events                   map[string]*QuotaLeaseDemoLedgerEvent
	nodes                    map[string]*QuotaLeaseDemoNode
	pendingEvents            map[string]QuotaLeaseDemoUsageEvent
	pendingUsageLogs         map[string]QuotaLeaseDemoUsageLogSnapshot
	pendingOpsErrorLogs      map[string]QuotaLeaseDemoOpsErrorLogSnapshot
	prefetchState            map[string]*quotaLeaseDemoPrefetchState
	clientAuthCache          map[string]*quotaLeaseDemoClientAuthCacheEntry
	leaseRequestGroup        singleflight.Group
	accountTasks             map[string]*QuotaLeaseDemoAccountLoginTask
	usageProbeTasks          map[string]*QuotaLeaseDemoUsageProbeTask
	assignedAccounts         map[int64]*QuotaLeaseDemoAssignedAccount
	registrationURLs         map[string]*QuotaLeaseDemoNodeRegistrationURL
	mirrorStore              QuotaLeaseDemoMirrorStore
	channelService           *ChannelService
	mirrorVersionStates      map[string]*quotaLeaseDemoMirrorVersionState
	reconcileWatermarks      map[string]string
	syncStartedAt            time.Time
	syncSuccessAt            time.Time
	syncFailedAt             time.Time
	syncError                string
	syncMode                 string
	syncedGroupCount         int
	syncedChannelCount       int
	syncedProxyCount         int
	syncedAccountCount       int
	mirrorVersion            int64
	remoteNodeID             string
	remoteNodeSecret         string
	remoteControlURL         string
	mirrorReady              bool
	mirrorSyncedAt           time.Time
}

func NewQuotaLeaseDemoService(cfg *config.Config) *QuotaLeaseDemoService {
	return &QuotaLeaseDemoService{
		cfg:                 cfg,
		leases:              make(map[string]*QuotaLeaseDemoLease),
		leaseIndex:          make(map[string]map[string]struct{}),
		events:              make(map[string]*QuotaLeaseDemoLedgerEvent),
		nodes:               make(map[string]*QuotaLeaseDemoNode),
		pendingEvents:       make(map[string]QuotaLeaseDemoUsageEvent),
		pendingUsageLogs:    make(map[string]QuotaLeaseDemoUsageLogSnapshot),
		pendingOpsErrorLogs: make(map[string]QuotaLeaseDemoOpsErrorLogSnapshot),
		prefetchState:       make(map[string]*quotaLeaseDemoPrefetchState),
		clientAuthCache:     make(map[string]*quotaLeaseDemoClientAuthCacheEntry),
		accountTasks:        make(map[string]*QuotaLeaseDemoAccountLoginTask),
		usageProbeTasks:     make(map[string]*QuotaLeaseDemoUsageProbeTask),
		assignedAccounts:    make(map[int64]*QuotaLeaseDemoAssignedAccount),
		registrationURLs:    make(map[string]*QuotaLeaseDemoNodeRegistrationURL),
		mirrorVersionStates: make(map[string]*quotaLeaseDemoMirrorVersionState),
		reconcileWatermarks: make(map[string]string),
	}
}

func (s *QuotaLeaseDemoService) SetConfig(cfg *config.Config) {
	if s == nil {
		return
	}
	s.cfgMu.Lock()
	s.cfg = cfg
	s.cfgMu.Unlock()
}

func (s *QuotaLeaseDemoService) SetUsageBillingRepository(repo UsageBillingRepository) {
	if s == nil {
		return
	}
	s.cfgMu.Lock()
	s.billingRepo = repo
	s.cfgMu.Unlock()
}

func (s *QuotaLeaseDemoService) usageBillingRepository() UsageBillingRepository {
	if s == nil {
		return nil
	}
	s.cfgMu.RLock()
	defer s.cfgMu.RUnlock()
	return s.billingRepo
}

func (s *QuotaLeaseDemoService) cfgSnapshot() config.GatewayQuotaLeaseDemoConfig {
	if s == nil {
		return config.GatewayQuotaLeaseDemoConfig{}
	}
	s.cfgMu.RLock()
	defer s.cfgMu.RUnlock()
	if s.cfg == nil {
		return config.GatewayQuotaLeaseDemoConfig{}
	}
	return s.cfg.Gateway.QuotaLeaseDemo
}

func (s *QuotaLeaseDemoService) Enabled() bool {
	return s.cfgSnapshot().Enabled
}

func (s *QuotaLeaseDemoService) NodeSecret() string {
	return strings.TrimSpace(s.cfgSnapshot().NodeSecret)
}

func (s *QuotaLeaseDemoService) RegistrationURL() string {
	return strings.TrimSpace(s.cfgSnapshot().RegistrationURL)
}

func (s *QuotaLeaseDemoService) ControlPlaneBaseURL() string {
	if s != nil {
		s.remoteMu.Lock()
		remoteControlURL := strings.TrimSpace(s.remoteControlURL)
		s.remoteMu.Unlock()
		if remoteControlURL != "" {
			return remoteControlURL
		}
	}
	cfg := s.cfgSnapshot()
	if baseURL := strings.TrimSpace(cfg.ControlPlaneBaseURL); baseURL != "" {
		return baseURL
	}
	if registrationURL := strings.TrimSpace(cfg.RegistrationURL); registrationURL != "" {
		_, _, controlBaseURL, err := quotaLeaseDemoParseRegistrationURL(registrationURL)
		if err == nil {
			return strings.TrimSpace(controlBaseURL)
		}
	}
	return ""
}

func (s *QuotaLeaseDemoService) ControlPlaneKey() string {
	return strings.TrimSpace(s.cfgSnapshot().ControlPlaneKey)
}

func (s *QuotaLeaseDemoService) NodeID() string {
	cfg := s.cfgSnapshot()
	nodeID := strings.TrimSpace(cfg.NodeID)
	if nodeID != "" {
		return nodeID
	}
	if host, err := os.Hostname(); err == nil && strings.TrimSpace(host) != "" {
		return strings.TrimSpace(host)
	}
	return "gateway-demo"
}

func (s *QuotaLeaseDemoService) PreflightReserveAmount() float64 {
	cfg := s.cfgSnapshot()
	reserve := cfg.PreflightReserveAmount
	if reserve <= 0 {
		return 0.000001
	}
	return reserve
}

func (s *QuotaLeaseDemoService) DefaultGrantAmount() float64 {
	cfg := s.cfgSnapshot()
	amount := cfg.DefaultGrantAmount
	if amount <= 0 {
		return 1
	}
	return amount
}

type QuotaLeaseDemoNodeRegistrationRequest struct {
	NodeID            string            `json:"node_id"`
	NodeSecret        string            `json:"node_secret,omitempty"`
	Region            string            `json:"region"`
	BaseURL           string            `json:"base_url"`
	PublicKey         string            `json:"public_key"`
	Metadata          map[string]string `json:"metadata"`
	RegistrationToken string            `json:"registration_token,omitempty"`
}

type QuotaLeaseDemoNodeRegistrationResult struct {
	Node       *QuotaLeaseDemoNode `json:"node"`
	NodeSecret string              `json:"node_secret"`
}

type QuotaLeaseDemoNodeRegistrationURLRequest struct {
	NodeID     string            `json:"node_id"`
	Region     string            `json:"region"`
	BaseURL    string            `json:"base_url"`
	PublicKey  string            `json:"public_key"`
	Metadata   map[string]string `json:"metadata"`
	TTLSeconds int               `json:"ttl_seconds"`
}

type QuotaLeaseDemoNodeRegistrationURL struct {
	Token           string                                `json:"-"`
	RegistrationURL string                                `json:"registration_url"`
	NodeID          string                                `json:"node_id,omitempty"`
	ExpiresAt       time.Time                             `json:"expires_at"`
	Request         QuotaLeaseDemoNodeRegistrationRequest `json:"-"`
	CreatedAt       time.Time                             `json:"created_at"`
}

type QuotaLeaseDemoNodeHeartbeatRequest struct {
	NodeID           string                        `json:"node_id"`
	InflightRequests int                           `json:"inflight_requests"`
	LeaseRemaining   float64                       `json:"lease_remaining"`
	Metrics          map[string]float64            `json:"metrics"`
	SyncStatus       *QuotaLeaseDemoNodeSyncStatus `json:"sync_status,omitempty"`
	Status           string                        `json:"status"`
}

type QuotaLeaseDemoNodeUpdateRequest struct {
	Region    *string           `json:"region"`
	BaseURL   *string           `json:"base_url"`
	PublicKey *string           `json:"public_key"`
	Metadata  map[string]string `json:"metadata"`
	Status    *string           `json:"status"`
}

type QuotaLeaseDemoNode struct {
	NodeID           string                        `json:"node_id"`
	Secret           string                        `json:"-"`
	Region           string                        `json:"region,omitempty"`
	BaseURL          string                        `json:"base_url,omitempty"`
	PublicKey        string                        `json:"public_key,omitempty"`
	Metadata         map[string]string             `json:"metadata,omitempty"`
	Status           string                        `json:"status"`
	InflightRequests int                           `json:"inflight_requests"`
	LeaseRemaining   float64                       `json:"lease_remaining"`
	Metrics          map[string]float64            `json:"metrics,omitempty"`
	SyncStatus       *QuotaLeaseDemoNodeSyncStatus `json:"sync_status,omitempty"`
	RegisteredAt     time.Time                     `json:"registered_at"`
	LastHeartbeatAt  *time.Time                    `json:"last_heartbeat_at,omitempty"`
	UpdatedAt        time.Time                     `json:"updated_at"`
}

type QuotaLeaseDemoNodeSyncStatus struct {
	MirrorReady         bool       `json:"mirror_ready"`
	MirrorSyncedAt      *time.Time `json:"mirror_synced_at,omitempty"`
	LastSyncStartedAt   *time.Time `json:"last_sync_started_at,omitempty"`
	LastSyncSuccessAt   *time.Time `json:"last_sync_success_at,omitempty"`
	LastSyncFailedAt    *time.Time `json:"last_sync_failed_at,omitempty"`
	LastSyncError       string     `json:"last_sync_error,omitempty"`
	LastSyncMode        string     `json:"last_sync_mode,omitempty"`
	MirrorVersion       int64      `json:"mirror_version"`
	SyncedGroupCount    int        `json:"synced_group_count"`
	SyncedChannelCount  int        `json:"synced_channel_count"`
	SyncedProxyCount    int        `json:"synced_proxy_count"`
	SyncedAccountCount  int        `json:"synced_account_count"`
	PendingUsageEvents  int        `json:"pending_usage_events"`
	PendingUsageLogs    int        `json:"pending_usage_logs"`
	PendingOpsErrorLogs int        `json:"pending_ops_error_logs"`
}

func (s *QuotaLeaseDemoService) RegisterNode(ctx context.Context, req QuotaLeaseDemoNodeRegistrationRequest) (*QuotaLeaseDemoNodeRegistrationResult, error) {
	if s.remoteMode() {
		return s.registerRemoteNode(ctx, req)
	}
	return s.registerNodeLocal(ctx, req)
}

func (s *QuotaLeaseDemoService) CreateNodeRegistrationURL(ctx context.Context, req QuotaLeaseDemoNodeRegistrationURLRequest, externalBaseURL string) (*QuotaLeaseDemoNodeRegistrationURL, error) {
	if s == nil || !s.Enabled() {
		return nil, ErrQuotaLeaseDemoDisabled
	}
	externalBaseURL = strings.TrimSpace(externalBaseURL)
	if externalBaseURL == "" {
		return nil, fmt.Errorf("%w: external control plane URL is required", ErrQuotaLeaseDemoInvalidInput)
	}
	token, err := generateQuotaLeaseDemoRegistrationToken()
	if err != nil {
		return nil, err
	}
	now := time.Now().UTC()
	ttl := time.Duration(req.TTLSeconds) * time.Second
	if ttl <= 0 {
		ttl = 15 * time.Minute
	}
	if ttl > 24*time.Hour {
		ttl = 24 * time.Hour
	}
	expiresAt := now.Add(ttl)
	registrationURL, err := quotaLeaseDemoBuildRegistrationURL(externalBaseURL, token)
	if err != nil {
		return nil, err
	}
	nodeReq := QuotaLeaseDemoNodeRegistrationRequest{
		NodeID:    strings.TrimSpace(req.NodeID),
		Region:    strings.TrimSpace(req.Region),
		BaseURL:   strings.TrimSpace(req.BaseURL),
		PublicKey: strings.TrimSpace(req.PublicKey),
		Metadata:  cloneQuotaLeaseDemoStringMap(req.Metadata),
	}
	item := &QuotaLeaseDemoNodeRegistrationURL{
		Token:           token,
		RegistrationURL: registrationURL,
		NodeID:          nodeReq.NodeID,
		ExpiresAt:       expiresAt,
		Request:         nodeReq,
		CreatedAt:       now,
	}

	s.mu.Lock()
	if s.registrationURLs == nil {
		s.registrationURLs = make(map[string]*QuotaLeaseDemoNodeRegistrationURL)
	}
	s.cleanupExpiredRegistrationURLsLocked(now)
	s.registrationURLs[token] = item
	s.mu.Unlock()
	_ = ctx
	return cloneQuotaLeaseDemoNodeRegistrationURL(item), nil
}

func (s *QuotaLeaseDemoService) registerNodeLocal(ctx context.Context, req QuotaLeaseDemoNodeRegistrationRequest) (*QuotaLeaseDemoNodeRegistrationResult, error) {
	if s == nil || !s.Enabled() {
		return nil, ErrQuotaLeaseDemoDisabled
	}
	if token := strings.TrimSpace(req.RegistrationToken); token != "" {
		tokenReq, err := s.resolveNodeRegistrationURL(token, time.Now().UTC())
		if err != nil {
			return nil, err
		}
		if nodeSecret := strings.TrimSpace(req.NodeSecret); nodeSecret != "" {
			tokenReq.NodeSecret = nodeSecret
		}
		if publicKey := strings.TrimSpace(req.PublicKey); publicKey != "" {
			tokenReq.PublicKey = publicKey
		}
		if baseURL := strings.TrimSpace(req.BaseURL); baseURL != "" {
			tokenReq.BaseURL = baseURL
		}
		if len(req.Metadata) > 0 {
			metadata := cloneQuotaLeaseDemoStringMap(tokenReq.Metadata)
			if metadata == nil {
				metadata = map[string]string{}
			}
			for key, value := range req.Metadata {
				key = strings.TrimSpace(key)
				if key == "" {
					continue
				}
				metadata[key] = strings.TrimSpace(value)
			}
			tokenReq.Metadata = metadata
		}
		req = tokenReq
	}
	nodeID := strings.TrimSpace(req.NodeID)
	if nodeID == "" {
		nodeID = "node_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	}
	secret := strings.TrimSpace(req.NodeSecret)
	if secret == "" {
		generated, err := generateQuotaLeaseDemoNodeSecret()
		if err != nil {
			return nil, err
		}
		secret = generated
	}
	now := time.Now().UTC()
	heartbeatAt := now
	node := &QuotaLeaseDemoNode{
		NodeID:          nodeID,
		Secret:          secret,
		Region:          strings.TrimSpace(req.Region),
		BaseURL:         strings.TrimSpace(req.BaseURL),
		PublicKey:       strings.TrimSpace(req.PublicKey),
		Metadata:        cloneQuotaLeaseDemoStringMap(req.Metadata),
		Status:          QuotaLeaseDemoNodeStatusOnline,
		RegisteredAt:    now,
		LastHeartbeatAt: &heartbeatAt,
		UpdatedAt:       now,
	}

	s.mu.Lock()
	s.nodes[node.NodeID] = node
	persistedNode := cloneQuotaLeaseDemoNode(node)
	s.mu.Unlock()
	if err := s.persistQuotaLeaseDemoNode(ctx, persistedNode); err != nil {
		return nil, err
	}
	_ = ctx
	return &QuotaLeaseDemoNodeRegistrationResult{
		Node:       cloneQuotaLeaseDemoNode(node),
		NodeSecret: secret,
	}, nil
}

func (s *QuotaLeaseDemoService) resolveNodeRegistrationURL(token string, now time.Time) (QuotaLeaseDemoNodeRegistrationRequest, error) {
	token = strings.TrimSpace(token)
	if token == "" {
		return QuotaLeaseDemoNodeRegistrationRequest{}, fmt.Errorf("%w: registration_token is required", ErrQuotaLeaseDemoInvalidInput)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.cleanupExpiredRegistrationURLsLocked(now)
	item := s.registrationURLs[token]
	if item == nil {
		return QuotaLeaseDemoNodeRegistrationRequest{}, fmt.Errorf("%w: registration token is invalid or expired", ErrQuotaLeaseDemoInvalidInput)
	}
	return cloneQuotaLeaseDemoNodeRegistrationRequest(item.Request), nil
}

func (s *QuotaLeaseDemoService) cleanupExpiredRegistrationURLsLocked(now time.Time) {
	if s == nil || len(s.registrationURLs) == 0 {
		return
	}
	for token, item := range s.registrationURLs {
		if item == nil || (!item.ExpiresAt.IsZero() && !now.Before(item.ExpiresAt)) {
			delete(s.registrationURLs, token)
		}
	}
}

func (s *QuotaLeaseDemoService) AuthenticateNode(nodeID, secret string) bool {
	if s == nil || !s.Enabled() {
		return false
	}
	nodeID = strings.TrimSpace(nodeID)
	secret = strings.TrimSpace(secret)
	if nodeID == "" || secret == "" {
		return false
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	node := s.nodes[nodeID]
	return node != nil && node.Status != QuotaLeaseDemoNodeStatusDisabled && node.Secret == secret
}

func (s *QuotaLeaseDemoService) HeartbeatNode(ctx context.Context, req QuotaLeaseDemoNodeHeartbeatRequest) (*QuotaLeaseDemoNode, error) {
	if s.remoteMode() {
		return s.heartbeatRemoteNode(ctx, req)
	}
	return s.heartbeatNodeLocal(ctx, req)
}

func (s *QuotaLeaseDemoService) heartbeatNodeLocal(ctx context.Context, req QuotaLeaseDemoNodeHeartbeatRequest) (*QuotaLeaseDemoNode, error) {
	if s == nil || !s.Enabled() {
		return nil, ErrQuotaLeaseDemoDisabled
	}
	nodeID := strings.TrimSpace(req.NodeID)
	if nodeID == "" {
		return nil, fmt.Errorf("%w: node_id is required", ErrQuotaLeaseDemoInvalidInput)
	}
	now := time.Now().UTC()
	status := strings.TrimSpace(req.Status)
	if status == "" {
		status = QuotaLeaseDemoNodeStatusOnline
	}

	s.mu.Lock()
	node := s.nodes[nodeID]
	if node == nil {
		s.mu.Unlock()
		return nil, ErrQuotaLeaseDemoNodeNotFound
	}
	if node.Status == QuotaLeaseDemoNodeStatusDisabled {
		s.mu.Unlock()
		return nil, ErrQuotaLeaseDemoNodeNotFound
	}
	node.Status = status
	node.InflightRequests = req.InflightRequests
	node.LeaseRemaining = req.LeaseRemaining
	node.Metrics = cloneQuotaLeaseDemoFloatMap(req.Metrics)
	if req.SyncStatus != nil {
		node.SyncStatus = cloneQuotaLeaseDemoNodeSyncStatus(req.SyncStatus)
	}
	node.LastHeartbeatAt = &now
	node.UpdatedAt = now
	persistedNode := cloneQuotaLeaseDemoNode(node)
	result := cloneQuotaLeaseDemoNode(node)
	s.mu.Unlock()
	if err := s.persistQuotaLeaseDemoNode(ctx, persistedNode); err != nil {
		return nil, err
	}
	_ = ctx
	return result, nil
}

func (s *QuotaLeaseDemoService) UpdateNode(ctx context.Context, nodeID string, req QuotaLeaseDemoNodeUpdateRequest) (*QuotaLeaseDemoNode, error) {
	if s == nil || !s.Enabled() {
		return nil, ErrQuotaLeaseDemoDisabled
	}
	nodeID = strings.TrimSpace(nodeID)
	if nodeID == "" {
		return nil, fmt.Errorf("%w: node_id is required", ErrQuotaLeaseDemoInvalidInput)
	}

	s.mu.Lock()

	node := s.nodes[nodeID]
	if node == nil {
		s.mu.Unlock()
		return nil, ErrQuotaLeaseDemoNodeNotFound
	}

	if req.Region != nil {
		node.Region = strings.TrimSpace(*req.Region)
	}
	if req.BaseURL != nil {
		node.BaseURL = strings.TrimSpace(*req.BaseURL)
	}
	if req.PublicKey != nil {
		node.PublicKey = strings.TrimSpace(*req.PublicKey)
	}
	if req.Metadata != nil {
		node.Metadata = cloneQuotaLeaseDemoStringMap(req.Metadata)
	}
	if req.Status != nil {
		status := strings.TrimSpace(*req.Status)
		switch status {
		case QuotaLeaseDemoNodeStatusOnline, QuotaLeaseDemoNodeStatusOffline, QuotaLeaseDemoNodeStatusDisabled:
			node.Status = status
		default:
			s.mu.Unlock()
			return nil, fmt.Errorf("%w: invalid node status", ErrQuotaLeaseDemoInvalidInput)
		}
	}
	node.UpdatedAt = time.Now().UTC()
	persistedNode := cloneQuotaLeaseDemoNode(node)
	result := cloneQuotaLeaseDemoNode(node)
	s.mu.Unlock()
	if err := s.persistQuotaLeaseDemoNode(ctx, persistedNode); err != nil {
		return nil, err
	}
	_ = ctx
	return result, nil
}

func (s *QuotaLeaseDemoService) ListNodes() []QuotaLeaseDemoNode {
	if s == nil || !s.Enabled() {
		return nil
	}
	now := time.Now().UTC()
	s.mu.Lock()
	defer s.mu.Unlock()
	remainingByNode := s.nodeLeaseRemainingByNodeLocked(now)
	nodes := make([]QuotaLeaseDemoNode, 0, len(s.nodes))
	for _, node := range s.nodes {
		if cloned := cloneQuotaLeaseDemoNode(node); cloned != nil {
			cloned.LeaseRemaining = remainingByNode[cloned.NodeID]
			nodes = append(nodes, *cloned)
		}
	}
	sort.Slice(nodes, func(i, j int) bool {
		return nodes[i].RegisteredAt.Before(nodes[j].RegisteredAt)
	})
	return nodes
}

func (s *QuotaLeaseDemoService) RuntimeHeartbeatRequest() QuotaLeaseDemoNodeHeartbeatRequest {
	req := QuotaLeaseDemoNodeHeartbeatRequest{
		NodeID: s.activeNodeID(),
		Status: QuotaLeaseDemoNodeStatusOnline,
	}
	if s == nil || !s.Enabled() {
		return req
	}

	now := time.Now().UTC()
	s.mu.Lock()
	activeLeases := 0
	for _, lease := range s.leases {
		if lease == nil || lease.NodeID != req.NodeID {
			continue
		}
		s.refreshLeaseStatusLocked(lease, now)
		if lease.Status == QuotaLeaseDemoStatusActive {
			activeLeases++
			req.LeaseRemaining += lease.Remaining()
		}
	}
	pendingUsageEvents := len(s.pendingEvents)
	pendingUsageLogs := len(s.pendingUsageLogs)
	pendingOpsErrorLogs := len(s.pendingOpsErrorLogs)
	s.mu.Unlock()

	syncStatus := s.nodeSyncStatusSnapshot()
	syncStatus.PendingUsageEvents = pendingUsageEvents
	syncStatus.PendingUsageLogs = pendingUsageLogs
	syncStatus.PendingOpsErrorLogs = pendingOpsErrorLogs
	req.SyncStatus = &syncStatus
	req.Metrics = map[string]float64{
		"active_leases":          float64(activeLeases),
		"pending_usage_events":   float64(pendingUsageEvents),
		"pending_usage_logs":     float64(pendingUsageLogs),
		"pending_ops_error_logs": float64(pendingOpsErrorLogs),
	}
	return req
}

func (s *QuotaLeaseDemoService) ReportRuntimeHeartbeat(ctx context.Context) (*QuotaLeaseDemoNode, error) {
	if s == nil || !s.Enabled() {
		return nil, ErrQuotaLeaseDemoDisabled
	}
	return s.HeartbeatNode(ctx, s.RuntimeHeartbeatRequest())
}

type QuotaLeaseDemoLeaseRequest struct {
	NodeID              string  `json:"node_id"`
	UserID              int64   `json:"user_id"`
	APIKeyID            int64   `json:"api_key_id"`
	Amount              float64 `json:"amount"`
	RequestID           string  `json:"request_id,omitempty"`
	TraceID             string  `json:"trace_id,omitempty"`
	TTLSeconds          int     `json:"ttl_seconds"`
	ReclaimGraceSeconds int     `json:"reclaim_grace_seconds"`
}

type QuotaLeaseDemoLease struct {
	ID        string    `json:"id"`
	NodeID    string    `json:"node_id"`
	UserID    int64     `json:"user_id"`
	APIKeyID  int64     `json:"api_key_id"`
	Granted   float64   `json:"granted"`
	Consumed  float64   `json:"consumed"`
	Reclaimed float64   `json:"reclaimed"`
	Version   int64     `json:"version"`
	TraceID   string    `json:"trace_id,omitempty"`
	Status    string    `json:"status"`
	ExpiresAt time.Time `json:"expires_at"`
	ReclaimAt time.Time `json:"reclaim_at"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

func (l QuotaLeaseDemoLease) Remaining() float64 {
	return l.Granted - l.Consumed - l.Reclaimed
}

type QuotaLeaseDemoUsageEvent struct {
	EventID   string    `json:"event_id"`
	LeaseID   string    `json:"lease_id"`
	NodeID    string    `json:"node_id"`
	UserID    int64     `json:"user_id"`
	APIKeyID  int64     `json:"api_key_id"`
	RequestID string    `json:"request_id"`
	TraceID   string    `json:"trace_id,omitempty"`
	Amount    float64   `json:"amount"`
	EventType string    `json:"event_type"`
	CreatedAt time.Time `json:"created_at"`
}

type QuotaLeaseDemoLedgerEvent struct {
	EventID     string    `json:"event_id"`
	LeaseID     string    `json:"lease_id"`
	NodeID      string    `json:"node_id"`
	UserID      int64     `json:"user_id"`
	APIKeyID    int64     `json:"api_key_id"`
	RequestID   string    `json:"request_id"`
	TraceID     string    `json:"trace_id,omitempty"`
	Amount      float64   `json:"amount"`
	EventType   string    `json:"event_type"`
	PayloadHash string    `json:"payload_hash"`
	CreatedAt   time.Time `json:"created_at"`
}

type QuotaLeaseDemoUsageResult struct {
	EventID   string               `json:"event_id"`
	LeaseID   string               `json:"lease_id"`
	TraceID   string               `json:"trace_id,omitempty"`
	Applied   bool                 `json:"applied"`
	Duplicate bool                 `json:"duplicate"`
	Error     string               `json:"error,omitempty"`
	Lease     *QuotaLeaseDemoLease `json:"lease,omitempty"`
}

type QuotaLeaseDemoUsageBatchRequest struct {
	NodeID string                     `json:"node_id"`
	Events []QuotaLeaseDemoUsageEvent `json:"events"`
}

type QuotaLeaseDemoUsageBatchResult struct {
	Results []QuotaLeaseDemoUsageResult `json:"results"`
}

type QuotaLeaseDemoReclaimResult struct {
	ExpiredCount   int     `json:"expired_count"`
	ReclaimedCount int     `json:"reclaimed_count"`
	ReclaimedTotal float64 `json:"reclaimed_total"`
}

type QuotaLeaseDemoCleanupResult struct {
	LeaseCount       int64 `json:"lease_count"`
	LedgerEventCount int64 `json:"ledger_event_count"`
}

type QuotaLeaseDemoSnapshot struct {
	Enabled        bool                        `json:"enabled"`
	NodeID         string                      `json:"node_id"`
	MirrorReady    bool                        `json:"mirror_ready"`
	MirrorSyncedAt *time.Time                  `json:"mirror_synced_at,omitempty"`
	Nodes          []QuotaLeaseDemoNode        `json:"nodes"`
	Leases         []QuotaLeaseDemoLease       `json:"leases"`
	Events         []QuotaLeaseDemoLedgerEvent `json:"events"`
	Stats          QuotaLeaseDemoSnapshotStats `json:"stats"`
}

type QuotaLeaseDemoSnapshotStats struct {
	ActiveLeases    int     `json:"active_leases"`
	ExpiredLeases   int     `json:"expired_leases"`
	ClosedLeases    int     `json:"closed_leases"`
	ReclaimedLeases int     `json:"reclaimed_leases"`
	GrantedTotal    float64 `json:"granted_total"`
	ConsumedTotal   float64 `json:"consumed_total"`
	ReclaimedTotal  float64 `json:"reclaimed_total"`
	RemainingTotal  float64 `json:"remaining_total"`
	EventCount      int     `json:"event_count"`
	NodeCount       int     `json:"node_count"`
	OnlineNodes     int     `json:"online_nodes"`
}

func (s *QuotaLeaseDemoService) RequestLease(ctx context.Context, req QuotaLeaseDemoLeaseRequest) (*QuotaLeaseDemoLease, error) {
	if s == nil {
		return nil, ErrQuotaLeaseDemoDisabled
	}
	if s.remoteMode() {
		return s.requestLeaseCoalesced(ctx, req, s.requestRemoteLease)
	}
	return s.requestLeaseCoalesced(ctx, req, s.requestLeaseLocal)
}

func (s *QuotaLeaseDemoService) requestLeaseCoalesced(ctx context.Context, req QuotaLeaseDemoLeaseRequest, request func(context.Context, QuotaLeaseDemoLeaseRequest) (*QuotaLeaseDemoLease, error)) (*QuotaLeaseDemoLease, error) {
	if request == nil {
		return nil, ErrQuotaLeaseDemoDisabled
	}
	if s == nil || req.UserID <= 0 || req.APIKeyID <= 0 {
		return request(ctx, req)
	}
	nodeID := strings.TrimSpace(req.NodeID)
	if nodeID == "" {
		if s.remoteMode() {
			nodeID = s.activeNodeID()
		} else {
			nodeID = s.NodeID()
		}
	}
	amount := req.Amount
	if amount <= 0 {
		amount = s.DefaultGrantAmount()
	}
	if s.remoteMode() && finitePositive(amount) {
		if existing := s.findLeaseForConsumption(nodeID, req.UserID, req.APIKeyID, amount, time.Now().UTC()); existing != nil {
			return existing, nil
		}
	}
	key := quotaLeaseDemoLeaseRequestGroupKey(nodeID, req.UserID, req.APIKeyID)
	value, err, _ := s.leaseRequestGroup.Do(key, func() (any, error) {
		return request(ctx, req)
	})
	if err != nil {
		return nil, err
	}
	lease, ok := value.(*QuotaLeaseDemoLease)
	if !ok || lease == nil {
		return nil, fmt.Errorf("%w: lease response missing lease", ErrQuotaLeaseDemoInvalidInput)
	}
	return cloneQuotaLeaseDemoLease(lease), nil
}

func (s *QuotaLeaseDemoService) requestLeaseLocal(ctx context.Context, req QuotaLeaseDemoLeaseRequest) (*QuotaLeaseDemoLease, error) {
	if s == nil || !s.Enabled() {
		return nil, ErrQuotaLeaseDemoDisabled
	}
	cfg := s.cfgSnapshot()
	nodeID := strings.TrimSpace(req.NodeID)
	if nodeID == "" {
		nodeID = s.NodeID()
	}
	if req.UserID <= 0 || req.APIKeyID <= 0 || nodeID == "" {
		return nil, fmt.Errorf("%w: user_id, api_key_id and node_id are required", ErrQuotaLeaseDemoInvalidInput)
	}
	req.RequestID = strings.TrimSpace(req.RequestID)
	req.TraceID = quotaLeaseDemoTraceID(req.TraceID, nodeID, req.UserID, req.APIKeyID, req.RequestID)
	amount := req.Amount
	if amount <= 0 {
		amount = cfg.DefaultGrantAmount
	}
	if !finitePositive(amount) {
		return nil, fmt.Errorf("%w: amount must be positive and finite", ErrQuotaLeaseDemoInvalidInput)
	}
	graceSeconds := req.ReclaimGraceSeconds
	if graceSeconds <= 0 {
		graceSeconds = cfg.ReclaimGraceSeconds
	}
	if graceSeconds <= 0 {
		graceSeconds = 3600
	}

	now := time.Now().UTC()
	expiresAt := quotaLeaseDemoIdleExpiresAt(now)
	reclaimAt := quotaLeaseDemoReclaimAt(expiresAt, graceSeconds)

	var persistedLease *QuotaLeaseDemoLease
	var persistedEvent *QuotaLeaseDemoLedgerEvent

	s.mu.Lock()

	var reusable *QuotaLeaseDemoLease
	var extendable *QuotaLeaseDemoLease
	for _, lease := range s.indexedQuotaLeaseDemoLeasesLocked(nodeID, req.UserID, req.APIKeyID) {
		s.refreshLeaseStatusLocked(lease, now)
		if lease.Status != QuotaLeaseDemoStatusActive {
			continue
		}
		if extendable == nil || lease.ExpiresAt.Before(extendable.ExpiresAt) {
			extendable = lease
		}
		if lease.Remaining()+1e-12 >= amount && (reusable == nil || lease.ExpiresAt.Before(reusable.ExpiresAt)) {
			reusable = lease
		}
	}
	if reusable != nil {
		reusable.ExpiresAt = expiresAt
		reusable.ReclaimAt = reclaimAt
		reusable.UpdatedAt = now
		reusable.TraceID = req.TraceID
		advanceQuotaLeaseDemoLeaseVersion(reusable)
		persistedLease = cloneQuotaLeaseDemoLease(reusable)
		s.logQuotaLeaseDemoTrace("lease_reused", req.TraceID, req.RequestID, reusable.ID, nodeID, req.UserID, req.APIKeyID, map[string]any{
			"remaining": reusable.Remaining(),
		})
		s.mu.Unlock()
		if err := s.persistQuotaLeaseDemoLease(ctx, persistedLease); err != nil {
			return nil, err
		}
		return cloneQuotaLeaseDemoLease(persistedLease), nil
	}
	if extendable != nil {
		delta := amount - extendable.Remaining()
		targetGranted := extendable.Granted
		if delta > 0 {
			targetGranted += delta
		}
		extendable.ExpiresAt = expiresAt
		extendable.ReclaimAt = reclaimAt
		extendable.Granted = targetGranted
		extendable.UpdatedAt = now
		extendable.TraceID = req.TraceID
		advanceQuotaLeaseDemoLeaseVersion(extendable)
		eventID := "lease:" + extendable.ID
		event := &QuotaLeaseDemoLedgerEvent{
			EventID:     eventID,
			LeaseID:     extendable.ID,
			NodeID:      extendable.NodeID,
			UserID:      extendable.UserID,
			APIKeyID:    extendable.APIKeyID,
			RequestID:   req.RequestID,
			TraceID:     req.TraceID,
			Amount:      extendable.Granted,
			EventType:   QuotaLeaseDemoEventLeaseGranted,
			PayloadHash: quotaLeaseDemoPayloadHash(extendable.ID, extendable.NodeID, extendable.UserID, extendable.APIKeyID, "", extendable.Granted, QuotaLeaseDemoEventLeaseGranted),
			CreatedAt:   extendable.CreatedAt,
		}
		s.events[eventID] = event
		persistedLease = cloneQuotaLeaseDemoLease(extendable)
		persistedEvent = event
		s.logQuotaLeaseDemoTrace("lease_extended", req.TraceID, req.RequestID, extendable.ID, nodeID, req.UserID, req.APIKeyID, map[string]any{
			"granted":   extendable.Granted,
			"remaining": extendable.Remaining(),
		})
		s.mu.Unlock()
		if err := s.persistQuotaLeaseDemoLeaseAndEvent(ctx, persistedLease, persistedEvent); err != nil {
			return nil, err
		}
		return cloneQuotaLeaseDemoLease(persistedLease), nil
	}

	lease := &QuotaLeaseDemoLease{
		ID:        "ql_demo_" + uuid.NewString(),
		NodeID:    nodeID,
		UserID:    req.UserID,
		APIKeyID:  req.APIKeyID,
		Granted:   amount,
		Version:   1,
		TraceID:   req.TraceID,
		Status:    QuotaLeaseDemoStatusActive,
		ExpiresAt: expiresAt,
		ReclaimAt: reclaimAt,
		CreatedAt: now,
		UpdatedAt: now,
	}

	s.upsertQuotaLeaseDemoLeaseLocked(lease)
	event := &QuotaLeaseDemoLedgerEvent{
		EventID:     "lease:" + lease.ID,
		LeaseID:     lease.ID,
		NodeID:      lease.NodeID,
		UserID:      lease.UserID,
		APIKeyID:    lease.APIKeyID,
		RequestID:   req.RequestID,
		TraceID:     req.TraceID,
		Amount:      lease.Granted,
		EventType:   QuotaLeaseDemoEventLeaseGranted,
		PayloadHash: quotaLeaseDemoPayloadHash(lease.ID, lease.NodeID, lease.UserID, lease.APIKeyID, "", lease.Granted, QuotaLeaseDemoEventLeaseGranted),
		CreatedAt:   now,
	}
	s.events[event.EventID] = event
	persistedLease = cloneQuotaLeaseDemoLease(lease)
	persistedEvent = event
	s.logQuotaLeaseDemoTrace("lease_granted", req.TraceID, req.RequestID, lease.ID, nodeID, req.UserID, req.APIKeyID, map[string]any{
		"granted": lease.Granted,
	})
	s.mu.Unlock()
	if err := s.persistQuotaLeaseDemoLeaseAndEvent(ctx, persistedLease, persistedEvent); err != nil {
		return nil, err
	}
	_ = ctx
	return cloneQuotaLeaseDemoLease(persistedLease), nil
}

func quotaLeaseDemoIdleExpiresAt(now time.Time) time.Time {
	if now.IsZero() {
		now = time.Now().UTC()
	}
	return now.UTC().Add(quotaLeaseDemoIdleLeaseTTL)
}

func quotaLeaseDemoReclaimAt(expiresAt time.Time, graceSeconds int) time.Time {
	if graceSeconds <= 0 {
		graceSeconds = 3600
	}
	return expiresAt.Add(time.Duration(graceSeconds) * time.Second)
}

func (s *QuotaLeaseDemoService) CanAuthorizeRequest(ctx context.Context, apiKey *APIKey, subscription *UserSubscription) bool {
	if s == nil || !s.Enabled() || apiKey == nil || apiKey.User == nil || subscription != nil {
		return false
	}
	nodeID := s.activeNodeID()
	requestID := quotaLeaseDemoContextRequestID(ctx)
	traceID := quotaLeaseDemoTraceID("", nodeID, apiKey.User.ID, apiKey.ID, requestID)
	amount := s.DefaultGrantAmount()
	if balance := apiKey.User.Balance; finitePositive(balance) && balance < amount {
		amount = balance
	}
	if !finitePositive(amount) {
		return false
	}
	s.logQuotaLeaseDemoTrace("gateway_preflight_start", traceID, requestID, "", nodeID, apiKey.User.ID, apiKey.ID, map[string]any{
		"amount": amount,
	})
	return s.ensureCapacityWithMinimum(ctx, "gateway_preflight", nodeID, apiKey.User.ID, apiKey.ID, amount, s.preflightCapacityCheckAmount(amount))
}

func (s *QuotaLeaseDemoService) ApplyUsageBilling(ctx context.Context, cmd *UsageBillingCommand) (handled bool, applied bool, err error) {
	if s == nil || !s.Enabled() || cmd == nil {
		return false, false, nil
	}
	if cmd.BalanceCost <= 0 || cmd.SubscriptionCost > 0 {
		return false, false, nil
	}

	nodeID := s.activeNodeID()
	lease := s.findLeaseForConsumption(nodeID, cmd.UserID, cmd.APIKeyID, cmd.BalanceCost, time.Now().UTC())
	if lease == nil && s.remoteMode() {
		_ = s.ensureCapacity(ctx, "usage_billing", nodeID, cmd.UserID, cmd.APIKeyID, s.usageBillingCapacityTarget(cmd.BalanceCost))
		lease = s.findLeaseForConsumption(nodeID, cmd.UserID, cmd.APIKeyID, cmd.BalanceCost, time.Now().UTC())
	}
	if lease == nil {
		lease = s.findActiveLeaseForBilling(nodeID, cmd.UserID, cmd.APIKeyID, time.Now().UTC())
	}
	if lease == nil {
		return true, false, ErrQuotaLeaseDemoNoCapacity
	}
	traceID := quotaLeaseDemoTraceID(strings.TrimSpace(lease.TraceID), nodeID, cmd.UserID, cmd.APIKeyID, cmd.RequestID)
	eventID := quotaLeaseDemoUsageEventID(nodeID, lease.ID, cmd.RequestID)
	event := QuotaLeaseDemoUsageEvent{
		EventID:   eventID,
		LeaseID:   lease.ID,
		NodeID:    nodeID,
		UserID:    cmd.UserID,
		APIKeyID:  cmd.APIKeyID,
		RequestID: cmd.RequestID,
		TraceID:   traceID,
		Amount:    cmd.BalanceCost,
		EventType: QuotaLeaseDemoEventUsagePosted,
		CreatedAt: time.Now().UTC(),
	}
	s.logQuotaLeaseDemoTrace("usage_billing_selected", traceID, cmd.RequestID, lease.ID, nodeID, cmd.UserID, cmd.APIKeyID, map[string]any{
		"amount":    cmd.BalanceCost,
		"remaining": lease.Remaining(),
	})
	result, consumeErr := s.consumeUsageLocal(ctx, event)
	if consumeErr != nil {
		return true, false, consumeErr
	}
	if s.remoteMode() && result.Applied && !result.Duplicate {
		if err := s.enqueuePendingUsageEventWithContext(ctx, event); err != nil {
			return true, true, err
		}
		if result.Lease != nil && result.Lease.Remaining() < -1e-12 {
			target := s.usageBillingCapacityTarget(-result.Lease.Remaining())
			if flushErr := s.FlushPendingUsage(ctx); flushErr != nil {
				probe := s.inspectCapacitySnapshot(nodeID, cmd.UserID, cmd.APIKeyID, s.preflightCapacityCheckAmount(target), time.Now().UTC())
				s.logCapacityDenied("usage_billing_overdraft", "pending_usage_flush_failed", nodeID, cmd.UserID, cmd.APIKeyID, target, probe, flushErr)
				return true, true, ErrQuotaLeaseDemoNoCapacity
			}
			if _, requestErr := s.RequestLease(ctx, QuotaLeaseDemoLeaseRequest{
				UserID:    cmd.UserID,
				APIKeyID:  cmd.APIKeyID,
				Amount:    target,
				RequestID: cmd.RequestID,
				TraceID:   traceID,
			}); requestErr != nil {
				probe := s.inspectCapacitySnapshot(nodeID, cmd.UserID, cmd.APIKeyID, s.preflightCapacityCheckAmount(target), time.Now().UTC())
				s.logCapacityDenied("usage_billing_overdraft", "request_lease_failed", nodeID, cmd.UserID, cmd.APIKeyID, target, probe, requestErr)
				return true, true, ErrQuotaLeaseDemoNoCapacity
			}
			if ok, probe := s.inspectCapacity(nodeID, cmd.UserID, cmd.APIKeyID, s.preflightCapacityCheckAmount(target), time.Now().UTC()); !ok {
				s.logCapacityDenied("usage_billing_overdraft", "post_request_capacity_check_failed", nodeID, cmd.UserID, cmd.APIKeyID, target, probe)
				return true, true, ErrQuotaLeaseDemoNoCapacity
			}
			return true, true, nil
		}
		s.flushPendingUsageAsync()
		s.maybePrefetchUsageLease(ctx, result.Lease, cmd.BalanceCost)
	}
	return true, result.Applied && !result.Duplicate, nil
}

func (s *QuotaLeaseDemoService) ConsumeUsage(ctx context.Context, event QuotaLeaseDemoUsageEvent) (*QuotaLeaseDemoUsageResult, error) {
	result, err := s.consumeUsageLocal(ctx, event)
	if err != nil {
		return nil, err
	}
	if s.remoteMode() && result.Applied && !result.Duplicate {
		event.EventID = strings.TrimSpace(result.EventID)
		if event.LeaseID == "" {
			event.LeaseID = strings.TrimSpace(result.LeaseID)
		}
		if event.NodeID == "" {
			event.NodeID = s.activeNodeID()
		}
		if event.EventType == "" {
			event.EventType = QuotaLeaseDemoEventUsagePosted
		}
		if event.CreatedAt.IsZero() {
			event.CreatedAt = time.Now().UTC()
		}
		if err := s.enqueuePendingUsageEventWithContext(ctx, event); err != nil {
			return nil, err
		}
		s.flushPendingUsageAsync()
		s.maybePrefetchUsageLease(ctx, result.Lease, event.Amount)
	}
	return result, nil
}

func (s *QuotaLeaseDemoService) consumeUsageLocal(ctx context.Context, event QuotaLeaseDemoUsageEvent) (*QuotaLeaseDemoUsageResult, error) {
	if s == nil || !s.Enabled() {
		return nil, ErrQuotaLeaseDemoDisabled
	}
	if !finitePositive(event.Amount) {
		return nil, fmt.Errorf("%w: amount must be positive and finite", ErrQuotaLeaseDemoInvalidInput)
	}
	event.EventID = strings.TrimSpace(event.EventID)
	event.LeaseID = strings.TrimSpace(event.LeaseID)
	event.NodeID = strings.TrimSpace(event.NodeID)
	event.RequestID = strings.TrimSpace(event.RequestID)
	event.TraceID = strings.TrimSpace(event.TraceID)
	event.EventType = strings.TrimSpace(event.EventType)
	if event.EventType == "" {
		event.EventType = QuotaLeaseDemoEventUsagePosted
	}
	if event.NodeID == "" {
		event.NodeID = s.NodeID()
	}
	if event.TraceID == "" {
		event.TraceID = quotaLeaseDemoTraceID("", event.NodeID, event.UserID, event.APIKeyID, event.RequestID)
	}
	if event.EventID == "" || event.LeaseID == "" || event.UserID <= 0 || event.APIKeyID <= 0 || event.RequestID == "" {
		return nil, fmt.Errorf("%w: event_id, lease_id, user_id, api_key_id and request_id are required", ErrQuotaLeaseDemoInvalidInput)
	}
	if event.CreatedAt.IsZero() {
		event.CreatedAt = time.Now().UTC()
	}
	payloadHash := quotaLeaseDemoPayloadHash(event.LeaseID, event.NodeID, event.UserID, event.APIKeyID, event.RequestID, event.Amount, event.EventType)

	s.mu.Lock()

	if existing := s.events[event.EventID]; existing != nil {
		if existing.PayloadHash != payloadHash {
			s.mu.Unlock()
			return nil, ErrQuotaLeaseDemoConflict
		}
		result := &QuotaLeaseDemoUsageResult{
			EventID:   event.EventID,
			LeaseID:   existing.LeaseID,
			TraceID:   existing.TraceID,
			Applied:   false,
			Duplicate: true,
			Lease:     cloneQuotaLeaseDemoLease(s.leases[existing.LeaseID]),
		}
		s.mu.Unlock()
		return &QuotaLeaseDemoUsageResult{
			EventID:   result.EventID,
			LeaseID:   result.LeaseID,
			TraceID:   result.TraceID,
			Applied:   result.Applied,
			Duplicate: result.Duplicate,
			Lease:     result.Lease,
		}, nil
	}

	lease := s.leases[event.LeaseID]
	if lease == nil {
		s.mu.Unlock()
		return nil, fmt.Errorf("%w: lease not found", ErrQuotaLeaseDemoInvalidInput)
	}
	now := time.Now().UTC()
	s.refreshLeaseStatusLocked(lease, now)
	if lease.Status != QuotaLeaseDemoStatusActive {
		s.mu.Unlock()
		return nil, ErrQuotaLeaseDemoNoCapacity
	}
	if lease.NodeID != event.NodeID || lease.UserID != event.UserID || lease.APIKeyID != event.APIKeyID {
		s.mu.Unlock()
		return nil, fmt.Errorf("%w: event does not match lease", ErrQuotaLeaseDemoInvalidInput)
	}
	nextLease := *lease
	nextLease.Consumed += event.Amount
	nextLease.Version = nextQuotaLeaseDemoLeaseVersion(lease.Version)
	remaining := nextLease.Remaining()
	if remaining > 1e-12 || remaining < -1e-12 {
		graceSeconds := int(nextLease.ReclaimAt.Sub(nextLease.ExpiresAt).Seconds())
		if graceSeconds <= 0 {
			graceSeconds = 3600
		}
		nextLease.ExpiresAt = quotaLeaseDemoIdleExpiresAt(now)
		nextLease.ReclaimAt = quotaLeaseDemoReclaimAt(nextLease.ExpiresAt, graceSeconds)
	}
	nextLease.UpdatedAt = now
	nextLease.TraceID = event.TraceID
	if math.Abs(remaining) <= 1e-12 {
		nextLease.Status = QuotaLeaseDemoStatusClosed
	}
	ledgerEvent := &QuotaLeaseDemoLedgerEvent{
		EventID:     event.EventID,
		LeaseID:     event.LeaseID,
		NodeID:      event.NodeID,
		UserID:      event.UserID,
		APIKeyID:    event.APIKeyID,
		RequestID:   event.RequestID,
		TraceID:     event.TraceID,
		Amount:      event.Amount,
		EventType:   event.EventType,
		PayloadHash: payloadHash,
		CreatedAt:   event.CreatedAt,
	}
	billingApplied, persistedByBilling, err := s.applyLeaseUsageBilling(ctx, event, &nextLease, ledgerEvent)
	if err != nil {
		s.mu.Unlock()
		return nil, err
	}
	if !billingApplied {
		result := &QuotaLeaseDemoUsageResult{
			EventID:   event.EventID,
			LeaseID:   event.LeaseID,
			TraceID:   event.TraceID,
			Applied:   false,
			Duplicate: true,
			Lease:     cloneQuotaLeaseDemoLease(lease),
		}
		s.mu.Unlock()
		return &QuotaLeaseDemoUsageResult{
			EventID:   result.EventID,
			LeaseID:   result.LeaseID,
			TraceID:   result.TraceID,
			Applied:   result.Applied,
			Duplicate: result.Duplicate,
			Lease:     result.Lease,
		}, nil
	}
	*lease = nextLease
	s.events[event.EventID] = ledgerEvent
	persistedLease := cloneQuotaLeaseDemoLease(lease)
	persistedEvent := *ledgerEvent
	s.logQuotaLeaseDemoTrace("usage_billing_applied", event.TraceID, event.RequestID, event.LeaseID, event.NodeID, event.UserID, event.APIKeyID, map[string]any{
		"amount":    event.Amount,
		"remaining": persistedLease.Remaining(),
	})
	s.mu.Unlock()
	if !persistedByBilling {
		if err := s.persistQuotaLeaseDemoLeaseAndEvent(ctx, persistedLease, &persistedEvent); err != nil {
			return nil, err
		}
	}
	_ = ctx
	return &QuotaLeaseDemoUsageResult{
		EventID: event.EventID,
		LeaseID: event.LeaseID,
		TraceID: event.TraceID,
		Applied: true,
		Lease:   persistedLease,
	}, nil
}

func (s *QuotaLeaseDemoService) PostUsageBatch(ctx context.Context, req QuotaLeaseDemoUsageBatchRequest) QuotaLeaseDemoUsageBatchResult {
	if s.remoteMode() {
		result, err := s.postRemoteUsageBatch(ctx, req)
		if err != nil {
			results := make([]QuotaLeaseDemoUsageResult, 0, len(req.Events))
			for _, event := range req.Events {
				results = append(results, QuotaLeaseDemoUsageResult{
					EventID: strings.TrimSpace(event.EventID),
					LeaseID: strings.TrimSpace(event.LeaseID),
					Error:   err.Error(),
				})
			}
			return QuotaLeaseDemoUsageBatchResult{Results: results}
		}
		return result
	}
	return s.postUsageBatchLocal(ctx, req)
}

func (s *QuotaLeaseDemoService) postUsageBatchLocal(ctx context.Context, req QuotaLeaseDemoUsageBatchRequest) QuotaLeaseDemoUsageBatchResult {
	results := make([]QuotaLeaseDemoUsageResult, 0, len(req.Events))
	for _, event := range req.Events {
		if strings.TrimSpace(event.NodeID) == "" {
			event.NodeID = strings.TrimSpace(req.NodeID)
		}
		result, err := s.ConsumeUsage(ctx, event)
		if err != nil {
			results = append(results, QuotaLeaseDemoUsageResult{
				EventID: strings.TrimSpace(event.EventID),
				LeaseID: strings.TrimSpace(event.LeaseID),
				Error:   err.Error(),
			})
			continue
		}
		results = append(results, *result)
	}
	return QuotaLeaseDemoUsageBatchResult{Results: results}
}

func (s *QuotaLeaseDemoService) ReclaimExpired(ctx context.Context, now time.Time) QuotaLeaseDemoReclaimResult {
	if now.IsZero() {
		now = time.Now().UTC()
	}
	result := QuotaLeaseDemoReclaimResult{}
	if s == nil || !s.Enabled() {
		return result
	}

	var persistedLeases []*QuotaLeaseDemoLease
	var persistedEvents []*QuotaLeaseDemoLedgerEvent
	s.mu.Lock()
	for _, lease := range s.leases {
		before := lease.Status
		s.refreshLeaseStatusLocked(lease, now)
		if before == QuotaLeaseDemoStatusActive && lease.Status == QuotaLeaseDemoStatusExpired {
			result.ExpiredCount++
		}
		if lease.Status != QuotaLeaseDemoStatusExpired {
			continue
		}
		remaining := lease.Remaining()
		reclaimed := remaining
		if reclaimed < 0 {
			reclaimed = 0
		}
		if reclaimed > 0 {
			lease.Reclaimed += reclaimed
			result.ReclaimedTotal += reclaimed
		}
		lease.Status = QuotaLeaseDemoStatusReclaimed
		lease.UpdatedAt = now
		advanceQuotaLeaseDemoLeaseVersion(lease)
		result.ReclaimedCount++
		eventID := "reclaim:" + lease.ID
		event := &QuotaLeaseDemoLedgerEvent{
			EventID:     eventID,
			LeaseID:     lease.ID,
			NodeID:      lease.NodeID,
			UserID:      lease.UserID,
			APIKeyID:    lease.APIKeyID,
			Amount:      reclaimed,
			EventType:   QuotaLeaseDemoEventReclaimed,
			TraceID:     lease.TraceID,
			PayloadHash: quotaLeaseDemoPayloadHash(lease.ID, lease.NodeID, lease.UserID, lease.APIKeyID, eventID, reclaimed, QuotaLeaseDemoEventReclaimed),
			CreatedAt:   now,
		}
		s.events[eventID] = event
		persistedLeases = append(persistedLeases, cloneQuotaLeaseDemoLease(lease))
		persistedEvents = append(persistedEvents, event)
	}
	s.mu.Unlock()
	for i := range persistedLeases {
		var event *QuotaLeaseDemoLedgerEvent
		if i < len(persistedEvents) {
			event = persistedEvents[i]
		}
		if err := s.persistQuotaLeaseDemoLeaseAndEvent(ctx, persistedLeases[i], event); err != nil {
			slog.Warn("quota_lease_demo.reclaim_persist_failed", "lease_id", persistedLeases[i].ID, "error", err)
		}
	}
	_ = ctx
	return result
}

func (s *QuotaLeaseDemoService) Snapshot() QuotaLeaseDemoSnapshot {
	snap := QuotaLeaseDemoSnapshot{
		Enabled: s != nil && s.Enabled(),
		NodeID:  "",
	}
	if s == nil {
		return snap
	}
	snap.NodeID = s.NodeID()
	mirrorReady, mirrorSyncedAt := s.mirrorSnapshotState()
	snap.MirrorReady = mirrorReady
	if !mirrorSyncedAt.IsZero() {
		syncedAt := mirrorSyncedAt
		snap.MirrorSyncedAt = &syncedAt
	}
	now := time.Now().UTC()

	s.mu.Lock()
	defer s.mu.Unlock()
	remainingByNode := s.nodeLeaseRemainingByNodeLocked(now)

	snap.Nodes = make([]QuotaLeaseDemoNode, 0, len(s.nodes))
	for _, node := range s.nodes {
		if cloned := cloneQuotaLeaseDemoNode(node); cloned != nil {
			cloned.LeaseRemaining = remainingByNode[cloned.NodeID]
			snap.Nodes = append(snap.Nodes, *cloned)
			snap.Stats.NodeCount++
			if cloned.Status == QuotaLeaseDemoNodeStatusOnline {
				snap.Stats.OnlineNodes++
			}
		}
	}
	sort.Slice(snap.Nodes, func(i, j int) bool {
		return snap.Nodes[i].RegisteredAt.Before(snap.Nodes[j].RegisteredAt)
	})

	snap.Leases = make([]QuotaLeaseDemoLease, 0, len(s.leases))
	for _, lease := range s.leases {
		s.refreshLeaseStatusLocked(lease, now)
		value := *lease
		snap.Leases = append(snap.Leases, value)
		snap.Stats.GrantedTotal += lease.Granted
		snap.Stats.ConsumedTotal += lease.Consumed
		snap.Stats.ReclaimedTotal += lease.Reclaimed
		snap.Stats.RemainingTotal += lease.Remaining()
		switch lease.Status {
		case QuotaLeaseDemoStatusActive:
			snap.Stats.ActiveLeases++
		case QuotaLeaseDemoStatusExpired:
			snap.Stats.ExpiredLeases++
		case QuotaLeaseDemoStatusClosed:
			snap.Stats.ClosedLeases++
		case QuotaLeaseDemoStatusReclaimed:
			snap.Stats.ReclaimedLeases++
		}
	}
	sort.Slice(snap.Leases, func(i, j int) bool {
		return snap.Leases[i].CreatedAt.Before(snap.Leases[j].CreatedAt)
	})

	snap.Events = make([]QuotaLeaseDemoLedgerEvent, 0, len(s.events))
	for _, event := range s.events {
		value := *event
		snap.Events = append(snap.Events, value)
	}
	sort.Slice(snap.Events, func(i, j int) bool {
		return snap.Events[i].CreatedAt.Before(snap.Events[j].CreatedAt)
	})
	snap.Stats.EventCount = len(snap.Events)
	return snap
}

func (s *QuotaLeaseDemoService) nodeLeaseRemainingByNodeLocked(now time.Time) map[string]float64 {
	remainingByNode := make(map[string]float64)
	for _, lease := range s.leases {
		if lease == nil {
			continue
		}
		s.refreshLeaseStatusLocked(lease, now)
		if lease.Status == QuotaLeaseDemoStatusActive {
			remainingByNode[lease.NodeID] += lease.Remaining()
		}
	}
	return remainingByNode
}

func (s *QuotaLeaseDemoService) upsertQuotaLeaseDemoLeaseLocked(lease *QuotaLeaseDemoLease) {
	if s == nil || lease == nil || strings.TrimSpace(lease.ID) == "" {
		return
	}
	if s.leases == nil {
		s.leases = make(map[string]*QuotaLeaseDemoLease)
	}
	if s.leaseIndex == nil {
		s.leaseIndex = make(map[string]map[string]struct{})
	}
	if existing := s.leases[lease.ID]; existing != nil {
		s.removeQuotaLeaseDemoLeaseIndexLocked(existing)
	}
	s.leases[lease.ID] = lease
	key := quotaLeaseDemoLeaseIndexKey(lease.NodeID, lease.UserID, lease.APIKeyID)
	if key == "" {
		return
	}
	bucket := s.leaseIndex[key]
	if bucket == nil {
		bucket = make(map[string]struct{})
		s.leaseIndex[key] = bucket
	}
	bucket[lease.ID] = struct{}{}
}

func (s *QuotaLeaseDemoService) deleteQuotaLeaseDemoLeaseLocked(leaseID string) {
	if s == nil || strings.TrimSpace(leaseID) == "" {
		return
	}
	lease := s.leases[strings.TrimSpace(leaseID)]
	if lease != nil {
		s.removeQuotaLeaseDemoLeaseIndexLocked(lease)
	}
	delete(s.leases, strings.TrimSpace(leaseID))
}

func (s *QuotaLeaseDemoService) removeQuotaLeaseDemoLeaseIndexLocked(lease *QuotaLeaseDemoLease) {
	if s == nil || lease == nil || s.leaseIndex == nil {
		return
	}
	key := quotaLeaseDemoLeaseIndexKey(lease.NodeID, lease.UserID, lease.APIKeyID)
	if key == "" {
		return
	}
	bucket := s.leaseIndex[key]
	if bucket == nil {
		return
	}
	delete(bucket, lease.ID)
	if len(bucket) == 0 {
		delete(s.leaseIndex, key)
	}
}

func (s *QuotaLeaseDemoService) indexedQuotaLeaseDemoLeasesLocked(nodeID string, userID, apiKeyID int64) []*QuotaLeaseDemoLease {
	if s == nil {
		return nil
	}
	if s.leaseIndex == nil || (len(s.leaseIndex) == 0 && len(s.leases) > 0) {
		s.rebuildQuotaLeaseDemoLeaseIndexLocked()
	}
	key := quotaLeaseDemoLeaseIndexKey(nodeID, userID, apiKeyID)
	if key == "" {
		return nil
	}
	ids := s.leaseIndex[key]
	if len(ids) == 0 {
		return nil
	}
	out := make([]*QuotaLeaseDemoLease, 0, len(ids))
	for id := range ids {
		if lease := s.leases[id]; lease != nil {
			out = append(out, lease)
		}
	}
	return out
}

func (s *QuotaLeaseDemoService) rebuildQuotaLeaseDemoLeaseIndexLocked() {
	if s == nil {
		return
	}
	s.leaseIndex = make(map[string]map[string]struct{})
	for _, lease := range s.leases {
		if lease == nil || strings.TrimSpace(lease.ID) == "" {
			continue
		}
		key := quotaLeaseDemoLeaseIndexKey(lease.NodeID, lease.UserID, lease.APIKeyID)
		if key == "" {
			continue
		}
		bucket := s.leaseIndex[key]
		if bucket == nil {
			bucket = make(map[string]struct{})
			s.leaseIndex[key] = bucket
		}
		bucket[lease.ID] = struct{}{}
	}
}

func (s *QuotaLeaseDemoService) hasCapacity(nodeID string, userID, apiKeyID int64, amount float64, now time.Time) bool {
	return s.findLeaseForConsumption(nodeID, userID, apiKeyID, amount, now) != nil
}

func (s *QuotaLeaseDemoService) HasCapacity(nodeID string, userID, apiKeyID int64, amount float64) bool {
	if amount <= 0 && s != nil {
		amount = s.PreflightReserveAmount()
	}
	nodeID = strings.TrimSpace(nodeID)
	if nodeID == "" && s != nil {
		nodeID = s.NodeID()
	}
	return s.hasCapacity(nodeID, userID, apiKeyID, amount, time.Now().UTC())
}

func (s *QuotaLeaseDemoService) usageBillingCapacityTarget(amount float64) float64 {
	target := amount
	defaultGrant := s.DefaultGrantAmount()
	if defaultGrant > target {
		target = defaultGrant
	}
	return target
}

func (s *QuotaLeaseDemoService) preflightCapacityCheckAmount(requestAmount float64) float64 {
	minimum := s.PreflightReserveAmount()
	if requestAmount > 0 && requestAmount < minimum {
		return requestAmount
	}
	return minimum
}

func (s *QuotaLeaseDemoService) findLeaseForConsumption(nodeID string, userID, apiKeyID int64, amount float64, now time.Time) *QuotaLeaseDemoLease {
	if s == nil || amount <= 0 {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	var best *QuotaLeaseDemoLease
	for _, lease := range s.indexedQuotaLeaseDemoLeasesLocked(nodeID, userID, apiKeyID) {
		s.refreshLeaseStatusLocked(lease, now)
		if lease.Status != QuotaLeaseDemoStatusActive {
			continue
		}
		if lease.Remaining()+1e-12 < amount {
			continue
		}
		if best == nil || lease.ExpiresAt.Before(best.ExpiresAt) {
			best = lease
		}
	}
	if best == nil {
		return nil
	}
	return cloneQuotaLeaseDemoLease(best)
}

func (s *QuotaLeaseDemoService) findActiveLeaseForBilling(nodeID string, userID, apiKeyID int64, now time.Time) *QuotaLeaseDemoLease {
	if s == nil {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	var best *QuotaLeaseDemoLease
	for _, lease := range s.indexedQuotaLeaseDemoLeasesLocked(nodeID, userID, apiKeyID) {
		s.refreshLeaseStatusLocked(lease, now)
		if lease.Status != QuotaLeaseDemoStatusActive {
			continue
		}
		if best == nil ||
			lease.Remaining() > best.Remaining()+1e-12 ||
			(math.Abs(lease.Remaining()-best.Remaining()) <= 1e-12 && lease.ExpiresAt.Before(best.ExpiresAt)) {
			best = lease
		}
	}
	return cloneQuotaLeaseDemoLease(best)
}

func (s *QuotaLeaseDemoService) refreshLeaseStatusLocked(lease *QuotaLeaseDemoLease, now time.Time) {
	convergeQuotaLeaseDemoLeaseState(lease, now)
}

func cloneQuotaLeaseDemoLease(lease *QuotaLeaseDemoLease) *QuotaLeaseDemoLease {
	if lease == nil {
		return nil
	}
	value := *lease
	return &value
}

func cloneQuotaLeaseDemoNode(node *QuotaLeaseDemoNode) *QuotaLeaseDemoNode {
	if node == nil {
		return nil
	}
	value := *node
	value.Metadata = cloneQuotaLeaseDemoStringMap(node.Metadata)
	value.Metrics = cloneQuotaLeaseDemoFloatMap(node.Metrics)
	value.SyncStatus = cloneQuotaLeaseDemoNodeSyncStatus(node.SyncStatus)
	if node.LastHeartbeatAt != nil {
		heartbeat := *node.LastHeartbeatAt
		value.LastHeartbeatAt = &heartbeat
	}
	return &value
}

func cloneQuotaLeaseDemoNodeSyncStatus(status *QuotaLeaseDemoNodeSyncStatus) *QuotaLeaseDemoNodeSyncStatus {
	if status == nil {
		return nil
	}
	value := *status
	value.MirrorSyncedAt = cloneQuotaLeaseDemoTimePtr(status.MirrorSyncedAt)
	value.LastSyncStartedAt = cloneQuotaLeaseDemoTimePtr(status.LastSyncStartedAt)
	value.LastSyncSuccessAt = cloneQuotaLeaseDemoTimePtr(status.LastSyncSuccessAt)
	value.LastSyncFailedAt = cloneQuotaLeaseDemoTimePtr(status.LastSyncFailedAt)
	return &value
}

func cloneQuotaLeaseDemoNodeRegistrationRequest(req QuotaLeaseDemoNodeRegistrationRequest) QuotaLeaseDemoNodeRegistrationRequest {
	req.NodeID = strings.TrimSpace(req.NodeID)
	req.NodeSecret = strings.TrimSpace(req.NodeSecret)
	req.Region = strings.TrimSpace(req.Region)
	req.BaseURL = strings.TrimSpace(req.BaseURL)
	req.PublicKey = strings.TrimSpace(req.PublicKey)
	req.RegistrationToken = strings.TrimSpace(req.RegistrationToken)
	req.Metadata = cloneQuotaLeaseDemoStringMap(req.Metadata)
	return req
}

func cloneQuotaLeaseDemoNodeRegistrationURL(item *QuotaLeaseDemoNodeRegistrationURL) *QuotaLeaseDemoNodeRegistrationURL {
	if item == nil {
		return nil
	}
	value := *item
	value.Request = cloneQuotaLeaseDemoNodeRegistrationRequest(item.Request)
	return &value
}

func cloneQuotaLeaseDemoStringMap(src map[string]string) map[string]string {
	if len(src) == 0 {
		return nil
	}
	dst := make(map[string]string, len(src))
	for k, v := range src {
		key := strings.TrimSpace(k)
		if key == "" {
			continue
		}
		dst[key] = strings.TrimSpace(v)
	}
	if len(dst) == 0 {
		return nil
	}
	return dst
}

func cloneQuotaLeaseDemoFloatMap(src map[string]float64) map[string]float64 {
	if len(src) == 0 {
		return nil
	}
	dst := make(map[string]float64, len(src))
	for k, v := range src {
		key := strings.TrimSpace(k)
		if key == "" || math.IsNaN(v) || math.IsInf(v, 0) {
			continue
		}
		dst[key] = v
	}
	if len(dst) == 0 {
		return nil
	}
	return dst
}

func generateQuotaLeaseDemoNodeSecret() (string, error) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return "qln_" + base64.RawURLEncoding.EncodeToString(buf), nil
}

func generateQuotaLeaseDemoRegistrationToken() (string, error) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return "qlr_" + base64.RawURLEncoding.EncodeToString(buf), nil
}

func quotaLeaseDemoUsageEventID(nodeID, leaseID, requestID string) string {
	raw := strings.Join([]string{
		strings.TrimSpace(nodeID),
		strings.TrimSpace(leaseID),
		strings.TrimSpace(requestID),
		QuotaLeaseDemoEventUsagePosted,
	}, "|")
	sum := sha256.Sum256([]byte(raw))
	return "usage:" + hex.EncodeToString(sum[:])
}

func quotaLeaseDemoLeaseRequestGroupKey(nodeID string, userID, apiKeyID int64) string {
	return fmt.Sprintf("%s:%d:%d", strings.TrimSpace(nodeID), userID, apiKeyID)
}

func quotaLeaseDemoLeaseIndexKey(nodeID string, userID, apiKeyID int64) string {
	nodeID = strings.TrimSpace(nodeID)
	if nodeID == "" || userID <= 0 || apiKeyID <= 0 {
		return ""
	}
	return fmt.Sprintf("%s:%d:%d", nodeID, userID, apiKeyID)
}

func quotaLeaseDemoContextRequestID(ctx context.Context) string {
	if ctx == nil {
		return ""
	}
	if value, ok := ctx.Value(ctxkey.RequestID).(string); ok {
		return strings.TrimSpace(value)
	}
	return ""
}

func quotaLeaseDemoTraceID(candidate, nodeID string, userID, apiKeyID int64, requestID string) string {
	candidate = strings.TrimSpace(candidate)
	if candidate != "" {
		return candidate
	}
	requestID = strings.TrimSpace(requestID)
	if requestID == "" {
		return "qlt_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	}
	raw := fmt.Sprintf("%s|%d|%d|%s", strings.TrimSpace(nodeID), userID, apiKeyID, requestID)
	sum := sha256.Sum256([]byte(raw))
	return "qlt_" + hex.EncodeToString(sum[:12])
}

func (s *QuotaLeaseDemoService) logQuotaLeaseDemoTrace(stage, traceID, requestID, leaseID, nodeID string, userID, apiKeyID int64, extra map[string]any) {
	traceID = strings.TrimSpace(traceID)
	if traceID == "" {
		return
	}
	fields := []any{
		"stage", strings.TrimSpace(stage),
		"trace_id", traceID,
		"request_id", strings.TrimSpace(requestID),
		"lease_id", strings.TrimSpace(leaseID),
		"node_id", strings.TrimSpace(nodeID),
		"user_id", userID,
		"api_key_id", apiKeyID,
	}
	for key, value := range extra {
		key = strings.TrimSpace(key)
		if key == "" {
			continue
		}
		fields = append(fields, key, value)
	}
	slog.Info("quota_lease_demo.trace", fields...)
}

func nextQuotaLeaseDemoLeaseVersion(current int64) int64 {
	if current < 0 {
		return 1
	}
	return current + 1
}

func advanceQuotaLeaseDemoLeaseVersion(lease *QuotaLeaseDemoLease) {
	if lease == nil {
		return
	}
	lease.Version = nextQuotaLeaseDemoLeaseVersion(lease.Version)
}

func quotaLeaseDemoPayloadHash(leaseID, nodeID string, userID, apiKeyID int64, requestID string, amount float64, eventType string) string {
	raw := fmt.Sprintf("%s|%s|%d|%d|%s|%0.10f|%s",
		strings.TrimSpace(leaseID),
		strings.TrimSpace(nodeID),
		userID,
		apiKeyID,
		strings.TrimSpace(requestID),
		amount,
		strings.TrimSpace(eventType),
	)
	sum := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(sum[:])
}

func finitePositive(value float64) bool {
	return value > 0 && !math.IsNaN(value) && !math.IsInf(value, 0)
}
