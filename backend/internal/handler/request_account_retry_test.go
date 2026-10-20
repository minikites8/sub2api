//go:build unit

package handler

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/pkg/requestretry"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type requestAccountRetryUpstream struct {
	service.HTTPUpstream
	status               int
	eventType, errorBody string
	partial, allFail     bool
	preamble             string
	failureIDs           map[int64]bool
	cancel               context.CancelFunc
	hits                 []int64
}

func (u *requestAccountRetryUpstream) Do(req *http.Request, _ string, id int64, _ int) (*http.Response, error) {
	u.hits = append(u.hits, id)
	if req.Header.Get(requestretry.AccountHeader) != "" || req.Header.Get(requestretry.MaxRetriesHeader) != "" {
		return nil, fmt.Errorf("gateway retry control headers leaked upstream")
	}
	if u.cancel != nil {
		u.cancel()
	}
	status, contentType := http.StatusOK, "text/event-stream"
	body := "data: {\"type\":\"response.output_text.delta\",\"delta\":\"healthy\"}\n\ndata: " + `{"type":"response.completed","response":{"id":"resp_healthy","object":"response","model":"gpt-5.2","status":"completed","output":[{"type":"message","role":"assistant","content":[{"type":"output_text","text":"healthy"}]}],"usage":{"input_tokens":1,"output_tokens":1}}}` + "\n\n"
	if id == 801 || u.allFail || u.failureIDs[id] {
		status, contentType = u.status, "application/json"
		body = u.errorBody
		if body == "" {
			body = `{"error":{"type":"invalid_request_error","message":"provider could not handle this request"}}`
		}
		if u.eventType != "" {
			status, contentType = http.StatusOK, "text/event-stream"
			if u.eventType == "response.failed" {
				body = `{"type":"response.failed","response":{"id":"resp_failed_attempt","status":"failed","error":` + strings.TrimSuffix(strings.TrimPrefix(body, `{"error":`), "}") + `}}`
			} else {
				body = `{"type":"error",` + strings.TrimPrefix(body, "{")
			}
			prefix := "data: {\"type\":\"response.created\",\"response\":{\"id\":\"resp_failed_attempt\"}}\n\n"
			if u.partial {
				prefix += "data: {\"type\":\"response.output_text.delta\",\"delta\":\"partial\"}\n\n"
			}
			prefix += u.preamble
			body = prefix + "data: " + body + "\n\n"
		}
	}
	return &http.Response{StatusCode: status, Header: http.Header{"Content-Type": {contentType}}, Body: io.NopCloser(strings.NewReader(body))}, nil
}

func TestRequestAccountRetryOverloadAfterEmptyProgressSwitchesToAPIKey(t *testing.T) {
	for _, progress := range []string{
		`{"type":"response.output_text.delta","delta":""}`,
		`{"type":"response.reasoning_summary_text.delta","delta":""}`,
		`{"type":"response.output_item.done","item":{"type":"reasoning","summary":[]}}`,
		`{"type":"response.content_part.done","part":{"type":"output_text","text":""}}`,
	} {
		for _, stream := range []bool{false, true} {
			for _, passthrough := range []bool{false, true} {
				t.Run(fmt.Sprintf("%s/stream=%t/passthrough=%t", progress, stream, passthrough), func(t *testing.T) {
					u := &requestAccountRetryUpstream{
						eventType:  "response.failed",
						errorBody:  `{"error":{"code":"server_error","message":"Our servers are currently overloaded. Please try again later."}}`,
						preamble:   "data: {\"type\":\"response.in_progress\"}\n\ndata: {\"type\":\"response.output_item.added\",\"item\":{\"type\":\"reasoning\",\"summary\":[]}}\n\ndata: " + progress + "\n\n",
						failureIDs: map[int64]bool{802: true},
					}
					accounts := []service.Account{
						{ID: 801, Platform: service.PlatformOpenAI, Type: service.AccountTypeOAuth, Status: service.StatusActive, Schedulable: true, Concurrency: 1, Priority: 1,
							Credentials: map[string]any{"access_token": "test-access"}, Extra: map[string]any{"openai_passthrough": passthrough}},
						{ID: 802, Platform: service.PlatformOpenAI, Type: service.AccountTypeOAuth, Status: service.StatusActive, Schedulable: true, Concurrency: 1, Priority: 2,
							Credentials: map[string]any{"access_token": "test-access-2"}, Extra: map[string]any{"openai_passthrough": passthrough}},
						{ID: 803, Platform: service.PlatformOpenAI, Type: service.AccountTypeAPIKey, Status: service.StatusActive, Schedulable: true, Concurrency: 1, Priority: 3,
							Credentials: map[string]any{"api_key": "test-key", "base_url": "https://api.example.test/v1"}},
					}
					router := newOpenAI502503RouterWithAccounts(t, u, accounts)
					req := httptest.NewRequest("POST", "/v1/responses", strings.NewReader(fmt.Sprintf(`{"model":"gpt-5.2","input":"hello","stream":%t}`, stream)))
					req.Header.Set(requestretry.AccountHeader, "True")
					req.Header.Set(requestretry.MaxRetriesHeader, "2")
					rec := httptest.NewRecorder()
					router.ServeHTTP(rec, req)
					require.Equal(t, []int64{801, 802, 803}, u.hits, rec.Body.String())
					require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
					require.Contains(t, rec.Body.String(), "healthy")
					require.NotContains(t, rec.Body.String(), "resp_failed_attempt")
					require.NotContains(t, rec.Body.String(), "overloaded")
				})
			}
		}
	}
}

var requestAccountRetryEndpoints = []struct{ path, body string }{
	{"/v1/responses", `{"model":"gpt-5.2","input":"hello","stream":false}`},
	{"/v1/responses", `{"model":"gpt-5.2","input":"hello","stream":true}`},
	{"/v1/chat/completions", `{"model":"gpt-5.2","messages":[{"role":"user","content":"hello"}],"stream":false}`},
	{"/v1/chat/completions", `{"model":"gpt-5.2","messages":[{"role":"user","content":"hello"}],"stream":true}`},
	{"/v1/messages", `{"model":"gpt-5.2","messages":[{"role":"user","content":"hello"}],"max_tokens":32,"stream":false}`},
	{"/v1/messages", `{"model":"gpt-5.2","messages":[{"role":"user","content":"hello"}],"max_tokens":32,"stream":true}`},
}

func TestRequestAccountRetrySwitchesOnUpstreamError(t *testing.T) {
	for _, endpoint := range requestAccountRetryEndpoints {
		for _, eventType := range []string{"", "error", "response.failed"} {
			t.Run(endpoint.path+"/"+endpoint.body+"/"+eventType, func(t *testing.T) {
				u := &requestAccountRetryUpstream{status: 400, eventType: eventType}
				router := newOpenAI502503Router(t, u)
				req := httptest.NewRequest("POST", endpoint.path, strings.NewReader(endpoint.body))
				req.Header.Set(requestretry.AccountHeader, "True")
				req.Header.Set(requestretry.MaxRetriesHeader, "1")
				rec := httptest.NewRecorder()
				router.ServeHTTP(rec, req)
				require.Equal(t, []int64{801, 802}, u.hits, rec.Body.String())
				require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
				require.Contains(t, rec.Body.String(), "healthy")
				require.NotContains(t, rec.Body.String(), "resp_failed_attempt")
			})
		}
	}
}

func TestRequestAccountRetryLimitsAndLegacyBehavior(t *testing.T) {
	for _, tc := range []struct {
		name, optIn, max string
		status           int
		wantHits         []int64
	}{
		{"missing", "", "", 400, []int64{801}},
		{"false", "false", "1", 400, []int64{801}},
		{"limit alone", "", "0", 400, []int64{801}},
		{"zero", "True", "0", 400, []int64{801}},
		{"one", "true", "1", 400, []int64{801, 802}},
		{"two", "true", "2", 400, []int64{801, 802, 803}},
		{"server cap", "true", "100", 400, []int64{801, 802, 803, 804}},
		{"invalid limit", "true", "-1", 400, []int64{801, 802, 803, 804}},
		{"zero with legacy retryable error", "true", "0", 503, []int64{801}},
		{"skip same account", "true", "1", 503, []int64{801, 802}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			u := &requestAccountRetryUpstream{status: tc.status, allFail: true}
			router := newOpenAI502503Router(t, u, 801, 802, 803, 804, 805)
			req := httptest.NewRequest("POST", "/v1/responses", strings.NewReader(requestAccountRetryEndpoints[0].body))
			req.Header.Set(requestretry.AccountHeader, tc.optIn)
			req.Header.Set(requestretry.MaxRetriesHeader, tc.max)
			rec := httptest.NewRecorder()
			router.ServeHTTP(rec, req)
			require.Equal(t, tc.wantHits, u.hits, rec.Body.String())
			require.Contains(t, rec.Body.String(), "error")
		})
	}
}

func TestRequestAccountRetryKeepsReplayBoundaries(t *testing.T) {
	for _, mode := range []string{"partial", "cancelled", "policy", "context", "local validation"} {
		t.Run(mode, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			u := &requestAccountRetryUpstream{status: 400}
			body := requestAccountRetryEndpoints[1].body
			switch mode {
			case "partial":
				u.eventType, u.partial = "response.failed", true
			case "cancelled":
				u.cancel = cancel
			case "policy":
				u.errorBody = `{"error":{"code":"content_policy_violation","type":"invalid_request_error","message":"content rejected"}}`
			case "context":
				u.errorBody = `{"error":{"code":"context_length_exceeded","type":"invalid_request_error","message":"maximum context length exceeded"}}`
			case "local validation":
				body = `{"input":"missing model"}`
			}
			router := newOpenAI502503Router(t, u)
			req := httptest.NewRequest("POST", "/v1/responses", strings.NewReader(body)).WithContext(ctx)
			req.Header.Set(requestretry.AccountHeader, "true")
			rec := httptest.NewRecorder()
			router.ServeHTTP(rec, req)
			if mode == "local validation" {
				require.Empty(t, u.hits)
			} else {
				require.Equal(t, []int64{801}, u.hits, rec.Body.String())
			}
			if mode == "partial" {
				require.Contains(t, rec.Body.String(), "partial")
			}
		})
	}
}

func TestRequestAccountRetrySharedFailoverState(t *testing.T) {
	for _, limit := range []string{"0", "1"} {
		c, _ := gin.CreateTestContext(httptest.NewRecorder())
		c.Request = httptest.NewRequest("POST", "/v1/messages", nil)
		c.Request.Header.Set(requestretry.AccountHeader, "true")
		c.Request.Header.Set(requestretry.MaxRetriesHeader, limit)
		state := newRequestFailoverState(c, 3, true)
		unscheduler := &mockTempUnscheduler{}
		failover := &service.UpstreamFailoverError{StatusCode: 503, RetryableOnSameAccount: true}
		action := state.HandleFailoverError(context.Background(), unscheduler, 1, service.PlatformAnthropic, 3, failover)
		if limit == "0" {
			require.Equal(t, FailoverExhausted, action)
		} else {
			require.Equal(t, FailoverContinue, action)
			require.Equal(t, FailoverExhausted, state.HandleFailoverError(context.Background(), unscheduler, 2, service.PlatformAnthropic, 3, failover))
		}
		require.Empty(t, state.SameAccountRetryCount)
		require.Empty(t, unscheduler.calls)
		require.Equal(t, FailoverExhausted, state.HandleSelectionExhausted(context.Background()))
	}
}

func TestRequestAccountRetryWebSocket(t *testing.T) {
	headers := http.Header{}
	headers.Set(requestretry.AccountHeader, "True")
	headers.Set(requestretry.MaxRetriesHeader, "1")
	for _, mode := range []string{service.OpenAIWSIngressModePassthrough, service.OpenAIWSIngressModeCtxPool} {
		t.Run(mode, func(t *testing.T) {
			runWSProcessingFixture(t, mode, `{"type":"error","error":{"type":"invalid_request_error","message":"provider cannot process this request"}}`, true, false, headers)
		})
	}
}

func TestRequestAccountRetryBPSBudget(t *testing.T) {
	for _, maxRetries := range []int{0, 1, 2, 3} {
		t.Run(fmt.Sprint(maxRetries), func(t *testing.T) {
			u := &excelBPSRateLimitUpstream{allFail: true, failureStatus: 503, ordinaryStatus: 400}
			router := newExcelBPSRateLimitRouter(t, u)
			req := httptest.NewRequest("POST", "/v1/responses", strings.NewReader(`{"model":"gpt-6-sol","input":"hello","stream":false}`))
			req.Header.Set(requestretry.AccountHeader, "True")
			req.Header.Set(requestretry.MaxRetriesHeader, fmt.Sprint(maxRetries))
			rec := httptest.NewRecorder()
			router.ServeHTTP(rec, req)
			require.NotEmpty(t, u.hits)
			require.LessOrEqual(t, len(u.hits), maxRetries+1, "BPS fallback must not reset the request budget: %v", u.paths)
			require.Contains(t, rec.Body.String(), "error")
		})
	}
}
