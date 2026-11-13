package service

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func excelBPSRepeatedResponse(status int, headers http.Header, body string, count int) *httpUpstreamRecorder {
	upstream := &httpUpstreamRecorder{}
	for i := 0; i < count; i++ {
		upstream.responses = append(upstream.responses, &http.Response{StatusCode: status, Header: headers.Clone(), Body: io.NopCloser(strings.NewReader(body))})
	}
	return upstream
}

func TestExcelBPSRejectionReplayBoundary(t *testing.T) {
	for _, tc := range []struct {
		name   string
		status int
		body   string
		retry  bool
	}{
		{"account forbidden", 403, `{"error":{"code":"permission_denied"}}`, true},
		{"authentication", 401, `{"error":{"code":"invalid_api_key"}}`, true},
		{"bad gateway", 502, `{"error":{"message":"Upstream request failed"}}`, true},
		{"unavailable", 503, `{"error":{"code":"overloaded_error"}}`, true},
		{"invalid request", 400, `{"error":{"code":"invalid_request_error"}}`, false},
		{"blocked conversation", 403, `{"error":{"code":"session_blocked_by_cyber_policy"}}`, false},
		{"content policy", 403, `{"error":{"code":"content_policy_violation"}}`, false},
	} {
		for _, stream := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/stream=%t", tc.name, stream), func(t *testing.T) {
				upstream := excelBPSRepeatedResponse(tc.status, http.Header{}, tc.body, 2)
				svc := openAIClientToolsTestService(upstream)
				rec := httptest.NewRecorder()
				c, _ := gin.CreateTestContext(rec)
				c.Request = httptest.NewRequest("POST", "/v1/responses", nil)
				_, err := svc.Forward(context.Background(), c, excelAccount(), []byte(fmt.Sprintf(`{"model":"gpt-6-astra","input":"test","stream":%t}`, stream)))
				require.Error(t, err)
				var failure *UpstreamFailoverError
				require.Equal(t, tc.retry, errors.As(err, &failure))
				if tc.retry {
					require.Len(t, upstream.requests, 2)
				} else {
					require.Len(t, upstream.requests, 1)
				}
				if tc.retry {
					require.False(t, c.Writer.Written())
					require.Empty(t, rec.Body.String())
					require.Equal(t, tc.status, failure.StatusCode)
					require.False(t, failure.ShouldReportAccountScheduleFailure())
				} else {
					require.True(t, c.Writer.Written())
				}
			})
		}
	}
}

func TestExcelBPSStreamReplayBoundary(t *testing.T) {
	const metadata = "event: response.created\ndata: {\"type\":\"response.created\",\"response\":{\"id\":\"resp_private\"}}\n\n"
	const failed = "event: response.failed\ndata: {\"type\":\"response.failed\",\"response\":{\"status\":\"failed\",\"error\":{\"type\":\"server_error\",\"code\":\"server_error\",\"message\":\"unavailable\"}}}\n\n"
	for _, tc := range []struct {
		name, prefix, suffix string
		retry                bool
		usage                int
	}{
		{"metadata rejection", metadata, failed, true, 0},
		{"empty stream", metadata, "", true, 0},
		{"text output", metadata + "event: response.output_text.delta\ndata: {\"type\":\"response.output_text.delta\",\"delta\":\"hello\"}\n\n", failed, false, 0},
		{"metered failure", metadata, "event: response.failed\ndata: {\"type\":\"response.failed\",\"response\":{\"status\":\"failed\",\"usage\":{\"input_tokens\":3,\"output_tokens\":2},\"error\":{\"type\":\"server_error\",\"code\":\"server_error\"}}}\n\n", false, 3},
	} {
		t.Run(tc.name, func(t *testing.T) {
			upstream := excelBPSRepeatedResponse(200, http.Header{"Content-Type": {"text/event-stream"}}, tc.prefix+tc.suffix, 2)
			svc := openAIClientToolsTestService(upstream)
			rec := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(rec)
			c.Request = httptest.NewRequest("POST", "/v1/responses", nil)
			result, err := svc.Forward(context.Background(), c, excelAccount(), []byte(`{"model":"gpt-6-astra","input":"test","stream":true}`))
			require.Error(t, err)
			var failure *UpstreamFailoverError
			require.Equal(t, tc.retry, errors.As(err, &failure))
			if tc.retry {
				require.Len(t, upstream.requests, 2)
			} else {
				require.Len(t, upstream.requests, 1)
			}
			if tc.retry {
				require.False(t, c.Writer.Written())
				require.Empty(t, rec.Body.String())
			} else {
				require.NotNil(t, result)
				require.True(t, c.Writer.Written())
				if tc.usage > 0 {
					require.EqualValues(t, tc.usage, result.Usage.InputTokens)
				}
			}
		})
	}
}

func TestExcelBPSSameAccountRetrySuccessDoesNotPunishAccount(t *testing.T) {
	const completed = "data: {\"type\":\"response.completed\",\"response\":{\"id\":\"resp_success\",\"status\":\"completed\",\"output\":[],\"usage\":{\"input_tokens\":3,\"output_tokens\":2}}}\n\n"
	for _, status := range []int{401, 403, 429, 502, 503} {
		t.Run(fmt.Sprint(status), func(t *testing.T) {
			upstream := &httpUpstreamRecorder{responses: []*http.Response{bpsCompletionResponse(status, `{"error":{"code":"permission_denied"}}`), bpsCompletionResponse(200, completed)}}
			svc := openAIClientToolsTestService(upstream)
			account := excelAccount()
			account.Extra["openai_excel_bps_auto_disable_on_403"] = true
			svc.accountRepo = &excelBPSAutoDisableRepo{disable: func(context.Context, *Account) (bool, error) {
				t.Fatal("a recovered first rejection must not disable the account")
				return false, nil
			}}
			c, _ := gin.CreateTestContext(httptest.NewRecorder())
			c.Request = httptest.NewRequest("POST", "/v1/responses", nil)
			result, err := svc.Forward(context.Background(), c, account, []byte(`{"model":"gpt-6-astra","input":"test"}`))
			require.NoError(t, err)
			require.NotNil(t, result)
			require.Len(t, upstream.requests, 2)
			require.Equal(t, upstream.requests[0].Header.Get("Authorization"), upstream.requests[1].Header.Get("Authorization"))
			require.False(t, svc.excelBPSFailureCooldownActive(account.ID))
			require.Equal(t, 3, result.Usage.InputTokens)
		})
	}
}

func TestExcelBPSSameAccountRetryHonorsExplicitHeaders(t *testing.T) {
	for _, tc := range []struct{ name, clientValue, upstreamValue string }{
		{"response opt out", "", "false"}, {"explicit account retry", "true", ""}, {"client default false", "false", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			upstream := excelBPSRepeatedResponse(403, http.Header{"X-Should-Retry": {tc.upstreamValue}}, `{"error":{"code":"permission_denied"}}`, 2)
			svc := openAIClientToolsTestService(upstream)
			c, _ := gin.CreateTestContext(httptest.NewRecorder())
			c.Request = httptest.NewRequest("POST", "/v1/responses", nil)
			if tc.clientValue != "" {
				c.Request.Header.Set("X-Sub2API-Retry-Account", tc.clientValue)
			}
			_, err := svc.Forward(context.Background(), c, excelAccount(), []byte(`{"model":"gpt-6-astra","input":"test"}`))
			var failure *UpstreamFailoverError
			require.ErrorAs(t, err, &failure)
			if tc.clientValue == "false" {
				require.Len(t, upstream.requests, 2)
			} else {
				require.Len(t, upstream.requests, 1)
			}
			require.False(t, c.Writer.Written())
		})
	}
}

func TestExcelBPSSameAccountStreamRetrySuccess(t *testing.T) {
	failed := "data: {\"type\":\"response.created\",\"response\":{\"id\":\"resp_rejected\"}}\n\ndata: {\"type\":\"response.failed\",\"response\":{\"status\":\"failed\",\"error\":{\"type\":\"server_error\",\"code\":\"server_error\"}}}\n\n"
	completed := "data: {\"type\":\"response.completed\",\"response\":{\"id\":\"resp_success\",\"status\":\"completed\",\"output\":[],\"usage\":{\"input_tokens\":3,\"output_tokens\":2}}}\n\n"
	upstream := &httpUpstreamRecorder{responses: []*http.Response{bpsCompletionResponse(200, failed), bpsCompletionResponse(200, completed)}}
	svc := openAIClientToolsTestService(upstream)
	account := excelAccount()
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest("POST", "/v1/responses", nil)
	result, err := svc.Forward(context.Background(), c, account, []byte(`{"model":"gpt-6-astra","input":"test","stream":true}`))
	require.NoError(t, err)
	require.NotNil(t, result)
	require.Len(t, upstream.requests, 2)
	require.Equal(t, 3, result.Usage.InputTokens)
	require.NotContains(t, rec.Body.String(), "resp_rejected")
	require.Contains(t, rec.Body.String(), "resp_success")
	require.NotContains(t, rec.Body.String(), "response.failed")
	require.False(t, svc.excelBPSFailureCooldownActive(account.ID))
}
