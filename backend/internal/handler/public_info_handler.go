package handler

import (
	"context"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/service"

	"github.com/gin-gonic/gin"
)

// PublicInfoHandler exposes read-only public metadata for external sites.
type PublicInfoHandler struct {
	groupRepo      service.GroupRepository
	monitorService *service.ChannelMonitorService
	settingService *service.SettingService
	paymentConfig  *service.PaymentConfigService
}

func NewPublicInfoHandler(
	groupRepo service.GroupRepository,
	monitorService *service.ChannelMonitorService,
	settingService *service.SettingService,
	paymentConfig *service.PaymentConfigService,
) *PublicInfoHandler {
	return &PublicInfoHandler{
		groupRepo:      groupRepo,
		monitorService: monitorService,
		settingService: settingService,
		paymentConfig:  paymentConfig,
	}
}

type publicSiteInfoResponse struct {
	GeneratedAt       string                      `json:"generated_at"`
	Groups            []publicGroupRate           `json:"groups"`
	ModelAvailability []publicMonitorAvailability `json:"model_availability"`
	Recharge          publicRechargeInfo          `json:"recharge"`
}

type publicGroupRate struct {
	ID                   int64   `json:"id"`
	Name                 string  `json:"name"`
	Platform             string  `json:"platform"`
	RateMultiplier       float64 `json:"rate_multiplier"`
	AllowImageGeneration bool    `json:"allow_image_generation"`
	ImageRateMultiplier  float64 `json:"image_rate_multiplier"`
}

type publicMonitorAvailability struct {
	ID        int64                        `json:"id"`
	Name      string                       `json:"name"`
	Provider  string                       `json:"provider"`
	GroupName string                       `json:"group_name"`
	Models    []publicModelAvailability    `json:"models"`
	Timeline  []publicMonitorTimelinePoint `json:"timeline"`
}

type publicModelAvailability struct {
	Model           string  `json:"model"`
	LatestStatus    string  `json:"latest_status"`
	Availability7d  float64 `json:"availability_7d"`
	Availability15d float64 `json:"availability_15d"`
	Availability30d float64 `json:"availability_30d"`
}

type publicMonitorTimelinePoint struct {
	Status        string `json:"status"`
	LatencyMs     *int   `json:"latency_ms"`
	PingLatencyMs *int   `json:"ping_latency_ms"`
	CheckedAt     string `json:"checked_at"`
}

type publicRechargeInfo struct {
	PaymentEnabled            bool    `json:"payment_enabled"`
	BalanceDisabled           bool    `json:"balance_disabled"`
	BalanceRechargeMultiplier float64 `json:"balance_recharge_multiplier"`
}

// GetSiteInfo returns public group rates, model availability, and recharge ratio.
// GET /api/v1/public/site-info
func (h *PublicInfoHandler) GetSiteInfo(c *gin.Context) {
	ctx := c.Request.Context()

	groups, err := h.loadPublicGroupRates(ctx)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	availability, err := h.loadPublicModelAvailability(ctx)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	recharge, err := h.loadPublicRechargeInfo(ctx)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}

	response.Success(c, publicSiteInfoResponse{
		GeneratedAt:       time.Now().UTC().Format(time.RFC3339),
		Groups:            groups,
		ModelAvailability: availability,
		Recharge:          recharge,
	})
}

func (h *PublicInfoHandler) loadPublicGroupRates(ctx context.Context) ([]publicGroupRate, error) {
	if h.groupRepo == nil {
		return []publicGroupRate{}, nil
	}
	groups, err := h.groupRepo.ListActive(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]publicGroupRate, 0, len(groups))
	for i := range groups {
		g := groups[i]
		if !isPubliclyAvailableGroup(&g) {
			continue
		}
		out = append(out, publicGroupRate{
			ID:                   g.ID,
			Name:                 g.Name,
			Platform:             g.Platform,
			RateMultiplier:       g.RateMultiplier,
			AllowImageGeneration: g.AllowImageGeneration,
			ImageRateMultiplier:  g.ImageRateMultiplier,
		})
	}
	return out, nil
}

func isPubliclyAvailableGroup(g *service.Group) bool {
	if g == nil || !g.IsActive() {
		return false
	}
	if g.IsExclusive || g.IsSubscriptionType() {
		return false
	}
	return g.ActiveAccountCount > 0
}

func (h *PublicInfoHandler) loadPublicModelAvailability(ctx context.Context) ([]publicMonitorAvailability, error) {
	if h.monitorService == nil {
		return []publicMonitorAvailability{}, nil
	}
	if h.settingService != nil && !h.settingService.GetChannelMonitorRuntime(ctx).Enabled {
		return []publicMonitorAvailability{}, nil
	}
	views, err := h.monitorService.ListUserView(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]publicMonitorAvailability, 0, len(views))
	for _, view := range views {
		detail, err := h.monitorService.GetUserDetail(ctx, view.ID)
		if err != nil {
			return nil, err
		}
		models := make([]publicModelAvailability, 0, len(detail.Models))
		for _, model := range detail.Models {
			models = append(models, publicModelAvailability{
				Model:           model.Model,
				LatestStatus:    model.LatestStatus,
				Availability7d:  model.Availability7d,
				Availability15d: model.Availability15d,
				Availability30d: model.Availability30d,
			})
		}
		out = append(out, publicMonitorAvailability{
			ID:        detail.ID,
			Name:      detail.Name,
			Provider:  detail.Provider,
			GroupName: detail.GroupName,
			Models:    models,
			Timeline:  publicTimelineFromUserView(view.Timeline),
		})
	}
	return out, nil
}

func publicTimelineFromUserView(points []service.UserMonitorTimelinePoint) []publicMonitorTimelinePoint {
	timeline := make([]publicMonitorTimelinePoint, 0, len(points))
	for _, point := range points {
		timeline = append(timeline, publicMonitorTimelinePoint{
			Status:        point.Status,
			LatencyMs:     point.LatencyMs,
			PingLatencyMs: point.PingLatencyMs,
			CheckedAt:     point.CheckedAt.UTC().Format(time.RFC3339),
		})
	}
	return timeline
}

func (h *PublicInfoHandler) loadPublicRechargeInfo(ctx context.Context) (publicRechargeInfo, error) {
	if h.paymentConfig == nil {
		return publicRechargeInfo{BalanceRechargeMultiplier: 1}, nil
	}
	cfg, err := h.paymentConfig.GetPaymentConfig(ctx)
	if err != nil {
		return publicRechargeInfo{}, err
	}
	return publicRechargeInfo{
		PaymentEnabled:            cfg.Enabled,
		BalanceDisabled:           cfg.BalanceDisabled,
		BalanceRechargeMultiplier: cfg.BalanceRechargeMultiplier,
	}, nil
}
