package typesafe

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"strings"
)

// Question types supported by the System One wire protocol.
const (
	QuestionTypeNoul   = "noul"
	QuestionTypeChoice = "choice"
	QuestionTypeScore  = "score"
)

// ChoiceAnswer is the "choice" answer shape: the selected criterion key plus
// the calibration metadata the model reports alongside it.
type ChoiceAnswer struct {
	Choice        string
	Confidence    float64
	Probabilities map[string]float64
}

// ScoreAnswer is the "score" answer shape. Score is the probability-weighted
// expected level, so it is not necessarily an integer.
type ScoreAnswer struct {
	Score         float64
	Confidence    float64
	Legend        map[string]string
	Probabilities map[string]float64
}

// Answers holds a decoded multi-question System One response. Each map is
// keyed by the question id supplied in the Request.
type Answers struct {
	Model  string
	Usage  Usage
	Noul   map[string]float64
	Choice map[string]ChoiceAnswer
	Score  map[string]ScoreAnswer
}

// maxClassifyResponseBytes bounds a buffered classification response.
const maxClassifyResponseBytes = 1 << 20

type wireAnswer struct {
	Type       string   `json:"type"`
	Noul       *float64 `json:"noul"`
	Choice     *string  `json:"choice"`
	Score      *float64 `json:"score"`
	Confidence *float64 `json:"confidence"`
	// Legend values may be strings, objects or arrays (they echo the score
	// criteria), so they are decoded raw.
	Legend        map[string]json.RawMessage `json:"legend"`
	Probabilities map[string]float64         `json:"probabilities"`
}

// EvaluateQuestions performs one System One attempt that may mix noul, choice
// and score questions, and decodes every answer type. The caller owns
// timeouts, retries and key rotation.
//
// When a 2xx response decodes but an answer is missing or invalid, the call
// was still billed: the returned error is accompanied by a partial *Answers
// carrying only Model and Usage so the caller can account for the cost.
//
// Evaluate remains the noul-only entry point used by content moderation; this
// function exists because that decoder rejects any non-noul answer.
func EvaluateQuestions(ctx context.Context, client *http.Client, baseURL, key string, input Request) (*Answers, int, error) {
	body, err := json.Marshal(input)
	if err != nil {
		return nil, 0, errors.New("typesafe invalid request")
	}
	req, err := NewSystemOneRequest(ctx, baseURL, key, body)
	if err != nil {
		return nil, 0, err
	}
	resp, err := client.Do(req)
	if err != nil {
		if ctx.Err() != nil {
			return nil, 0, ctx.Err()
		}
		return nil, 0, errors.New("typesafe transport unavailable")
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		// Do not log provider error bodies: they may echo user input or credentials.
		return nil, resp.StatusCode, fmt.Errorf("typesafe API status %d", resp.StatusCode)
	}
	var out struct {
		Model   string                     `json:"model"`
		Usage   map[string]json.RawMessage `json:"usage"`
		Answers map[string]wireAnswer      `json:"answers"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, maxClassifyResponseBytes)).Decode(&out); err != nil || strings.TrimSpace(out.Model) == "" {
		return nil, resp.StatusCode, errors.New("typesafe invalid response")
	}
	// Usage is decoded leniently (as DecodeSystemOneResponse does): the call was
	// billed, so an unexpected number shape must not hide its cost.
	usage := Usage{
		InputTokens:  systemOneTokenCount(out.Usage["input_tokens"]),
		OutputTokens: systemOneTokenCount(out.Usage["output_tokens"]),
	}
	billed := &Answers{Model: out.Model, Usage: usage}

	answers := &Answers{
		Model:  out.Model,
		Usage:  usage,
		Noul:   make(map[string]float64),
		Choice: make(map[string]ChoiceAnswer),
		Score:  make(map[string]ScoreAnswer),
	}
	for id, question := range input.Questions {
		answer, ok := out.Answers[id]
		if !ok {
			return billed, resp.StatusCode, fmt.Errorf("typesafe missing answer for %s", id)
		}
		if answer.Type != question.Type {
			return billed, resp.StatusCode, fmt.Errorf("typesafe invalid answer for %s", id)
		}
		switch question.Type {
		case QuestionTypeNoul:
			if answer.Noul == nil || !validUnit(*answer.Noul) {
				return billed, resp.StatusCode, fmt.Errorf("typesafe invalid answer for %s", id)
			}
			answers.Noul[id] = *answer.Noul
		case QuestionTypeChoice:
			if answer.Choice == nil || !choiceAllowed(question.Criteria, *answer.Choice) {
				return billed, resp.StatusCode, fmt.Errorf("typesafe invalid answer for %s", id)
			}
			answers.Choice[id] = ChoiceAnswer{
				Choice:        *answer.Choice,
				Confidence:    unitOrZero(answer.Confidence),
				Probabilities: sanitizeProbabilities(answer.Probabilities),
			}
		case QuestionTypeScore:
			if answer.Score == nil || math.IsNaN(*answer.Score) || math.IsInf(*answer.Score, 0) {
				return billed, resp.StatusCode, fmt.Errorf("typesafe invalid answer for %s", id)
			}
			answers.Score[id] = ScoreAnswer{
				Score:         *answer.Score,
				Confidence:    unitOrZero(answer.Confidence),
				Legend:        decodeLegend(answer.Legend),
				Probabilities: sanitizeProbabilities(answer.Probabilities),
			}
		default:
			return billed, resp.StatusCode, fmt.Errorf("typesafe unsupported question type %q", question.Type)
		}
	}
	return answers, resp.StatusCode, nil
}

// decodeLegend keeps string level descriptions as-is and renders structured
// ones as compact JSON.
func decodeLegend(in map[string]json.RawMessage) map[string]string {
	if len(in) == 0 {
		return nil
	}
	out := make(map[string]string, len(in))
	for key, raw := range in {
		var text string
		if json.Unmarshal(raw, &text) == nil {
			out[key] = text
			continue
		}
		out[key] = string(raw)
	}
	return out
}

func validUnit(value float64) bool {
	return !math.IsNaN(value) && !math.IsInf(value, 0) && value >= 0 && value <= 1
}

// unitOrZero keeps an out-of-range or absent confidence from poisoning stored
// metrics. Confidence is advisory, so a bad value degrades instead of failing.
func unitOrZero(value *float64) float64 {
	if value == nil || !validUnit(*value) {
		return 0
	}
	return *value
}

func sanitizeProbabilities(in map[string]float64) map[string]float64 {
	if len(in) == 0 {
		return nil
	}
	out := make(map[string]float64, len(in))
	for key, value := range in {
		if validUnit(value) {
			out[key] = value
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// choiceAllowed rejects a selection the gateway never offered, so a confused
// or hostile upstream cannot inject an unknown model id downstream.
func choiceAllowed(criteria any, choice string) bool {
	switch typed := criteria.(type) {
	case map[string]string:
		_, ok := typed[choice]
		return ok
	case map[string]any:
		_, ok := typed[choice]
		return ok
	default:
		return false
	}
}
