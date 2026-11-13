package service

import (
	"net/http"

	openaiwsv2 "github.com/Wei-Shaw/sub2api/internal/service/openai_ws_v2"
	coderws "github.com/coder/websocket"
	"github.com/gin-gonic/gin"
)

// Existing processing failures and explicitly opted-in upstream errors enter
// this recovery route. Policy/context-window rejection boundaries still apply.
func openAIWSProcessingFailure(payload []byte, requestContexts ...*gin.Context) bool {
	eventType, _, _ := parseOpenAIWSEventEnvelope(payload)
	if eventType != "error" && eventType != "response.failed" {
		return false
	}
	message := extractOpenAISSEErrorMessage(payload)
	if isOpenAIContextWindowError(message, payload) {
		return false
	}
	if !isOpenAIProcessingFailureError(http.StatusServiceUnavailable, message, payload) &&
		!requestAccountRetryFromContexts(requestContexts, http.StatusBadGateway, message, payload) {
		return false
	}
	if eventType == "error" {
		return openAIStreamErrorEventShouldFailover(payload, message, requestContexts...)
	}
	return openAIStreamFailedEventShouldFailover(payload, message, requestContexts...)
}

func (s *OpenAIGatewayService) newOpenAIWSProcessingFailoverError(c *gin.Context, account *Account, payload []byte, model string, headers http.Header) *UpstreamFailoverError {
	failover := s.newOpenAIStreamFailoverErrorWithModel(c, account, true, headers.Get("x-request-id"), payload, extractOpenAISSEErrorMessage(payload), model, headers)
	// The WS handler excludes this account immediately; keep that policy explicit.
	failover.RetryableOnSameAccount = false
	failover.SameAccountRetryMax = 0
	return failover
}

// Stage the initial attempt's lifecycle metadata until client-visible content
// or a terminal event commits it. Overflow commits the bounded preamble and
// closes the replay window. Callers intercept processing failure before flush.
type openAIWSFirstOutputStage struct {
	requestContext *gin.Context
	pending        [][]byte
	bytes          int64
	committed      bool
}

func (s *openAIWSFirstOutputStage) messages(payload []byte) [][]byte {
	if s.committed {
		return [][]byte{payload}
	}
	eventType, _, _ := parseOpenAIWSEventEnvelope(payload)
	if openAIWSProcessingFailure(payload, s.requestContext) {
		s.pending = nil
		s.bytes = 0
		return [][]byte{payload}
	}
	if eventType == "error" || openAIStreamDataStartsClientOutput(string(payload), eventType) || isOpenAIWSTerminalEvent(eventType) || s.bytes+int64(len(payload)) > openAIFirstOutputStageMaxBytes || len(s.pending) >= 256 {
		s.committed = true
		messages := append(s.pending, payload)
		s.pending = nil
		s.bytes = 0
		return messages
	}
	s.pending = append(s.pending, append([]byte(nil), payload...))
	s.bytes += int64(len(payload))
	return nil
}

// frames stages delivery only: the relay has already observed this frame's
// usage, identity and timing. A transport failure cannot discard that evidence.
func (s *openAIWSFirstOutputStage) frames(kind coderws.MessageType, payload []byte) []openaiwsv2.RelayFrame {
	if kind == coderws.MessageText {
		messages := s.messages(payload)
		frames := make([]openaiwsv2.RelayFrame, 0, len(messages))
		for _, message := range messages {
			frames = append(frames, openaiwsv2.RelayFrame{MessageType: kind, Payload: message})
		}
		return frames
	}
	frames := make([]openaiwsv2.RelayFrame, 0, len(s.pending)+1)
	for _, message := range s.pending {
		frames = append(frames, openaiwsv2.RelayFrame{MessageType: coderws.MessageText, Payload: message})
	}
	s.pending, s.bytes, s.committed = nil, 0, true
	return append(frames, openaiwsv2.RelayFrame{MessageType: kind, Payload: payload})
}
