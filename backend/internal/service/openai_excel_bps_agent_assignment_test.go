package service

import (
	"context"
	"fmt"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestExcelBPSRejectsNewEncryptedAssignmentBeforeAnySubmission(t *testing.T) {
	for _, ignore := range []bool{false, true} {
		for _, top := range []bool{false, true} {
			upstream := &httpUpstreamRecorder{}
			svc := openAIClientToolsTestService(upstream)
			account := excelAccount()
			account.Extra[ExcelBPSIgnoreEncryptedContentKey] = ignore
			extra := ""
			content := `[{"type":"input_text","text":"Header"},{"type":"encrypted_content","data":"SYNTHETIC_PRIVATE"}]`
			if top {
				extra = `,"encrypted_content":"SYNTHETIC_PRIVATE"`
				content = `[{"type":"input_text","text":"Header"}]`
			}
			body := []byte(fmt.Sprintf(`{"model":"gpt-6-astra","input":[{"type":"agent_message","content":%s%s},{"type":"additional_tools","tools":[]}]}`, content, extra))
			rec := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(rec)
			c.Request = httptest.NewRequest("POST", "/v1/responses", nil)
			_, err := svc.Forward(context.Background(), c, account, body)
			require.Error(t, err)
			var failure *UpstreamFailoverError
			require.ErrorAs(t, err, &failure)
			require.Equal(t, 400, failure.StatusCode)
			require.Equal(t, GatewayFailureReason(ExcelBPSLocalCapabilityReason), failure.Reason)
			require.True(t, failure.ShouldRetryNextAccount())
			require.False(t, IsResponseCommitted(c))
			require.Empty(t, upstream.requests)
			require.Empty(t, rec.Body.String())
			require.NotContains(t, string(failure.ResponseBody), "SYNTHETIC_PRIVATE")
			require.NotContains(t, rec.Body.String(), "basispoints")
			require.NotContains(t, rec.Body.String(), "SYNTHETIC_PRIVATE")
		}
	}
}
