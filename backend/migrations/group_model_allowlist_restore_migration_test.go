package migrations

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestRestoreDisplayOnlyGroupModelsListConfig(t *testing.T) {
	b, err := FS.ReadFile("250_restore_group_models_list_config.sql")
	require.NoError(t, err)
	sql := strings.Join(strings.Fields(string(b)), " ")
	require.Contains(t, sql, "ADD COLUMN IF NOT EXISTS models_list_config JSONB NOT NULL DEFAULT '{}'::jsonb")
	require.Contains(t, sql, "SET models_list_config = model_allowlist")
	require.Contains(t, sql, "COALESCE(models_list_config, '{}'::jsonb) = '{}'::jsonb")
	require.NotContains(t, sql, "SET model_allowlist = models_list_config")
}
