//go:build unit

package middleware

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/Wei-Shaw/sub2api/internal/pkg/timezone"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func unavailableTimeTestKey() *service.APIKey {
	key := queueAuthTestKey(service.StatusActive, 1)
	key.Group.SubscriptionType = service.SubscriptionTypeSubscription
	key.Group.UnavailableTimeEnabled = true
	now := time.Now().In(timezone.Location())
	key.Group.UnavailableStart = now.Add(-time.Hour).Format("15:04")
	key.Group.UnavailableEnd = now.Add(time.Hour).Format("15:04")
	return key
}

func TestGroupUnavailableTimeRejectsBothAuthProtocols(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, google := range []bool{false, true} {
		key := unavailableTimeTestKey()
		repo := &stubApiKeyRepo{getByKey: func(context.Context, string) (*service.APIKey, error) { return key, nil }}
		cfg := &config.Config{RunMode: config.RunModeSimple}
		svc := service.NewAPIKeyService(repo, nil, nil, nil, nil, nil, cfg)
		router := gin.New()
		var rejected IngressRejectReason
		router.Use(func(c *gin.Context) { c.Next(); rejected, _ = GetIngressRejectReason(c) })
		if google {
			router.Use(APIKeyAuthGoogle(svc, cfg))
		} else {
			router.Use(gin.HandlerFunc(NewAPIKeyAuthMiddleware(svc, nil, cfg)))
		}
		router.GET("/test", func(c *gin.Context) { c.Status(http.StatusOK) })
		request := httptest.NewRequest(http.MethodGet, "/test", nil)
		request.Header.Set("x-api-key", key.Key)
		request.Header.Set("x-goog-api-key", key.Key)
		response := httptest.NewRecorder()
		router.ServeHTTP(response, request)
		require.Equal(t, http.StatusForbidden, response.Code, "google=%v body=%s", google, response.Body.String())
		require.Equal(t, IngressRejectGroupUnavailableTime, rejected)
	}
}

func TestGroupUnavailableTimeQueueRevalidation(t *testing.T) {
	current := unavailableTimeTestKey()
	current.Group.UnavailableTimeEnabled = false
	repo := &stubApiKeyRepo{getByKey: func(context.Context, string) (*service.APIKey, error) { return current, nil }}
	svc := service.NewAPIKeyService(repo, nil, nil, nil, nil, nil, &config.Config{})
	revalidate := newAPIKeyAuthRevalidator(svc.GetByKey, current.Key, "", current)
	current = unavailableTimeTestKey()
	_, err := revalidate(context.Background())
	require.Error(t, err)
	require.Equal(t, http.StatusForbidden, infraerrors.Code(err))
	require.Equal(t, "GROUP_UNAVAILABLE_TIME", infraerrors.Reason(err))
}
