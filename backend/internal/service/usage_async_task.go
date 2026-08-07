package service

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"time"
)

const grokVideoUsageRequestPrefix = "grok_video:"

type UsageAsyncTaskDetails struct {
	Kind       string
	TaskID     string
	Status     string
	StatusURL  string
	ResultURLs []string
	ExpiresAt  *time.Time
}

type UsageAsyncTaskService struct {
	baiduVODTasks BaiduVODVideoTaskRepository
	batchImages   BatchImageRepository
}

func NewUsageAsyncTaskService(baiduVODTasks BaiduVODVideoTaskRepository, batchImages BatchImageRepository) *UsageAsyncTaskService {
	return &UsageAsyncTaskService{baiduVODTasks: baiduVODTasks, batchImages: batchImages}
}

func GrokVideoUsageRequestID(taskID string) string {
	return grokVideoUsageRequestPrefix + strings.TrimSpace(taskID)
}

func (s *UsageAsyncTaskService) GetDetails(ctx context.Context, usageLog *UsageLog) (*UsageAsyncTaskDetails, error) {
	if s == nil || usageLog == nil {
		return nil, nil
	}
	requestID := strings.TrimSpace(usageLog.RequestID)
	switch {
	case strings.HasPrefix(requestID, baiduVODCapturePrefix):
		return s.getBaiduVODDetails(ctx, usageLog, strings.TrimPrefix(requestID, baiduVODCapturePrefix))
	case strings.HasPrefix(requestID, batchImageCaptureRequestPrefix):
		return s.getBatchImageDetails(ctx, usageLog, strings.TrimPrefix(requestID, batchImageCaptureRequestPrefix))
	case strings.HasPrefix(requestID, batchImageSettlementRequestPrefix):
		return s.getBatchImageDetails(ctx, usageLog, strings.TrimPrefix(requestID, batchImageSettlementRequestPrefix))
	case strings.HasPrefix(requestID, grokVideoUsageRequestPrefix):
		taskID := strings.TrimSpace(strings.TrimPrefix(requestID, grokVideoUsageRequestPrefix))
		if taskID == "" {
			return nil, nil
		}
		return &UsageAsyncTaskDetails{
			Kind:      "grok_video",
			TaskID:    taskID,
			Status:    "submitted",
			StatusURL: videoTaskStatusURL(taskID),
		}, nil
	default:
		return nil, nil
	}
}

func (s *UsageAsyncTaskService) getBaiduVODDetails(ctx context.Context, usageLog *UsageLog, taskID string) (*UsageAsyncTaskDetails, error) {
	taskID = strings.TrimSpace(taskID)
	if taskID == "" || s.baiduVODTasks == nil {
		return nil, nil
	}
	task, err := s.baiduVODTasks.GetForOwner(ctx, usageLog.UserID, usageLog.APIKeyID, taskID)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get baidu vod usage task: %w", err)
	}
	details := &UsageAsyncTaskDetails{
		Kind:      "video",
		TaskID:    task.TaskID,
		Status:    task.Status,
		StatusURL: videoTaskStatusURL(task.TaskID),
		ExpiresAt: task.ResultExpiresAt,
	}
	if task.ResultURL != nil && strings.TrimSpace(*task.ResultURL) != "" {
		details.ResultURLs = []string{strings.TrimSpace(*task.ResultURL)}
	}
	return details, nil
}

func (s *UsageAsyncTaskService) getBatchImageDetails(ctx context.Context, usageLog *UsageLog, batchID string) (*UsageAsyncTaskDetails, error) {
	batchID = strings.TrimSpace(batchID)
	if batchID == "" || s.batchImages == nil {
		return nil, nil
	}
	job, err := s.batchImages.GetBatchImageJobByBatchIDForOwner(ctx, usageLog.UserID, usageLog.APIKeyID, batchID)
	if errors.Is(err, ErrBatchImageJobNotFound) || errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get batch image usage task: %w", err)
	}
	details := &UsageAsyncTaskDetails{
		Kind:      "batch_image",
		TaskID:    job.BatchID,
		Status:    job.Status,
		StatusURL: batchImageTaskStatusURL(job.BatchID),
		ExpiresAt: job.OutputExpiresAt,
	}
	items, err := s.batchImages.ListBatchImageItems(ctx, batchID, BatchImageItemFilter{Status: BatchImageItemStatusSuccess})
	if err != nil {
		return nil, fmt.Errorf("list batch image usage results: %w", err)
	}
	for _, item := range items {
		if item == nil || strings.TrimSpace(item.CustomID) == "" {
			continue
		}
		imageCount := item.ImageCount
		if imageCount <= 0 {
			imageCount = 1
		}
		for imageIndex := 0; imageIndex < imageCount; imageIndex++ {
			details.ResultURLs = append(details.ResultURLs, batchImageItemResultURL(job.BatchID, item.CustomID, imageIndex, imageCount))
		}
	}
	return details, nil
}

func videoTaskStatusURL(taskID string) string {
	return "/v1/videos/" + url.PathEscape(strings.TrimSpace(taskID))
}

func batchImageTaskStatusURL(batchID string) string {
	return "/v1/images/batches/" + url.PathEscape(strings.TrimSpace(batchID))
}

func batchImageItemResultURL(batchID, customID string, imageIndex, imageCount int) string {
	resultURL := batchImageTaskStatusURL(batchID) + "/items/" + url.PathEscape(strings.TrimSpace(customID)) + "/content"
	if imageCount > 1 {
		resultURL += "?image_index=" + fmt.Sprintf("%d", imageIndex)
	}
	return resultURL
}
