package service

import (
	"context"
	"fmt"
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

const excelBPSEligibilityMessage = "Account eligibility changed; please retry with complete context"

func TestExcelBPSAccountEligibilityChangedFallsBack(t *testing.T) {
	for _, tc := range []struct {
		name       string
		stream     bool
		sse        bool
		errorEvent bool
	}{
		{name: "HTTP buffered"},
		{name: "HTTP streaming", stream: true},
		{name: "SSE buffered", sse: true},
		{name: "SSE streaming", stream: true, sse: true},
		{name: "SSE error buffered", errorEvent: true},
		{name: "SSE error streaming", stream: true, errorEvent: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			rejected := excelBPSJSONResponse(http.StatusServiceUnavailable, `{"error":{"type":"admission_unavailable","message":"`+excelBPSEligibilityMessage+`"}}`)
			if tc.sse {
				rejected = excelBPSSSEResponse("event: response.failed\ndata: {\"type\":\"response.failed\",\"response\":{\"error\":{\"code\":\"admission_unavailable\",\"message\":\"" + excelBPSEligibilityMessage + "\"}}}\n\n")
			}
			if tc.errorEvent {
				rejected = excelBPSSSEResponse("event: error\ndata: {\"type\":\"error\",\"code\":\"admission_unavailable\",\"message\":\"" + excelBPSEligibilityMessage + "\"}\n\n")
			}
			upstream := &httpUpstreamRecorder{responses: []*http.Response{rejected, excelBPSOrdinaryResponse()}}
			svc := openAIClientToolsTestService(upstream)
			stream := "false"
			if tc.stream {
				stream = "true"
			}
			body := []byte(`{"model":"gpt-5.6-sol","stream":` + stream + `,"input":[{"role":"user","content":"original question"},{"role":"assistant","content":"earlier answer"},{"role":"user","content":"follow-up question"}],"tools":[{"type":"function","name":"keep_context","parameters":{"type":"object"}}]}`)
			originalBody := string(body)
			c, rec := newExcelBPSFallbackContext(body, tc.stream)
			account := excelAccount()
			latest := *account
			svc.accountRepo = &turnAdmissionRepo{account: &latest}

			result, err := svc.Forward(context.Background(), c, account, body)
			require.NoError(t, err)
			require.NotNil(t, result)
			require.Len(t, upstream.requests, 2)
			require.Equal(t, "/basispoints/api/responses", upstream.requests[0].URL.Path)
			require.Equal(t, chatgptCodexURL, upstream.requests[1].URL.String())
			require.Equal(t, upstream.requests[0].Header.Get("Authorization"), upstream.requests[1].Header.Get("Authorization"))
			require.Equal(t, "gpt-5.6-sol", gjson.GetBytes(upstream.bodies[1], "model").String())
			for _, text := range []string{"original question", "earlier answer", "follow-up question", "keep_context"} {
				require.Contains(t, string(upstream.bodies[1]), text)
			}
			require.Equal(t, originalBody, string(body))
			require.True(t, account.IsExcelBPSEnabled())
			require.True(t, latest.IsExcelBPSEnabled())
			require.Contains(t, rec.Body.String(), "ordinary output")
			require.NotContains(t, rec.Body.String(), excelBPSEligibilityMessage)
			require.Equal(t, "codex", rec.Header().Get("X-Codex2API-Upstream"))
			require.Equal(t, "account_eligibility_changed", rec.Header().Get("X-Codex2API-Basispoints-Bypass"))
			require.Equal(t, openAIResponsesUpstreamEndpoint, GetActualOpenAIUpstreamEndpoint(c))
			eventsValue, ok := c.Get(OpsUpstreamErrorsKey)
			require.True(t, ok)
			events := eventsValue.([]*OpsUpstreamErrorEvent)
			fallback := events[len(events)-1]
			require.Equal(t, "bps_native_fallback", fallback.Kind)
			require.Equal(t, "route", fallback.Stage)
			require.Equal(t, "bps", fallback.Scope)
			require.Equal(t, "account_eligibility_changed", fallback.Reason)
			require.Contains(t, fallback.Detail, "request_path=/v1/responses")
			require.Contains(t, fallback.Detail, "stream="+stream)
			require.Equal(t, account.ID, fallback.AccountID)
			require.Equal(t, "gpt-5.6-sol", fallback.RequestedModel)
		})
	}
}

func TestExcelBPSAccountEligibilityChangedClassification(t *testing.T) {
	for _, tc := range []struct {
		name    string
		status  int
		payload string
		match   bool
	}{
		{name: "HTTP error type", status: 503, payload: `{"error":{"type":"admission_unavailable","message":"` + excelBPSEligibilityMessage + `"}}`, match: true},
		{name: "HTTP error code", status: 503, payload: `{"error":{"code":"admission_unavailable","message":"` + excelBPSEligibilityMessage + `"}}`, match: true},
		{name: "SSE response error", status: 200, payload: `{"response":{"error":{"message":"` + excelBPSEligibilityMessage + `"}}}`, match: true},
		{name: "SSE top-level error", status: 200, payload: `{"type":"error","code":"admission_unavailable","message":"` + excelBPSEligibilityMessage + `"}`, match: true},
		{name: "normalized message", status: 503, payload: `{"error":{"message":"  account eligibility changed; please retry with complete context  "}}`, match: true},
		{name: "other admission failure", status: 503, payload: `{"error":{"type":"admission_unavailable","message":"account disabled"}}`},
		{name: "generic service failure", status: 503, payload: `{"error":{"message":"temporarily unavailable"}}`},
		{name: "malformed JSON", status: 503, payload: `{"error":`},
		{name: "plain text", status: 503, payload: excelBPSEligibilityMessage},
		{name: "input echo", status: 503, payload: `{"input":{"message":"` + excelBPSEligibilityMessage + `"}}`},
		{name: "authentication failure", status: 401, payload: `{"error":{"message":"` + excelBPSEligibilityMessage + `"}}`},
		{name: "authorization failure", status: 403, payload: `{"error":{"message":"` + excelBPSEligibilityMessage + `"}}`},
		{name: "rate limited", status: 429, payload: `{"error":{"message":"` + excelBPSEligibilityMessage + `"}}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			require.Equal(t, tc.match, excelBPSAccountEligibilityChanged(tc.status, []byte(tc.payload)))
		})
	}
}

func excelBPSEligibilityHTTPResponse() *http.Response {
	return excelBPSJSONResponse(http.StatusServiceUnavailable, `{"error":{"type":"admission_unavailable","message":"`+excelBPSEligibilityMessage+`"}}`)
}

func TestExcelBPSAccountEligibilityFallbackStopsAfterOutput(t *testing.T) {
	for _, stream := range []bool{false, true} {
		t.Run(fmt.Sprintf("stream=%t", stream), func(t *testing.T) {
			body := []byte(fmt.Sprintf(`{"model":"gpt-5.6-sol","stream":%t,"input":"test"}`, stream))
			sse := "event: response.output_text.delta\ndata: {\"type\":\"response.output_text.delta\",\"delta\":\"already generated\"}\n\n" +
				"event: response.failed\ndata: {\"type\":\"response.failed\",\"response\":{\"error\":{\"code\":\"admission_unavailable\",\"message\":\"" + excelBPSEligibilityMessage + "\"}}}\n\n"
			upstream := &httpUpstreamRecorder{responses: []*http.Response{excelBPSSSEResponse(sse), excelBPSOrdinaryResponse()}}
			svc := openAIClientToolsTestService(upstream)
			c, _ := newExcelBPSFallbackContext(body, stream)
			_, err := svc.Forward(context.Background(), c, excelAccount(), body)
			require.Error(t, err)
			require.Len(t, upstream.requests, 1)
			require.Empty(t, c.Writer.Header().Get("X-Codex2API-Basispoints-Bypass"))
		})
	}
}

func TestExcelBPSAccountEligibilityFallbackPreservesProbe(t *testing.T) {
	upstream := &httpUpstreamRecorder{responses: []*http.Response{excelBPSEligibilityHTTPResponse(), excelBPSOrdinaryResponse()}}
	svc := openAIClientToolsTestService(upstream)
	body := []byte(`{"model":"gpt-5.6-sol","input":"probe"}`)
	c, _ := newExcelBPSFallbackContext(body, false)
	c.Set(bpsAccountProbeRequiredContextKey, true)
	_, err := svc.Forward(context.Background(), c, excelAccount(), body)
	require.Error(t, err)
	require.Len(t, upstream.requests, 1)
	require.Empty(t, c.Writer.Header().Get("X-Codex2API-Basispoints-Bypass"))
}

func TestExcelBPSAccountEligibilityFallbackStopsOnCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	calls := 0
	svc := openAIClientToolsTestService(nil)
	svc.httpUpstream = &bpsTestUpstream{send: func(req *http.Request, _ string) (*http.Response, error) {
		calls++
		require.Equal(t, "/basispoints/api/responses", req.URL.Path)
		require.NoError(t, req.Body.Close())
		cancel()
		return excelBPSEligibilityHTTPResponse(), nil
	}}
	body := []byte(`{"model":"gpt-5.6-sol","input":"cancel"}`)
	c, _ := newExcelBPSFallbackContext(body, false)
	c.Request = c.Request.WithContext(ctx)
	_, err := svc.Forward(context.Background(), c, excelAccount(), body)
	require.Error(t, err)
	require.Equal(t, 1, calls)
	require.Empty(t, c.Writer.Header().Get("X-Codex2API-Basispoints-Bypass"))
}

func TestExcelBPSAccountEligibilityFallbackUsesOrdinaryErrorHandlingOnce(t *testing.T) {
	upstream := &httpUpstreamRecorder{responses: []*http.Response{excelBPSEligibilityHTTPResponse(), excelBPSJSONResponse(http.StatusBadRequest, `{"error":{"type":"invalid_request_error","message":"ordinary validation error"}}`)}}
	svc := openAIClientToolsTestService(upstream)
	body := []byte(`{"model":"gpt-5.6-sol","input":"test"}`)
	c, _ := newExcelBPSFallbackContext(body, false)
	_, err := svc.Forward(context.Background(), c, excelAccount(), body)
	require.EqualError(t, err, "upstream error: 400 (client response sanitized)")
	require.Len(t, upstream.requests, 2)
	require.Equal(t, chatgptCodexURL, upstream.requests[1].URL.String())
}

func TestExcelBPSAccountEligibilityFallbackPreservesLocalAdmission(t *testing.T) {
	for _, disableBeforeBPS := range []bool{false, true} {
		t.Run(fmt.Sprintf("disabled_before_bps=%t", disableBeforeBPS), func(t *testing.T) {
			account := excelAccount()
			latest := *account
			if disableBeforeBPS {
				latest.Status = "disabled"
			}
			calls := 0
			svc := openAIClientToolsTestService(nil)
			svc.accountRepo = &turnAdmissionRepo{account: &latest}
			svc.httpUpstream = &bpsTestUpstream{send: func(req *http.Request, _ string) (*http.Response, error) {
				calls++
				require.Equal(t, "/basispoints/api/responses", req.URL.Path)
				require.NoError(t, req.Body.Close())
				latest.Status = "disabled"
				return excelBPSEligibilityHTTPResponse(), nil
			}}
			body := []byte(`{"model":"gpt-5.6-sol","input":"test"}`)
			c, _ := newExcelBPSFallbackContext(body, false)
			_, err := svc.Forward(context.Background(), c, account, body)
			require.True(t, IsOpenAITurnAdmissionError(err), "%v", err)
			expectedCalls := 1
			if disableBeforeBPS {
				expectedCalls = 0
			}
			require.Equal(t, expectedCalls, calls)
		})
	}
}

func TestExcelBPSAccountEligibilityFallbackPreservesOrdinaryRouting(t *testing.T) {
	upstream := &httpUpstreamRecorder{responses: []*http.Response{excelBPSOrdinaryResponse()}}
	svc := openAIClientToolsTestService(upstream)
	account := excelAccount()
	account.Extra["openai_excel_bps"] = false
	body := []byte(`{"model":"gpt-5.6-sol","input":"test"}`)
	c, _ := newExcelBPSFallbackContext(body, false)
	result, err := svc.Forward(context.Background(), c, account, body)
	require.NoError(t, err)
	require.NotNil(t, result)
	require.Len(t, upstream.requests, 1)
	require.Equal(t, chatgptCodexURL, upstream.requests[0].URL.String())
	require.Empty(t, c.Writer.Header().Get("X-Codex2API-Basispoints-Bypass"))
}
