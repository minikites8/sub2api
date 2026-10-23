package routes

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/handler"
	servermiddleware "github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type multiPeriodUsageKeyRepo struct {
	ccSwitchKeyRepo
	rate      *service.APIKeyRateLimitData
	err       error
	rateErr   error
	freshUsed *float64
}

type multiPeriodUsageUserRepo struct{ ccSwitchUserRepo }

func (r *multiPeriodUsageUserRepo) GetUserAvatar(context.Context, int64) (*service.UserAvatar, error) {
	return nil, nil
}

func (r *multiPeriodUsageKeyRepo) GetByID(_ context.Context, id int64) (*service.APIKey, error) {
	if r.err != nil {
		return nil, r.err
	}
	if r.apiKey == nil || r.apiKey.ID != id {
		return nil, service.ErrAPIKeyNotFound
	}
	copyKey := *r.apiKey
	if r.freshUsed != nil {
		copyKey.QuotaUsed = *r.freshUsed
	}
	return &copyKey, nil
}

func (r *multiPeriodUsageKeyRepo) GetRateLimitData(context.Context, int64) (*service.APIKeyRateLimitData, error) {
	return r.rate, r.rateErr
}

type multiPeriodUsageSubRepo struct {
	ccSwitchSubRepo
	err error
}

func (r *multiPeriodUsageSubRepo) GetActiveByUserIDAndGroupID(ctx context.Context, userID, groupID int64) (*service.UserSubscription, error) {
	if r.err != nil {
		return nil, r.err
	}
	return r.ccSwitchSubRepo.GetActiveByUserIDAndGroupID(ctx, userID, groupID)
}

func TestCCSwitchMultiPeriodUsageAuthenticatedRoutes(t *testing.T) {
	for _, mode := range []string{config.RunModeStandard, config.RunModeSimple} {
		for _, scenario := range []string{"subscription", "daily_only", "daily_exhausted", "weekly_exhausted", "unlimited", "wallet", "key_quota", "subscription_and_key_quota", "subscription_and_rates", "wallet_and_rates", "expired_key", "runtime_expired_key", "missing_subscription", "wrong_user_subscription", "disabled_key", "unknown_key", "missing_key", "reset_windows", "key_exhausted", "expired_subscription", "rates_reset", "rate_data_missing", "rate_data_error", "key_read_error", "subscription_read_error", "fresh_quota"} {
			t.Run(mode+"/"+scenario, func(t *testing.T) {
				gin.SetMode(gin.TestMode)
				now := time.Now()
				daily, weekly, monthly := 90.0, 500.0, 1300.0
				group := &service.Group{ID: 11, Name: "Pro stable", Status: service.StatusActive, Hydrated: true, Platform: service.PlatformOpenAI, SubscriptionType: service.SubscriptionTypeSubscription, DailyLimitUSD: &daily, WeeklyLimitUSD: &weekly, MonthlyLimitUSD: &monthly}
				user := &service.User{ID: 7, Status: service.StatusActive, Role: service.RoleUser, Balance: 8}
				key := &service.APIKey{ID: 99, UserID: user.ID, User: user, Key: "cc-switch-synthetic-key", Status: service.StatusActive, GroupID: &group.ID, Group: group}
				sub := &service.UserSubscription{UserID: user.ID, GroupID: group.ID, Status: service.SubscriptionStatusActive, StartsAt: now.Add(-time.Hour), ExpiresAt: now.AddDate(0, 3, 0), DailyWindowStart: &now, WeeklyWindowStart: &now, MonthlyWindowStart: &now, DailyUsageUSD: 32, WeeklyUsageUSD: 285, MonthlyUsageUSD: 720}
				subs := map[int64]*service.UserSubscription{group.ID: sub}
				switch scenario {
				case "daily_only":
					group.WeeklyLimitUSD, group.MonthlyLimitUSD = nil, nil
				case "daily_exhausted":
					sub.DailyUsageUSD = 90
				case "weekly_exhausted":
					sub.WeeklyUsageUSD = 500
				case "unlimited":
					group.DailyLimitUSD, group.WeeklyLimitUSD, group.MonthlyLimitUSD = nil, nil, nil
				case "wallet", "key_quota", "wallet_and_rates", "rates_reset":
					group.SubscriptionType = service.SubscriptionTypeStandard
				case "expired_key":
					key.Status = service.StatusAPIKeyExpired
				case "runtime_expired_key":
					expired := now.Add(-time.Hour)
					key.ExpiresAt = &expired
				case "missing_subscription":
					delete(subs, group.ID)
				case "wrong_user_subscription":
					sub.UserID = 8
				case "expired_subscription":
					sub.ExpiresAt = now.Add(-time.Minute)
				case "key_exhausted":
					key.Quota, key.QuotaUsed = 100, 100
				case "disabled_key":
					key.Status = service.StatusDisabled
				case "reset_windows":
					old := now.AddDate(0, 0, -32)
					sub.StartsAt = old
					sub.DailyWindowStart, sub.WeeklyWindowStart, sub.MonthlyWindowStart = &old, &old, &old
				}
				if scenario == "key_quota" || scenario == "subscription_and_key_quota" || scenario == "fresh_quota" {
					key.Quota, key.QuotaUsed = 100, 10
				}
				if scenario == "subscription_and_rates" || scenario == "wallet_and_rates" || scenario == "rates_reset" || scenario == "rate_data_missing" || scenario == "rate_data_error" {
					key.RateLimit5h = 10
				}
				keys := &multiPeriodUsageKeyRepo{ccSwitchKeyRepo: ccSwitchKeyRepo{keyBillingRouteAPIKeyRepo: keyBillingRouteAPIKeyRepo{apiKey: key}}, rate: &service.APIKeyRateLimitData{Usage5h: 3, Window5hStart: &now}}
				if scenario == "rate_data_missing" {
					keys.rate = nil
				}
				if scenario == "rate_data_error" {
					keys.rateErr = errors.New("synthetic dependency error")
				}
				if scenario == "key_read_error" {
					keys.err = errors.New("synthetic dependency error")
				}
				if scenario == "rates_reset" {
					old := now.Add(-6 * time.Hour)
					keys.rate.Window5hStart = &old
				}
				if scenario == "fresh_quota" {
					used := 20.0
					keys.freshUsed = &used
				}
				subRepo := &multiPeriodUsageSubRepo{ccSwitchSubRepo: ccSwitchSubRepo{subs: subs}}
				if scenario == "subscription_read_error" {
					subRepo.err = errors.New("synthetic dependency error")
				}
				users := &multiPeriodUsageUserRepo{ccSwitchUserRepo: ccSwitchUserRepo{user: user}}
				cfg := &config.Config{RunMode: mode}
				keyService := service.NewAPIKeyService(keys, users, &ccSwitchGroupRepo{groups: map[int64]*service.Group{group.ID: group}}, subRepo, &keyBillingRouteRateRepo{}, nil, cfg)
				gateway := handler.NewGatewayHandler(nil, nil, nil, nil, service.NewUserService(users, nil, nil, nil), nil, nil, nil, keyService, nil, nil, nil, nil, nil, cfg, nil)
				router := gin.New()
				RegisterGatewayRoutes(router, &handler.Handlers{Gateway: gateway, OpenAIGateway: &handler.OpenAIGatewayHandler{}}, servermiddleware.NewAPIKeyAuthMiddleware(keyService, nil, cfg), keyService, nil, nil, nil, nil, cfg)
				req := httptest.NewRequest(http.MethodGet, "/v1/usage", nil)
				req.Header.Set("User-Agent", "cc-switch/1.0")
				if scenario != "missing_key" {
					credential := key.Key
					if scenario == "unknown_key" {
						credential = "unknown-key"
					}
					req.Header.Set("Authorization", "Bearer "+credential)
				}
				w := httptest.NewRecorder()
				router.ServeHTTP(w, req)
				require.NotContains(t, w.Body.String(), key.Key)
				require.Zero(t, keys.touches, "usage lookup must not mutate last_used")
				if scenario == "rate_data_missing" || scenario == "rate_data_error" || scenario == "key_read_error" || scenario == "subscription_read_error" {
					require.Equal(t, http.StatusServiceUnavailable, w.Code, w.Body.String())
					require.Contains(t, w.Body.String(), "error")
					return
				}
				if scenario == "missing_key" || scenario == "unknown_key" || scenario == "disabled_key" {
					require.Equal(t, 401, w.Code)
					return
				}
				require.Equal(t, 200, w.Code, w.Body.String())
				require.Equal(t, "no-store", w.Header().Get("Cache-Control"))
				var got struct {
					IsValid        bool                                      `json:"isValid"`
					InvalidMessage string                                    `json:"invalidMessage"`
					Remaining      float64                                   `json:"remaining"`
					Balance        *float64                                  `json:"balance"`
					Subscription   *service.SubscriptionUsageSnapshot        `json:"subscription"`
					Quota          *struct{ Limit, Used, Remaining float64 } `json:"quota"`
					Rates          []struct {
						Window                 string
						Limit, Used, Remaining float64
					} `json:"rate_limits"`
				}
				require.NoError(t, json.Unmarshal(w.Body.Bytes(), &got))
				invalid := scenario == "expired_key" || scenario == "runtime_expired_key" || scenario == "missing_subscription" || scenario == "wrong_user_subscription" || scenario == "expired_subscription" || scenario == "key_exhausted"
				require.Equal(t, !invalid, got.IsValid)
				if invalid {
					require.NotEmpty(t, got.InvalidMessage)
					return
				}
				if group.IsSubscriptionType() {
					require.NotNil(t, got.Subscription)
					if scenario == "reset_windows" {
						require.Zero(t, got.Subscription.DailyUsageUSD)
						require.Zero(t, got.Subscription.WeeklyUsageUSD)
						require.Zero(t, got.Subscription.MonthlyUsageUSD)
						require.Nil(t, got.Subscription.DailyResetAt)
					} else {
						require.Equal(t, sub.DailyUsageUSD, got.Subscription.DailyUsageUSD)
					}
					if scenario == "daily_exhausted" || scenario == "weekly_exhausted" {
						require.Zero(t, got.Remaining)
						require.Equal(t, 580.0, *got.Subscription.MonthlyRemainingUSD)
					}
					if scenario == "unlimited" {
						require.Equal(t, -1.0, got.Remaining)
						require.True(t, got.Subscription.Unlimited)
					}
				} else {
					require.Nil(t, got.Subscription)
				}
				if key.Quota > 0 {
					require.NotNil(t, got.Quota)
					if scenario == "fresh_quota" {
						require.Equal(t, 80.0, got.Remaining)
					} else {
						require.Equal(t, 90.0, got.Remaining)
					}
				}
				if key.HasRateLimits() {
					require.Len(t, got.Rates, 1)
					require.Equal(t, "5h", got.Rates[0].Window)
					if scenario == "rates_reset" {
						require.Equal(t, 10.0, got.Rates[0].Remaining)
						require.Zero(t, got.Rates[0].Used)
					} else {
						require.Equal(t, 7.0, got.Rates[0].Remaining)
					}
				}
				if scenario == "wallet" || scenario == "wallet_and_rates" {
					require.Equal(t, 8.0, got.Remaining)
					require.Equal(t, 8.0, *got.Balance)
				}
			})
		}
	}
}
