package service

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestForwardOpenAIWSV2_EmptyProgressKeepsAccountFailover(t *testing.T) {
	for _, transport := range []string{"accelerated_oauth", "native_apikey", "nonstream_apikey"} {
		for _, eventType := range []string{"error", "response.failed"} {
			for _, progress := range []string{
				`{"type":"response.metadata","metadata":{"trace":"failed-metadata"}}`,
				`{"type":"codex.response.metadata","headers":{"x-codex-turn-state":"failed-state"}}`,
				`{"type":"response.output_text.delta","delta":""}`,
				`{"type":"response.reasoning_summary_text.delta","delta":""}`,
				`{"type":"response.output_item.done","item":{"type":"reasoning","summary":[],"encrypted_content":"failed-ciphertext"}}`,
			} {
				t.Run(fmt.Sprintf("%s/%s/%s", transport, eventType, progress), func(t *testing.T) {
					rec := httptest.NewRecorder()
					c, _ := gin.CreateTestContext(rec)
					c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
					errorBody := `{"code":"server_is_overloaded","type":"service_unavailable_error","message":"Our servers are currently overloaded. Please try again later."}`
					failure := `{"type":"error","error":` + errorBody + `}`
					if eventType == "response.failed" {
						failure = `{"type":"response.failed","response":{"id":"resp_failed","status":"failed","error":` + errorBody + `}}`
					}
					conn := &openAIWSCaptureConn{events: [][]byte{
						[]byte(`{"type":"response.created","response":{"id":"resp_failed"}}`),
						[]byte(progress), []byte(failure),
					}}
					d := &wsSSETestDialer{conn: conn}
					h := &httpUpstreamRecorder{}
					s := wsSSETestService(t, d, h)
					s.openaiWSPool.setClientDialerForTest(&wsSSEGuardHeaderDialer{d, http.Header{
						http.CanonicalHeaderKey(openAIWSTurnStateHeader): {"failed-handshake-state"},
						"X-Request-Id": {"req_failed_ws"},
					}})
					a := wsSSETestAccount()
					if transport == "accelerated_oauth" {
						SetOpenAIClientTransport(c, OpenAIClientTransportHTTP)
					}
					stream := transport != "nonstream_apikey"
					if transport != "accelerated_oauth" {
						a.Type = AccountTypeAPIKey
						a.Credentials = map[string]any{"api_key": "test-key"}
						a.Extra = map[string]any{"openai_apikey_responses_websockets_v2_enabled": true}
					}
					_, err := s.Forward(context.Background(), c, a, []byte(fmt.Sprintf(`{"model":"gpt-6.1-sol","stream":%t,"input":"hello"}`, stream)))
					var failover *UpstreamFailoverError
					require.ErrorAs(t, err, &failover)
					require.Equal(t, http.StatusServiceUnavailable, failover.StatusCode)
					require.True(t, failover.ShouldRetryNextAccount())
					require.False(t, failover.RetryableOnSameAccount)
					require.False(t, c.Writer.Written())
					require.Empty(t, rec.Body.String())
					require.Empty(t, rec.Header().Get(openAIWSTurnStateHeader))
					require.Empty(t, h.requests, "an explicit overload must reach account failover, not HTTP replay")
					require.EqualValues(t, 1, d.calls.Load())
					events, ok := c.MustGet(OpsUpstreamErrorsKey).([]*OpsUpstreamErrorEvent)
					require.True(t, ok)
					require.Len(t, events, 1)
					require.Equal(t, "failover", events[0].Kind)
					require.Equal(t, a.ID, events[0].AccountID)
					require.Equal(t, "req_failed_ws", events[0].UpstreamRequestID)
					require.Equal(t, http.StatusServiceUnavailable, events[0].UpstreamStatusCode)
				})
			}
		}
	}
}

func TestForwardOpenAIWSV2_ActualOutputStopsAccountReplay(t *testing.T) {
	for _, content := range []string{
		`{"type":"response.output_text.delta","delta":"partial"}`,
		`{"type":"response.reasoning_summary_text.delta","delta":"thinking"}`,
		`{"type":"response.output_item.added","item":{"type":"function_call","name":"lookup","call_id":"call_1","arguments":"{}"}}`,
	} {
		t.Run(content, func(t *testing.T) {
			rec := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(rec)
			c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
			SetOpenAIClientTransport(c, OpenAIClientTransportHTTP)
			conn := &openAIWSCaptureConn{events: [][]byte{
				[]byte(`{"type":"response.created","response":{"id":"resp_partial"}}`),
				[]byte(content),
				[]byte(`{"type":"error","error":{"code":"server_is_overloaded","type":"service_unavailable_error","message":"Our servers are currently overloaded. Please try again later."}}`),
			}}
			h := &httpUpstreamRecorder{}
			s := wsSSETestService(t, &wsSSETestDialer{conn: conn}, h)
			_, err := s.Forward(context.Background(), c, wsSSETestAccount(), []byte(wsSSETestRequest))
			require.Error(t, err)
			var failover *UpstreamFailoverError
			require.False(t, errors.As(err, &failover))
			require.Contains(t, rec.Body.String(), content)
			require.Contains(t, rec.Body.String(), "Our servers are currently overloaded.")
			require.NotContains(t, rec.Body.String(), "Upstream request failed")
			require.Equal(t, 1, strings.Count(rec.Body.String(), "event: response.failed"))
			require.True(t, IsResponseCommitted(c), "handler must not append a generic 502 after the upstream error")
			events := requireOpsType[[]*OpsUpstreamErrorEvent](t, c.MustGet(OpsUpstreamErrorsKey))
			require.Len(t, events, 1)
			require.Equal(t, "stream_failed", events[0].Kind)
			require.Equal(t, "downstream_response_committed", events[0].FailoverSuppressedReason)
			require.NotEmpty(t, events[0].FirstOutputEventType)
			require.Empty(t, h.requests)
		})
	}
}

func TestOpenAIWSSSEOverloadFallsThroughOAuthPoolToAPIKey(t *testing.T) {
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
	SetOpenAIClientTransport(c, OpenAIClientTransportHTTP)
	var accounts []Account
	dialer := &openAIWSQueueDialer{}
	for priority, id := range []int64{1440, 1439, 1431} {
		a := wsSSETestAccount()
		a.ID, a.Priority = id, priority
		accounts = append(accounts, *a)
		dialer.conns = append(dialer.conns, &openAIWSCaptureConn{events: [][]byte{
			[]byte(`{"type":"response.created","response":{"id":"resp_failed_attempt"}}`),
			[]byte(`{"type":"response.metadata","metadata":{"trace":"failed-metadata"}}`),
			[]byte(`{"type":"error","error":{"code":"server_is_overloaded","type":"service_unavailable_error","message":"Our servers are currently overloaded. Please try again later."}}`),
		}})
	}
	accounts = append(accounts, Account{ID: 1418, Priority: 100, Platform: PlatformOpenAI,
		Type: AccountTypeAPIKey, Status: StatusActive, Schedulable: true, Concurrency: 1,
		Credentials: map[string]any{"api_key": "test-key", "base_url": "https://api.example.test/v1"}})
	h := wsSSEGuardHTTPUpstream()
	s := wsSSETestService(t, &wsSSETestDialer{}, h)
	s.accountRepo = stubOpenAIAccountRepo{accounts: accounts}
	s.openaiWSPool.setClientDialerForTest(dialer)
	s.rateLimitService = newOpenAIAdvancedSchedulerRateLimitService("false")
	excluded := map[int64]struct{}{}
	var attempts []int64
	for range accounts {
		selection, _, err := s.SelectAccountWithScheduler(context.Background(), nil, "", "", "gpt-6.1-sol", excluded, OpenAIUpstreamTransportAny, false)
		require.NoError(t, err)
		require.NotNil(t, selection)
		a := selection.Account
		attempts = append(attempts, a.ID)
		_, err = s.Forward(context.Background(), c, a, []byte(`{"model":"gpt-6.1-sol","stream":true,"input":"hello"}`))
		if selection.ReleaseFunc != nil {
			selection.ReleaseFunc()
		}
		if a.ID == 1418 {
			require.NoError(t, err)
			break
		}
		var failover *UpstreamFailoverError
		require.ErrorAs(t, err, &failover)
		require.True(t, failover.ShouldRetryNextAccount())
		require.False(t, c.Writer.Written())
		excluded[a.ID] = struct{}{}
	}
	require.Equal(t, []int64{1440, 1439, 1431, 1418}, attempts)
	require.Equal(t, 3, dialer.DialCount())
	require.Len(t, h.requests, 1)
	require.Contains(t, rec.Body.String(), "resp_http")
	require.NotContains(t, rec.Body.String(), "failed-metadata")
	require.NotContains(t, rec.Body.String(), "resp_failed_attempt")
	require.NotContains(t, rec.Body.String(), "overloaded")
	events := requireOpsType[[]*OpsUpstreamErrorEvent](t, c.MustGet(OpsUpstreamErrorsKey))
	require.Len(t, events, 3)
}
