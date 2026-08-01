package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"net/url"
	"path"
	"strings"
	"sync"
	"time"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/Wei-Shaw/sub2api/internal/pkg/logger"
)

const (
	settingKeyGeneratedMediaStorage = "generated_media_storage_config"
	generatedMediaDefaultPrefix     = "generated-videos"
	generatedMediaMaxBytes          = int64(2 * 1024 * 1024 * 1024)
)

var (
	ErrGeneratedMediaStorageConfigCorrupt = infraerrors.InternalServer("GENERATED_MEDIA_STORAGE_CONFIG_CORRUPT", "generated media storage config is corrupted")
	ErrGeneratedMediaStorageIncomplete    = infraerrors.BadRequest("GENERATED_MEDIA_STORAGE_INCOMPLETE", "generated media storage config is incomplete")
	errGeneratedMediaTooLarge             = errors.New("generated media exceeds the storage size limit")
)

// GeneratedMediaStorageConfig configures an S3-compatible destination for generated videos.
type GeneratedMediaStorageConfig struct {
	Enabled          bool   `json:"enabled"`
	Endpoint         string `json:"endpoint"`
	Region           string `json:"region"`
	Bucket           string `json:"bucket"`
	AccessKeyID      string `json:"access_key_id"`
	SecretAccessKey  string `json:"secret_access_key,omitempty"` //nolint:revive // field name follows S3 conventions
	SecretConfigured bool   `json:"secret_configured,omitempty"`
	Prefix           string `json:"prefix"`
	PublicBaseURL    string `json:"public_base_url"`
	ForcePathStyle   bool   `json:"force_path_style"`
}

func (c *GeneratedMediaStorageConfig) normalize() {
	c.Endpoint = strings.TrimRight(strings.TrimSpace(c.Endpoint), "/")
	c.Region = strings.TrimSpace(c.Region)
	c.Bucket = strings.TrimSpace(c.Bucket)
	c.AccessKeyID = strings.TrimSpace(c.AccessKeyID)
	c.Prefix = strings.Trim(strings.ReplaceAll(strings.TrimSpace(c.Prefix), "\\", "/"), "/")
	if c.Prefix == "" {
		c.Prefix = generatedMediaDefaultPrefix
	}
	c.PublicBaseURL = strings.TrimRight(strings.TrimSpace(c.PublicBaseURL), "/")
}

func (c *GeneratedMediaStorageConfig) validateEnabled() error {
	if c == nil || !c.Enabled {
		return nil
	}
	if c.Bucket == "" || c.AccessKeyID == "" || c.SecretAccessKey == "" || c.PublicBaseURL == "" {
		return ErrGeneratedMediaStorageIncomplete
	}
	parsed, err := url.Parse(c.PublicBaseURL)
	if err != nil || parsed.Host == "" || (parsed.Scheme != "http" && parsed.Scheme != "https") {
		return infraerrors.BadRequest("GENERATED_MEDIA_PUBLIC_URL_INVALID", "generated media public base URL is invalid")
	}
	return nil
}

type GeneratedMediaObjectStore interface {
	Upload(ctx context.Context, key string, body io.Reader, contentType string, contentLength int64) (int64, error)
	HeadBucket(ctx context.Context) error
}

type GeneratedMediaObjectStoreFactory func(ctx context.Context, cfg *GeneratedMediaStorageConfig) (GeneratedMediaObjectStore, error)

type GeneratedMediaArchiver interface {
	Archive(ctx context.Context, sourceURL, objectID string, createdAt time.Time) (string, error)
}

// GeneratedMediaStorageConfigSource supplies a cluster-wide runtime config.
// The handled result is false when the caller should use its local settings.
type GeneratedMediaStorageConfigSource interface {
	ResolveGeneratedMediaStorageConfig(ctx context.Context) (cfg *GeneratedMediaStorageConfig, handled bool, err error)
}

type GeneratedMediaStorageService struct {
	settingRepo  SettingRepository
	encryptor    SecretEncryptor
	storeFactory GeneratedMediaObjectStoreFactory
	http         HTTPUpstream
	configSource GeneratedMediaStorageConfigSource

	configSourceMu   sync.RWMutex
	storeMu          sync.Mutex
	store            GeneratedMediaObjectStore
	storeFingerprint string
}

func NewGeneratedMediaStorageService(
	settingRepo SettingRepository,
	encryptor SecretEncryptor,
	storeFactory GeneratedMediaObjectStoreFactory,
	httpUpstream HTTPUpstream,
) *GeneratedMediaStorageService {
	return &GeneratedMediaStorageService{
		settingRepo: settingRepo, encryptor: encryptor, storeFactory: storeFactory, http: httpUpstream,
	}
}

func (s *GeneratedMediaStorageService) SetConfigSource(source GeneratedMediaStorageConfigSource) {
	if s == nil {
		return
	}
	s.configSourceMu.Lock()
	s.configSource = source
	s.configSourceMu.Unlock()
	s.clearStore()
}

func (s *GeneratedMediaStorageService) GetConfig(ctx context.Context) (*GeneratedMediaStorageConfig, error) {
	cfg, err := s.loadLocalConfig(ctx)
	if err != nil {
		return nil, err
	}
	if cfg == nil {
		return &GeneratedMediaStorageConfig{Prefix: generatedMediaDefaultPrefix}, nil
	}
	cfg.SecretConfigured = cfg.SecretAccessKey != ""
	cfg.SecretAccessKey = ""
	return cfg, nil
}

func (s *GeneratedMediaStorageService) UpdateConfig(ctx context.Context, cfg GeneratedMediaStorageConfig) (*GeneratedMediaStorageConfig, error) {
	cfg.normalize()
	if cfg.SecretAccessKey == "" {
		old, err := s.loadLocalConfig(ctx)
		if err != nil {
			return nil, err
		}
		if old != nil {
			if old.SecretAccessKey != "" {
				if s.encryptor == nil {
					return nil, errors.New("generated media secret encryptor is unavailable")
				}
				encrypted, encryptErr := s.encryptor.Encrypt(old.SecretAccessKey)
				if encryptErr != nil {
					return nil, fmt.Errorf("encrypt generated media secret: %w", encryptErr)
				}
				cfg.SecretAccessKey = encrypted
			}
		}
	} else {
		if s.encryptor == nil {
			return nil, errors.New("generated media secret encryptor is unavailable")
		}
		encrypted, err := s.encryptor.Encrypt(cfg.SecretAccessKey)
		if err != nil {
			return nil, fmt.Errorf("encrypt generated media secret: %w", err)
		}
		cfg.SecretAccessKey = encrypted
	}
	if err := cfg.validateEnabled(); err != nil {
		return nil, err
	}

	stored := cfg
	stored.SecretConfigured = false
	data, err := json.Marshal(stored)
	if err != nil {
		return nil, fmt.Errorf("marshal generated media storage config: %w", err)
	}
	if err := s.settingRepo.Set(ctx, settingKeyGeneratedMediaStorage, string(data)); err != nil {
		return nil, fmt.Errorf("save generated media storage config: %w", err)
	}
	s.clearStore()

	cfg.SecretConfigured = cfg.SecretAccessKey != ""
	cfg.SecretAccessKey = ""
	return &cfg, nil
}

func (s *GeneratedMediaStorageService) TestConnection(ctx context.Context, cfg GeneratedMediaStorageConfig) error {
	cfg.normalize()
	if cfg.SecretAccessKey == "" {
		old, err := s.loadLocalConfig(ctx)
		if err != nil {
			return err
		}
		if old != nil {
			cfg.SecretAccessKey = old.SecretAccessKey
		}
	}
	if cfg.Bucket == "" || cfg.AccessKeyID == "" || cfg.SecretAccessKey == "" {
		return ErrGeneratedMediaStorageIncomplete
	}
	store, err := s.storeFactory(ctx, &cfg)
	if err != nil {
		return err
	}
	return store.HeadBucket(ctx)
}

func (s *GeneratedMediaStorageService) Archive(ctx context.Context, sourceURL, objectID string, createdAt time.Time) (string, error) {
	cfg, err := s.resolveConfig(ctx)
	if err != nil {
		return "", err
	}
	if cfg == nil || !cfg.Enabled {
		logger.LegacyPrintf("service.generated_media_storage", "archive skipped object_id=%s reason=disabled", objectID)
		return strings.TrimSpace(sourceURL), nil
	}
	if err := cfg.validateEnabled(); err != nil {
		return "", err
	}
	parsedSource, err := url.Parse(strings.TrimSpace(sourceURL))
	if err != nil || parsedSource.Host == "" || (parsedSource.Scheme != "http" && parsedSource.Scheme != "https") {
		return "", errors.New("generated media source URL is invalid")
	}
	if s.http == nil {
		return "", errors.New("generated media download client is unavailable")
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, parsedSource.String(), nil)
	if err != nil {
		return "", errors.New("create generated media download request")
	}
	resp, err := s.http.Do(req, "", 0, 0)
	if err != nil {
		return "", fmt.Errorf("download generated media: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return "", fmt.Errorf("download generated media returned status %d", resp.StatusCode)
	}
	if resp.ContentLength > generatedMediaMaxBytes {
		return "", errGeneratedMediaTooLarge
	}

	store, err := s.getOrCreateStore(ctx, cfg)
	if err != nil {
		return "", err
	}
	key := generatedMediaObjectKey(cfg.Prefix, objectID, createdAt, parsedSource.Path)
	contentType := strings.TrimSpace(strings.Split(resp.Header.Get("Content-Type"), ";")[0])
	if contentType == "" || contentType == "application/octet-stream" {
		contentType = mime.TypeByExtension(path.Ext(key))
	}
	if contentType == "" {
		contentType = "video/mp4"
	}
	body := &generatedMediaLimitReader{reader: resp.Body, remaining: generatedMediaMaxBytes}
	if _, err := store.Upload(ctx, key, body, contentType, resp.ContentLength); err != nil {
		return "", fmt.Errorf("upload generated media: %w", err)
	}
	publicURL := generatedMediaPublicURL(cfg.PublicBaseURL, key)
	logger.LegacyPrintf("service.generated_media_storage", "archive completed object_id=%s object_key=%s", objectID, key)
	return publicURL, nil
}

func (s *GeneratedMediaStorageService) resolveConfig(ctx context.Context) (*GeneratedMediaStorageConfig, error) {
	if s == nil {
		return nil, nil //nolint:nilnil // an absent service config keeps the upstream URL behavior
	}
	s.configSourceMu.RLock()
	source := s.configSource
	s.configSourceMu.RUnlock()
	if source != nil {
		cfg, handled, err := source.ResolveGeneratedMediaStorageConfig(ctx)
		if handled {
			if err != nil {
				return nil, err
			}
			if cfg != nil {
				cfg.normalize()
			}
			return cfg, nil
		}
	}
	return s.loadLocalConfig(ctx)
}

func (s *GeneratedMediaStorageService) loadLocalConfig(ctx context.Context) (*GeneratedMediaStorageConfig, error) {
	if s == nil || s.settingRepo == nil {
		return nil, nil //nolint:nilnil // an absent service config keeps the upstream URL behavior
	}
	raw, err := s.settingRepo.GetValue(ctx, settingKeyGeneratedMediaStorage)
	if err != nil {
		if errors.Is(err, ErrSettingNotFound) {
			return nil, nil //nolint:nilnil // an absent setting is valid
		}
		return nil, err
	}
	if strings.TrimSpace(raw) == "" {
		return nil, nil //nolint:nilnil // an absent setting is valid
	}
	var cfg GeneratedMediaStorageConfig
	if err := json.Unmarshal([]byte(raw), &cfg); err != nil {
		return nil, ErrGeneratedMediaStorageConfigCorrupt
	}
	cfg.normalize()
	if cfg.SecretAccessKey != "" && s.encryptor != nil {
		decrypted, decryptErr := s.encryptor.Decrypt(cfg.SecretAccessKey)
		if decryptErr != nil {
			logger.LegacyPrintf("service.generated_media_storage", "stored secret decryption failed: %v", decryptErr)
			return nil, ErrGeneratedMediaStorageConfigCorrupt
		}
		cfg.SecretAccessKey = decrypted
	}
	return &cfg, nil
}

func (s *GeneratedMediaStorageService) getOrCreateStore(ctx context.Context, cfg *GeneratedMediaStorageConfig) (GeneratedMediaObjectStore, error) {
	if s.storeFactory == nil {
		return nil, errors.New("generated media object store factory is unavailable")
	}
	fingerprintData, _ := json.Marshal(cfg)
	fingerprint := string(fingerprintData)
	s.storeMu.Lock()
	defer s.storeMu.Unlock()
	if s.store != nil && s.storeFingerprint == fingerprint {
		return s.store, nil
	}
	store, err := s.storeFactory(ctx, cfg)
	if err != nil {
		return nil, err
	}
	s.store = store
	s.storeFingerprint = fingerprint
	return store, nil
}

func (s *GeneratedMediaStorageService) clearStore() {
	s.storeMu.Lock()
	s.store = nil
	s.storeFingerprint = ""
	s.storeMu.Unlock()
}

func generatedMediaObjectKey(prefix, objectID string, createdAt time.Time, sourcePath string) string {
	if createdAt.IsZero() {
		createdAt = time.Now()
	}
	extension := strings.ToLower(path.Ext(sourcePath))
	switch extension {
	case ".mp4", ".mov", ".webm", ".mkv":
	default:
		extension = ".mp4"
	}
	objectID = sanitizeGeneratedMediaObjectID(objectID)
	return path.Join(prefix, createdAt.Format("2006/01/02"), objectID+extension)
}

func sanitizeGeneratedMediaObjectID(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return "video"
	}
	var b strings.Builder
	for _, r := range value {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '-' || r == '_' {
			b.WriteRune(r)
		} else {
			b.WriteByte('_')
		}
	}
	return b.String()
}

func generatedMediaPublicURL(baseURL, key string) string {
	segments := strings.Split(key, "/")
	for i := range segments {
		segments[i] = url.PathEscape(segments[i])
	}
	return strings.TrimRight(baseURL, "/") + "/" + strings.Join(segments, "/")
}

type generatedMediaLimitReader struct {
	reader    io.Reader
	remaining int64
}

func (r *generatedMediaLimitReader) Read(p []byte) (int, error) {
	if r.remaining <= 0 {
		var extra [1]byte
		if n, err := r.reader.Read(extra[:]); n > 0 {
			return 0, errGeneratedMediaTooLarge
		} else {
			return 0, err
		}
	}
	if int64(len(p)) > r.remaining {
		p = p[:r.remaining]
	}
	n, err := r.reader.Read(p)
	r.remaining -= int64(n)
	return n, err
}

var _ GeneratedMediaArchiver = (*GeneratedMediaStorageService)(nil)
