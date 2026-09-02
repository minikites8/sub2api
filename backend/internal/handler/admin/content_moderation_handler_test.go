package admin

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestBindRiskControlJSON_ReplacesRawControlBytes(t *testing.T) {
	raw := append([]byte(`{"block_message":"before`), byte(0x19))
	raw = append(raw, []byte(`after"}`)...)
	request := httptest.NewRequest(http.MethodPut, "/admin/risk-control/config", bytes.NewReader(raw))
	contextValue, _ := gin.CreateTestContext(httptest.NewRecorder())
	contextValue.Request = request

	var payload contentModerationConfigRequest
	err := bindRiskControlJSON(contextValue, &payload)

	require.NoError(t, err)
	require.Equal(t, "before after", dereferenceString(payload.BlockMessage))
}

func dereferenceString(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

func TestBindRiskControlJSON_PreservesJSONWhitespace(t *testing.T) {
	request := httptest.NewRequest(http.MethodPut, "/admin/risk-control/config", bytes.NewBufferString("{\n  \"enabled\": true\n}"))
	contextValue, _ := gin.CreateTestContext(httptest.NewRecorder())
	contextValue.Request = request

	var payload contentModerationConfigRequest
	require.NoError(t, bindRiskControlJSON(contextValue, &payload))
	require.NotNil(t, payload.Enabled)
	require.True(t, *payload.Enabled)
}
