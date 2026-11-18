package admin

import (
	"net/http"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestGroupHandlerPassesCodexModelsManifestConfigToService(t *testing.T) {
	gin.SetMode(gin.TestMode)
	svc := newStubAdminService()
	h := NewGroupHandlerWithConfig(svc, nil, nil, &config.Config{})
	r := gin.New()
	r.POST("/groups", h.Create)
	r.PUT("/groups/:id", h.Update)

	res := serveGroupRequest(r, http.MethodPost, "/groups", `{"name":"pinned","platform":"openai","codex_models_manifest_config":{"enabled":false,"account_ids":[17]}}`)
	require.Equal(t, http.StatusOK, res.Code, res.Body.String())
	require.Equal(t, service.GroupCodexModelsManifestConfig{AccountIDs: []int64{17}}, svc.createdGroups[0].CodexModelsManifestConfig)

	for _, tc := range []struct {
		name, body string
		want       *service.GroupCodexModelsManifestConfig
	}{
		{"enable", `{"codex_models_manifest_config":{"enabled":true,"account_ids":[17,4],"fallback_to_scheduler":true}}`, &service.GroupCodexModelsManifestConfig{Enabled: true, AccountIDs: []int64{17, 4}, FallbackToScheduler: true}},
		{"disable", `{"codex_models_manifest_config":{"enabled":false}}`, &service.GroupCodexModelsManifestConfig{}},
		{"omit", `{"name":"renamed"}`, nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			res := serveGroupRequest(r, http.MethodPut, "/groups/2", tc.body)
			require.Equal(t, http.StatusOK, res.Code, res.Body.String())
			require.Equal(t, tc.want, svc.updatedGroups[len(svc.updatedGroups)-1].CodexModelsManifestConfig)
		})
	}
}
