package handler

import (
	"errors"
	"io"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/ip"
	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	middleware2 "github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"

	"github.com/gin-gonic/gin"
)

type DailyCheckinHandler struct {
	service          *service.DailyCheckinService
	turnstileService *service.TurnstileService
}

type DailyCheckinClaimRequest struct {
	TurnstileToken string `json:"turnstile_token"`
}

type DailyCheckinPublicStatus struct {
	Enabled          bool       `json:"enabled"`
	AdsEnabled       bool       `json:"ads_enabled"`
	CheckedInToday   bool       `json:"checked_in_today"`
	TodayReward      float64    `json:"today_reward"`
	RechargeEligible bool       `json:"recharge_eligible"`
	CheckinDate      string     `json:"checkin_date"`
	LastCheckinAt    *time.Time `json:"last_checkin_at,omitempty"`
	NextAvailableAt  time.Time  `json:"next_available_at"`
	ExhaustedToday   bool       `json:"exhausted_today"`
}

type DailyCheckinPublicResult struct {
	DailyCheckinPublicStatus
	Reward  float64 `json:"reward"`
	Balance float64 `json:"balance"`
}

func NewDailyCheckinHandler(service *service.DailyCheckinService, turnstileService *service.TurnstileService) *DailyCheckinHandler {
	return &DailyCheckinHandler{service: service, turnstileService: turnstileService}
}

// GetStatus returns the current user's daily check-in state.
// GET /api/v1/user/daily-checkin
func (h *DailyCheckinHandler) GetStatus(c *gin.Context) {
	subject, ok := middleware2.GetAuthSubjectFromContext(c)
	if !ok {
		response.Unauthorized(c, "User not authenticated")
		return
	}
	if h == nil || h.service == nil {
		response.InternalError(c, "Daily check-in service not configured")
		return
	}

	status, err := h.service.GetStatus(c.Request.Context(), subject.UserID)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, toDailyCheckinPublicStatus(status))
}

// Claim grants the current user's daily check-in reward.
// POST /api/v1/user/daily-checkin
func (h *DailyCheckinHandler) Claim(c *gin.Context) {
	subject, ok := middleware2.GetAuthSubjectFromContext(c)
	if !ok {
		response.Unauthorized(c, "User not authenticated")
		return
	}
	if h == nil || h.service == nil {
		response.InternalError(c, "Daily check-in service not configured")
		return
	}

	var req DailyCheckinClaimRequest
	if err := c.ShouldBindJSON(&req); err != nil && !errors.Is(err, io.EOF) {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}

	status, err := h.service.GetStatus(c.Request.Context(), subject.UserID)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	if !status.Enabled {
		response.ErrorFrom(c, service.ErrDailyCheckinDisabled)
		return
	}
	if status.CheckedInToday {
		response.ErrorFrom(c, service.ErrDailyCheckinAlready)
		return
	}
	if status.ExhaustedToday {
		response.ErrorFrom(c, service.ErrDailyCheckinExhausted)
		return
	}
	if !status.RechargeEligible {
		response.ErrorFrom(c, service.ErrDailyCheckinRechargeRequired)
		return
	}
	if h.turnstileService == nil || !h.turnstileService.IsEnabled(c.Request.Context()) {
		response.ErrorFrom(c, service.ErrTurnstileNotConfigured)
		return
	}
	if err := h.turnstileService.VerifyToken(c.Request.Context(), req.TurnstileToken, ip.GetClientIP(c)); err != nil {
		response.ErrorFrom(c, err)
		return
	}

	result, err := h.service.Claim(c.Request.Context(), subject.UserID)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, toDailyCheckinPublicResult(result))
}

func toDailyCheckinPublicStatus(status *service.DailyCheckinStatus) DailyCheckinPublicStatus {
	if status == nil {
		return DailyCheckinPublicStatus{}
	}
	return DailyCheckinPublicStatus{
		Enabled:          status.Enabled,
		AdsEnabled:       status.AdsEnabled,
		CheckedInToday:   status.CheckedInToday,
		TodayReward:      status.TodayReward,
		RechargeEligible: status.RechargeEligible,
		CheckinDate:      status.CheckinDate,
		LastCheckinAt:    status.LastCheckinAt,
		NextAvailableAt:  status.NextAvailableAt,
		ExhaustedToday:   status.ExhaustedToday,
	}
}

func toDailyCheckinPublicResult(result *service.DailyCheckinResult) DailyCheckinPublicResult {
	if result == nil {
		return DailyCheckinPublicResult{}
	}
	return DailyCheckinPublicResult{
		DailyCheckinPublicStatus: toDailyCheckinPublicStatus(&result.DailyCheckinStatus),
		Reward:                   result.Reward,
		Balance:                  result.Balance,
	}
}
