package admin

import (
	"net/http"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestGroupHandlerPassesModelAllowlistToService(t *testing.T) {
	gin.SetMode(gin.TestMode)
	svc := newStubAdminService()
	h := NewGroupHandlerWithConfig(svc, nil, nil, &config.Config{})
	r := gin.New()
	r.POST("/groups", h.Create)
	r.PUT("/groups/:id", h.Update)

	res := serveGroupRequest(r, http.MethodPost, "/groups", `{"name":"allowlist","platform":"openai","rate_multiplier":1,"model_allowlist":{"enabled":true,"models":["gpt-5.4"]}}`)
	require.Equal(t, http.StatusOK, res.Code, res.Body.String())
	require.Equal(t, service.GroupModelAllowlist{Enabled: true, Models: []string{"gpt-5.4"}}, svc.createdGroups[0].ModelAllowlist)

	for _, test := range []struct {
		name string
		body string
		want *service.GroupModelAllowlist
	}{
		{"disable retaining models", `{"model_allowlist":{"enabled":false,"models":["gpt-5.4"]}}`, &service.GroupModelAllowlist{Models: []string{"gpt-5.4"}}},
		{"disable clearing models", `{"model_allowlist":{"enabled":false,"models":[]}}`, &service.GroupModelAllowlist{Models: []string{}}},
		{"enable", `{"model_allowlist":{"enabled":true,"models":["gpt-5.4"]}}`, &service.GroupModelAllowlist{Enabled: true, Models: []string{"gpt-5.4"}}},
		{"preserve omitted config", `{"name":"renamed"}`, nil},
	} {
		t.Run(test.name, func(t *testing.T) {
			res := serveGroupRequest(r, http.MethodPut, "/groups/2", test.body)
			require.Equal(t, http.StatusOK, res.Code, res.Body.String())
			require.Equal(t, test.want, svc.updatedGroups[len(svc.updatedGroups)-1].ModelAllowlist)
		})
	}
}

func TestGroupHandlerSimpleModeSanitizesModelAllowlist(t *testing.T) {
	gin.SetMode(gin.TestMode)
	svc := newStubAdminService()
	r := newSimpleModeGroupRouter(svc)
	res := serveGroupRequest(r, http.MethodPost, "/groups", `{"name":"simple","platform":"openai","model_allowlist":{"enabled":true,"models":["gpt-5.4"]}}`)
	require.Equal(t, http.StatusOK, res.Code, res.Body.String())
	require.False(t, svc.createdGroups[0].ModelAllowlist.Enabled)
	require.Empty(t, svc.createdGroups[0].ModelAllowlist.Models)
	res = serveGroupRequest(r, http.MethodPut, "/groups/2", `{"model_allowlist":{"enabled":false,"models":["gpt-5.4"]}}`)
	require.Equal(t, http.StatusOK, res.Code, res.Body.String())
	require.Nil(t, svc.updatedGroups[0].ModelAllowlist)
}
