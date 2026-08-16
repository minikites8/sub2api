package service

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"maps"
	"strconv"
	"strings"
	"sync"
	"time"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/google/uuid"
	"golang.org/x/sync/errgroup"
)

const (
	OpenAIAccountGuardEnabledExtraKey         = "openai_account_guard_enabled"
	OpenAIAccountGuardIntervalMinutesExtraKey = "openai_account_guard_interval_minutes"
	OpenAIAccountGuardLastRunAtExtraKey       = "openai_account_guard_last_run_at"

	OpenAIAccountGuardDefaultIntervalMinutes = 30
	OpenAIAccountGuardMinIntervalMinutes     = 5
	OpenAIAccountGuardMaxIntervalMinutes     = 24 * 60

	openAIAccountGuardCycleInterval = time.Minute
	openAIAccountGuardConcurrency   = 6
	openAIAccountGuardLeaderLockKey = "openai:account:guard:leader"
	openAIAccountGuardLeaderLockTTL = 30 * time.Minute
)

type openAIAccountGuardRepository interface {
	FindByExtraField(ctx context.Context, key string, value any) ([]Account, error)
	UpdateExtra(ctx context.Context, id int64, updates map[string]any) error
}

type openAIAccountGuardSessionService interface {
	ListSessions(ctx context.Context, accountID int64) (*OpenAISessionsResponse, error)
	RevokeSession(ctx context.Context, accountID int64, sessionID string) (*OpenAISessionRevokeResult, error)
}

// OpenAIAccountGuardService periodically removes every non-current ChatGPT
// device session from opted-in OpenAI OAuth accounts.
type OpenAIAccountGuardService struct {
	accountRepo openAIAccountGuardRepository
	sessions    openAIAccountGuardSessionService

	parentCtx    context.Context
	parentCancel context.CancelFunc
	wg           sync.WaitGroup
	mu           sync.Mutex
	cycleMu      sync.Mutex
	started      bool
	stopped      bool
	now          func() time.Time
	lockCache    LeaderLockCache
	db           *sql.DB
	instanceID   string
}

func NewOpenAIAccountGuardService(
	accountRepo openAIAccountGuardRepository,
	sessions openAIAccountGuardSessionService,
) *OpenAIAccountGuardService {
	ctx, cancel := context.WithCancel(context.Background())
	return &OpenAIAccountGuardService{
		accountRepo:  accountRepo,
		sessions:     sessions,
		parentCtx:    ctx,
		parentCancel: cancel,
		now:          time.Now,
		instanceID:   uuid.NewString(),
	}
}

func (s *OpenAIAccountGuardService) SetLeaderLock(lockCache LeaderLockCache, db *sql.DB) {
	if s == nil {
		return
	}
	s.lockCache = lockCache
	s.db = db
}

// ProvideOpenAIAccountGuardService starts the account guard runner.
func ProvideOpenAIAccountGuardService(
	accountRepo AccountRepository,
	sessions *OpenAISessionService,
	lockCache LeaderLockCache,
	db *sql.DB,
) *OpenAIAccountGuardService {
	svc := NewOpenAIAccountGuardService(accountRepo, sessions)
	svc.SetLeaderLock(lockCache, db)
	svc.Start()
	return svc
}

func (s *OpenAIAccountGuardService) Start() {
	if s == nil {
		return
	}
	s.mu.Lock()
	if s.started || s.stopped {
		s.mu.Unlock()
		return
	}
	s.started = true
	s.wg.Add(1)
	s.mu.Unlock()
	go s.runLoop()
}

func (s *OpenAIAccountGuardService) Stop() {
	if s == nil {
		return
	}
	s.mu.Lock()
	if s.stopped {
		s.mu.Unlock()
		return
	}
	s.stopped = true
	s.parentCancel()
	s.mu.Unlock()
	s.wg.Wait()
}

func (s *OpenAIAccountGuardService) runLoop() {
	defer s.wg.Done()
	if err := s.RunDue(s.parentCtx); err != nil {
		slog.Warn("openai_account_guard_run_failed", "error", err)
	}
	ticker := time.NewTicker(openAIAccountGuardCycleInterval)
	defer ticker.Stop()
	for {
		select {
		case <-s.parentCtx.Done():
			return
		case <-ticker.C:
			if err := s.RunDue(s.parentCtx); err != nil {
				slog.Warn("openai_account_guard_run_failed", "error", err)
			}
		}
	}
}

// RunDue scans enabled accounts and guards every account whose interval has elapsed.
func (s *OpenAIAccountGuardService) RunDue(ctx context.Context) error {
	if s == nil || s.accountRepo == nil || s.sessions == nil {
		return nil
	}
	s.cycleMu.Lock()
	defer s.cycleMu.Unlock()

	release, acquired, err := s.tryAcquireLeaderLock(ctx)
	if err != nil {
		return fmt.Errorf("acquire OpenAI account guard leader lock: %w", err)
	}
	if !acquired {
		return nil
	}
	defer release()

	accounts, err := s.accountRepo.FindByExtraField(ctx, OpenAIAccountGuardEnabledExtraKey, true)
	if err != nil {
		return fmt.Errorf("list guarded OpenAI accounts: %w", err)
	}
	now := s.currentTime().UTC()
	due := make([]Account, 0, len(accounts))
	for i := range accounts {
		account := accounts[i]
		if account.IsShadow() || account.Platform != PlatformOpenAI || account.Type != AccountTypeOAuth {
			continue
		}
		lastRun := openAIAccountGuardLastRunAt(account.Extra)
		interval := time.Duration(openAIAccountGuardIntervalMinutes(account.Extra)) * time.Minute
		if !lastRun.IsZero() && now.Before(lastRun.Add(interval)) {
			continue
		}
		due = append(due, account)
	}

	var group errgroup.Group
	group.SetLimit(openAIAccountGuardConcurrency)
	var errorsMu sync.Mutex
	var runErrors []error
	for i := range due {
		accountID := due[i].ID
		group.Go(func() error {
			revoked, guardErr := revokeOtherOpenAISessions(ctx, s.sessions, accountID)
			updateErr := s.accountRepo.UpdateExtra(ctx, accountID, map[string]any{
				OpenAIAccountGuardLastRunAtExtraKey: now.Format(time.RFC3339Nano),
			})
			if guardErr == nil && updateErr == nil {
				slog.Info("openai_account_guard_completed", "account_id", accountID, "revoked", revoked)
				return nil
			}
			combined := errors.Join(guardErr, updateErr)
			slog.Warn("openai_account_guard_account_failed", "account_id", accountID, "revoked", revoked, "error", combined)
			errorsMu.Lock()
			runErrors = append(runErrors, fmt.Errorf("account %d: %w", accountID, combined))
			errorsMu.Unlock()
			return nil
		})
	}
	_ = group.Wait()
	return errors.Join(runErrors...)
}

func (s *OpenAIAccountGuardService) currentTime() time.Time {
	if s.now == nil {
		return time.Now()
	}
	return s.now()
}

func (s *OpenAIAccountGuardService) tryAcquireLeaderLock(ctx context.Context) (func(), bool, error) {
	lockCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	if s.lockCache != nil {
		acquired, err := s.lockCache.TryAcquireLeaderLock(lockCtx, openAIAccountGuardLeaderLockKey, s.instanceID, openAIAccountGuardLeaderLockTTL)
		if err != nil || !acquired {
			return nil, acquired, err
		}
		return func() {
			releaseCtx, releaseCancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer releaseCancel()
			_ = s.lockCache.ReleaseLeaderLock(releaseCtx, openAIAccountGuardLeaderLockKey, s.instanceID)
		}, true, nil
	}
	if s.db != nil {
		return tryAcquireDBAdvisoryLockWithError(lockCtx, s.db, hashAdvisoryLockID(openAIAccountGuardLeaderLockKey))
	}
	return func() {}, true, nil
}

func revokeOtherOpenAISessions(ctx context.Context, sessions openAIAccountGuardSessionService, accountID int64) (int, error) {
	listed, err := sessions.ListSessions(ctx, accountID)
	if err != nil {
		return 0, err
	}
	if listed == nil {
		return 0, errors.New("online sessions response is empty")
	}
	revoked := 0
	seen := make(map[string]struct{}, len(listed.Devices))
	var revokeErrors []error
	for _, device := range listed.Devices {
		sessionID := strings.TrimSpace(device.SessionID)
		if sessionID == "" || device.IsCurrentDevice {
			continue
		}
		if _, exists := seen[sessionID]; exists {
			continue
		}
		seen[sessionID] = struct{}{}
		if _, err := sessions.RevokeSession(ctx, accountID, sessionID); err != nil {
			revokeErrors = append(revokeErrors, fmt.Errorf("revoke session %s: %w", sessionID, err))
			continue
		}
		revoked++
	}
	return revoked, errors.Join(revokeErrors...)
}

func normalizeOpenAIAccountGuardExtra(platform, accountType string, extra map[string]any) (map[string]any, error) {
	normalized := maps.Clone(extra)
	if normalized == nil {
		normalized = make(map[string]any)
	}

	rawEnabled, enabledProvided := normalized[OpenAIAccountGuardEnabledExtraKey]
	enabled := false
	if enabledProvided {
		var ok bool
		enabled, ok = rawEnabled.(bool)
		if !ok {
			return nil, infraerrors.BadRequest("OPENAI_ACCOUNT_GUARD_ENABLED_INVALID", "openai_account_guard_enabled must be a boolean")
		}
	}

	if platform != PlatformOpenAI || accountType != AccountTypeOAuth {
		if enabled {
			return nil, infraerrors.BadRequest("OPENAI_ACCOUNT_GUARD_ACCOUNT_INVALID", "account guard requires an OpenAI OAuth account")
		}
		delete(normalized, OpenAIAccountGuardEnabledExtraKey)
		delete(normalized, OpenAIAccountGuardIntervalMinutesExtraKey)
		delete(normalized, OpenAIAccountGuardLastRunAtExtraKey)
		return normalized, nil
	}

	rawInterval, intervalProvided := normalized[OpenAIAccountGuardIntervalMinutesExtraKey]
	interval := OpenAIAccountGuardDefaultIntervalMinutes
	if intervalProvided {
		var ok bool
		interval, ok = accountGuardInteger(rawInterval)
		if !ok || interval < OpenAIAccountGuardMinIntervalMinutes || interval > OpenAIAccountGuardMaxIntervalMinutes {
			return nil, infraerrors.BadRequest(
				"OPENAI_ACCOUNT_GUARD_INTERVAL_INVALID",
				fmt.Sprintf("openai_account_guard_interval_minutes must be between %d and %d", OpenAIAccountGuardMinIntervalMinutes, OpenAIAccountGuardMaxIntervalMinutes),
			)
		}
	}
	if enabled {
		normalized[OpenAIAccountGuardEnabledExtraKey] = true
		normalized[OpenAIAccountGuardIntervalMinutesExtraKey] = interval
	} else {
		delete(normalized, OpenAIAccountGuardEnabledExtraKey)
		delete(normalized, OpenAIAccountGuardLastRunAtExtraKey)
		if intervalProvided {
			normalized[OpenAIAccountGuardIntervalMinutesExtraKey] = interval
		}
	}
	return normalized, nil
}

func accountGuardInteger(value any) (int, bool) {
	switch typed := value.(type) {
	case int:
		return typed, true
	case int64:
		return int(typed), int64(int(typed)) == typed
	case float64:
		converted := int(typed)
		return converted, float64(converted) == typed
	case json.Number:
		parsed, err := strconv.Atoi(typed.String())
		return parsed, err == nil
	case string:
		parsed, err := strconv.Atoi(strings.TrimSpace(typed))
		return parsed, err == nil
	default:
		return 0, false
	}
}

func openAIAccountGuardIntervalMinutes(extra map[string]any) int {
	interval, ok := accountGuardInteger(extra[OpenAIAccountGuardIntervalMinutesExtraKey])
	if !ok || interval < OpenAIAccountGuardMinIntervalMinutes || interval > OpenAIAccountGuardMaxIntervalMinutes {
		return OpenAIAccountGuardDefaultIntervalMinutes
	}
	return interval
}

func openAIAccountGuardLastRunAt(extra map[string]any) time.Time {
	raw, ok := extra[OpenAIAccountGuardLastRunAtExtraKey]
	if !ok {
		return time.Time{}
	}
	switch typed := raw.(type) {
	case string:
		parsed, err := time.Parse(time.RFC3339Nano, strings.TrimSpace(typed))
		if err == nil {
			return parsed
		}
	case time.Time:
		return typed
	}
	return time.Time{}
}
