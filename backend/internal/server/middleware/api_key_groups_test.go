//go:build unit

package middleware

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/ctxkey"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type orderedAuthGroups struct {
	service.GroupRepository
	groups map[int64]*service.Group
}

func (r *orderedAuthGroups) GetByID(_ context.Context, id int64) (*service.Group, error) {
	if group := r.groups[id]; group != nil {
		return group, nil
	}
	return nil, service.ErrGroupNotFound
}

func (r *orderedAuthGroups) GetByIDLite(ctx context.Context, id int64) (*service.Group, error) {
	return r.GetByID(ctx, id)
}

func TestAPIKeyOrderedGroupsAuthUsesSelectedGroup(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, google := range []bool{false, true} {
		for _, mode := range []string{"disabled", "nonstream", "all unavailable", "key quota"} {
			t.Run(mode+map[bool]string{true: " google", false: " standard"}[google], func(t *testing.T) {
				first := &service.Group{ID: 1, Hydrated: true, Platform: service.PlatformGemini, Status: service.StatusActive}
				second := &service.Group{ID: 2, Hydrated: true, Platform: service.PlatformGemini, Status: service.StatusActive, RateMultiplier: 3}
				groups := &orderedAuthGroups{groups: map[int64]*service.Group{1: first, 2: second}}
				if mode == "nonstream" {
					first.StreamOnly = true
				} else {
					first.Status = service.StatusDisabled
				}
				if mode == "all unavailable" {
					second.Status = service.StatusDisabled
				}
				key := &service.APIKey{ID: 9, UserID: 7, Key: "ordered-test", Status: service.StatusActive,
					GroupID: &first.ID, GroupIDs: []int64{1, 2}, Group: first,
					User: &service.User{ID: 7, Status: service.StatusActive, Balance: 10}}
				if mode == "key quota" {
					key.Quota = 1
					key.QuotaUsed = 1
				}
				repo := fakeAPIKeyRepo{getByKey: func(context.Context, string) (*service.APIKey, error) { copyKey := *key; return &copyKey, nil }}
				cfg := &config.Config{}
				keys := service.NewAPIKeyService(repo, nil, groups, nil, nil, nil, cfg)
				router := gin.New()
				path, route := "/v1/messages", "/v1/messages"
				if google {
					path, route = "/v1beta/models/gemini-test:generateContent", "/v1beta/models/*modelAction"
					router.Use(APIKeyAuthWithSubscriptionGoogle(keys, nil, cfg))
				} else {
					router.Use(gin.HandlerFunc(NewAPIKeyAuthMiddleware(keys, nil, cfg)))
				}
				body := `{"model":"gemini-test","stream":false}`
				called := false
				router.POST(route, func(c *gin.Context) {
					called = true
					selected, ok := GetAPIKeyFromContext(c)
					require.True(t, ok)
					require.EqualValues(t, 2, *selected.GroupID)
					require.Equal(t, 3.0, selected.Group.RateMultiplier)
					require.EqualValues(t, 2, c.Request.Context().Value(ctxkey.Group).(*service.Group).ID)
					forwardBody, err := io.ReadAll(c.Request.Body)
					require.NoError(t, err)
					require.Equal(t, body, string(forwardBody))
					c.Status(http.StatusOK)
				})
				request := httptest.NewRequest(http.MethodPost, path, strings.NewReader(body))
				request.Header.Set("Content-Type", "application/json")
				request.Header.Set("x-api-key", key.Key)
				response := httptest.NewRecorder()
				router.ServeHTTP(response, request)
				switch mode {
				case "all unavailable":
					require.Equal(t, http.StatusServiceUnavailable, response.Code)
					require.False(t, called)
				case "key quota":
					require.Equal(t, http.StatusTooManyRequests, response.Code)
					require.False(t, called)
				default:
					require.Equal(t, http.StatusOK, response.Code)
					require.True(t, called)
				}
				require.EqualValues(t, 1, *key.GroupID)
			})
		}
	}
}

func TestAPIKeyOrderedGroupsQueueKeepsSelectedIdentity(t *testing.T) {
	groups := &orderedAuthGroups{groups: map[int64]*service.Group{
		1: {ID: 1, Hydrated: true, Platform: service.PlatformOpenAI, Status: service.StatusActive},
		2: {ID: 2, Hydrated: true, Platform: service.PlatformOpenAI, Status: service.StatusActive},
	}}
	primary := groups.groups[1]
	key := &service.APIKey{ID: 9, UserID: 7, Key: "ordered-test", Status: service.StatusActive,
		ConcurrencyLimit: 4, GroupID: &primary.ID, GroupIDs: []int64{1, 2}, Group: primary,
		User: &service.User{ID: 7, Status: service.StatusActive, Balance: 10}}
	repo := fakeAPIKeyRepo{getByKey: func(context.Context, string) (*service.APIKey, error) { copyKey := *key; return &copyKey, nil }}
	keys := service.NewAPIKeyService(repo, nil, groups, nil, nil, nil, &config.Config{})
	selected, err := keys.APIKeyInBoundGroup(context.Background(), key, 2)
	require.NoError(t, err)
	revalidate := newAPIKeyQueueAuthRevalidator(keys, key.Key, "127.0.0.1", selected)
	limit, err := revalidate(context.Background())
	require.NoError(t, err)
	require.Equal(t, 4, limit)
	key.GroupIDs = []int64{1}
	_, err = revalidate(context.Background())
	require.Error(t, err, "removing the selected binding must revoke the queued request")
}
