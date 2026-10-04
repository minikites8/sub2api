package service

import (
	"bufio"
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestPrismBrowserKeepaliveArrivesBeforeAdapterSettles(t *testing.T) {
	for _, status := range []int{http.StatusOK, http.StatusTooManyRequests} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			var calls atomic.Int32
			release := make(chan struct{})
			var once sync.Once
			done := func() { once.Do(func() { close(release) }) }
			adapter := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				select {
				case <-release:
				case <-r.Context().Done():
					return
				}
				w.WriteHeader(status)
				if status == http.StatusOK {
					_, _ = io.WriteString(w, "event: response.completed\ndata: {\"type\":\"response.completed\",\"sequence_number\":0,\"response\":{\"id\":\"resp_fixture\",\"status\":\"completed\",\"model\":\"gpt-6.1-sol\",\"output\":[{\"content\":[{\"text\":\"fixture answer\"}]}]}}\n\n")
				} else {
					_, _ = io.WriteString(w, `{"error":{"type":"prism_busy","message":"Prism queue wait expired; request was not submitted"}}`)
				}
			}))
			defer adapter.Close()
			defer done()
			svc, account := prismTestService(adapter.URL)
			svc.cfg.Gateway.StreamKeepaliveInterval = 1
			router := gin.New()
			router.POST("/v1/responses", func(c *gin.Context) {
				body, _ := io.ReadAll(c.Request.Body)
				_, _ = svc.forwardPrismBrowser(c.Request.Context(), c, account, body, time.Now())
			})
			gateway := httptest.NewServer(router)
			defer gateway.Close()
			client := &http.Client{Timeout: 4 * time.Second}
			response, err := client.Post(gateway.URL+"/v1/responses", "application/json",
				strings.NewReader(`{"model":"gpt-6.1-sol","input":"fixture","stream":true}`))
			require.NoError(t, err)
			defer response.Body.Close()
			require.Equal(t, http.StatusOK, response.StatusCode)
			require.Equal(t, "no", response.Header.Get("X-Accel-Buffering"))
			require.Equal(t, "response-metadata", response.Header.Get("X-Prism-Usage"))
			reader := bufio.NewReader(response.Body)
			first, err := reader.ReadString('\n')
			require.NoError(t, err)
			require.Equal(t, ": keepalive\n", first)
			require.EqualValues(t, 1, calls.Load())
			done()
			body, err := io.ReadAll(reader)
			require.NoError(t, err)
			terminal := "response.completed"
			if status != http.StatusOK {
				terminal = "response.failed"
				require.Contains(t, string(body), "prism_busy")
			}
			require.Equal(t, 1, strings.Count(string(body), "event: "+terminal))
			require.NotContains(t, string(body), "fixture-oauth")
			require.EqualValues(t, 1, calls.Load(), "a heartbeat must preserve the single adapter submission")
		})
	}
}

func TestPrismBrowserCanceledCallerKeepsWriterUntouched(t *testing.T) {
	svc, account := prismTestService("http://127.0.0.1:1")
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil).WithContext(ctx)
	_, err := svc.forwardPrismBrowser(ctx, c, account,
		[]byte(`{"model":"gpt-6.1-sol","input":"fixture","stream":true}`), time.Now())
	require.True(t, errors.Is(err, context.Canceled))
	require.Empty(t, recorder.Body.String())
	require.False(t, c.Writer.Written())
}
