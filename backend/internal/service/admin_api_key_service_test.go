package service

import (
	"context"
	"encoding/json"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
)

type adminAPIKeyTestRepo struct {
	mu   sync.Mutex
	data map[string]string
}

func newAdminAPIKeyTestRepo() *adminAPIKeyTestRepo {
	return &adminAPIKeyTestRepo{data: make(map[string]string)}
}

func (r *adminAPIKeyTestRepo) Get(_ context.Context, key string) (*Setting, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	value, ok := r.data[key]
	if !ok {
		return nil, ErrSettingNotFound
	}
	return &Setting{Key: key, Value: value}, nil
}

func (r *adminAPIKeyTestRepo) GetValue(_ context.Context, key string) (string, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	value, ok := r.data[key]
	if !ok {
		return "", ErrSettingNotFound
	}
	return value, nil
}

func (r *adminAPIKeyTestRepo) Set(_ context.Context, key, value string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.data[key] = value
	return nil
}

func (r *adminAPIKeyTestRepo) GetMultiple(_ context.Context, keys []string) (map[string]string, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	result := make(map[string]string, len(keys))
	for _, key := range keys {
		if value, ok := r.data[key]; ok {
			result[key] = value
		}
	}
	return result, nil
}

func (r *adminAPIKeyTestRepo) SetMultiple(_ context.Context, settings map[string]string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	for key, value := range settings {
		r.data[key] = value
	}
	return nil
}

func (r *adminAPIKeyTestRepo) GetAll(_ context.Context) (map[string]string, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	result := make(map[string]string, len(r.data))
	for key, value := range r.data {
		result[key] = value
	}
	return result, nil
}

func (r *adminAPIKeyTestRepo) Delete(_ context.Context, key string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.data, key)
	return nil
}

func TestAdminAPIKeyServiceCreateListValidateAndDelete(t *testing.T) {
	repo := newAdminAPIKeyTestRepo()
	svc := NewSettingService(repo, nil)

	created, err := svc.CreateAdminAPIKey(context.Background(), "pool worker", AdminAPIKeyPermissionAutoPool, AdminAPIKeyAccountDefaults{})
	require.NoError(t, err)
	require.True(t, strings.HasPrefix(created.Key, AdminAPIKeyPrefix))
	require.Equal(t, AdminAPIKeyPermissionAutoPool, created.APIKey.Permission)
	require.NotContains(t, repo.data[SettingKeyAdminAPIKeys], created.Key)

	var stored []storedAdminAPIKey
	require.NoError(t, json.Unmarshal([]byte(repo.data[SettingKeyAdminAPIKeys]), &stored))
	require.Len(t, stored, 1)
	require.NotEqual(t, created.Key, stored[0].Hash)
	require.Equal(t, hashAdminAPIKey(created.Key), stored[0].Hash)

	permission, keyID, err := svc.GetAdminAPIKeyPermission(context.Background(), created.Key)
	require.NoError(t, err)
	require.Equal(t, AdminAPIKeyPermissionAutoPool, permission)
	require.Equal(t, created.APIKey.ID, keyID)

	keys, err := svc.ListAdminAPIKeys(context.Background())
	require.NoError(t, err)
	require.Len(t, keys, 1)
	require.Equal(t, created.APIKey.ID, keys[0].ID)
	require.Equal(t, created.APIKey.MaskedKey, keys[0].MaskedKey)

	require.NoError(t, svc.DeleteAdminAPIKeyByID(context.Background(), created.APIKey.ID))
	permission, keyID, err = svc.GetAdminAPIKeyPermission(context.Background(), created.Key)
	require.NoError(t, err)
	require.Empty(t, permission)
	require.Empty(t, keyID)
	require.ErrorIs(t, svc.DeleteAdminAPIKeyByID(context.Background(), created.APIKey.ID), ErrAdminAPIKeyNotFound)
}

func TestAdminAPIKeyServicePersistsAndUpdatesAccountDefaults(t *testing.T) {
	repo := newAdminAPIKeyTestRepo()
	svc := NewSettingService(repo, nil)
	proxyID := int64(9)
	defaults := AdminAPIKeyAccountDefaults{
		ProxyMode:                   AdminAPIKeyProxyModeFixed,
		ProxyID:                     &proxyID,
		CodexFingerprintMode:        AdminAPIKeyCodexFingerprintSession,
		EnableAccountGuard:          true,
		AccountGuardIntervalMinutes: 15,
	}

	created, err := svc.CreateAdminAPIKey(context.Background(), "import defaults", AdminAPIKeyPermissionAutoPool, defaults)
	require.NoError(t, err)
	require.Equal(t, defaults, created.APIKey.AccountDefaults)

	auth, err := svc.ResolveAdminAPIKey(context.Background(), created.Key)
	require.NoError(t, err)
	require.Equal(t, defaults, auth.AccountDefaults)

	updated, err := svc.UpdateAdminAPIKey(context.Background(), created.APIKey.ID, "updated", AdminAPIKeyPermissionAutoPool, AdminAPIKeyAccountDefaults{
		ProxyMode:            AdminAPIKeyProxyModeRandom,
		CodexFingerprintMode: AdminAPIKeyCodexFingerprintFull,
	})
	require.NoError(t, err)
	require.Equal(t, AdminAPIKeyProxyModeRandom, updated.AccountDefaults.ProxyMode)
	require.Equal(t, AdminAPIKeyCodexFingerprintFull, updated.AccountDefaults.CodexFingerprintMode)
	require.False(t, updated.AccountDefaults.EnableAccountGuard)
	require.Equal(t, OpenAIAccountGuardDefaultIntervalMinutes, updated.AccountDefaults.AccountGuardIntervalMinutes)

	keys, err := svc.ListAdminAPIKeys(context.Background())
	require.NoError(t, err)
	require.Equal(t, updated.AccountDefaults, keys[0].AccountDefaults)
}

func TestAdminAPIKeyServiceMigratesLegacySessionCleanupDefault(t *testing.T) {
	repo := newAdminAPIKeyTestRepo()
	repo.data[SettingKeyAdminAPIKeys] = `[{"id":"key_legacy","name":"legacy","permission":"auto_pool","hash":"0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef","account_defaults":{"proxy_mode":"none","codex_fingerprint_mode":"off","revoke_other_sessions":true},"created_at":"2026-08-17T00:00:00Z"}]`
	svc := NewSettingService(repo, nil)

	keys, err := svc.ListAdminAPIKeys(context.Background())
	require.NoError(t, err)
	require.Len(t, keys, 1)
	require.True(t, keys[0].AccountDefaults.EnableAccountGuard)
	require.Equal(t, OpenAIAccountGuardDefaultIntervalMinutes, keys[0].AccountDefaults.AccountGuardIntervalMinutes)
	require.False(t, keys[0].AccountDefaults.LegacyRevokeOtherSessions)
}

func TestAdminAPIKeyServiceFullPermissionAndLegacyCompatibility(t *testing.T) {
	repo := newAdminAPIKeyTestRepo()
	repo.data[SettingKeyAdminAPIKey] = "legacy-admin-secret"
	svc := NewSettingService(repo, nil)

	permission, keyID, err := svc.GetAdminAPIKeyPermission(context.Background(), "legacy-admin-secret")
	require.NoError(t, err)
	require.Equal(t, AdminAPIKeyPermissionFull, permission)
	require.Equal(t, adminAPIKeyLegacyID, keyID)

	keys, err := svc.ListAdminAPIKeys(context.Background())
	require.NoError(t, err)
	require.Len(t, keys, 1)
	require.Equal(t, adminAPIKeyLegacyID, keys[0].ID)
	require.Equal(t, AdminAPIKeyPermissionFull, keys[0].Permission)
	require.Equal(t, "legacy-adm...cret", keys[0].MaskedKey)

	require.NoError(t, svc.DeleteAdminAPIKeyByID(context.Background(), adminAPIKeyLegacyID))
	_, err = svc.GetAdminAPIKey(context.Background())
	require.NoError(t, err)
	require.Empty(t, repo.data[SettingKeyAdminAPIKey])
}

func TestAdminAPIKeyServiceRejectsInvalidInput(t *testing.T) {
	svc := NewSettingService(newAdminAPIKeyTestRepo(), nil)

	_, err := svc.CreateAdminAPIKey(context.Background(), "", AdminAPIKeyPermissionFull, AdminAPIKeyAccountDefaults{})
	require.Error(t, err)
	_, err = svc.CreateAdminAPIKey(context.Background(), "valid", "read_only", AdminAPIKeyAccountDefaults{})
	require.Error(t, err)
	_, err = svc.CreateAdminAPIKey(context.Background(), strings.Repeat("x", adminAPIKeyMaxNameLength+1), AdminAPIKeyPermissionFull, AdminAPIKeyAccountDefaults{})
	require.Error(t, err)
}
