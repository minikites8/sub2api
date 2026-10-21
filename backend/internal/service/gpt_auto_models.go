package service

import (
	"sort"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/pkg/openai"
)

// GPT-Auto Phase 1 operates in shadow mode only: nothing in this file is
// allowed to influence a live customer request. The identifiers below are the
// fixed Phase 1 candidate pool.
const (
	GPTAutoModelGPT56Luna = "gpt-5.6-luna"
	GPTAutoModelGPT56Sol  = "gpt-5.6-sol"
	GPTAutoModelGPT6Luna  = "gpt-6-luna"
	GPTAutoModelGPT61Sol  = "gpt-6.1-sol"
)

// GPTAutoModelPool is the ordered Phase 1 candidate pool.
func GPTAutoModelPool() []string {
	return []string{GPTAutoModelGPT56Luna, GPTAutoModelGPT56Sol, GPTAutoModelGPT6Luna, GPTAutoModelGPT61Sol}
}

// Reasoning effort candidates Jev may choose from.
const (
	GPTAutoEffortNone   = "none"
	GPTAutoEffortLow    = "low"
	GPTAutoEffortMedium = "medium"
	GPTAutoEffortHigh   = "high"
	GPTAutoEffortXHigh  = "xhigh"
	GPTAutoEffortMax    = "max"
)

// GPTAutoEffortLadder is ordered from cheapest to most expensive. Policy
// downgrades walk this ladder backwards to find the nearest supported level.
func GPTAutoEffortLadder() []string {
	return []string{
		GPTAutoEffortNone, GPTAutoEffortLow, GPTAutoEffortMedium,
		GPTAutoEffortHigh, GPTAutoEffortXHigh, GPTAutoEffortMax,
	}
}

func gptAutoEffortRank(effort string) int {
	for i, value := range GPTAutoEffortLadder() {
		if value == effort {
			return i
		}
	}
	return -1
}

// IsGPTAutoModel reports whether a model id belongs to the Phase 1 pool.
func IsGPTAutoModel(model string) bool {
	switch strings.ToLower(strings.TrimSpace(model)) {
	case GPTAutoModelGPT56Luna, GPTAutoModelGPT56Sol, GPTAutoModelGPT6Luna, GPTAutoModelGPT61Sol:
		return true
	default:
		return false
	}
}

// IsGPTAutoEffort reports whether a reasoning effort is a known candidate.
func IsGPTAutoEffort(effort string) bool {
	return gptAutoEffortRank(strings.ToLower(strings.TrimSpace(effort))) >= 0
}

// gptAutoNativeSupportedEfforts returns the reasoning efforts the gateway's
// existing OpenAI-compatible request path actually accepts for a model.
//
// This mirrors the live behaviour rather than restating a vendor datasheet:
//   - "max" survives as a distinct level only for families that
//     supportsOpenAIReasoningEffortMax accepts; everything else is collapsed to
//     "xhigh" by normalizeOpenAIReasoningEffort.
//   - gpt-6.1-sol rejects "none"/"minimal" outright in
//     openai.ValidateGPT61SolReasoningEffort, so disabled reasoning is not a
//     legal request for it on the compatibility paths.
//
// Admins may narrow this further per model, but never widen it: the policy
// engine intersects the configured set with this one.
func gptAutoNativeSupportedEfforts(model string) []string {
	efforts := make([]string, 0, len(GPTAutoEffortLadder()))
	for _, effort := range GPTAutoEffortLadder() {
		if gptAutoModelSupportsEffort(model, effort) {
			efforts = append(efforts, effort)
		}
	}
	return efforts
}

func gptAutoModelSupportsEffort(model, effort string) bool {
	switch effort {
	case GPTAutoEffortNone:
		// Reuses the gateway's own rejection rule so the shadow matrix cannot
		// drift from what a real request would be allowed to send.
		return openai.ValidateGPT61SolReasoningEffort(model, GPTAutoEffortNone) == nil
	case GPTAutoEffortMax:
		return gptAutoModelSupportsMaxEffort(model)
	case GPTAutoEffortLow, GPTAutoEffortMedium, GPTAutoEffortHigh, GPTAutoEffortXHigh:
		return true
	default:
		return false
	}
}

// gptAutoModelSupportsMaxEffort mirrors supportsOpenAIReasoningEffortMax for
// the Phase 1 pool. All four models are GPT-5.6/GPT-6 family members, which
// that function accepts; keeping the check explicit documents the dependency
// and fails closed if the pool ever grows.
func gptAutoModelSupportsMaxEffort(model string) bool {
	return supportsOpenAIReasoningEffortMax(model)
}

// gptAutoSortEfforts orders a set by the ladder so stored and displayed values
// are stable regardless of admin input order.
func gptAutoSortEfforts(efforts []string) []string {
	out := make([]string, 0, len(efforts))
	seen := make(map[string]struct{}, len(efforts))
	for _, effort := range efforts {
		normalized := strings.ToLower(strings.TrimSpace(effort))
		if gptAutoEffortRank(normalized) < 0 {
			continue
		}
		if _, dup := seen[normalized]; dup {
			continue
		}
		seen[normalized] = struct{}{}
		out = append(out, normalized)
	}
	sort.Slice(out, func(i, j int) bool {
		return gptAutoEffortRank(out[i]) < gptAutoEffortRank(out[j])
	})
	return out
}

// Task categories Jev may assign. Stored verbatim on the decision row.
const (
	GPTAutoCategoryGeneral  = "general_conversation"
	GPTAutoCategoryWriting  = "writing_transformation"
	GPTAutoCategoryCoding   = "coding_debugging"
	GPTAutoCategoryMath     = "mathematical_reasoning"
	GPTAutoCategoryResearch = "research_analysis"
	GPTAutoCategoryAgentic  = "agent_tool_workflow"
)

// GPTAutoCategories returns the ordered task-category candidates.
func GPTAutoCategories() []string {
	return []string{
		GPTAutoCategoryGeneral, GPTAutoCategoryWriting, GPTAutoCategoryCoding,
		GPTAutoCategoryMath, GPTAutoCategoryResearch, GPTAutoCategoryAgentic,
	}
}
