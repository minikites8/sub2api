package service

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/openai"
	"github.com/google/uuid"
)

var ErrOpenAICodexTicketUnavailable = errors.New("codex turn-state ticket unavailable")

type codexTurnTicket struct {
	attempts   int
	state      string
	expires    time.Time
	credential [32]byte
}
type codexTicketBackoff struct {
	until        time.Time
	credential   [32]byte
	unauthorized bool
	failures     int
}

func codexTicketAccount(account *Account) bool {
	return account != nil && account.IsOpenAIOAuthLike() && !account.IsShadow()
}
func codexTicketCredential(account *Account) [32]byte {
	return sha256.Sum256([]byte(account.GetCredential("access_token") + "\x00" + account.GetCredential("chatgpt_account_id")))
}
func codexTicketLength(account *Account) int {
	plan := strings.ToLower(account.GetCredential("plan_type"))
	plan = strings.Map(func(r rune) rune {
		switch r {
		case ' ', '\t', '\n', '\r', '-', '_':
			return -1
		}
		return r
	}, plan)
	switch plan {
	case "team", "business", "selfservebusinessprolite":
		return 332
	}
	return 292
}
func codexTicketKey(accountID int64, model string) string {
	return fmt.Sprintf("%d\x00%s", accountID, strings.TrimSpace(model))
}
func (t *codexTurnTicket) valid(account *Account, now time.Time) bool {
	return t != nil && account != nil && now.Before(t.expires) && t.credential == codexTicketCredential(account) && len(t.state) == codexTicketLength(account) && strings.HasPrefix(t.state, "gAAAAA")
}
func (t *codexTurnTicket) validSchedulerSnapshot(account *Account, now time.Time) bool {
	return t != nil && account != nil && now.Before(t.expires) && len(t.state) == codexTicketLength(account) && strings.HasPrefix(t.state, "gAAAAA")
}
func (s *OpenAIGatewayService) codexTicketConfig(ctx context.Context) config.OpenAICodexTicketConfig {
	cfg := config.OpenAICodexTicketConfig{}
	if s == nil {
		return normalizeCodexTicketConfig(cfg)
	}
	if s.cfg != nil {
		cfg = s.cfg.Gateway.OpenAICodexTicket
	}
	return s.settingService.codexTicketRuntimeConfig(ctx, cfg)
}
func codexTicketGated(cfg config.OpenAICodexTicketConfig, account *Account, model string) bool {
	if !cfg.Enabled || !codexTicketAccount(account) {
		return false
	}
	for _, item := range cfg.Models {
		if strings.TrimSpace(model) == item {
			return true
		}
	}
	return false
}
func (s *OpenAIGatewayService) cachedCodexTicket(account *Account, model string) *codexTurnTicket {
	v, _ := s.codexTickets.Load(codexTicketKey(account.ID, model))
	t, _ := v.(*codexTurnTicket)
	return t
}
func (s *OpenAIGatewayService) applyOpenAICodexTicket(ctx context.Context, account *Account, model string, headers http.Header) error {
	if s == nil || headers == nil || !codexTicketAccount(account) {
		return nil
	}
	cfg := s.codexTicketConfig(ctx)
	if !codexTicketGated(cfg, account, model) {
		return nil
	}
	if ticket := s.cachedCodexTicket(account, model); ticket.valid(account, time.Now()) {
		headers.Set(openAICodexTurnStateHeader, ticket.state)
		return nil
	}
	if cfg.FailClosed {
		return ErrOpenAICodexTicketUnavailable
	}
	return nil
}
func (s *OpenAIGatewayService) codexTicketBlocksAccount(ctx context.Context, account *Account, requestedModel string, requireCompact bool) bool {
	if s == nil || !codexTicketAccount(account) {
		return false
	}
	cfg := s.codexTicketConfig(ctx)
	if !cfg.Enabled || !cfg.FailClosed {
		return false
	}
	_, model := resolveOpenAIForwardMappedModels(account, requestedModel, requireCompact)
	if requireCompact {
		if fallback := strings.TrimSpace(s.resolveOpenAICompactFallbackModel(account, requestedModel)); fallback != "" {
			model = fallback
		}
	}
	return codexTicketGated(cfg, account, model) && !s.cachedCodexTicket(account, model).valid(account, time.Now())
}

func (s *OpenAIGatewayService) codexTicketBlocksSchedulerSnapshot(ctx context.Context, account *Account, requestedModel string, requireCompact bool) bool {
	if s == nil || !codexTicketAccount(account) {
		return false
	}
	cfg := s.codexTicketConfig(ctx)
	if !cfg.Enabled || !cfg.FailClosed {
		return false
	}
	_, model := resolveOpenAIForwardMappedModels(account, requestedModel, requireCompact)
	if requireCompact {
		if fallback := strings.TrimSpace(s.resolveOpenAICompactFallbackModel(account, requestedModel)); fallback != "" {
			model = fallback
		}
	}
	return codexTicketGated(cfg, account, model) && !s.cachedCodexTicket(account, model).validSchedulerSnapshot(account, time.Now())
}

// Lifecycle is idempotent. Shutdown cancels any pending HTTP request before waiting.
func (s *OpenAIGatewayService) StartOpenAICodexTicketHarvester() {
	if s == nil || s.cfg == nil || s.accountRepo == nil || s.httpUpstream == nil {
		return
	}
	s.codexTicketLifecycleMu.Lock()
	defer s.codexTicketLifecycleMu.Unlock()
	if s.codexTicketStopped || s.codexTicketDone != nil {
		return
	}
	ctx, cancel := context.WithCancel(context.Background())
	s.codexTicketCancel, s.codexTicketDone = cancel, make(chan struct{})
	done := s.codexTicketDone
	go func() {
		defer close(done)
		timer := time.NewTimer(time.Second)
		defer timer.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-timer.C:
				s.codexTicketNextHarvest.Store(0)
				s.refreshCodexTickets(ctx)
				cfg := s.codexTicketConfig(ctx)
				s.codexTicketNextHarvest.Store(time.Now().Add(time.Duration(cfg.HarvestProbeIntervalSeconds) * time.Second).UnixNano())
				timer.Reset(time.Duration(cfg.HarvestProbeIntervalSeconds) * time.Second)
			}
		}
	}()
}
func (s *OpenAIGatewayService) StopOpenAICodexTicketHarvester() {
	if s == nil {
		return
	}
	s.codexTicketLifecycleMu.Lock()
	s.codexTicketStopped = true
	cancel, done := s.codexTicketCancel, s.codexTicketDone
	s.codexTicketLifecycleMu.Unlock()
	if cancel != nil {
		cancel()
	}
	if done != nil {
		<-done
	}
}

// Each account is probed sequentially so a 401/429 pauses all of its models.
// The loop uses bounded parallelism across accounts and performs no work on business requests.
func (s *OpenAIGatewayService) refreshCodexTickets(ctx context.Context) {
	cfg := s.codexTicketConfig(ctx)
	if !cfg.Enabled || s.accountRepo == nil || s.httpUpstream == nil || cfg.HarvestProxyURL == "" || ValidateOpenAICodexTicketHarvestProxyURL(cfg.HarvestProxyURL) != nil || ctx.Err() != nil {
		return
	}
	listCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
	accounts, err := s.accountRepo.ListByPlatform(listCtx, PlatformOpenAI)
	cancel()
	if err != nil {
		slog.Warn("codex ticket account scan failed")
		return
	}
	live := make(map[int64]bool, len(accounts))
	for i := range accounts {
		live[accounts[i].ID] = true
	}
	s.codexTickets.Range(func(key, value any) bool {
		ticket, ok := value.(*codexTurnTicket)
		id, _, _ := strings.Cut(key.(string), "\x00")
		accountID, _ := strconv.ParseInt(id, 10, 64)
		if !ok || !live[accountID] || time.Now().After(ticket.expires) {
			s.codexTickets.Delete(key)
		}
		return true
	})
	s.codexTicketBackoffs.Range(func(key, value any) bool {
		if !live[key.(int64)] {
			s.codexTicketBackoffs.Delete(key)
		}
		return true
	})
	s.codexTicketMissBackoffs.Range(func(key, value any) bool {
		id, model, _ := strings.Cut(key.(string), "\x00")
		accountID, _ := strconv.ParseInt(id, 10, 64)
		configured := false
		for _, m := range cfg.Models {
			if m == model {
				configured = true
			}
		}
		if !live[accountID] || !configured {
			s.codexTicketMissBackoffs.Delete(key)
		}
		return true
	})
	// A fixed worker pool also bounds concurrent token refresh and proxy connections.
	jobs := make(chan *Account)
	done := make(chan struct{}, 4)
	for i := 0; i < 4; i++ {
		go func() {
			defer func() { done <- struct{}{} }()
			for account := range jobs {
				if !codexTicketAccount(account) || !account.IsSchedulable() {
					continue
				}
				if paused, _ := shouldAutoPauseOpenAIAccountByQuota(ctx, account); paused {
					continue
				}
				for _, model := range cfg.Models {
					if ctx.Err() != nil {
						break
					}
					if t := s.cachedCodexTicket(account, model); t.valid(account, time.Now().Add(time.Duration(cfg.RefreshBeforeSeconds)*time.Second)) {
						continue
					}
					s.probeCodexTicket(ctx, account, model, cfg)
				}
			}
		}()
	}
send:
	for i := range accounts {
		select {
		case jobs <- &accounts[i]:
		case <-ctx.Done():
			break send
		}
	}
	close(jobs)
	for i := 0; i < 4; i++ {
		<-done
	}
}

func (s *OpenAIGatewayService) probeCodexTicket(ctx context.Context, account *Account, model string, cfg config.OpenAICodexTicketConfig) {
	if !codexTicketGated(cfg, account, model) || ctx.Err() != nil || account.IsRateLimited() || cfg.HarvestProxyURL == "" || ValidateOpenAICodexTicketHarvestProxyURL(cfg.HarvestProxyURL) != nil {
		return
	}
	key := codexTicketKey(account.ID, model)
	if _, loaded := s.codexTicketFlight.LoadOrStore(key, struct{}{}); loaded {
		return
	}
	defer s.codexTicketFlight.Delete(key)
	attemptCtx, cancel := context.WithTimeout(ctx, time.Duration(cfg.HarvestAttemptTimeoutSeconds)*time.Second)
	defer cancel()
	credential := codexTicketCredential(account)
	if raw, ok := s.codexTicketBackoffs.Load(account.ID); ok {
		b := raw.(codexTicketBackoff)
		if (b.unauthorized && b.credential == credential) || (!b.unauthorized && time.Now().Before(b.until)) {
			return
		}
	}
	if raw, ok := s.codexTicketMissBackoffs.Load(key); ok && time.Now().Before(raw.(codexTicketBackoff).until) {
		return
	}
	token, _, err := s.GetAccessToken(attemptCtx, account)
	if err != nil || strings.TrimSpace(token) == "" {
		s.pauseCodexTicket(account, 0, nil)
		return
	}
	payload, _ := json.Marshal(map[string]any{"model": model, "store": false, "stream": true, "instructions": "Reply with exactly: pong", "input": []any{map[string]any{"role": "user", "content": []any{map[string]any{"type": "input_text", "text": "ping"}}}}})
	attemptCtx = WithHTTPUpstreamRedirectsDisabled(WithHTTPUpstreamProfile(attemptCtx, HTTPUpstreamProfileOpenAIHarvest))
	req, err := http.NewRequestWithContext(attemptCtx, http.MethodPost, chatgptCodexURL, bytes.NewReader(payload))
	if err != nil {
		return
	}
	req.Close = true
	req.Host = "chatgpt.com"
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "text/event-stream")
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("OpenAI-Beta", "responses=experimental")
	req.Header.Set("session_id", uuid.NewString())
	if err := resolveAndSetOpenAIChatGPTAccountHeaders(attemptCtx, s.accountRepo, req.Header, account); err != nil {
		s.pauseCodexTicket(account, 0, nil)
		return
	}
	enforceCodexIdentityHeadersWithUA(req.Header, s.codexIdentityOverrideUA(account))
	// Match the reference harvester's minimum Astra client identity.
	if strings.Contains(strings.ToLower(model), "gpt-6") || strings.Contains(strings.ToLower(model), "astra") {
		if CompareVersions(req.Header.Get("version"), "0.153.4") < 0 {
			req.Header.Set("version", "0.153.4")
			req.Header.Set("user-agent", buildCodexCLIUserAgent("0.153.4"))
			req.Header.Set("originator", openai.CodexDefaultOriginator)
		}
	}
	attempt, logID := s.codexTicketTelemetry.begin(account, model)
	started := time.Now()
	egress := OpenAICodexTicketEgressResult{Error: &OpenAICodexTicketEgressError{Reason: "unknown"}}
	req = req.WithContext(WithOpenAICodexTicketEgressObserver(req.Context(), func(result OpenAICodexTicketEgressResult) {
		egress = normalizeOpenAICodexTicketEgressResult(result)
		s.codexTicketTelemetry.setEgress(account.ID, model, logID, egress)
	}))
	event := OpenAICodexTicketLogEntry{Attempt: attempt, TargetLength: codexTicketLength(account), Event: "error", Reason: "request_error"}
	defer func() {
		elapsed := time.Since(started).Milliseconds()
		event.DurationMS = &elapsed
		event.EgressIP = egress.IP
		event.EgressCountryCode = egress.CountryCode
		event.EgressError = egress.Error
		s.codexTicketTelemetry.finish(account.ID, model, event)
	}()
	resp, err := s.httpUpstream.Do(req, cfg.HarvestProxyURL, account.ID, account.Concurrency)
	if resp != nil && resp.Body != nil {
		defer resp.Body.Close()
	}
	if err != nil || resp == nil {
		event.Reason = codexTicketProbeErrorReason(err)
		if ctx.Err() == nil {
			s.deferCodexTicketModel(account, model, 0, nil)
		}
		return
	}
	event.HTTPStatus = resp.StatusCode
	state := strings.TrimSpace(resp.Header.Get(openAICodexTurnStateHeader))
	length := len(state)
	event.TicketLength = &length
	event.Event = "miss"
	switch {
	case resp.StatusCode != http.StatusOK:
		event.Reason = "http_error"
	case state == "":
		event.Reason = "missing_state"
	case !strings.HasPrefix(state, "gAAAAA"):
		event.Reason = "invalid_prefix"
	default:
		event.Reason = "length_mismatch"
	}
	ticket := &codexTurnTicket{attempts: attempt, state: state, expires: time.Now().Add(time.Duration(cfg.TTLSeconds) * time.Second), credential: codexTicketCredential(account)}
	if resp.StatusCode == http.StatusOK && ticket.valid(account, time.Now()) {
		event.Event = "acquired"
		event.Reason = "target_length_matched"
		s.codexTicketMissBackoffs.Delete(key)
		s.codexTickets.Store(key, ticket)
		s.codexTicketBackoffs.Delete(account.ID)
		slog.Info("codex ticket acquired", "account_id", account.ID, "model", model, "length", len(state))
		return
	}
	s.deferCodexTicketModel(account, model, resp.StatusCode, resp.Header)
	slog.Info("codex ticket probe deferred", "account_id", account.ID, "model", model, "status", resp.StatusCode, "length", len(state))
}

func (s *OpenAIGatewayService) pauseCodexTicket(account *Account, status int, h http.Header) {
	failures := 1
	if old, ok := s.codexTicketBackoffs.Load(account.ID); ok {
		failures = min(old.(codexTicketBackoff).failures+1, 8)
	}
	delay := min(6*time.Second*time.Duration(1<<uint(failures-1)), 10*time.Minute)
	if status == http.StatusTooManyRequests {
		if delay < time.Minute {
			delay = time.Minute
		}
	}
	if h != nil {
		if seconds, err := strconv.ParseInt(h.Get("Retry-After"), 10, 32); err == nil && seconds > 0 {
			if next := time.Duration(seconds) * time.Second; next > delay {
				delay = next
			}
		} else if deadline, err := http.ParseTime(h.Get("Retry-After")); err == nil {
			if next := time.Until(deadline); next > delay {
				delay = next
			}
		}
	}
	s.codexTicketBackoffs.Store(account.ID, codexTicketBackoff{until: time.Now().Add(delay), credential: codexTicketCredential(account), unauthorized: status == http.StatusUnauthorized, failures: failures})
	if status == http.StatusUnauthorized {
		prefix := strconv.FormatInt(account.ID, 10) + "\x00"
		s.codexTickets.Range(func(key, value any) bool {
			if strings.HasPrefix(key.(string), prefix) {
				s.codexTickets.Delete(key)
			}
			return true
		})
	}
}

// Ordinary misses back off per model; authentication and quota failures pause the account.
func (s *OpenAIGatewayService) deferCodexTicketModel(account *Account, model string, status int, h http.Header) {
	if status == http.StatusUnauthorized || status == http.StatusTooManyRequests {
		s.pauseCodexTicket(account, status, h)
		return
	}
	key := codexTicketKey(account.ID, model)
	failures := 1
	if old, ok := s.codexTicketMissBackoffs.Load(key); ok {
		failures = min(old.(codexTicketBackoff).failures+1, 8)
	}
	delay := min(6*time.Second*time.Duration(1<<uint(failures-1)), 10*time.Minute)
	if h != nil {
		if seconds, err := strconv.ParseInt(h.Get("Retry-After"), 10, 32); err == nil && seconds > 0 {
			if next := time.Duration(seconds) * time.Second; next > delay {
				delay = next
			}
		} else if deadline, err := http.ParseTime(h.Get("Retry-After")); err == nil {
			if next := time.Until(deadline); next > delay {
				delay = next
			}
		}
	}
	s.codexTicketMissBackoffs.Store(key, codexTicketBackoff{until: time.Now().Add(delay), failures: failures})
}
