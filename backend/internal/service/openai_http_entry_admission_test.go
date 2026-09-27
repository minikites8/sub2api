package service

import (
	"context"
	"fmt"
	"maps"
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestOpenAIHTTPEntryRefreshesBPSConfiguration(t *testing.T) {
	for _, enabled := range []bool{false, true} {
		t.Run(fmt.Sprintf("latest_bps=%t", enabled), func(t *testing.T) {
			selected := excelAccount()
			selected.Extra["openai_excel_bps"] = !enabled
			latest := *selected
			latest.Extra = maps.Clone(selected.Extra)
			latest.Extra["openai_excel_bps"] = enabled
			upstream := &httpUpstreamRecorder{responses: []*http.Response{excelBPSOrdinaryResponse()}}
			svc := openAIClientToolsTestService(upstream)
			svc.accountRepo = &turnAdmissionRepo{account: &latest}
			body := []byte(`{"model":"gpt-5.6-sol","stream":false,"input":"complete original history"}`)
			c, rec := newExcelBPSFallbackContext(body, false)
			result, err := svc.Forward(context.Background(), c, selected, body)
			require.NoError(t, err)
			require.NotNil(t, result)
			require.Equal(t, http.StatusOK, rec.Code)
			require.Len(t, upstream.requests, 1)
			if enabled {
				require.Equal(t, "/basispoints/api/responses", upstream.requests[0].URL.Path)
			} else {
				require.Equal(t, chatgptCodexURL, upstream.requests[0].URL.String())
			}
			require.Equal(t, !enabled, selected.IsExcelBPSEnabled())
			require.Equal(t, enabled, latest.IsExcelBPSEnabled())
		})
	}
}

func TestOpenAIHTTPEntryKeepsContinuationsBound(t *testing.T) {
	selected := excelAccount()
	latest := *selected
	latest.Extra = maps.Clone(selected.Extra)
	latest.Extra["openai_excel_bps"] = false
	upstream := &httpUpstreamRecorder{}
	svc := openAIClientToolsTestService(upstream)
	svc.accountRepo = &turnAdmissionRepo{account: &latest}
	body := []byte(`{"model":"gpt-5.6-sol","previous_response_id":"resp_bound","input":"continue"}`)
	c, _ := newExcelBPSFallbackContext(body, false)
	_, err := svc.Forward(context.Background(), c, selected, body)
	require.Equal(t, "account_binding_changed", OpenAITurnAdmissionReason(err))
	require.Empty(t, upstream.requests)
	require.Contains(t, c.GetString(OpsUpstreamErrorMessageKey), "account_binding_changed")
}

func TestOpenAIHTTPEntryLogsActualReasonWithBPSDisabled(t *testing.T) {
	selected := excelAccount()
	selected.Extra["openai_excel_bps"] = false
	latest := *selected
	latest.Schedulable = false
	upstream := &httpUpstreamRecorder{}
	svc := openAIClientToolsTestService(upstream)
	svc.accountRepo = &turnAdmissionRepo{account: &latest}
	body := []byte(`{"model":"gpt-5.6-sol","input":"private conversation"}`)
	c, _ := newExcelBPSFallbackContext(body, false)
	c.Set(OpsUpstreamStatusCodeKey, http.StatusServiceUnavailable)
	_, err := svc.Forward(context.Background(), c, selected, body)
	require.Zero(t, c.GetInt(OpsUpstreamStatusCodeKey))
	require.Equal(t, "account_ineligible", OpenAITurnAdmissionReason(err))
	require.Empty(t, upstream.requests)
	eventsValue, ok := c.Get(OpsUpstreamErrorsKey)
	require.True(t, ok)
	events := eventsValue.([]*OpsUpstreamErrorEvent)
	require.Len(t, events, 1)
	require.Equal(t, "admission_error", events[0].Kind)
	require.Equal(t, "admission", events[0].Stage)
	require.Equal(t, "account_ineligible", events[0].Reason)
	require.Contains(t, events[0].Detail, "bps_enabled=false")
	require.NotContains(t, events[0].Detail, "private conversation")
	require.NotContains(t, events[0].Detail, "test-token")
	require.Contains(t, c.GetString(OpsUpstreamErrorMessageKey), "account_ineligible")
	clearOpenAIHTTPAdmissionFailure(c)
	require.Empty(t, c.GetString(OpsUpstreamErrorMessageKey))
	require.Empty(t, c.GetString(OpsUpstreamErrorDetailKey))
	preserved, _ := c.Get(OpsUpstreamErrorsKey)
	require.Len(t, preserved, 1)
}

type httpEntryChangingAdmissionRepo struct {
	AccountRepository
	first, later *Account
	reads        int
}

func (r *httpEntryChangingAdmissionRepo) GetOpenAITurnAdmission(context.Context, int64) (*Account, *Account, error) {
	r.reads++
	if r.reads == 1 {
		return r.first, nil, nil
	}
	return r.later, nil, nil
}

func TestOpenAIHTTPEntryPreservesPerSendAdmission(t *testing.T) {
	selected := excelAccount()
	selected.Extra["openai_excel_bps"] = false
	later := *selected
	later.Schedulable = false
	upstream := &httpUpstreamRecorder{}
	svc := openAIClientToolsTestService(upstream)
	svc.accountRepo = &httpEntryChangingAdmissionRepo{first: selected, later: &later}
	body := []byte(`{"model":"gpt-5.6-sol","input":"complete history"}`)
	c, _ := newExcelBPSFallbackContext(body, false)
	_, err := svc.Forward(context.Background(), c, selected, body)
	require.Equal(t, "account_ineligible", OpenAITurnAdmissionReason(err))
	require.Empty(t, upstream.requests)
}
