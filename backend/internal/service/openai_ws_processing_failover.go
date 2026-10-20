package service

import (
	"context"
	openaiwsv2 "github.com/Wei-Shaw/sub2api/internal/service/openai_ws_v2"
	coderws "github.com/coder/websocket"
	"github.com/gin-gonic/gin"
	"net/http"
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
	if eventType == "error" || openAIStreamDataStartsClientOutput(string(payload), eventType, s.requestContext) || isOpenAIWSTerminalEvent(eventType) || s.bytes+int64(len(payload)) > openAIFirstOutputStageMaxBytes || len(s.pending) >= 256 {
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

// The upstream wrapper observes deadlines before staging; metadata still
// advances transport activity while the first-output deadline stays armed.
type openAIWSProcessingStageFrameConn struct {
	openaiwsv2.FrameConn
	stage         openAIWSFirstOutputStage
	ready         [][]byte
	opaqueType    coderws.MessageType
	opaquePayload []byte
	opaqueReady   bool
}

func (c *openAIWSProcessingStageFrameConn) ReadFrame(ctx context.Context) (coderws.MessageType, []byte, error) {
	for {
		if len(c.ready) > 0 {
			payload := c.ready[0]
			c.ready = c.ready[1:]
			return coderws.MessageText, payload, nil
		}
		if c.opaqueReady {
			c.opaqueReady = false
			return c.opaqueType, c.opaquePayload, nil
		}
		kind, payload, err := c.FrameConn.ReadFrame(ctx)
		if err != nil {
			return kind, payload, err
		}
		if kind != coderws.MessageText {
			c.stage.committed = true
			c.ready = c.stage.pending
			c.stage.pending = nil
			c.stage.bytes = 0
			c.opaqueType = kind
			c.opaquePayload = payload
			c.opaqueReady = true
			continue
		}
		c.ready = c.stage.messages(payload)
	}
}
