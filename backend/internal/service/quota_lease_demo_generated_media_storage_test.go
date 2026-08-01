package service

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
)

func newQuotaLeaseGeneratedMediaControl(t *testing.T) (*QuotaLeaseDemoService, *GeneratedMediaStorageService, string) {
	t.Helper()
	control := NewQuotaLeaseDemoService(&config.Config{Gateway: config.GatewayConfig{
		QuotaLeaseDemo: config.GatewayQuotaLeaseDemoConfig{
			Enabled: true, NodeID: "control", NodeSecret: "control-secret",
			DefaultGrantAmount: 1, LeaseTTLSeconds: 600,
		},
	}})
	repo := newGeneratedMediaSettingRepo()
	storage := NewGeneratedMediaStorageService(repo, generatedMediaTestEncryptor{}, nil, nil)
	_, err := storage.UpdateConfig(context.Background(), generatedMediaTestConfig())
	require.NoError(t, err)
	control.SetGeneratedMediaStorageService(storage)

	registered, err := control.RegisterNode(context.Background(), QuotaLeaseDemoNodeRegistrationRequest{
		NodeID: "video-node-1", NodeSecret: "video-node-secret", BaseURL: "https://video-node-1.example",
	})
	require.NoError(t, err)
	require.NotNil(t, registered)
	return control, storage, registered.NodeSecret
}

func TestQuotaLeaseGeneratedMediaStorageEnvelopeEncryptsCredentials(t *testing.T) {
	control, _, nodeSecret := newQuotaLeaseGeneratedMediaControl(t)

	envelope, err := control.EncryptGeneratedMediaStorageConfigForNode(context.Background(), "video-node-1", nodeSecret)
	require.NoError(t, err)
	require.Equal(t, quotaLeaseGeneratedMediaConfigVersion, envelope.Version)

	wirePayload, err := json.Marshal(envelope)
	require.NoError(t, err)
	require.NotContains(t, string(wirePayload), "secret-key")
	require.NotContains(t, string(wirePayload), "media-1250000000")

	decrypted, err := decryptQuotaLeaseGeneratedMediaStorageConfig(envelope, "video-node-1", nodeSecret, time.Now())
	require.NoError(t, err)
	require.True(t, decrypted.Config.Enabled)
	require.Equal(t, "secret-key", decrypted.Config.SecretAccessKey)
	require.Equal(t, "media-1250000000", decrypted.Config.Bucket)

	_, err = decryptQuotaLeaseGeneratedMediaStorageConfig(envelope, "video-node-1", "wrong-secret", time.Now())
	require.Error(t, err)
	_, err = decryptQuotaLeaseGeneratedMediaStorageConfig(envelope, "another-node", nodeSecret, time.Now())
	require.Error(t, err)
}

func TestQuotaLeaseRemoteGeneratedMediaStorageConfigDrivesArchive(t *testing.T) {
	control, _, _ := newQuotaLeaseGeneratedMediaControl(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/v1/node-leases/nodes/register":
			require.Equal(t, "control-secret", r.Header.Get("X-Node-Secret"))
			var req QuotaLeaseDemoNodeRegistrationRequest
			require.NoError(t, json.NewDecoder(r.Body).Decode(&req))
			result, err := control.RegisterNode(r.Context(), req)
			require.NoError(t, err)
			require.NoError(t, json.NewEncoder(w).Encode(result))
		case "/api/v1/node-leases/generated-media-storage/config":
			nodeID := r.Header.Get("X-Node-ID")
			nodeSecret := r.Header.Get("X-Node-Secret")
			require.True(t, control.AuthenticateNode(nodeID, nodeSecret))
			envelope, err := control.EncryptGeneratedMediaStorageConfigForNode(r.Context(), nodeID, nodeSecret)
			require.NoError(t, err)
			require.NoError(t, json.NewEncoder(w).Encode(map[string]any{"data": envelope}))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	node := NewQuotaLeaseDemoService(&config.Config{Gateway: config.GatewayConfig{
		QuotaLeaseDemo: config.GatewayQuotaLeaseDemoConfig{
			Enabled: true, NodeID: "remote-video-node", ControlPlaneBaseURL: server.URL,
			ControlPlaneKey: "control-secret", DefaultGrantAmount: 1, LeaseTTLSeconds: 600,
		},
	}})
	store := &generatedMediaTestStore{}
	httpClient := &generatedMediaTestHTTP{response: &http.Response{
		StatusCode:    http.StatusOK,
		Header:        http.Header{"Content-Type": []string{"video/mp4"}},
		Body:          io.NopCloser(strings.NewReader("video-bytes")),
		ContentLength: int64(len("video-bytes")),
	}}
	storage := NewGeneratedMediaStorageService(newGeneratedMediaSettingRepo(), generatedMediaTestEncryptor{}, func(context.Context, *GeneratedMediaStorageConfig) (GeneratedMediaObjectStore, error) {
		return store, nil
	}, httpClient)
	storage.SetConfigSource(node)

	result, err := storage.Archive(context.Background(), "https://upstream.example.com/result.mp4?token=private", "video_remote-1", time.Date(2026, 8, 1, 12, 0, 0, 0, time.UTC))
	require.NoError(t, err)
	require.Equal(t, "https://media.example.com/videos/2026/08/01/video_remote-1.mp4", result)
	require.Equal(t, "videos/2026/08/01/video_remote-1.mp4", store.key)
	require.Equal(t, "video-bytes", store.body)
}

func TestQuotaLeaseRemoteGeneratedMediaStorageConfigFailsClosed(t *testing.T) {
	node := NewQuotaLeaseDemoService(&config.Config{Gateway: config.GatewayConfig{
		QuotaLeaseDemo: config.GatewayQuotaLeaseDemoConfig{
			Enabled: true, NodeID: "offline-video-node", ControlPlaneBaseURL: "http://127.0.0.1:1",
			ControlPlaneKey: "control-secret", DefaultGrantAmount: 1, LeaseTTLSeconds: 600,
			RemoteTimeoutSeconds: 1,
		},
	}})
	cfg, handled, err := node.ResolveGeneratedMediaStorageConfig(context.Background())
	require.Error(t, err)
	require.True(t, handled)
	require.Nil(t, cfg)
}

func TestQuotaLeaseRemoteGeneratedMediaStorageConfigRequiresHTTPS(t *testing.T) {
	node := NewQuotaLeaseDemoService(&config.Config{Gateway: config.GatewayConfig{
		QuotaLeaseDemo: config.GatewayQuotaLeaseDemoConfig{
			Enabled: true, NodeID: "insecure-video-node", ControlPlaneBaseURL: "http://control.example.com",
			ControlPlaneKey: "control-secret", DefaultGrantAmount: 1, LeaseTTLSeconds: 600,
		},
	}})
	cfg, handled, err := node.ResolveGeneratedMediaStorageConfig(context.Background())
	require.EqualError(t, err, "sync generated media storage config: generated media storage sync control plane URL requires HTTPS")
	require.True(t, handled)
	require.Nil(t, cfg)
}
