//go:build unit

package handler

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

const overloadHandlerFixture = "Our servers are currently overloaded. Please try again later."

type overloadUpstream struct {
	service.HTTPUpstream
	status                          int
	sse, allFail, partial, topLevel bool
	cancel                          context.CancelFunc
	hits                            []int64
}

func (u *overloadUpstream) Do(req *http.Request, _ string, id int64, _ int) (*http.Response, error) {
	u.hits = append(u.hits, id)
	if u.cancel != nil {
		u.cancel()
	}
	if id == 801 || u.allFail {
		errorBody := fmt.Sprintf(`{"type":"error","error":{"type":"invalid_request_error","message":%q}}`, overloadHandlerFixture)
		if u.topLevel {
			errorBody = fmt.Sprintf(`{"type":"error","message":%q}`, overloadHandlerFixture)
		}
		status, contentType := u.status, "application/json"
		if u.sse {
			prefix := "data: {\"type\":\"response.created\",\"response\":{\"id\":\"resp_failed_attempt\"}}\n\n"
			if u.partial {
				prefix += "data: {\"type\":\"response.output_text.delta\",\"delta\":\"partial\"}\n\n"
			}
			errorBody = prefix + "data: " + errorBody + "\n\n"
			status = 200
			contentType = "text/event-stream"
		}
		return &http.Response{StatusCode: status, Header: http.Header{"Content-Type": {contentType}}, Body: io.NopCloser(strings.NewReader(errorBody))}, nil
	}
	healthy := `{"type":"response.completed","response":{"id":"resp_healthy","object":"response","model":"gpt-5.2","status":"completed","output":[{"id":"msg_healthy","type":"message","role":"assistant","status":"completed","content":[{"type":"output_text","text":"ok","annotations":[]}]}],"usage":{"input_tokens":1,"output_tokens":1}}}`
	return &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": {"text/event-stream"}}, Body: io.NopCloser(strings.NewReader("data: " + healthy + "\n\n"))}, nil
}

func TestOpenAIOverloadHandlerAutoSwitch(t *testing.T) {
	for _, endpoint := range []struct{ path, body string }{
		{"/v1/responses", `{"model":"gpt-5.2","input":"hello","stream":false}`},
		{"/v1/responses", `{"model":"gpt-5.2","input":"hello","stream":true}`},
		{"/v1/chat/completions", `{"model":"gpt-5.2","messages":[{"role":"user","content":"hello"}],"stream":true}`},
		{"/v1/messages", `{"model":"gpt-5.2","messages":[{"role":"user","content":"hello"}],"max_tokens":32,"stream":true}`},
	} {
		for _, format := range []string{"http_error", "http_message", "sse_error", "sse_message"} {
			sse := strings.HasPrefix(format, "sse_")
			t.Run(fmt.Sprintf("%s/%s/%s", endpoint.path, endpoint.body, format), func(t *testing.T) {
				u := &overloadUpstream{status: 400, sse: sse, topLevel: strings.HasSuffix(format, "_message")}
				r := newOpenAI502503Router(t, u)
				rec := httptest.NewRecorder()
				req := httptest.NewRequest(http.MethodPost, endpoint.path, bytes.NewBufferString(endpoint.body))
				req.Header.Set("Content-Type", "application/json")
				r.ServeHTTP(rec, req)
				require.Equal(t, 200, rec.Code, rec.Body.String())
				require.Equal(t, int64(801), u.hits[0])
				require.Equal(t, int64(802), u.hits[len(u.hits)-1])
				require.Equal(t, []int64{801, 802}, u.hits)
				require.NotContains(t, rec.Body.String(), overloadHandlerFixture)
				require.NotContains(t, rec.Body.String(), "resp_failed_attempt")
				t.Logf("overload route=%s sse=%t scheduled_accounts=%v final_status=%d", endpoint.path, sse, u.hits, rec.Code)
			})
		}
	}
}

func TestOpenAIOverloadRetryBoundaries(t *testing.T) {
	for _, mode := range []string{"exhausted", "cancelled", "partial"} {
		t.Run(mode, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			u := &overloadUpstream{status: 400, allFail: mode == "exhausted", partial: mode == "partial", sse: mode == "partial"}
			if mode == "cancelled" {
				u.cancel = cancel
			}
			r := newOpenAI502503Router(t, u)
			rec := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodPost, "/v1/responses", strings.NewReader(`{"model":"gpt-5.2","input":"hello","stream":true}`)).WithContext(ctx)
			req.Header.Set("Content-Type", "application/json")
			r.ServeHTTP(rec, req)
			switch mode {
			case "exhausted":
				require.Equal(t, []int64{801, 802}, u.hits)
				require.Equal(t, 503, rec.Code)
				require.Contains(t, rec.Body.String(), overloadHandlerFixture)
				require.Contains(t, rec.Body.String(), "error")
			case "cancelled":
				require.Equal(t, []int64{801}, u.hits)
			case "partial":
				require.Equal(t, []int64{801}, u.hits)
				require.Contains(t, rec.Body.String(), "partial")
			}
			t.Logf("overload boundary=%s scheduled_accounts=%v final_status=%d", mode, u.hits, rec.Code)
		})
	}
}

type loggedOverloadUpstream struct {
	service.HTTPUpstream
	hits []int64
}

func (u *loggedOverloadUpstream) Do(req *http.Request, proxy string, id int64, concurrency int) (*http.Response, error) {
	u.hits = append(u.hits, id)
	if id == 1350 {
		body := "data: {\"type\":\"response.created\",\"response\":{\"id\":\"resp_logged\"}}\n\n" +
			"data: " + `{"error":{"code":"server_is_overloaded","message":"Our servers are currently overloaded. Please try again later.","param":null,"type":"service_unavailable_error"},"sequence_number":2,"type":"error"}` + "\n\n" +
			"data: " + `{"type":"response.failed","sequence_number":4,"response":{"id":"resp_051d834ce1882b2b016ac9d4eeb39c87d0824602d6dd951db3","model":"gpt-6.1-sol","status":"failed","error":{"code":"server_is_overloaded","type":"service_unavailable_error","message":"Our servers are currently overloaded. Please try again later."}}}` + "\n\n"
		return &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": {"text/event-stream"}}, Body: io.NopCloser(strings.NewReader(body))}, nil
	}
	return (&overloadUpstream{}).Do(req, proxy, id, concurrency)
}
func TestOpenAIOverloadLoggedSequenceImmediateSwitch(t *testing.T) {
	for _, stream := range []bool{false, true} {
		for _, single := range []bool{false, true} {
			t.Run(fmt.Sprintf("stream_%t/single_%t", stream, single), func(t *testing.T) {
				u := &loggedOverloadUpstream{}
				ids := []int64{1350, 1351}
				if single {
					ids = ids[:1]
				}
				r := newOpenAI502503Router(t, u, ids...)
				rec := httptest.NewRecorder()
				req := httptest.NewRequest(http.MethodPost, "/v1/responses", strings.NewReader(fmt.Sprintf(`{"model":"gpt-6.1-sol","input":"hello","stream":%t}`, stream)))
				req.Header.Set("Content-Type", "application/json")
				r.ServeHTTP(rec, req)
				if single {
					require.Equal(t, []int64{1350}, u.hits)
					require.Equal(t, 503, rec.Code)
				} else {
					require.Equal(t, []int64{1350, 1351}, u.hits)
					require.Equal(t, 200, rec.Code)
				}
				t.Logf("logged overload account=1350 model=gpt-6.1-sol scheduled_accounts=%v final_status=%d", u.hits, rec.Code)
			})
		}
	}
}
