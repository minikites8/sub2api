package service

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/url"
	"strconv"
	"strings"
	"time"
)

const (
	quotaLeaseGeneratedMediaConfigVersion   = 1
	quotaLeaseGeneratedMediaConfigCacheTTL  = 15 * time.Second
	quotaLeaseGeneratedMediaConfigLifetime  = 5 * time.Minute
	quotaLeaseGeneratedMediaConfigClockSkew = time.Minute
	quotaLeaseGeneratedMediaConfigKeyLabel  = "sub2api/quota-lease/generated-media-storage/v1"
)

type QuotaLeaseGeneratedMediaStorageEnvelope struct {
	Version    int    `json:"version"`
	Nonce      string `json:"nonce"`
	Ciphertext string `json:"ciphertext"`
}

type quotaLeaseGeneratedMediaStoragePayload struct {
	Version   int                         `json:"version"`
	NodeID    string                      `json:"node_id"`
	IssuedAt  time.Time                   `json:"issued_at"`
	ExpiresAt time.Time                   `json:"expires_at"`
	Config    GeneratedMediaStorageConfig `json:"config"`
}

type quotaLeaseRemoteGeneratedMediaStorageResponse struct {
	Data       *QuotaLeaseGeneratedMediaStorageEnvelope `json:"data"`
	Version    int                                      `json:"version"`
	Nonce      string                                   `json:"nonce"`
	Ciphertext string                                   `json:"ciphertext"`
}

func (r quotaLeaseRemoteGeneratedMediaStorageResponse) envelope() (*QuotaLeaseGeneratedMediaStorageEnvelope, error) {
	if r.Data != nil {
		return r.Data, nil
	}
	if r.Version > 0 || strings.TrimSpace(r.Nonce) != "" || strings.TrimSpace(r.Ciphertext) != "" {
		return &QuotaLeaseGeneratedMediaStorageEnvelope{
			Version: r.Version, Nonce: r.Nonce, Ciphertext: r.Ciphertext,
		}, nil
	}
	return nil, errors.New("generated media storage sync response is missing data")
}

func (s *QuotaLeaseDemoService) SetGeneratedMediaStorageService(storage *GeneratedMediaStorageService) {
	if s == nil {
		return
	}
	s.generatedMediaMu.Lock()
	s.generatedMediaStorage = storage
	s.remoteGeneratedMedia = nil
	s.remoteGeneratedMediaTTL = time.Time{}
	s.remoteGeneratedMediaMax = time.Time{}
	s.generatedMediaMu.Unlock()
}

// ResolveGeneratedMediaStorageConfig implements GeneratedMediaStorageConfigSource.
// Remote nodes resolve the control-plane config; standalone and control-plane
// processes continue to use their local encrypted setting.
func (s *QuotaLeaseDemoService) ResolveGeneratedMediaStorageConfig(ctx context.Context) (*GeneratedMediaStorageConfig, bool, error) {
	if s == nil || !s.remoteMode() {
		return nil, false, nil
	}
	now := time.Now()
	if cfg := s.cachedRemoteGeneratedMediaStorage(now, false); cfg != nil {
		return cfg, true, nil
	}

	value, err, _ := s.generatedMediaGroup.Do("generated-media-storage-config", func() (any, error) {
		fetchNow := time.Now()
		if cfg := s.cachedRemoteGeneratedMediaStorage(fetchNow, false); cfg != nil {
			return cfg, nil
		}
		cfg, validUntil, fetchErr := s.fetchRemoteGeneratedMediaStorageConfig(ctx)
		if fetchErr != nil {
			return nil, fetchErr
		}
		ttl := fetchNow.Add(quotaLeaseGeneratedMediaConfigCacheTTL)
		if validUntil.Before(ttl) {
			ttl = validUntil
		}
		s.generatedMediaMu.Lock()
		s.remoteGeneratedMedia = cloneGeneratedMediaStorageConfig(cfg)
		s.remoteGeneratedMediaTTL = ttl
		s.remoteGeneratedMediaMax = validUntil
		s.generatedMediaMu.Unlock()
		return cloneGeneratedMediaStorageConfig(cfg), nil
	})
	if err != nil {
		if cached := s.cachedRemoteGeneratedMediaStorage(time.Now(), true); cached != nil {
			slog.Warn("quota_lease.generated_media_storage_sync_stale", "error", err)
			return cached, true, nil
		}
		return nil, true, fmt.Errorf("sync generated media storage config: %w", err)
	}
	cfg, _ := value.(*GeneratedMediaStorageConfig)
	return cloneGeneratedMediaStorageConfig(cfg), true, nil
}

func (s *QuotaLeaseDemoService) cachedRemoteGeneratedMediaStorage(now time.Time, allowStale bool) *GeneratedMediaStorageConfig {
	if s == nil {
		return nil
	}
	s.generatedMediaMu.RLock()
	defer s.generatedMediaMu.RUnlock()
	if s.remoteGeneratedMedia == nil || (!allowStale && !now.Before(s.remoteGeneratedMediaTTL)) || (allowStale && !now.Before(s.remoteGeneratedMediaMax)) {
		return nil
	}
	return cloneGeneratedMediaStorageConfig(s.remoteGeneratedMedia)
}

func (s *QuotaLeaseDemoService) fetchRemoteGeneratedMediaStorageConfig(ctx context.Context) (*GeneratedMediaStorageConfig, time.Time, error) {
	if err := validateQuotaLeaseGeneratedMediaStorageControlPlaneURL(s.ControlPlaneBaseURL()); err != nil {
		return nil, time.Time{}, err
	}
	nodeID, secret, err := s.remoteNodeAuth(ctx)
	if err != nil {
		return nil, time.Time{}, err
	}
	var response quotaLeaseRemoteGeneratedMediaStorageResponse
	if err := s.doRemoteJSON(ctx, "GET", "/generated-media-storage/config", nodeID, secret, nil, &response); err != nil {
		return nil, time.Time{}, err
	}
	envelope, err := response.envelope()
	if err != nil {
		return nil, time.Time{}, err
	}
	payload, err := decryptQuotaLeaseGeneratedMediaStorageConfig(envelope, nodeID, secret, time.Now())
	if err != nil {
		return nil, time.Time{}, err
	}
	return cloneGeneratedMediaStorageConfig(&payload.Config), payload.ExpiresAt, nil
}

func validateQuotaLeaseGeneratedMediaStorageControlPlaneURL(baseURL string) error {
	parsed, err := url.Parse(strings.TrimSpace(baseURL))
	if err != nil || parsed.Host == "" {
		return errors.New("generated media storage sync control plane URL is invalid")
	}
	if strings.EqualFold(parsed.Scheme, "https") {
		return nil
	}
	hostname := strings.TrimSpace(parsed.Hostname())
	if strings.EqualFold(hostname, "localhost") {
		return nil
	}
	if ip := net.ParseIP(hostname); ip != nil && ip.IsLoopback() {
		return nil
	}
	return errors.New("generated media storage sync control plane URL requires HTTPS")
}

func (s *QuotaLeaseDemoService) EncryptGeneratedMediaStorageConfigForNode(ctx context.Context, nodeID, nodeSecret string) (*QuotaLeaseGeneratedMediaStorageEnvelope, error) {
	if s == nil || !s.Enabled() {
		return nil, ErrQuotaLeaseDemoDisabled
	}
	nodeID = strings.TrimSpace(nodeID)
	nodeSecret = strings.TrimSpace(nodeSecret)
	if !s.AuthenticateNode(nodeID, nodeSecret) {
		return nil, ErrQuotaLeaseDemoNodeNotFound
	}

	s.generatedMediaMu.RLock()
	storage := s.generatedMediaStorage
	s.generatedMediaMu.RUnlock()
	if storage == nil {
		return nil, errors.New("generated media storage service is unavailable")
	}
	cfg, err := storage.loadLocalConfig(ctx)
	if err != nil {
		return nil, err
	}
	if cfg == nil {
		cfg = &GeneratedMediaStorageConfig{Prefix: generatedMediaDefaultPrefix}
	}
	cfg = cloneGeneratedMediaStorageConfig(cfg)
	cfg.normalize()
	if err := cfg.validateEnabled(); err != nil {
		return nil, err
	}
	if !cfg.Enabled {
		cfg.SecretAccessKey = ""
	}
	cfg.SecretConfigured = cfg.SecretAccessKey != ""

	now := time.Now().UTC()
	payload := quotaLeaseGeneratedMediaStoragePayload{
		Version:   quotaLeaseGeneratedMediaConfigVersion,
		NodeID:    nodeID,
		IssuedAt:  now,
		ExpiresAt: now.Add(quotaLeaseGeneratedMediaConfigLifetime),
		Config:    *cfg,
	}
	return encryptQuotaLeaseGeneratedMediaStorageConfig(payload, nodeSecret)
}

func encryptQuotaLeaseGeneratedMediaStorageConfig(payload quotaLeaseGeneratedMediaStoragePayload, nodeSecret string) (*QuotaLeaseGeneratedMediaStorageEnvelope, error) {
	plaintext, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("marshal generated media storage sync payload: %w", err)
	}
	gcm, err := quotaLeaseGeneratedMediaStorageGCM(nodeSecret)
	if err != nil {
		return nil, err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, fmt.Errorf("generate generated media storage sync nonce: %w", err)
	}
	aad := quotaLeaseGeneratedMediaStorageAAD(payload.Version, payload.NodeID)
	ciphertext := gcm.Seal(nil, nonce, plaintext, aad)
	return &QuotaLeaseGeneratedMediaStorageEnvelope{
		Version:    payload.Version,
		Nonce:      base64.RawURLEncoding.EncodeToString(nonce),
		Ciphertext: base64.RawURLEncoding.EncodeToString(ciphertext),
	}, nil
}

func decryptQuotaLeaseGeneratedMediaStorageConfig(envelope *QuotaLeaseGeneratedMediaStorageEnvelope, nodeID, nodeSecret string, now time.Time) (*quotaLeaseGeneratedMediaStoragePayload, error) {
	if envelope == nil || envelope.Version != quotaLeaseGeneratedMediaConfigVersion {
		return nil, errors.New("unsupported generated media storage sync version")
	}
	nonce, err := base64.RawURLEncoding.DecodeString(strings.TrimSpace(envelope.Nonce))
	if err != nil {
		return nil, errors.New("invalid generated media storage sync nonce")
	}
	ciphertext, err := base64.RawURLEncoding.DecodeString(strings.TrimSpace(envelope.Ciphertext))
	if err != nil {
		return nil, errors.New("invalid generated media storage sync ciphertext")
	}
	gcm, err := quotaLeaseGeneratedMediaStorageGCM(nodeSecret)
	if err != nil {
		return nil, err
	}
	if len(nonce) != gcm.NonceSize() {
		return nil, errors.New("invalid generated media storage sync nonce size")
	}
	plaintext, err := gcm.Open(nil, nonce, ciphertext, quotaLeaseGeneratedMediaStorageAAD(envelope.Version, nodeID))
	if err != nil {
		return nil, errors.New("decrypt generated media storage sync payload")
	}
	var payload quotaLeaseGeneratedMediaStoragePayload
	if err := json.Unmarshal(plaintext, &payload); err != nil {
		return nil, errors.New("decode generated media storage sync payload")
	}
	if payload.Version != envelope.Version || strings.TrimSpace(payload.NodeID) != strings.TrimSpace(nodeID) {
		return nil, errors.New("generated media storage sync payload identity mismatch")
	}
	if now.IsZero() {
		now = time.Now()
	}
	if payload.IssuedAt.After(now.Add(quotaLeaseGeneratedMediaConfigClockSkew)) || !now.Before(payload.ExpiresAt) {
		return nil, errors.New("generated media storage sync payload expired")
	}
	payload.Config.normalize()
	if err := payload.Config.validateEnabled(); err != nil {
		return nil, err
	}
	return &payload, nil
}

func quotaLeaseGeneratedMediaStorageGCM(nodeSecret string) (cipher.AEAD, error) {
	nodeSecret = strings.TrimSpace(nodeSecret)
	if nodeSecret == "" {
		return nil, errors.New("generated media storage sync node secret is required")
	}
	mac := hmac.New(sha256.New, []byte(nodeSecret))
	_, _ = mac.Write([]byte(quotaLeaseGeneratedMediaConfigKeyLabel))
	block, err := aes.NewCipher(mac.Sum(nil))
	if err != nil {
		return nil, err
	}
	return cipher.NewGCM(block)
}

func quotaLeaseGeneratedMediaStorageAAD(version int, nodeID string) []byte {
	return []byte(quotaLeaseGeneratedMediaConfigKeyLabel + ":" + strconv.Itoa(version) + ":" + strings.TrimSpace(nodeID))
}

func cloneGeneratedMediaStorageConfig(cfg *GeneratedMediaStorageConfig) *GeneratedMediaStorageConfig {
	if cfg == nil {
		return nil
	}
	cloned := *cfg
	return &cloned
}

var _ GeneratedMediaStorageConfigSource = (*QuotaLeaseDemoService)(nil)
