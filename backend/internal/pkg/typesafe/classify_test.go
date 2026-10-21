package typesafe

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/stretchr/testify/require"
)

func classifyQuestions() map[string]Question {
	return map[string]Question{
		"model": {
			Type:         QuestionTypeChoice,
			Instructions: "Pick the model",
			Criteria:     map[string]string{"gpt-6-luna": "balanced", "gpt-6.1-sol": "hardest tasks"},
		},
		"complexity": {
			Type:         QuestionTypeScore,
			Instructions: "Rate complexity",
			Criteria:     []string{"trivial", "moderate", "hard"},
		},
	}
}

func TestEvaluateQuestionsDecodesChoiceAndScore(t *testing.T) {
	questions := classifyQuestions()
	response := `{"model":"jev-1.13.0","usage":{"input_tokens":120,"output_tokens":8},"answers":{` +
		`"model":{"type":"choice","choice":"gpt-6-luna","confidence":0.82,"probabilities":{"gpt-6-luna":0.82,"gpt-6.1-sol":0.18}},` +
		`"complexity":{"type":"score","score":1.4,"confidence":0.6,"legend":{"0":"trivial","1":"moderate","2":"hard"},"probabilities":{"0":0.1,"1":0.4,"2":0.5}}}}`

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "/v1/systemone", r.URL.Path)
		require.Equal(t, "Bearer test-secret", r.Header.Get("Authorization"))
		var raw map[string]json.RawMessage
		require.NoError(t, json.NewDecoder(r.Body).Decode(&raw))
		// Criteria must reach the upstream or choice questions are rejected.
		require.Contains(t, string(raw["questions"]), "criteria")
		_, err := w.Write([]byte(response))
		require.NoError(t, err)
	}))
	defer server.Close()

	answers, status, err := EvaluateQuestions(context.Background(), server.Client(), server.URL, "test-secret",
		Request{Model: JevLatestModel, State: "sample", Questions: questions})
	require.NoError(t, err)
	require.Equal(t, 200, status)
	require.Equal(t, "jev-1.13.0", answers.Model)
	require.Equal(t, 120, answers.Usage.InputTokens)
	require.Equal(t, "gpt-6-luna", answers.Choice["model"].Choice)
	require.InDelta(t, 0.82, answers.Choice["model"].Confidence, 1e-9)
	require.InDelta(t, 0.18, answers.Choice["model"].Probabilities["gpt-6.1-sol"], 1e-9)
	require.InDelta(t, 1.4, answers.Score["complexity"].Score, 1e-9)
	require.Equal(t, "hard", answers.Score["complexity"].Legend["2"])
}

func TestEvaluateQuestionsRejectsBadAnswers(t *testing.T) {
	for _, tc := range []struct {
		name, response string
		// billed: the 2xx body decoded, so usage must still be reported.
		billed bool
	}{
		{"missing answer", `{"model":"jev-1","usage":{"input_tokens":90,"output_tokens":3},"answers":{"model":{"type":"choice","choice":"gpt-6-luna"}}}`, true},
		{"answer type mismatch", `{"model":"jev-1","usage":{"input_tokens":90,"output_tokens":3},"answers":{"model":{"type":"score","score":1},"complexity":{"type":"score","score":1}}}`, true},
		{"choice outside criteria", `{"model":"jev-1","usage":{"input_tokens":90,"output_tokens":3},"answers":{"model":{"type":"choice","choice":"gpt-9-rogue"},"complexity":{"type":"score","score":1}}}`, true},
		{"missing choice", `{"model":"jev-1","usage":{"input_tokens":90,"output_tokens":3},"answers":{"model":{"type":"choice"},"complexity":{"type":"score","score":1}}}`, true},
		{"missing score", `{"model":"jev-1","usage":{"input_tokens":90,"output_tokens":3},"answers":{"model":{"type":"choice","choice":"gpt-6-luna"},"complexity":{"type":"score"}}}`, true},
		{"missing model", `{"answers":{"model":{"type":"choice","choice":"gpt-6-luna"},"complexity":{"type":"score","score":1}}}`, false},
		{"invalid json", `{bad`, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				_, err := w.Write([]byte(tc.response))
				require.NoError(t, err)
			}))
			defer server.Close()
			answers, _, err := EvaluateQuestions(context.Background(), server.Client(), server.URL, "test-secret",
				Request{Model: JevLatestModel, State: "sample", Questions: classifyQuestions()})
			require.Error(t, err)
			if !tc.billed {
				require.Nil(t, answers)
				return
			}
			require.NotNil(t, answers, "a billed response must report its usage")
			require.Equal(t, "jev-1", answers.Model)
			require.Equal(t, Usage{InputTokens: 90, OutputTokens: 3}, answers.Usage)
			require.Empty(t, answers.Choice, "no answer may be trusted from an invalid response")
			require.Empty(t, answers.Score)
		})
	}
}

func TestEvaluateQuestionsLenientUsageAndStructuredLegend(t *testing.T) {
	response := `{"model":"jev-1","usage":{"input_tokens":120.0,"output_tokens":"7"},"answers":{` +
		`"model":{"type":"choice","choice":"gpt-6-luna","confidence":0.5,"probabilities":{"gpt-6-luna":0.5}},` +
		`"complexity":{"type":"score","score":0.4,"confidence":0.5,"legend":{"0":"trivial","1":{"label":"moderate"},"2":["hard"]},"probabilities":{"0":0.6}}}}`
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, err := w.Write([]byte(response))
		require.NoError(t, err)
	}))
	defer server.Close()
	answers, _, err := EvaluateQuestions(context.Background(), server.Client(), server.URL, "test-secret",
		Request{Model: JevLatestModel, State: "sample", Questions: classifyQuestions()})
	require.NoError(t, err)
	require.Equal(t, Usage{InputTokens: 120, OutputTokens: 7}, answers.Usage)
	require.Equal(t, "trivial", answers.Score["complexity"].Legend["0"])
	require.JSONEq(t, `{"label":"moderate"}`, answers.Score["complexity"].Legend["1"])
	require.JSONEq(t, `["hard"]`, answers.Score["complexity"].Legend["2"])
}

// TestEvaluateQuestionsOfficialContractFixtures replays the request and
// response examples published in https://api.typesafe.ai/openapi.json
// (TypeSafe API 0.2.0, fetched 2026-10-21) through the decoder.
func TestEvaluateQuestionsOfficialContractFixtures(t *testing.T) {
	raw, err := os.ReadFile("testdata/systemone_openapi_0.2.0_examples.json")
	require.NoError(t, err)
	var fixtures map[string]struct {
		Request  json.RawMessage `json:"request"`
		Response json.RawMessage `json:"response"`
	}
	require.NoError(t, json.Unmarshal(raw, &fixtures))
	require.Contains(t, fixtures, "choice")
	require.Contains(t, fixtures, "score")

	for name, fixture := range fixtures {
		t.Run(name, func(t *testing.T) {
			var wireRequest struct {
				Model     string `json:"model"`
				State     string `json:"state"`
				Questions map[string]struct {
					Type         string          `json:"type"`
					Instructions string          `json:"instructions"`
					Criteria     json.RawMessage `json:"criteria"`
				} `json:"questions"`
			}
			require.NoError(t, json.Unmarshal(fixture.Request, &wireRequest))
			// Rebuild the request the way the gateway does, then check it
			// serializes to the published example exactly.
			request := Request{Model: wireRequest.Model, State: wireRequest.State, Questions: map[string]Question{}}
			for id, question := range wireRequest.Questions {
				var criteria any
				switch question.Type {
				case QuestionTypeChoice:
					var choices map[string]string
					require.NoError(t, json.Unmarshal(question.Criteria, &choices))
					criteria = choices
				case QuestionTypeScore:
					var levels []string
					require.NoError(t, json.Unmarshal(question.Criteria, &levels))
					criteria = levels
				}
				request.Questions[id] = Question{Type: question.Type, Instructions: question.Instructions, Criteria: criteria}
			}
			encoded, err := json.Marshal(request)
			require.NoError(t, err)
			require.JSONEq(t, string(fixture.Request), string(encoded))

			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				body, err := io.ReadAll(r.Body)
				require.NoError(t, err)
				require.JSONEq(t, string(fixture.Request), string(body))
				_, err = w.Write(fixture.Response)
				require.NoError(t, err)
			}))
			defer server.Close()
			answers, status, err := EvaluateQuestions(context.Background(), server.Client(), server.URL, "test-secret", request)
			require.NoError(t, err)
			require.Equal(t, http.StatusOK, status)
			require.Equal(t, "jev-latest", answers.Model)
			require.Equal(t, Usage{InputTokens: 120, OutputTokens: 12}, answers.Usage)
			switch name {
			case "choice":
				tone := answers.Choice["tone"]
				require.Equal(t, "calm", tone.Choice)
				require.InDelta(t, 0.7, tone.Confidence, 1e-9)
				require.InDelta(t, 0.8, tone.Probabilities["calm"], 1e-9)
				require.Len(t, tone.Probabilities, 3)
			case "score":
				urgency := answers.Score["urgency"]
				require.InDelta(t, 1.7, urgency.Score, 1e-9)
				require.InDelta(t, 0.55, urgency.Confidence, 1e-9)
				require.Equal(t, "Needs attention today", urgency.Legend["2"])
				require.InDelta(t, 0.8, urgency.Probabilities["2"], 1e-9)
			}
		})
	}
}

func TestEvaluateQuestionsDegradesBadConfidence(t *testing.T) {
	// Confidence is advisory metadata; a bad value must not discard an answer
	// the upstream already charged for.
	response := `{"model":"jev-1","answers":{"model":{"type":"choice","choice":"gpt-6-luna","confidence":9,"probabilities":{"gpt-6-luna":5}},"complexity":{"type":"score","score":1,"confidence":-1}}}`
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, err := w.Write([]byte(response))
		require.NoError(t, err)
	}))
	defer server.Close()
	answers, _, err := EvaluateQuestions(context.Background(), server.Client(), server.URL, "test-secret",
		Request{Model: JevLatestModel, State: "sample", Questions: classifyQuestions()})
	require.NoError(t, err)
	require.Zero(t, answers.Choice["model"].Confidence)
	require.Nil(t, answers.Choice["model"].Probabilities)
	require.Zero(t, answers.Score["complexity"].Confidence)
}

func TestEvaluateQuestionsDoesNotExposePayload(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, err := w.Write([]byte("private prompt text and test-secret"))
		require.NoError(t, err)
	}))
	defer server.Close()
	_, status, err := EvaluateQuestions(context.Background(), server.Client(), server.URL, "test-secret",
		Request{Model: JevLatestModel, State: "private prompt text", Questions: classifyQuestions()})
	require.Equal(t, http.StatusUnauthorized, status)
	require.Error(t, err)
	require.NotContains(t, err.Error(), "private")
	require.NotContains(t, err.Error(), "test-secret")
}
