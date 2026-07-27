package handler

import (
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	pkghttputil "github.com/Wei-Shaw/sub2api/internal/pkg/httputil"
	middleware2 "github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"go.uber.org/zap"
)

func (h *OpenAIGatewayHandler) BaiduVODVideoCreate(c *gin.Context) {
	apiKey, ok := middleware2.GetAPIKeyFromContext(c)
	if !ok || apiKey == nil {
		h.errorResponse(c, http.StatusUnauthorized, "authentication_error", "Invalid API key")
		return
	}
	subject, ok := middleware2.GetAuthSubjectFromContext(c)
	if !ok {
		h.errorResponse(c, http.StatusInternalServerError, "api_error", "User context not found")
		return
	}
	if h.baiduVODVideoService == nil {
		h.errorResponse(c, http.StatusServiceUnavailable, "api_error", "Baidu VOD video service is unavailable")
		return
	}

	body, err := pkghttputil.ReadRequestBodyWithPrealloc(c.Request)
	if err != nil {
		if maxErr, ok := extractMaxBytesError(err); ok {
			h.errorResponse(c, http.StatusRequestEntityTooLarge, "invalid_request_error", buildBodyTooLargeMessage(maxErr.Limit))
			return
		}
		h.errorResponse(c, http.StatusBadRequest, "invalid_request_error", "Failed to read request body")
		return
	}
	request, err := service.ParseBaiduVODVideoRequest(body)
	if err != nil {
		h.errorResponse(c, http.StatusBadRequest, "invalid_request_error", err.Error())
		return
	}
	spec, upstreamRequest, err := service.TranslateBaiduVODVideoRequest(request)
	if err != nil {
		h.errorResponse(c, http.StatusBadRequest, "invalid_request_error", err.Error())
		return
	}

	reqLog := requestLogger(c, "handler.openai_gateway.baidu_vod_video",
		zap.Int64("user_id", subject.UserID), zap.Int64("api_key_id", apiKey.ID),
		zap.Any("group_id", apiKey.GroupID), zap.String("model", request.Model))
	setOpsRequestContext(c, request.Model, false)
	setOpsEndpointContext(c, "", int16(service.RequestTypeSync))
	if !service.GroupAllowsImageGeneration(apiKey.Group) {
		h.errorResponse(c, http.StatusForbidden, "permission_error", service.ImageGenerationPermissionMessage())
		return
	}
	if decision := h.checkContentModeration(c, reqLog, apiKey, subject, service.ContentModerationProtocolOpenAIImages, request.Model, body); decision != nil && decision.Blocked {
		h.errorResponse(c, contentModerationStatus(decision), contentModerationErrorCode(decision), decision.Message)
		return
	}

	streamStarted := false
	mediaRelease, acquired := h.acquireImageGenerationSlot(c, streamStarted)
	if !acquired {
		return
	}
	if mediaRelease != nil {
		defer mediaRelease()
	}
	userRelease, err := h.concurrencyHelper.AcquireUserSlotWithWait(c, subject.UserID, subject.Concurrency, false, &streamStarted)
	if err != nil {
		h.handleConcurrencyError(c, err, "user", false)
		return
	}
	if userRelease != nil {
		defer userRelease()
	}

	subscription, _ := middleware2.GetSubscriptionFromContext(c)
	if err := h.checkBillingEligibility(c.Request.Context(), apiKey.User, apiKey, apiKey.Group, subscription, service.PlatformBaiduVOD); err != nil {
		status, code, message, retryAfter := billingErrorDetails(err)
		if retryAfter > 0 {
			c.Header("Retry-After", stringInt(retryAfter))
		}
		h.errorResponse(c, status, code, message)
		return
	}

	account, err := h.baiduVODVideoService.SelectAccount(c.Request.Context(), apiKey.GroupID, request.Model)
	if err != nil {
		markOpsRoutingCapacityLimitedIfNoAvailable(c, err)
		h.errorResponse(c, http.StatusServiceUnavailable, "server_error", err.Error())
		return
	}
	setOpsSelectedAccount(c, account.ID, account.Platform)
	accountRelease, err := h.concurrencyHelper.AcquireAccountSlotWithWait(c, account.ID, account.Concurrency, false, &streamStarted)
	if err != nil {
		h.handleConcurrencyError(c, err, "account", false)
		return
	}
	if accountRelease != nil {
		defer accountRelease()
	}

	publicID := "video_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	requestSum := sha256.Sum256(body)
	requestHash := hex.EncodeToString(requestSum[:])
	task, err := h.baiduVODVideoService.NewTask(c.Request.Context(), publicID, apiKey, account, request, spec,
		&service.BaiduVODSubmitResult{TaskStatus: "SUBMITTING"}, requestHash)
	if err != nil {
		h.errorResponse(c, http.StatusInternalServerError, "api_error", "Failed to prepare video task")
		return
	}
	if err := h.baiduVODVideoService.Reserve(c.Request.Context(), task); err != nil {
		status, code, message, retryAfter := billingErrorDetails(err)
		if retryAfter > 0 {
			c.Header("Retry-After", stringInt(retryAfter))
		}
		h.errorResponse(c, status, code, message)
		return
	}
	if err := h.baiduVODVideoService.CreateTask(c.Request.Context(), task); err != nil {
		_ = h.baiduVODVideoService.Release(c.Request.Context(), task)
		reqLog.Error("baidu_vod_video.task_create_failed", zap.Error(err))
		h.errorResponse(c, http.StatusInternalServerError, "api_error", "Failed to persist video task")
		return
	}

	submitted, err := h.baiduVODVideoService.Submit(c.Request.Context(), account, upstreamRequest)
	if err != nil {
		h.failBaiduVODSubmission(c, reqLog, task, err)
		return
	}
	updated, err := h.baiduVODVideoService.MarkSubmitted(c.Request.Context(), task.TaskID, *submitted)
	if err != nil || !updated {
		_ = h.baiduVODVideoService.Release(c.Request.Context(), task)
		_, _ = h.baiduVODVideoService.MarkSubmissionFailed(c.Request.Context(), task.TaskID, "TASK_PERSIST_FAILED", "Failed to persist Baidu VOD submission")
		reqLog.Error("baidu_vod_video.submission_persist_failed", zap.Error(err), zap.Bool("updated", updated))
		h.errorResponse(c, http.StatusInternalServerError, "api_error", "Failed to persist video submission")
		return
	}
	task.UpstreamTaskID = submitted.TaskID
	task.UpstreamStatus = submitted.TaskStatus
	task.Status = service.BaiduVODTaskStatusQueued
	task.SubmittedAt = time.Now()
	h.writeBaiduVODVideo(c, task)
}

func (h *OpenAIGatewayHandler) failBaiduVODSubmission(c *gin.Context, reqLog *zap.Logger, task *service.BaiduVODVideoTask, submitErr error) {
	code := "UPSTREAM_SUBMISSION_FAILED"
	message := "Baidu VOD task submission failed"
	status := http.StatusBadGateway
	var upstreamErr *service.BaiduVODUpstreamError
	if errors.As(submitErr, &upstreamErr) {
		if strings.TrimSpace(upstreamErr.Code) != "" {
			code = strings.TrimSpace(upstreamErr.Code)
		}
		if strings.TrimSpace(upstreamErr.Message) != "" {
			message = strings.TrimSpace(upstreamErr.Message)
		}
		if upstreamErr.StatusCode >= 400 && upstreamErr.StatusCode < 500 {
			status = upstreamErr.StatusCode
		}
	}
	if releaseErr := h.baiduVODVideoService.Release(c.Request.Context(), task); releaseErr != nil {
		reqLog.Error("baidu_vod_video.release_after_submit_failed", zap.Error(releaseErr))
	} else {
		_, _ = h.baiduVODVideoService.MarkSubmissionFailed(c.Request.Context(), task.TaskID, code, message)
	}
	reqLog.Warn("baidu_vod_video.submit_failed", zap.Error(submitErr), zap.String("upstream_code", code))
	h.errorResponse(c, status, "api_error", message)
}

func (h *OpenAIGatewayHandler) BaiduVODVideoStatus(c *gin.Context) {
	apiKey, ok := middleware2.GetAPIKeyFromContext(c)
	if !ok || apiKey == nil {
		h.errorResponse(c, http.StatusUnauthorized, "authentication_error", "Invalid API key")
		return
	}
	subject, ok := middleware2.GetAuthSubjectFromContext(c)
	if !ok {
		h.errorResponse(c, http.StatusInternalServerError, "api_error", "User context not found")
		return
	}
	if h.baiduVODVideoService == nil {
		h.errorResponse(c, http.StatusServiceUnavailable, "api_error", "Baidu VOD video service is unavailable")
		return
	}
	task, err := h.baiduVODVideoService.GetForOwner(c.Request.Context(), subject.UserID, apiKey.ID, c.Param("request_id"))
	if errors.Is(err, sql.ErrNoRows) {
		h.errorResponse(c, http.StatusNotFound, "not_found_error", "Video task not found")
		return
	}
	if err != nil {
		h.errorResponse(c, http.StatusInternalServerError, "api_error", "Failed to load video task")
		return
	}
	setOpsRequestContext(c, task.Model, false)
	h.writeBaiduVODVideo(c, task)
}

func (h *OpenAIGatewayHandler) writeBaiduVODVideo(c *gin.Context, task *service.BaiduVODVideoTask) {
	status := task.Status
	if status == service.BaiduVODTaskStatusSubmitting {
		status = service.BaiduVODTaskStatusQueued
	}
	progress := 0
	if status == service.BaiduVODTaskStatusInProgress {
		progress = 50
	}
	if service.BaiduVODTaskIsTerminal(status) {
		progress = 100
	}
	seconds := task.RequestedDuration
	if task.OutputDuration > 0 {
		seconds = task.OutputDuration
	}
	response := gin.H{
		"id": task.TaskID, "object": "video", "created_at": task.CreatedAt.Unix(),
		"status": status, "progress": progress, "model": task.Model, "provider": task.Provider,
		"seconds": seconds, "size": baiduVODVideoSize(task.Resolution, task.Ratio),
		"resolution": task.Resolution, "ratio": task.Ratio,
	}
	if task.FinishedAt != nil {
		response["completed_at"] = task.FinishedAt.Unix()
	}
	if task.ResultURL != nil && strings.TrimSpace(*task.ResultURL) != "" {
		response["video_url"] = strings.TrimSpace(*task.ResultURL)
	}
	if task.ResultExpiresAt != nil {
		response["expires_at"] = task.ResultExpiresAt.Unix()
	}
	if task.Status == service.BaiduVODTaskStatusFailed {
		code, message := "video_generation_failed", "Video generation failed"
		if task.LastErrorCode != nil && strings.TrimSpace(*task.LastErrorCode) != "" {
			code = strings.TrimSpace(*task.LastErrorCode)
		}
		if task.LastErrorMessage != nil && strings.TrimSpace(*task.LastErrorMessage) != "" {
			message = strings.TrimSpace(*task.LastErrorMessage)
		}
		response["error"] = gin.H{"code": code, "message": message}
	}
	c.JSON(http.StatusOK, response)
}

func baiduVODVideoSize(resolution, ratio string) string {
	short, long := 720, 1280
	switch service.NormalizeVideoBillingResolutionOrDefault(resolution) {
	case service.VideoBillingResolution480P:
		short, long = 480, 864
	case service.VideoBillingResolution1080P:
		short, long = 1080, 1920
	case service.VideoBillingResolution4K:
		short, long = 2160, 3840
	}
	switch strings.TrimSpace(ratio) {
	case "9:16":
		return stringInt(short) + "x" + stringInt(long)
	case "3:4":
		return stringInt(short) + "x" + stringInt(short*4/3)
	case "1:1":
		return stringInt(short) + "x" + stringInt(short)
	case "4:3":
		return stringInt(short*4/3) + "x" + stringInt(short)
	case "21:9":
		return stringInt(short*21/9) + "x" + stringInt(short)
	default:
		return stringInt(long) + "x" + stringInt(short)
	}
}

func stringInt(value int) string {
	return strconv.Itoa(value)
}
