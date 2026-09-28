package service

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/pkg/apicompat"
	"github.com/gin-gonic/gin"
	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
)

// Run the canonical Responses path, including group policy, BPS fallbacks and
// rate-limit retries. The writer only adapts the downstream wire protocol and
// retains the actual client's write state for the handler's replay decision.
func (s *OpenAIGatewayService) forwardExcelBPSChatCompletions(ctx context.Context, c *gin.Context, account *Account, body []byte, originalModel, billingModel, promptCacheKey string, stream bool) (*OpenAIForwardResult, error) {
	body, err := sjson.SetBytes(body, "stream", true)
	if err != nil {
		return nil, err
	}
	if promptCacheKey != "" && gjson.GetBytes(body, "prompt_cache_key").String() == "" {
		body, err = sjson.SetBytes(body, "prompt_cache_key", promptCacheKey)
		if err != nil {
			return nil, err
		}
	}
	originalWriter := c.Writer
	writer := newExcelBPSChatWriter(originalWriter, originalModel, stream)
	c.Writer = writer
	defer func() { c.Writer = originalWriter }()
	result, err := s.Forward(ctx, c, account, body)
	if result != nil {
		result.Model = originalModel
		result.BillingModel = billingModel
		result.Stream = stream
	}
	return result, err
}

type excelBPSChatWriter struct {
	gin.ResponseWriter
	model        string
	stream       bool
	pending      []byte
	state        *apicompat.ResponsesEventToChatState
	accumulator  *apicompat.BufferedResponseAccumulator
	sawReasoning bool
	done         bool
}

func newExcelBPSChatWriter(writer gin.ResponseWriter, model string, stream bool) *excelBPSChatWriter {
	state := apicompat.NewResponsesEventToChatState()
	state.Model = model
	state.IncludeUsage = true
	return &excelBPSChatWriter{ResponseWriter: writer, model: model, stream: stream, state: state, accumulator: apicompat.NewBufferedResponseAccumulator()}
}

func (w *excelBPSChatWriter) Write(data []byte) (int, error) {
	// JSON errors are produced by canonical forwarding before a stream starts.
	if len(w.pending) == 0 && gjson.ValidBytes(data) {
		if !gjson.GetBytes(data, "error").Exists() && (gjson.GetBytes(data, "object").String() == "response" || gjson.GetBytes(data, "output").IsArray()) {
			payload, err := json.Marshal(gin.H{"type": "response.completed", "response": json.RawMessage(data)})
			if err != nil {
				return 0, err
			}
			err = w.writeFrame(append([]byte("data: "), payload...))
			return len(data), err
		}
		if w.stream && w.ResponseWriter.Written() && gjson.GetBytes(data, "error").Exists() {
			w.done = true
			_, err := w.ResponseWriter.WriteString("data: " + string(data) + "\n\ndata: [DONE]\n\n")
			return len(data), err
		}
		if !w.ResponseWriter.Written() {
			w.Header().Set("Content-Type", "application/json")
		}
		_, err := w.ResponseWriter.Write(data)
		return len(data), err
	}
	w.pending = append(w.pending, data...)
	for {
		end := bytes.Index(w.pending, []byte("\n\n"))
		delimiterSize := 2
		if crlf := bytes.Index(w.pending, []byte("\r\n\r\n")); crlf >= 0 && (end < 0 || crlf < end) {
			end, delimiterSize = crlf, 4
		}
		if end < 0 {
			break
		}
		frame := w.pending[:end]
		if err := w.writeFrame(frame); err != nil {
			return 0, err
		}
		w.pending = w.pending[end+delimiterSize:]
	}
	if len(w.pending) == 0 {
		w.pending = nil
	}
	return len(data), nil
}

func (w *excelBPSChatWriter) WriteString(data string) (int, error) { return w.Write([]byte(data)) }
func (w *excelBPSChatWriter) Flush() {
	// Buffered Chat requests stay wholly JSON, including during BPS heartbeats.
	if w.stream {
		w.ResponseWriter.Flush()
	}
}

func (w *excelBPSChatWriter) writeFrame(frame []byte) error {
	var payload []byte
	var eventType string
	for _, line := range bytes.Split(frame, []byte("\n")) {
		line = bytes.TrimSuffix(line, []byte("\r"))
		if bytes.HasPrefix(line, []byte("event:")) {
			eventType = strings.TrimSpace(string(bytes.TrimPrefix(line, []byte("event:"))))
		}
		if bytes.HasPrefix(line, []byte("data:")) {
			if len(payload) > 0 {
				payload = append(payload, '\n')
			}
			payload = append(payload, bytes.TrimPrefix(bytes.TrimPrefix(line, []byte("data:")), []byte(" "))...)
		}
	}
	if len(payload) == 0 {
		if w.stream && bytes.HasPrefix(bytes.TrimSpace(frame), []byte(":")) {
			_, err := w.ResponseWriter.Write(append(append([]byte(nil), frame...), '\n', '\n'))
			return err
		}
		return nil
	}
	if w.done || bytes.Equal(payload, []byte("[DONE]")) {
		return nil
	}
	var event apicompat.ResponsesStreamEvent
	if err := json.Unmarshal(payload, &event); err != nil {
		return fmt.Errorf("decode BPS chat event: %w", err)
	}
	if event.Type == "" {
		event.Type = eventType
	}
	if event.Type == "error" || event.Type == "response.failed" {
		code := extractUpstreamErrorCode(payload)
		message := extractOpenAISSEErrorMessage(payload)
		if code == "" {
			code = "upstream_error"
		}
		if message == "" {
			message = "Upstream response failed"
		}
		w.done = true
		if w.stream {
			_, err := w.ResponseWriter.WriteString(buildChatStreamErrorSSE(code, message) + "data: [DONE]\n\n")
			return err
		}
		w.Header().Set("Content-Type", "application/json")
		w.ResponseWriter.WriteHeader(http.StatusBadGateway)
		raw, _ := json.Marshal(gin.H{"error": gin.H{"type": "upstream_error", "code": code, "message": message}})
		_, err := w.ResponseWriter.Write(raw)
		return err
	}
	terminal := event.Type == "response.completed" || event.Type == "response.done" || event.Type == "response.incomplete"
	if !w.stream {
		w.accumulator.ProcessEvent(&event)
		if !terminal {
			return nil
		}
		if event.Response == nil {
			return fmt.Errorf("BPS chat terminal event is missing response")
		}
		if event.Response.Usage == nil {
			event.Response.Usage = event.Usage
		}
		w.accumulator.SupplementResponseOutput(event.Response)
		raw, err := json.Marshal(apicompat.ResponsesToChatCompletions(event.Response, w.model))
		if err != nil {
			return err
		}
		w.done = true
		w.Header().Set("Content-Type", "application/json")
		w.Header().Del("Content-Length")
		_, err = w.ResponseWriter.Write(raw)
		return err
	}
	if terminal {
		if !w.state.SentRole {
			if err := w.emit(&apicompat.ResponsesStreamEvent{Type: "response.created", Response: event.Response}); err != nil {
				return err
			}
		}
		if err := w.supplementTerminal(payload); err != nil {
			return err
		}
	}
	if event.Type == "response.reasoning_text.delta" || event.Type == "response.reasoning_summary_text.delta" {
		w.sawReasoning = w.sawReasoning || event.Delta != ""
	}
	if err := w.emit(&event); err != nil {
		return err
	}
	if terminal {
		w.done = true
		_, err := w.ResponseWriter.WriteString("data: [DONE]\n\n")
		return err
	}
	return nil
}

func (w *excelBPSChatWriter) emit(event *apicompat.ResponsesStreamEvent) error {
	for _, chunk := range apicompat.ResponsesEventToChatChunks(event, w.state) {
		if !w.ResponseWriter.Written() {
			w.Header().Set("Content-Type", "text/event-stream")
			w.Header().Del("Content-Length")
		}
		frame, err := apicompat.ChatChunkToSSE(chunk)
		if err != nil {
			return err
		}
		if _, err = w.ResponseWriter.WriteString(frame); err != nil {
			return err
		}
	}
	return nil
}

// Some upstreams put all content in the terminal response. Emit only the
// content that was absent from deltas; shared tool tracking fills argument tails.
func (w *excelBPSChatWriter) supplementTerminal(payload []byte) error {
	hadText, hadReasoning := w.state.SawText, w.sawReasoning
	for index, item := range gjson.GetBytes(payload, "response.output").Array() {
		switch item.Get("type").String() {
		case "message":
			if hadText {
				continue
			}
			for _, part := range item.Get("content").Array() {
				text := part.Get("text").String()
				if part.Get("type").String() == "refusal" {
					text = part.Get("refusal").String()
				}
				if err := w.emit(&apicompat.ResponsesStreamEvent{Type: "response.output_text.delta", OutputIndex: index, Delta: text}); err != nil {
					return err
				}
			}
		case "reasoning":
			if hadReasoning {
				continue
			}
			for _, part := range item.Get("summary").Array() {
				if err := w.emit(&apicompat.ResponsesStreamEvent{Type: "response.reasoning_summary_text.delta", OutputIndex: index, Delta: part.Get("text").String()}); err != nil {
					return err
				}
			}
		case "function_call", "custom_tool_call":
			var output apicompat.ResponsesOutput
			if err := json.Unmarshal([]byte(item.Raw), &output); err != nil {
				return err
			}
			if _, exists := w.state.OutputIndexToToolIndex[index]; !exists {
				if err := w.emit(&apicompat.ResponsesStreamEvent{Type: "response.output_item.added", OutputIndex: index, Item: &output}); err != nil {
					return err
				}
			}
			kind := "response.function_call_arguments.done"
			if strings.HasPrefix(item.Get("type").String(), "custom_") {
				kind = "response.custom_tool_call_input.done"
			}
			if err := w.emit(&apicompat.ResponsesStreamEvent{Type: kind, OutputIndex: index, Arguments: item.Get("arguments").String(), Input: item.Get("input").String()}); err != nil {
				return err
			}
		}
	}
	return nil
}
