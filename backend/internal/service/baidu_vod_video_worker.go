package service

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/logger"
)

const (
	baiduVODPollLease       = 2 * time.Minute
	baiduVODTaskTimeout     = 30 * time.Minute
	baiduVODSeedanceTimeout = 72 * time.Hour
	baiduVODWorkerInterval  = 3 * time.Second
	baiduVODPollMaxAttempts = 12
)

type BaiduVODVideoWorker struct {
	service *BaiduVODVideoService
	stop    chan struct{}
	done    chan struct{}
	mu      sync.Mutex
}

func NewBaiduVODVideoWorker(service *BaiduVODVideoService) *BaiduVODVideoWorker {
	return &BaiduVODVideoWorker{service: service}
}

func (w *BaiduVODVideoWorker) Start() {
	if w == nil || w.service == nil || w.service.tasks == nil {
		return
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.stop != nil {
		return
	}
	w.stop = make(chan struct{})
	w.done = make(chan struct{})
	go w.run(w.stop, w.done)
}

func (w *BaiduVODVideoWorker) Stop() {
	if w == nil {
		return
	}
	w.mu.Lock()
	stop, done := w.stop, w.done
	w.stop, w.done = nil, nil
	w.mu.Unlock()
	if stop != nil {
		close(stop)
	}
	if done != nil {
		<-done
	}
}

func (w *BaiduVODVideoWorker) run(stop <-chan struct{}, done chan<- struct{}) {
	defer close(done)
	ticker := time.NewTicker(baiduVODWorkerInterval)
	defer ticker.Stop()
	for {
		select {
		case <-stop:
			return
		case <-ticker.C:
			w.processDue(context.Background())
		}
	}
}

func (w *BaiduVODVideoWorker) processDue(ctx context.Context) {
	now := time.Now()
	tasks, err := w.service.tasks.ListDue(ctx, now, 25)
	if err != nil {
		return
	}
	for _, task := range tasks {
		claimed, claimErr := w.service.tasks.Claim(ctx, task.TaskID, task.Version, now.Add(baiduVODPollLease))
		if claimErr != nil || !claimed {
			continue
		}
		task.Version++
		w.processOne(ctx, task)
	}
}

func (w *BaiduVODVideoWorker) processOne(ctx context.Context, task *BaiduVODVideoTask) {
	if task == nil {
		return
	}
	if task.Status == BaiduVODTaskStatusSubmitting && strings.TrimSpace(task.UpstreamTaskID) == "" {
		w.fail(ctx, task, "SUBMISSION_INTERRUPTED", "Baidu VOD task submission did not complete")
		return
	}
	taskTimeout := baiduVODTaskTimeout
	if task.Provider == BaiduVODProviderSeedance {
		taskTimeout = baiduVODSeedanceTimeout
	}
	if time.Since(task.SubmittedAt) > taskTimeout {
		if w.service.Release(ctx, task) == nil {
			finished := time.Now()
			settled := finished
			_, _ = w.service.tasks.UpdatePoll(ctx, task.TaskID, BaiduVODVideoTaskPollUpdate{Version: task.Version, Status: BaiduVODTaskStatusFailed,
				UpstreamStatus: "TIMEOUT", LastErrorCode: baiduVODStringPtr("TASK_TIMEOUT"), LastErrorMessage: baiduVODStringPtr("Baidu VOD task exceeded the maximum polling time"),
				NextPollAt: finished, RetryCount: task.RetryCount, FinishedAt: &finished, SettledAt: &settled})
		}
		return
	}
	account, err := w.service.accounts.GetByID(ctx, task.AccountID)
	if err != nil || account == nil {
		w.retry(ctx, task, "ACCOUNT_UNAVAILABLE", "the account bound to this task is unavailable")
		return
	}
	pollCtx, cancel := context.WithTimeout(ctx, 90*time.Second)
	response, err := w.service.Poll(pollCtx, account, task)
	cancel()
	if err != nil {
		var upstreamErr *BaiduVODUpstreamError
		if errors.As(err, &upstreamErr) && upstreamErr.StatusCode >= 400 && upstreamErr.StatusCode < 500 && upstreamErr.StatusCode != httpStatusTooManyRequests {
			w.fail(ctx, task, upstreamErr.Code, upstreamErr.Message)
			return
		}
		w.retry(ctx, task, "UPSTREAM_POLL_FAILED", err.Error())
		return
	}
	status := strings.ToUpper(strings.TrimSpace(response.Output.TaskStatus))
	switch status {
	case "SUCCEEDED":
		w.succeed(ctx, task, response)
	case "FAILED", "UNKNOWN", "CANCELED", "CANCELLED", "EXPIRED":
		code := response.Output.Code
		message := response.Output.Message
		if status == "UNKNOWN" && code == "" {
			code = "TASK_UNKNOWN"
		}
		if message == "" {
			message = "Baidu VOD task ended with status " + status
		}
		w.fail(ctx, task, code, message)
	default:
		w.pending(ctx, task, status)
	}
}

const httpStatusTooManyRequests = 429

func (w *BaiduVODVideoWorker) retry(ctx context.Context, task *BaiduVODVideoTask, code, message string) {
	finished := time.Now()
	attempt := task.RetryCount + 1
	if attempt > baiduVODPollMaxAttempts {
		w.fail(ctx, task, "POLL_RETRY_EXHAUSTED", message)
		return
	}
	delay := time.Duration(attempt*5) * time.Second
	if delay > 30*time.Second {
		delay = 30 * time.Second
	}
	_, _ = w.service.tasks.UpdatePoll(ctx, task.TaskID, BaiduVODVideoTaskPollUpdate{Version: task.Version, Status: BaiduVODTaskStatusInProgress,
		UpstreamStatus: code, LastErrorCode: baiduVODStringPtr(code), LastErrorMessage: baiduVODStringPtr(message), NextPollAt: finished.Add(delay), RetryCount: attempt})
}

func (w *BaiduVODVideoWorker) pending(ctx context.Context, task *BaiduVODVideoTask, status string) {
	if status == "" {
		status = "PENDING"
	}
	now := time.Now()
	_, _ = w.service.tasks.UpdatePoll(ctx, task.TaskID, BaiduVODVideoTaskPollUpdate{
		Version: task.Version, Status: BaiduVODTaskStatusInProgress, UpstreamStatus: status,
		NextPollAt: now.Add(10 * time.Second), RetryCount: task.RetryCount,
	})
}

func (w *BaiduVODVideoWorker) succeed(ctx context.Context, task *BaiduVODVideoTask, response *BaiduVODTaskResponse) {
	if response == nil || strings.TrimSpace(response.Output.VideoURL) == "" {
		w.fail(ctx, task, "RESULT_URL_MISSING", "Baidu VOD returned SUCCEEDED without video_url")
		return
	}
	outputDuration := task.RequestedDuration
	videoCount := 1
	if response.Usage != nil {
		if response.Usage.OutputVideoDuration > 0 {
			outputDuration = response.Usage.OutputVideoDuration
		}
		if response.Usage.Duration > 0 && response.Usage.OutputVideoDuration <= 0 {
			outputDuration = response.Usage.Duration
		}
		if response.Usage.VideoCount > 0 {
			videoCount = response.Usage.VideoCount
		}
	}
	actualResolution := task.Resolution
	completionTokens := 0
	if response.Usage != nil {
		completionTokens = response.Usage.CompletionTokens
		if strings.TrimSpace(response.Usage.Resolution) != "" {
			actualResolution = NormalizeVideoBillingResolutionOrDefault(response.Usage.Resolution)
		} else if response.Usage.SR > 0 {
			actualResolution = NormalizeVideoBillingResolutionOrDefault(strconv.Itoa(response.Usage.SR))
		}
	}
	actual := task.EstimatedCost
	billingMode := firstNonEmpty(task.BillingMode, string(BillingModeVideo))
	if (billingMode == string(BillingModeToken) || billingMode == string(BillingModeVideoToken)) && completionTokens > 0 && w.service.billing != nil {
		cost := w.service.calculateVideoCost(ctx, task.Model, task.GroupID, actualResolution, videoCount, outputDuration, completionTokens, task.InputContainsVideo, nil, false, task.VideoRateMultiplier)
		actual = cost.ActualCost
		billingMode = firstNonEmpty(cost.BillingMode, billingMode)
	} else if NormalizeVideoBillingResolutionOrDefault(actualResolution) == NormalizeVideoBillingResolutionOrDefault(task.Resolution) && task.RequestedDuration > 0 && outputDuration > 0 {
		actual = task.EstimatedCost * float64(outputDuration) / float64(task.RequestedDuration) * float64(videoCount)
	} else if w.service.billing != nil {
		cost := w.service.calculateVideoCost(ctx, task.Model, task.GroupID, actualResolution, videoCount, outputDuration, 0, task.InputContainsVideo, nil, false, task.VideoRateMultiplier)
		actual = cost.ActualCost
		billingMode = firstNonEmpty(cost.BillingMode, billingMode)
	}
	if actual-task.HoldAmount > 0.00000001 {
		w.fail(ctx, task, "ACTUAL_COST_EXCEEDS_HOLD", "actual video duration exceeds the reserved balance")
		return
	}
	resultURL := strings.TrimSpace(response.Output.VideoURL)
	expires := ParseBaiduVODVideoURLExpiry(resultURL)
	if w.service.mediaStore != nil {
		archiveCtx, cancel := context.WithTimeout(ctx, 15*time.Minute)
		archivedURL, archiveErr := w.service.mediaStore.Archive(archiveCtx, resultURL, task.TaskID, task.CreatedAt)
		cancel()
		if archiveErr != nil {
			logger.LegacyPrintf("service.baidu_vod_video", "generated media archive failed task_id=%s: %v", task.TaskID, archiveErr)
			w.retry(ctx, task, "RESULT_STORAGE_FAILED", "generated video storage failed")
			return
		}
		resultURL = archivedURL
		expires = nil
	}
	if err := w.service.Capture(ctx, task, actual); err != nil {
		w.retry(ctx, task, "CAPTURE_FAILED", "video billing settlement failed")
		return
	}
	finished := time.Now()
	settled := finished
	updated, err := w.service.tasks.UpdatePoll(ctx, task.TaskID, BaiduVODVideoTaskPollUpdate{Version: task.Version, Status: BaiduVODTaskStatusCompleted,
		UpstreamStatus: "SUCCEEDED", ResultURL: &resultURL, ResultExpiresAt: expires, OutputDuration: outputDuration,
		InputVideoDuration: responseUsageInputDuration(response), VideoCount: videoCount, ActualCost: &actual, NextPollAt: finished,
		RetryCount: task.RetryCount, FinishedAt: &finished, SettledAt: &settled})
	if err == nil && updated {
		task.OutputDuration, task.VideoCount, task.ActualCost = outputDuration, videoCount, &actual
		w.service.recordUsage(ctx, task, actual, billingMode, completionTokens, finished)
	}
}

func responseUsageInputDuration(response *BaiduVODTaskResponse) int {
	if response != nil && response.Usage != nil {
		return response.Usage.InputVideoDuration
	}
	return 0
}

func (w *BaiduVODVideoWorker) fail(ctx context.Context, task *BaiduVODVideoTask, code, message string) {
	if code == "" {
		code = "UPSTREAM_TASK_FAILED"
	}
	if message == "" {
		message = "Baidu VOD task failed"
	}
	if err := w.service.Release(ctx, task); err != nil {
		w.retry(ctx, task, "RELEASE_FAILED", err.Error())
		return
	}
	finished := time.Now()
	settled := finished
	_, _ = w.service.tasks.UpdatePoll(ctx, task.TaskID, BaiduVODVideoTaskPollUpdate{Version: task.Version, Status: BaiduVODTaskStatusFailed,
		UpstreamStatus: "FAILED", LastErrorCode: baiduVODStringPtr(code), LastErrorMessage: baiduVODStringPtr(message), NextPollAt: finished, RetryCount: task.RetryCount, FinishedAt: &finished, SettledAt: &settled})
}

func baiduVODStringPtr(value string) *string { return &value }

type BaiduVODVideoWorkerRuntime struct {
	worker *BaiduVODVideoWorker
}

func ProvideBaiduVODVideoWorkerRuntime(
	tasks BaiduVODVideoTaskRepository,
	accounts AccountRepository,
	billingRepo UsageBillingRepository,
	usageLogs UsageLogRepository,
	httpUpstream HTTPUpstream,
	billing *BillingService,
	pricing *ModelPricingResolver,
	authCache APIKeyAuthCacheInvalidator,
	mediaStorage *GeneratedMediaStorageService,
	cfg *config.Config,
) *BaiduVODVideoWorkerRuntime {
	service := NewBaiduVODVideoService(tasks, accounts, billingRepo, usageLogs, httpUpstream, billing, pricing, authCache, mediaStorage, cfg)
	runtime := &BaiduVODVideoWorkerRuntime{worker: NewBaiduVODVideoWorker(service)}
	runtime.worker.Start()
	return runtime
}

func (r *BaiduVODVideoWorkerRuntime) Stop() {
	if r != nil && r.worker != nil {
		r.worker.Stop()
	}
}

func (r *BaiduVODVideoWorkerRuntime) Service() *BaiduVODVideoService {
	if r == nil || r.worker == nil {
		return nil
	}
	return r.worker.service
}

var _ = fmt.Sprintf
