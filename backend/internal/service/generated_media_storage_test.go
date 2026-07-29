package service

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

type generatedMediaSettingRepo struct {
	values map[string]string
}

func newGeneratedMediaSettingRepo() *generatedMediaSettingRepo {
	return &generatedMediaSettingRepo{values: map[string]string{}}
}

func (r *generatedMediaSettingRepo) Get(_ context.Context, key string) (*Setting, error) {
	value, ok := r.values[key]
	if !ok {
		return nil, ErrSettingNotFound
	}
	return &Setting{Key: key, Value: value}, nil
}

func (r *generatedMediaSettingRepo) GetValue(_ context.Context, key string) (string, error) {
	value, ok := r.values[key]
	if !ok {
		return "", ErrSettingNotFound
	}
	return value, nil
}

func (r *generatedMediaSettingRepo) Set(_ context.Context, key, value string) error {
	r.values[key] = value
	return nil
}

func (r *generatedMediaSettingRepo) GetMultiple(_ context.Context, keys []string) (map[string]string, error) {
	result := map[string]string{}
	for _, key := range keys {
		if value, ok := r.values[key]; ok {
			result[key] = value
		}
	}
	return result, nil
}

func (r *generatedMediaSettingRepo) SetMultiple(_ context.Context, values map[string]string) error {
	for key, value := range values {
		r.values[key] = value
	}
	return nil
}

func (r *generatedMediaSettingRepo) GetAll(_ context.Context) (map[string]string, error) {
	return r.values, nil
}

func (r *generatedMediaSettingRepo) Delete(_ context.Context, key string) error {
	delete(r.values, key)
	return nil
}

type generatedMediaTestEncryptor struct{}

func (generatedMediaTestEncryptor) Encrypt(value string) (string, error) {
	return "encrypted:" + value, nil
}

func (generatedMediaTestEncryptor) Decrypt(value string) (string, error) {
	if !strings.HasPrefix(value, "encrypted:") {
		return "", errors.New("invalid ciphertext")
	}
	return strings.TrimPrefix(value, "encrypted:"), nil
}

type generatedMediaTestStore struct {
	key         string
	contentType string
	body        string
	uploadErr   error
	headErr     error
}

func (s *generatedMediaTestStore) Upload(_ context.Context, key string, body io.Reader, contentType string, _ int64) (int64, error) {
	data, err := io.ReadAll(body)
	if err != nil {
		return 0, err
	}
	s.key, s.contentType, s.body = key, contentType, string(data)
	if s.uploadErr != nil {
		return 0, s.uploadErr
	}
	return int64(len(data)), nil
}

func (s *generatedMediaTestStore) HeadBucket(_ context.Context) error { return s.headErr }

type generatedMediaTestHTTP struct {
	HTTPUpstream
	response *http.Response
	err      error
	calls    int
}

func (h *generatedMediaTestHTTP) Do(_ *http.Request, _ string, _ int64, _ int) (*http.Response, error) {
	h.calls++
	return h.response, h.err
}

func generatedMediaTestConfig() GeneratedMediaStorageConfig {
	return GeneratedMediaStorageConfig{
		Enabled: true, Endpoint: "https://cos.ap-guangzhou.myqcloud.com", Region: "ap-guangzhou",
		Bucket: "media-1250000000", AccessKeyID: "secret-id", SecretAccessKey: "secret-key",
		Prefix: "videos", PublicBaseURL: "https://media.example.com", ForcePathStyle: false,
	}
}

func TestGeneratedMediaStorageConfigEncryptsAndPreservesSecret(t *testing.T) {
	repo := newGeneratedMediaSettingRepo()
	store := &generatedMediaTestStore{}
	service := NewGeneratedMediaStorageService(repo, generatedMediaTestEncryptor{}, func(context.Context, *GeneratedMediaStorageConfig) (GeneratedMediaObjectStore, error) {
		return store, nil
	}, nil)

	updated, err := service.UpdateConfig(context.Background(), generatedMediaTestConfig())
	require.NoError(t, err)
	require.True(t, updated.SecretConfigured)
	require.Empty(t, updated.SecretAccessKey)
	require.Contains(t, repo.values[settingKeyGeneratedMediaStorage], "encrypted:secret-key")
	require.NotContains(t, repo.values[settingKeyGeneratedMediaStorage], `"secret_access_key":"secret-key"`)

	patch := generatedMediaTestConfig()
	patch.SecretAccessKey = ""
	patch.Prefix = "archived"
	_, err = service.UpdateConfig(context.Background(), patch)
	require.NoError(t, err)
	require.Contains(t, repo.values[settingKeyGeneratedMediaStorage], "encrypted:secret-key")

	loaded, err := service.GetConfig(context.Background())
	require.NoError(t, err)
	require.True(t, loaded.SecretConfigured)
	require.Empty(t, loaded.SecretAccessKey)
	require.Equal(t, "archived", loaded.Prefix)
}

func TestGeneratedMediaStorageArchiveReturnsPublicURL(t *testing.T) {
	repo := newGeneratedMediaSettingRepo()
	store := &generatedMediaTestStore{}
	httpClient := &generatedMediaTestHTTP{response: &http.Response{
		StatusCode:    http.StatusOK,
		Header:        http.Header{"Content-Type": []string{"video/mp4"}},
		Body:          io.NopCloser(strings.NewReader("video-bytes")),
		ContentLength: int64(len("video-bytes")),
	}}
	service := NewGeneratedMediaStorageService(repo, generatedMediaTestEncryptor{}, func(context.Context, *GeneratedMediaStorageConfig) (GeneratedMediaObjectStore, error) {
		return store, nil
	}, httpClient)
	_, err := service.UpdateConfig(context.Background(), generatedMediaTestConfig())
	require.NoError(t, err)

	result, err := service.Archive(context.Background(), "https://upstream.example.com/result.mp4?token=private", "video_task-1", time.Date(2026, 7, 29, 12, 0, 0, 0, time.UTC))
	require.NoError(t, err)
	require.Equal(t, "https://media.example.com/videos/2026/07/29/video_task-1.mp4", result)
	require.Equal(t, "videos/2026/07/29/video_task-1.mp4", store.key)
	require.Equal(t, "video/mp4", store.contentType)
	require.Equal(t, "video-bytes", store.body)
	require.Equal(t, 1, httpClient.calls)
}

func TestGeneratedMediaStorageDisabledKeepsUpstreamURL(t *testing.T) {
	repo := newGeneratedMediaSettingRepo()
	httpClient := &generatedMediaTestHTTP{err: fmt.Errorf("download should not run")}
	service := NewGeneratedMediaStorageService(repo, generatedMediaTestEncryptor{}, nil, httpClient)
	cfg := generatedMediaTestConfig()
	cfg.Enabled = false
	_, err := service.UpdateConfig(context.Background(), cfg)
	require.NoError(t, err)

	source := "https://upstream.example.com/result.mp4?token=private"
	result, err := service.Archive(context.Background(), source, "video_task-1", time.Now())
	require.NoError(t, err)
	require.Equal(t, source, result)
	require.Zero(t, httpClient.calls)
}
