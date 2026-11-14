package service

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"image"
	"image/png"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestExcelBPSInlineImageLimitReturnsUnsubmittedRouteFailover(t *testing.T) {
	for _, count := range []int{21} {
		t.Run(string(rune('a'+count)), func(t *testing.T) {
			var pngBytes bytes.Buffer
			require.NoError(t, png.Encode(&pngBytes, image.NewRGBA(image.Rect(0, 0, 1, 1))))
			dataURL := "data:image/png;base64," + base64.StdEncoding.EncodeToString(pngBytes.Bytes())
			content := []map[string]any{{"type": "input_text", "text": "describe"}}
			for i := 0; i < count; i++ {
				content = append(content, map[string]any{"type": "input_image", "image_url": dataURL})
			}
			body, err := json.Marshal(map[string]any{"model": "gpt-6-astra", "input": []map[string]any{{"role": "user", "content": content}}})
			require.NoError(t, err)
			upstream := &httpUpstreamRecorder{}
			svc := openAIClientToolsTestService(upstream)
			svc.settingService = NewSettingService(&excelBPSImageSettingsRepo{values: map[string]string{SettingKeyExcelBPSImageMode: ExcelBPSImageModeNative}}, svc.cfg)
			account := excelAccount()
			c, _ := gin.CreateTestContext(httptest.NewRecorder())
			c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", bytes.NewReader(body))
			_, err = svc.Forward(context.Background(), c, account, body)
			require.Error(t, err)
			if count == 21 {
				var failure *UpstreamFailoverError
				require.ErrorAs(t, err, &failure)
				require.Equal(t, GatewayFailureReason(ExcelBPSLocalCapabilityReason), failure.Reason)
				require.True(t, failure.ShouldRetryNextAccount())
				require.False(t, failure.RetryableOnSameAccount)
				require.False(t, c.Writer.Written())
				require.Empty(t, upstream.requests)
				require.True(t, account.Schedulable)
				require.False(t, svc.isExcelBPSCoolingDown(account, "gpt-6-astra"))
			}
		})
	}
}

func TestManagedBPSLocalCapabilityFallbackRequiresNativeGrant(t *testing.T) {
	reauth, repo, bps := managedBPSConnectionFailureFixture(t)
	gateway := &OpenAIGatewayService{excelOAuthReauth: reauth}
	parent := managedBPSBillingParent(nil)
	parent.Extra["openai_auth_connections"] = []*OpenAIAuthConnection{bps}
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
	ctx := WithOpenAIBPSNativeFallback(WithOpenAIConnectionCapabilityForRequest(context.Background(), c))
	require.False(t, parent.supportsOpenAIEndpointCapabilityForRequest(ctx, OpenAIEndpointCapabilityResponses))
	_, err := gateway.prepareAuthConnection(ctx, c, parent, nil, "gpt-6-astra")
	require.Error(t, err)
	native := *bps
	native.ID, native.Kind, native.Preferred = "c910c45b-79a0-4367-a1dc-60e12210e199", "api_key", false
	native.Capabilities = []string{"responses"}
	require.NoError(t, reauth.encryptConnectionCredentials(&native, map[string]any{"api_key": "synthetic-native-key"}))
	repo.rows = append(repo.rows, &native)
	parent.Extra["openai_auth_connections"] = []*OpenAIAuthConnection{bps, &native}
	require.True(t, parent.supportsOpenAIEndpointCapabilityForRequest(ctx, OpenAIEndpointCapabilityResponses))
	projected, err := gateway.prepareAuthConnection(ctx, c, parent, nil, "gpt-6-astra")
	require.NoError(t, err)
	require.Equal(t, native.ID, projected.AuthConnectionID)
}

func TestManagedBPSLocalCapabilityNativeForwardUsesAuthorizedGrant(t *testing.T) {
	for _, bound := range []bool{false, true} {
		t.Run(map[bool]string{false: "unbound_native_send", true: "existing_bps_affinity_stays_bound"}[bound], func(t *testing.T) {
			reauth, repo, bps := managedBPSConnectionFailureFixture(t)
			native := *bps
			native.ID, native.Kind, native.Preferred = "c910c45b-79a0-4367-a1dc-60e12210e199", "api_key", false
			native.Capabilities = []string{"responses"}
			require.NoError(t, reauth.encryptConnectionCredentials(&native, map[string]any{"api_key": "synthetic-native-key"}))
			repo.rows = append(repo.rows, &native)
			wire := `data: {"type":"response.completed","response":{"id":"resp_native_fallback","status":"completed","model":"gpt-6-astra","output":[],"usage":{"input_tokens":1,"output_tokens":1}}}` + "\n\n"
			upstream := &httpUpstreamRecorder{resp: &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": []string{"text/event-stream"}}, Body: io.NopCloser(strings.NewReader(wire))}}
			gateway := openAIClientToolsTestService(upstream)
			gateway.excelOAuthReauth = reauth
			parent := managedBPSBillingParent(nil)
			parent.Extra["openai_auth_connections"] = []*OpenAIAuthConnection{bps, &native}
			body := []byte(`{"model":"gpt-6-astra","stream":true,"input":"test"}`)
			c, _ := gin.CreateTestContext(httptest.NewRecorder())
			c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", bytes.NewReader(body))
			if bound {
				reauth.repo = &bpsCooldownAffinityRepo{connectionTypeRepo: repo, bound: bps.ID}
				c.Set("openai_auth_connection_session_hash", "existing-bps-session")
			}
			ctx := WithOpenAIBPSNativeFallback(WithOpenAIConnectionCapabilityForRequest(context.Background(), c))
			result, err := gateway.Forward(ctx, c, parent, body)
			if bound {
				require.Error(t, err)
				require.Empty(t, upstream.requests)
				return
			}
			require.NoError(t, err)
			require.NotNil(t, result)
			require.Len(t, upstream.requests, 1)
			require.Equal(t, "/v1/responses", upstream.lastReq.URL.Path)
			require.Equal(t, "Bearer synthetic-native-key", upstream.lastReq.Header.Get("Authorization"))
			require.Equal(t, native.ID, result.AuthConnectionID)
		})
	}
}
