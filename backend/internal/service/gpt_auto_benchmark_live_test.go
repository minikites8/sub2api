package service

import (
	"context"
	"net/http"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// TestGPTAutoLiveJevBenchmark sends the synthetic benchmark (no customer
// data) to the real TypeSafe System One API and prints the resulting model,
// effort, complexity and category distribution next to the expectations. It
// is a diagnostic, not a gate: it asserts only that calls succeed.
//
// Opt-in only, because it spends real Jev credit (28 calls):
//
//	GPT_AUTO_LIVE_JEV_KEY=... go test ./internal/service -run TestGPTAutoLiveJevBenchmark -v
func TestGPTAutoLiveJevBenchmark(t *testing.T) {
	key := os.Getenv("GPT_AUTO_LIVE_JEV_KEY")
	if key == "" {
		t.Skip("set GPT_AUTO_LIVE_JEV_KEY to run the live Jev benchmark")
	}
	cfg := DefaultGPTAutoShadowConfig()
	cfg.ClassifierAPIKey = key
	if base := os.Getenv("GPT_AUTO_LIVE_JEV_BASE_URL"); base != "" {
		cfg.ClassifierBaseURL = base
	}
	classifier := NewTypeSafeGPTAutoClassifier(func(context.Context, GPTAutoShadowConfig) (*http.Client, error) {
		return &http.Client{Timeout: 30 * time.Second}, nil
	})
	models, tierHits, categoryHits := map[string]int{}, 0, 0
	samples := GPTAutoBenchmarkSamples()
	for _, sample := range samples {
		snapshot := buildGPTAutoTestSnapshot(*cfg, GPTAutoTestInput{Prompt: sample.Prompt, Context: sample.Context, Endpoint: GPTAutoEndpointChatCompletions, HasTools: sample.HasTools})
		cls, err := classifier.Classify(context.Background(), *cfg, snapshot)
		require.NoError(t, err, sample.ID)
		models[cls.Model]++
		tier := "luna"
		if cls.Model == GPTAutoModelGPT56Sol || cls.Model == GPTAutoModelGPT61Sol {
			tier = "sol"
		}
		if tier == sample.ExpectedTier {
			tierHits++
		}
		if cls.Category == sample.ExpectedCategory {
			categoryHits++
		}
		t.Logf("%-10s expected=%-4s got=%-13s p=%.2f effort=%-6s complexity=%.2f (band %.0f-%.0f) category=%s",
			sample.ID, sample.ExpectedTier, cls.Model, cls.ModelConfidence, cls.Effort, cls.Complexity,
			sample.ExpectedComplexity[0], sample.ExpectedComplexity[1], cls.Category)
	}
	t.Logf("model distribution: %v", models)
	t.Logf("tier agreement: %d/%d, category agreement: %d/%d", tierHits, len(samples), categoryHits, len(samples))
}
