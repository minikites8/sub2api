package service

import (
	"context"
	"net/http"
	"strings"
	"time"
)

const quotaLeaseDemoMirrorFreshness = 5 * time.Second

type QuotaLeaseDemoGroupSnapshot struct {
	ID                              int64                             `json:"id"`
	Name                            string                            `json:"name"`
	Description                     string                            `json:"description,omitempty"`
	Platform                        string                            `json:"platform"`
	RateMultiplier                  float64                           `json:"rate_multiplier"`
	PeakRateEnabled                 bool                              `json:"peak_rate_enabled"`
	PeakStart                       string                            `json:"peak_start,omitempty"`
	PeakEnd                         string                            `json:"peak_end,omitempty"`
	PeakRateMultiplier              float64                           `json:"peak_rate_multiplier"`
	IsExclusive                     bool                              `json:"is_exclusive"`
	Status                          string                            `json:"status"`
	SubscriptionType                string                            `json:"subscription_type"`
	DailyLimitUSD                   *float64                          `json:"daily_limit_usd,omitempty"`
	WeeklyLimitUSD                  *float64                          `json:"weekly_limit_usd,omitempty"`
	MonthlyLimitUSD                 *float64                          `json:"monthly_limit_usd,omitempty"`
	DefaultValidityDays             int                               `json:"default_validity_days"`
	AllowImageGeneration            bool                              `json:"allow_image_generation"`
	AllowBatchImageGeneration       bool                              `json:"allow_batch_image_generation"`
	ImageRateIndependent            bool                              `json:"image_rate_independent"`
	ImageRateMultiplier             float64                           `json:"image_rate_multiplier"`
	ImagePrice1K                    *float64                          `json:"image_price_1k,omitempty"`
	ImagePrice2K                    *float64                          `json:"image_price_2k,omitempty"`
	ImagePrice4K                    *float64                          `json:"image_price_4k,omitempty"`
	BatchImageDiscountMultiplier    float64                           `json:"batch_image_discount_multiplier"`
	BatchImageHoldMultiplier        float64                           `json:"batch_image_hold_multiplier"`
	VideoRateIndependent            bool                              `json:"video_rate_independent"`
	VideoRateMultiplier             float64                           `json:"video_rate_multiplier"`
	VideoPrice480P                  *float64                          `json:"video_price_480p,omitempty"`
	VideoPrice720P                  *float64                          `json:"video_price_720p,omitempty"`
	VideoPrice1080P                 *float64                          `json:"video_price_1080p,omitempty"`
	WebSearchPricePerCall           *float64                          `json:"web_search_price_per_call,omitempty"`
	ClaudeCodeOnly                  bool                              `json:"claude_code_only"`
	FallbackGroupID                 *int64                            `json:"fallback_group_id,omitempty"`
	FallbackGroupIDOnInvalidRequest *int64                            `json:"fallback_group_id_on_invalid_request,omitempty"`
	ModelRouting                    map[string][]int64                `json:"model_routing,omitempty"`
	ModelRoutingEnabled             bool                              `json:"model_routing_enabled"`
	MCPXMLInject                    bool                              `json:"mcp_xml_inject"`
	SupportedModelScopes            []string                          `json:"supported_model_scopes,omitempty"`
	SortOrder                       int                               `json:"sort_order"`
	AllowMessagesDispatch           bool                              `json:"allow_messages_dispatch"`
	RequireOAuthOnly                bool                              `json:"require_oauth_only"`
	RequirePrivacySet               bool                              `json:"require_privacy_set"`
	DefaultMappedModel              string                            `json:"default_mapped_model,omitempty"`
	MessagesDispatchModelConfig     OpenAIMessagesDispatchModelConfig `json:"messages_dispatch_model_config,omitempty"`
	ModelsListConfig                GroupModelsListConfig             `json:"models_list_config,omitempty"`
	RPMLimit                        int                               `json:"rpm_limit"`
	KiroCacheEmulationEnabled       bool                              `json:"kiro_cache_emulation_enabled"`
	KiroAutoStickyEnabled           bool                              `json:"kiro_auto_sticky_enabled"`
	KiroStickySessionTTLSeconds     int                               `json:"kiro_sticky_session_ttl_seconds"`
	KiroCacheEmulationRatio         float64                           `json:"kiro_cache_emulation_ratio"`
	KiroEndpointMode                string                            `json:"kiro_endpoint_mode,omitempty"`
	CreatedAt                       time.Time                         `json:"created_at"`
	UpdatedAt                       time.Time                         `json:"updated_at"`
}

type QuotaLeaseDemoMirrorSnapshot struct {
	NodeID        string                               `json:"node_id,omitempty"`
	SyncedAt      time.Time                            `json:"synced_at"`
	Groups        []QuotaLeaseDemoGroupSnapshot        `json:"groups"`
	Proxies       []QuotaLeaseDemoProxySnapshot        `json:"proxies"`
	Accounts      []QuotaLeaseDemoAccountSnapshot      `json:"accounts"`
	AccountGroups []QuotaLeaseDemoAccountGroupSnapshot `json:"account_groups"`
}

type QuotaLeaseDemoMirrorStore interface {
	ApplySnapshot(ctx context.Context, snapshot QuotaLeaseDemoMirrorSnapshot) error
	UpsertAccount(ctx context.Context, account QuotaLeaseDemoAccountSnapshot) error
	ListSchedulableAccounts(ctx context.Context, groupID *int64, platform string) ([]Account, error)
	GetAccountByID(ctx context.Context, accountID int64) (*Account, error)
}

func QuotaLeaseDemoGroupSnapshotToGroup(snapshot QuotaLeaseDemoGroupSnapshot) Group {
	group := Group{
		ID:                              snapshot.ID,
		Name:                            snapshot.Name,
		Description:                     snapshot.Description,
		Platform:                        snapshot.Platform,
		RateMultiplier:                  snapshot.RateMultiplier,
		PeakRateEnabled:                 snapshot.PeakRateEnabled,
		PeakStart:                       snapshot.PeakStart,
		PeakEnd:                         snapshot.PeakEnd,
		PeakRateMultiplier:              snapshot.PeakRateMultiplier,
		IsExclusive:                     snapshot.IsExclusive,
		Status:                          snapshot.Status,
		SubscriptionType:                snapshot.SubscriptionType,
		DailyLimitUSD:                   snapshot.DailyLimitUSD,
		WeeklyLimitUSD:                  snapshot.WeeklyLimitUSD,
		MonthlyLimitUSD:                 snapshot.MonthlyLimitUSD,
		DefaultValidityDays:             snapshot.DefaultValidityDays,
		AllowImageGeneration:            snapshot.AllowImageGeneration,
		AllowBatchImageGeneration:       snapshot.AllowBatchImageGeneration,
		ImageRateIndependent:            snapshot.ImageRateIndependent,
		ImageRateMultiplier:             snapshot.ImageRateMultiplier,
		ImagePrice1K:                    snapshot.ImagePrice1K,
		ImagePrice2K:                    snapshot.ImagePrice2K,
		ImagePrice4K:                    snapshot.ImagePrice4K,
		BatchImageDiscountMultiplier:    snapshot.BatchImageDiscountMultiplier,
		BatchImageHoldMultiplier:        snapshot.BatchImageHoldMultiplier,
		VideoRateIndependent:            snapshot.VideoRateIndependent,
		VideoRateMultiplier:             snapshot.VideoRateMultiplier,
		VideoPrice480P:                  snapshot.VideoPrice480P,
		VideoPrice720P:                  snapshot.VideoPrice720P,
		VideoPrice1080P:                 snapshot.VideoPrice1080P,
		WebSearchPricePerCall:           snapshot.WebSearchPricePerCall,
		ClaudeCodeOnly:                  snapshot.ClaudeCodeOnly,
		FallbackGroupID:                 snapshot.FallbackGroupID,
		FallbackGroupIDOnInvalidRequest: snapshot.FallbackGroupIDOnInvalidRequest,
		ModelRouting:                    snapshot.ModelRouting,
		ModelRoutingEnabled:             snapshot.ModelRoutingEnabled,
		MCPXMLInject:                    snapshot.MCPXMLInject,
		SupportedModelScopes:            snapshot.SupportedModelScopes,
		SortOrder:                       snapshot.SortOrder,
		AllowMessagesDispatch:           snapshot.AllowMessagesDispatch,
		RequireOAuthOnly:                snapshot.RequireOAuthOnly,
		RequirePrivacySet:               snapshot.RequirePrivacySet,
		DefaultMappedModel:              snapshot.DefaultMappedModel,
		MessagesDispatchModelConfig:     snapshot.MessagesDispatchModelConfig,
		ModelsListConfig:                snapshot.ModelsListConfig,
		RPMLimit:                        snapshot.RPMLimit,
		KiroCacheEmulationEnabled:       snapshot.KiroCacheEmulationEnabled,
		KiroAutoStickyEnabled:           snapshot.KiroAutoStickyEnabled,
		KiroStickySessionTTLSeconds:     snapshot.KiroStickySessionTTLSeconds,
		KiroCacheEmulationRatio:         snapshot.KiroCacheEmulationRatio,
		KiroEndpointMode:                snapshot.KiroEndpointMode,
		CreatedAt:                       snapshot.CreatedAt,
		UpdatedAt:                       snapshot.UpdatedAt,
	}
	NormalizeGroupRuntimeFields(&group)
	return group
}

func (s *QuotaLeaseDemoService) SetMirrorStore(store QuotaLeaseDemoMirrorStore) {
	if s == nil {
		return
	}
	s.remoteMu.Lock()
	s.mirrorStore = store
	s.remoteMu.Unlock()
}

func (s *QuotaLeaseDemoService) quotaLeaseDemoMirrorStore() QuotaLeaseDemoMirrorStore {
	if s == nil {
		return nil
	}
	s.remoteMu.Lock()
	defer s.remoteMu.Unlock()
	return s.mirrorStore
}

func (s *QuotaLeaseDemoService) EnsureMirrorSnapshot(ctx context.Context) error {
	if s == nil || !s.remoteMode() || s.quotaLeaseDemoMirrorStore() == nil {
		return nil
	}
	if s.mirrorSnapshotFresh(time.Now().UTC()) {
		return nil
	}
	return s.SyncMirrorSnapshot(ctx)
}

func (s *QuotaLeaseDemoService) SyncMirrorSnapshot(ctx context.Context) error {
	if s == nil || !s.remoteMode() {
		return nil
	}
	store := s.quotaLeaseDemoMirrorStore()
	if store == nil {
		return nil
	}
	nodeID, secret, err := s.remoteNodeAuth(ctx)
	if err != nil {
		return err
	}
	var result struct {
		Snapshot QuotaLeaseDemoMirrorSnapshot `json:"snapshot"`
	}
	if err := s.doRemoteJSON(ctx, http.MethodGet, "/mirror/snapshot", nodeID, secret, nil, &result); err != nil {
		return err
	}
	snapshot := result.Snapshot
	if strings.TrimSpace(snapshot.NodeID) == "" {
		snapshot.NodeID = nodeID
	}
	if snapshot.SyncedAt.IsZero() {
		snapshot.SyncedAt = time.Now().UTC()
	}
	if err := store.ApplySnapshot(ctx, snapshot); err != nil {
		return err
	}
	s.markMirrorSynced(snapshot.SyncedAt)
	return nil
}

func (s *QuotaLeaseDemoService) markMirrorSynced(syncedAt time.Time) {
	if s == nil {
		return
	}
	if syncedAt.IsZero() {
		syncedAt = time.Now().UTC()
	}
	s.remoteMu.Lock()
	s.mirrorReady = true
	s.mirrorSyncedAt = syncedAt.UTC()
	s.remoteMu.Unlock()
}

func (s *QuotaLeaseDemoService) mirrorSnapshotFresh(now time.Time) bool {
	if s == nil {
		return false
	}
	if now.IsZero() {
		now = time.Now().UTC()
	}
	s.remoteMu.Lock()
	ready := s.mirrorReady
	syncedAt := s.mirrorSyncedAt
	s.remoteMu.Unlock()
	return ready && !syncedAt.IsZero() && now.Sub(syncedAt) < quotaLeaseDemoMirrorFreshness
}

func (s *QuotaLeaseDemoService) mirrorSnapshotState() (bool, time.Time) {
	if s == nil {
		return false, time.Time{}
	}
	s.remoteMu.Lock()
	defer s.remoteMu.Unlock()
	return s.mirrorReady, s.mirrorSyncedAt
}
