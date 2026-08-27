package service

import (
	"context"
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
		if err := s.invalidateOpenAITokenCache(ctx, account); err != nil {
			slog.Warn("ye_team_auto_reclaim_cache_invalidate_failed", "account_id", account.ID, "error", err)
			return false
		}
		return true
	}
	packages, err := s.yeTeam.Reclaim401Packages(ctx, cardCode)
	if err != nil {
		slog.Warn("ye_team_auto_reclaim_failed", "account_id", account.ID, "error", err)
		return false
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
		slog.Warn("ye_team_auto_reclaim_match_failed", "account_id", account.ID, "error", matchErr)
		return false
	}
	matched.Credentials = shallowCopyMap(matched.Credentials)
	matched.Credentials["_token_version"] = nextYeTeamCredentialVersion(account)
	replacementCredential := strings.TrimSpace(credentialFromMap(matched.Credentials))
	if replacementCredential == "" {
		slog.Warn("ye_team_auto_reclaim_credential_missing", "account_id", account.ID)
		return false
	}
	if beforeCredential != "" && replacementCredential == beforeCredential {
		slog.Warn("ye_team_auto_reclaim_credential_unchanged", "account_id", account.ID)
		return false
	}
	if err := persistAccountCredentials(ctx, s.accountRepo, account, matched.Credentials); err != nil {
		slog.Warn("ye_team_auto_reclaim_persist_failed", "account_id", account.ID, "error", err)
		return false
	}
	if err := s.invalidateOpenAITokenCache(ctx, account); err != nil {
		slog.Warn("ye_team_auto_reclaim_cache_invalidate_failed", "account_id", account.ID, "error", err)
		return false
	}
	if len(matched.Extra) > 0 {
		if err := s.accountRepo.UpdateExtra(ctx, account.ID, matched.Extra); err == nil {
			if account.Extra == nil {
				account.Extra = make(map[string]any, len(matched.Extra))
			}
			for key, value := range matched.Extra {
				account.Extra[key] = value
			}
		}
	}
	slog.Info("ye_team_auto_reclaim_succeeded", "account_id", account.ID, "credential_changed", true, "token_cache_invalidated", true)
	return true
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
