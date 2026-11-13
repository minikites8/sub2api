package service

import (
	"bytes"
	"github.com/gin-gonic/gin"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
)

func admissionSSEResponse(body string) *http.Response {
	return &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": {"text/event-stream"}},
		Body:       io.NopCloser(strings.NewReader(body)),
	}
}

func admissionOrdinaryResponse() *http.Response {
	return admissionOrdinaryStreamResponse()
}

func admissionOrdinaryStreamResponse() *http.Response {
	return admissionSSEResponse("event: response.created\ndata: {\"type\":\"response.created\",\"response\":{\"id\":\"ordinary-stream\",\"model\":\"gpt-5.6-sol\"}}\n\nevent: response.output_text.delta\ndata: {\"type\":\"response.output_text.delta\",\"delta\":\"ordinary output\"}\n\nevent: response.completed\ndata: {\"type\":\"response.completed\",\"response\":{\"id\":\"ordinary-stream\",\"status\":\"completed\",\"model\":\"gpt-5.6-sol\",\"output\":[],\"usage\":{\"input_tokens\":2,\"output_tokens\":1}}}\n\n")
}

func newAdmissionTestContext(body []byte, stream bool) (*gin.Context, *httptest.ResponseRecorder) {
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", bytes.NewReader(body))
	if stream {
		c.Request.Header.Set("Accept", "text/event-stream")
	}
	return c, rec
}
