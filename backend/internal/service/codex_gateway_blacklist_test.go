package service

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestNormalizeCodexGatewayBlacklist(t *testing.T) {
	for _, tc := range []struct {
		name    string
		raw     any
		want    []string
		invalid bool
	}{
		{name: "text and dedup", raw: " UNIFIED-12, unified-35\nunified12\r\n35 ", want: []string{"unified-12", "unified-35"}},
		{name: "array", raw: []any{"unified-12", "chat.gateway.unified-35.api.openai.com", "12"}, want: []string{"unified-12", "unified-35"}},
		{name: "empty", raw: " ,\n", want: []string{}},
		{name: "nil", raw: nil, want: []string{}},
		{name: "wildcard", raw: "any", invalid: true},
		{name: "malformed", raw: "unified-x", invalid: true},
		{name: "object", raw: map[string]any{"gateway": "unified-12"}, invalid: true},
		{name: "number", raw: []any{12}, invalid: true},
		{name: "header injection", raw: "unified-12\r\nX-Foo: injected", invalid: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			extra := map[string]any{OpenAICodexTicketGatewayBlacklistExtraKey: tc.raw, "other": true}
			err := NormalizeOpenAICodexTicketGatewayBlacklistExtra(extra)
			if tc.invalid {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			require.Equal(t, tc.want, extra[OpenAICodexTicketGatewayBlacklistExtraKey])
			require.True(t, extra["other"].(bool))
		})
	}
	require.NoError(t, NormalizeOpenAICodexTicketGatewayBlacklistExtra(nil))
	require.Empty(t, OpenAICodexTicketGatewayBlacklist(nil))
}

func TestCodexGatewayBlacklistAccountEdits(t *testing.T) {
	account := ticketTestAccount(41)
	account.Extra = map[string]any{"other": true}
	repo := &upstreamBillingProbeAccountRepo{accounts: map[int64]*Account{41: account}}
	svc := &adminServiceImpl{accountRepo: repo}
	key := OpenAICodexTicketGatewayBlacklistExtraKey
	updated, err := svc.UpdateAccount(context.Background(), 41, &UpdateAccountInput{Extra: map[string]any{key: "unified-12,unified-35"}})
	require.NoError(t, err)
	require.Equal(t, []string{"unified-12", "unified-35"}, OpenAICodexTicketGatewayBlacklist(updated))
	require.NoError(t, svc.UpdateAccountExtra(context.Background(), 41, map[string]any{key: ""}))
	require.Empty(t, OpenAICodexTicketGatewayBlacklist(repo.accounts[41]))
	require.Error(t, svc.UpdateAccountExtra(context.Background(), 41, map[string]any{key: []any{true}}))
	_, err = svc.BulkUpdateAccounts(context.Background(), &BulkUpdateAccountsInput{AccountIDs: []int64{41}, Extra: map[string]any{key: "any"}})
	require.Error(t, err)
	created, err := svc.CreateAccount(context.Background(), &CreateAccountInput{Platform: PlatformOpenAI, Type: AccountTypeOAuth, Credentials: map[string]any{"access_token": "test"}, SkipDefaultGroupBind: true, Extra: map[string]any{key: "unified-12"}})
	require.NoError(t, err)
	require.Equal(t, []string{"unified-12"}, OpenAICodexTicketGatewayBlacklist(created))
}

type codexBlacklistAccountRepo struct {
	upstreamBillingProbeAccountRepo
	added []string
}

func (r *codexBlacklistAccountRepo) AddCodexTicketGatewayToBlacklist(ctx context.Context, id int64, gateway string) error {
	r.added = append(r.added, gateway)
	gateways := OpenAICodexTicketGatewayBlacklist(r.accounts[id])
	extra := map[string]any{OpenAICodexTicketGatewayBlacklistExtraKey: append(gateways, gateway)}
	if err := NormalizeOpenAICodexTicketGatewayBlacklistExtra(extra); err != nil {
		return err
	}
	return r.UpdateExtra(ctx, id, extra)
}

func TestCodexGatewayBlacklistAddValidation(t *testing.T) {
	account := ticketTestAccount(41)
	account.GroupIDs = []int64{10}
	account.Extra = map[string]any{OpenAICodexTicketGatewayBlacklistExtraKey: []string{"unified-12"}, "other": true}
	repo := &codexBlacklistAccountRepo{upstreamBillingProbeAccountRepo: upstreamBillingProbeAccountRepo{accounts: map[int64]*Account{41: account}}}
	svc := &adminServiceImpl{accountRepo: repo}
	ctx := WithObserverScope(context.Background(), []int64{10})
	updated, err := svc.AddCodexTicketGatewayToBlacklist(ctx, 41, "35")
	require.NoError(t, err)
	require.Equal(t, []string{"unified-12", "unified-35"}, OpenAICodexTicketGatewayBlacklist(updated))
	require.Equal(t, true, updated.Extra["other"])
	_, err = svc.AddCodexTicketGatewayToBlacklist(ctx, 41, "unified-35")
	require.NoError(t, err)
	require.Len(t, OpenAICodexTicketGatewayBlacklist(account), 2)
	_, err = svc.AddCodexTicketGatewayToBlacklist(ctx, 41, "any")
	require.Error(t, err)
	_, err = svc.AddCodexTicketGatewayToBlacklist(WithObserverScope(context.Background(), []int64{20}), 41, "unified-88")
	require.ErrorIs(t, err, ErrObserverScope)
	account.Type = AccountTypeAPIKey
	_, err = svc.AddCodexTicketGatewayToBlacklist(ctx, 41, "unified-88")
	require.Error(t, err)
	require.Equal(t, []string{"unified-35", "unified-35"}, repo.added)
}

func TestCodexGatewayBlacklistRouteCacheKey(t *testing.T) {
	account := ticketTestAccount(41)
	base := codex780RouteKey(account, "token", "header", "any", "sse")
	account.Extra = map[string]any{OpenAICodexTicketGatewayBlacklistExtraKey: []string{"unified-35", "unified-12"}}
	blocked := codex780RouteKey(account, "token", "header", "any", "sse")
	require.NotEqual(t, base, blocked)
	account.Extra[OpenAICodexTicketGatewayBlacklistExtraKey] = []string{"unified-12", "unified-35"}
	require.Equal(t, blocked, codex780RouteKey(account, "token", "header", "any", "sse"))
}
