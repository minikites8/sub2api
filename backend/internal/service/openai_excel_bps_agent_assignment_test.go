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
			require.Equal(t, 400, rec.Code)
			require.Empty(t, upstream.requests)
			require.Contains(t, rec.Body.String(), "new encrypted agent assignment")
			require.NotContains(t, rec.Body.String(), "SYNTHETIC_PRIVATE")
		}
	}
}

func TestExcelBPSNewEncryptedAssignmentRequestsOrdinaryRouting(t *testing.T) {
	for _, ignore := range []bool{false, true} {
		for _, top := range []bool{false, true} {
			for _, probe := range []bool{false, true} {
				t.Run(fmt.Sprintf("ignore=%t/top=%t/probe=%t", ignore, top, probe), func(t *testing.T) {
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
					c, rec := newExcelBPSFallbackContext(body, false)
					c.Set(bpsAccountProbeRequiredContextKey, probe)
					result, err := svc.Forward(WithExcelBPSFailoverContext(context.Background()), c, account, body)
					require.Error(t, err)
					require.Nil(t, result)
					require.Empty(t, upstream.requests)
					require.True(t, account.IsExcelBPSEnabled())
					if probe {
						require.NotErrorIs(t, err, ErrExcelBPSNativeBypass)
						require.Equal(t, 400, rec.Code)
						require.Contains(t, rec.Body.String(), "new encrypted agent assignment")
					} else {
						require.ErrorIs(t, err, ErrExcelBPSNativeBypass)
						require.False(t, c.Writer.Written())
						require.Equal(t, ExcelBPSEncryptedAgentAssignmentReason, rec.Header().Get("X-Codex2API-Basispoints-Bypass"))
					}
					require.NotContains(t, rec.Body.String(), "SYNTHETIC_PRIVATE")
				})
			}
		}
	}
}
