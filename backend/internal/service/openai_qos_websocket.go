package service

import (
	"context"
	"sync"
	"time"

	"github.com/tidwall/gjson"
)

// A WebSocket deadline belongs to a response.create turn, never idle connection
// time. Expiry stops the connection; a successful turn stops only its timer.
func (s *OpenAIGatewayService) PrepareGPTQoSWebSocket(ctx context.Context, groupID *int64, body []byte) (context.Context, func(bool)) {
	cfg := s.gptQoSConfigForContext(ctx)
	if !cfg.QualityRoutingEnabled || !cfg.Applies(groupID) {
		return ctx, func(bool) {}
	}
	ctx, cancel := context.WithCancelCause(ctx)
	state := &gptQoSRequestState{config: cfg, group: cfg.Group(groupID)}
	ctx = context.WithValue(ctx, gptQoSRequestKey{}, state)
	ctx = context.WithValue(ctx, gptQoSGroupIDKey{}, derefGroupID(groupID))
	ctx = context.WithValue(ctx, gptQoSCancelKey{}, context.CancelFunc(func() { cancel(ErrGPTQoSLeaseLost) }))
	ctx = context.WithValue(ctx, gptQoSBudgetCancelKey{}, context.CancelFunc(func() { cancel(ErrGPTQoSRequestDeadline) }))
	finish := s.BeginGPTQoSWebSocketTurn(ctx, groupID, body)
	return ctx, finish
}
func (s *OpenAIGatewayService) BeginGPTQoSWebSocketTurn(ctx context.Context, groupID *int64, body []byte) func(bool) {
	state, _ := ctx.Value(gptQoSRequestKey{}).(*gptQoSRequestState)
	if state == nil {
		return func(bool) {}
	}
	now := time.Now()
	state.mu.Lock()
	state.started = now
	state.strongDeadline = time.Time{}
	state.attempts.Store(0)
	state.selectionAttempts.Store(0)
	state.outcomeKnown = false
	state.lastSucceeded = false
	state.submittedAccountType = ""
	state.queueWait = 0
	state.selectionDuration = 0
	state.accountID = 0
	state.quality = ""
	state.pool = ""
	state.fallback = ""
	state.sticky = ""
	state.downgraded = false
	state.peak = false
	state.model = truncateQoSLabel(gjson.GetBytes(body, "model").String(), 128)
	state.effort = gjson.GetBytes(body, "reasoning.effort").String()
	budget := state.config.RequestBudgetMs
	if state.effort == "high" || state.effort == "xhigh" || state.effort == "max" {
		budget = state.config.HighEffortBudgetMs
	}
	state.mu.Unlock()
	var timer *time.Timer
	if state.config.Enabled {
		if cancel, _ := ctx.Value(gptQoSBudgetCancelKey{}).(context.CancelFunc); cancel != nil {
			timer = time.AfterFunc(time.Duration(budget)*time.Millisecond, cancel)
		}
	}
	var once sync.Once
	return func(success bool) {
		once.Do(func() {
			if timer != nil {
				timer.Stop()
			}
			s.finishGPTQoSRequest(ctx, groupID, state, success)
		})
	}
}
