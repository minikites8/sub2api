package service

import (
	"context"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestExcelBPSConfigurationUpdateRequestsOrdinaryRouting(t *testing.T) {
	for _, nativeImages := range []bool{false, true} {
		for _, stream := range []bool{false, true} {
			for _, mode := range []string{"client", "probe", "without failover", "canceled"} {
				t.Run(fmt.Sprintf("native_images=%t/stream=%t/%s", nativeImages, stream, mode), func(t *testing.T) {
					upstream := &httpUpstreamRecorder{}
					svc := openAIClientToolsTestService(upstream)
					if nativeImages {
						enableNativeAttachments(svc)
					}
					body := []byte(fmt.Sprintf(`{"model":"gpt-5.6-sol","stream":%t,"input":[{"role":"user","content":"retain context"},{"type":"configuration_update","reasoning":{"effort":"high"}}]}`, stream))
					c, rec := newExcelBPSFallbackContext(body, stream)
					c.Set(bpsAccountProbeRequiredContextKey, mode == "probe")
					ctx := context.Background()
					if mode != "without failover" {
						ctx = WithExcelBPSFailoverContext(ctx)
					}
					if mode == "canceled" {
						requestCtx, cancel := context.WithCancel(c.Request.Context())
						cancel()
						c.Request = c.Request.WithContext(requestCtx)
					}
					account := excelAccount()
					result, err := svc.Forward(ctx, c, account, body)
					require.Error(t, err)
					require.Nil(t, result)
					require.Empty(t, upstream.requests)
					require.True(t, account.IsExcelBPSEnabled())
					require.False(t, svc.isExcelBPSModelCoolingDown(account, "gpt-5.6-sol"))
					if mode == "client" {
						require.ErrorIs(t, err, ErrExcelBPSNativeBypass)
						require.False(t, c.Writer.Written())
						require.Empty(t, rec.Body.String())
						require.Equal(t, ExcelBPSConfigurationUpdateReason, rec.Header().Get("X-Codex2API-Basispoints-Bypass"))
					} else {
						require.NotErrorIs(t, err, ErrExcelBPSNativeBypass)
						require.Equal(t, 400, rec.Code)
						require.Contains(t, rec.Body.String(), "basispoints does not support configuration_update")
						require.Empty(t, rec.Header().Get("X-Codex2API-Basispoints-Bypass"))
					}
				})
			}
		}
	}
}
