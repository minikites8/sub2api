package handler

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestExcelBPSAllFailuresFallBackToOrdinaryUpstream(t *testing.T) {
	for _, endpoint := range []struct {
		name string
		path string
		body func(stream bool) string
	}{
		{name: "responses", path: "/v1/responses", body: func(stream bool) string {
			return fmt.Sprintf(`{"model":"gpt-6-sol","input":"hello","stream":%t}`, stream)
		}},
		{name: "chat_completions", path: "/v1/chat/completions", body: func(stream bool) string {
			return fmt.Sprintf(`{"model":"gpt-6-sol","messages":[{"role":"user","content":"hello"}],"stream":%t}`, stream)
		}},
	} {
		for _, stream := range []bool{false, true} {
			for _, failure := range []struct {
				name       string
				status     int
				transport  bool
				sse        bool
				streamBody string
			}{
				{name: "http_401", status: http.StatusUnauthorized},
				{name: "http_429", status: http.StatusTooManyRequests},
				{name: "http_503", status: http.StatusServiceUnavailable},
				{name: "http_502", status: http.StatusBadGateway},
				{name: "stream_eof", streamBody: "data: {\"type\":\"response.created\",\"response\":{\"id\":\"resp_failed\",\"output\":[]}}\n\n"},
				{name: "response_failed", streamBody: "data: {\"type\":\"response.failed\",\"response\":{\"id\":\"resp_failed\",\"status\":\"failed\",\"error\":{\"code\":\"server_error\",\"message\":\"BPS failed\"},\"output\":[]}}\n\n"},
				{name: "response_incomplete", streamBody: "data: {\"type\":\"response.incomplete\",\"response\":{\"id\":\"resp_failed\",\"status\":\"incomplete\",\"incomplete_details\":{\"reason\":\"max_output_tokens\"},\"output\":[]}}\n\n"},
				{name: "http_403", status: http.StatusForbidden},
				{name: "transport", transport: true},
				{name: "stream_error", status: http.StatusBadGateway, sse: true},
			} {
				t.Run(fmt.Sprintf("%s/stream=%t/%s", endpoint.name, stream, failure.name), func(t *testing.T) {
					upstream := &excelBPSRateLimitUpstream{
						allFail: true, failureStatus: failure.status,
						transportFailure: failure.transport, sse: failure.sse, streamBody: failure.streamBody,
					}
					router := newExcelBPSRateLimitRouter(t, upstream)
					rec := httptest.NewRecorder()
					req := httptest.NewRequest(http.MethodPost, endpoint.path, strings.NewReader(endpoint.body(stream)))
					req.Header.Set("Content-Type", "application/json")
					router.ServeHTTP(rec, req)

					require.Equal(t, []int64{801, 802, 801}, upstream.hits, "status=%d body=%s", rec.Code, rec.Body.String())
					require.Equal(t, []string{"/basispoints/api/responses", "/basispoints/api/responses", "/backend-api/codex/responses"}, upstream.paths)
					require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
					require.Contains(t, rec.Body.String(), "ok")
					require.NotContains(t, rec.Body.String(), "upstream_failure")
					require.NotContains(t, rec.Body.String(), "BPS connection failed")
					if endpoint.name == "chat_completions" {
						require.Contains(t, rec.Body.String(), "chat.completion")
					}
				})
			}
		}
	}
}

func TestExcelBPSOrdinaryFallbackExhaustionTerminates(t *testing.T) {
	for _, endpoint := range []struct{ path, body string }{
		{"/v1/responses", `{"model":"gpt-6-sol","input":"hello","stream":false}`},
		{"/v1/chat/completions", `{"model":"gpt-6-sol","messages":[{"role":"user","content":"hello"}],"stream":false}`},
	} {
		for _, status := range []int{http.StatusBadRequest, http.StatusBadGateway} {
			t.Run(fmt.Sprintf("%s/status=%d", endpoint.path, status), func(t *testing.T) {
				upstream := &excelBPSRateLimitUpstream{allFail: true, failureStatus: http.StatusBadGateway, ordinaryStatus: status}
				router := newExcelBPSRateLimitRouter(t, upstream)
				rec := httptest.NewRecorder()
				req := httptest.NewRequest(http.MethodPost, endpoint.path, strings.NewReader(endpoint.body))
				req.Header.Set("Content-Type", "application/json")
				router.ServeHTTP(rec, req)
				require.GreaterOrEqual(t, rec.Code, http.StatusBadRequest, rec.Body.String())
				require.NotEmpty(t, rec.Body.String())
				require.GreaterOrEqual(t, len(upstream.paths), 3)
				expectedHits := []int64{801, 802, 801}
				if status == http.StatusBadGateway {
					// The ordinary 502 policy retries each account twice before switching.
					expectedHits = []int64{801, 802, 801, 801, 801, 802, 802, 802}
				}
				require.Equal(t, expectedHits, upstream.hits, "ordinary fallback must terminate within its account budget")
				require.Equal(t, []string{"/basispoints/api/responses", "/basispoints/api/responses"}, upstream.paths[:2])
				for _, path := range upstream.paths[2:] {
					require.Equal(t, "/backend-api/codex/responses", path)
				}
			})
		}
	}
}
