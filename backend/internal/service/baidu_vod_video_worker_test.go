package service

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

type baiduVODWorkerTaskRepo struct {
	BaiduVODVideoTaskRepository
	updates []BaiduVODVideoTaskPollUpdate
}

func (r *baiduVODWorkerTaskRepo) UpdatePoll(_ context.Context, _ string, update BaiduVODVideoTaskPollUpdate) (bool, error) {
	r.updates = append(r.updates, update)
	return true, nil
}

type baiduVODWorkerAccountRepo struct {
	AccountRepository
	account *Account
	err     error
}

func (r *baiduVODWorkerAccountRepo) GetByID(_ context.Context, _ int64) (*Account, error) {
	return r.account, r.err
}

type baiduVODWorkerBillingRepo struct {
	UsageBillingRepository
	captures []*BalanceHoldCommand
	releases []*BalanceHoldCommand
}

func (r *baiduVODWorkerBillingRepo) CaptureBalanceHold(_ context.Context, cmd *BalanceHoldCommand) (*BalanceHoldResult, error) {
	copy := *cmd
	r.captures = append(r.captures, &copy)
	return &BalanceHoldResult{Applied: true}, nil
}

func (r *baiduVODWorkerBillingRepo) ReleaseBalanceHold(_ context.Context, cmd *BalanceHoldCommand) (*BalanceHoldResult, error) {
	copy := *cmd
	r.releases = append(r.releases, &copy)
	return &BalanceHoldResult{Applied: true}, nil
}

func newBaiduVODWorkerHarness(response *http.Response, upstreamErr error) (*BaiduVODVideoWorker, *baiduVODWorkerTaskRepo, *baiduVODWorkerBillingRepo) {
	tasks := &baiduVODWorkerTaskRepo{}
	billingRepo := &baiduVODWorkerBillingRepo{}
	account := &Account{ID: 31, Platform: PlatformBaiduVOD, Type: AccountTypeAPIKey, Concurrency: 1, Credentials: map[string]any{
		"auth_mode": BaiduVODAuthModeAPIKey,
		"api_key":   "vod-key",
	}}
	service := &BaiduVODVideoService{
		tasks:       tasks,
		accounts:    &baiduVODWorkerAccountRepo{account: account},
		billingRepo: billingRepo,
		http:        &httpUpstreamRecorder{resp: response, err: upstreamErr},
		billing:     NewBillingService(nil, nil),
	}
	return NewBaiduVODVideoWorker(service), tasks, billingRepo
}

func newBaiduVODWorkerTask() *BaiduVODVideoTask {
	return &BaiduVODVideoTask{
		Platform: PlatformBaiduVOD, Provider: BaiduVODProvider, TaskID: "video-task-1", UpstreamTaskID: "upstream-task-1",
		UserID: 11, APIKeyID: 21, AccountID: 31, Model: "happyhorse-1.1-t2v", UpstreamModel: "happyhorse-1.1-t2v",
		Capability: BaiduVODCapabilityT2V, Status: BaiduVODTaskStatusQueued, UpstreamStatus: "PENDING",
		Resolution: "720P", Ratio: "16:9", RequestedDuration: 5, VideoCount: 1, EstimatedCost: 4.5, HoldAmount: 4.5,
		GroupRateMultiplier: 1, VideoRateMultiplier: 1, AccountRateMultiplier: 1, RequestHash: "request-hash",
		RetryCount: 0, Version: 4, SubmittedAt: time.Now(),
	}
}

func baiduVODWorkerResponse(status int, body string) *http.Response {
	return &http.Response{StatusCode: status, Body: io.NopCloser(strings.NewReader(body))}
}

func TestBaiduVODVideoWorkerSucceededCapturesActualUsage(t *testing.T) {
	worker, tasks, billingRepo := newBaiduVODWorkerHarness(baiduVODWorkerResponse(http.StatusOK,
		`{"output":{"task_status":"SUCCEEDED","video_url":"https://example.com/video.mp4"},"usage":{"output_video_duration":10,"video_count":2,"SR":1080}}`), nil)
	task := newBaiduVODWorkerTask()
	task.HoldAmount = 30

	worker.processOne(context.Background(), task)

	require.Len(t, billingRepo.captures, 1)
	require.InDelta(t, 24, billingRepo.captures[0].ActualAmount, 0.00000001)
	require.Empty(t, billingRepo.releases)
	require.Len(t, tasks.updates, 1)
	update := tasks.updates[0]
	require.Equal(t, BaiduVODTaskStatusCompleted, update.Status)
	require.Equal(t, 10, update.OutputDuration)
	require.Equal(t, 2, update.VideoCount)
	require.NotNil(t, update.ActualCost)
	require.InDelta(t, 24, *update.ActualCost, 0.00000001)
}

func TestBaiduVODVideoWorkerTerminalFailuresReleaseHold(t *testing.T) {
	for _, status := range []string{"FAILED", "UNKNOWN"} {
		t.Run(status, func(t *testing.T) {
			worker, tasks, billingRepo := newBaiduVODWorkerHarness(baiduVODWorkerResponse(http.StatusOK,
				`{"output":{"task_status":"`+status+`","code":"UPSTREAM_FAILED","message":"generation failed"}}`), nil)

			worker.processOne(context.Background(), newBaiduVODWorkerTask())

			require.Empty(t, billingRepo.captures)
			require.Len(t, billingRepo.releases, 1)
			require.Len(t, tasks.updates, 1)
			require.Equal(t, BaiduVODTaskStatusFailed, tasks.updates[0].Status)
		})
	}
}

func TestBaiduVODVideoWorkerPendingKeepsRetryCount(t *testing.T) {
	worker, tasks, billingRepo := newBaiduVODWorkerHarness(baiduVODWorkerResponse(http.StatusOK,
		`{"output":{"task_status":"RUNNING"}}`), nil)
	task := newBaiduVODWorkerTask()
	task.RetryCount = 5

	worker.processOne(context.Background(), task)

	require.Empty(t, billingRepo.captures)
	require.Empty(t, billingRepo.releases)
	require.Len(t, tasks.updates, 1)
	require.Equal(t, BaiduVODTaskStatusInProgress, tasks.updates[0].Status)
	require.Equal(t, 5, tasks.updates[0].RetryCount)
}

func TestBaiduVODVideoWorkerNetworkRetriesThenReleases(t *testing.T) {
	t.Run("increments retry", func(t *testing.T) {
		worker, tasks, billingRepo := newBaiduVODWorkerHarness(nil, errors.New("network unavailable"))
		task := newBaiduVODWorkerTask()
		task.RetryCount = 2

		worker.processOne(context.Background(), task)

		require.Empty(t, billingRepo.releases)
		require.Len(t, tasks.updates, 1)
		require.Equal(t, 3, tasks.updates[0].RetryCount)
		require.Equal(t, BaiduVODTaskStatusInProgress, tasks.updates[0].Status)
	})

	t.Run("releases after retry limit", func(t *testing.T) {
		worker, tasks, billingRepo := newBaiduVODWorkerHarness(nil, errors.New("network unavailable"))
		task := newBaiduVODWorkerTask()
		task.RetryCount = baiduVODPollMaxAttempts

		worker.processOne(context.Background(), task)

		require.Len(t, billingRepo.releases, 1)
		require.Len(t, tasks.updates, 1)
		require.Equal(t, BaiduVODTaskStatusFailed, tasks.updates[0].Status)
		require.Equal(t, "POLL_RETRY_EXHAUSTED", *tasks.updates[0].LastErrorCode)
	})
}

func TestBaiduVODVideoWorkerActualCostAboveHoldReleases(t *testing.T) {
	worker, tasks, billingRepo := newBaiduVODWorkerHarness(baiduVODWorkerResponse(http.StatusOK,
		`{"output":{"task_status":"SUCCEEDED","video_url":"https://example.com/video.mp4"},"usage":{"output_video_duration":6,"video_count":1,"SR":720}}`), nil)

	worker.processOne(context.Background(), newBaiduVODWorkerTask())

	require.Empty(t, billingRepo.captures)
	require.Len(t, billingRepo.releases, 1)
	require.Len(t, tasks.updates, 1)
	require.Equal(t, BaiduVODTaskStatusFailed, tasks.updates[0].Status)
	require.Equal(t, "ACTUAL_COST_EXCEEDS_HOLD", *tasks.updates[0].LastErrorCode)
}

func TestBaiduVODVideoWorkerSubmittingInterruptionReleases(t *testing.T) {
	worker, tasks, billingRepo := newBaiduVODWorkerHarness(nil, nil)
	task := newBaiduVODWorkerTask()
	task.Status = BaiduVODTaskStatusSubmitting
	task.UpstreamTaskID = ""

	worker.processOne(context.Background(), task)

	require.Len(t, billingRepo.releases, 1)
	require.Len(t, tasks.updates, 1)
	require.Equal(t, BaiduVODTaskStatusFailed, tasks.updates[0].Status)
	require.Equal(t, "SUBMISSION_INTERRUPTED", *tasks.updates[0].LastErrorCode)
}
