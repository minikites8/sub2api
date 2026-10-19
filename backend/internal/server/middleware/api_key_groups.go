package middleware

import (
	"net/http"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
)

func selectOrderedAPIKeyGroup(c *gin.Context, keys *service.APIKeyService, subscriptions *service.SubscriptionService, key *service.APIKey, googleOnly bool) (*service.APIKey, bool) {
	if len(key.OrderedGroupIDs()) <= 1 {
		return key, true
	}
	var models []string
	if model := groupModelAllowlistModelFromParams(c); model != "" {
		models = []string{model}
	} else if c.Request.Method == http.MethodPost || c.Request.Method == http.MethodPut || c.Request.Method == http.MethodPatch {
		var ok bool
		models, ok = groupModelAllowlistModelsFromBody(c)
		if !ok {
			return nil, false
		}
	} else if model := c.Query("model"); model != "" {
		models = []string{model}
	}
	path := c.Request.URL.Path
	showcaseRead := (c.Request.Method == http.MethodGet || c.Request.Method == http.MethodHead) && isPublicPelicanShowcasePath(path)
	metadata := path == "/v1/usage" || path == "/v1/sub2api/billing" || isAsyncImageTaskRead(c.Request.Method, path) || showcaseRead
	capacity := !metadata && (len(models) > 0 || isResponsesWebSocketRoute(c))
	var requiresNonStreamGroup bool
	if c.Request.Method == http.MethodPost && streamOnlyJSONRoutes[c.FullPath()] {
		body, ok := readAdmissionRequestBody(c)
		if !ok {
			return nil, false
		}
		requiresNonStreamGroup = !jsonBodyRequestsStream(body)
	} else if streamOnlyGeminiRoutes[c.FullPath()] {
		requiresNonStreamGroup = strings.HasSuffix(c.Param("modelAction"), ":generateContent")
	}
	selected, err := keys.SelectOrderedGroup(c.Request.Context(), key, service.APIKeyGroupSelection{
		Models: models, GoogleOnly: googleOnly, CheckCapacity: capacity, CheckBilling: !metadata,
		AcceptGroup: func(group *service.Group) bool { return !requiresNonStreamGroup || !group.StreamOnly },
	}, subscriptions)
	if err != nil {
		if googleOnly {
			abortWithGoogleError(c, http.StatusServiceUnavailable, "No available bound group can serve this request")
		} else {
			AbortWithError(c, http.StatusServiceUnavailable, "API_KEY_GROUPS_UNAVAILABLE", "No available bound group can serve this request")
		}
		return nil, false
	}
	SetOpsFallbackAPIKey(c, selected)
	return selected, true
}
