package handler

import (
	"errors"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// Retry inside the selected-account lease, never by re-entering the scheduler.
// A partial result may represent metered work even when nothing was delivered.
func forwardOpenAIWithDefaultRetry(c *gin.Context, account *service.Account, forward func() (*service.OpenAIForwardResult, error), retryBudgets ...*retryWaitBudget) (*service.OpenAIForwardResult, error) {
	if !service.OpenAIDefaultRetryPolicy(account) {
		return forward()
	}
	budget := &retryWaitBudget{}
	if len(retryBudgets) > 0 && retryBudgets[0] != nil {
		budget = retryBudgets[0]
	}
	originalRequest := c.Request
	ctx, submissionCount := service.WithOpenAIDefaultRetryObservation(c.Request.Context())
	c.Request = c.Request.WithContext(ctx)
	service.SetOpenAIDefaultRetryGuard(ctx, func() bool {
		return !requestPrefersAccountRetry(c) && !failoverClientGone(c) && !service.OpenAIAgentRecoveryNoReplay(ctx) &&
			!c.Writer.Written() && !service.IsResponseCommitted(c) && !strings.HasPrefix(service.GetActualOpenAIUpstreamEndpoint(c), "/basispoints/")
	})
	defer func() { service.FinishOpenAIDefaultRetryState(ctx, true) }()

	defer func() { c.Request = originalRequest }()
	result, err := forward()
	if !service.OpenAIDefaultRetryPolicy(account) {
		return result, err
	}
	var failure *service.UpstreamFailoverError
	if service.OpenAIDefaultRetryHasNoWork(result) && submissionCount() == 1 && !strings.HasPrefix(service.GetActualOpenAIUpstreamEndpoint(c), "/basispoints/") && errors.As(err, &failure) && service.OpenAIDefaultFailureCanRetry(account, failure) &&
		!service.OpenAIAgentRecoveryNoReplay(c.Request.Context()) && !requestPrefersAccountRetry(c) && !failoverClientGone(c) &&
		openAIForwardMayFailover(c, service.OpenAICompactKeepaliveAdjustedWrittenSize(c), failure) {
		// Default retries never replay after any byte has been committed, including
		// heartbeats. A broader after-write opt-in must not weaken this default.
		if !failure.IsExcelBPSImageAdmissionRejection() && !c.Writer.Written() && !service.IsResponseCommitted(c) {
			if delay, allowed := openAIDefaultRetryWait(failure, time.Now()); allowed && budget.allowsSameAccountRetry(ctx, account.ID, failure, delay) && budget.wait(ctx, delay) {
				service.BeginOpenAIDefaultRetryState(ctx)
				result, err = forward()
				service.FinishOpenAIDefaultRetryState(ctx, err != nil)
			}
		}
	}
	// This selected-account attempt already owns the complete default budget.
	// Prevent the outer handler from reselecting under a same-account label or
	// allowing an OAuth deadline to override the single-resubmission cap.
	if errors.As(err, &failure) {
		copied := *failure
		copied.RetryableOnSameAccount = false
		copied.SameAccountRetryMax = 1
		err = &copied
	}
	if err == nil {
		service.FinishOpenAIDefaultRetryState(ctx, false)
	}
	return result, err
}

func openAIDefaultRetryWait(failure *service.UpstreamFailoverError, now time.Time) (time.Duration, bool) {
	if failure == nil {
		return 0, false
	}
	delay := sameAccountRetryDelayFor(failure, 1)
	if hint := strings.TrimSpace(failure.ResponseHeaders.Get("Retry-After")); hint != "" {
		var hinted time.Duration
		if seconds, err := strconv.ParseUint(hint, 10, 32); err == nil {
			hinted = time.Duration(seconds) * time.Second
		} else if until, err := http.ParseTime(hint); err == nil {
			hinted = until.Sub(now)
		}
		if hinted > delay {
			delay = hinted
		}
	}
	if delay > maxRequestRetryWait || !failure.SameAccountRetryDeadline.IsZero() && !now.Add(delay).Before(failure.SameAccountRetryDeadline) {
		return 0, false
	}
	return delay, true
}
