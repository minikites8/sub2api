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

const processingHandlerFixture = "An error occurred while processing your request. You can retry your request, or contact us through our help center at help.openai.com if the error persists. Please include the request ID 9d49f990-38d5-4780-b391-3a128743ace9 in your message."

type processingFailureUpstream struct {
	service.HTTPUpstream
	status                int
	sse, allFail, partial bool
	cancel                context.CancelFunc
	hits                  []int64
}

func (u *processingFailureUpstream) Do(req *http.Request, _ string, id int64, _ int) (*http.Response, error) {
	u.hits = append(u.hits, id)
	if u.cancel != nil {
		u.cancel()
	}
	if id == 801 || u.allFail {
		errorBody := fmt.Sprintf(`{"type":"error","error":{"type":"invalid_request_error","message":%q}}`, processingHandlerFixture)
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

func TestOpenAIProcessingFailureHandlerAutoSwitch(t *testing.T) {
	for _, endpoint := range []struct{ path, body string }{
		{"/v1/responses", `{"model":"gpt-5.2","input":"hello","stream":false}`},
		{"/v1/responses", `{"model":"gpt-5.2","input":"hello","stream":true}`},
		{"/v1/chat/completions", `{"model":"gpt-5.2","messages":[{"role":"user","content":"hello"}],"stream":true}`},
		{"/v1/messages", `{"model":"gpt-5.2","messages":[{"role":"user","content":"hello"}],"max_tokens":32,"stream":true}`},
	} {
		for _, sse := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/%s/sse_%t", endpoint.path, endpoint.body, sse), func(t *testing.T) {
				u := &processingFailureUpstream{status: 400, sse: sse}
				r := newOpenAI502503Router(t, u)
				rec := httptest.NewRecorder()
				req := httptest.NewRequest(http.MethodPost, endpoint.path, bytes.NewBufferString(endpoint.body))
				req.Header.Set("Content-Type", "application/json")
				r.ServeHTTP(rec, req)
				require.Equal(t, 200, rec.Code, rec.Body.String())
				require.Equal(t, int64(801), u.hits[0])
				require.Equal(t, int64(802), u.hits[len(u.hits)-1])
				require.LessOrEqual(t, len(u.hits), 4)
				require.NotContains(t, rec.Body.String(), processingHandlerFixture)
				require.NotContains(t, rec.Body.String(), "resp_failed_attempt")
				t.Logf("processing route=%s sse=%t scheduled_accounts=%v final_status=%d", endpoint.path, sse, u.hits, rec.Code)
			})
		}
	}
}

func TestOpenAIProcessingFailureRetryBoundaries(t *testing.T) {
	for _, mode := range []string{"exhausted", "cancelled", "partial"} {
		t.Run(mode, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			u := &processingFailureUpstream{status: 400, allFail: mode == "exhausted", partial: mode == "partial", sse: mode == "partial"}
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
				require.Contains(t, rec.Body.String(), "error")
			case "cancelled":
				require.Equal(t, []int64{801}, u.hits)
			case "partial":
				require.Equal(t, []int64{801}, u.hits)
				require.Contains(t, rec.Body.String(), "partial")
			}
			t.Logf("processing boundary=%s scheduled_accounts=%v final_status=%d", mode, u.hits, rec.Code)
		})
	}
}
