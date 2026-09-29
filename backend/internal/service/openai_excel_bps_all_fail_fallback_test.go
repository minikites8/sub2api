package service

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestExcelBPSHTTPFailureReturnsBPSFailoverBeforeWriting(t *testing.T) {
	for _, status := range []int{http.StatusUnauthorized, http.StatusForbidden, http.StatusBadGateway, http.StatusServiceUnavailable} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			upstream := &httpUpstreamRecorder{resp: excelBPSJSONResponse(status, `{"error":{"code":"upstream_failure","message":"BPS failed"}}`)}
			svc := openAIClientToolsTestService(upstream)
			body := []byte(`{"model":"gpt-5.6-sol","stream":false,"input":"hello"}`)
			rec := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(rec)
			c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", bytes.NewReader(body))

			ctx := WithExcelBPSFailoverContext(context.Background())
			_, err := svc.Forward(ctx, c, excelAccount(), body)
			var failover *UpstreamFailoverError
			require.ErrorAs(t, err, &failover)
			require.True(t, IsExcelBPSFailover(err))
			require.Equal(t, status, failover.StatusCode)
			require.Empty(t, rec.Body.String())
			require.Equal(t, "/basispoints/api/responses", upstream.requests[0].URL.Path)
		})
	}
}

func TestExcelBPSStreamFailureReturnsBPSFailoverBeforeSemanticOutput(t *testing.T) {
	upstream := &httpUpstreamRecorder{resp: excelBPSSSEResponse("event: response.created\ndata: {\"type\":\"response.created\",\"response\":{\"id\":\"resp_failed\",\"output\":[]}}\n\nevent: error\ndata: {\"type\":\"error\",\"error\":{\"code\":\"upstream_failure\",\"message\":\"BPS failed\"}}\n\n")}
	svc := openAIClientToolsTestService(upstream)
	body := []byte(`{"model":"gpt-5.6-sol","stream":true,"input":"hello"}`)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", bytes.NewReader(body))

	ctx := WithExcelBPSFailoverContext(context.Background())
	_, err := svc.Forward(ctx, c, excelAccount(), body)
	require.Error(t, err)
	require.True(t, IsExcelBPSFailover(err))
	require.Empty(t, rec.Body.String())
}

func TestExcelBPSFallbackAdmissionNormalizesBothRoutes(t *testing.T) {
	selected := excelAccount()
	latest := *selected
	latest.Extra = map[string]any{"openai_excel_bps": true, "openai_passthrough": true}
	ctx := WithExcelBPSFallbackContext(context.Background())

	ordinary, ok := accountForExcelBPSFallbackAdmission(ctx, selected, &latest)
	require.True(t, ok)
	require.NotNil(t, ordinary)
	require.False(t, ordinary.IsExcelBPSEnabled())
	require.True(t, selected.IsExcelBPSEnabled())
	require.Equal(t, openAITurnRouteFingerprint(accountForExcelBPSFallback(selected)), openAITurnRouteFingerprint(ordinary))
}

func TestExcelBPSGenericFailureReplayBoundary(t *testing.T) {
	const created = "data: {\"type\":\"response.created\",\"response\":{\"id\":\"resp_rejected\",\"output\":[]}}\n\n"
	for _, terminal := range []string{
		"event: error\ndata: {\"type\":\"error\",\"error\":{\"code\":\"upstream_failure\",\"message\":\"BPS failed\"}}\n\n",
		"data: {\"type\":\"response.incomplete\",\"response\":{\"status\":\"incomplete\",\"output\":[]}}\n\n",
		"",
	} {
		for _, tc := range []struct {
			name                                 string
			prefix                               string
			heartbeat, probe, canceled, failover bool
		}{
			{name: "prelude", failover: true},
			{name: "keepalive", heartbeat: true, failover: true},
			{name: "semantic_output", prefix: "data: {\"type\":\"response.output_text.delta\",\"delta\":\"already streamed\"}\n\n"},
			{name: "probe", probe: true},
			{name: "canceled", canceled: true},
		} {
			t.Run(tc.name+"/"+terminal, func(t *testing.T) {
				upstream := &httpUpstreamRecorder{resp: excelBPSSSEResponse(created + tc.prefix + terminal)}
				svc := openAIClientToolsTestService(upstream)
				c, rec := newExcelBPSFallbackContext([]byte(`{"model":"gpt-6-sol","input":"hello","stream":true}`), true)
				ctx, cancel := context.WithCancel(WithExcelBPSFailoverContext(context.Background()))
				defer cancel()
				c.Request = c.Request.WithContext(ctx)
				if tc.probe {
					c.Set(bpsAccountProbeRequiredContextKey, true)
				}
				if tc.canceled {
					cancel()
				}
				if tc.heartbeat {
					_, err := c.Writer.WriteString(": keepalive\n\n")
					require.NoError(t, err)
					c.Writer.Flush()
				}
				_, err := svc.Forward(ctx, c, excelAccount(), []byte(`{"model":"gpt-6-sol","input":"hello","stream":true}`))
				require.Error(t, err)
				require.Equal(t, tc.failover, IsExcelBPSFailover(err))
				if tc.failover {
					require.NotContains(t, rec.Body.String(), "resp_rejected")
					require.False(t, IsResponseCommitted(c))
				}
				if strings.Contains(tc.prefix, "already streamed") {
					require.Contains(t, rec.Body.String(), "already streamed")
				}
			})
		}
	}
}
