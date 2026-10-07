package handler

import (
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/service"
)

func TestReadRequestBodyRejectsAmbiguousModels(t *testing.T) {
	for _, body := range []string{
		`{"model":"gpt-5.6-luna","model":"gpt-6-astra"}`,
		`{"model":"gpt-6-astra","Model":"gpt-5.6-luna"}`,
		`{"model":"gpt-6-astra","\u006dodel":"gpt-5.6-luna"}`,
		`{"model":"gpt-6-astra","model":null}`,
	} {
		t.Run(body, func(t *testing.T) {
			req := httptest.NewRequest("POST", "/v1/responses", strings.NewReader(body))
			if _, err := readLenientJSONRequestBodyWithPrealloc(req, nil); err == nil {
				t.Fatal("ambiguous model fields accepted")
			}
		})
	}
}

func TestOpenAIResponsesWebSocketAmbiguousModelsWithoutAllowlist(t *testing.T) {
	for _, mode := range []string{service.OpenAIWSIngressModePassthrough, service.OpenAIWSIngressModeDedicated} {
		t.Run(mode+"/first", func(t *testing.T) {
			runOpenAIResponsesWebSocketUsageLogCase(t, openAIResponsesWSUsageLogCase{
				firstPayload: `{"type":"response.create","model":"gpt-5.4","model":"gpt-4.1","stream":false}`,
				group:        wsAllowlistGroup(false), ingressMode: mode, firstFrameCloseExpected: true, closeReason: "must be unique",
			})
		})
		t.Run(mode+"/subsequent", func(t *testing.T) {
			runOpenAIResponsesWebSocketUsageLogCase(t, openAIResponsesWSUsageLogCase{
				firstPayload:  `{"type":"response.create","model":"gpt-5.4","stream":false}`,
				secondPayload: `{"type":"response.create","model":"gpt-5.4","model":"gpt-4.1","stream":false}`,
				group:         wsAllowlistGroup(false), ingressMode: mode, secondTurnCloseExpected: true, closeReason: "must be unique",
			})
		})
	}
}
