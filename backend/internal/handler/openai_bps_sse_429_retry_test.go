//go:build unit

package handler

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func TestBPSSSE429RetriesThenSwitchesToNative(t *testing.T) {
	for _, chat := range []bool{false, true} {
		for _, stream := range []bool{false, true} {
			for _, generated := range []bool{false, true} {
				t.Run(fmt.Sprintf("chat=%t/stream=%t/generated=%t", chat, stream, generated), func(t *testing.T) {
					accounts := []service.Account{}
					for _, id := range []int64{801, 802} {
						accounts = append(accounts, service.Account{ID: id, Platform: service.PlatformOpenAI, Type: service.AccountTypeOAuth, Status: service.StatusActive, Schedulable: true, Concurrency: 1, Priority: int(id), Credentials: map[string]any{"access_token": "synthetic-token", "chatgpt_account_id": fmt.Sprintf("synthetic-%d", id)}, Extra: map[string]any{"openai_excel_bps": id == 801}})
					}
					upstream := &excelBPSFailoverUpstream{answer: func(call int) *http.Response {
						if call >= 2 {
							success := excelBPSCompleted()
							success.Body = io.NopCloser(io.MultiReader(strings.NewReader("data: {\"type\":\"response.output_text.delta\",\"delta\":\"done\"}\n\n"), success.Body))
							return success
						}
						wire := "data: {\"type\":\"response.created\"}\n\n"
						if generated {
							wire += "data: {\"type\":\"response.output_text.delta\",\"delta\":\"already generated\"}\n\n"
						}
						wire += "event: error\ndata: {\"type\":\"error\",\"error\":{\"code\":\"rate_limit_exceeded\",\"headers\":{\"retry-after\":\"1\"}}}\n\n"
						return &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": {"text/event-stream"}}, Body: io.NopCloser(strings.NewReader(wire))}
					}}
					router := newBPSLocalCapabilityRouter(t, upstream, accounts)
					path := "/v1/responses"
					body := fmt.Sprintf(`{"model":"gpt-6-astra","input":"hello","stream":%t}`, stream)
					if chat {
						path = "/v1/chat/completions"
						body = fmt.Sprintf(`{"model":"gpt-6-astra","messages":[{"role":"user","content":"hello"}],"stream":%t}`, stream)
					}
					rec := httptest.NewRecorder()
					router.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, path, strings.NewReader(body)))
					if generated {
						require.Equal(t, []int64{801}, upstream.calls(), rec.Body.String())
						require.Contains(t, rec.Body.String(), "rate_limit_exceeded")
						return
					}
					require.Equal(t, []int64{801, 801, 802}, upstream.calls(), rec.Body.String())
					require.Contains(t, upstream.urls[0], "/basispoints/")
					require.NotContains(t, upstream.urls[2], "/basispoints/")
					require.Equal(t, http.StatusOK, rec.Code)
					require.Contains(t, rec.Body.String(), "done")
					require.NotContains(t, rec.Body.String(), "rate_limit_exceeded")
				})
			}
		}
	}
}
