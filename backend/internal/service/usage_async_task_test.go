package service

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

type usageAsyncBaiduTaskRepo struct {
	BaiduVODVideoTaskRepository
	task      *BaiduVODVideoTask
	err       error
	gotUserID int64
	gotAPIKey int64
	gotTaskID string
}

func (r *usageAsyncBaiduTaskRepo) GetForOwner(_ context.Context, userID, apiKeyID int64, taskID string) (*BaiduVODVideoTask, error) {
	r.gotUserID = userID
	r.gotAPIKey = apiKeyID
	r.gotTaskID = taskID
	return r.task, r.err
}

type usageAsyncBatchImageRepo struct {
	BatchImageRepository
	job       *BatchImageJob
	items     []*BatchImageItem
	err       error
	gotUserID int64
	gotAPIKey int64
	gotBatch  string
	filter    BatchImageItemFilter
}

func (r *usageAsyncBatchImageRepo) GetBatchImageJobByBatchIDForOwner(_ context.Context, userID, apiKeyID int64, batchID string) (*BatchImageJob, error) {
	r.gotUserID = userID
	r.gotAPIKey = apiKeyID
	r.gotBatch = batchID
	return r.job, r.err
}

func (r *usageAsyncBatchImageRepo) ListBatchImageItems(_ context.Context, batchID string, filter BatchImageItemFilter) ([]*BatchImageItem, error) {
	r.gotBatch = batchID
	r.filter = filter
	return r.items, r.err
}

func TestUsageAsyncTaskServiceBaiduVODDetails(t *testing.T) {
	expiresAt := time.Date(2026, 8, 9, 12, 0, 0, 0, time.UTC)
	resultURL := "https://cdn.example.com/generated/video.mp4"
	repo := &usageAsyncBaiduTaskRepo{task: &BaiduVODVideoTask{
		TaskID:          "video-task-1",
		Status:          BaiduVODTaskStatusCompleted,
		ResultURL:       &resultURL,
		ResultExpiresAt: &expiresAt,
	}}
	svc := NewUsageAsyncTaskService(repo, nil)

	details, err := svc.GetDetails(context.Background(), &UsageLog{
		UserID:    42,
		APIKeyID:  7,
		RequestID: baiduVODCapturePrefix + "video-task-1",
	})

	require.NoError(t, err)
	require.NotNil(t, details)
	require.Equal(t, int64(42), repo.gotUserID)
	require.Equal(t, int64(7), repo.gotAPIKey)
	require.Equal(t, "video-task-1", repo.gotTaskID)
	require.Equal(t, "video", details.Kind)
	require.Equal(t, BaiduVODTaskStatusCompleted, details.Status)
	require.Equal(t, "/v1/videos/video-task-1", details.StatusURL)
	require.Equal(t, []string{resultURL}, details.ResultURLs)
	require.Equal(t, expiresAt, *details.ExpiresAt)
}

func TestUsageAsyncTaskServiceBatchImageDetails(t *testing.T) {
	expiresAt := time.Date(2026, 8, 10, 12, 0, 0, 0, time.UTC)
	repo := &usageAsyncBatchImageRepo{
		job: &BatchImageJob{
			BatchID:         "batch-1",
			Status:          BatchImageJobStatusCompleted,
			OutputExpiresAt: &expiresAt,
		},
		items: []*BatchImageItem{
			{CustomID: "item/a b", Status: BatchImageItemStatusSuccess, ImageCount: 2},
			{CustomID: "item-2", Status: BatchImageItemStatusSuccess, ImageCount: 1},
		},
	}
	svc := NewUsageAsyncTaskService(nil, repo)

	details, err := svc.GetDetails(context.Background(), &UsageLog{
		UserID:    51,
		APIKeyID:  9,
		RequestID: BatchImageCaptureRequestID("batch-1"),
	})

	require.NoError(t, err)
	require.NotNil(t, details)
	require.Equal(t, int64(51), repo.gotUserID)
	require.Equal(t, int64(9), repo.gotAPIKey)
	require.Equal(t, "batch-1", repo.gotBatch)
	require.Equal(t, BatchImageItemStatusSuccess, repo.filter.Status)
	require.Equal(t, "batch_image", details.Kind)
	require.Equal(t, "/v1/images/batches/batch-1", details.StatusURL)
	require.Equal(t, []string{
		"/v1/images/batches/batch-1/items/item%2Fa%20b/content?image_index=0",
		"/v1/images/batches/batch-1/items/item%2Fa%20b/content?image_index=1",
		"/v1/images/batches/batch-1/items/item-2/content",
	}, details.ResultURLs)
}

func TestUsageAsyncTaskServiceGrokVideoDetails(t *testing.T) {
	svc := NewUsageAsyncTaskService(nil, nil)

	details, err := svc.GetDetails(context.Background(), &UsageLog{RequestID: GrokVideoUsageRequestID("video-response-1")})

	require.NoError(t, err)
	require.NotNil(t, details)
	require.Equal(t, "grok_video", details.Kind)
	require.Equal(t, "video-response-1", details.TaskID)
	require.Equal(t, "submitted", details.Status)
	require.Equal(t, "/v1/videos/video-response-1", details.StatusURL)
	require.Empty(t, details.ResultURLs)
}

func TestUsageAsyncTaskServiceIgnoresRegularUsage(t *testing.T) {
	svc := NewUsageAsyncTaskService(nil, nil)
	details, err := svc.GetDetails(context.Background(), &UsageLog{RequestID: "req-regular"})
	require.NoError(t, err)
	require.Nil(t, details)
}
