package migrations

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestChannelMonitorV2TypeSafeMigration(t *testing.T) {
	content, err := FS.ReadFile("272_channel_monitor_v2_typesafe.sql")
	require.NoError(t, err)
	sql := strings.Join(strings.Fields(string(content)), " ")
	require.Contains(t, sql, "ALTER COLUMN platforms SET DEFAULT")
	require.Contains(t, sql, `{"platform":"typesafe","enabled":true,"models":[]}`)
	require.Contains(t, sql, "config.platforms ||")
	require.Contains(t, sql, "WHEN cardinality(config.group_ids) = 0 THEN config.group_ids")
	require.Contains(t, sql, "SELECT unnest(config.group_ids) AS group_id")
	require.Contains(t, sql, "SELECT DISTINCT group_id")
	require.Contains(t, sql, "status = 'active' AND deleted_at IS NULL")
	require.Contains(t, sql, "version = config.version + 1")
	require.Contains(t, sql, "AND NOT EXISTS")
	require.Contains(t, sql, "WHERE lower(btrim(provider->>'platform')) = 'typesafe'")
}
