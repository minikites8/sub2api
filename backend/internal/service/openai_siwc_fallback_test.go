package service

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestSIWCCapabilityFailoverBoundaries(t *testing.T) {
	for _, mode := range []string{"unsupported_input", "unsupported_tool", "unsupported_endpoint", "invalid_request", "canceled", "output", "committed"} {
		t.Run(mode, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			c, _ := gin.CreateTestContext(httptest.NewRecorder())
			c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil).WithContext(ctx)
			code := "OPENAI_SIWC_UNSUPPORTED_INPUT"
			switch mode {
			case "unsupported_tool":
				code = "OPENAI_SIWC_UNSUPPORTED_TOOL"
			case "unsupported_endpoint":
				code = "OPENAI_SIWC_UNSUPPORTED_ENDPOINT"
			case "invalid_request":
				code = "OPENAI_SIWC_INVALID_REQUEST"
			case "canceled":
				cancel()
			case "output":
				_, _ = c.Writer.WriteString("data: accepted output\n\n")
			case "committed":
				MarkResponseCommitted(c)
			}
			failure := siwcCapabilityFailover(ctx, c, &Account{ID: 42}, infraerrors.BadRequest(code, "safe capability diagnostic"))
			if mode != "unsupported_input" && mode != "unsupported_tool" && mode != "unsupported_endpoint" {
				require.Nil(t, failure)
				return
			}
			require.NotNil(t, failure)
			require.True(t, failure.ShouldRetryNextAccount())
			require.False(t, failure.ShouldReportAccountScheduleFailure())
			require.False(t, failure.RetryableOnSameAccount)
			require.False(t, c.Writer.Written())
		})
	}
}

func TestSIWCNativeFallbackDoesNotChangeAccountSettings(t *testing.T) {
	base := context.Background()
	ctx := WithOpenAISIWCNativeFallback(base)
	account := &Account{ID: 42, Platform: PlatformOpenAI, Type: AccountTypeOAuth, Extra: map[string]any{OpenAISIWCEnabledKey: true}}
	require.True(t, openAISIWCRequestCompatible(base, account))
	require.False(t, openAISIWCRequestCompatible(ctx, account))
	require.True(t, account.IsSIWCEnabled())
	compatible, reason := (&defaultOpenAIAccountScheduler{service: &OpenAIGatewayService{}}).isAccountRequestCompatibleReason(ctx, account, OpenAIAccountScheduleRequest{})
	require.False(t, compatible)
	require.Equal(t, "siwc_local_capability_unavailable", reason)
}

func TestManagedSIWCNativeFallbackRequiresIndependentGrant(t *testing.T) {
	reauth, repo, siwc := managedBPSConnectionFailureFixture(t)
	siwc.Kind, siwc.Capabilities = "siwc", []string{"responses"}
	parent := managedBPSBillingParent(nil)
	parent.Extra["openai_auth_connections"] = []*OpenAIAuthConnection{siwc}
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
	ctx := WithOpenAISIWCNativeFallback(WithOpenAIConnectionCapabilityForRequest(context.Background(), c))
	gateway := &OpenAIGatewayService{excelOAuthReauth: reauth}
	require.False(t, parent.supportsOpenAIEndpointCapabilityForRequest(ctx, OpenAIEndpointCapabilityResponses))
	_, err := gateway.prepareAuthConnection(ctx, c, parent, nil, "gpt-6-astra")
	require.Error(t, err)
	native := *siwc
	native.ID, native.Kind, native.Preferred = "c910c45b-79a0-4367-a1dc-60e12210e199", "api_key", false
	require.NoError(t, reauth.encryptConnectionCredentials(&native, map[string]any{"api_key": "synthetic-native-key"}))
	repo.rows = append(repo.rows, &native)
	parent.Extra["openai_auth_connections"] = []*OpenAIAuthConnection{siwc, &native}
	require.True(t, parent.supportsOpenAIEndpointCapabilityForRequest(ctx, OpenAIEndpointCapabilityResponses))
	projected, err := gateway.prepareAuthConnection(ctx, c, parent, nil, "gpt-6-astra")
	require.NoError(t, err)
	require.Equal(t, native.ID, projected.AuthConnectionID)
	require.True(t, siwc.Preferred, "fallback must not change the administrator's preferred grant")
}
