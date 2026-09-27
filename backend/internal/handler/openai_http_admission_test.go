package handler

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	middleware2 "github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type httpAdmissionRecoveryRepo struct {
	httpAdmissionSelectedRepo
	allDenied bool
	readErr   error
	reads     int
}

func (r *httpAdmissionRecoveryRepo) GetOpenAITurnAdmission(ctx context.Context, id int64) (*service.Account, *service.Account, error) {
	r.reads++
	if r.readErr != nil {
		return nil, nil, r.readErr
	}
	account, err := r.GetByID(ctx, id)
	if err != nil {
		return nil, nil, err
	}
	if id == 1 || r.allDenied {
		account.Schedulable = false
	}
	return account, nil, nil
}

type httpAdmissionRecoveryUpstream struct {
	service.HTTPUpstream
	accounts []int64
}

func (u *httpAdmissionRecoveryUpstream) Do(req *http.Request, _ string, accountID int64, _ int) (*http.Response, error) {
	u.accounts = append(u.accounts, accountID)
	if req.Body != nil {
		_ = req.Body.Close()
	}
	return &http.Response{StatusCode: http.StatusBadRequest, Header: http.Header{"Content-Type": {"application/json"}}, Body: io.NopCloser(strings.NewReader(`{"error":{"type":"invalid_request_error","message":"fixture ordinary response"}}`))}, nil
}

func TestOpenAIHTTPAdmissionRecoveryReselectsEligibleAccount(t *testing.T) {
	for _, tc := range []struct {
		name      string
		allDenied bool
		dbFailure bool
		limit     int
		wantCalls int
		status    int
		reason    string
	}{
		{name: "reselect ordinary account", limit: 3, wantCalls: 1, status: 400},
		{name: "exhausted accounts retain reason", allDenied: true, limit: 3, status: 503, reason: "account_ineligible"},
		{name: "state read outage stops", dbFailure: true, limit: 3, status: 503, reason: "latest_state_unavailable"},
		{name: "retry limit respected", limit: 0, status: 503, reason: "account_ineligible"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			accounts := []service.Account{
				{ID: 1, Platform: service.PlatformOpenAI, Type: service.AccountTypeOAuth, Status: service.StatusActive, Schedulable: true, Priority: 0, Credentials: map[string]any{"access_token": "token-1"}, Extra: map[string]any{"openai_excel_bps": false, "openai_passthrough": true}},
				{ID: 2, Platform: service.PlatformOpenAI, Type: service.AccountTypeOAuth, Status: service.StatusActive, Schedulable: true, Priority: 1, Credentials: map[string]any{"access_token": "token-2"}, Extra: map[string]any{"openai_excel_bps": false, "openai_passthrough": true}},
			}
			repo := &httpAdmissionRecoveryRepo{httpAdmissionSelectedRepo: httpAdmissionSelectedRepo{accounts: accounts}, allDenied: tc.allDenied}
			if tc.dbFailure {
				repo.readErr = errors.New("fixture database read outage")
			}
			upstream := &httpAdmissionRecoveryUpstream{}
			cfg := &config.Config{RunMode: config.RunModeSimple}
			gateway := service.NewOpenAIGatewayService(repo, nil, nil, nil, nil, nil, nil, nil, cfg, nil, nil, nil, nil, nil, upstream, nil, nil, nil, nil, nil, nil, nil, nil, nil)
			billing := service.NewBillingCacheService(nil, nil, nil, nil, nil, nil, cfg, nil)
			t.Cleanup(billing.Stop)
			h := NewOpenAIGatewayHandler(gateway, service.NewConcurrencyService(nil), billing, service.NewAPIKeyService(nil, nil, nil, nil, nil, nil, cfg), nil, nil, nil, nil, nil, cfg)
			h.maxAccountSwitches = tc.limit
			c, rec := newHTTPAdmissionRecoveryContext(t, context.Background())
			h.Responses(c)
			require.Equal(t, tc.status, rec.Code, rec.Body.String())
			require.Len(t, upstream.accounts, tc.wantCalls)
			if tc.wantCalls > 0 {
				require.Equal(t, int64(2), upstream.accounts[0])
				require.NotContains(t, rec.Body.String(), "admission_unavailable")
			}
			if tc.reason != "" {
				require.Contains(t, rec.Body.String(), tc.reason)
			}
			if tc.dbFailure {
				require.Equal(t, 1, repo.reads)
			}
			for _, account := range repo.accounts {
				require.True(t, account.Schedulable)
				require.Nil(t, account.TempUnschedulableUntil)
			}
		})
	}
}

func TestOpenAIHTTPAdmissionRetryBoundaries(t *testing.T) {
	for _, tc := range []struct {
		name      string
		reason    string
		previous  string
		output    bool
		partial   bool
		canceled  bool
		committed bool
		switches  int
		want      bool
	}{
		{name: "ineligible account", reason: "account_ineligible", want: true},
		{name: "changed binding", reason: "account_binding_changed", want: true},
		{name: "group changed", reason: "group_membership_changed", want: true},
		{name: "model blocked", reason: "model_runtime_blocked", want: true},
		{name: "rate limited", reason: "model_rate_limited", want: true},
		{name: "ticket expired", reason: "model_ticket_unavailable", want: true},
		{name: "database unavailable", reason: "latest_state_unavailable"},
		{name: "unknown rejection", reason: "unknown"},
		{name: "continuation bound", reason: "account_ineligible", previous: "resp_bound"},
		{name: "output started", reason: "account_ineligible", output: true},
		{name: "partial result", reason: "account_ineligible", partial: true},
		{name: "client canceled", reason: "account_ineligible", canceled: true},
		{name: "committed response", reason: "account_ineligible", committed: true},
		{name: "retry budget", reason: "account_ineligible", switches: 2},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			c, _ := newHTTPAdmissionRecoveryContext(t, ctx)
			before := c.Writer.Size()
			if tc.output {
				_, err := c.Writer.WriteString("partial")
				require.NoError(t, err)
			}
			if tc.canceled {
				cancel()
			}
			if tc.committed {
				service.MarkResponseCommitted(c)
			}
			var result *service.OpenAIForwardResult
			if tc.partial {
				result = &service.OpenAIForwardResult{}
			}
			err := &service.OpenAITurnAdmissionError{Reason: tc.reason}
			require.Equal(t, tc.want, openAIHTTPAdmissionMayRetry(c, err, tc.previous, result, before, tc.switches, 2))
		})
	}
}

type httpAdmissionSelectedRepo struct {
	service.AccountRepository
	accounts []service.Account
}

func (r httpAdmissionSelectedRepo) GetByID(_ context.Context, id int64) (*service.Account, error) {
	for _, account := range r.accounts {
		if account.ID == id {
			return &account, nil
		}
	}
	return nil, service.ErrNoAvailableAccounts
}
func (r httpAdmissionSelectedRepo) ListSchedulableByPlatform(context.Context, string) ([]service.Account, error) {
	return r.accounts, nil
}
func (r httpAdmissionSelectedRepo) ListSchedulableUngroupedByPlatform(context.Context, string) ([]service.Account, error) {
	return r.accounts, nil
}
func (r httpAdmissionSelectedRepo) ListSchedulableByGroupIDAndPlatform(context.Context, int64, string) ([]service.Account, error) {
	return r.accounts, nil
}

func newHTTPAdmissionRecoveryContext(t *testing.T, ctx context.Context) (*gin.Context, *httptest.ResponseRecorder) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	groupID := int64(3131)
	req := httptest.NewRequest(http.MethodPost, "/v1/responses", strings.NewReader(`{"model":"gpt-5.1","stream":false,"input":"hello"}`)).WithContext(ctx)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = req
	c.Set(string(middleware2.ContextKeyAPIKey), &service.APIKey{ID: 99, GroupID: &groupID, Group: &service.Group{ID: groupID, Platform: service.PlatformOpenAI}, User: &service.User{ID: 100}})
	c.Set(string(middleware2.ContextKeyUser), middleware2.AuthSubject{UserID: 100, Concurrency: 0})
	return c, rec
}
