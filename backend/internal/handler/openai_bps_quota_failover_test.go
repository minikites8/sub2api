package handler

import (
	"context"
	"fmt"
	"maps"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

// Selection sees available quota, but the authoritative pre-forward read sees
// the last concurrent request exhaust it. The request must reach account 2.
type bpsQuotaAdmissionRepo struct {
	httpAdmissionSelectedRepo
	fullAccountReads int
}

func (r *bpsQuotaAdmissionRepo) GetOpenAITurnAdmission(ctx context.Context, id int64) (*service.Account, *service.Account, error) {
	account, err := r.GetByID(ctx, id)
	if err != nil {
		return nil, nil, err
	}
	if id == 1 {
		r.fullAccountReads++
		account.Extra = maps.Clone(account.Extra)
		account.Extra["codex_5h_used_percent"] = 100.0
		account.Extra["codex_5h_reset_at"] = time.Now().Add(time.Hour).Format(time.RFC3339)
	}
	return account, nil, nil
}

func TestOpenAIResponsesSwitchesAccountWhenBPSQuotaExhaustedAfterSelection(t *testing.T) {
	for _, stream := range []bool{false, true} {
		t.Run(fmt.Sprintf("stream=%t", stream), func(t *testing.T) {
			repo := &bpsQuotaAdmissionRepo{httpAdmissionSelectedRepo: httpAdmissionSelectedRepo{accounts: []service.Account{
				{ID: 1, Platform: service.PlatformOpenAI, Type: service.AccountTypeOAuth, Status: service.StatusActive, Schedulable: true, Priority: 0,
					Credentials: map[string]any{"access_token": "token-1", "chatgpt_account_id": "workspace-1"}, Extra: map[string]any{"openai_excel_bps": true, "openai_passthrough": true}},
				{ID: 2, Platform: service.PlatformOpenAI, Type: service.AccountTypeOAuth, Status: service.StatusActive, Schedulable: true, Priority: 1,
					Credentials: map[string]any{"access_token": "token-2"}, Extra: map[string]any{"openai_passthrough": true}},
			}}}
			upstream := &httpAdmissionRecoveryUpstream{}
			cfg := &config.Config{RunMode: config.RunModeSimple}
			gateway := service.NewOpenAIGatewayService(repo, nil, nil, nil, nil, nil, nil, nil, cfg, nil, nil, nil, nil, nil, upstream, nil, nil, nil, nil, nil, nil, nil, nil, nil)
			billing := service.NewBillingCacheService(nil, nil, nil, nil, nil, nil, cfg, nil)
			t.Cleanup(billing.Stop)
			h := NewOpenAIGatewayHandler(gateway, service.NewConcurrencyService(nil), billing, service.NewAPIKeyService(nil, nil, nil, nil, nil, nil, cfg), nil, nil, nil, nil, nil, cfg)
			h.maxAccountSwitches = 3
			c, rec := newHTTPAdmissionRecoveryContext(t, context.Background())
			c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", strings.NewReader(fmt.Sprintf(`{"model":"gpt-6-astra","stream":%t,"input":"hello"}`, stream)))
			c.Request.Header.Set("Content-Type", "application/json")
			h.Responses(c)

			require.Positive(t, repo.fullAccountReads, "the BPS account must have been selected before quota changed")
			require.Equal(t, []int64{2}, upstream.accounts, "the exhausted BPS account must be excluded before any upstream submission")
			require.Equal(t, http.StatusBadRequest, rec.Code, rec.Body.String())
			require.NotContains(t, rec.Body.String(), "basispoints")
			require.NotContains(t, rec.Body.String(), "BPS")
			require.NotContains(t, repo.accounts[0].Extra, "codex_5h_used_percent")
		})
	}
}
