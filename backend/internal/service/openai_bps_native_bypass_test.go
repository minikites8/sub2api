package service

import (
	"context"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/openai"
	"github.com/stretchr/testify/require"
)

func TestExcelBPSNativeBypassScheduling(t *testing.T) {
	accounts := []Account{
		{ID: 1, Platform: PlatformOpenAI, Type: AccountTypeOAuth, Status: StatusActive, Schedulable: true, Concurrency: 1,
			Extra: map[string]any{"openai_excel_bps": true}},
		{ID: 2, Platform: PlatformOpenAI, Type: AccountTypeOAuth, Status: StatusActive, Schedulable: true, Concurrency: 1,
			Credentials: map[string]any{"client_id": openai.ExcelClientID}},
		{ID: 4, Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Status: StatusActive, Schedulable: true, Concurrency: 1,
			Extra: map[string]any{"openai_responses_supported": false}},
		{ID: 3, Platform: PlatformOpenAI, Type: AccountTypeOAuth, Status: StatusActive, Schedulable: true, Concurrency: 1, Priority: 10},
	}
	for _, advanced := range []bool{false, true} {
		name := "legacy"
		if advanced {
			name = "advanced"
		}
		t.Run(name, func(t *testing.T) {
			svc := &OpenAIGatewayService{accountRepo: schedulerTestOpenAIAccountRepo{accounts: accounts}, cfg: &config.Config{}}
			// The native-capability phase takes precedence over fallback that masks BPS settings.
			ctx := WithExcelBPSFallbackContext(WithExcelBPSNativeBypassContext(context.Background(), "web_search"))
			require.False(t, excelBPSFallbackContextEnabled(ctx))
			if advanced {
				scheduler := newDefaultOpenAIAccountScheduler(svc, nil)
				selection, _, err := scheduler.Select(ctx, OpenAIAccountScheduleRequest{
					Platform: PlatformOpenAI, RequestedModel: "gpt-5.6-sol", StickyAccountID: 1, SessionHash: "bps-session",
				})
				require.NoError(t, err)
				require.Equal(t, int64(3), selection.Account.ID)
				if selection.ReleaseFunc != nil {
					selection.ReleaseFunc()
				}
			} else {
				account, err := svc.selectAccountForModelWithExclusions(ctx, nil, PlatformOpenAI, "bps-session", "gpt-5.6-sol", nil, false, 1, "", false)
				require.NoError(t, err)
				require.Equal(t, int64(3), account.ID)
			}
			require.True(t, accounts[0].IsExcelBPSEnabled())
		})
	}
}

func TestExcelBPSNativeBypassRespectsModelScopeAndFinalAdmission(t *testing.T) {
	account := excelAccount()
	account.Extra["openai_excel_bps_models"] = []string{"gpt-6-astra"}
	ctx := WithExcelBPSNativeBypassContext(context.Background(), "web_search")

	require.True(t, excelBPSNativeBypassExcludesAccount(ctx, account, "gpt-6-astra"))
	require.False(t, excelBPSNativeBypassExcludesAccount(ctx, account, "gpt-5.6-sol"))
	account.bpsDisabledByGroup = true
	require.True(t, excelBPSNativeBypassExcludesAccount(ctx, account, "gpt-6-astra"))
	account.bpsDisabledByGroup = false
	svc := &OpenAIGatewayService{}
	_, err := svc.admitOpenAITurnSnapshot(ctx, account, account, "gpt-6-astra", 0, false)
	require.Equal(t, "bps_native_bypass", OpenAITurnAdmissionReason(err))
}
