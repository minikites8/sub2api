package handler

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestOpenAIDefaultRetryPinsOneAccountAndCapsDeadline(t *testing.T) {
	for _, kind := range []string{service.AccountTypeOAuth, service.AccountTypeAPIKey} {
		for _, status := range []int{429, 502, 503} {
			t.Run(kind+http.StatusText(status), func(t *testing.T) {
				account := &service.Account{ID: 71, Platform: service.PlatformOpenAI, Type: kind}
				c, _ := gin.CreateTestContext(httptest.NewRecorder())
				c.Request = httptest.NewRequest("POST", "/v1/responses", nil)
				calls := 0
				_, err := forwardOpenAIWithDefaultRetry(c, account, func() (*service.OpenAIForwardResult, error) {
					calls++
					service.ObserveOpenAIDefaultRetrySubmission(c.Request.Context())
					return nil, &service.UpstreamFailoverError{StatusCode: status, RetryableOnSameAccount: true, SameAccountRetryDeadline: time.Now().Add(2 * time.Minute)}
				})
				require.Equal(t, 2, calls)
				var failure *service.UpstreamFailoverError
				require.ErrorAs(t, err, &failure)
				require.False(t, failure.RetryableOnSameAccount)
				require.False(t, sameAccountRetryAllowed(failure, 0, 3))
				require.False(t, c.Writer.Written())
			})
		}
	}
}

func TestOpenAIDefaultRetryPreservesExplicitPoolPolicy(t *testing.T) {
	for _, override := range []map[string]any{
		{"pool_mode": true, "pool_mode_retry_count": 0},
		{"pool_mode": true, "pool_mode_retry_count": 5},
		{"pool_mode": true, "pool_mode_retry_status_codes": []any{}},
		{"pool_mode": true, "pool_mode_retry_status_codes": []any{float64(403)}},
	} {
		c, _ := gin.CreateTestContext(httptest.NewRecorder())
		c.Request = httptest.NewRequest("POST", "/v1/responses", nil)
		account := &service.Account{Platform: service.PlatformOpenAI, Type: service.AccountTypeAPIKey, Credentials: override}
		original := &service.UpstreamFailoverError{StatusCode: 502, RetryableOnSameAccount: true}
		calls := 0
		_, err := forwardOpenAIWithDefaultRetry(c, account, func() (*service.OpenAIForwardResult, error) {
			calls++
			service.ObserveOpenAIDefaultRetrySubmission(c.Request.Context())
			return nil, original
		})
		require.Equal(t, 1, calls)
		require.Same(t, original, err)
	}
}

func TestOpenAIDefaultRetryNeverReplaysUnsafeWork(t *testing.T) {
	for _, name := range []string{"usage", "output", "canceled", "policy", "session", "header timeout", "first output timeout", "body read failure", "request scope", "provider opt out", "prefer next", "bps", "internal recovery", "actual BPS route"} {
		t.Run(name, func(t *testing.T) {
			c, _ := gin.CreateTestContext(httptest.NewRecorder())
			c.Request = httptest.NewRequest("POST", "/v1/responses", nil)
			account := &service.Account{ID: 71, Platform: service.PlatformOpenAI, Type: service.AccountTypeOAuth}
			failure := &service.UpstreamFailoverError{StatusCode: 502, RetryableOnSameAccount: true}
			var partial *service.OpenAIForwardResult
			switch name {
			case "usage":
				partial = &service.OpenAIForwardResult{Usage: service.OpenAIUsage{InputTokens: 3}}
			case "output":
				_, err := c.Writer.WriteString("text")
				require.NoError(t, err)
			case "canceled":
				ctx, cancel := context.WithCancel(context.Background())
				cancel()
				c.Request = c.Request.WithContext(ctx)
			case "policy":
				failure.ResponseBody = []byte(`{"error":{"code":"content_policy_violation"}}`)
			case "session":
				failure.ResponseBody = []byte(`{"error":{"code":"session_blocked_by_cyber_policy"}}`)
			case "header timeout":
				failure.HeaderTimeoutSubmitted = true
			case "first output timeout":
				failure.StatusCode = http.StatusGatewayTimeout
				failure.Reason = service.OpenAIFirstOutputTimeoutReason
				failure.SafeToFailoverAfterWrite = true
			case "body read failure":
				failure.Reason = service.OpenAITransportFailureReason
			case "request scope":
				failure.Scope = service.GatewayFailureScopeRequest
			case "provider opt out":
				failure.ResponseHeaders = http.Header{"X-Should-Retry": {"false"}}
			case "prefer next":
				c.Request.Header.Set("X-Sub2API-Retry-Account", "true")
			case "actual BPS route":
				service.SetActualOpenAIUpstreamEndpoint(c, "/basispoints/api/responses")
			case "internal recovery":
			case "bps":
				failure.Reason = service.ExcelBPSRateLimitedReason
			}
			calls := 0
			_, err := forwardOpenAIWithDefaultRetry(c, account, func() (*service.OpenAIForwardResult, error) {
				calls++
				service.ObserveOpenAIDefaultRetrySubmission(c.Request.Context())
				if name == "internal recovery" {
					service.ObserveOpenAIDefaultRetrySubmission(c.Request.Context())
				}
				return partial, failure
			})
			require.Error(t, err)
			require.Equal(t, 1, calls)
			require.True(t, errors.As(err, &failure))
		})
	}
}

func TestOpenAIDefaultRetryWaitHonorsDeadlineAndProviderHint(t *testing.T) {
	now := time.Now()
	failure := &service.UpstreamFailoverError{SameAccountRetryDelay: time.Second, ResponseHeaders: http.Header{"Retry-After": {"3"}}}
	delay, allowed := openAIDefaultRetryWait(failure, now)
	require.True(t, allowed)
	require.Equal(t, 3*time.Second, delay)
	failure.SameAccountRetryDeadline = now.Add(2 * time.Second)
	_, allowed = openAIDefaultRetryWait(failure, now)
	require.False(t, allowed)
	failure.SameAccountRetryDeadline = now.Add(time.Minute)
	failure.ResponseHeaders.Set("Retry-After", now.Add(5*time.Second).UTC().Format(http.TimeFormat))
	delay, allowed = openAIDefaultRetryWait(failure, now)
	require.True(t, allowed)
	require.InDelta(t, 5, delay.Seconds(), 1)
}

func TestOpenAIDefaultRetrySharesRequestWaitBudgetAcrossAccounts(t *testing.T) {
	budget := &retryWaitBudget{waited: maxRequestRetryWait - sameAccountRetryDelay}
	for index, id := range []int64{71, 72} {
		c, _ := gin.CreateTestContext(httptest.NewRecorder())
		c.Request = httptest.NewRequest("POST", "/v1/responses", nil)
		account := &service.Account{ID: id, Platform: service.PlatformOpenAI, Type: service.AccountTypeOAuth}
		calls := 0
		_, err := forwardOpenAIWithDefaultRetry(c, account, func() (*service.OpenAIForwardResult, error) {
			calls++
			service.ObserveOpenAIDefaultRetrySubmission(c.Request.Context())
			return nil, &service.UpstreamFailoverError{StatusCode: 503}
		}, budget)
		require.Error(t, err)
		if index == 0 {
			require.Equal(t, 2, calls)
		} else {
			require.Equal(t, 1, calls, "a new account must not reset the request's wait budget")
		}
	}
	require.Equal(t, maxRequestRetryWait, budget.waited)
}

func TestOpenAIDefaultRetryAllowsOnlyUnmeteredMetadataResult(t *testing.T) {
	for _, tc := range []struct {
		name      string
		result    *service.OpenAIForwardResult
		wantCalls int
	}{
		{"metadata", &service.OpenAIForwardResult{RequestID: "req_metadata", ResponseID: "resp_metadata", Stream: true}, 2},
		{"tokens", &service.OpenAIForwardResult{Usage: service.OpenAIUsage{InputTokens: 1}}, 1},
		{"hosted tool", &service.OpenAIForwardResult{Usage: service.OpenAIUsage{HostedWebSearchCalls: 1}}, 1},
		{"observed usage", &service.OpenAIForwardResult{ObservedUsageOnError: true}, 1},
		{"unavailable usage", &service.OpenAIForwardResult{UsageUnavailable: true}, 1},
		{"content", &service.OpenAIForwardResult{FirstActualContentKind: "text"}, 1},
		{"search", &service.OpenAIForwardResult{SearchCount: 1}, 1},
		{"regional settlement", &service.OpenAIForwardResult{RegionalMetered: true}, 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c, _ := gin.CreateTestContext(httptest.NewRecorder())
			c.Request = httptest.NewRequest("POST", "/v1/messages", nil)
			account := &service.Account{ID: 71, Platform: service.PlatformOpenAI, Type: service.AccountTypeOAuth}
			calls := 0
			_, err := forwardOpenAIWithDefaultRetry(c, account, func() (*service.OpenAIForwardResult, error) {
				calls++
				service.ObserveOpenAIDefaultRetrySubmission(c.Request.Context())
				return tc.result, &service.UpstreamFailoverError{StatusCode: 503, SameAccountRetryDelay: time.Nanosecond}
			})
			require.Error(t, err)
			require.Equal(t, tc.wantCalls, calls)
		})
	}
}
