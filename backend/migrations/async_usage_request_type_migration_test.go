package migrations

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestAsyncUsageRequestTypeMigration(t *testing.T) {
	content, err := FS.ReadFile("191_allow_async_usage_request_type.sql")
	require.NoError(t, err)

	sql := string(content)
	require.Contains(t, sql, "DROP CONSTRAINT IF EXISTS usage_logs_request_type_check")
	require.True(t, strings.Contains(sql, "CHECK (request_type IN (0, 1, 2, 3, 4, 5))"))
}
