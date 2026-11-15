package service

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"maps"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestExcelBPSQuotaDefaultsToFullWindow(t *testing.T) {
	for _, window := range []string{"5h", "7d"} {
		for _, tc := range []struct {
			name    string
			used    float64
			reset   time.Duration
			age     time.Duration
			disable bool
			native  bool
			want    bool
		}{
			{name: "exhausted", used: 100, reset: time.Hour, want: true},
			{name: "below limit", used: 99, reset: time.Hour},
			{name: "window reset", used: 100, reset: -time.Minute},
			{name: "unknown reset", used: 100, want: true},
			{name: "stale without reset", used: 100, age: 48 * time.Hour},
			{name: "stale but reset pending", used: 100, age: 48 * time.Hour, reset: time.Hour, want: true},
			{name: "explicit opt out", used: 100, reset: time.Hour, disable: true},
			{name: "native default unchanged", used: 100, reset: time.Hour, native: true},
		} {
			t.Run(window+"/"+tc.name, func(t *testing.T) {
				account := excelAccount()
				account.Extra["openai_excel_bps"] = !tc.native
				account.Extra["codex_"+window+"_used_percent"] = tc.used
				account.Extra["codex_usage_updated_at"] = time.Now().Add(-tc.age).Format(time.RFC3339Nano)
				account.Extra["auto_pause_"+window+"_disabled"] = tc.disable
				if tc.reset != 0 {
					account.Extra["codex_"+window+"_reset_at"] = time.Now().Add(tc.reset).Format(time.RFC3339)
				}
				paused, decision := shouldAutoPauseOpenAIAccountByQuota(context.Background(), account)
				require.Equal(t, tc.want, paused)
				if paused {
					require.Equal(t, window, decision.window)
					require.Equal(t, 1.0, decision.threshold)
				}
			})
		}
	}
}

func TestExcelBPSQuotaSchedulerSkipsExhaustedPreferredAccount(t *testing.T) {
	for _, advanced := range []bool{false, true} {
		t.Run(fmt.Sprintf("advanced=%t", advanced), func(t *testing.T) {
			bps := excelAccount()
			bps.ID, bps.Priority = 1, 0
			bps.Extra["codex_5h_used_percent"] = 100.0
			bps.Extra["codex_5h_reset_at"] = time.Now().Add(time.Hour).Format(time.RFC3339)
			native := Account{ID: 2, Platform: PlatformOpenAI, Type: AccountTypeOAuth, Status: StatusActive, Schedulable: true, Concurrency: 1, Priority: 10}
			acquired := []int64{}
			svc := &OpenAIGatewayService{
				accountRepo: schedulerTestOpenAIAccountRepo{accounts: []Account{*bps, native}},
				cfg:         &config.Config{},
				concurrencyService: NewConcurrencyService(schedulerTestConcurrencyCache{
					acquireResults: map[int64]bool{1: true, 2: true}, acquiredIDs: &acquired,
				}),
			}
			if advanced {
				selection, _, err := newDefaultOpenAIAccountScheduler(svc, nil).Select(context.Background(), OpenAIAccountScheduleRequest{Platform: PlatformOpenAI, RequestedModel: "gpt-6-astra"})
				require.NoError(t, err)
				require.Equal(t, native.ID, selection.Account.ID)
				require.NotContains(t, acquired, bps.ID)
				if selection.ReleaseFunc != nil {
					selection.ReleaseFunc()
				}
			} else {
				selected, err := svc.SelectAccountForModelWithExclusions(context.Background(), nil, "", "gpt-6-astra", nil)
				require.NoError(t, err)
				require.Equal(t, native.ID, selected.ID)
			}
		})
	}
}

func TestExcelBPSQuotaForwardRejectsLatestExhaustedSnapshotBeforeSending(t *testing.T) {
	for _, stream := range []bool{false, true} {
		t.Run(fmt.Sprintf("stream=%t", stream), func(t *testing.T) {
			selected := excelAccount()
			latest := *selected
			latest.Extra = maps.Clone(selected.Extra)
			latest.Extra["codex_5h_used_percent"] = 100.0
			latest.Extra["codex_5h_reset_at"] = time.Now().Add(time.Hour).Format(time.RFC3339)
			upstream := &httpUpstreamRecorder{resp: &http.Response{StatusCode: http.StatusBadRequest, Header: http.Header{"Content-Type": {"application/json"}}, Body: io.NopCloser(strings.NewReader(`{"error":{"message":"unexpected submission"}}`))}}
			svc := openAIClientToolsTestService(upstream)
			svc.accountRepo = &turnAdmissionRepo{account: &latest}
			body := []byte(fmt.Sprintf(`{"model":"gpt-6-astra","input":"test","stream":%t}`, stream))
			c, _ := gin.CreateTestContext(httptest.NewRecorder())
			c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", bytes.NewReader(body))

			result, err := svc.Forward(context.Background(), c, selected, body)

			require.Nil(t, result)
			requireExcelBPSRateLimitFailover(t, err, c)
			require.Empty(t, upstream.requests, "an exhausted account must never receive the request")
			require.NotContains(t, selected.Extra, "codex_5h_used_percent", "the scheduler's snapshot stays immutable")
		})
	}
}

func TestExcelBPSQuotaForwardResumesAfterWindowReset(t *testing.T) {
	account := excelAccount()
	account.Extra["codex_5h_used_percent"] = 100.0
	account.Extra["codex_5h_reset_at"] = time.Now().Add(-time.Minute).Format(time.RFC3339)
	wire := `data: {"type":"response.completed","response":{"id":"resp_reset","status":"completed","output":[]}}` + "\n\n"
	upstream := &httpUpstreamRecorder{resp: &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": {"text/event-stream"}}, Body: io.NopCloser(strings.NewReader(wire))}}
	svc := openAIClientToolsTestService(upstream)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
	result, err := svc.Forward(context.Background(), c, account, []byte(`{"model":"gpt-6-astra","input":"test"}`))
	require.NoError(t, err)
	require.NotNil(t, result)
	require.Len(t, upstream.requests, 1)
}
