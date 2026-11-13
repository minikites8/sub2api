package service

import (
	"context"
	"net/http"
	"reflect"
	"strings"
	"sync"
	"sync/atomic"
)

// OpenAIDefaultRetryPolicy applies only when an OpenAI API-key pool has no
// explicit retry override. Other providers and configured pool behavior retain
// their existing policy.
func OpenAIDefaultRetryPolicy(account *Account) bool {
	if account == nil || account.Platform != PlatformOpenAI {
		return false
	}
	if account.Type == AccountTypeAPIKey && account.IsPoolMode() {
		for _, key := range []string{"pool_mode_retry_count", "pool_mode_retry_status_codes"} {
			if value, exists := account.Credentials[key]; exists && value != nil {
				return false
			}
		}
	}
	return true
}

// OpenAIDefaultFailureCanRetry narrows the default to explicit rejection before
// output. The caller separately rejects usage, cancellation and committed writes.
func OpenAIDefaultFailureCanRetry(account *Account, failure *UpstreamFailoverError) bool {
	capacity := failure != nil && failure.StatusCode >= 500 && failure.RequestScopedTransient &&
		isOpenAIRequestScopedCapacityResponse(failure.StatusCode, failure.ResponseHeaders, failure.ResponseBody)
	if !OpenAIDefaultRetryPolicy(account) || failure == nil || !failure.ShouldRetryNextAccount() ||
		failure.IsCredentialFailure() || failure.Scope == GatewayFailureScopeRequest && !capacity || failure.RequestScopedTransient && !capacity ||
		failure.HeaderTimeoutSubmitted || failure.Reason == OpenAITransportFailureReason || failure.Reason == OpenAIFirstOutputTimeoutReason || failure.Scope == excelBPSFailureScope || failure.Reason == ExcelBPSRateLimitedReason ||
		strings.EqualFold(strings.TrimSpace(failure.ResponseHeaders.Get("X-Should-Retry")), "false") {
		return false
	}
	classificationStatus := failure.StatusCode
	if classificationStatus == http.StatusTooManyRequests {
		classificationStatus = http.StatusBadGateway
	}
	if !excelBPSReplayableRejection(account, classificationStatus, failure.ResponseBody) {
		return false
	}
	// Authentication and authorization failures retain their existing explicit
	// classification. A transient OAuth 429 may carry a deadline, but only one
	// resubmission is permitted for this request.
	if failure.StatusCode < http.StatusInternalServerError {
		return failure.RetryableOnSameAccount
	}
	return true
}

type openAIDefaultRetryObservationKey struct{}

// The counter includes service-internal compatibility and compaction sends.
// An outer retry is allowed only after exactly one rejected submission.
func WithOpenAIDefaultRetryObservation(ctx context.Context) (context.Context, func() int64) {
	state := &openAIDefaultRetryObservation{}
	return context.WithValue(ctx, openAIDefaultRetryObservationKey{}, state), state.submissions.Load
}
func ObserveOpenAIDefaultRetrySubmission(ctx context.Context) {
	if state, _ := ctx.Value(openAIDefaultRetryObservationKey{}).(*openAIDefaultRetryObservation); state != nil {
		state.submissions.Add(1)
	}
}

// State changes are deferred only while a single, explicitly rejected native
// inference remains eligible for the one in-place retry. No authorization check
// or revision comparison is bypassed on the next send.
type openAIDefaultRetryObservation struct {
	submissions atomic.Int64
	mu          sync.Mutex
	finished    bool
	guard       func() bool
	pending     func() bool
	projection  *Account
}

func SetOpenAIDefaultRetryGuard(ctx context.Context, guard func() bool) {
	if state, _ := ctx.Value(openAIDefaultRetryObservationKey{}).(*openAIDefaultRetryObservation); state != nil {
		state.guard = guard
	}
}
func BeginOpenAIDefaultRetryState(ctx context.Context) {
	if state, _ := ctx.Value(openAIDefaultRetryObservationKey{}).(*openAIDefaultRetryObservation); state != nil {
		state.mu.Lock()
		state.finished = true
		state.mu.Unlock()
	}
}
func openAIDefaultRetryPinnedConnection(ctx context.Context) *Account {
	state, _ := ctx.Value(openAIDefaultRetryObservationKey{}).(*openAIDefaultRetryObservation)
	if state == nil {
		return nil
	}
	state.mu.Lock()
	defer state.mu.Unlock()
	if state.finished {
		return state.projection
	}
	return nil
}
func observeOpenAIDefaultRetryConnection(ctx context.Context, account *Account) {
	state, _ := ctx.Value(openAIDefaultRetryObservationKey{}).(*openAIDefaultRetryObservation)
	if state == nil || account == nil {
		return
	}
	state.mu.Lock()
	defer state.mu.Unlock()
	if !state.finished && state.submissions.Load() == 0 {
		snapshot := *account
		state.projection = &snapshot
	}
}
func FinishOpenAIDefaultRetryState(ctx context.Context, apply bool) {
	state, _ := ctx.Value(openAIDefaultRetryObservationKey{}).(*openAIDefaultRetryObservation)
	if state == nil {
		return
	}
	state.mu.Lock()
	state.finished = true
	pending := state.pending
	state.pending = nil
	state.mu.Unlock()
	if apply && pending != nil {
		pending()
	}
}
func (s *OpenAIGatewayService) deferOpenAIDefaultRetryState(ctx context.Context, account *Account, status int, headers http.Header, body []byte, models ...string) bool {
	state, _ := ctx.Value(openAIDefaultRetryObservationKey{}).(*openAIDefaultRetryObservation)
	if state != nil && state.submissions.Load() > 1 {
		state.mu.Lock()
		state.pending = nil
		state.mu.Unlock()
		return false
	}
	if state == nil || state.submissions.Load() != 1 || ctx.Err() != nil || !OpenAIDefaultRetryPolicy(account) ||
		OpenAIAgentRecoveryNoReplay(ctx) || isQualityObservation(ctx) || isControlledExperiment(ctx) ||
		strings.EqualFold(strings.TrimSpace(headers.Get("X-Should-Retry")), "false") ||
		isOpenAIHTTPUpstreamAccessStateError(status, "", body) || isOpenAIRequestScopedCapacityResponse(status, headers, body) {
		return false
	}
	eligible := status >= 500 && excelBPSReplayableRejection(account, status, body)
	if status == http.StatusTooManyRequests {
		eligible = s.shouldRetryOpenAIOAuth429OnSameAccountWithResponse(account, status, false, headers, body)
	}
	if !eligible {
		return false
	}
	state.mu.Lock()
	defer state.mu.Unlock()
	if state.finished || state.pending != nil || state.guard == nil || !state.guard() {
		return false
	}
	savedBody := append([]byte(nil), body...)
	savedHeaders := headers.Clone()
	savedModels := append([]string(nil), models...)
	state.pending = func() bool {
		return s.handleOpenAIAccountUpstreamError(ctx, account, status, savedHeaders, savedBody, savedModels...)
	}
	return true
}

// OpenAIDefaultRetryHasNoWork permits conversion metadata while conservatively
// retaining every result that may represent generated content or billable work.
func OpenAIDefaultRetryHasNoWork(result *OpenAIForwardResult) bool {
	if result == nil {
		return true
	}
	return reflect.DeepEqual(result.Usage, OpenAIUsage{}) &&
		!result.RegionalMetered && !result.ObservedUsageOnError && !result.ClientDisconnect &&
		!result.UsageUnavailable && !result.streamReadIncomplete &&
		result.FirstActualContentMs == nil && result.FirstActualContentKind == "" &&
		result.HeaderTimeoutRetries == 0 && result.ImageCount == 0 && result.VideoCount == 0 &&
		result.WebSearchCalls == 0 && result.SearchCount == 0 && result.AudioUsage == nil && result.MiniMaxH3BaseCost == nil
}
