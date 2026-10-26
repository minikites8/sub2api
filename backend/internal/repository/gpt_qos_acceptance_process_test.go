//go:build integration

package repository

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/golang-jwt/jwt/v5"
	"github.com/stretchr/testify/require"
)

const acceptanceJWT = "isolated-qos-acceptance-signing-secret-only-2026-10-26"

type acceptanceEvent struct {
	ID      string    `json:"request"`
	Account int64     `json:"account"`
	Quality string    `json:"quality"`
	Started time.Time `json:"started"`
	Ended   time.Time `json:"ended"`
	Status  int       `json:"status"`
}
type acceptanceUpstream struct {
	mu              sync.Mutex
	accounts        map[int64]string
	events          []*acceptanceEvent
	active, maximum map[string]int
	failures        map[string]int
}

func (u *acceptanceUpstream) serve(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost || !strings.HasSuffix(r.URL.Path, "/responses") {
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"object":"list","data":[{"id":"gpt-5.2","object":"model"}]}`)
		return
	}
	var body struct {
		Model    string
		Input    string
		Previous string `json:"previous_response_id"`
	}
	if json.NewDecoder(r.Body).Decode(&body) != nil {
		http.Error(w, "invalid fixture", 400)
		return
	}
	id, _ := strconv.ParseInt(strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer fixture-account-"), 10, 64)
	u.mu.Lock()
	quality := u.accounts[id]
	event := &acceptanceEvent{ID: body.Input, Account: id, Quality: quality, Started: time.Now(), Status: 200}
	u.events = append(u.events, event)
	u.active[quality]++
	if u.active[quality] > u.maximum[quality] {
		u.maximum[quality] = u.active[quality]
	}
	u.mu.Unlock()
	defer func() { u.mu.Lock(); u.active[quality]--; event.Ended = time.Now(); u.mu.Unlock() }()
	if strings.Contains(body.Input, "partial-drop") {
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprintf(w, "data: {\"type\":\"response.output_text.delta\",\"delta\":\"irreversible-fixture-token\"}\n\n")
		w.(http.Flusher).Flush()
		return // EOF after semantic output, with no completed/usage event.
	}
	if strings.Contains(body.Input, "fail429") && quality == "strong" {
		u.mu.Lock()
		failed := u.failures[body.Input] == 0
		u.failures[body.Input]++
		if failed {
			event.Status = 429
		}
		u.mu.Unlock()
		if failed {
			w.Header().Set("Retry-After", "1")
			w.WriteHeader(429)
			_, _ = io.WriteString(w, `{"error":{"type":"rate_limit_error","message":"isolated fixture throttled"}}`)
			return
		}
	}
	delay := 40 * time.Millisecond
	if strings.Contains(body.Input, "slow") {
		delay = 300 * time.Millisecond
	}
	if strings.Contains(body.Input, "hold") {
		delay = 2 * time.Second
	}
	select {
	case <-r.Context().Done():
		return
	case <-time.After(delay):
	}
	w.Header().Set("Content-Type", "text/event-stream")
	response := map[string]any{"type": "response.completed", "response": map[string]any{
		"id": fmt.Sprintf("resp_%d_%s", id, body.Input), "object": "response", "model": body.Model, "status": "completed",
		"output": []any{map[string]any{"id": "msg_fixture", "type": "message", "role": "assistant", "status": "completed", "content": []any{map[string]any{"type": "output_text", "text": "fixture-ok", "annotations": []any{}}}}},
		"usage":  map[string]any{"input_tokens": 100, "output_tokens": 50, "total_tokens": 150},
	}}
	encoded, _ := json.Marshal(response)
	fmt.Fprintf(w, "data: %s\n\n", encoded)
}

type acceptanceProcess struct {
	cmd      *exec.Cmd
	URL, log string
}
type acceptanceHarness struct {
	t                       *testing.T
	dir, binary, adminToken string
	upstream                *acceptanceUpstream
	upstreamURL             string
	processes               []*acceptanceProcess
	groups                  map[string]int64
	keys                    map[string][]string
	strong, basic           []int64
	policy                  service.GPTQoSConfig
	redisProxy              *acceptanceRedisProxy
}

func acceptanceToken(t *testing.T, user *service.User) string {
	t.Helper()
	version := user.TokenVersion
	if !user.TokenVersionResolved {
		sum := sha256.Sum256([]byte(strings.ToLower(strings.TrimSpace(user.Email)) + "\n" + user.PasswordHash))
		version ^= int64(binary.BigEndian.Uint64(sum[:8]) & 0x7fffffffffffffff)
	}
	claims := service.JWTClaims{UserID: user.ID, Email: user.Email, Role: user.Role, TokenVersion: version,
		RegisteredClaims: jwt.RegisteredClaims{ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour)), IssuedAt: jwt.NewNumericDate(time.Now())}}
	token, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString([]byte(acceptanceJWT))
	require.NoError(t, err)
	return token
}
func newAcceptanceHarness(t *testing.T) *acceptanceHarness {
	t.Helper()
	binary := os.Getenv("SUB2API_ACCEPTANCE_BINARY")
	if binary == "" {
		t.Skip("set SUB2API_ACCEPTANCE_BINARY to a locally built cmd/server binary")
	}
	require.FileExists(t, binary)
	dir := os.Getenv("SUB2API_ACCEPTANCE_EVIDENCE")
	if dir == "" {
		dir = t.TempDir()
	}
	require.NoError(t, os.MkdirAll(dir, 0700))
	h := &acceptanceHarness{t: t, dir: dir, binary: binary, groups: map[string]int64{}, keys: map[string][]string{},
		upstream: &acceptanceUpstream{accounts: map[int64]string{}, active: map[string]int{}, maximum: map[string]int{}, failures: map[string]int{}}}
	h.redisProxy = newAcceptanceRedisProxy(t, integrationRedis.Options().Addr)
	srv := httptest.NewServer(http.HandlerFunc(h.upstream.serve))
	t.Cleanup(srv.Close)
	h.upstreamURL = srv.URL
	ctx := context.Background()
	// Scheduler saturation uses many fixture users at one loopback IP; disable unrelated anti-abuse only in this disposable database.
	require.NoError(t, NewSettingRepository(integrationEntClient).Set(ctx, service.SettingKeyAntiAbuseEnabled, "false"))
	require.NoError(t, NewSettingRepository(integrationEntClient).Set(ctx, service.SettingKeyAPIUsageIPUARiskControlThreshold, "100000"))
	admin := mustCreateUser(t, integrationEntClient, &service.User{Email: "audit-admin@example.invalid", Role: service.RoleAdmin, Balance: 1000, Concurrency: 1000})
	h.adminToken = acceptanceToken(t, admin)
	ack, _ := json.Marshal(service.AdminComplianceAcknowledgement{Version: service.AdminComplianceVersion, AdminUserID: admin.ID})
	require.NoError(t, NewSettingRepository(integrationEntClient).Set(ctx, fmt.Sprintf("admin_compliance_acknowledgement:%d", admin.ID), string(ack)))
	for _, tier := range []string{"premium", "standard", "economy"} {
		group := mustCreateGroup(t, integrationEntClient, &service.Group{Name: "audit-" + tier, Platform: service.PlatformOpenAI, RateMultiplier: 1})
		h.groups[tier] = group.ID
		for i := 0; i < 10; i++ {
			user := mustCreateUser(t, integrationEntClient, &service.User{Email: fmt.Sprintf("audit-%s-%d@example.invalid", tier, i), Balance: 1000, Concurrency: 1000})
			key := fmt.Sprintf("sk-audit-%s-user-%02d-local-fixture", tier, i)
			_, err := integrationEntClient.APIKey.Create().SetUserID(user.ID).SetGroupID(group.ID).SetKey(key).SetName("audit-key").SetStatus(service.StatusActive).Save(ctx)
			require.NoError(t, err)
			h.keys[tier] = append(h.keys[tier], key)
		}
	}
	for _, quality := range []string{"strong", "basic"} {
		for i := 0; i < 2; i++ {
			cap := 10
			if quality == "basic" {
				cap = 15
			}
			a := mustCreateAccount(t, integrationEntClient, &service.Account{Name: fmt.Sprintf("audit-%s-%d", quality, i), Platform: service.PlatformOpenAI, Type: service.AccountTypeAPIKey, Concurrency: cap,
				Credentials: map[string]any{"base_url": srv.URL + "/v1", "api_key": "placeholder"}, Extra: map[string]any{service.AccountQualityTierKey: quality, "openai_passthrough": true}})
			_, err := integrationEntClient.Account.UpdateOneID(a.ID).SetCredentials(map[string]any{"base_url": srv.URL + "/v1", "api_key": fmt.Sprintf("fixture-account-%d", a.ID)}).Save(ctx)
			require.NoError(t, err)
			for _, gid := range h.groups {
				_, err = integrationEntClient.AccountGroup.Create().SetAccountID(a.ID).SetGroupID(gid).Save(ctx)
				require.NoError(t, err)
			}
			h.upstream.accounts[a.ID] = quality
			if quality == "strong" {
				h.strong = append(h.strong, a.ID)
			} else {
				h.basic = append(h.basic, a.ID)
			}
		}
	}
	policy := service.DefaultGPTQoSConfig()
	policy.Enabled = true
	policy.QualityRoutingEnabled = true
	policy.LogSampleRate = 1
	for tier, gid := range h.groups {
		g := service.DefaultGPTQoSGroupConfig(tier)
		g.StrongWaitMs = 5000
		g.UserMaxWaiting = 20
		policy.Groups[strconv.FormatInt(gid, 10)] = g
	}
	require.NoError(t, service.NewSettingService(NewSettingRepository(integrationEntClient), nil).SaveGPTQoSConfig(ctx, policy))
	h.policy = policy
	for i := 0; i < 2; i++ {
		h.processes = append(h.processes, h.start(i))
	}
	t.Cleanup(func() {
		data, _ := json.MarshalIndent(h.upstream.events, "", "  ")
		_ = os.WriteFile(filepath.Join(h.dir, "upstream-events.json"), data, 0600)
	})
	return h
}

func (h *acceptanceHarness) start(index int) *acceptanceProcess {
	t := h.t
	pg, err := url.Parse(integrationPostgresDSN)
	require.NoError(t, err)
	require.Contains(t, []string{"localhost", "127.0.0.1", "::1"}, pg.Hostname(), "never start against a non-loopback database")
	redisHost, redisPort, err := net.SplitHostPort(h.redisProxy.listener.Addr().String())
	require.NoError(t, err)
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	port := listener.Addr().(*net.TCPAddr).Port
	require.NoError(t, listener.Close())
	dir := filepath.Join(h.dir, fmt.Sprintf("instance-%d", index))
	require.NoError(t, os.MkdirAll(dir, 0700))
	password, _ := pg.User.Password()
	backendRoot, err := filepath.Abs("../..")
	require.NoError(t, err)
	text := fmt.Sprintf(`server:
  host: "127.0.0.1"
  port: %d
database:
  host: %q
  port: %s
  user: %q
  password: %q
  dbname: %q
  sslmode: disable
  max_open_conns: 20
  max_idle_conns: 10
redis:
  host: %q
  port: %s
jwt:
  secret: %q
run_mode: standard
security:
  url_allowlist:
    enabled: false
    allow_private_hosts: true
    allow_insecure_http: true
    upstream_hosts: ["127.0.0.1", "localhost"]
    pricing_hosts: ["127.0.0.1", "localhost"]
pricing:
  remote_url: %q
  hash_url: %q
  fallback_file: %q
gateway:
  concurrency_slot_ttl_minutes: 1
  scheduling:
    db_fallback_enabled: true
    outbox_poll_interval_seconds: 1
    full_rebuild_interval_seconds: 5
  api_key_queue:
    max_waiting: 0
`, port, pg.Hostname(), pg.Port(), pg.User.Username(), password, strings.TrimPrefix(pg.Path, "/"), redisHost, redisPort, acceptanceJWT,
		h.upstreamURL+"/pricing", h.upstreamURL+"/hash", filepath.Join(backendRoot, "resources/model-pricing/model_prices_and_context_window.json"))
	configFile := filepath.Join(dir, "config.yaml")
	require.NoError(t, os.WriteFile(configFile, []byte(text), 0600))
	logPath := filepath.Join(dir, "server.log")
	logFile, err := os.Create(logPath)
	require.NoError(t, err)
	cmd := exec.Command(h.binary)
	cmd.Dir = dir
	cmd.Env = []string{"PATH=" + os.Getenv("PATH"), "SKIP_SETUP=true", "DATA_DIR=" + dir, "CONFIG_FILE=" + configFile, "TZ=UTC", "HOME=" + dir, "HTTP_PROXY=http://127.0.0.1:1", "HTTPS_PROXY=http://127.0.0.1:1", "NO_PROXY=localhost,127.0.0.1"}
	cmd.Stdout = logFile
	cmd.Stderr = logFile
	require.NoError(t, cmd.Start())
	p := &acceptanceProcess{cmd: cmd, URL: fmt.Sprintf("http://127.0.0.1:%d", port), log: logPath}
	t.Cleanup(func() { _ = cmd.Process.Kill(); _ = cmd.Wait(); _ = logFile.Close() })
	client := &http.Client{Timeout: time.Second}
	require.Eventually(t, func() bool {
		resp, err := client.Get(p.URL + "/health")
		if err != nil {
			return false
		}
		defer resp.Body.Close()
		return resp.StatusCode == 200
	}, 60*time.Second, 100*time.Millisecond, "service failed to boot; inspect %s", logPath)
	t.Logf("real Sub2API process pid=%d url=%s", cmd.Process.Pid, p.URL)
	return p
}

func (h *acceptanceHarness) request(ctx context.Context, index int, key, id string, stream bool) (int, []byte, time.Duration) {
	return h.requestBody(ctx, index, key, map[string]any{"model": "gpt-5.2", "input": id, "stream": stream})
}
func (h *acceptanceHarness) requestBody(ctx context.Context, index int, key string, value any) (int, []byte, time.Duration) {
	started := time.Now()
	body, _ := json.Marshal(value)
	req, _ := http.NewRequestWithContext(ctx, http.MethodPost, h.processes[index%len(h.processes)].URL+"/v1/responses", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+key)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return 0, []byte(err.Error()), time.Since(started)
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, data, time.Since(started)
}

func TestGPTQoSAcceptanceTwoProcesses(t *testing.T) {
	h := newAcceptanceHarness(t)
	for i := 0; i < 2; i++ {
		status, body, _ := h.request(context.Background(), i, h.keys["premium"][0], fmt.Sprintf("smoke-%d", i), false)
		require.Equal(t, 200, status, string(body))
		require.Contains(t, string(body), "fixture-ok")
	}
	h.runAcceptance(t)
}
