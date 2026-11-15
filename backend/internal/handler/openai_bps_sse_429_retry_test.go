//go:build unit

package handler

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
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

type bpsHeartbeatRecorder struct {
	*httptest.ResponseRecorder
	flushed chan struct{}
	once    sync.Once
}

func (r *bpsHeartbeatRecorder) Flush() {
	r.ResponseRecorder.Flush()
	r.once.Do(func() { close(r.flushed) })
}

func TestBPSSSE429AfterHeartbeatStillSwitchesAccount(t *testing.T) {
	// Wait for the real BPS ticker to flush its heartbeat before releasing the
	// rejection. The response contains no generated content or metered work.
	rec := &bpsHeartbeatRecorder{ResponseRecorder: httptest.NewRecorder(), flushed: make(chan struct{})}
	reader, writer := io.Pipe()
	t.Cleanup(func() { _ = reader.Close(); _ = writer.Close() })
	ctx, cancel := context.WithTimeout(context.Background(), 25*time.Second)
	defer cancel()
	done := make(chan struct{})
	go func() {
		defer close(done)
		defer writer.Close()
		select {
		case <-rec.flushed:
			_, _ = io.WriteString(writer, "event: error\ndata: {\"type\":\"error\",\"error\":{\"code\":\"rate_limit_exceeded\",\"headers\":{\"retry-after\":\"1\"}}}\n\n")
		case <-ctx.Done():
		}
	}()
	upstream := &excelBPSFailoverUpstream{answer: func(call int) *http.Response {
		if call == 0 {
			return &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": {"text/event-stream"}}, Body: reader}
		}
		if call == 1 {
			return excelBPS429("1")
		}
		return excelBPSCompleted()
	}}
	handler := newExcelBPSFailoverTestHandler(t, upstream, false)
	c, _ := newExcelBPSFailoverTestContext(ctx, true)
	ginContext, _ := gin.CreateTestContext(rec)
	c.Writer = ginContext.Writer
	handler.Responses(c)
	cancel()
	<-done
	calls := upstream.calls()
	require.Len(t, calls, 3, rec.Body.String())
	require.Equal(t, calls[0], calls[1], "a heartbeat must not prevent the existing same-account retry")
	require.NotEqual(t, calls[0], calls[2], "a repeated rejection must switch accounts")
	require.Equal(t, http.StatusOK, rec.Code)
	require.Contains(t, rec.Body.String(), ": keepalive\n\n")
	require.Contains(t, rec.Body.String(), "done")
	require.NotContains(t, rec.Body.String(), "rate_limit_exceeded")
}
