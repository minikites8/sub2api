package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/Wei-Shaw/sub2api/internal/pkg/typesafe"
)

// GPTAutoContextTurn is one bounded, redacted conversation turn.
type GPTAutoContextTurn struct {
	Role string `json:"role"`
	Text string `json:"text"`
}

// GPTAutoRequestSnapshot is the sanitized, bounded view of a customer request
// that the shadow pipeline is allowed to hold. It is built synchronously from
// the request body and never references the body afterwards.
type GPTAutoRequestSnapshot struct {
	Endpoint        string
	RequestedModel  string
	RequestedEffort string
	Stream          bool
	HasTools        bool
	ToolCount       int
	HasImages       bool
	MessageCount    int
	// HasSystemPrompt records that operator/harness instructions (system or
	// developer turns, Responses "instructions") were present. Their text is
	// never forwarded: it describes the client application, not the task.
	HasSystemPrompt bool
	// AgentStepsSinceUser counts assistant, tool and reasoning items after
	// the latest user turn, i.e. how far an agent loop has progressed.
	AgentStepsSinceUser int
	LatestUser          string
	LatestUserKind      string
	RecentContext       []GPTAutoContextTurn
	TaskContext         *GPTAutoTaskContext
	AgentActivity       *GPTAutoAgentActivity
	SystemContext       GPTAutoSystemContext
	// Limitations lists what was not visible (history_by_reference, ...).
	Limitations []string
	// Context is extraction metadata (counts and budgets, no content).
	Context GPTAutoContextInfo
	// EstimatedInputTokens approximates the full prompt size from character
	// counts before truncation. It is an estimate, not a tokenizer count.
	EstimatedInputTokens int

	RequestID string
	UserID    int64
	APIKeyID  int64
	GroupID   *int64
}

// gptAutoCharsPerToken is a coarse character-to-token ratio used only for
// hypothetical cost estimates; the dashboard labels the result as estimated.
const gptAutoCharsPerToken = 4

var gptAutoPIIPatterns = []*regexp.Regexp{
	regexp.MustCompile(`[A-Za-z0-9._%+-]+@[A-Za-z0-9.-]+\.[A-Za-z]{2,}`),
	regexp.MustCompile(`\+?\d[\d\s().-]{8,}\d`),
}

// redactGPTAutoText removes credentials (reusing the content-moderation
// patterns) and common personal identifiers before text leaves the gateway.
func redactGPTAutoText(text string) string {
	text = redactContentModerationSecrets(text)
	for _, pattern := range gptAutoPIIPatterns {
		text = pattern.ReplaceAllString(text, "[redacted]")
	}
	return text
}

// gptAutoTurnCaptureFactor sizes the per-turn capture window relative to the
// final budget. The final head/tail trim always falls strictly inside this
// window, so a secret split at the capture boundary can never survive
// redaction into the snapshot.
const gptAutoTurnCaptureFactor = 2

func gptAutoNormalizeRole(role string) string {
	switch strings.ToLower(strings.TrimSpace(role)) {
	case "user":
		return "user"
	case "assistant":
		return "assistant"
	case "tool", "function":
		return "tool"
	case "system", "developer":
		return "system"
	default:
		return "other"
	}
}

// trimRunesKeepTail keeps the head and tail of an over-long text: the end of
// a prompt usually carries the actual instruction. It slices by byte offset so
// a multi-megabyte input is never converted to a rune slice.
func trimRunesKeepTail(text string, max int) string {
	if max <= 0 {
		return ""
	}
	if utf8.RuneCountInString(text) <= max {
		return text
	}
	const marker = " … "
	markerRunes := utf8.RuneCountInString(marker)
	if max <= 2*markerRunes {
		return strings.Clone(text[:gptAutoRuneOffset(text, max)])
	}
	head := (max - markerRunes) / 3
	tail := max - markerRunes - head
	return text[:gptAutoRuneOffset(text, head)] + marker + text[gptAutoTailOffset(text, tail):]
}

// gptAutoRuneOffset returns the byte offset just after the first n runes.
func gptAutoRuneOffset(text string, n int) int {
	offset := 0
	for i := 0; i < n && offset < len(text); i++ {
		_, size := utf8.DecodeRuneInString(text[offset:])
		offset += size
	}
	return offset
}

// gptAutoTailOffset returns the byte offset where the last n runes begin.
func gptAutoTailOffset(text string, n int) int {
	offset := len(text)
	for i := 0; i < n && offset > 0; i++ {
		_, size := utf8.DecodeLastRuneInString(text[:offset])
		offset -= size
	}
	return offset
}

// Jev question ids. Stable: stored payloads and tests rely on them.
const (
	gptAutoQuestionModel      = "target_model"
	gptAutoQuestionEffort     = "reasoning_effort"
	gptAutoQuestionComplexity = "task_complexity"
	gptAutoQuestionCategory   = "task_category"
)

// gptAutoClassifierState is the compact state object sent to Jev. It is data
// only: operator instructions travel in the question-level "instructions"
// fields and are never mixed into it.
//
// state-v3 (see gpt_auto_context.go):
//   - never contains the requested model/effort, prompt size, or any
//     system/developer text (system_context describes them without content);
//   - latest_user_request is the newest user turn of the whole request, and
//     latest_user_request_kind says whether it is a short follow-up;
//   - task_context, agent_activity and recent_context are bounded, redacted
//     selections; tool outputs are summarised, never forwarded whole;
//   - context_limitations states what was not visible (for example history
//     held by previous_response_id or removed by compaction).
type gptAutoClassifierState struct {
	Endpoint              string                `json:"endpoint"`
	HasTools              bool                  `json:"has_tools"`
	ToolCount             int                   `json:"tool_count"`
	HasImages             bool                  `json:"has_images"`
	MessageCount          int                   `json:"message_count"`
	SystemContext         *GPTAutoSystemContext `json:"system_context,omitempty"`
	LatestUserRequest     string                `json:"latest_user_request"`
	LatestUserRequestKind string                `json:"latest_user_request_kind"`
	TaskContext           *GPTAutoTaskContext   `json:"task_context,omitempty"`
	AgentActivity         *GPTAutoAgentActivity `json:"agent_activity,omitempty"`
	RecentContext         []GPTAutoContextTurn  `json:"recent_context,omitempty"`
	ContextLimitations    []string              `json:"context_limitations,omitempty"`
}

func buildGPTAutoClassifierState(snapshot GPTAutoRequestSnapshot) (string, error) {
	state := gptAutoClassifierState{
		Endpoint:              snapshot.Endpoint,
		HasTools:              snapshot.HasTools,
		ToolCount:             snapshot.ToolCount,
		HasImages:             snapshot.HasImages,
		MessageCount:          snapshot.MessageCount,
		LatestUserRequest:     snapshot.LatestUser,
		LatestUserRequestKind: snapshot.LatestUserKind,
		TaskContext:           snapshot.TaskContext,
		AgentActivity:         snapshot.AgentActivity,
		RecentContext:         snapshot.RecentContext,
		ContextLimitations:    snapshot.Limitations,
	}
	if state.LatestUserRequestKind == "" {
		state.LatestUserRequestKind = GPTAutoUserKindNone
	}
	if snapshot.SystemContext.Present || snapshot.SystemContext.HarnessMessages > 0 {
		system := snapshot.SystemContext
		state.SystemContext = &system
	}
	raw, err := json.Marshal(state)
	if err != nil {
		return "", err
	}
	return string(raw), nil
}

// ErrGPTAutoClassifierNotConfigured is returned when no API key is set.
var ErrGPTAutoClassifierNotConfigured = errors.New("gpt-auto classifier is not configured")

// ErrGPTAutoNothingToClassify is returned when the snapshot has no user text.
var ErrGPTAutoNothingToClassify = errors.New("no user text to classify")

// GPTAutoClassifier performs one Jev classification. Implementations must not
// call the public, customer-billable gateway.
type GPTAutoClassifier interface {
	Classify(ctx context.Context, cfg GPTAutoShadowConfig, snapshot GPTAutoRequestSnapshot) (*GPTAutoClassification, error)
}

// GPTAutoClassifierError carries a stable error class for metrics/logs.
// Message never contains provider bodies or credentials.
type GPTAutoClassifierError struct {
	Class      string
	StatusCode int
	Err        error
	// Billed usage reported by a 2xx response whose answers were unusable.
	ClassifierModel string
	InputTokens     int
	OutputTokens    int
}

func (e *GPTAutoClassifierError) Error() string {
	if e.StatusCode > 0 {
		return fmt.Sprintf("%s (status %d)", e.Class, e.StatusCode)
	}
	return e.Class
}

func (e *GPTAutoClassifierError) Unwrap() error { return e.Err }

// Classifier error classes.
const (
	GPTAutoErrTimeout        = "timeout"
	GPTAutoErrAuth           = "auth"
	GPTAutoErrRateLimited    = "rate_limited"
	GPTAutoErrUpstream       = "upstream_error"
	GPTAutoErrInvalidRequest = "invalid_request"
	GPTAutoErrInvalidAnswer  = "invalid_answer"
	GPTAutoErrTransport      = "transport"
	GPTAutoErrNotConfigured  = "not_configured"
	GPTAutoErrNothingToClass = "nothing_to_classify"
	// Budget admission failures. Only stored when an earlier attempt for the
	// same decision already reached Jev.
	GPTAutoErrBudgetExhausted   = "budget_exhausted"
	GPTAutoErrBudgetUnavailable = "budget_unavailable"
)

// GPTAutoHTTPClientFactory returns the HTTP client for a classifier call,
// honouring the configured proxy.
type GPTAutoHTTPClientFactory func(ctx context.Context, cfg GPTAutoShadowConfig) (*http.Client, error)

// typeSafeGPTAutoClassifier calls TypeSafe System One directly with the
// server-side credential from the shadow config.
type typeSafeGPTAutoClassifier struct {
	httpClient GPTAutoHTTPClientFactory
}

// NewTypeSafeGPTAutoClassifier builds the production classifier.
func NewTypeSafeGPTAutoClassifier(factory GPTAutoHTTPClientFactory) GPTAutoClassifier {
	return &typeSafeGPTAutoClassifier{httpClient: factory}
}

func (c *typeSafeGPTAutoClassifier) Classify(ctx context.Context, cfg GPTAutoShadowConfig, snapshot GPTAutoRequestSnapshot) (*GPTAutoClassification, error) {
	if strings.TrimSpace(cfg.ClassifierAPIKey) == "" {
		return nil, &GPTAutoClassifierError{Class: GPTAutoErrNotConfigured, Err: ErrGPTAutoClassifierNotConfigured}
	}
	if !snapshot.HasClassifiableContent() {
		return nil, &GPTAutoClassifierError{Class: GPTAutoErrNothingToClass, Err: ErrGPTAutoNothingToClassify}
	}
	state, err := buildGPTAutoClassifierState(snapshot)
	if err != nil {
		return nil, &GPTAutoClassifierError{Class: GPTAutoErrInvalidRequest, Err: err}
	}
	client := http.DefaultClient
	if c.httpClient != nil {
		client, err = c.httpClient(ctx, cfg)
		if err != nil {
			return nil, &GPTAutoClassifierError{Class: GPTAutoErrTransport, Err: err}
		}
	}
	callCtx, cancel := context.WithTimeout(ctx, time.Duration(cfg.ClassifierTimeoutMS)*time.Millisecond)
	defer cancel()
	answers, status, err := typesafe.EvaluateQuestions(callCtx, client, cfg.ClassifierBaseURL, cfg.ClassifierAPIKey, typesafe.Request{
		Model:     cfg.ClassifierModel,
		State:     state,
		Questions: buildGPTAutoQuestions(cfg),
	})
	if err != nil {
		classified := classifyGPTAutoError(err, status)
		if answers != nil {
			classified.ClassifierModel = answers.Model
			classified.InputTokens = answers.Usage.InputTokens
			classified.OutputTokens = answers.Usage.OutputTokens
		}
		return nil, classified
	}
	return gptAutoClassificationFromAnswers(answers), nil
}

func classifyGPTAutoError(err error, status int) *GPTAutoClassifierError {
	switch {
	case errors.Is(err, context.DeadlineExceeded):
		return &GPTAutoClassifierError{Class: GPTAutoErrTimeout, Err: err}
	case status == http.StatusUnauthorized || status == http.StatusForbidden || status == http.StatusPaymentRequired:
		return &GPTAutoClassifierError{Class: GPTAutoErrAuth, StatusCode: status, Err: err}
	case status == http.StatusTooManyRequests:
		return &GPTAutoClassifierError{Class: GPTAutoErrRateLimited, StatusCode: status, Err: err}
	case status == http.StatusBadRequest || status == http.StatusRequestEntityTooLarge || status == http.StatusUnprocessableEntity:
		return &GPTAutoClassifierError{Class: GPTAutoErrInvalidRequest, StatusCode: status, Err: err}
	case status >= 500:
		return &GPTAutoClassifierError{Class: GPTAutoErrUpstream, StatusCode: status, Err: err}
	case status >= 200 && status < 300:
		return &GPTAutoClassifierError{Class: GPTAutoErrInvalidAnswer, StatusCode: status, Err: err}
	default:
		return &GPTAutoClassifierError{Class: GPTAutoErrTransport, StatusCode: status, Err: err}
	}
}

// isRetryableGPTAutoError reports transient failures worth one more attempt.
func isRetryableGPTAutoError(err error) bool {
	var classified *GPTAutoClassifierError
	if !errors.As(err, &classified) {
		return false
	}
	switch classified.Class {
	case GPTAutoErrTransport, GPTAutoErrUpstream, GPTAutoErrRateLimited:
		return true
	default:
		return false
	}
}

func gptAutoClassificationFromAnswers(answers *typesafe.Answers) *GPTAutoClassification {
	out := &GPTAutoClassification{
		ClassifierModel: answers.Model,
		InputTokens:     answers.Usage.InputTokens,
		OutputTokens:    answers.Usage.OutputTokens,
	}
	if model, ok := answers.Choice[gptAutoQuestionModel]; ok {
		out.Model, out.ModelConfidence, out.ModelProbabilities = model.Choice, model.Confidence, model.Probabilities
	}
	if effort, ok := answers.Choice[gptAutoQuestionEffort]; ok {
		out.Effort, out.EffortConfidence, out.EffortProbabilities = effort.Choice, effort.Confidence, effort.Probabilities
	}
	if complexity, ok := answers.Score[gptAutoQuestionComplexity]; ok {
		out.Complexity = complexity.Score
		out.ComplexityConfidence = complexity.Confidence
		out.ComplexityProbabilities = complexity.Probabilities
	}
	if category, ok := answers.Choice[gptAutoQuestionCategory]; ok {
		out.Category, out.CategoryConfidence, out.CategoryProbabilities = category.Choice, category.Confidence, category.Probabilities
	}
	return out
}
