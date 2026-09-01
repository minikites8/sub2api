package admin

import (
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func TestMergeWorkspaceMetadataCredentialsPreservesExistingCredentials(t *testing.T) {
	existing := map[string]any{
		"access_token":  "secret-token",
		"client_id":     "client-id",
		"model_mapping": map[string]any{"gpt-5": "gpt-5"},
		"plan_type":     "team",
	}
	info := &service.OpenAIWorkspaceInfo{
		AccountID:      "team-account-id",
		Name:           "workspace",
		CreatedTime:    "2026-05-14T12:28:12.982090Z",
		OrganizationID: "org-team",
		PlanType:       "self_serve_business_prolite",
		WorkspaceType:  "business",
	}

	credentials := mergeWorkspaceMetadataCredentials(existing, info)

	require.Equal(t, "secret-token", credentials["access_token"])
	require.Equal(t, "client-id", credentials["client_id"])
	require.Equal(t, map[string]any{"gpt-5": "gpt-5"}, credentials["model_mapping"])
	require.Equal(t, "workspace", credentials["name"])
	require.Equal(t, "2026-05-14T12:28:12.982090Z", credentials["created_time"])
	require.Equal(t, "org-team", credentials["organization_id"])
	require.Equal(t, "workspace", credentials["team_name"])
	require.Equal(t, "team-account-id", credentials["team_account_id"])
	require.Equal(t, "self_serve_business_prolite", credentials["team_plan_type"])
	require.Equal(t, "business", credentials["team_workspace_type"])
	require.Equal(t, "team", existing["plan_type"])
}
