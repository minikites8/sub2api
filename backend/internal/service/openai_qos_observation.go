package service

import (
	"context"
	"fmt"
)

// MarkGPTQoSSubmission is called at real transport submission boundaries, so
// retries are attempts of one request and queue polls are never model attempts.
func MarkGPTQoSSubmission(ctx context.Context) error {
	state, _ := ctx.Value(gptQoSRequestKey{}).(*gptQoSRequestState)
	if state == nil {
		return nil
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	n := state.attempts.Add(1)
	if state.config.Enabled && int(n) > state.config.MaxAttempts {
		state.attempts.Add(-1)
		return fmt.Errorf("%w: upstream attempt budget exhausted", ErrGPTQoSUnavailable)
	}
	state.mu.Lock()
	// Attribute one request to its last submitted account, not a later selection
	// that might fail admission before any upstream transport is attempted.
	state.submittedAccountType = state.accountType
	state.outcomeKnown = true
	state.lastSucceeded = false
	state.mu.Unlock()
	return nil
}
func ObserveGPTQoSOutcome(ctx context.Context, success bool) {
	if state, _ := ctx.Value(gptQoSRequestKey{}).(*gptQoSRequestState); state != nil {
		state.mu.Lock()
		state.outcomeKnown = true
		state.lastSucceeded = success
		state.mu.Unlock()
	}
}
func ObserveGPTQoSForwardResult(ctx context.Context, result *OpenAIForwardResult) {
	if result == nil {
		return
	}
	if state, _ := ctx.Value(gptQoSRequestKey{}).(*gptQoSRequestState); state != nil {
		state.mu.Lock()
		defer state.mu.Unlock()
		if result.UpstreamModel != "" {
			state.effectiveModel = truncateQoSLabel(result.UpstreamModel, 128)
		}
		if result.ReasoningEffort != nil && *result.ReasoningEffort != "" {
			state.effort = truncateQoSLabel(*result.ReasoningEffort, 16)
		}
	}
}
