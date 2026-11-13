package service

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestExcelBPSFailureCooldownSchedulers(t *testing.T) {
	for _, advanced := range []bool{true, false} {
		for _, status := range []int{403, 502} {
			t.Run(fmt.Sprintf("advanced=%t/status=%d", advanced, status), func(t *testing.T) {
				ctx := context.Background()
				groupID := int64(101301)
				first, second := excelAccount(), excelAccount()
				first.ID, second.ID = 38301, 38302
				for _, account := range []*Account{first, second} {
					account.Extra["openai_excel_bps_models"] = []any{"gpt-6-astra"}
				}
				cfg := &config.Config{}
				cfg.Gateway.Scheduling.LoadBatchEnabled = true
				svc := &OpenAIGatewayService{accountRepo: schedulerTestOpenAIAccountRepo{accounts: []Account{*first, *second}}, cache: &schedulerTestGatewayCache{}, cfg: cfg, rateLimitService: newOpenAIAdvancedSchedulerRateLimitService(strconv.FormatBool(advanced)), concurrencyService: NewConcurrencyService(schedulerTestConcurrencyCache{})}
				svc.coolDownExcelBPSFailure(ctx, first, status, nil)
				for range 5 {
					selection, _, err := svc.SelectAccountWithScheduler(ctx, &groupID, "", "", "gpt-6-astra", nil, OpenAIUpstreamTransportAny, false)
					require.NoError(t, err)
					require.Equal(t, second.ID, selection.Account.ID)
					if selection.ReleaseFunc != nil {
						selection.ReleaseFunc()
					}
				}
				require.False(t, svc.isOpenAIAccountRequestRuntimeBlocked(first, "gpt-5.1", false), "native route stays available")
				svc.coolDownExcelBPSFailure(ctx, second, status, nil)
				_, _, err := svc.SelectAccountWithScheduler(ctx, &groupID, "", "", "gpt-6-astra", nil, OpenAIUpstreamTransportAny, false)
				require.ErrorIs(t, err, ErrNoAvailableAccounts)
				svc.excelBPSCooldownUntil.Store(first.ID, time.Now().Add(-time.Second))
				require.False(t, svc.isOpenAIAccountRequestRuntimeBlocked(first, "gpt-6-astra", false))
			})
		}
	}
}

type bpsCooldownAffinityRepo struct {
	*connectionTypeRepo
	bound    string
	proposed string
}

func (r *bpsCooldownAffinityRepo) ResolveAuthConnectionAffinity(_ context.Context, _ int64, _ int64, _ string, proposed string) (string, error) {
	r.proposed = proposed
	return r.bound, nil
}

func TestManagedBPSFailureCooldownKeepsOtherConnection(t *testing.T) {
	reauth, repo, bps := managedBPSConnectionFailureFixture(t)
	native := *bps
	native.ID, native.Kind, native.Preferred = "1d2c8a1e-3f4b-4c6d-8e9f-0a1b2c3d4e5f", "codex", false
	require.NoError(t, reauth.encryptConnectionCredentials(&native, map[string]any{"access_token": "native-token", "expires_at": strconv.FormatInt(time.Now().Add(time.Hour).Unix(), 10)}))
	repo.rows = append(repo.rows, &native)
	gateway := &OpenAIGatewayService{excelOAuthReauth: reauth}
	parent := managedBPSBillingParent(nil)
	parent.Extra["openai_auth_connections"] = []*OpenAIAuthConnection{bps, &native}
	failed := *parent
	failed.AuthConnectionID = bps.ID
	failed.Extra = shallowCopyMap(parent.Extra)
	failed.Extra["openai_connection_kind"] = "bps"
	gateway.coolDownExcelBPSFailure(context.Background(), &failed, 502, nil)
	require.Nil(t, bps.CooldownUntil)
	require.Equal(t, int64(1), bps.Revision)
	require.False(t, gateway.managedExcelBPSRequestCoolingDown(parent, "gpt-6-astra"))
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
	projected, err := gateway.prepareAuthConnection(context.Background(), c, parent, nil, "gpt-6-astra")
	require.NoError(t, err)
	require.Equal(t, native.ID, projected.AuthConnectionID)
	_, err = gateway.prepareAuthConnection(context.Background(), c, parent, &failed, "gpt-6-astra")
	require.Error(t, err, "an explicit connection pin must not migrate to the healthy grant")
	affinity := &bpsCooldownAffinityRepo{connectionTypeRepo: repo, bound: bps.ID}
	reauth.repo = affinity
	c.Set("openai_auth_connection_session_hash", "existing-session")
	_, err = gateway.prepareAuthConnection(context.Background(), c, parent, nil, "gpt-6-astra")
	require.Error(t, err, "existing session affinity must not be bypassed by the healthy fallback")
	require.Equal(t, native.ID, affinity.proposed)
	c.Set("openai_auth_connection_session_hash", "")
	parent.Extra["openai_auth_connections"] = []*OpenAIAuthConnection{bps}
	require.True(t, gateway.managedExcelBPSRequestCoolingDown(parent, "gpt-6-astra"))
	gateway.excelBPSCooldownUntil.Store(excelBPSConnectionCooldownKey{accountID: parent.ID, connectionID: bps.ID}, time.Now().Add(-time.Second))
	projected, err = gateway.prepareAuthConnection(context.Background(), c, parent, nil, "gpt-6-astra")
	require.NoError(t, err)
	require.Equal(t, bps.ID, projected.AuthConnectionID)
	gateway.coolDownExcelBPSModel(&failed, "gpt-6-astra")
	require.True(t, gateway.excelBPSConnectionCoolingDown(parent.ID, bps, "gpt-6-astra"))
	require.False(t, gateway.excelBPSConnectionCoolingDown(parent.ID, bps, "gpt-5.1"))
	require.Nil(t, bps.CooldownUntil, "model denial must not mutate the whole connection")
	require.Equal(t, int64(1), bps.Revision)
}

func TestExcelBPSFailureCooldownBoundsAndNoAuthSideEffects(t *testing.T) {
	svc := &OpenAIGatewayService{}
	account := excelAccount()
	for _, status := range []int{400, 401, 429} {
		svc.coolDownExcelBPSFailure(context.Background(), account, status, nil)
		require.False(t, svc.isExcelBPSCoolingDown(account, "gpt-6-astra"))
	}
	start := time.Now()
	svc.coolDownExcelBPSFailure(context.Background(), account, 502, http.Header{"Retry-After": []string{"99999"}})
	raw, ok := svc.excelBPSCooldownUntil.Load(account.ID)
	require.True(t, ok)
	until, ok := raw.(time.Time)
	require.True(t, ok)
	require.WithinDuration(t, start.Add(time.Minute), until, time.Second)
	require.True(t, account.Schedulable)
	require.Equal(t, StatusActive, account.Status)
}

func TestManagedBPSChatBridgeSchedulingIsRequestScoped(t *testing.T) {
	for _, advanced := range []bool{true, false} {
		for _, route := range []string{"/v1/chat/completions", "/v1/messages"} {
			for _, enabled := range []bool{true, false} {
				t.Run(fmt.Sprintf("advanced=%t/route=%s/enabled=%t", advanced, route, enabled), func(t *testing.T) {
					account := excelAccount()
					account.Extra["openai_connections_managed"] = true
					account.Extra["openai_auth_connections"] = []*OpenAIAuthConnection{{ID: "bps-only", Kind: "bps", Status: "active", Preferred: true, Capabilities: []string{"responses"}}}
					cfg := &config.Config{}
					cfg.Gateway.Scheduling.LoadBatchEnabled = true
					svc := &OpenAIGatewayService{accountRepo: schedulerTestOpenAIAccountRepo{accounts: []Account{*account}}, cache: &schedulerTestGatewayCache{}, cfg: cfg, rateLimitService: newOpenAIAdvancedSchedulerRateLimitService(strconv.FormatBool(advanced)), concurrencyService: NewConcurrencyService(schedulerTestConcurrencyCache{})}
					c, _ := gin.CreateTestContext(httptest.NewRecorder())
					c.Request = httptest.NewRequest(http.MethodPost, route, nil)
					ctx := WithOpenAIConnectionCapabilityForRequest(context.Background(), c)
					if !enabled {
						ctx = context.WithValue(ctx, openAIBPSChatBridgeEnabledContextKey{}, false)
					}
					groupID := int64(101301)
					selection, _, err := svc.SelectAccountWithSchedulerForCapability(ctx, &groupID, "", "", "gpt-6-astra", nil, OpenAIUpstreamTransportAny, OpenAIEndpointCapabilityChatCompletions, false, false, true)
					if route == "/v1/chat/completions" && enabled {
						require.NoError(t, err)
						require.Equal(t, account.ID, selection.Account.ID)
						if selection.ReleaseFunc != nil {
							selection.ReleaseFunc()
						}
					} else {
						require.ErrorIs(t, err, ErrNoAvailableAccounts)
					}
				})
			}
		}
	}
}

func TestManagedBPSCooldownChecksInboundNativeCapability(t *testing.T) {
	gateway := &OpenAIGatewayService{}
	parent := excelAccount()
	parent.Extra["openai_connections_managed"] = true
	bps := &OpenAIAuthConnection{ID: "bps", Kind: "bps", Status: "active", Capabilities: []string{"responses"}}
	native := &OpenAIAuthConnection{ID: "chat-only", Kind: "api_key", Status: "active", Capabilities: []string{"chat_completions"}}
	parent.Extra["openai_auth_connections"] = []*OpenAIAuthConnection{bps, native}
	failed := *parent
	failed.AuthConnectionID = bps.ID
	gateway.coolDownExcelBPSFailure(context.Background(), &failed, 502, nil)
	for _, route := range []string{"/v1/chat/completions", "/v1/responses"} {
		c, _ := gin.CreateTestContext(httptest.NewRecorder())
		c.Request = httptest.NewRequest(http.MethodPost, route, nil)
		ctx := gateway.withOpenAIBPSChatBridgeScheduling(WithOpenAIConnectionCapabilityForRequest(context.Background(), c))
		require.Equal(t, route == "/v1/responses", gateway.managedExcelBPSRequestCoolingDown(parent, "gpt-6-astra", ctx))
		require.Equal(t, route == "/v1/responses", gateway.isOpenAIAccountRequestRuntimeBlocked(parent, "gpt-6-astra", false, ctx))
	}
}
