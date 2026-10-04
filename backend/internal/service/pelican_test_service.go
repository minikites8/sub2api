package service

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

type pelicanTestContextKey struct{}

type pelicanTestOptions struct {
	prompt          string
	reasoningEffort string
	testChannel     string
	observeOnly     bool
}

func withPelicanTestOptions(ctx context.Context, options pelicanTestOptions) context.Context {
	return context.WithValue(ctx, pelicanTestContextKey{}, options)
}

func pelicanTestOptionsFromContext(ctx context.Context) (pelicanTestOptions, bool) {
	options, ok := ctx.Value(pelicanTestContextKey{}).(pelicanTestOptions)
	return options, ok
}

func isQualityObservation(ctx context.Context) bool {
	options, _ := pelicanTestOptionsFromContext(ctx)
	return options.observeOnly
}

// TestPelicanAccountConnection is the dedicated account test path for the
// Pelican UI. The existing /test endpoint deliberately keeps its historical
// probe payload; only this endpoint opts into the user prompt and reasoning.
func (s *AccountTestService) TestPelicanAccountConnection(c *gin.Context, accountID int64, modelID, prompt, reasoningEffort string) error {
	options, _ := pelicanTestOptionsFromContext(c.Request.Context())
	options.prompt = strings.TrimSpace(prompt)
	options.reasoningEffort = normalizePelicanReasoningEffort(reasoningEffort)
	ctx := withPelicanTestOptions(c.Request.Context(), options)
	c.Request = c.Request.WithContext(ctx)
	return s.TestAccountConnection(c, accountID, modelID, options.prompt, AccountTestModeDefault)
}

// OpenAI question tests share business forwarding's ticket admission, cookie
// restoration, model mapping, transport selection, and response observation.
func (s *AccountTestService) testOpenAICodexPelicanConnection(c *gin.Context, account *Account, model string, options pelicanTestOptions) error {
	if s.openaiGatewayService == nil {
		return s.sendErrorAndEnd(c, "OpenAI gateway is unavailable for the question test")
	}
	body, err := json.Marshal(createPelicanOpenAIPayload(model, true, options.prompt, options.reasoningEffort))
	if err != nil {
		return s.sendErrorAndEnd(c, "Failed to create question test payload")
	}
	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")
	c.Header("X-Accel-Buffering", "no")
	s.sendEvent(c, TestEvent{Type: "test_start", Model: account.GetMappedModel(model)})

	stream := &pelicanOpenAIStreamRecorder{
		ResponseRecorder: httptest.NewRecorder(), service: s, client: c,
		usage: startPelicanTestStream(c, "openai"),
	}
	probe, _ := gin.CreateTestContext(stream)
	probe.Request = c.Request.Clone(c.Request.Context())
	if probe.Request.Header == nil {
		probe.Request.Header = make(http.Header)
	}
	if scope, _ := resolveOpenAIWSExecutionScope(probe, body, 0); scope == "" {
		probe.Request.Header.Set("Session-Id", "question-test-"+uuid.NewString())
	}
	result, err := s.openaiGatewayService.Forward(probe.Request.Context(), probe, account, body)
	if err != nil {
		if errors.Is(err, ErrOpenAICodexTicketUnavailable) {
			return s.sendErrorAndEnd(c, "当前账号的门票暂不可用，请等待打票成功后重试")
		}
		var failover *UpstreamFailoverError
		if errors.As(err, &failover) && failover.ClientMessage != "" {
			return s.sendErrorAndEnd(c, failover.ClientMessage)
		}
		return s.sendErrorAndEnd(c, err.Error())
	}
	if result == nil || result.ClientDisconnect {
		return s.sendErrorAndEnd(c, "Question test response was interrupted")
	}
	if stream.errorMsg != "" {
		return s.sendErrorAndEnd(c, stream.errorMsg)
	}
	if !stream.done {
		return s.sendErrorAndEnd(c, "Stream ended before response.completed")
	}
	s.sendEvent(c, TestEvent{Type: "test_complete", Success: true})
	return nil
}

// Translate Responses deltas into the account-test SSE format as they arrive.
type pelicanOpenAIStreamRecorder struct {
	*httptest.ResponseRecorder
	service  *AccountTestService
	client   *gin.Context
	usage    *pelicanTestUsage
	pending  []byte
	done     bool
	errorMsg string
}

func (w *pelicanOpenAIStreamRecorder) Write(data []byte) (int, error) {
	n, err := w.ResponseRecorder.Write(data)
	w.pending = append(w.pending, data[:n]...)
	for {
		end := bytes.IndexByte(w.pending, '\n')
		if end < 0 {
			break
		}
		line := strings.TrimSpace(string(w.pending[:end]))
		w.pending = w.pending[end+1:]
		if raw, ok := strings.CutPrefix(line, "data:"); ok {
			raw = strings.TrimSpace(raw)
			w.usage.read(raw)
			if !w.done {
				w.done, w.errorMsg = w.service.processOpenAIStreamEvent(w.client, raw)
			}
		}
	}
	return n, err
}

func (w *pelicanOpenAIStreamRecorder) WriteString(data string) (int, error) {
	return w.Write([]byte(data))
}

func normalizePelicanReasoningEffort(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "minimal", "low", "medium", "high", "xhigh":
		return strings.ToLower(strings.TrimSpace(value))
	default:
		return ""
	}
}

func createPelicanClaudePayload(modelID, prompt string) (map[string]any, error) {
	payload, err := createTestPayload(modelID)
	if err != nil {
		return nil, err
	}
	messages, ok := payload["messages"].([]map[string]any)
	if !ok || len(messages) == 0 {
		return payload, nil
	}
	content, ok := messages[0]["content"].([]map[string]any)
	if ok && len(content) > 0 {
		content[0]["text"] = promptOrDefault(prompt)
	}
	return payload, nil
}

func createPelicanOpenAIPayload(modelID string, isOAuth bool, prompt, reasoningEffort string) map[string]any {
	payload := createOpenAITestPayload(modelID, isOAuth)
	input, ok := payload["input"].([]map[string]any)
	if ok && len(input) > 0 {
		content, ok := input[0]["content"].([]map[string]any)
		if ok && len(content) > 0 {
			content[0]["text"] = promptOrDefault(prompt)
		}
	}
	if effort := normalizePelicanReasoningEffort(reasoningEffort); effort != "" {
		payload["reasoning"] = map[string]any{"effort": effort}
	}
	return payload
}

func promptOrDefault(prompt string) string {
	if value := strings.TrimSpace(prompt); value != "" {
		return value
	}
	return "hi"
}
