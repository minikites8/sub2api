package routes

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/handler"
	servermiddleware "github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/Wei-Shaw/sub2api/internal/web"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type ccSwitchUserRepo struct {
	service.UserRepository
	user *service.User
	err  error
}

func (r *ccSwitchUserRepo) GetByID(context.Context, int64) (*service.User, error) {
	return r.user, r.err
}

type ccSwitchGroupRepo struct {
	service.GroupRepository
	groups map[int64]*service.Group
}

func (r *ccSwitchGroupRepo) GetByID(_ context.Context, id int64) (*service.Group, error) {
	if group := r.groups[id]; group != nil {
		return group, nil
	}
	return nil, service.ErrGroupNotFound
}

func (r *ccSwitchGroupRepo) GetByIDLite(ctx context.Context, id int64) (*service.Group, error) {
	return r.GetByID(ctx, id)
}

type ccSwitchSubRepo struct {
	service.UserSubscriptionRepository
	subs map[int64]*service.UserSubscription
}

func (r *ccSwitchSubRepo) GetActiveByUserIDAndGroupID(_ context.Context, _, groupID int64) (*service.UserSubscription, error) {
	if sub := r.subs[groupID]; sub != nil {
		return sub, nil
	}
	return nil, service.ErrSubscriptionNotFound
}

type ccSwitchKeyRepo struct {
	keyBillingRouteAPIKeyRepo
	touches int
}

func (r *ccSwitchKeyRepo) UpdateLastUsed(context.Context, int64, time.Time) error {
	r.touches++
	return nil
}

func TestCCSwitchUsageAuthenticatedRoutes(t *testing.T) {
	for _, tc := range []struct {
		name        string
		method      string
		credentials string
		configure   func(*service.APIKey, *ccSwitchUserRepo, map[int64]*service.UserSubscription)
		wantStatus  int
		wantGroup   int64
		wantError   string
	}{
		{name: "empty POST chooses first funded group", method: http.MethodPost, credentials: "valid", wantStatus: 200, wantGroup: 2},
		{name: "GET alias", method: http.MethodGet, credentials: "valid", wantStatus: 200, wantGroup: 2},
		{name: "missing credentials", method: http.MethodPost, wantStatus: 401},
		{name: "wrong credentials", method: http.MethodPost, credentials: "invalid", wantStatus: 401},
		{name: "disabled key", method: http.MethodPost, credentials: "valid", wantStatus: 401, configure: func(key *service.APIKey, _ *ccSwitchUserRepo, _ map[int64]*service.UserSubscription) {
			key.Status = service.StatusDisabled
		}},
		{name: "IP restriction", method: http.MethodPost, credentials: "valid", wantStatus: 403, configure: func(key *service.APIKey, _ *ccSwitchUserRepo, _ map[int64]*service.UserSubscription) {
			key.IPWhitelist = []string{"192.0.2.0/24"}
		}},
		{name: "expired key can report invalidity", method: http.MethodPost, credentials: "valid", wantStatus: 200, wantError: "API_KEY_EXPIRED", configure: func(key *service.APIKey, _ *ccSwitchUserRepo, _ map[int64]*service.UserSubscription) {
			key.Status = service.StatusAPIKeyExpired
		}},
		{name: "key cap exhausted", method: http.MethodPost, credentials: "valid", wantStatus: 200, wantError: "API_KEY_QUOTA_EXHAUSTED", configure: func(key *service.APIKey, _ *ccSwitchUserRepo, _ map[int64]*service.UserSubscription) {
			key.Quota, key.QuotaUsed = 1, 1
		}},
		{name: "no remaining quota", method: http.MethodPost, credentials: "valid", wantStatus: 200, wantError: "NO_AVAILABLE_GROUP", configure: func(_ *service.APIKey, _ *ccSwitchUserRepo, subs map[int64]*service.UserSubscription) {
			subs[2].DailyUsageUSD = 10
		}},
		{name: "earlier group recovers", method: http.MethodPost, credentials: "valid", wantStatus: 200, wantGroup: 1, configure: func(_ *service.APIKey, _ *ccSwitchUserRepo, subs map[int64]*service.UserSubscription) {
			subs[1].DailyUsageUSD = 4
		}},
		{name: "query failure is unavailable", method: http.MethodPost, credentials: "valid", wantStatus: 503, configure: func(_ *service.APIKey, users *ccSwitchUserRepo, _ map[int64]*service.UserSubscription) {
			users.err = errors.New("database down")
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			gin.SetMode(gin.TestMode)
			now, limit := time.Now(), 10.0
			groups := map[int64]*service.Group{
				1: {ID: 1, Name: "First subscription", Status: service.StatusActive, Hydrated: true, Platform: service.PlatformOpenAI, SubscriptionType: service.SubscriptionTypeSubscription, RateMultiplier: 2, DailyLimitUSD: &limit},
				2: {ID: 2, Name: "Second subscription", Status: service.StatusActive, Hydrated: true, Platform: service.PlatformAnthropic, SubscriptionType: service.SubscriptionTypeSubscription, RateMultiplier: 2, DailyLimitUSD: &limit},
			}
			subs := map[int64]*service.UserSubscription{}
			for id := range groups {
				subs[id] = &service.UserSubscription{UserID: 7, GroupID: id, Status: service.SubscriptionStatusActive, ExpiresAt: now.Add(time.Hour), DailyWindowStart: &now, DailyUsageUSD: 4}
			}
			subs[1].DailyUsageUSD = 10
			user := &service.User{ID: 7, Status: service.StatusActive, Role: service.RoleUser}
			users := &ccSwitchUserRepo{user: user}
			key := &service.APIKey{ID: 99, UserID: 7, Key: "cc-switch-local-test", Status: service.StatusActive, GroupID: &groups[1].ID, GroupIDs: []int64{1, 2}, Group: groups[1], User: user, ConcurrencyLimit: 1}
			if tc.configure != nil {
				tc.configure(key, users, subs)
			}
			keys := &ccSwitchKeyRepo{keyBillingRouteAPIKeyRepo: keyBillingRouteAPIKeyRepo{apiKey: key}}
			cfg := &config.Config{RunMode: config.RunModeStandard, Gateway: config.GatewayConfig{MaxBodySize: 1024}}
			svc := service.NewAPIKeyService(keys, users, &ccSwitchGroupRepo{groups: groups}, &ccSwitchSubRepo{subs: subs}, &keyBillingRouteRateRepo{}, nil, cfg)
			router := gin.New()
			if web.HasEmbeddedFrontend() {
				router.Use(web.ServeEmbeddedFrontend())
			}
			RegisterGatewayRoutes(router, &handler.Handlers{Gateway: &handler.GatewayHandler{}, OpenAIGateway: &handler.OpenAIGatewayHandler{}}, servermiddleware.NewAPIKeyAuthMiddleware(svc, nil, cfg), svc, nil, nil, nil, nil, cfg)
			req := httptest.NewRequest(tc.method, "/api/usage", nil)
			req.RemoteAddr = "127.0.0.1:12345"
			req.Header.Set("User-Agent", "cc-switch/1.0")
			if tc.credentials != "" {
				credential := key.Key
				if tc.credentials != "valid" {
					credential = "invalid-key"
				}
				req.Header.Set("Authorization", "Bearer "+credential)
			}
			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)
			require.Equal(t, tc.wantStatus, w.Code, w.Body.String())
			require.Contains(t, w.Header().Get("Content-Type"), "application/json")
			require.NotContains(t, strings.ToLower(w.Body.String()), "<!doctype")
			require.NotContains(t, w.Body.String(), key.Key)
			require.Zero(t, keys.touches, "balance polling is read-only")
			if tc.wantStatus != http.StatusOK {
				var failure struct {
					Error json.RawMessage `json:"error"`
				}
				require.NoError(t, json.Unmarshal(w.Body.Bytes(), &failure))
				require.NotEmpty(t, failure.Error, "the supplied extractor requires a top-level error for every failure")
				return
			}
			require.Equal(t, "no-store", w.Header().Get("Cache-Control"))
			var got service.APIKeyGroupQuota
			require.NoError(t, json.Unmarshal(w.Body.Bytes(), &got))
			require.Equal(t, tc.wantGroup, got.GroupID)
			if tc.wantError != "" {
				require.False(t, got.IsValid)
				require.NotNil(t, got.Error)
				require.Equal(t, tc.wantError, got.Error.Code)
				return
			}
			require.True(t, got.IsValid)
			require.Nil(t, got.Error)
			require.Equal(t, 3.0, got.Balance)
			require.Equal(t, 5.0, *got.Total)
			require.Equal(t, 2.0, *got.Used)
			require.Equal(t, groups[tc.wantGroup].Name, got.PlanName)
		})
	}
}
