package service

import (
	"context"
	"errors"
	"log/slog"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/yeteam"
)

var (
	yeTeamRCLCardCodePattern  = regexp.MustCompile(`(?i)(RCL-[A-Z0-9][A-Z0-9-]{3,})`)
	yeTeamTeamCardCodePattern = regexp.MustCompile(`(?i)(team-[A-Z0-9][A-Z0-9-]{3,})`)
)

const yeTeamAutoReclaimTimeout = 15 * time.Minute
const yeTeamRefreshStatusPersistTimeout = 5 * time.Second

const (
	yeTeamLastRefreshStatusKey = "ye_team_last_refresh_status"
	yeTeamLastRefreshAtKey     = "ye_team_last_refresh_at"
	yeTeamLastRefreshErrorKey  = "ye_team_last_refresh_error"
	yeTeamRefreshStatusSuccess = "success"
	yeTeamRefreshStatusFailed  = "failed"
	yeTeamRefreshErrorMaxRunes = 300
)

// SetYeTeamClient attaches the optional external reclaim integration after
// normal service construction, preserving existing test constructors.
func (s *OpenAIGatewayService) SetYeTeamClient(client *yeteam.Client) {
	if s != nil {
		s.yeTeam = client
	}
}

func (s *OpenAIGatewayService) reclaimOpenAIAccount401(ctx context.Context, account *Account) bool {
	if s == nil || account == nil || account.Platform != PlatformOpenAI || s.yeTeam == nil || !s.yeTeam.AutoRefresh401Enabled() {
		return false
	}
	cardCode := yeTeamCardCode(account)
	if cardCode == "" || s.accountRepo == nil {
		return false
	}
	// A reclaim repairs shared account state and must finish after the initiating
	// browser/SSE request disconnects. Keep request values for logging and apply
	// an independent bound to the external polling workflow.
	reclaimCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), yeTeamAutoReclaimTimeout)
	defer cancel()
	beforeCredential := account.GetCredential("access_token")
	if beforeCredential == "" {
		beforeCredential = account.GetCredential("api_key")
	}
	lock := s.yeTeamReclaimLock(account.ID)
	lock.Lock()
	defer lock.Unlock()
	currentCredential := account.GetCredential("access_token")
	if currentCredential == "" {
		currentCredential = account.GetCredential("api_key")
	}
	if beforeCredential != "" && currentCredential != "" && beforeCredential != currentCredential {
		if err := s.invalidateOpenAITokenCache(reclaimCtx, account); err != nil {
			return s.failYeTeamReclaim(reclaimCtx, account, "ye_team_auto_reclaim_cache_invalidate_failed", err)
		}
		s.recordYeTeamReclaimResult(reclaimCtx, account, yeTeamRefreshStatusSuccess, nil, nil)
		return true
	}
	packages, err := s.yeTeam.Reclaim401Packages(reclaimCtx, cardCode)
	if err != nil {
		return s.failYeTeamReclaim(reclaimCtx, account, "ye_team_auto_reclaim_failed", err)
	}
	hints := []string{account.Name, account.GetCredential("email"), account.GetChatGPTAccountID()}
	var matched yeteam.AccountCredentials
	var matchErr error
	for _, packageData := range packages {
		matched, matchErr = yeteam.FindAccountCredentials(packageData, hints...)
		if matchErr == nil {
			break
		}
	}
	if matchErr != nil {
		return s.failYeTeamReclaim(reclaimCtx, account, "ye_team_auto_reclaim_match_failed", matchErr)
	}
	matched.Credentials = shallowCopyMap(matched.Credentials)
	matched.Credentials["_token_version"] = nextYeTeamCredentialVersion(account)
	replacementCredential := strings.TrimSpace(credentialFromMap(matched.Credentials))
	if replacementCredential == "" {
		return s.failYeTeamReclaim(reclaimCtx, account, "ye_team_auto_reclaim_credential_missing", errors.New("ye.team replacement credentials did not include access_token or api_key"))
	}
	if beforeCredential != "" && replacementCredential == beforeCredential {
		return s.failYeTeamReclaim(reclaimCtx, account, "ye_team_auto_reclaim_credential_unchanged", errors.New("ye.team returned the current credential without a replacement"))
	}
	if err := persistAccountCredentials(reclaimCtx, s.accountRepo, account, matched.Credentials); err != nil {
		return s.failYeTeamReclaim(reclaimCtx, account, "ye_team_auto_reclaim_persist_failed", err)
	}
	if err := s.invalidateOpenAITokenCache(reclaimCtx, account); err != nil {
		return s.failYeTeamReclaim(reclaimCtx, account, "ye_team_auto_reclaim_cache_invalidate_failed", err)
	}
	s.recordYeTeamReclaimResult(reclaimCtx, account, yeTeamRefreshStatusSuccess, nil, matched.Extra)
	slog.Info("ye_team_auto_reclaim_succeeded", "account_id", account.ID, "credential_changed", true, "token_cache_invalidated", true)
	return true
}

func (s *OpenAIGatewayService) failYeTeamReclaim(ctx context.Context, account *Account, event string, err error) bool {
	slog.Warn(event, "account_id", account.ID, "error", err)
	s.recordYeTeamReclaimResult(ctx, account, yeTeamRefreshStatusFailed, err, nil)
	return false
}

func (s *OpenAIGatewayService) recordYeTeamReclaimResult(ctx context.Context, account *Account, status string, reclaimErr error, extra map[string]any) {
	if s == nil || s.accountRepo == nil || account == nil {
		return
	}
	updates := shallowCopyMap(extra)
	if updates == nil {
		updates = make(map[string]any, 3)
	}
	updates[yeTeamLastRefreshStatusKey] = status
	updates[yeTeamLastRefreshAtKey] = time.Now().UTC().Format(time.RFC3339)
	updates[yeTeamLastRefreshErrorKey] = compactYeTeamRefreshError(reclaimErr)
	persistCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), yeTeamRefreshStatusPersistTimeout)
	defer cancel()
	if err := s.accountRepo.UpdateExtra(persistCtx, account.ID, updates); err != nil {
		slog.Warn("ye_team_auto_reclaim_status_persist_failed", "account_id", account.ID, "status", status, "error", err)
		return
	}
	if account.Extra == nil {
		account.Extra = make(map[string]any, len(updates))
	}
	for key, value := range updates {
		account.Extra[key] = value
	}
}

func compactYeTeamRefreshError(err error) string {
	if err == nil {
		return ""
	}
	message := strings.Join(strings.Fields(err.Error()), " ")
	runes := []rune(message)
	if len(runes) <= yeTeamRefreshErrorMaxRunes {
		return message
	}
	return string(runes[:yeTeamRefreshErrorMaxRunes]) + "..."
}

func (s *OpenAIGatewayService) invalidateOpenAITokenCache(ctx context.Context, account *Account) error {
	if s == nil || s.openAITokenProvider == nil {
		return nil
	}
	return s.openAITokenProvider.invalidateAccessToken(ctx, account)
}

func credentialFromMap(credentials map[string]any) string {
	for _, key := range []string{"access_token", "api_key"} {
		if value, ok := credentials[key].(string); ok && strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func nextYeTeamCredentialVersion(account *Account) int64 {
	version := time.Now().UnixMilli()
	if account != nil {
		if current := account.GetCredentialAsInt64("_token_version"); version <= current {
			return current + 1
		}
	}
	return version
}

func yeTeamCardCode(account *Account) string {
	if account == nil {
		return ""
	}
	for _, source := range []map[string]any{account.Extra, account.Credentials} {
		for _, key := range []string{"ye_team_card_code", "card_code", "cdk"} {
			if value, ok := source[key].(string); ok {
				if card := normalizeYeTeamCardCode(value); card != "" {
					return card
				}
			}
		}
	}
	matches := yeTeamRCLCardCodePattern.FindAllString(account.Name, -1)
	if len(matches) == 0 {
		return ""
	}
	return normalizeYeTeamCardCode(matches[len(matches)-1])
}

func normalizeYeTeamCardCode(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	for _, pattern := range []*regexp.Regexp{yeTeamRCLCardCodePattern, yeTeamTeamCardCodePattern} {
		if pattern.MatchString(value) {
			return strings.ToUpper(pattern.FindString(value))
		}
	}
	return ""
}

// Keep the mutexes local to the gateway service so independent test services
// and separate application instances do not share process state.
func (s *OpenAIGatewayService) yeTeamReclaimLock(accountID int64) *sync.Mutex {
	if value, ok := s.yeTeamReclaimLocks.Load(accountID); ok {
		return value.(*sync.Mutex)
	}
	lock := &sync.Mutex{}
	actual, _ := s.yeTeamReclaimLocks.LoadOrStore(accountID, lock)
	return actual.(*sync.Mutex)
}
