package service

import (
	"context"
	"errors"
	"regexp"
	"slices"
	"strings"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
)

const OpenAICodexTicketGatewayBlacklistExtraKey = "openai_codex_ticket_gateway_blacklist"

var codexGatewayNamePattern = regexp.MustCompile(`^unified-[0-9]{1,5}$`)

// NormalizeOpenAICodexTicketGatewayBlacklistExtra stores a deduplicated list.
// Empty lists remain present so key-level updates can clear the setting.
func NormalizeOpenAICodexTicketGatewayBlacklistExtra(extra map[string]any) error {
	raw, exists := extra[OpenAICodexTicketGatewayBlacklistExtraKey]
	if !exists {
		return nil
	}
	values := []string{}
	switch value := raw.(type) {
	case nil:
	case string:
		values = strings.FieldsFunc(value, func(r rune) bool { return r == ',' || r == '\n' || r == '\r' })
	case []string:
		values = value
	case []any:
		for _, item := range value {
			text, ok := item.(string)
			if !ok {
				return invalidCodexGatewayBlacklist()
			}
			values = append(values, text)
		}
	default:
		return invalidCodexGatewayBlacklist()
	}
	gateways := make([]string, 0, len(values))
	for _, value := range values {
		if strings.TrimSpace(value) == "" {
			continue
		}
		gateway := normalizeCodex780Gateway(value)
		if !codexGatewayNamePattern.MatchString(gateway) {
			return invalidCodexGatewayBlacklist()
		}
		if !slices.Contains(gateways, gateway) {
			gateways = append(gateways, gateway)
		}
	}
	extra[OpenAICodexTicketGatewayBlacklistExtraKey] = gateways
	return nil
}

func invalidCodexGatewayBlacklist() error {
	return infraerrors.BadRequest("CODEX_GATEWAY_BLACKLIST_INVALID", "Gateway blacklist must contain unified-N gateway names")
}

func OpenAICodexTicketGatewayBlacklist(account *Account) []string {
	if account == nil {
		return []string{}
	}
	extra := map[string]any{OpenAICodexTicketGatewayBlacklistExtraKey: account.Extra[OpenAICodexTicketGatewayBlacklistExtraKey]}
	if NormalizeOpenAICodexTicketGatewayBlacklistExtra(extra) != nil {
		return []string{}
	}
	return extra[OpenAICodexTicketGatewayBlacklistExtraKey].([]string)
}

// CodexGatewayBlacklistRepository appends under a database row lock, preserving
// concurrent additions and unrelated account extra keys.
type CodexGatewayBlacklistRepository interface {
	AddCodexTicketGatewayToBlacklist(context.Context, int64, string) error
}

func (s *adminServiceImpl) AddCodexTicketGatewayToBlacklist(ctx context.Context, id int64, gateway string) (*Account, error) {
	gateway = normalizeCodex780Gateway(gateway)
	if !codexGatewayNamePattern.MatchString(gateway) {
		return nil, invalidCodexGatewayBlacklist()
	}
	account, err := s.accountRepo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if account == nil {
		return nil, ErrAccountNotFound
	}
	if !ObserverCanManageAccount(ctx, account) {
		return nil, ErrObserverScope
	}
	if !account.IsOpenAIOAuthLike() || account.IsShadow() {
		return nil, infraerrors.BadRequest("CODEX_TICKETS_UNSUPPORTED", "Account does not support Codex tickets")
	}
	repo, ok := s.accountRepo.(CodexGatewayBlacklistRepository)
	if !ok {
		return nil, errors.New("gateway blacklist repository unavailable")
	}
	if err := repo.AddCodexTicketGatewayToBlacklist(ctx, id, gateway); err != nil {
		return nil, err
	}
	return s.accountRepo.GetByID(ctx, id)
}
