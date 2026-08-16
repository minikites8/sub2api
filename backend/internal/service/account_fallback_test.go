package service

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

func fallbackTestAccount(id int64, fallback bool) *Account {
	return &Account{
		ID:          id,
		Name:        "account",
		Platform:    PlatformOpenAI,
		Type:        AccountTypeAPIKey,
		Status:      StatusActive,
		Schedulable: true,
		IsFallback:  fallback,
	}
}

func TestPreferNonFallbackAccountPointers(t *testing.T) {
	regular := fallbackTestAccount(1, false)
	fallbackA := fallbackTestAccount(2, true)
	fallbackB := fallbackTestAccount(3, true)

	preferred := preferNonFallbackAccountPointers([]*Account{fallbackA, regular, fallbackB})
	require.Equal(t, []*Account{regular}, preferred)

	fallbackOnly := preferNonFallbackAccountPointers([]*Account{fallbackA, fallbackB})
	require.Equal(t, []*Account{fallbackA, fallbackB}, fallbackOnly)
}

func TestPreferNonFallbackAccountsAfterRequestExclusion(t *testing.T) {
	regular := *fallbackTestAccount(1, false)
	fallback := *fallbackTestAccount(2, true)

	candidates := []Account{regular, fallback}
	excluded := map[int64]struct{}{regular.ID: {}}
	eligible := make([]*Account, 0, len(candidates))
	for i := range candidates {
		if _, skip := excluded[candidates[i].ID]; skip {
			continue
		}
		eligible = append(eligible, &candidates[i])
	}

	require.Equal(t, []*Account{&candidates[1]}, preferNonFallbackAccountPointers(eligible))
}

func TestFallbackAccountClearsStickySession(t *testing.T) {
	account := fallbackTestAccount(1, true)
	require.True(t, shouldClearStickySession(account, ""))
}

func TestOpenAIAccountPreferenceUsesRegularAccount(t *testing.T) {
	service := &OpenAIGatewayService{}
	regular := fallbackTestAccount(1, false)
	regular.Priority = 100
	fallback := fallbackTestAccount(2, true)
	fallback.Priority = 1

	require.True(t, service.isBetterAccount(regular, fallback))
	require.False(t, service.isBetterAccount(fallback, regular))
}

func TestGeminiSelectionUsesRegularAccount(t *testing.T) {
	service := &GeminiMessagesCompatService{}
	regular := *fallbackTestAccount(1, false)
	regular.Platform = PlatformGemini
	regular.Priority = 100
	fallback := *fallbackTestAccount(2, true)
	fallback.Platform = PlatformGemini
	fallback.Priority = 1

	selected := service.selectBestGeminiAccount(
		context.Background(),
		[]Account{fallback, regular},
		"",
		nil,
		PlatformGemini,
		false,
	)
	require.NotNil(t, selected)
	require.Equal(t, regular.ID, selected.ID)
}

func TestBuildAccountForCreatePersistsFallbackSetting(t *testing.T) {
	account, err := buildAccountForCreate(&CreateAccountInput{
		Name:       "fallback",
		Platform:   PlatformOpenAI,
		Type:       AccountTypeOAuth,
		IsFallback: true,
	}, map[string]any{})
	require.NoError(t, err)
	require.True(t, account.IsFallback)
}
