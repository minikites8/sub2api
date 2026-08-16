package service

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
)

const (
	AdminAPIKeyPermissionFull          = "full"
	AdminAPIKeyPermissionAutoPool      = "auto_pool"
	AdminAPIKeyProxyModeNone           = "none"
	AdminAPIKeyProxyModeFixed          = "fixed"
	AdminAPIKeyProxyModeRandom         = "random"
	AdminAPIKeyCodexFingerprintOff     = "off"
	AdminAPIKeyCodexFingerprintDevice  = "device"
	AdminAPIKeyCodexFingerprintSession = "session"
	AdminAPIKeyCodexFingerprintFull    = "full"
	adminAPIKeyMaxNameLength           = 100
	adminAPIKeyMaxCount                = 100
	adminAPIKeyLegacyID                = "legacy"
)

// AdminAPIKey is the safe administrator-facing projection. The secret value
// is returned only by CreateAdminAPIKey and is never persisted in this form.
type AdminAPIKey struct {
	ID              string                     `json:"id"`
	Name            string                     `json:"name"`
	Permission      string                     `json:"permission"`
	MaskedKey       string                     `json:"masked_key"`
	AccountDefaults AdminAPIKeyAccountDefaults `json:"account_defaults"`
	CreatedAt       *time.Time                 `json:"created_at"`
}

// AdminAPIKeyAccountDefaults are applied when an auto-pool key creates an
// account and the incoming request leaves the corresponding field unset.
type AdminAPIKeyAccountDefaults struct {
	ProxyMode            string `json:"proxy_mode"`
	ProxyID              *int64 `json:"proxy_id,omitempty"`
	CodexFingerprintMode string `json:"codex_fingerprint_mode"`
	RevokeOtherSessions  bool   `json:"revoke_other_sessions"`
}

type AdminAPIKeyAuth struct {
	ID              string
	Permission      string
	AccountDefaults AdminAPIKeyAccountDefaults
}

type adminAPIKeyAccountDefaultsContextKey struct{}

func ContextWithAdminAPIKeyAccountDefaults(ctx context.Context, defaults AdminAPIKeyAccountDefaults) context.Context {
	return context.WithValue(ctx, adminAPIKeyAccountDefaultsContextKey{}, defaults)
}

func AdminAPIKeyAccountDefaultsFromContext(ctx context.Context) (AdminAPIKeyAccountDefaults, bool) {
	if ctx == nil {
		return AdminAPIKeyAccountDefaults{}, false
	}
	defaults, ok := ctx.Value(adminAPIKeyAccountDefaultsContextKey{}).(AdminAPIKeyAccountDefaults)
	return defaults, ok
}

type AdminAPIKeyCreation struct {
	Key    string      `json:"key"`
	APIKey AdminAPIKey `json:"api_key"`
}

type storedAdminAPIKey struct {
	ID              string                     `json:"id"`
	Name            string                     `json:"name"`
	Permission      string                     `json:"permission"`
	Hash            string                     `json:"hash"`
	MaskedKey       string                     `json:"masked_key"`
	AccountDefaults AdminAPIKeyAccountDefaults `json:"account_defaults"`
	CreatedAt       time.Time                  `json:"created_at"`
}

var ErrAdminAPIKeyNotFound = infraerrors.NotFound("ADMIN_API_KEY_NOT_FOUND", "admin API key not found")

func normalizeAdminAPIKeyPermission(permission string) (string, error) {
	switch strings.ToLower(strings.TrimSpace(permission)) {
	case AdminAPIKeyPermissionFull:
		return AdminAPIKeyPermissionFull, nil
	case AdminAPIKeyPermissionAutoPool:
		return AdminAPIKeyPermissionAutoPool, nil
	default:
		return "", infraerrors.BadRequest("ADMIN_API_KEY_PERMISSION_INVALID", "permission must be full or auto_pool")
	}
}

func normalizeAdminAPIKeyName(name string) (string, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return "", infraerrors.BadRequest("ADMIN_API_KEY_NAME_REQUIRED", "name is required")
	}
	if len([]rune(name)) > adminAPIKeyMaxNameLength {
		return "", infraerrors.BadRequest("ADMIN_API_KEY_NAME_TOO_LONG", "name is too long")
	}
	return name, nil
}

func normalizeAdminAPIKeyAccountDefaults(defaults AdminAPIKeyAccountDefaults) (AdminAPIKeyAccountDefaults, error) {
	defaults.ProxyMode = strings.ToLower(strings.TrimSpace(defaults.ProxyMode))
	if defaults.ProxyMode == "" {
		defaults.ProxyMode = AdminAPIKeyProxyModeNone
	}
	switch defaults.ProxyMode {
	case AdminAPIKeyProxyModeNone, AdminAPIKeyProxyModeRandom:
		defaults.ProxyID = nil
	case AdminAPIKeyProxyModeFixed:
		if defaults.ProxyID == nil || *defaults.ProxyID <= 0 {
			return AdminAPIKeyAccountDefaults{}, infraerrors.BadRequest("ADMIN_API_KEY_PROXY_REQUIRED", "proxy_id is required for fixed proxy mode")
		}
	default:
		return AdminAPIKeyAccountDefaults{}, infraerrors.BadRequest("ADMIN_API_KEY_PROXY_MODE_INVALID", "proxy_mode must be none, fixed, or random")
	}

	defaults.CodexFingerprintMode = strings.ToLower(strings.TrimSpace(defaults.CodexFingerprintMode))
	if defaults.CodexFingerprintMode == "" {
		defaults.CodexFingerprintMode = AdminAPIKeyCodexFingerprintOff
	}
	switch defaults.CodexFingerprintMode {
	case AdminAPIKeyCodexFingerprintOff,
		AdminAPIKeyCodexFingerprintDevice,
		AdminAPIKeyCodexFingerprintSession,
		AdminAPIKeyCodexFingerprintFull:
		return defaults, nil
	default:
		return AdminAPIKeyAccountDefaults{}, infraerrors.BadRequest("ADMIN_API_KEY_CODEX_FINGERPRINT_INVALID", "codex_fingerprint_mode must be off, device, session, or full")
	}
}

func normalizedAdminAPIKeyDefaultsForPermission(permission string, defaults AdminAPIKeyAccountDefaults) (AdminAPIKeyAccountDefaults, error) {
	if permission == AdminAPIKeyPermissionFull {
		return normalizeAdminAPIKeyAccountDefaults(AdminAPIKeyAccountDefaults{})
	}
	return normalizeAdminAPIKeyAccountDefaults(defaults)
}

func hashAdminAPIKey(key string) string {
	hash := sha256.Sum256([]byte(key))
	return hex.EncodeToString(hash[:])
}

func maskAdminAPIKey(key string) string {
	if len(key) <= 14 {
		return key
	}
	return key[:10] + "..." + key[len(key)-4:]
}

func generateAdminAPIKeyValue() (string, error) {
	bytes := make([]byte, 32)
	if _, err := rand.Read(bytes); err != nil {
		return "", fmt.Errorf("generate random bytes: %w", err)
	}
	return AdminAPIKeyPrefix + hex.EncodeToString(bytes), nil
}

func generateAdminAPIKeyID() (string, error) {
	bytes := make([]byte, 12)
	if _, err := rand.Read(bytes); err != nil {
		return "", fmt.Errorf("generate admin API key id: %w", err)
	}
	return "key_" + hex.EncodeToString(bytes), nil
}

func (s *SettingService) loadStoredAdminAPIKeys(ctx context.Context) ([]storedAdminAPIKey, error) {
	raw, err := s.settingRepo.GetValue(ctx, SettingKeyAdminAPIKeys)
	if err != nil {
		if errors.Is(err, ErrSettingNotFound) {
			return []storedAdminAPIKey{}, nil
		}
		return nil, err
	}
	if strings.TrimSpace(raw) == "" {
		return []storedAdminAPIKey{}, nil
	}
	var keys []storedAdminAPIKey
	if err := json.Unmarshal([]byte(raw), &keys); err != nil {
		return nil, fmt.Errorf("decode admin api keys: %w", err)
	}
	if len(keys) > adminAPIKeyMaxCount {
		return nil, fmt.Errorf("admin API key count exceeds limit")
	}
	for i := range keys {
		if keys[i].ID == "" || keys[i].Hash == "" || len(keys[i].Hash) != sha256.Size*2 {
			return nil, fmt.Errorf("admin API key record %d is invalid", i)
		}
		permission, err := normalizeAdminAPIKeyPermission(keys[i].Permission)
		if err != nil {
			return nil, fmt.Errorf("admin API key record %s has invalid permission", keys[i].ID)
		}
		keys[i].Permission = permission
		defaults, err := normalizedAdminAPIKeyDefaultsForPermission(permission, keys[i].AccountDefaults)
		if err != nil {
			return nil, fmt.Errorf("admin API key record %s has invalid account defaults", keys[i].ID)
		}
		keys[i].AccountDefaults = defaults
	}
	return keys, nil
}

func (s *SettingService) saveStoredAdminAPIKeys(ctx context.Context, keys []storedAdminAPIKey) error {
	data, err := json.Marshal(keys)
	if err != nil {
		return fmt.Errorf("encode admin api keys: %w", err)
	}
	return s.settingRepo.Set(ctx, SettingKeyAdminAPIKeys, string(data))
}

func storedAdminAPIKeyInfo(key storedAdminAPIKey) AdminAPIKey {
	maskedKey := key.MaskedKey
	if maskedKey == "" {
		maskedKey = "admin-" + key.Hash[:10] + "..." + key.Hash[len(key.Hash)-4:]
	}
	createdAt := key.CreatedAt
	return AdminAPIKey{
		ID:              key.ID,
		Name:            key.Name,
		Permission:      key.Permission,
		MaskedKey:       maskedKey,
		AccountDefaults: key.AccountDefaults,
		CreatedAt:       &createdAt,
	}
}

// ListAdminAPIKeys lists all managed keys. The legacy single key appears as a
// full-permission record so administrators can identify and remove it.
func (s *SettingService) ListAdminAPIKeys(ctx context.Context) ([]AdminAPIKey, error) {
	if s == nil || s.settingRepo == nil {
		return nil, infraerrors.InternalServer("ADMIN_API_KEYS_NOT_CONFIGURED", "admin API key service is not configured")
	}
	keys, err := s.loadStoredAdminAPIKeys(ctx)
	if err != nil {
		return nil, err
	}
	result := make([]AdminAPIKey, 0, len(keys)+1)
	legacy, err := s.GetAdminAPIKey(ctx)
	if err != nil {
		return nil, err
	}
	if legacy != "" {
		result = append(result, AdminAPIKey{
			ID:              adminAPIKeyLegacyID,
			Name:            "Legacy admin API key",
			Permission:      AdminAPIKeyPermissionFull,
			MaskedKey:       maskAdminAPIKey(legacy),
			AccountDefaults: AdminAPIKeyAccountDefaults{ProxyMode: AdminAPIKeyProxyModeNone, CodexFingerprintMode: AdminAPIKeyCodexFingerprintOff},
			CreatedAt:       nil,
		})
	}
	for _, key := range keys {
		result = append(result, storedAdminAPIKeyInfo(key))
	}
	return result, nil
}

// CreateAdminAPIKey creates a named key and returns its secret once.
func (s *SettingService) CreateAdminAPIKey(ctx context.Context, name, permission string, defaults AdminAPIKeyAccountDefaults) (*AdminAPIKeyCreation, error) {
	if s == nil || s.settingRepo == nil {
		return nil, infraerrors.InternalServer("ADMIN_API_KEYS_NOT_CONFIGURED", "admin API key service is not configured")
	}
	name, err := normalizeAdminAPIKeyName(name)
	if err != nil {
		return nil, err
	}
	permission, err = normalizeAdminAPIKeyPermission(permission)
	if err != nil {
		return nil, err
	}
	defaults, err = normalizedAdminAPIKeyDefaultsForPermission(permission, defaults)
	if err != nil {
		return nil, err
	}
	key, err := generateAdminAPIKeyValue()
	if err != nil {
		return nil, err
	}
	id, err := generateAdminAPIKeyID()
	if err != nil {
		return nil, err
	}
	stored := storedAdminAPIKey{
		ID:              id,
		Name:            name,
		Permission:      permission,
		Hash:            hashAdminAPIKey(key),
		MaskedKey:       maskAdminAPIKey(key),
		AccountDefaults: defaults,
		CreatedAt:       time.Now().UTC(),
	}

	s.adminAPIKeysMu.Lock()
	defer s.adminAPIKeysMu.Unlock()
	keys, err := s.loadStoredAdminAPIKeys(ctx)
	if err != nil {
		return nil, err
	}
	if len(keys) >= adminAPIKeyMaxCount {
		return nil, infraerrors.BadRequest("ADMIN_API_KEY_LIMIT_REACHED", "admin API key limit reached")
	}
	keys = append(keys, stored)
	if err := s.saveStoredAdminAPIKeys(ctx, keys); err != nil {
		return nil, fmt.Errorf("save admin api keys: %w", err)
	}
	return &AdminAPIKeyCreation{Key: key, APIKey: storedAdminAPIKeyInfo(stored)}, nil
}

// UpdateAdminAPIKey updates a managed key's label, permission, and account defaults.
func (s *SettingService) UpdateAdminAPIKey(ctx context.Context, id, name, permission string, defaults AdminAPIKeyAccountDefaults) (*AdminAPIKey, error) {
	id = strings.TrimSpace(id)
	if id == "" || id == adminAPIKeyLegacyID {
		return nil, ErrAdminAPIKeyNotFound
	}
	name, err := normalizeAdminAPIKeyName(name)
	if err != nil {
		return nil, err
	}
	permission, err = normalizeAdminAPIKeyPermission(permission)
	if err != nil {
		return nil, err
	}
	defaults, err = normalizedAdminAPIKeyDefaultsForPermission(permission, defaults)
	if err != nil {
		return nil, err
	}

	s.adminAPIKeysMu.Lock()
	defer s.adminAPIKeysMu.Unlock()
	keys, err := s.loadStoredAdminAPIKeys(ctx)
	if err != nil {
		return nil, err
	}
	for i := range keys {
		if keys[i].ID != id {
			continue
		}
		keys[i].Name = name
		keys[i].Permission = permission
		keys[i].AccountDefaults = defaults
		if err := s.saveStoredAdminAPIKeys(ctx, keys); err != nil {
			return nil, fmt.Errorf("save admin api keys: %w", err)
		}
		updated := storedAdminAPIKeyInfo(keys[i])
		return &updated, nil
	}
	return nil, ErrAdminAPIKeyNotFound
}

// DeleteAdminAPIKeyByID removes a managed key or the legacy key record.
func (s *SettingService) DeleteAdminAPIKeyByID(ctx context.Context, id string) error {
	id = strings.TrimSpace(id)
	if id == adminAPIKeyLegacyID {
		return s.DeleteAdminAPIKey(ctx)
	}
	if id == "" {
		return ErrAdminAPIKeyNotFound
	}
	s.adminAPIKeysMu.Lock()
	defer s.adminAPIKeysMu.Unlock()
	keys, err := s.loadStoredAdminAPIKeys(ctx)
	if err != nil {
		return err
	}
	for i, key := range keys {
		if key.ID != id {
			continue
		}
		keys = append(keys[:i], keys[i+1:]...)
		if err := s.saveStoredAdminAPIKeys(ctx, keys); err != nil {
			return fmt.Errorf("save admin api keys: %w", err)
		}
		return nil
	}
	return ErrAdminAPIKeyNotFound
}

// GetAdminAPIKeyPermission validates a key and returns its permission scope.
// The legacy key retains full access for compatibility with existing clients.
func (s *SettingService) GetAdminAPIKeyPermission(ctx context.Context, presented string) (permission, keyID string, err error) {
	auth, err := s.ResolveAdminAPIKey(ctx, presented)
	if err != nil || auth == nil {
		return "", "", err
	}
	return auth.Permission, auth.ID, nil
}

// ResolveAdminAPIKey validates a key and returns its scope and import defaults.
func (s *SettingService) ResolveAdminAPIKey(ctx context.Context, presented string) (*AdminAPIKeyAuth, error) {
	presented = strings.TrimSpace(presented)
	if presented == "" {
		return nil, nil
	}
	keys, err := s.loadStoredAdminAPIKeys(ctx)
	if err != nil {
		return nil, err
	}
	presentedHash := hashAdminAPIKey(presented)
	for _, key := range keys {
		if subtle.ConstantTimeCompare([]byte(presentedHash), []byte(key.Hash)) == 1 {
			return &AdminAPIKeyAuth{ID: key.ID, Permission: key.Permission, AccountDefaults: key.AccountDefaults}, nil
		}
	}
	legacy, err := s.GetAdminAPIKey(ctx)
	if err != nil {
		return nil, err
	}
	if legacy != "" && subtle.ConstantTimeCompare([]byte(presented), []byte(legacy)) == 1 {
		return &AdminAPIKeyAuth{ID: adminAPIKeyLegacyID, Permission: AdminAPIKeyPermissionFull}, nil
	}
	return nil, nil
}
