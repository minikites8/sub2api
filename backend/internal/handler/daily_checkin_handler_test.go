package handler

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
)

func TestDailyCheckinPublicStatusOmitsRewardParameters(t *testing.T) {
	now := time.Date(2026, 7, 16, 0, 0, 0, 0, time.UTC)
	status := service.DailyCheckinStatus{
		Enabled:           true,
		AdsEnabled:        true,
		CheckedInToday:    true,
		TodayReward:       0.5,
		TodayTotalGranted: 20,
		DailyTotalLimit:   100,
		MinReward:         0.1,
		MaxReward:         1,
		MinRechargeAmount: 5,
		TotalRecharged:    3,
		RechargeEligible:  false,
		CheckinDate:       "2026-07-16",
		LastCheckinAt:     &now,
		LastReward:        0.5,
		NextAvailableAt:   now.Add(24 * time.Hour),
		RemainingToday:    80,
		ExhaustedToday:    false,
	}

	assertDailyCheckinPayloadOmitsRewardParameters(t, toDailyCheckinPublicStatus(&status))
	assertDailyCheckinPayloadOmitsRewardParameters(t, toDailyCheckinPublicResult(&service.DailyCheckinResult{
		DailyCheckinStatus: status,
		Reward:             0.5,
		Balance:            12,
	}))
}

func assertDailyCheckinPayloadOmitsRewardParameters(t *testing.T, payload any) {
	t.Helper()

	raw, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal public daily check-in payload: %v", err)
	}
	body := string(raw)
	for _, field := range []string{
		"today_total_granted",
		"daily_total_limit",
		"min_reward",
		"max_reward",
		"min_recharge_amount",
		"total_recharged",
		"remaining_today",
		"last_reward",
	} {
		if strings.Contains(body, field) {
			t.Fatalf("public payload contains %q: %s", field, body)
		}
	}
}
