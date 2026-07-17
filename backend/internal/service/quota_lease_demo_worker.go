package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
)

const (
	quotaLeaseDemoNodeWorkerInterval   = 10 * time.Second
	quotaLeaseDemoNodeWorkerRunTimeout = 30 * time.Second
)

type QuotaLeaseDemoAccountTaskExecutor interface {
	ExecuteAccountLoginTask(ctx context.Context, task QuotaLeaseDemoAccountLoginTask) (QuotaLeaseDemoAccountSnapshot, error)
}

type QuotaLeaseDemoPayloadAccountTaskExecutor struct{}

func NewQuotaLeaseDemoPayloadAccountTaskExecutor() *QuotaLeaseDemoPayloadAccountTaskExecutor {
	return &QuotaLeaseDemoPayloadAccountTaskExecutor{}
}

func (e *QuotaLeaseDemoPayloadAccountTaskExecutor) ExecuteAccountLoginTask(ctx context.Context, task QuotaLeaseDemoAccountLoginTask) (QuotaLeaseDemoAccountSnapshot, error) {
	_ = ctx
	now := time.Now().UTC()
	var account QuotaLeaseDemoAccountSnapshot
	if raw, ok := task.LoginPayload["account"]; ok {
		if err := decodeQuotaLeaseDemoAccountSnapshot(raw, &account); err != nil {
			return QuotaLeaseDemoAccountSnapshot{}, err
		}
	} else {
		credentials := quotaLeaseDemoAnyMapFromPayload(task.LoginPayload["credentials"])
		if len(credentials) == 0 {
			return QuotaLeaseDemoAccountSnapshot{}, fmt.Errorf("%w: login_payload.account or login_payload.credentials is required", ErrQuotaLeaseDemoInvalidInput)
		}
		account.Credentials = credentials
		account.Extra = quotaLeaseDemoAnyMapFromPayload(task.LoginPayload["extra"])
		account.Status = quotaLeaseDemoStringFromPayload(task.LoginPayload["status"])
		if schedulable, ok := quotaLeaseDemoBoolFromPayload(task.LoginPayload["schedulable"]); ok {
			account.Schedulable = schedulable
		}
	}
	account = normalizeQuotaLeaseDemoAccountSnapshot(account, &task, now)
	if account.ID <= 0 || account.Platform == "" || account.Type == "" {
		return QuotaLeaseDemoAccountSnapshot{}, fmt.Errorf("%w: account snapshot is incomplete", ErrQuotaLeaseDemoInvalidInput)
	}
	return account, nil
}

type QuotaLeaseDemoAccountLoginProgressError struct {
	Status            string
	Message           string
	LoginPayloadPatch map[string]any
	MetadataPatch     map[string]string
}

func (e *QuotaLeaseDemoAccountLoginProgressError) Error() string {
	if e == nil {
		return ""
	}
	if strings.TrimSpace(e.Message) != "" {
		return strings.TrimSpace(e.Message)
	}
	return "account login task progress"
}

type QuotaLeaseDemoOAuthAccountTaskExecutor struct {
	payloadExecutor *QuotaLeaseDemoPayloadAccountTaskExecutor
	openaiOAuth     *OpenAIOAuthService
	grokOAuth       *GrokOAuthService
}

func NewQuotaLeaseDemoOAuthAccountTaskExecutor(openaiOAuth *OpenAIOAuthService, grokOAuth *GrokOAuthService) *QuotaLeaseDemoOAuthAccountTaskExecutor {
	return &QuotaLeaseDemoOAuthAccountTaskExecutor{
		payloadExecutor: NewQuotaLeaseDemoPayloadAccountTaskExecutor(),
		openaiOAuth:     openaiOAuth,
		grokOAuth:       grokOAuth,
	}
}

func (e *QuotaLeaseDemoOAuthAccountTaskExecutor) ExecuteAccountLoginTask(ctx context.Context, task QuotaLeaseDemoAccountLoginTask) (QuotaLeaseDemoAccountSnapshot, error) {
	if _, ok := task.LoginPayload["account"]; ok {
		return e.payloadExecutor.ExecuteAccountLoginTask(ctx, task)
	}
	if _, ok := task.LoginPayload["credentials"]; ok && quotaLeaseDemoStringFromPayload(task.LoginPayload["code"]) == "" {
		return e.payloadExecutor.ExecuteAccountLoginTask(ctx, task)
	}
	switch strings.TrimSpace(task.Platform) {
	case PlatformOpenAI:
		return e.executeOpenAI(ctx, task)
	case PlatformGrok:
		return e.executeGrok(ctx, task)
	default:
		return e.payloadExecutor.ExecuteAccountLoginTask(ctx, task)
	}
}

func (e *QuotaLeaseDemoOAuthAccountTaskExecutor) executeOpenAI(ctx context.Context, task QuotaLeaseDemoAccountLoginTask) (QuotaLeaseDemoAccountSnapshot, error) {
	if e.openaiOAuth == nil {
		return QuotaLeaseDemoAccountSnapshot{}, fmt.Errorf("%w: openai oauth service is required", ErrQuotaLeaseDemoInvalidInput)
	}
	code, state := quotaLeaseDemoOAuthCodeAndState(task.LoginPayload)
	sessionID := quotaLeaseDemoStringFromPayload(task.LoginPayload["session_id"])
	redirectURI := quotaLeaseDemoStringFromPayload(task.LoginPayload["redirect_uri"])
	proxyID := quotaLeaseDemoProxyIDFromPayload(task.LoginPayload["proxy_id"])
	if code != "" && sessionID != "" {
		tokenInfo, err := e.openaiOAuth.ExchangeCode(ctx, &OpenAIExchangeCodeInput{
			SessionID:   sessionID,
			Code:        code,
			State:       state,
			RedirectURI: redirectURI,
			ProxyID:     proxyID,
		})
		if err != nil {
			return QuotaLeaseDemoAccountSnapshot{}, err
		}
		return quotaLeaseDemoSnapshotFromOAuthCredentials(task, e.openaiOAuth.BuildAccountCredentials(tokenInfo), task.LoginPayload), nil
	}

	result, err := e.openaiOAuth.GenerateAuthURL(ctx, proxyID, redirectURI, task.Platform)
	if err != nil {
		return QuotaLeaseDemoAccountSnapshot{}, err
	}
	patch := map[string]any{
		"auth_url":              result.AuthURL,
		"session_id":            result.SessionID,
		"auth_url_generated_at": time.Now().UTC().Format(time.RFC3339),
		"oauth_provider":        PlatformOpenAI,
	}
	if state := quotaLeaseDemoQueryValue(result.AuthURL, "state"); state != "" {
		patch["state"] = state
	}
	if redirect := quotaLeaseDemoQueryValue(result.AuthURL, "redirect_uri"); redirect != "" {
		patch["redirect_uri"] = redirect
	}
	return QuotaLeaseDemoAccountSnapshot{}, &QuotaLeaseDemoAccountLoginProgressError{
		Status:            QuotaLeaseDemoAccountTaskWaiting,
		Message:           "openai oauth authorization url generated",
		LoginPayloadPatch: patch,
	}
}

func (e *QuotaLeaseDemoOAuthAccountTaskExecutor) executeGrok(ctx context.Context, task QuotaLeaseDemoAccountLoginTask) (QuotaLeaseDemoAccountSnapshot, error) {
	if e.grokOAuth == nil {
		return QuotaLeaseDemoAccountSnapshot{}, fmt.Errorf("%w: grok oauth service is required", ErrQuotaLeaseDemoInvalidInput)
	}
	proxyID := quotaLeaseDemoProxyIDFromPayload(task.LoginPayload["proxy_id"])
	if ssoToken := quotaLeaseDemoStringFromPayload(task.LoginPayload["sso_token"]); ssoToken != "" {
		tokenInfo, err := e.grokOAuth.ConvertFromSSO(ctx, ssoToken, proxyID)
		if err != nil {
			return QuotaLeaseDemoAccountSnapshot{}, err
		}
		return quotaLeaseDemoSnapshotFromOAuthCredentials(task, e.grokOAuth.BuildAccountCredentials(tokenInfo), task.LoginPayload), nil
	}

	code, state := quotaLeaseDemoOAuthCodeAndState(task.LoginPayload)
	sessionID := quotaLeaseDemoStringFromPayload(task.LoginPayload["session_id"])
	redirectURI := quotaLeaseDemoStringFromPayload(task.LoginPayload["redirect_uri"])
	if code != "" && sessionID != "" {
		tokenInfo, err := e.grokOAuth.ExchangeCode(ctx, &GrokExchangeCodeInput{
			SessionID:   sessionID,
			Code:        code,
			State:       state,
			RedirectURI: redirectURI,
			ProxyID:     proxyID,
		})
		if err != nil {
			return QuotaLeaseDemoAccountSnapshot{}, err
		}
		return quotaLeaseDemoSnapshotFromOAuthCredentials(task, e.grokOAuth.BuildAccountCredentials(tokenInfo), task.LoginPayload), nil
	}

	result, err := e.grokOAuth.GenerateAuthURL(ctx, proxyID, redirectURI)
	if err != nil {
		return QuotaLeaseDemoAccountSnapshot{}, err
	}
	patch := map[string]any{
		"auth_url":              result.AuthURL,
		"session_id":            result.SessionID,
		"state":                 result.State,
		"auth_url_generated_at": time.Now().UTC().Format(time.RFC3339),
		"oauth_provider":        PlatformGrok,
	}
	if redirect := quotaLeaseDemoQueryValue(result.AuthURL, "redirect_uri"); redirect != "" {
		patch["redirect_uri"] = redirect
	}
	return QuotaLeaseDemoAccountSnapshot{}, &QuotaLeaseDemoAccountLoginProgressError{
		Status:            QuotaLeaseDemoAccountTaskWaiting,
		Message:           "grok oauth authorization url generated",
		LoginPayloadPatch: patch,
	}
}

type QuotaLeaseDemoNodeWorker struct {
	svc       *QuotaLeaseDemoService
	executor  QuotaLeaseDemoAccountTaskExecutor
	interval  time.Duration
	cancel    context.CancelFunc
	done      chan struct{}
	startOnce sync.Once
	stopOnce  sync.Once
}

func NewQuotaLeaseDemoNodeWorker(svc *QuotaLeaseDemoService, executor QuotaLeaseDemoAccountTaskExecutor, interval time.Duration) *QuotaLeaseDemoNodeWorker {
	if interval <= 0 {
		interval = quotaLeaseDemoNodeWorkerInterval
	}
	if executor == nil {
		executor = NewQuotaLeaseDemoPayloadAccountTaskExecutor()
	}
	return &QuotaLeaseDemoNodeWorker{
		svc:      svc,
		executor: executor,
		interval: interval,
	}
}

func ProvideQuotaLeaseDemoNodeWorker(cfg *config.Config, openaiOAuth *OpenAIOAuthService, grokOAuth *GrokOAuthService) *QuotaLeaseDemoNodeWorker {
	worker := NewQuotaLeaseDemoNodeWorker(
		GetQuotaLeaseDemoService(cfg),
		NewQuotaLeaseDemoOAuthAccountTaskExecutor(openaiOAuth, grokOAuth),
		quotaLeaseDemoNodeWorkerInterval,
	)
	worker.Start(context.Background())
	return worker
}

func (w *QuotaLeaseDemoNodeWorker) Start(parent context.Context) {
	if w == nil || w.svc == nil || !w.svc.remoteMode() {
		return
	}
	if parent == nil {
		parent = context.Background()
	}
	w.startOnce.Do(func() {
		ctx, cancel := context.WithCancel(parent)
		w.cancel = cancel
		w.done = make(chan struct{})
		go w.loop(ctx)
	})
}

func (w *QuotaLeaseDemoNodeWorker) Stop() {
	if w == nil {
		return
	}
	w.stopOnce.Do(func() {
		if w.cancel != nil {
			w.cancel()
		}
		if w.done != nil {
			<-w.done
		}
	})
}

func (w *QuotaLeaseDemoNodeWorker) RunOnce(ctx context.Context) error {
	if w == nil || w.svc == nil || !w.svc.remoteMode() {
		return nil
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if w.executor == nil {
		return fmt.Errorf("%w: account task executor is required", ErrQuotaLeaseDemoInvalidInput)
	}

	var combined error
	if err := w.svc.SyncAssignedAccounts(ctx); err != nil {
		combined = errors.Join(combined, err)
	}
	for _, status := range []string{QuotaLeaseDemoAccountTaskPending, QuotaLeaseDemoAccountTaskReady} {
		tasks, err := w.svc.fetchRemoteAccountLoginTasks(ctx, status)
		if err != nil {
			combined = errors.Join(combined, err)
			continue
		}
		for _, task := range tasks {
			if strings.TrimSpace(task.Status) != "" && task.Status != status {
				continue
			}
			if err := w.executeAccountLoginTask(ctx, task); err != nil {
				combined = errors.Join(combined, err)
			}
		}
	}
	if err := w.svc.SyncAssignedAccounts(ctx); err != nil {
		combined = errors.Join(combined, err)
	}
	if err := w.svc.FlushPendingUsage(ctx); err != nil {
		combined = errors.Join(combined, err)
	}
	if err := w.svc.FlushPendingUsageLogs(ctx); err != nil {
		combined = errors.Join(combined, err)
	}
	return combined
}

func (w *QuotaLeaseDemoNodeWorker) loop(ctx context.Context) {
	defer close(w.done)
	w.runOnceWithTimeout(ctx)
	ticker := time.NewTicker(w.interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			w.runOnceWithTimeout(ctx)
		}
	}
}

func (w *QuotaLeaseDemoNodeWorker) runOnceWithTimeout(ctx context.Context) {
	runCtx, cancel := context.WithTimeout(ctx, quotaLeaseDemoNodeWorkerRunTimeout)
	defer cancel()
	if err := w.RunOnce(runCtx); err != nil && !errors.Is(err, context.Canceled) {
		slog.Warn("quota lease demo node worker run failed", "error", err)
	}
}

func (w *QuotaLeaseDemoNodeWorker) executeAccountLoginTask(ctx context.Context, task QuotaLeaseDemoAccountLoginTask) error {
	account, execErr := w.executor.ExecuteAccountLoginTask(ctx, task)
	var progressErr *QuotaLeaseDemoAccountLoginProgressError
	if errors.As(execErr, &progressErr) && progressErr != nil {
		status := strings.TrimSpace(progressErr.Status)
		if status == "" {
			status = QuotaLeaseDemoAccountTaskWaiting
		}
		_, progressReportErr := w.svc.ReportAccountLoginTaskProgress(ctx, QuotaLeaseDemoAccountLoginTaskProgressRequest{
			TaskID:            task.ID,
			Status:            status,
			Error:             progressErr.Message,
			LoginPayloadPatch: progressErr.LoginPayloadPatch,
			MetadataPatch:     progressErr.MetadataPatch,
		})
		if progressReportErr != nil {
			return fmt.Errorf("account login task %s: %w", task.ID, errors.Join(execErr, progressReportErr))
		}
		return nil
	}
	req := QuotaLeaseDemoAccountLoginTaskCompleteRequest{
		TaskID: task.ID,
	}
	if execErr != nil {
		req.Error = execErr.Error()
	} else {
		req.Account = account
	}
	_, completeErr := w.svc.CompleteAccountLoginTask(ctx, req)
	switch {
	case execErr != nil && completeErr != nil:
		return fmt.Errorf("account login task %s: %w", task.ID, errors.Join(execErr, completeErr))
	case execErr != nil:
		return fmt.Errorf("account login task %s: %w", task.ID, execErr)
	case completeErr != nil:
		return fmt.Errorf("account login task %s: %w", task.ID, completeErr)
	default:
		return nil
	}
}

func decodeQuotaLeaseDemoAccountSnapshot(raw any, account *QuotaLeaseDemoAccountSnapshot) error {
	payload, err := json.Marshal(raw)
	if err != nil {
		return err
	}
	if err := json.Unmarshal(payload, account); err != nil {
		return err
	}
	return nil
}

func quotaLeaseDemoAnyMapFromPayload(raw any) map[string]any {
	if raw == nil {
		return nil
	}
	if value, ok := raw.(map[string]any); ok {
		return cloneQuotaLeaseDemoAnyMap(value)
	}
	payload, err := json.Marshal(raw)
	if err != nil {
		return nil
	}
	var value map[string]any
	if err := json.Unmarshal(payload, &value); err != nil {
		return nil
	}
	return cloneQuotaLeaseDemoAnyMap(value)
}

func quotaLeaseDemoStringFromPayload(raw any) string {
	if value, ok := raw.(string); ok {
		return strings.TrimSpace(value)
	}
	return ""
}

func quotaLeaseDemoBoolFromPayload(raw any) (bool, bool) {
	if value, ok := raw.(bool); ok {
		return value, true
	}
	return false, false
}

func quotaLeaseDemoSnapshotFromOAuthCredentials(task QuotaLeaseDemoAccountLoginTask, credentials map[string]any, payload map[string]any) QuotaLeaseDemoAccountSnapshot {
	now := time.Now().UTC()
	credentials = mergeQuotaLeaseDemoAnyPatch(credentials, quotaLeaseDemoAnyMapFromPayload(payload["credential_overrides"]))
	proxyID := quotaLeaseDemoProxyIDFromPayload(payload["proxy_id"])
	account := QuotaLeaseDemoAccountSnapshot{
		ID:          task.AccountID,
		Name:        task.Name,
		Platform:    task.Platform,
		Type:        task.Type,
		Credentials: cloneQuotaLeaseDemoAnyMap(credentials),
		Extra:       quotaLeaseDemoAnyMapFromPayload(payload["extra"]),
		ProxyID:     proxyID,
		Status:      StatusActive,
		Schedulable: true,
		Concurrency: task.Concurrency,
		Priority:    task.Priority,
		GroupIDs:    cloneQuotaLeaseDemoInt64Slice(task.GroupIDs),
		UpdatedAt:   now,
	}
	if status := quotaLeaseDemoStringFromPayload(payload["status"]); status != "" {
		account.Status = status
	}
	if schedulable, ok := quotaLeaseDemoBoolFromPayload(payload["schedulable"]); ok {
		account.Schedulable = schedulable
	}
	return normalizeQuotaLeaseDemoAccountSnapshot(account, &task, now)
}

func quotaLeaseDemoOAuthCodeAndState(payload map[string]any) (string, string) {
	code := quotaLeaseDemoStringFromPayload(payload["code"])
	state := quotaLeaseDemoStringFromPayload(payload["state"])
	callbackURL := quotaLeaseDemoStringFromPayload(payload["callback_url"])
	if callbackURL == "" {
		return code, state
	}
	if parsed, err := url.Parse(callbackURL); err == nil {
		query := parsed.Query()
		if code == "" {
			code = strings.TrimSpace(query.Get("code"))
		}
		if state == "" {
			state = strings.TrimSpace(query.Get("state"))
		}
	}
	return code, state
}

func quotaLeaseDemoQueryValue(rawURL, key string) string {
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(parsed.Query().Get(key))
}

func quotaLeaseDemoProxyIDFromPayload(raw any) *int64 {
	value, ok := quotaLeaseDemoInt64FromPayload(raw)
	if !ok || value <= 0 {
		return nil
	}
	return &value
}

func quotaLeaseDemoInt64FromPayload(raw any) (int64, bool) {
	switch value := raw.(type) {
	case int:
		return int64(value), true
	case int64:
		return value, true
	case float64:
		return int64(value), true
	case json.Number:
		parsed, err := value.Int64()
		return parsed, err == nil
	case string:
		if strings.TrimSpace(value) == "" {
			return 0, false
		}
		parsed, err := json.Number(strings.TrimSpace(value)).Int64()
		return parsed, err == nil
	default:
		return 0, false
	}
}
