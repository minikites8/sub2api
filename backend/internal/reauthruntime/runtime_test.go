package reauthruntime

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func archive(t *testing.T, entries []*tar.Header, content []string) []byte {
	t.Helper()
	var data bytes.Buffer
	gz := gzip.NewWriter(&data)
	tw := tar.NewWriter(gz)
	for i, h := range entries {
		require.NoError(t, tw.WriteHeader(h))
		if h.Typeflag == tar.TypeReg {
			_, err := tw.Write([]byte(content[i]))
			require.NoError(t, err)
		}
	}
	require.NoError(t, tw.Close())
	require.NoError(t, gz.Close())
	return data.Bytes()
}

func TestExtractRejectsEscapesAndLinks(t *testing.T) {
	for _, h := range []*tar.Header{
		{Name: "../escape", Typeflag: tar.TypeReg, Size: 1},
		{Name: "/escape", Typeflag: tar.TypeReg, Size: 1},
		{Name: "link", Typeflag: tar.TypeSymlink, Linkname: "../escape"},
		{Name: "device", Typeflag: tar.TypeChar},
	} {
		t.Run(h.Name, func(t *testing.T) {
			require.Error(t, extract(archive(t, []*tar.Header{h}, []string{"x"}), t.TempDir()))
		})
	}
}

func TestExtractPreservesExecutableAndRejectsDuplicate(t *testing.T) {
	root := t.TempDir()
	h := &tar.Header{Name: "bin/run", Typeflag: tar.TypeReg, Size: 4, Mode: 0755}
	require.NoError(t, extract(archive(t, []*tar.Header{h}, []string{"test"}), root))
	info, err := os.Stat(filepath.Join(root, "bin/run"))
	require.NoError(t, err)
	require.Equal(t, os.FileMode(0700), info.Mode().Perm())
	require.Error(t, extract(archive(t, []*tar.Header{h}, []string{"test"}), root))
}

type transportFunc func(*http.Request) (*http.Response, error)

func (f transportFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

// Packaging CI supplies the real architecture-matched archive. Only exercise
// local API requests with empty queues; never issue an upstream login.
func TestReleaseBundleDrainsEmptyQueue(t *testing.T) {
	if os.Getenv("TEST_REAUTH_RELEASE_BUNDLE") != "1" {
		t.Skip("requires a packaged Linux binary")
	}
	var claims atomic.Int64
	token := strings.Repeat("x", 64)
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-OpenAI-Reauth-Worker-Token") != token {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		if r.URL.Path == "/api/v1/internal/openai-reauth/runtime-settings" {
			_, _ = io.WriteString(w, `{"code":0,"data":{"worker_concurrency":1}}`)
			return
		}
		if r.URL.Path == "/api/v1/internal/openai-reauth/claim" {
			claims.Add(1)
		}
		_, _ = io.WriteString(w, `{"code":0,"data":null}`)
	}))
	defer api.Close()
	m := New(t.TempDir(), "main", api.URL, token)
	require.True(t, m.Bundled())
	defer m.Stop()
	m.Notify()
	require.Eventually(t, func() bool { return claims.Load() == 1 && m.Status().State == "idle" }, 2*time.Minute, 100*time.Millisecond)
}

func TestBundledDrainRestartsForCommittedWorkAndReturnsToIdle(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("Linux helper")
	}
	m := New(t.TempDir(), "main", "http://127.0.0.1", strings.Repeat("x", 32))
	defer m.Stop()
	// No upstream or local API is needed. Hold the first drain in the window
	// after its last claim and prove multiple concurrent enqueues wake it once.
	script := `#!/bin/sh
case "$2" in
  --check) exit 0;;
  --drain) echo drain >> runs; while [ ! -f release ]; do sleep 0.05; done;;
  *) exit 1;;
esac`
	m.archive = archive(t, []*tar.Header{{Name: "python", Mode: 0755, Typeflag: tar.TypeReg, Size: int64(len(script))}}, []string{script})
	m.client.Transport = transportFunc(func(*http.Request) (*http.Response, error) { panic("network used for embedded archive") })
	dir := filepath.Join(m.root, "bundle-"+m.archiveDigest())
	m.Notify()
	require.Eventually(t, func() bool { _, err := os.Stat(filepath.Join(dir, "runs")); return err == nil }, 5*time.Second, 10*time.Millisecond)
	m.Notify()
	m.Notify()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "release"), nil, 0600))
	require.Eventually(t, func() bool {
		b, _ := os.ReadFile(filepath.Join(dir, "runs"))
		return string(b) == "drain\ndrain\n" && m.Status().State == "idle"
	}, 5*time.Second, 10*time.Millisecond)
}

func TestPrepareVerifiesDigestBeforeExecutingAndReusesCache(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("Linux runtime")
	}
	for _, valid := range []bool{false, true} {
		t.Run(map[bool]string{true: "valid", false: "tampered"}[valid], func(t *testing.T) {
			root := t.TempDir()
			script := "#!/bin/sh\nexit 0\n"
			data := archive(t, []*tar.Header{{Name: "python", Typeflag: tar.TypeReg, Size: int64(len(script)), Mode: 0755}}, []string{script})
			sum := sha256.Sum256(data)
			digest := hex.EncodeToString(sum[:])
			if !valid {
				digest = strings.Repeat("0", 64)
			}
			requests := 0
			m := New(root, "1.2.3", "http://127.0.0.1:4040", strings.Repeat("x", 64))
			m.archive = nil // Exercise the unbundled release-download fallback.
			m.client = &http.Client{Transport: transportFunc(func(req *http.Request) (*http.Response, error) {
				requests++
				require.NotContains(t, req.URL.String(), m.token)
				body := data
				if strings.Contains(req.URL.Path, "/releases/tags/") {
					body, _ = json.Marshal(map[string]any{"assets": []map[string]string{{"name": "sub2api-reauth_1.2.3_linux_" + runtime.GOARCH + ".tar.gz", "digest": "sha256:" + digest}}})
				}
				return &http.Response{StatusCode: 200, Body: io.NopCloser(bytes.NewReader(body)), Header: http.Header{}}, nil
			})}
			dir, err := m.prepare(context.Background())
			if !valid {
				require.ErrorContains(t, err, "checksum")
				return
			}
			require.NoError(t, err)
			require.FileExists(t, filepath.Join(dir, "ready"))
			_, err = m.prepare(context.Background())
			require.NoError(t, err)
			require.Equal(t, 2, requests)
		})
	}
}

func TestStopCancelsPreparationAndCannotRestart(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("Linux runtime preparation")
	}
	m := New(t.TempDir(), "1.2.3", "http://127.0.0.1:4040", strings.Repeat("x", 64))
	m.archive = nil
	entered := make(chan struct{})
	m.client = &http.Client{Transport: transportFunc(func(req *http.Request) (*http.Response, error) {
		close(entered)
		<-req.Context().Done()
		return nil, req.Context().Err()
	})}
	m.Ensure()
	select {
	case <-entered:
	case <-time.After(time.Second):
		t.Fatal("preparation did not start")
	}
	m.Stop()
	require.Equal(t, "unavailable", m.Status().State)
	m.Ensure()
	require.False(t, m.started)
}

func TestManagedProcessGetsOnlyWorkerEnvironmentAndStops(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("Linux process supervision")
	}
	root := t.TempDir()
	dir := filepath.Join(root, "1.2.3-linux-"+runtime.GOARCH)
	require.NoError(t, os.MkdirAll(dir, 0700))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "ready"), []byte("cached"), 0600))
	script := "#!/bin/sh\nprintf '%s' \"$OPENAI_REAUTH_WORKER_TOKEN\" > worker-token\nprintf '%s' \"$DATABASE_PASSWORD\" > unrelated-secret\nprintf '%s' \"$OPENAI_REAUTH_CONCURRENCY\" > concurrency\nexec /bin/sleep 60\n"
	require.NoError(t, os.WriteFile(filepath.Join(dir, "python"), []byte(script), 0700))
	t.Setenv("OPENAI_REAUTH_CONCURRENCY", "4")
	t.Setenv("DATABASE_PASSWORD", "must-not-inherit")
	m := New(root, "1.2.3", "http://127.0.0.1:4040", "synthetic-worker-token")
	m.archive = nil
	m.Ensure()
	defer m.Stop()
	// Child output can precede the parent's running status update after Start.
	// Wait for both independently, including the last environment file write.
	require.Eventually(t, func() bool {
		return m.Status().State == "running"
	}, 5*time.Second, 10*time.Millisecond)
	require.Eventually(t, func() bool {
		got, err := os.ReadFile(filepath.Join(dir, "concurrency"))
		return err == nil && string(got) == "4"
	}, 5*time.Second, 10*time.Millisecond)
	got, err := os.ReadFile(filepath.Join(dir, "worker-token"))
	require.NoError(t, err)
	require.Equal(t, "synthetic-worker-token", string(got))
	got, err = os.ReadFile(filepath.Join(dir, "unrelated-secret"))
	require.NoError(t, err)
	require.Empty(t, got)
	got, err = os.ReadFile(filepath.Join(dir, "concurrency"))
	require.NoError(t, err)
	require.Equal(t, "4", string(got))
	m.Stop()
	require.Equal(t, "stopped", m.Status().State)
}
