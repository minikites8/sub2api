package service

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
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
	yeTeamLastRefreshFlowKey   = "ye_team_last_refresh_flow"
	yeTeamRefreshStatusRunning = "running"
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
	return s.reclaimOpenAIAccount(ctx, account, true)
}

// ReclaimOpenAIAccount performs a manual ye.team credential reset for an
// existing OpenAI account. Manual resets intentionally bypass the automatic
// 401 switch while still requiring the integration itself to be enabled.
func (s *OpenAIGatewayService) ReclaimOpenAIAccount(ctx context.Context, account *Account) error {
	if s == nil || account == nil {
		return errors.New("account is unavailable")
	}
	if account.Platform != PlatformOpenAI {
		return errors.New("ye.team reset requires an OpenAI account")
	}
	if s.yeTeam == nil || !s.yeTeam.Enabled() {
		return errors.New("ye.team integration is disabled")
	}
	if yeTeamCardCode(account) == "" {
		return errors.New("account has no ye.team card binding")
	}
	if !s.reclaimOpenAIAccount(ctx, account, false) {
		return errors.New("ye.team credential reset failed; check the account refresh status")
	}
	return nil
}

func (s *OpenAIGatewayService) reclaimOpenAIAccount(ctx context.Context, account *Account, automatic bool) bool {
	if s == nil || account == nil || account.Platform != PlatformOpenAI || s.yeTeam == nil || !s.yeTeam.Enabled() || (automatic && !s.yeTeam.AutoRefresh401Enabled()) {
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
		flow := yeteam.NewReclaimFlow()
		flow.Trigger = yeTeamRefreshTrigger(automatic)
		flow.CredentialChanged = boolPointer(true)
		flow.AddStage("cache_invalidate", "running", "credential already changed; invalidating local token cache")
		s.recordYeTeamReclaimResultWithFlow(reclaimCtx, account, yeTeamRefreshStatusRunning, nil, nil, &flow)
		if err := s.invalidateOpenAITokenCache(reclaimCtx, account); err != nil {
			flow.AddStage("cache_invalidate", "failed", err.Error())
			return s.failYeTeamReclaimWithFlow(reclaimCtx, account, "ye_team_auto_reclaim_cache_invalidate_failed", err, &flow)
		}
		flow.CacheInvalidated = true
		flow.AddStage("cache_invalidate", "success", "token cache invalidated")
		flow.AddStage("complete", "success", "another worker already persisted replacement credentials")
		s.recordYeTeamReclaimResultWithFlow(reclaimCtx, account, yeTeamRefreshStatusSuccess, nil, nil, &flow)
		return true
	}
	flow := yeteam.NewReclaimFlow()
	flow.Trigger = yeTeamRefreshTrigger(automatic)
	flow.AddStage("batch_reclaim", "running", "submitting ye.team reclaim request")
	s.recordYeTeamReclaimResultWithFlow(reclaimCtx, account, yeTeamRefreshStatusRunning, nil, nil, &flow)
	packages, clientFlow, err := s.yeTeam.Reclaim401PackagesWithTrace(reclaimCtx, cardCode)
	clientFlow.Trigger = yeTeamRefreshTrigger(automatic)
	if err != nil {
		return s.failYeTeamReclaimWithFlow(reclaimCtx, account, "ye_team_auto_reclaim_failed", err, &clientFlow)
	}
	flow = clientFlow
	appendYeTeamFlowStage(&flow, "match_credentials", "running", "matching downloaded account package")
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
		appendYeTeamFlowStage(&flow, "match_credentials", "failed", matchErr.Error())
		return s.failYeTeamReclaimWithFlow(reclaimCtx, account, "ye_team_auto_reclaim_match_failed", matchErr, &flow)
	}
	appendYeTeamFlowStage(&flow, "match_credentials", "success", "account package matched")
	matched.Credentials = shallowCopyMap(matched.Credentials)
	credentialPayloadChanged := yeTeamCredentialPayloadChanged(account.Credentials, matched.Credentials)
	matched.Credentials["_token_version"] = nextYeTeamCredentialVersion(account)
	replacementCredential := strings.TrimSpace(credentialFromMap(matched.Credentials))
	if replacementCredential == "" {
		err := errors.New("ye.team replacement credentials did not include access_token or api_key")
		appendYeTeamFlowStage(&flow, "credential_validation", "failed", err.Error())
		return s.failYeTeamReclaimWithFlow(reclaimCtx, account, "ye_team_auto_reclaim_credential_missing", err, &flow)
	}
	primaryCredentialChanged := beforeCredential == "" || replacementCredential != beforeCredential
	flow.CredentialChanged = boolPointer(credentialPayloadChanged)
	validationMessage := "replacement primary credential accepted"
	if !primaryCredentialChanged {
		validationMessage = "primary credential is unchanged; synchronizing the complete downloaded package"
	}
	appendYeTeamFlowStage(&flow, "credential_validation", "success", validationMessage)
	appendYeTeamFlowStage(&flow, "persist_credentials", "running", "writing replacement credentials")
	if err := persistAccountCredentials(reclaimCtx, s.accountRepo, account, matched.Credentials); err != nil {
		appendYeTeamFlowStage(&flow, "persist_credentials", "failed", err.Error())
		return s.failYeTeamReclaimWithFlow(reclaimCtx, account, "ye_team_auto_reclaim_persist_failed", err, &flow)
	}
	persisted, err := readBackYeTeamCredentials(reclaimCtx, s.accountRepo, account.ID, matched.Credentials)
	if err != nil {
		appendYeTeamFlowStage(&flow, "persist_credentials", "failed", err.Error())
		return s.failYeTeamReclaimWithFlow(reclaimCtx, account, "ye_team_auto_reclaim_readback_failed", err, &flow)
	}
	account.Credentials = shallowCopyMap(persisted.Credentials)
	appendYeTeamFlowStage(&flow, "persist_credentials", "success", "replacement credentials persisted and verified")
	appendYeTeamFlowStage(&flow, "cache_invalidate", "running", "invalidating local token cache")
	if err := s.invalidateOpenAITokenCache(reclaimCtx, account); err != nil {
		appendYeTeamFlowStage(&flow, "cache_invalidate", "failed", err.Error())
		return s.failYeTeamReclaimWithFlow(reclaimCtx, account, "ye_team_auto_reclaim_cache_invalidate_failed", err, &flow)
	}
	flow.CacheInvalidated = true
	appendYeTeamFlowStage(&flow, "cache_invalidate", "success", "token cache invalidated")
	appendYeTeamFlowStage(&flow, "complete", "success", "replacement credential ready for retry")
	s.recordYeTeamReclaimResultWithFlow(reclaimCtx, account, yeTeamRefreshStatusSuccess, nil, matched.Extra, &flow)
	slog.Info("ye_team_auto_reclaim_succeeded", "account_id", account.ID, "credential_changed", credentialPayloadChanged, "primary_credential_changed", primaryCredentialChanged, "token_cache_invalidated", true)
	return true
}

func yeTeamCredentialPayloadChanged(current, replacement map[string]any) bool {
	current = shallowCopyMap(current)
	replacement = shallowCopyMap(replacement)
	delete(current, "_token_version")
	delete(replacement, "_token_version")
	currentJSON, currentErr := json.Marshal(current)
	replacementJSON, replacementErr := json.Marshal(replacement)
	return currentErr != nil || replacementErr != nil || !bytes.Equal(currentJSON, replacementJSON)
}

func readBackYeTeamCredentials(ctx context.Context, repo AccountRepository, accountID int64, expected map[string]any) (*Account, error) {
	persisted, err := repo.GetByID(ctx, accountID)
	if err != nil {
		return nil, fmt.Errorf("read back ye.team credentials: %w", err)
	}
	if persisted == nil {
		return nil, errors.New("read back ye.team credentials: account is unavailable")
	}
	expectedJSON, err := json.Marshal(expected)
	if err != nil {
		return nil, fmt.Errorf("encode expected ye.team credentials: %w", err)
	}
	persistedJSON, err := json.Marshal(persisted.Credentials)
	if err != nil {
		return nil, fmt.Errorf("encode persisted ye.team credentials: %w", err)
	}
	if !bytes.Equal(expectedJSON, persistedJSON) {
		return nil, errors.New("ye.team replacement credentials did not match database readback")
	}
	return persisted, nil
}

func yeTeamRefreshTrigger(automatic bool) string {
	if automatic {
		return "automatic_401"
	}
	return "manual"
}

func boolPointer(value bool) *bool {
	return &value
}

func (s *OpenAIGatewayService) failYeTeamReclaim(ctx context.Context, account *Account, event string, err error) bool {
	return s.failYeTeamReclaimWithFlow(ctx, account, event, err, nil)
}

func (s *OpenAIGatewayService) failYeTeamReclaimWithFlow(ctx context.Context, account *Account, event string, err error, flow *yeteam.ReclaimFlow) bool {
	slog.Warn(event, "account_id", account.ID, "error", err)
	if flow != nil {
		appendYeTeamFlowStage(flow, "complete", "failed", err.Error())
	}
	s.recordYeTeamReclaimResultWithFlow(ctx, account, yeTeamRefreshStatusFailed, err, nil, flow)
	return false
}

func (s *OpenAIGatewayService) recordYeTeamReclaimResult(ctx context.Context, account *Account, status string, reclaimErr error, extra map[string]any) {
	s.recordYeTeamReclaimResultWithFlow(ctx, account, status, reclaimErr, extra, nil)
}

func (s *OpenAIGatewayService) recordYeTeamReclaimResultWithFlow(ctx context.Context, account *Account, status string, reclaimErr error, extra map[string]any, flow *yeteam.ReclaimFlow) {
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
	if flow != nil {
		flow.Status = status
		if status != yeTeamRefreshStatusRunning {
			flow.Finish(status)
		}
		updates[yeTeamLastRefreshFlowKey] = *flow
	}
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

func appendYeTeamFlowStage(flow *yeteam.ReclaimFlow, name, status, message string) {
	if flow != nil {
		flow.AddStage(name, status, message)
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
