package service

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/service/basispoints"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func excelBPSJSONResponse(status int, body string) *http.Response {
	return &http.Response{
		StatusCode: status,
		Header:     http.Header{"Content-Type": {"application/json"}},
		Body:       io.NopCloser(strings.NewReader(body)),
	}
}

func excelBPSSSEResponse(body string) *http.Response {
	return &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": {"text/event-stream"}},
		Body:       io.NopCloser(strings.NewReader(body)),
	}
}

func excelBPSOrdinaryResponse() *http.Response {
	return excelBPSOrdinaryStreamResponse()
}

func excelBPSOrdinaryStreamResponse() *http.Response {
	return excelBPSSSEResponse("event: response.created\ndata: {\"type\":\"response.created\",\"response\":{\"id\":\"ordinary-stream\",\"model\":\"gpt-5.6-sol\"}}\n\nevent: response.output_text.delta\ndata: {\"type\":\"response.output_text.delta\",\"delta\":\"ordinary output\"}\n\nevent: response.completed\ndata: {\"type\":\"response.completed\",\"response\":{\"id\":\"ordinary-stream\",\"status\":\"completed\",\"model\":\"gpt-5.6-sol\",\"output\":[],\"usage\":{\"input_tokens\":2,\"output_tokens\":1}}}\n\n")
}

func newExcelBPSFallbackContext(body []byte, stream bool) (*gin.Context, *httptest.ResponseRecorder) {
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", bytes.NewReader(body))
	if stream {
		c.Request.Header.Set("Accept", "text/event-stream")
	}
	return c, rec
}

func TestExcelBPSModelUnavailableClassification(t *testing.T) {
	tests := []struct {
		name    string
		status  int
		payload string
		match   bool
	}{
		{name: "basispoints access code", status: http.StatusForbidden, payload: `{"error":{"code":"basispoints_model_access_changed"}}`, match: true},
		{name: "model not found code", status: http.StatusNotFound, payload: `{"error":{"code":"model_not_found"}}`, match: true},
		{name: "unsupported model message", status: http.StatusBadRequest, payload: `{"error":{"type":"invalid_request_error","param":"model","message":"The model gpt-5.6-sol does not exist"}}`, match: true},
		{name: "nested response error", status: http.StatusOK, payload: `{"type":"response.failed","response":{"error":{"code":"unsupported_model"}}}`, match: true},
		{name: "input error", status: http.StatusBadRequest, payload: `{"error":{"code":"invalid_value","param":"input","message":"model is not supported in this field"}}`, match: false},
		{name: "rate limit", status: http.StatusTooManyRequests, payload: `{"error":{"code":"model_not_found"}}`, match: false},
		{name: "transport-shaped payload", status: http.StatusBadRequest, payload: `not-json`, match: false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			require.Equal(t, tc.match, excelBPSModelUnavailable(tc.status, []byte(tc.payload)))
		})
	}
}

func TestExcelBPSHTTPModelRejectionFallsBackAcrossStatuses(t *testing.T) {
	tests := []struct {
		name    string
		status  int
		payload string
	}{
		{name: "access changed", status: http.StatusForbidden, payload: `{"error":{"code":"basispoints_model_access_changed","message":"model unavailable"}}`},
		{name: "not found", status: http.StatusNotFound, payload: `{"error":{"code":"model_not_found","message":"model unavailable"}}`},
		{name: "unsupported", status: http.StatusUnprocessableEntity, payload: `{"error":{"code":"unsupported_model","message":"model unavailable"}}`},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			upstream := &httpUpstreamRecorder{responses: []*http.Response{
				excelBPSJSONResponse(tc.status, tc.payload),
				excelBPSOrdinaryResponse(),
			}}
			svc := openAIClientToolsTestService(upstream)
			body := []byte(`{"model":"gpt-5.6-sol","stream":false,"input":"retain this input","metadata":{"marker":"original"}}`)
			c, rec := newExcelBPSFallbackContext(body, false)
			account := excelAccount()

			result, err := svc.Forward(context.Background(), c, account, body)
			require.NoError(t, err)
			require.NotNil(t, result)
			require.Len(t, upstream.requests, 2)
			require.Equal(t, "/basispoints/api/responses", upstream.requests[0].URL.Path)
			require.Equal(t, chatgptCodexURL, upstream.requests[1].URL.String())
			require.Contains(t, string(upstream.bodies[1]), "retain this input")
			require.Equal(t, "gpt-5.6-sol", gjson.GetBytes(upstream.bodies[1], "model").String())
			require.Contains(t, rec.Body.String(), "ordinary-stream")
			require.True(t, account.IsExcelBPSEnabled())
			require.Equal(t, openAIResponsesUpstreamEndpoint, GetActualOpenAIUpstreamEndpoint(c))
			require.Equal(t, "gpt-5.6-sol", c.GetString(OpsUpstreamModelKey))
		})
	}
}

func TestExcelBPSFallbackPassesAuthoritativeAdmission(t *testing.T) {
	upstream := &httpUpstreamRecorder{responses: []*http.Response{
		excelBPSJSONResponse(http.StatusForbidden, `{"error":{"code":"model_not_found"}}`),
		excelBPSOrdinaryResponse(),
	}}
	svc := openAIClientToolsTestService(upstream)
	selected := excelAccount()
	latest := *selected
	latest.Extra = map[string]any{}
	for key, value := range selected.Extra {
		latest.Extra[key] = value
	}
	svc.accountRepo = &turnAdmissionRepo{account: &latest}
	body := []byte(`{"model":"gpt-5.6-sol","input":"authoritative admission"}`)
	c, _ := newExcelBPSFallbackContext(body, false)

	result, err := svc.Forward(context.Background(), c, selected, body)
	require.NoError(t, err)
	require.NotNil(t, result)
	require.Len(t, upstream.requests, 2)
	require.True(t, selected.IsExcelBPSEnabled())
	require.Equal(t, chatgptCodexURL, upstream.requests[1].URL.String())
}

func TestExcelBPSInitialModelFailureSSEFallsBackBeforeClientOutput(t *testing.T) {
	upstream := &httpUpstreamRecorder{responses: []*http.Response{
		excelBPSSSEResponse("event: response.failed\ndata: {\"type\":\"response.failed\",\"response\":{\"id\":\"bps-failed\",\"status\":\"failed\",\"error\":{\"code\":\"model_not_supported\",\"message\":\"model unavailable\"}}}\n\n"),
		excelBPSOrdinaryStreamResponse(),
	}}
	svc := openAIClientToolsTestService(upstream)
	body := []byte(`{"model":"gpt-5.6-sol","stream":true,"input":"stream input"}`)
	c, rec := newExcelBPSFallbackContext(body, true)
	account := excelAccount()

	result, err := svc.Forward(context.Background(), c, account, body)
	require.NoError(t, err)
	require.NotNil(t, result)
	require.Len(t, upstream.requests, 2)
	require.Equal(t, "/basispoints/api/responses", upstream.requests[0].URL.Path)
	require.Equal(t, chatgptCodexURL, upstream.requests[1].URL.String())
	require.Contains(t, rec.Body.String(), "ordinary-stream")
	require.Contains(t, rec.Body.String(), "ordinary output")
	require.NotContains(t, rec.Body.String(), "bps-failed")
	require.NotContains(t, rec.Body.String(), "model_not_supported")
	require.True(t, account.IsExcelBPSEnabled())
	require.Equal(t, openAIResponsesUpstreamEndpoint, GetActualOpenAIUpstreamEndpoint(c))

	eventsValue, ok := c.Get(OpsUpstreamErrorsKey)
	require.True(t, ok)
	events, ok := eventsValue.([]*OpsUpstreamErrorEvent)
	require.True(t, ok)
	require.Len(t, events, 1)
	require.Equal(t, "model_fallback", events[0].Kind)
	require.Equal(t, basispoints.ResponsesURL, events[0].UpstreamURL)
}

func TestExcelBPSFallbackPreservesOriginalImagesAndTools(t *testing.T) {
	imageURL, _ := excelBPSImageFixture(t)
	bodyValue := map[string]any{
		"model": "gpt-5.6-sol",
		"input": []any{map[string]any{
			"type": "message", "role": "user", "content": []any{
				map[string]any{"type": "input_text", "text": "look at this"},
				map[string]any{"type": "input_image", "image_url": imageURL, "detail": "high"},
			},
		}},
		"tools":    []any{map[string]any{"type": "function", "name": "keep_tool", "description": "keep this tool"}},
		"metadata": map[string]any{"marker": "original"},
	}
	body, err := json.Marshal(bodyValue)
	require.NoError(t, err)
	upstream := &httpUpstreamRecorder{responses: []*http.Response{
		excelBPSJSONResponse(http.StatusOK, `{"openai_file_id":"file-upload-1"}`),
		excelBPSJSONResponse(http.StatusForbidden, `{"error":{"code":"model_access_denied","message":"model unavailable"}}`),
		excelBPSOrdinaryResponse(),
	}}
	svc := openAIClientToolsTestService(upstream)
	c, _ := newExcelBPSFallbackContext(body, false)
	account := excelAccount()

	_, err = svc.Forward(context.Background(), c, account, body)
	require.NoError(t, err)
	require.Len(t, upstream.requests, 3)
	require.Equal(t, "/basispoints/api/attachments", upstream.requests[0].URL.Path)
	require.Equal(t, "/basispoints/api/responses", upstream.requests[1].URL.Path)
	require.Equal(t, chatgptCodexURL, upstream.requests[2].URL.String())
	require.Contains(t, string(upstream.bodies[2]), imageURL)
	require.NotContains(t, string(upstream.bodies[2]), "file-upload-1")
	require.Equal(t, "keep_tool", gjson.GetBytes(upstream.bodies[2], "tools.0.name").String())
	require.Equal(t, "gpt-5.6-sol", gjson.GetBytes(upstream.bodies[2], "model").String())
	require.True(t, account.IsExcelBPSEnabled())
}

func TestExcelBPSFallbackUsesOrdinaryErrorHandlingOnce(t *testing.T) {
	upstream := &httpUpstreamRecorder{responses: []*http.Response{
		excelBPSJSONResponse(http.StatusForbidden, `{"error":{"code":"model_not_found"}}`),
		excelBPSJSONResponse(http.StatusBadRequest, `{"error":{"type":"invalid_request_error","message":"ordinary validation error"}}`),
	}}
	svc := openAIClientToolsTestService(upstream)
	body := []byte(`{"model":"gpt-5.6-sol","input":"x"}`)
	c, _ := newExcelBPSFallbackContext(body, false)

	_, err := svc.Forward(context.Background(), c, excelAccount(), body)
	require.Error(t, err)
	require.Len(t, upstream.requests, 2)
	require.EqualError(t, err, "upstream error: 400 (client response sanitized)")
	require.Equal(t, chatgptCodexURL, upstream.requests[1].URL.String())
}
