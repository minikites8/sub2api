package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestAdminAPIKeyAutoPoolPathAllowed(t *testing.T) {
	gin.SetMode(gin.TestMode)
	tests := []struct {
		name   string
		method string
		path   string
		allow  bool
	}{
		{name: "accounts", method: http.MethodPost, path: "/api/v1/admin/accounts", allow: true},
		{name: "oauth account sessions", method: http.MethodGet, path: "/api/v1/admin/openai/accounts/7/sessions", allow: true},
		{name: "groups", method: http.MethodPut, path: "/api/v1/admin/groups/2", allow: true},
		{name: "usage", method: http.MethodGet, path: "/api/v1/admin/usage/stats", allow: true},
		{name: "usage cleanup", method: http.MethodPost, path: "/api/v1/admin/usage/cleanup-tasks", allow: false},
		{name: "dashboard usage query", method: http.MethodGet, path: "/api/v1/admin/dashboard/groups", allow: true},
		{name: "dashboard batch usage", method: http.MethodPost, path: "/api/v1/admin/dashboard/api-keys-usage", allow: true},
		{name: "api key group update", method: http.MethodPut, path: "/api/v1/admin/api-keys/4/group", allow: true},
		{name: "user usage", method: http.MethodGet, path: "/api/v1/admin/users/4/usage", allow: true},
		{name: "user api keys", method: http.MethodGet, path: "/api/v1/admin/users/4/api-keys", allow: true},
		{name: "settings", method: http.MethodGet, path: "/api/v1/admin/settings", allow: false},
		{name: "user update", method: http.MethodPut, path: "/api/v1/admin/users/4", allow: false},
		{name: "dashboard mutation", method: http.MethodPost, path: "/api/v1/admin/dashboard/aggregation/backfill", allow: false},
		{name: "ops dashboard", method: http.MethodGet, path: "/api/v1/admin/ops/dashboard/overview", allow: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(tt.method, tt.path, nil)
			ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
			ctx.Request = req
			require.Equal(t, tt.allow, adminAPIKeyAutoPoolPathAllowed(ctx))
		})
	}
}

func TestAuthorizeAdminAPIKeyRequest(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, tt := range []struct {
		name       string
		permission string
		path       string
		status     int
	}{
		{name: "full allows settings", permission: service.AdminAPIKeyPermissionFull, path: "/api/v1/admin/settings", status: http.StatusOK},
		{name: "auto pool allows accounts", permission: service.AdminAPIKeyPermissionAutoPool, path: "/api/v1/admin/accounts", status: http.StatusOK},
		{name: "auto pool forbids settings", permission: service.AdminAPIKeyPermissionAutoPool, path: "/api/v1/admin/settings", status: http.StatusForbidden},
	} {
		t.Run(tt.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			ctx, _ := gin.CreateTestContext(w)
			ctx.Request = httptest.NewRequest(http.MethodGet, tt.path, nil)
			ctx.Set(string(ContextKeyAdminAPIKeyPermission), tt.permission)
			require.Equal(t, tt.status == http.StatusOK, authorizeAdminAPIKeyRequest(ctx))
			require.Equal(t, tt.status, w.Code)
		})
	}
}
