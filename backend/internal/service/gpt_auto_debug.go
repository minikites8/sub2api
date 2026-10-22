package service

import (
	"bytes"
	"context"
	"encoding/json"
	"unicode/utf8"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/Wei-Shaw/sub2api/internal/pkg/typesafe"
)

// gptAutoMaxDebugBodyBytes bounds the synthetic request an administrator can
// paste into the state debugger.
const gptAutoMaxDebugBodyBytes = 4 << 20

// GPTAutoDebugInput is a synthetic request for the state debugger. Optional
// overrides let an administrator try other extraction settings without
// saving them.
type GPTAutoDebugInput struct {
	Endpoint     string          `json:"endpoint"`
	Request      json.RawMessage `json:"request"`
	MaxChars     *int            `json:"max_chars,omitempty"`
	ContextTurns *int            `json:"context_turns,omitempty"`
	Strategy     string          `json:"strategy,omitempty"`
	AgentSteps   *int            `json:"agent_steps,omitempty"`
}

// GPTAutoDebugResult is exactly what the classifier would send to System One
// for the input, plus extraction metadata. Nothing is called or stored.
type GPTAutoDebugResult struct {
	Options              GPTAutoContextOptions     `json:"options"`
	Classifiable         bool                      `json:"classifiable"`
	JevRequest           typesafe.Request          `json:"jev_request"`
	StateChars           int                       `json:"state_chars"`
	EstimatedJevTokens   int                       `json:"estimated_jev_input_tokens"`
	Context              GPTAutoContextInfo        `json:"context"`
	Effective            GPTAutoClassifierSnapshot `json:"effective"`
	ClassifierConfigHash string                    `json:"classifier_config_hash"`
	// EstimatedInputTokens is the cost-estimate heuristic for the customer
	// request (characters ÷ 4 of the whole conversation).
	EstimatedInputTokens int `json:"estimated_input_tokens"`
}

// DebugState builds the effective Jev state and questions for an
// administrator-supplied synthetic request. It never calls Jev, never stores
// the input and never reads stored customer data.
func (s *GPTAutoShadowService) DebugState(ctx context.Context, input GPTAutoDebugInput) (*GPTAutoDebugResult, error) {
	cfg, err := s.settings.GetGPTAutoShadowConfig(ctx)
	if err != nil {
		return nil, err
	}
	if input.Endpoint == "" {
		input.Endpoint = GPTAutoEndpointChatCompletions
	}
	if !isGPTAutoSupportedEndpoint(input.Endpoint) {
		return nil, infraerrors.BadRequest("GPT_AUTO_DEBUG_INVALID", "unsupported endpoint")
	}
	body := bytes.TrimSpace(input.Request)
	if len(body) == 0 || body[0] != '{' || !json.Valid(body) {
		return nil, infraerrors.BadRequest("GPT_AUTO_DEBUG_INVALID", "request must be a JSON object")
	}
	if len(body) > gptAutoMaxDebugBodyBytes {
		return nil, infraerrors.BadRequest("GPT_AUTO_DEBUG_INVALID", "request is larger than 4 MiB")
	}
	if input.MaxChars != nil {
		if *input.MaxChars < 100 || *input.MaxChars > gptAutoMaxContextChars {
			return nil, infraerrors.BadRequest("GPT_AUTO_DEBUG_INVALID", "max_chars must be between 100 and 200000")
		}
		cfg.ClassificationMaxContextChars = *input.MaxChars
	}
	if input.ContextTurns != nil {
		if *input.ContextTurns < 0 || *input.ContextTurns > gptAutoMaxContextMessages {
			return nil, infraerrors.BadRequest("GPT_AUTO_DEBUG_INVALID", "context_turns must be between 0 and 20")
		}
		cfg.ClassificationContextMessages = *input.ContextTurns
	}
	if input.Strategy != "" {
		if input.Strategy != GPTAutoContextTaskAware && input.Strategy != GPTAutoContextRecentTurns {
			return nil, infraerrors.BadRequest("GPT_AUTO_DEBUG_INVALID", "strategy must be task_aware or recent_turns")
		}
		cfg.ClassificationContextStrategy = input.Strategy
	}
	if input.AgentSteps != nil {
		if *input.AgentSteps < 1 || *input.AgentSteps > gptAutoMaxAgentSteps {
			return nil, infraerrors.BadRequest("GPT_AUTO_DEBUG_INVALID", "agent_steps must be between 1 and 30")
		}
		cfg.ClassificationAgentSteps = *input.AgentSteps
	}
	options := cfg.ContextOptions().normalized()
	snapshot := BuildGPTAutoSnapshot(input.Endpoint, body, options)
	state, err := buildGPTAutoClassifierState(snapshot)
	if err != nil {
		return nil, err
	}
	effective := NewGPTAutoClassifierSnapshot(*cfg)
	hash, _, _ := effective.Hash()
	request := typesafe.Request{Model: cfg.ClassifierModel, State: state, Questions: effective.Questions}
	raw, _ := json.Marshal(request)
	return &GPTAutoDebugResult{
		Options:              options,
		Classifiable:         snapshot.HasClassifiableContent(),
		JevRequest:           request,
		StateChars:           len([]rune(state)),
		EstimatedJevTokens:   (len(raw) + 3) / 4,
		Context:              snapshot.Context.withStateChars(utf8.RuneCountInString(state)),
		Effective:            effective,
		ClassifierConfigHash: hash,
		EstimatedInputTokens: snapshot.EstimatedInputTokens,
	}, nil
}
