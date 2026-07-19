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
	NodeID        string                               `json:"node_id,omitempty"`
	SyncedAt      time.Time                            `json:"synced_at"`
	Groups        []QuotaLeaseDemoGroupSnapshot        `json:"groups"`
	Channels      []QuotaLeaseDemoChannelSnapshot      `json:"channels"`
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
	if channelService := s.quotaLeaseDemoChannelService(); channelService != nil && snapshot.Channels != nil {
		if err := channelService.RefreshCache(ctx); err != nil {
			return err
		}
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
