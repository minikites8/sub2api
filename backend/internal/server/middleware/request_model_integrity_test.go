package middleware

import (
	"bytes"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

func integrityTestRouter() (*gin.Engine, *bool, *string) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	called, forwarded := false, ""
	r.Use(RequestModelIntegrity(1024 * 1024))
	for _, method := range []string{http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodGet} {
		r.Handle(method, "/v1/responses", func(c *gin.Context) {
			called = true
			body, _ := io.ReadAll(c.Request.Body)
			forwarded = string(body)
			c.Status(http.StatusOK)
		})
	}
	return r, &called, &forwarded
}

func TestRequestModelIntegrityRejectsBeforeForwarding(t *testing.T) {
	for _, method := range []string{http.MethodPost, http.MethodPut, http.MethodPatch} {
		for _, body := range []string{
			`{"model":"cheap","model":"expensive"}`,
			`{"model":"cheap","Model":"expensive"}`,
			`{"model":"cheap","\u006dodel":"expensive"}`,
			`{"model":"cheap","model":"cheap"}`,
			`{"session":{"model":"cheap"},"session":{"model":"expensive"}}`,
			"\xef\xbb\xbf{\"model\":\"cheap\",\"model\":\"expensive\"}",
			"{\"model\":\"cheap\",\"model\":\"expensive\",\"input\":\"raw\ncontrol\"}",
		} {
			t.Run(method+body, func(t *testing.T) {
				r, called, _ := integrityTestRouter()
				w := doJSON(t, r, method, "/v1/responses", body)
				if w.Code != http.StatusBadRequest || *called {
					t.Fatalf("status=%d, forwarded=%v, body=%s", w.Code, *called, w.Body.String())
				}
			})
		}
	}
}

func TestRequestModelIntegrityPreservesValidBodies(t *testing.T) {
	for _, body := range []string{
		`{"model":"cheap","input":[{"model":"application-data"}]}`,
		`{"session":{"model":"live"},"sdp":"v=0"}`,
		`{"input":"no model"}`,
		"{\"model\":\"cheap\",\"input\":\"raw\ncontrol\"}",
	} {
		r, called, forwarded := integrityTestRouter()
		w := doJSON(t, r, http.MethodPost, "/v1/responses", body)
		if w.Code != http.StatusOK || !*called || *forwarded != body {
			t.Fatalf("status=%d, forwarded=%q, original=%q", w.Code, *forwarded, body)
		}
	}
}

func TestRequestModelIntegrityMultipartAndBodyLimit(t *testing.T) {
	var body bytes.Buffer
	mw := multipart.NewWriter(&body)
	_ = mw.WriteField("model", "cheap")
	_ = mw.WriteField("model", "expensive")
	_ = mw.Close()
	r, called, _ := integrityTestRouter()
	req := httptest.NewRequest(http.MethodPost, "/v1/responses", &body)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest || *called {
		t.Fatalf("multipart: status=%d, called=%v", w.Code, *called)
	}

	r, called, _ = integrityTestRouter()
	req = httptest.NewRequest(http.MethodPost, "/v1/responses", strings.NewReader(`{"model":"cheap"}`))
	req.Body = http.MaxBytesReader(httptest.NewRecorder(), req.Body, 4)
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusRequestEntityTooLarge || *called {
		t.Fatalf("body limit: status=%d, called=%v", w.Code, *called)
	}
}

func TestRequestModelIntegrityLeavesWebSocketUpgradeBodyUnread(t *testing.T) {
	r, _, _ := integrityTestRouter()
	body := &readTrackingBody{Reader: strings.NewReader("payload")}
	// A terminal handler stops the chain before the test reader runs.
	r.GET("/upgrade", func(c *gin.Context) { c.Status(http.StatusOK) })
	req := httptest.NewRequest(http.MethodGet, "/upgrade", body)
	req.Header.Set("Upgrade", "websocket")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK || body.read {
		t.Fatalf("upgrade status=%d, body read=%v", w.Code, body.read)
	}
}
