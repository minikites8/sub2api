package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"
)

const (
	quotaLeaseDemoMirrorFreshness           = 5 * time.Second
	quotaLeaseDemoMirrorVersionHistoryLimit = 16
	quotaLeaseDemoMirrorSyncModeFull        = "full"
	quotaLeaseDemoMirrorSyncModeDelta       = "delta"
)

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

type QuotaLeaseDemoChannelSnapshot struct {
	ID                         int64                                       `json:"id"`
	Name                       string                                      `json:"name"`
	Description                string                                      `json:"description,omitempty"`
	Status                     string                                      `json:"status"`
	BillingModelSource         string                                      `json:"billing_model_source,omitempty"`
	RestrictModels             bool                                        `json:"restrict_models"`
	Features                   string                                      `json:"features,omitempty"`
	FeaturesConfig             map[string]any                              `json:"features_config,omitempty"`
	ApplyPricingToAccountStats bool                                        `json:"apply_pricing_to_account_stats"`
	GroupIDs                   []int64                                     `json:"group_ids,omitempty"`
	ModelMapping               map[string]map[string]string                `json:"model_mapping,omitempty"`
	ModelPricing               []QuotaLeaseDemoChannelModelPricingSnapshot `json:"model_pricing,omitempty"`
	CreatedAt                  time.Time                                   `json:"created_at"`
	UpdatedAt                  time.Time                                   `json:"updated_at"`
}

type QuotaLeaseDemoChannelModelPricingSnapshot struct {
	ID                 int64                                   `json:"id"`
	ChannelID          int64                                   `json:"channel_id"`
	Platform           string                                  `json:"platform,omitempty"`
	Models             []string                                `json:"models,omitempty"`
	BillingMode        BillingMode                             `json:"billing_mode,omitempty"`
	InputPrice         *float64                                `json:"input_price,omitempty"`
	OutputPrice        *float64                                `json:"output_price,omitempty"`
	CacheWritePrice    *float64                                `json:"cache_write_price,omitempty"`
	CacheReadPrice     *float64                                `json:"cache_read_price,omitempty"`
	ImageInputPrice    *float64                                `json:"image_input_price,omitempty"`
	ImageOutputPrice   *float64                                `json:"image_output_price,omitempty"`
	PerRequestPrice    *float64                                `json:"per_request_price,omitempty"`
	PriorityMultiplier *float64                                `json:"priority_multiplier,omitempty"`
	Intervals          []QuotaLeaseDemoPricingIntervalSnapshot `json:"intervals,omitempty"`
	CreatedAt          time.Time                               `json:"created_at"`
	UpdatedAt          time.Time                               `json:"updated_at"`
}

type QuotaLeaseDemoPricingIntervalSnapshot struct {
	ID              int64     `json:"id"`
	PricingID       int64     `json:"pricing_id"`
	MinTokens       int       `json:"min_tokens"`
	MaxTokens       *int      `json:"max_tokens,omitempty"`
	TierLabel       string    `json:"tier_label,omitempty"`
	InputPrice      *float64  `json:"input_price,omitempty"`
	OutputPrice     *float64  `json:"output_price,omitempty"`
	CacheWritePrice *float64  `json:"cache_write_price,omitempty"`
	CacheReadPrice  *float64  `json:"cache_read_price,omitempty"`
	PerRequestPrice *float64  `json:"per_request_price,omitempty"`
	SortOrder       int       `json:"sort_order"`
	CreatedAt       time.Time `json:"created_at"`
	UpdatedAt       time.Time `json:"updated_at"`
}

type QuotaLeaseDemoMirrorSnapshot struct {
	NodeID            string                               `json:"node_id,omitempty"`
	Version           int64                                `json:"version"`
	BaseVersion       int64                                `json:"base_version,omitempty"`
	Delta             bool                                 `json:"delta,omitempty"`
	SyncedAt          time.Time                            `json:"synced_at"`
	TotalGroupCount   int                                  `json:"total_group_count,omitempty"`
	TotalChannelCount int                                  `json:"total_channel_count,omitempty"`
	TotalProxyCount   int                                  `json:"total_proxy_count,omitempty"`
	TotalAccountCount int                                  `json:"total_account_count,omitempty"`
	TotalAPIKeyCount  int                                  `json:"total_api_key_count,omitempty"`
	Groups            []QuotaLeaseDemoGroupSnapshot        `json:"groups"`
	Channels          []QuotaLeaseDemoChannelSnapshot      `json:"channels"`
	Proxies           []QuotaLeaseDemoProxySnapshot        `json:"proxies"`
	Accounts          []QuotaLeaseDemoAccountSnapshot      `json:"accounts"`
	AccountGroups     []QuotaLeaseDemoAccountGroupSnapshot `json:"account_groups"`
	APIKeys           []QuotaLeaseDemoAPIKeySnapshot       `json:"api_keys"`
	DeletedGroupIDs   []int64                              `json:"deleted_group_ids,omitempty"`
	DeletedChannelIDs []int64                              `json:"deleted_channel_ids,omitempty"`
	DeletedProxyIDs   []int64                              `json:"deleted_proxy_ids,omitempty"`
	DeletedAccountIDs []int64                              `json:"deleted_account_ids,omitempty"`
	DeletedAPIKeyIDs  []int64                              `json:"deleted_api_key_ids,omitempty"`
}

type QuotaLeaseDemoAPIKeySnapshot struct {
	ID        int64              `json:"id"`
	Key       string             `json:"key"`
	Snapshot  APIKeyAuthSnapshot `json:"snapshot"`
	CreatedAt time.Time          `json:"created_at,omitempty"`
	UpdatedAt time.Time          `json:"updated_at,omitempty"`
}

func NewQuotaLeaseDemoAPIKeySnapshot(key string, snapshot *APIKeyAuthSnapshot) QuotaLeaseDemoAPIKeySnapshot {
	out := QuotaLeaseDemoAPIKeySnapshot{
		Key: strings.TrimSpace(key),
	}
	if snapshot != nil {
		out.ID = snapshot.APIKeyID
		out.Snapshot = quotaLeaseDemoMirrorJSONClone(*snapshot)
	}
	return out
}

type quotaLeaseDemoMirrorVersionState struct {
	Version   int64
	Hash      string
	Snapshot  QuotaLeaseDemoMirrorSnapshot
	History   []QuotaLeaseDemoMirrorSnapshot
	UpdatedAt time.Time
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

func NewQuotaLeaseDemoChannelSnapshot(channel Channel) QuotaLeaseDemoChannelSnapshot {
	snapshot := QuotaLeaseDemoChannelSnapshot{
		ID:                         channel.ID,
		Name:                       strings.TrimSpace(channel.Name),
		Description:                strings.TrimSpace(channel.Description),
		Status:                     strings.TrimSpace(channel.Status),
		BillingModelSource:         strings.TrimSpace(channel.BillingModelSource),
		RestrictModels:             channel.RestrictModels,
		Features:                   strings.TrimSpace(channel.Features),
		FeaturesConfig:             cloneQuotaLeaseDemoAnyMap(channel.FeaturesConfig),
		ApplyPricingToAccountStats: channel.ApplyPricingToAccountStats,
		GroupIDs:                   cloneQuotaLeaseDemoInt64Slice(channel.GroupIDs),
		ModelMapping:               cloneQuotaLeaseDemoStringMapMap(channel.ModelMapping),
		ModelPricing:               make([]QuotaLeaseDemoChannelModelPricingSnapshot, 0, len(channel.ModelPricing)),
		CreatedAt:                  channel.CreatedAt,
		UpdatedAt:                  channel.UpdatedAt,
	}
	for _, pricing := range channel.ModelPricing {
		snapshot.ModelPricing = append(snapshot.ModelPricing, NewQuotaLeaseDemoChannelModelPricingSnapshot(pricing))
	}
	if len(snapshot.ModelPricing) == 0 {
		snapshot.ModelPricing = nil
	}
	return snapshot
}

func NewQuotaLeaseDemoChannelModelPricingSnapshot(pricing ChannelModelPricing) QuotaLeaseDemoChannelModelPricingSnapshot {
	snapshot := QuotaLeaseDemoChannelModelPricingSnapshot{
		ID:                 pricing.ID,
		ChannelID:          pricing.ChannelID,
		Platform:           strings.TrimSpace(pricing.Platform),
		Models:             cloneQuotaLeaseDemoStringSlice(pricing.Models),
		BillingMode:        pricing.BillingMode,
		InputPrice:         cloneQuotaLeaseDemoFloat64Ptr(pricing.InputPrice),
		OutputPrice:        cloneQuotaLeaseDemoFloat64Ptr(pricing.OutputPrice),
		CacheWritePrice:    cloneQuotaLeaseDemoFloat64Ptr(pricing.CacheWritePrice),
		CacheReadPrice:     cloneQuotaLeaseDemoFloat64Ptr(pricing.CacheReadPrice),
		ImageInputPrice:    cloneQuotaLeaseDemoFloat64Ptr(pricing.ImageInputPrice),
		ImageOutputPrice:   cloneQuotaLeaseDemoFloat64Ptr(pricing.ImageOutputPrice),
		PerRequestPrice:    cloneQuotaLeaseDemoFloat64Ptr(pricing.PerRequestPrice),
		PriorityMultiplier: cloneQuotaLeaseDemoFloat64Ptr(pricing.PriorityMultiplier),
		Intervals:          make([]QuotaLeaseDemoPricingIntervalSnapshot, 0, len(pricing.Intervals)),
		CreatedAt:          pricing.CreatedAt,
		UpdatedAt:          pricing.UpdatedAt,
	}
	for _, interval := range pricing.Intervals {
		snapshot.Intervals = append(snapshot.Intervals, NewQuotaLeaseDemoPricingIntervalSnapshot(interval))
	}
	if len(snapshot.Intervals) == 0 {
		snapshot.Intervals = nil
	}
	return snapshot
}

func NewQuotaLeaseDemoPricingIntervalSnapshot(interval PricingInterval) QuotaLeaseDemoPricingIntervalSnapshot {
	return QuotaLeaseDemoPricingIntervalSnapshot{
		ID:              interval.ID,
		PricingID:       interval.PricingID,
		MinTokens:       interval.MinTokens,
		MaxTokens:       cloneQuotaLeaseDemoIntPtr(interval.MaxTokens),
		TierLabel:       strings.TrimSpace(interval.TierLabel),
		InputPrice:      cloneQuotaLeaseDemoFloat64Ptr(interval.InputPrice),
		OutputPrice:     cloneQuotaLeaseDemoFloat64Ptr(interval.OutputPrice),
		CacheWritePrice: cloneQuotaLeaseDemoFloat64Ptr(interval.CacheWritePrice),
		CacheReadPrice:  cloneQuotaLeaseDemoFloat64Ptr(interval.CacheReadPrice),
		PerRequestPrice: cloneQuotaLeaseDemoFloat64Ptr(interval.PerRequestPrice),
		SortOrder:       interval.SortOrder,
		CreatedAt:       interval.CreatedAt,
		UpdatedAt:       interval.UpdatedAt,
	}
}

func QuotaLeaseDemoChannelSnapshotToChannel(snapshot QuotaLeaseDemoChannelSnapshot) Channel {
	channel := Channel{
		ID:                         snapshot.ID,
		Name:                       strings.TrimSpace(snapshot.Name),
		Description:                strings.TrimSpace(snapshot.Description),
		Status:                     strings.TrimSpace(snapshot.Status),
		BillingModelSource:         strings.TrimSpace(snapshot.BillingModelSource),
		RestrictModels:             snapshot.RestrictModels,
		Features:                   strings.TrimSpace(snapshot.Features),
		FeaturesConfig:             cloneQuotaLeaseDemoAnyMap(snapshot.FeaturesConfig),
		ApplyPricingToAccountStats: snapshot.ApplyPricingToAccountStats,
		GroupIDs:                   cloneQuotaLeaseDemoInt64Slice(snapshot.GroupIDs),
		ModelMapping:               cloneQuotaLeaseDemoStringMapMap(snapshot.ModelMapping),
		ModelPricing:               make([]ChannelModelPricing, 0, len(snapshot.ModelPricing)),
		CreatedAt:                  snapshot.CreatedAt,
		UpdatedAt:                  snapshot.UpdatedAt,
	}
	for _, pricing := range snapshot.ModelPricing {
		item := QuotaLeaseDemoChannelModelPricingSnapshotToPricing(pricing)
		if item.ChannelID == 0 {
			item.ChannelID = channel.ID
		}
		channel.ModelPricing = append(channel.ModelPricing, item)
	}
	if len(channel.ModelPricing) == 0 {
		channel.ModelPricing = nil
	}
	return channel
}

func QuotaLeaseDemoChannelModelPricingSnapshotToPricing(snapshot QuotaLeaseDemoChannelModelPricingSnapshot) ChannelModelPricing {
	pricing := ChannelModelPricing{
		ID:                 snapshot.ID,
		ChannelID:          snapshot.ChannelID,
		Platform:           strings.TrimSpace(snapshot.Platform),
		Models:             cloneQuotaLeaseDemoStringSlice(snapshot.Models),
		BillingMode:        snapshot.BillingMode,
		InputPrice:         cloneQuotaLeaseDemoFloat64Ptr(snapshot.InputPrice),
		OutputPrice:        cloneQuotaLeaseDemoFloat64Ptr(snapshot.OutputPrice),
		CacheWritePrice:    cloneQuotaLeaseDemoFloat64Ptr(snapshot.CacheWritePrice),
		CacheReadPrice:     cloneQuotaLeaseDemoFloat64Ptr(snapshot.CacheReadPrice),
		ImageInputPrice:    cloneQuotaLeaseDemoFloat64Ptr(snapshot.ImageInputPrice),
		ImageOutputPrice:   cloneQuotaLeaseDemoFloat64Ptr(snapshot.ImageOutputPrice),
		PerRequestPrice:    cloneQuotaLeaseDemoFloat64Ptr(snapshot.PerRequestPrice),
		PriorityMultiplier: cloneQuotaLeaseDemoFloat64Ptr(snapshot.PriorityMultiplier),
		Intervals:          make([]PricingInterval, 0, len(snapshot.Intervals)),
		CreatedAt:          snapshot.CreatedAt,
		UpdatedAt:          snapshot.UpdatedAt,
	}
	for _, interval := range snapshot.Intervals {
		item := QuotaLeaseDemoPricingIntervalSnapshotToInterval(interval)
		if item.PricingID == 0 {
			item.PricingID = pricing.ID
		}
		pricing.Intervals = append(pricing.Intervals, item)
	}
	if len(pricing.Intervals) == 0 {
		pricing.Intervals = nil
	}
	return pricing
}

func QuotaLeaseDemoPricingIntervalSnapshotToInterval(snapshot QuotaLeaseDemoPricingIntervalSnapshot) PricingInterval {
	return PricingInterval{
		ID:              snapshot.ID,
		PricingID:       snapshot.PricingID,
		MinTokens:       snapshot.MinTokens,
		MaxTokens:       cloneQuotaLeaseDemoIntPtr(snapshot.MaxTokens),
		TierLabel:       strings.TrimSpace(snapshot.TierLabel),
		InputPrice:      cloneQuotaLeaseDemoFloat64Ptr(snapshot.InputPrice),
		OutputPrice:     cloneQuotaLeaseDemoFloat64Ptr(snapshot.OutputPrice),
		CacheWritePrice: cloneQuotaLeaseDemoFloat64Ptr(snapshot.CacheWritePrice),
		CacheReadPrice:  cloneQuotaLeaseDemoFloat64Ptr(snapshot.CacheReadPrice),
		PerRequestPrice: cloneQuotaLeaseDemoFloat64Ptr(snapshot.PerRequestPrice),
		SortOrder:       snapshot.SortOrder,
		CreatedAt:       snapshot.CreatedAt,
		UpdatedAt:       snapshot.UpdatedAt,
	}
}

func cloneQuotaLeaseDemoStringSlice(src []string) []string {
	if len(src) == 0 {
		return nil
	}
	dst := make([]string, 0, len(src))
	for _, value := range src {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		dst = append(dst, value)
	}
	if len(dst) == 0 {
		return nil
	}
	return dst
}

func cloneQuotaLeaseDemoStringMapMap(src map[string]map[string]string) map[string]map[string]string {
	if len(src) == 0 {
		return nil
	}
	dst := make(map[string]map[string]string, len(src))
	for platform, mapping := range src {
		platform = strings.TrimSpace(platform)
		if platform == "" || len(mapping) == 0 {
			continue
		}
		inner := make(map[string]string, len(mapping))
		for from, to := range mapping {
			from = strings.TrimSpace(from)
			to = strings.TrimSpace(to)
			if from == "" || to == "" {
				continue
			}
			inner[from] = to
		}
		if len(inner) > 0 {
			dst[platform] = inner
		}
	}
	if len(dst) == 0 {
		return nil
	}
	return dst
}

func (s *QuotaLeaseDemoService) SetMirrorStore(store QuotaLeaseDemoMirrorStore) {
	if s == nil {
		return
	}
	s.remoteMu.Lock()
	s.mirrorStore = store
	s.remoteMu.Unlock()
}

func (s *QuotaLeaseDemoService) SetChannelService(channelService *ChannelService) {
	if s == nil {
		return
	}
	s.remoteMu.Lock()
	s.channelService = channelService
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

func (s *QuotaLeaseDemoService) quotaLeaseDemoChannelService() *ChannelService {
	if s == nil {
		return nil
	}
	s.remoteMu.Lock()
	defer s.remoteMu.Unlock()
	return s.channelService
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

func (s *QuotaLeaseDemoService) SyncMirrorSnapshot(ctx context.Context) (err error) {
	if s == nil || !s.remoteMode() {
		return nil
	}
	store := s.quotaLeaseDemoMirrorStore()
	if store == nil {
		return nil
	}
	s.markNodeSyncStarted(time.Now().UTC())
	defer func() {
		if err != nil {
			s.markNodeSyncFailed(err, time.Now().UTC())
		}
	}()
	nodeID, secret, err := s.remoteNodeAuth(ctx)
	if err != nil {
		return err
	}
	endpoint := "/mirror/snapshot"
	if sinceVersion := s.currentMirrorVersion(); sinceVersion > 0 {
		query := url.Values{}
		query.Set("since_version", strconv.FormatInt(sinceVersion, 10))
		endpoint += "?" + query.Encode()
	}
	var result struct {
		Snapshot QuotaLeaseDemoMirrorSnapshot `json:"snapshot"`
	}
	if err := s.doRemoteJSON(ctx, http.MethodGet, endpoint, nodeID, secret, nil, &result); err != nil {
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
	if channelService := s.quotaLeaseDemoChannelService(); channelService != nil && snapshot.Channels != nil {
		if err := channelService.RefreshCache(ctx); err != nil {
			return err
		}
	}
	s.markMirrorSnapshotSynced(snapshot, time.Now().UTC())
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
	s.syncSuccessAt = time.Now().UTC()
	s.syncError = ""
	s.remoteMu.Unlock()
}

func (s *QuotaLeaseDemoService) markMirrorSnapshotSynced(snapshot QuotaLeaseDemoMirrorSnapshot, completedAt time.Time) {
	if s == nil {
		return
	}
	if snapshot.SyncedAt.IsZero() {
		snapshot.SyncedAt = completedAt
	}
	if completedAt.IsZero() {
		completedAt = time.Now().UTC()
	}
	s.remoteMu.Lock()
	s.mirrorReady = true
	s.mirrorSyncedAt = snapshot.SyncedAt.UTC()
	s.syncSuccessAt = completedAt.UTC()
	s.syncError = ""
	s.syncMode = quotaLeaseDemoMirrorSyncModeFull
	if snapshot.Delta {
		s.syncMode = quotaLeaseDemoMirrorSyncModeDelta
	}
	s.mirrorVersion = snapshot.Version
	s.syncedGroupCount = quotaLeaseDemoMirrorCountOrLen(snapshot.TotalGroupCount, len(snapshot.Groups))
	s.syncedChannelCount = quotaLeaseDemoMirrorCountOrLen(snapshot.TotalChannelCount, len(snapshot.Channels))
	s.syncedProxyCount = quotaLeaseDemoMirrorCountOrLen(snapshot.TotalProxyCount, len(snapshot.Proxies))
	s.syncedAccountCount = quotaLeaseDemoMirrorCountOrLen(snapshot.TotalAccountCount, len(snapshot.Accounts))
	s.remoteMu.Unlock()
}

func (s *QuotaLeaseDemoService) markAssignedAccountsSynced(accounts []QuotaLeaseDemoAssignedAccount, completedAt time.Time) {
	if s == nil {
		return
	}
	if completedAt.IsZero() {
		completedAt = time.Now().UTC()
	}
	s.remoteMu.Lock()
	s.syncSuccessAt = completedAt.UTC()
	s.syncError = ""
	s.syncMode = quotaLeaseDemoMirrorSyncModeFull
	s.syncedAccountCount = len(accounts)
	s.remoteMu.Unlock()
}

func (s *QuotaLeaseDemoService) markNodeSyncStarted(startedAt time.Time) {
	if s == nil {
		return
	}
	if startedAt.IsZero() {
		startedAt = time.Now().UTC()
	}
	s.remoteMu.Lock()
	s.syncStartedAt = startedAt.UTC()
	s.remoteMu.Unlock()
}

func (s *QuotaLeaseDemoService) markNodeSyncFailed(err error, failedAt time.Time) {
	if s == nil || err == nil {
		return
	}
	if failedAt.IsZero() {
		failedAt = time.Now().UTC()
	}
	s.remoteMu.Lock()
	s.syncFailedAt = failedAt.UTC()
	s.syncError = quotaLeaseDemoTrimSyncError(err.Error())
	s.remoteMu.Unlock()
}

func (s *QuotaLeaseDemoService) nodeSyncStatusSnapshot() QuotaLeaseDemoNodeSyncStatus {
	status := QuotaLeaseDemoNodeSyncStatus{}
	if s == nil {
		return status
	}
	s.remoteMu.Lock()
	defer s.remoteMu.Unlock()
	status.MirrorReady = s.mirrorReady
	status.MirrorSyncedAt = quotaLeaseDemoTimePtrFromValue(s.mirrorSyncedAt)
	status.LastSyncStartedAt = quotaLeaseDemoTimePtrFromValue(s.syncStartedAt)
	status.LastSyncSuccessAt = quotaLeaseDemoTimePtrFromValue(s.syncSuccessAt)
	status.LastSyncFailedAt = quotaLeaseDemoTimePtrFromValue(s.syncFailedAt)
	status.LastSyncError = s.syncError
	status.LastSyncMode = s.syncMode
	status.MirrorVersion = s.mirrorVersion
	status.SyncedGroupCount = s.syncedGroupCount
	status.SyncedChannelCount = s.syncedChannelCount
	status.SyncedProxyCount = s.syncedProxyCount
	status.SyncedAccountCount = s.syncedAccountCount
	return status
}

func (s *QuotaLeaseDemoService) currentMirrorVersion() int64 {
	if s == nil {
		return 0
	}
	s.remoteMu.Lock()
	defer s.remoteMu.Unlock()
	return s.mirrorVersion
}

func (s *QuotaLeaseDemoService) PrepareMirrorSnapshot(snapshot QuotaLeaseDemoMirrorSnapshot, sinceVersion int64) QuotaLeaseDemoMirrorSnapshot {
	snapshot = normalizeQuotaLeaseDemoMirrorSnapshot(snapshot)
	hash := quotaLeaseDemoMirrorSnapshotHash(snapshot)
	if hash == "" {
		if snapshot.Version <= 0 {
			snapshot.Version = 1
		}
		return snapshot
	}
	nodeID := strings.TrimSpace(snapshot.NodeID)
	if nodeID == "" {
		nodeID = "*"
	}

	s.remoteMu.Lock()
	if s.mirrorVersionStates == nil {
		s.mirrorVersionStates = make(map[string]*quotaLeaseDemoMirrorVersionState)
	}
	state := s.mirrorVersionStates[nodeID]
	now := time.Now().UTC()
	if state == nil {
		snapshot.Version = 1
		state = &quotaLeaseDemoMirrorVersionState{
			Version:   snapshot.Version,
			Hash:      hash,
			Snapshot:  cloneQuotaLeaseDemoMirrorSnapshot(snapshot),
			UpdatedAt: now,
		}
		s.mirrorVersionStates[nodeID] = state
	} else if state.Hash != hash {
		state.History = appendQuotaLeaseDemoMirrorHistory(state.History, state.Snapshot)
		state.Version++
		state.Hash = hash
		snapshot.Version = state.Version
		state.Snapshot = cloneQuotaLeaseDemoMirrorSnapshot(snapshot)
		state.UpdatedAt = now
	} else {
		snapshot.Version = state.Version
		state.Snapshot = cloneQuotaLeaseDemoMirrorSnapshot(snapshot)
		state.Snapshot.Version = state.Version
		state.UpdatedAt = now
	}
	current := cloneQuotaLeaseDemoMirrorSnapshot(state.Snapshot)
	base := quotaLeaseDemoMirrorHistorySnapshot(state, sinceVersion)
	s.remoteMu.Unlock()

	if sinceVersion <= 0 {
		current.Delta = false
		current.BaseVersion = 0
		return current
	}
	if sinceVersion == current.Version {
		return quotaLeaseDemoBuildMirrorDelta(current, current)
	}
	if base == nil {
		current.Delta = false
		current.BaseVersion = 0
		return current
	}
	return quotaLeaseDemoBuildMirrorDelta(*base, current)
}

func appendQuotaLeaseDemoMirrorHistory(history []QuotaLeaseDemoMirrorSnapshot, snapshot QuotaLeaseDemoMirrorSnapshot) []QuotaLeaseDemoMirrorSnapshot {
	if snapshot.Version <= 0 {
		return history
	}
	history = append(history, cloneQuotaLeaseDemoMirrorSnapshot(snapshot))
	if len(history) <= quotaLeaseDemoMirrorVersionHistoryLimit {
		return history
	}
	return append([]QuotaLeaseDemoMirrorSnapshot(nil), history[len(history)-quotaLeaseDemoMirrorVersionHistoryLimit:]...)
}

func quotaLeaseDemoMirrorHistorySnapshot(state *quotaLeaseDemoMirrorVersionState, version int64) *QuotaLeaseDemoMirrorSnapshot {
	if state == nil || version <= 0 {
		return nil
	}
	if state.Snapshot.Version == version {
		snapshot := cloneQuotaLeaseDemoMirrorSnapshot(state.Snapshot)
		return &snapshot
	}
	for i := len(state.History) - 1; i >= 0; i-- {
		if state.History[i].Version == version {
			snapshot := cloneQuotaLeaseDemoMirrorSnapshot(state.History[i])
			return &snapshot
		}
	}
	return nil
}

func quotaLeaseDemoBuildMirrorDelta(base, current QuotaLeaseDemoMirrorSnapshot) QuotaLeaseDemoMirrorSnapshot {
	base = normalizeQuotaLeaseDemoMirrorSnapshot(base)
	current = normalizeQuotaLeaseDemoMirrorSnapshot(current)
	delta := QuotaLeaseDemoMirrorSnapshot{
		NodeID:            current.NodeID,
		Version:           current.Version,
		BaseVersion:       base.Version,
		Delta:             true,
		SyncedAt:          current.SyncedAt,
		TotalGroupCount:   len(current.Groups),
		TotalChannelCount: len(current.Channels),
		TotalProxyCount:   len(current.Proxies),
		TotalAccountCount: len(current.Accounts),
		TotalAPIKeyCount:  len(current.APIKeys),
		Groups:            changedQuotaLeaseDemoMirrorGroups(base.Groups, current.Groups),
		Channels:          changedQuotaLeaseDemoMirrorChannels(base.Channels, current.Channels),
		Proxies:           changedQuotaLeaseDemoMirrorProxies(base.Proxies, current.Proxies),
		Accounts:          changedQuotaLeaseDemoMirrorAccounts(base.Accounts, current.Accounts),
		APIKeys:           changedQuotaLeaseDemoMirrorAPIKeys(base.APIKeys, current.APIKeys),
		DeletedGroupIDs:   deletedQuotaLeaseDemoMirrorGroupIDs(base.Groups, current.Groups),
		DeletedChannelIDs: deletedQuotaLeaseDemoMirrorChannelIDs(base.Channels, current.Channels),
		DeletedProxyIDs:   deletedQuotaLeaseDemoMirrorProxyIDs(base.Proxies, current.Proxies),
		DeletedAccountIDs: deletedQuotaLeaseDemoMirrorAccountIDs(base.Accounts, current.Accounts),
		DeletedAPIKeyIDs:  deletedQuotaLeaseDemoMirrorAPIKeyIDs(base.APIKeys, current.APIKeys),
	}
	delta.AccountGroups = quotaLeaseDemoMirrorAccountGroupsForAccounts(current.AccountGroups, delta.Accounts)
	return normalizeQuotaLeaseDemoMirrorSnapshot(delta)
}

func normalizeQuotaLeaseDemoMirrorSnapshot(snapshot QuotaLeaseDemoMirrorSnapshot) QuotaLeaseDemoMirrorSnapshot {
	snapshot.NodeID = strings.TrimSpace(snapshot.NodeID)
	if snapshot.SyncedAt.IsZero() {
		snapshot.SyncedAt = time.Now().UTC()
	} else {
		snapshot.SyncedAt = snapshot.SyncedAt.UTC()
	}
	sort.Slice(snapshot.Groups, func(i, j int) bool {
		return snapshot.Groups[i].ID < snapshot.Groups[j].ID
	})
	sort.Slice(snapshot.Channels, func(i, j int) bool {
		return snapshot.Channels[i].ID < snapshot.Channels[j].ID
	})
	for i := range snapshot.Channels {
		sort.Slice(snapshot.Channels[i].ModelPricing, func(a, b int) bool {
			return snapshot.Channels[i].ModelPricing[a].ID < snapshot.Channels[i].ModelPricing[b].ID
		})
		for j := range snapshot.Channels[i].ModelPricing {
			sort.Slice(snapshot.Channels[i].ModelPricing[j].Intervals, func(a, b int) bool {
				if snapshot.Channels[i].ModelPricing[j].Intervals[a].SortOrder == snapshot.Channels[i].ModelPricing[j].Intervals[b].SortOrder {
					return snapshot.Channels[i].ModelPricing[j].Intervals[a].ID < snapshot.Channels[i].ModelPricing[j].Intervals[b].ID
				}
				return snapshot.Channels[i].ModelPricing[j].Intervals[a].SortOrder < snapshot.Channels[i].ModelPricing[j].Intervals[b].SortOrder
			})
		}
	}
	sort.Slice(snapshot.Proxies, func(i, j int) bool {
		return snapshot.Proxies[i].ID < snapshot.Proxies[j].ID
	})
	sort.Slice(snapshot.Accounts, func(i, j int) bool {
		return snapshot.Accounts[i].ID < snapshot.Accounts[j].ID
	})
	for i := range snapshot.Accounts {
		sort.Slice(snapshot.Accounts[i].AccountGroups, func(a, b int) bool {
			if snapshot.Accounts[i].AccountGroups[a].AccountID == snapshot.Accounts[i].AccountGroups[b].AccountID {
				return snapshot.Accounts[i].AccountGroups[a].GroupID < snapshot.Accounts[i].AccountGroups[b].GroupID
			}
			return snapshot.Accounts[i].AccountGroups[a].AccountID < snapshot.Accounts[i].AccountGroups[b].AccountID
		})
	}
	sort.Slice(snapshot.AccountGroups, func(i, j int) bool {
		if snapshot.AccountGroups[i].AccountID == snapshot.AccountGroups[j].AccountID {
			return snapshot.AccountGroups[i].GroupID < snapshot.AccountGroups[j].GroupID
		}
		return snapshot.AccountGroups[i].AccountID < snapshot.AccountGroups[j].AccountID
	})
	sort.Slice(snapshot.APIKeys, func(i, j int) bool {
		return snapshot.APIKeys[i].ID < snapshot.APIKeys[j].ID
	})
	snapshot.DeletedGroupIDs = quotaLeaseDemoUniqueSortedInt64s(snapshot.DeletedGroupIDs)
	snapshot.DeletedChannelIDs = quotaLeaseDemoUniqueSortedInt64s(snapshot.DeletedChannelIDs)
	snapshot.DeletedProxyIDs = quotaLeaseDemoUniqueSortedInt64s(snapshot.DeletedProxyIDs)
	snapshot.DeletedAccountIDs = quotaLeaseDemoUniqueSortedInt64s(snapshot.DeletedAccountIDs)
	snapshot.DeletedAPIKeyIDs = quotaLeaseDemoUniqueSortedInt64s(snapshot.DeletedAPIKeyIDs)
	if snapshot.TotalGroupCount <= 0 && len(snapshot.Groups) > 0 {
		snapshot.TotalGroupCount = len(snapshot.Groups)
	}
	if snapshot.TotalChannelCount <= 0 && len(snapshot.Channels) > 0 {
		snapshot.TotalChannelCount = len(snapshot.Channels)
	}
	if snapshot.TotalProxyCount <= 0 && len(snapshot.Proxies) > 0 {
		snapshot.TotalProxyCount = len(snapshot.Proxies)
	}
	if snapshot.TotalAccountCount <= 0 && len(snapshot.Accounts) > 0 {
		snapshot.TotalAccountCount = len(snapshot.Accounts)
	}
	if snapshot.TotalAPIKeyCount <= 0 && len(snapshot.APIKeys) > 0 {
		snapshot.TotalAPIKeyCount = len(snapshot.APIKeys)
	}
	return snapshot
}

func quotaLeaseDemoMirrorSnapshotHash(snapshot QuotaLeaseDemoMirrorSnapshot) string {
	signature := normalizeQuotaLeaseDemoMirrorSnapshot(cloneQuotaLeaseDemoMirrorSnapshot(snapshot))
	signature.Version = 0
	signature.BaseVersion = 0
	signature.Delta = false
	signature.SyncedAt = time.Time{}
	signature.TotalGroupCount = 0
	signature.TotalChannelCount = 0
	signature.TotalProxyCount = 0
	signature.TotalAccountCount = 0
	signature.TotalAPIKeyCount = 0
	for i := range signature.Groups {
		signature.Groups[i] = quotaLeaseDemoGroupSignatureSnapshot(signature.Groups[i])
	}
	for i := range signature.Channels {
		signature.Channels[i] = quotaLeaseDemoChannelSignatureSnapshot(signature.Channels[i])
	}
	for i := range signature.Proxies {
		signature.Proxies[i] = quotaLeaseDemoProxySignatureSnapshot(signature.Proxies[i])
	}
	for i := range signature.Accounts {
		signature.Accounts[i] = quotaLeaseDemoAccountSignatureSnapshot(signature.Accounts[i])
	}
	for i := range signature.APIKeys {
		signature.APIKeys[i] = quotaLeaseDemoAPIKeySignatureSnapshot(signature.APIKeys[i])
	}
	for i := range signature.AccountGroups {
		signature.AccountGroups[i].CreatedAt = time.Time{}
	}
	payload, err := json.Marshal(signature)
	if err != nil {
		return ""
	}
	sum := sha256.Sum256(payload)
	return hex.EncodeToString(sum[:])
}

func cloneQuotaLeaseDemoMirrorSnapshot(snapshot QuotaLeaseDemoMirrorSnapshot) QuotaLeaseDemoMirrorSnapshot {
	payload, err := json.Marshal(snapshot)
	if err != nil {
		return snapshot
	}
	var out QuotaLeaseDemoMirrorSnapshot
	if err := json.Unmarshal(payload, &out); err != nil {
		return snapshot
	}
	return out
}

func changedQuotaLeaseDemoMirrorGroups(base, current []QuotaLeaseDemoGroupSnapshot) []QuotaLeaseDemoGroupSnapshot {
	baseHashes := make(map[int64]string, len(base))
	for _, item := range base {
		if item.ID > 0 {
			baseHashes[item.ID] = quotaLeaseDemoMirrorItemHash(quotaLeaseDemoGroupSignatureSnapshot(item))
		}
	}
	out := make([]QuotaLeaseDemoGroupSnapshot, 0)
	for _, item := range current {
		if item.ID <= 0 {
			continue
		}
		if baseHashes[item.ID] != quotaLeaseDemoMirrorItemHash(quotaLeaseDemoGroupSignatureSnapshot(item)) {
			out = append(out, item)
		}
	}
	return out
}

func changedQuotaLeaseDemoMirrorChannels(base, current []QuotaLeaseDemoChannelSnapshot) []QuotaLeaseDemoChannelSnapshot {
	baseHashes := make(map[int64]string, len(base))
	for _, item := range base {
		if item.ID > 0 {
			baseHashes[item.ID] = quotaLeaseDemoMirrorItemHash(quotaLeaseDemoChannelSignatureSnapshot(item))
		}
	}
	out := make([]QuotaLeaseDemoChannelSnapshot, 0)
	for _, item := range current {
		if item.ID <= 0 {
			continue
		}
		if baseHashes[item.ID] != quotaLeaseDemoMirrorItemHash(quotaLeaseDemoChannelSignatureSnapshot(item)) {
			out = append(out, item)
		}
	}
	return out
}

func changedQuotaLeaseDemoMirrorProxies(base, current []QuotaLeaseDemoProxySnapshot) []QuotaLeaseDemoProxySnapshot {
	baseHashes := make(map[int64]string, len(base))
	for _, item := range base {
		if item.ID > 0 {
			baseHashes[item.ID] = quotaLeaseDemoMirrorItemHash(quotaLeaseDemoProxySignatureSnapshot(item))
		}
	}
	out := make([]QuotaLeaseDemoProxySnapshot, 0)
	for _, item := range current {
		if item.ID <= 0 {
			continue
		}
		if baseHashes[item.ID] != quotaLeaseDemoMirrorItemHash(quotaLeaseDemoProxySignatureSnapshot(item)) {
			out = append(out, item)
		}
	}
	return out
}

func changedQuotaLeaseDemoMirrorAccounts(base, current []QuotaLeaseDemoAccountSnapshot) []QuotaLeaseDemoAccountSnapshot {
	baseHashes := make(map[int64]string, len(base))
	for _, item := range base {
		if item.ID > 0 {
			baseHashes[item.ID] = quotaLeaseDemoMirrorItemHash(quotaLeaseDemoAccountSignatureSnapshot(item))
		}
	}
	out := make([]QuotaLeaseDemoAccountSnapshot, 0)
	for _, item := range current {
		if item.ID <= 0 {
			continue
		}
		if baseHashes[item.ID] != quotaLeaseDemoMirrorItemHash(quotaLeaseDemoAccountSignatureSnapshot(item)) {
			out = append(out, item)
		}
	}
	return out
}

func changedQuotaLeaseDemoMirrorAPIKeys(base, current []QuotaLeaseDemoAPIKeySnapshot) []QuotaLeaseDemoAPIKeySnapshot {
	baseHashes := make(map[int64]string, len(base))
	for _, item := range base {
		if item.ID > 0 {
			baseHashes[item.ID] = quotaLeaseDemoMirrorItemHash(quotaLeaseDemoAPIKeySignatureSnapshot(item))
		}
	}
	out := make([]QuotaLeaseDemoAPIKeySnapshot, 0)
	for _, item := range current {
		if item.ID <= 0 {
			continue
		}
		if baseHashes[item.ID] != quotaLeaseDemoMirrorItemHash(quotaLeaseDemoAPIKeySignatureSnapshot(item)) {
			out = append(out, item)
		}
	}
	return out
}

func deletedQuotaLeaseDemoMirrorGroupIDs(base, current []QuotaLeaseDemoGroupSnapshot) []int64 {
	currentIDs := make(map[int64]struct{}, len(current))
	for _, item := range current {
		if item.ID > 0 {
			currentIDs[item.ID] = struct{}{}
		}
	}
	out := make([]int64, 0)
	for _, item := range base {
		if item.ID <= 0 {
			continue
		}
		if _, ok := currentIDs[item.ID]; !ok {
			out = append(out, item.ID)
		}
	}
	return quotaLeaseDemoUniqueSortedInt64s(out)
}

func deletedQuotaLeaseDemoMirrorChannelIDs(base, current []QuotaLeaseDemoChannelSnapshot) []int64 {
	currentIDs := make(map[int64]struct{}, len(current))
	for _, item := range current {
		if item.ID > 0 {
			currentIDs[item.ID] = struct{}{}
		}
	}
	out := make([]int64, 0)
	for _, item := range base {
		if item.ID <= 0 {
			continue
		}
		if _, ok := currentIDs[item.ID]; !ok {
			out = append(out, item.ID)
		}
	}
	return quotaLeaseDemoUniqueSortedInt64s(out)
}

func deletedQuotaLeaseDemoMirrorProxyIDs(base, current []QuotaLeaseDemoProxySnapshot) []int64 {
	currentIDs := make(map[int64]struct{}, len(current))
	for _, item := range current {
		if item.ID > 0 {
			currentIDs[item.ID] = struct{}{}
		}
	}
	out := make([]int64, 0)
	for _, item := range base {
		if item.ID <= 0 {
			continue
		}
		if _, ok := currentIDs[item.ID]; !ok {
			out = append(out, item.ID)
		}
	}
	return quotaLeaseDemoUniqueSortedInt64s(out)
}

func deletedQuotaLeaseDemoMirrorAccountIDs(base, current []QuotaLeaseDemoAccountSnapshot) []int64 {
	currentIDs := make(map[int64]struct{}, len(current))
	for _, item := range current {
		if item.ID > 0 {
			currentIDs[item.ID] = struct{}{}
		}
	}
	out := make([]int64, 0)
	for _, item := range base {
		if item.ID <= 0 {
			continue
		}
		if _, ok := currentIDs[item.ID]; !ok {
			out = append(out, item.ID)
		}
	}
	return quotaLeaseDemoUniqueSortedInt64s(out)
}

func deletedQuotaLeaseDemoMirrorAPIKeyIDs(base, current []QuotaLeaseDemoAPIKeySnapshot) []int64 {
	currentIDs := make(map[int64]struct{}, len(current))
	for _, item := range current {
		if item.ID > 0 {
			currentIDs[item.ID] = struct{}{}
		}
	}
	out := make([]int64, 0)
	for _, item := range base {
		if item.ID <= 0 {
			continue
		}
		if _, ok := currentIDs[item.ID]; !ok {
			out = append(out, item.ID)
		}
	}
	return quotaLeaseDemoUniqueSortedInt64s(out)
}

func quotaLeaseDemoMirrorAccountGroupsForAccounts(groups []QuotaLeaseDemoAccountGroupSnapshot, accounts []QuotaLeaseDemoAccountSnapshot) []QuotaLeaseDemoAccountGroupSnapshot {
	if len(accounts) == 0 || len(groups) == 0 {
		return nil
	}
	accountIDs := make(map[int64]struct{}, len(accounts))
	for _, account := range accounts {
		if account.ID > 0 {
			accountIDs[account.ID] = struct{}{}
		}
	}
	out := make([]QuotaLeaseDemoAccountGroupSnapshot, 0)
	for _, group := range groups {
		if _, ok := accountIDs[group.AccountID]; ok {
			out = append(out, group)
		}
	}
	return out
}

func quotaLeaseDemoGroupSignatureSnapshot(group QuotaLeaseDemoGroupSnapshot) QuotaLeaseDemoGroupSnapshot {
	group.UpdatedAt = time.Time{}
	return group
}

func quotaLeaseDemoChannelSignatureSnapshot(channel QuotaLeaseDemoChannelSnapshot) QuotaLeaseDemoChannelSnapshot {
	channel = quotaLeaseDemoMirrorJSONClone(channel)
	channel.UpdatedAt = time.Time{}
	for i := range channel.ModelPricing {
		channel.ModelPricing[i].UpdatedAt = time.Time{}
		for j := range channel.ModelPricing[i].Intervals {
			channel.ModelPricing[i].Intervals[j].UpdatedAt = time.Time{}
		}
	}
	return channel
}

func quotaLeaseDemoProxySignatureSnapshot(proxy QuotaLeaseDemoProxySnapshot) QuotaLeaseDemoProxySnapshot {
	proxy.UpdatedAt = time.Time{}
	return proxy
}

func quotaLeaseDemoAccountSignatureSnapshot(account QuotaLeaseDemoAccountSnapshot) QuotaLeaseDemoAccountSnapshot {
	account = quotaLeaseDemoMirrorJSONClone(account)
	account.UpdatedAt = time.Time{}
	account.Extra = cloneQuotaLeaseDemoAnyMap(account.Extra)
	delete(account.Extra, "node_oauth_last_synced_at")
	for i := range account.AccountGroups {
		account.AccountGroups[i].CreatedAt = time.Time{}
	}
	return account
}

func quotaLeaseDemoAPIKeySignatureSnapshot(apiKey QuotaLeaseDemoAPIKeySnapshot) QuotaLeaseDemoAPIKeySnapshot {
	apiKey = quotaLeaseDemoMirrorJSONClone(apiKey)
	apiKey.CreatedAt = time.Time{}
	apiKey.UpdatedAt = time.Time{}
	return apiKey
}

func quotaLeaseDemoMirrorJSONClone[T any](value T) T {
	payload, err := json.Marshal(value)
	if err != nil {
		return value
	}
	var out T
	if err := json.Unmarshal(payload, &out); err != nil {
		return value
	}
	return out
}

func quotaLeaseDemoMirrorItemHash(value any) string {
	payload, err := json.Marshal(value)
	if err != nil {
		return ""
	}
	sum := sha256.Sum256(payload)
	return hex.EncodeToString(sum[:])
}

func quotaLeaseDemoUniqueSortedInt64s(ids []int64) []int64 {
	if len(ids) == 0 {
		return nil
	}
	seen := make(map[int64]struct{}, len(ids))
	out := make([]int64, 0, len(ids))
	for _, id := range ids {
		if id <= 0 {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		out = append(out, id)
	}
	if len(out) == 0 {
		return nil
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i] < out[j]
	})
	return out
}

func quotaLeaseDemoMirrorCountOrLen(total, length int) int {
	if total > 0 || length == 0 {
		return total
	}
	return length
}

func quotaLeaseDemoTimePtrFromValue(value time.Time) *time.Time {
	if value.IsZero() {
		return nil
	}
	value = value.UTC()
	return &value
}

func quotaLeaseDemoTrimSyncError(message string) string {
	message = strings.TrimSpace(message)
	if len(message) <= 500 {
		return message
	}
	return message[:500]
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
