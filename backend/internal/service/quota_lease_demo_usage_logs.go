package service

import (
	"context"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
)

type QuotaLeaseDemoUsageLogSnapshot struct {
	NodeID                    string         `json:"node_id,omitempty"`
	UserID                    int64          `json:"user_id"`
	APIKeyID                  int64          `json:"api_key_id"`
	AccountID                 int64          `json:"account_id"`
	RequestID                 string         `json:"request_id"`
	Model                     string         `json:"model"`
	RequestedModel            string         `json:"requested_model,omitempty"`
	UpstreamModel             *string        `json:"upstream_model,omitempty"`
	ChannelID                 *int64         `json:"channel_id,omitempty"`
	ModelMappingChain         *string        `json:"model_mapping_chain,omitempty"`
	BillingTier               *string        `json:"billing_tier,omitempty"`
	BillingMode               *string        `json:"billing_mode,omitempty"`
	ServiceTier               *string        `json:"service_tier,omitempty"`
	ReasoningEffort           *string        `json:"reasoning_effort,omitempty"`
	InboundEndpoint           *string        `json:"inbound_endpoint,omitempty"`
	UpstreamEndpoint          *string        `json:"upstream_endpoint,omitempty"`
	GroupID                   *int64         `json:"group_id,omitempty"`
	SubscriptionID            *int64         `json:"subscription_id,omitempty"`
	InputTokens               int            `json:"input_tokens"`
	OutputTokens              int            `json:"output_tokens"`
	CacheCreationTokens       int            `json:"cache_creation_tokens"`
	CacheReadTokens           int            `json:"cache_read_tokens"`
	CacheCreation5mTokens     int            `json:"cache_creation_5m_tokens"`
	CacheCreation1hTokens     int            `json:"cache_creation_1h_tokens"`
	ImageInputTokens          int            `json:"image_input_tokens"`
	ImageInputCost            float64        `json:"image_input_cost"`
	ImageOutputTokens         int            `json:"image_output_tokens"`
	ImageOutputCost           float64        `json:"image_output_cost"`
	InputCost                 float64        `json:"input_cost"`
	OutputCost                float64        `json:"output_cost"`
	CacheCreationCost         float64        `json:"cache_creation_cost"`
	CacheReadCost             float64        `json:"cache_read_cost"`
	TotalCost                 float64        `json:"total_cost"`
	ActualCost                float64        `json:"actual_cost"`
	RateMultiplier            float64        `json:"rate_multiplier"`
	LongContextBillingApplied bool           `json:"long_context_billing_applied"`
	AccountRateMultiplier     *float64       `json:"account_rate_multiplier,omitempty"`
	AccountStatsCost          *float64       `json:"account_stats_cost,omitempty"`
	KiroCredits               *float64       `json:"kiro_credits,omitempty"`
	BillingType               int8           `json:"billing_type"`
	RequestType               RequestType    `json:"request_type"`
	Stream                    bool           `json:"stream"`
	OpenAIWSMode              bool           `json:"openai_ws_mode"`
	DurationMs                *int           `json:"duration_ms,omitempty"`
	FirstTokenMs              *int           `json:"first_token_ms,omitempty"`
	UserAgent                 *string        `json:"user_agent,omitempty"`
	IPAddress                 *string        `json:"ip_address,omitempty"`
	CacheTTLOverridden        bool           `json:"cache_ttl_overridden"`
	ImageCount                int            `json:"image_count"`
	ImageSize                 *string        `json:"image_size,omitempty"`
	ImageInputSize            *string        `json:"image_input_size,omitempty"`
	ImageOutputSize           *string        `json:"image_output_size,omitempty"`
	ImageSizeSource           *string        `json:"image_size_source,omitempty"`
	ImageSizeBreakdown        map[string]int `json:"image_size_breakdown,omitempty"`
	MediaType                 *string        `json:"media_type,omitempty"`
	VideoCount                int            `json:"video_count"`
	VideoResolution           *string        `json:"video_resolution,omitempty"`
	VideoDurationSeconds      *int           `json:"video_duration_seconds,omitempty"`
	CreatedAt                 time.Time      `json:"created_at"`
}

type QuotaLeaseDemoUsageLogBatchRequest struct {
	NodeID string                           `json:"node_id"`
	Logs   []QuotaLeaseDemoUsageLogSnapshot `json:"logs"`
}

type QuotaLeaseDemoUsageLogResult struct {
	RequestID string `json:"request_id"`
	APIKeyID  int64  `json:"api_key_id"`
	Applied   bool   `json:"applied"`
	Duplicate bool   `json:"duplicate"`
	Error     string `json:"error,omitempty"`
}

type QuotaLeaseDemoUsageLogBatchResult struct {
	Results []QuotaLeaseDemoUsageLogResult `json:"results"`
}

func NewQuotaLeaseDemoUsageLogSnapshot(nodeID string, log *UsageLog) QuotaLeaseDemoUsageLogSnapshot {
	if log == nil {
		return QuotaLeaseDemoUsageLogSnapshot{}
	}
	log.SyncRequestTypeAndLegacyFields()
	snap := QuotaLeaseDemoUsageLogSnapshot{
		NodeID:                    strings.TrimSpace(nodeID),
		UserID:                    log.UserID,
		APIKeyID:                  log.APIKeyID,
		AccountID:                 log.AccountID,
		RequestID:                 strings.TrimSpace(log.RequestID),
		Model:                     log.Model,
		RequestedModel:            log.RequestedModel,
		UpstreamModel:             cloneQuotaLeaseDemoStringPtr(log.UpstreamModel),
		ChannelID:                 cloneQuotaLeaseDemoInt64Ptr(log.ChannelID),
		ModelMappingChain:         cloneQuotaLeaseDemoStringPtr(log.ModelMappingChain),
		BillingTier:               cloneQuotaLeaseDemoStringPtr(log.BillingTier),
		BillingMode:               cloneQuotaLeaseDemoStringPtr(log.BillingMode),
		ServiceTier:               cloneQuotaLeaseDemoStringPtr(log.ServiceTier),
		ReasoningEffort:           cloneQuotaLeaseDemoStringPtr(log.ReasoningEffort),
		InboundEndpoint:           cloneQuotaLeaseDemoStringPtr(log.InboundEndpoint),
		UpstreamEndpoint:          cloneQuotaLeaseDemoStringPtr(log.UpstreamEndpoint),
		GroupID:                   cloneQuotaLeaseDemoInt64Ptr(log.GroupID),
		SubscriptionID:            cloneQuotaLeaseDemoInt64Ptr(log.SubscriptionID),
		InputTokens:               log.InputTokens,
		OutputTokens:              log.OutputTokens,
		CacheCreationTokens:       log.CacheCreationTokens,
		CacheReadTokens:           log.CacheReadTokens,
		CacheCreation5mTokens:     log.CacheCreation5mTokens,
		CacheCreation1hTokens:     log.CacheCreation1hTokens,
		ImageInputTokens:          log.ImageInputTokens,
		ImageInputCost:            log.ImageInputCost,
		ImageOutputTokens:         log.ImageOutputTokens,
		ImageOutputCost:           log.ImageOutputCost,
		InputCost:                 log.InputCost,
		OutputCost:                log.OutputCost,
		CacheCreationCost:         log.CacheCreationCost,
		CacheReadCost:             log.CacheReadCost,
		TotalCost:                 log.TotalCost,
		ActualCost:                log.ActualCost,
		RateMultiplier:            log.RateMultiplier,
		LongContextBillingApplied: log.LongContextBillingApplied,
		AccountRateMultiplier:     cloneQuotaLeaseDemoFloat64Ptr(log.AccountRateMultiplier),
		AccountStatsCost:          cloneQuotaLeaseDemoFloat64Ptr(log.AccountStatsCost),
		KiroCredits:               cloneQuotaLeaseDemoFloat64Ptr(log.KiroCredits),
		BillingType:               log.BillingType,
		RequestType:               log.RequestType,
		Stream:                    log.Stream,
		OpenAIWSMode:              log.OpenAIWSMode,
		DurationMs:                cloneQuotaLeaseDemoIntPtr(log.DurationMs),
		FirstTokenMs:              cloneQuotaLeaseDemoIntPtr(log.FirstTokenMs),
		UserAgent:                 cloneQuotaLeaseDemoStringPtr(log.UserAgent),
		IPAddress:                 cloneQuotaLeaseDemoStringPtr(log.IPAddress),
		CacheTTLOverridden:        log.CacheTTLOverridden,
		ImageCount:                log.ImageCount,
		ImageSize:                 cloneQuotaLeaseDemoStringPtr(log.ImageSize),
		ImageInputSize:            cloneQuotaLeaseDemoStringPtr(log.ImageInputSize),
		ImageOutputSize:           cloneQuotaLeaseDemoStringPtr(log.ImageOutputSize),
		ImageSizeSource:           cloneQuotaLeaseDemoStringPtr(log.ImageSizeSource),
		ImageSizeBreakdown:        cloneQuotaLeaseDemoIntMap(log.ImageSizeBreakdown),
		MediaType:                 cloneQuotaLeaseDemoStringPtr(log.MediaType),
		VideoCount:                log.VideoCount,
		VideoResolution:           cloneQuotaLeaseDemoStringPtr(log.VideoResolution),
		VideoDurationSeconds:      cloneQuotaLeaseDemoIntPtr(log.VideoDurationSeconds),
		CreatedAt:                 log.CreatedAt,
	}
	if strings.TrimSpace(snap.RequestedModel) == "" {
		snap.RequestedModel = strings.TrimSpace(snap.Model)
	}
	if snap.CreatedAt.IsZero() {
		snap.CreatedAt = time.Now()
	}
	return snap
}

func (s QuotaLeaseDemoUsageLogSnapshot) ToUsageLog() *UsageLog {
	log := &UsageLog{
		UserID:                    s.UserID,
		APIKeyID:                  s.APIKeyID,
		AccountID:                 s.AccountID,
		RequestID:                 strings.TrimSpace(s.RequestID),
		Model:                     s.Model,
		RequestedModel:            s.RequestedModel,
		UpstreamModel:             cloneQuotaLeaseDemoStringPtr(s.UpstreamModel),
		ChannelID:                 cloneQuotaLeaseDemoInt64Ptr(s.ChannelID),
		ModelMappingChain:         cloneQuotaLeaseDemoStringPtr(s.ModelMappingChain),
		BillingTier:               cloneQuotaLeaseDemoStringPtr(s.BillingTier),
		BillingMode:               cloneQuotaLeaseDemoStringPtr(s.BillingMode),
		ServiceTier:               cloneQuotaLeaseDemoStringPtr(s.ServiceTier),
		ReasoningEffort:           cloneQuotaLeaseDemoStringPtr(s.ReasoningEffort),
		InboundEndpoint:           cloneQuotaLeaseDemoStringPtr(s.InboundEndpoint),
		UpstreamEndpoint:          cloneQuotaLeaseDemoStringPtr(s.UpstreamEndpoint),
		GroupID:                   cloneQuotaLeaseDemoInt64Ptr(s.GroupID),
		SubscriptionID:            cloneQuotaLeaseDemoInt64Ptr(s.SubscriptionID),
		InputTokens:               s.InputTokens,
		OutputTokens:              s.OutputTokens,
		CacheCreationTokens:       s.CacheCreationTokens,
		CacheReadTokens:           s.CacheReadTokens,
		CacheCreation5mTokens:     s.CacheCreation5mTokens,
		CacheCreation1hTokens:     s.CacheCreation1hTokens,
		ImageInputTokens:          s.ImageInputTokens,
		ImageInputCost:            s.ImageInputCost,
		ImageOutputTokens:         s.ImageOutputTokens,
		ImageOutputCost:           s.ImageOutputCost,
		InputCost:                 s.InputCost,
		OutputCost:                s.OutputCost,
		CacheCreationCost:         s.CacheCreationCost,
		CacheReadCost:             s.CacheReadCost,
		TotalCost:                 s.TotalCost,
		ActualCost:                s.ActualCost,
		RateMultiplier:            s.RateMultiplier,
		LongContextBillingApplied: s.LongContextBillingApplied,
		AccountRateMultiplier:     cloneQuotaLeaseDemoFloat64Ptr(s.AccountRateMultiplier),
		AccountStatsCost:          cloneQuotaLeaseDemoFloat64Ptr(s.AccountStatsCost),
		KiroCredits:               cloneQuotaLeaseDemoFloat64Ptr(s.KiroCredits),
		BillingType:               s.BillingType,
		RequestType:               s.RequestType,
		Stream:                    s.Stream,
		OpenAIWSMode:              s.OpenAIWSMode,
		DurationMs:                cloneQuotaLeaseDemoIntPtr(s.DurationMs),
		FirstTokenMs:              cloneQuotaLeaseDemoIntPtr(s.FirstTokenMs),
		UserAgent:                 cloneQuotaLeaseDemoStringPtr(s.UserAgent),
		IPAddress:                 cloneQuotaLeaseDemoStringPtr(s.IPAddress),
		CacheTTLOverridden:        s.CacheTTLOverridden,
		ImageCount:                s.ImageCount,
		ImageSize:                 cloneQuotaLeaseDemoStringPtr(s.ImageSize),
		ImageInputSize:            cloneQuotaLeaseDemoStringPtr(s.ImageInputSize),
		ImageOutputSize:           cloneQuotaLeaseDemoStringPtr(s.ImageOutputSize),
		ImageSizeSource:           cloneQuotaLeaseDemoStringPtr(s.ImageSizeSource),
		ImageSizeBreakdown:        cloneQuotaLeaseDemoIntMap(s.ImageSizeBreakdown),
		MediaType:                 cloneQuotaLeaseDemoStringPtr(s.MediaType),
		VideoCount:                s.VideoCount,
		VideoResolution:           cloneQuotaLeaseDemoStringPtr(s.VideoResolution),
		VideoDurationSeconds:      cloneQuotaLeaseDemoIntPtr(s.VideoDurationSeconds),
		CreatedAt:                 s.CreatedAt,
	}
	if strings.TrimSpace(log.RequestedModel) == "" {
		log.RequestedModel = strings.TrimSpace(log.Model)
	}
	if log.CreatedAt.IsZero() {
		log.CreatedAt = time.Now()
	}
	log.SyncRequestTypeAndLegacyFields()
	return log
}

func enqueueQuotaLeaseDemoUsageLogSnapshot(ctx context.Context, usageLog *UsageLog, cfg *config.Config) {
	if usageLog == nil || cfg == nil || !cfg.IsNodeRole() || !QuotaLeaseDemoEnabled(cfg) {
		return
	}
	svc := GetQuotaLeaseDemoService(cfg)
	if svc == nil || !svc.remoteMode() {
		return
	}
	nodeID := svc.activeNodeID()
	snapshot := NewQuotaLeaseDemoUsageLogSnapshot(nodeID, usageLog)
	if strings.TrimSpace(snapshot.RequestID) == "" || snapshot.APIKeyID <= 0 {
		return
	}
	svc.enqueuePendingUsageLogSnapshot(snapshot)
	svc.flushPendingUsageLogsAsync()
	_ = ctx
}

func (s *QuotaLeaseDemoService) enqueuePendingUsageLogSnapshot(snapshot QuotaLeaseDemoUsageLogSnapshot) {
	if s == nil {
		return
	}
	snapshot.NodeID = strings.TrimSpace(snapshot.NodeID)
	snapshot.RequestID = strings.TrimSpace(snapshot.RequestID)
	if snapshot.NodeID == "" {
		snapshot.NodeID = s.NodeID()
	}
	if snapshot.RequestID == "" || snapshot.APIKeyID <= 0 {
		return
	}
	if snapshot.CreatedAt.IsZero() {
		snapshot.CreatedAt = time.Now()
	}
	key := quotaLeaseDemoUsageLogSnapshotKey(snapshot.NodeID, snapshot.APIKeyID, snapshot.RequestID)
	s.mu.Lock()
	if s.pendingUsageLogs == nil {
		s.pendingUsageLogs = make(map[string]QuotaLeaseDemoUsageLogSnapshot)
	}
	s.pendingUsageLogs[key] = snapshot
	s.mu.Unlock()
}

func (s *QuotaLeaseDemoService) pendingUsageLogSnapshots() []QuotaLeaseDemoUsageLogSnapshot {
	if s == nil {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	logs := make([]QuotaLeaseDemoUsageLogSnapshot, 0, len(s.pendingUsageLogs))
	for _, log := range s.pendingUsageLogs {
		logs = append(logs, log)
	}
	sort.Slice(logs, func(i, j int) bool {
		return logs[i].CreatedAt.Before(logs[j].CreatedAt)
	})
	return logs
}

func (s *QuotaLeaseDemoService) removePendingUsageLogResults(result QuotaLeaseDemoUsageLogBatchResult) {
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
			for key, snapshot := range s.pendingUsageLogs {
				if strings.TrimSpace(snapshot.RequestID) == strings.TrimSpace(item.RequestID) && snapshot.APIKeyID == item.APIKeyID {
					delete(s.pendingUsageLogs, key)
				}
			}
		}
	}
}

func quotaLeaseDemoUsageLogSnapshotKey(nodeID string, apiKeyID int64, requestID string) string {
	return strings.TrimSpace(nodeID) + "\x1f" + strconv.FormatInt(apiKeyID, 10) + "\x1f" + strings.TrimSpace(requestID)
}

func cloneQuotaLeaseDemoStringPtr(src *string) *string {
	if src == nil {
		return nil
	}
	value := *src
	return &value
}

func cloneQuotaLeaseDemoIntPtr(src *int) *int {
	if src == nil {
		return nil
	}
	value := *src
	return &value
}

func cloneQuotaLeaseDemoFloat64Ptr(src *float64) *float64 {
	if src == nil {
		return nil
	}
	value := *src
	return &value
}

func cloneQuotaLeaseDemoIntMap(src map[string]int) map[string]int {
	if len(src) == 0 {
		return nil
	}
	dst := make(map[string]int, len(src))
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
