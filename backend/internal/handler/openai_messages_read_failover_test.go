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

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

type messagesReadFailureBody struct{}

func (*messagesReadFailureBody) Read([]byte) (int, error) { return 0, io.ErrUnexpectedEOF }

type messagesReadFailureUpstream struct {
	service.HTTPUpstream
	hits    []int64
	allFail bool
	cancel  context.CancelFunc
}

func (u *messagesReadFailureUpstream) Do(req *http.Request, proxy string, id int64, concurrency int) (*http.Response, error) {
	u.hits = append(u.hits, id)
	if u.cancel != nil {
		u.cancel()
	}
	if id == 801 || u.allFail {
		prefix := "data: {\"type\":\"response.created\",\"response\":{\"id\":\"failed-attempt\",\"output\":[]}}\n\n"
		return &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": {"text/event-stream"}}, Body: io.NopCloser(io.MultiReader(strings.NewReader(prefix), &messagesReadFailureBody{}))}, nil
	}
	return (&openAI502503Upstream{}).Do(req, proxy, id, concurrency)
}

func TestOpenAIMessagesReadFailureSwitchesAccount(t *testing.T) {
	for _, stream := range []bool{false, true} {
		for _, mode := range []string{"healthy alternate", "all fail", "canceled"} {
			t.Run(fmt.Sprintf("stream=%t/%s", stream, mode), func(t *testing.T) {
				ctx, cancel := context.WithCancel(context.Background())
				defer cancel()
				upstream := &messagesReadFailureUpstream{allFail: mode == "all fail"}
				if mode == "canceled" {
					upstream.cancel = cancel
				}
				router := newOpenAI502503Router(t, upstream)
				body := fmt.Sprintf(`{"model":"gpt-6-luna","messages":[{"role":"user","content":"hello"}],"max_tokens":32,"stream":%t}`, stream)
				req := httptest.NewRequest(http.MethodPost, "/v1/messages", strings.NewReader(body)).WithContext(ctx)
				req.Header.Set("Content-Type", "application/json")
				rec := httptest.NewRecorder()
				router.ServeHTTP(rec, req)
				if mode == "canceled" {
					require.Equal(t, []int64{801}, upstream.hits)
					return
				}
				require.Equal(t, []int64{801, 802}, upstream.hits)
				require.NotContains(t, rec.Body.String(), "failed-attempt")
				if mode == "all fail" {
					require.Equal(t, http.StatusBadGateway, rec.Code)
				} else {
					require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
					require.Contains(t, rec.Body.String(), "ok")
					require.NotContains(t, rec.Body.String(), "Upstream request failed")
				}
			})
		}
	}
}
