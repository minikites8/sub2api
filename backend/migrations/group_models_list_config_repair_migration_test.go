package migrations

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestGroupModelsListConfigRepairMigrationIsConditional(t *testing.T) {
	content, err := FS.ReadFile("252_repair_group_models_list_config.sql")
	require.NoError(t, err)

	sql := strings.Join(strings.Fields(string(content)), " ")
	require.Contains(t, sql, "ADD COLUMN models_list_config JSONB NOT NULL DEFAULT '{}'::jsonb")
	require.Contains(t, sql, "has_model_allowlist")
	require.Contains(t, sql, "COALESCE(models_list_config, '{}'::jsonb) = '{}'::jsonb")
	require.Contains(t, sql, "COMMENT ON COLUMN groups.models_list_config")
}
