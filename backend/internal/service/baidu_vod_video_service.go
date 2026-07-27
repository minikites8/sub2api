package service

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
)

const (
	baiduVODHoldPrefix    = "baidu_vod_hold:"
	baiduVODCapturePrefix = "baidu_vod_capture:"
	baiduVODReleasePrefix = "baidu_vod_release:"
	baiduVODMaxBodyBytes  = 2 << 20
)

type BaiduVODUpstreamError struct {
	StatusCode int
	Code       string
	Message    string
}

func (e *BaiduVODUpstreamError) Error() string {
	if e == nil {
		return "baidu vod upstream error"
	}
	if e.Code != "" {
		return fmt.Sprintf("baidu vod upstream %s: %s", e.Code, e.Message)
	}
	return fmt.Sprintf("baidu vod upstream returned status %d: %s", e.StatusCode, e.Message)
}

type BaiduVODSubmitResult struct {
	TaskID     string
	TaskStatus string
	RequestID  string
}

type BaiduVODVideoService struct {
	tasks       BaiduVODVideoTaskRepository
	accounts    AccountRepository
	billingRepo UsageBillingRepository
	usageLogs   UsageLogRepository
	http        HTTPUpstream
	billing     *BillingService
	pricing     *ModelPricingResolver
	authCache   APIKeyAuthCacheInvalidator
	cfg         *config.Config
}

func NewBaiduVODVideoService(
	tasks BaiduVODVideoTaskRepository,
	accounts AccountRepository,
	billingRepo UsageBillingRepository,
	usageLogs UsageLogRepository,
	httpUpstream HTTPUpstream,
	billing *BillingService,
	pricing *ModelPricingResolver,
	authCache APIKeyAuthCacheInvalidator,
	cfg *config.Config,
) *BaiduVODVideoService {
	return &BaiduVODVideoService{tasks: tasks, accounts: accounts, billingRepo: billingRepo, usageLogs: usageLogs, http: httpUpstream, billing: billing, pricing: pricing, authCache: authCache, cfg: cfg}
}

func (s *BaiduVODVideoService) SelectAccount(ctx context.Context, groupID *int64, model string) (*Account, error) {
	if s == nil || s.accounts == nil {
		return nil, errors.New("baidu vod account repository is not configured")
	}
	var accounts []Account
	var err error
	if groupID != nil && *groupID > 0 {
		accounts, err = s.accounts.ListSchedulableByGroupIDAndPlatform(ctx, *groupID, PlatformBaiduVOD)
	} else {
		accounts, err = s.accounts.ListSchedulableUngroupedByPlatform(ctx, PlatformBaiduVOD)
	}
	if err != nil {
		return nil, err
	}
	eligible := make([]*Account, 0, len(accounts))
	for i := range accounts {
		account := &accounts[i]
		if account.Type != AccountTypeAPIKey || !account.IsSchedulableForModelWithContext(ctx, model) {
			continue
		}
		eligible = append(eligible, account)
	}
	if len(eligible) == 0 {
		return nil, fmt.Errorf("no available Baidu VOD accounts supporting model: %s", model)
	}
	sort.SliceStable(eligible, func(i, j int) bool {
		if eligible[i].Priority != eligible[j].Priority {
			return eligible[i].Priority > eligible[j].Priority
		}
		if eligible[i].LastUsedAt == nil {
			return true
		}
		if eligible[j].LastUsedAt == nil {
			return false
		}
		return eligible[i].LastUsedAt.Before(*eligible[j].LastUsedAt)
	})
	return eligible[0], nil
}

func (s *BaiduVODVideoService) Submit(ctx context.Context, account *Account, payload BaiduVODUpstreamRequest) (*BaiduVODSubmitResult, error) {
	spec, ok := BaiduVODModel(payload.Model)
	if !ok {
		return nil, fmt.Errorf("unsupported Baidu VOD model: %s", payload.Model)
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	respBody, status, err := s.do(ctx, account, spec.Provider, http.MethodPost, spec.CreatePath, body)
	if err != nil {
		return nil, err
	}
	if spec.Provider == BaiduVODProviderSeedance {
		var response baiduVODSeedanceCreateResponse
		if err := json.Unmarshal(respBody, &response); err != nil {
			return nil, fmt.Errorf("decode baidu vod Seedance create response: %w", err)
		}
		code, message := baiduVODJSONCode(response.Code), strings.TrimSpace(response.Message)
		if response.Error != nil {
			code = firstNonEmpty(baiduVODJSONCode(response.Error.Code), code)
			message = firstNonEmpty(strings.TrimSpace(response.Error.Message), message)
		}
		if status >= http.StatusBadRequest || code != "" {
			return nil, &BaiduVODUpstreamError{StatusCode: status, Code: code, Message: message}
		}
		if strings.TrimSpace(response.ID) == "" {
			return nil, &BaiduVODUpstreamError{StatusCode: status, Code: "INVALID_RESPONSE", Message: "upstream response is missing id"}
		}
		if s.accounts != nil {
			_ = s.accounts.UpdateLastUsed(ctx, account.ID)
		}
		return &BaiduVODSubmitResult{TaskID: strings.TrimSpace(response.ID), TaskStatus: "queued", RequestID: strings.TrimSpace(response.RequestID)}, nil
	}
	var response BaiduVODCreateResponse
	if err := json.Unmarshal(respBody, &response); err != nil {
		return nil, fmt.Errorf("decode baidu vod create response: %w", err)
	}
	if status >= http.StatusBadRequest || response.Code != "" {
		return nil, &BaiduVODUpstreamError{StatusCode: status, Code: response.Code, Message: response.Message}
	}
	if strings.TrimSpace(response.Output.TaskID) == "" {
		return nil, &BaiduVODUpstreamError{StatusCode: status, Code: "INVALID_RESPONSE", Message: "upstream response is missing task_id"}
	}
	if s.accounts != nil {
		_ = s.accounts.UpdateLastUsed(ctx, account.ID)
	}
	return &BaiduVODSubmitResult{TaskID: strings.TrimSpace(response.Output.TaskID), TaskStatus: strings.TrimSpace(response.Output.TaskStatus), RequestID: strings.TrimSpace(response.RequestID)}, nil
}

func (s *BaiduVODVideoService) Poll(ctx context.Context, account *Account, task *BaiduVODVideoTask) (*BaiduVODTaskResponse, error) {
	if task == nil {
		return nil, errors.New("baidu vod task is required")
	}
	provider, taskPath := task.Provider, BaiduVODTaskPath
	if spec, ok := BaiduVODModel(task.Model); ok {
		provider, taskPath = spec.Provider, spec.TaskPath
	} else if provider == BaiduVODProviderSeedance {
		taskPath = BaiduVODSeedanceTaskPath
	}
	respBody, status, err := s.do(ctx, account, provider, http.MethodGet, taskPath+strings.TrimSpace(task.UpstreamTaskID), nil)
	if err != nil {
		return nil, err
	}
	if provider == BaiduVODProviderSeedance {
		var response baiduVODSeedanceTaskResponse
		if err := json.Unmarshal(respBody, &response); err != nil {
			return nil, fmt.Errorf("decode baidu vod Seedance task response: %w", err)
		}
		code, message := baiduVODJSONCode(response.Code), strings.TrimSpace(response.Message)
		if response.Error != nil {
			code = firstNonEmpty(baiduVODJSONCode(response.Error.Code), code)
			message = firstNonEmpty(strings.TrimSpace(response.Error.Message), message)
		}
		if status >= http.StatusBadRequest || (code != "" && strings.TrimSpace(response.Status) == "") {
			return nil, &BaiduVODUpstreamError{StatusCode: status, Code: code, Message: message}
		}
		result := &BaiduVODTaskResponse{Provider: provider, RequestID: strings.TrimSpace(response.RequestID), Code: code, Message: message}
		result.Output.TaskID = strings.TrimSpace(response.ID)
		result.Output.TaskStatus = strings.ToUpper(strings.TrimSpace(response.Status))
		result.Output.Code = code
		result.Output.Message = message
		result.Output.VideoURL = firstNonEmpty(strings.TrimSpace(response.Content.VideoURL), strings.TrimSpace(response.Content.FileURL))
		result.Usage = &BaiduVODTaskUsage{Duration: response.Duration, OutputVideoDuration: response.Duration, VideoCount: 1,
			Ratio: strings.TrimSpace(response.Ratio), Resolution: strings.TrimSpace(response.Resolution)}
		if response.Usage != nil {
			result.Usage.CompletionTokens = response.Usage.CompletionTokens
			result.Usage.TotalTokens = response.Usage.TotalTokens
		}
		return result, nil
	}
	var response BaiduVODTaskResponse
	if err := json.Unmarshal(respBody, &response); err != nil {
		return nil, fmt.Errorf("decode baidu vod task response: %w", err)
	}
	if status >= http.StatusBadRequest || response.Code != "" {
		return nil, &BaiduVODUpstreamError{StatusCode: status, Code: response.Code, Message: response.Message}
	}
	return &response, nil
}

func (s *BaiduVODVideoService) do(ctx context.Context, account *Account, provider, method, suffix string, body []byte) ([]byte, int, error) {
	if s == nil || s.http == nil || account == nil || account.Platform != PlatformBaiduVOD {
		return nil, 0, errors.New("baidu vod upstream service or account is invalid")
	}
	target, authMode, err := baiduVODUpstreamURL(account, provider, suffix)
	if err != nil {
		return nil, 0, err
	}
	var reader io.Reader
	if len(body) > 0 {
		reader = bytes.NewReader(body)
	}
	req, err := http.NewRequestWithContext(ctx, method, target, reader)
	if err != nil {
		return nil, 0, err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Content-Type", "application/json")
	if method == http.MethodPost && provider == BaiduVODProviderHappyHorse {
		req.Header.Set("X-DashScope-Async", "enable")
	}
	switch authMode {
	case BaiduVODAuthModeAKSK:
		ak := firstNonEmpty(account.GetCredential("access_key_id"), account.GetCredential("access_key"), account.GetCredential("ak"))
		sk := firstNonEmpty(account.GetCredential("secret_access_key"), account.GetCredential("secret_key"), account.GetCredential("sk"))
		if err := BCEAuthV1(req, ak, sk, time.Now(), 30*time.Minute); err != nil {
			return nil, 0, err
		}
	default:
		apiKey := strings.TrimSpace(account.GetCredential("api_key"))
		if apiKey == "" {
			return nil, 0, errors.New("baidu vod api_key is required")
		}
		req.Header.Set("Authorization", "Bearer "+apiKey)
	}
	account.ApplyHeaderOverrides(req.Header)
	proxyURL := ""
	if account.ProxyID != nil && account.Proxy != nil {
		proxyURL = account.Proxy.URL()
	}
	resp, err := s.http.Do(req, proxyURL, account.ID, account.Concurrency)
	if err != nil {
		return nil, 0, err
	}
	defer resp.Body.Close()
	respBody, err := io.ReadAll(io.LimitReader(resp.Body, baiduVODMaxBodyBytes+1))
	if err != nil {
		return nil, resp.StatusCode, err
	}
	if len(respBody) > baiduVODMaxBodyBytes {
		return nil, resp.StatusCode, errors.New("baidu vod upstream response is too large")
	}
	return respBody, resp.StatusCode, nil
}

func baiduVODUpstreamURL(account *Account, provider, suffix string) (string, string, error) {
	authMode := strings.ToLower(strings.TrimSpace(account.GetCredential("auth_mode")))
	if authMode != BaiduVODAuthModeAKSK {
		authMode = BaiduVODAuthModeAPIKey
	}
	base := strings.TrimSpace(account.GetCredential("base_url"))
	if base == "" {
		base = BaiduVODDefaultBaseURL
	}
	parsed, err := url.Parse(base)
	if err != nil || parsed.Host == "" {
		return "", "", errors.New("invalid baidu vod base_url")
	}
	parsed.Scheme = "https"
	parsed.RawQuery = ""
	parsed.Fragment = ""
	parsed.Path = strings.TrimRight(parsed.Path, "/")
	for _, knownPrefix := range []string{"/v2/aigc/bailian", "/v3/aigc/bailian", "/v2/aigc/seedance", "/v3/aigc/seedance"} {
		if strings.HasSuffix(parsed.Path, knownPrefix) {
			parsed.Path = strings.TrimSuffix(parsed.Path, knownPrefix)
			break
		}
	}
	product := "bailian"
	if provider == BaiduVODProviderSeedance {
		product = "seedance"
	}
	prefix := "/v3/aigc/" + product
	if authMode == BaiduVODAuthModeAKSK {
		prefix = "/v2/aigc/" + product
	}
	parsed.Path = strings.TrimRight(parsed.Path, "/") + prefix + "/" + strings.TrimLeft(suffix, "/")
	return parsed.String(), authMode, nil
}

func (s *BaiduVODVideoService) NewTask(ctx context.Context, publicID string, apiKey *APIKey, account *Account, req BaiduVODVideoRequest, spec BaiduVODModelSpec, submitted *BaiduVODSubmitResult, requestHash string) (*BaiduVODVideoTask, error) {
	if s == nil || s.billing == nil || apiKey == nil || apiKey.User == nil || account == nil || submitted == nil {
		return nil, errors.New("baidu vod task billing context is incomplete")
	}
	groupMultiplier := 1.0
	videoMultiplier := 1.0
	var groupConfig *VideoPriceConfig
	if apiKey.Group != nil {
		groupMultiplier = apiKey.Group.RateMultiplier
		videoMultiplier = groupMultiplier
		if apiKey.Group.VideoRateIndependent {
			videoMultiplier = apiKey.Group.VideoRateMultiplier
		}
		groupConfig = &VideoPriceConfig{Price480P: apiKey.Group.VideoPrice480P, Price720P: apiKey.Group.VideoPrice720P, Price1080P: apiKey.Group.VideoPrice1080P, Price4K: apiKey.Group.VideoPrice4K}
	}
	resolution := NormalizeVideoBillingResolutionOrDefault(req.Resolution)
	groupPriceConfigured := apiKeyHasConfiguredVideoPrice(apiKey, resolution)
	billingDuration := req.Duration
	if billingDuration == -1 && spec.MaxDuration > 0 {
		billingDuration = spec.MaxDuration
	}
	estimatedTokens := 0
	if spec.Provider == BaiduVODProviderSeedance {
		estimatedTokens = estimateSeedanceCompletionTokens(req, spec)
	}
	cost := s.calculateVideoCost(ctx, req.Model, apiKey.GroupID, resolution, 1, billingDuration, estimatedTokens, groupConfig, groupPriceConfigured, videoMultiplier)
	billingMode := firstNonEmpty(cost.BillingMode, string(BillingModeVideo))
	now := time.Now()
	requestID := strings.TrimSpace(submitted.RequestID)
	task := &BaiduVODVideoTask{
		Platform: PlatformBaiduVOD, Provider: spec.Provider, TaskID: strings.TrimSpace(publicID), UpstreamTaskID: submitted.TaskID,
		UserID: apiKey.UserID, APIKeyID: apiKey.ID, AccountID: account.ID, GroupID: apiKey.GroupID,
		Model: req.Model, UpstreamModel: spec.UpstreamModel, Capability: spec.Capability, Status: BaiduVODTaskStatusQueued,
		UpstreamStatus: firstNonEmpty(strings.TrimSpace(submitted.TaskStatus), "PENDING"), Resolution: resolution, Ratio: req.Ratio,
		RequestedDuration: req.Duration, VideoCount: 1, BillingMode: billingMode, EstimatedCost: cost.ActualCost, HoldAmount: cost.ActualCost,
		GroupRateMultiplier: groupMultiplier, VideoRateMultiplier: videoMultiplier, AccountRateMultiplier: account.BillingRateMultiplier(),
		RequestHash: requestHash, NextPollAt: now.Add(5 * time.Second), SubmittedAt: now, CreatedAt: now, UpdatedAt: now,
	}
	if strings.TrimSpace(submitted.TaskID) == "" {
		task.Status = BaiduVODTaskStatusSubmitting
		task.UpstreamStatus = "SUBMITTING"
		task.NextPollAt = now.Add(2 * time.Minute)
	}
	if requestID != "" {
		task.UpstreamRequestID = &requestID
	}
	return task, nil
}

func (s *BaiduVODVideoService) calculateVideoCost(
	ctx context.Context,
	model string,
	groupID *int64,
	resolution string,
	videoCount int,
	durationSeconds int,
	completionTokens int,
	groupConfig *VideoPriceConfig,
	groupPriceConfigured bool,
	rateMultiplier float64,
) *CostBreakdown {
	if s == nil || s.billing == nil {
		return &CostBreakdown{}
	}
	if groupPriceConfigured || s.pricing == nil || groupID == nil {
		return s.billing.CalculateVideoCost(model, resolution, videoCount, durationSeconds, groupConfig, rateMultiplier)
	}
	resolved := s.pricing.Resolve(ctx, PricingInput{Model: model, GroupID: groupID})
	if resolved != nil && (resolved.Mode == BillingModeVideo || (resolved.Mode == BillingModeToken && completionTokens > 0)) {
		cost, err := s.billing.CalculateCostUnified(CostInput{
			Ctx:             ctx,
			Model:           model,
			GroupID:         groupID,
			RequestCount:    videoCount,
			SizeTier:        resolution,
			DurationSeconds: durationSeconds,
			Tokens:          UsageTokens{OutputTokens: completionTokens},
			RateMultiplier:  rateMultiplier,
			Resolver:        s.pricing,
			Resolved:        resolved,
		})
		if err == nil {
			return cost
		}
	}
	return s.billing.CalculateVideoCost(model, resolution, videoCount, durationSeconds, groupConfig, rateMultiplier)
}

func (s *BaiduVODVideoService) MarkSubmitted(ctx context.Context, taskID string, submitted BaiduVODSubmitResult) (bool, error) {
	if s == nil || s.tasks == nil {
		return false, errors.New("baidu vod task repository is not configured")
	}
	return s.tasks.MarkSubmitted(ctx, taskID, submitted, time.Now().Add(5*time.Second))
}

func (s *BaiduVODVideoService) MarkSubmissionFailed(ctx context.Context, taskID, code, message string) (bool, error) {
	if s == nil || s.tasks == nil {
		return false, errors.New("baidu vod task repository is not configured")
	}
	return s.tasks.MarkSubmissionFailed(ctx, taskID, code, message, time.Now())
}

func (s *BaiduVODVideoService) Reserve(ctx context.Context, task *BaiduVODVideoTask) error {
	if task == nil || task.HoldAmount <= 0 {
		return nil
	}
	_, err := s.billingRepo.ReserveBalanceHold(ctx, baiduVODHoldCommand(task, baiduVODHoldPrefix+task.TaskID, "", 0))
	return err
}

func (s *BaiduVODVideoService) Capture(ctx context.Context, task *BaiduVODVideoTask, actual float64) error {
	if task == nil {
		return errors.New("baidu vod task is required")
	}
	_, err := s.billingRepo.CaptureBalanceHold(ctx, baiduVODHoldCommand(task, baiduVODCapturePrefix+task.TaskID, baiduVODHoldPrefix+task.TaskID, actual))
	return err
}

func (s *BaiduVODVideoService) Release(ctx context.Context, task *BaiduVODVideoTask) error {
	if task == nil || task.HoldAmount <= 0 {
		return nil
	}
	_, err := s.billingRepo.ReleaseBalanceHold(ctx, baiduVODHoldCommand(task, baiduVODReleasePrefix+task.TaskID, baiduVODHoldPrefix+task.TaskID, 0))
	return err
}

func baiduVODHoldCommand(task *BaiduVODVideoTask, requestID, reserveRequestID string, actual float64) *BalanceHoldCommand {
	return &BalanceHoldCommand{RequestID: requestID, APIKeyID: task.APIKeyID, UserID: task.UserID, HoldID: task.TaskID,
		ReserveRequestID: reserveRequestID, HoldAmount: task.HoldAmount, ActualAmount: actual, RequestPayloadHash: task.RequestHash}
}

func (s *BaiduVODVideoService) CreateTask(ctx context.Context, task *BaiduVODVideoTask) error {
	if s == nil || s.tasks == nil {
		return errors.New("baidu vod task repository is not configured")
	}
	return s.tasks.Create(ctx, task)
}

func (s *BaiduVODVideoService) GetForOwner(ctx context.Context, userID, apiKeyID int64, taskID string) (*BaiduVODVideoTask, error) {
	if s == nil || s.tasks == nil {
		return nil, sql.ErrNoRows
	}
	return s.tasks.GetForOwner(ctx, userID, apiKeyID, taskID)
}

func (s *BaiduVODVideoService) recordUsage(ctx context.Context, task *BaiduVODVideoTask, actual float64, billingMode string, completionTokens int, now time.Time) {
	if s == nil || s.usageLogs == nil || task == nil {
		return
	}
	billingMode = firstNonEmpty(strings.TrimSpace(billingMode), string(BillingModeVideo))
	inbound, upstream, mediaType := "/v1/videos/generations", BaiduVODCreatePath, "video"
	if spec, ok := BaiduVODModel(task.Model); ok {
		upstream = spec.CreatePath
	}
	resolution, duration := task.Resolution, task.OutputDuration
	if duration <= 0 {
		duration = task.RequestedDuration
	}
	totalCost := actual
	if task.VideoRateMultiplier > 0 {
		totalCost = actual / task.VideoRateMultiplier
	}
	accountRate := task.AccountRateMultiplier
	log := &UsageLog{UserID: task.UserID, APIKeyID: task.APIKeyID, AccountID: task.AccountID, RequestID: baiduVODCapturePrefix + task.TaskID,
		Model: task.Model, RequestedModel: task.Model, UpstreamModel: optionalNonEqualStringPtr(task.UpstreamModel, task.Model), GroupID: task.GroupID,
		InboundEndpoint: &inbound, UpstreamEndpoint: &upstream, ImageCount: task.VideoCount, MediaType: &mediaType,
		OutputTokens: completionTokens, VideoCount: task.VideoCount, VideoResolution: &resolution, VideoDurationSeconds: &duration, TotalCost: totalCost, ActualCost: actual,
		RateMultiplier: task.VideoRateMultiplier, AccountRateMultiplier: &accountRate, BillingType: BillingTypeBalance, RequestType: RequestTypeSync,
		BillingMode: &billingMode, CreatedAt: now}
	if billingMode == string(BillingModeToken) {
		log.OutputCost = totalCost
	}
	writeUsageLogBestEffort(ctx, s.usageLogs, log, "service.baidu_vod_video", s.cfg)
	if s.authCache != nil {
		s.authCache.InvalidateAuthCacheByUserID(ctx, task.UserID)
	}
}
