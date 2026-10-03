package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
)

const (
	defaultCodexCloudMintKeyEnv = "SUB2API_CODEX_CLOUD_MINT_KEY"
	cloudMintResponseLimit      = 256 << 10
)

type codexCloudMintTicket struct {
	TurnState   string `json:"turn_state"`
	TicketLen   int    `json:"ticket_len"`
	ServedModel string `json:"served_model"`
	IssuedAt    string `json:"issued_at"`
	ExpiresAt   string `json:"expires_at"`
}

type codexCloudMintResponse struct {
	Transport    string                          `json:"transport"`
	Gateway      string                          `json:"gateway"`
	Cookies      map[string]string               `json:"cookies"`
	CookieHeader string                          `json:"cookie_header"`
	EdgeIP       string                          `json:"edge_ip"`
	Tickets      map[string]codexCloudMintTicket `json:"tickets"`
	Model        string                          `json:"model"`
	TurnState    string                          `json:"turn_state"`
	TicketLen    int                             `json:"ticket_len"`
	ServedModel  string                          `json:"served_model"`
	Errors       map[string]json.RawMessage      `json:"errors"`
	Code         string                          `json:"code"`
	Error        struct {
		Code string `json:"code"`
	} `json:"error"`
}

func looksLikeCodexRelayKey(value string) bool {
	value = strings.TrimSpace(value)
	if len(value) < 32 {
		return false
	}
	for _, r := range value {
		if (r < 'a' || r > 'z') && (r < 'A' || r > 'Z') && (r < '0' || r > '9') && r != '-' && r != '_' {
			return false
		}
	}
	return true
}

func codexCloudMintKey(cfg config.OpenAICodexCloudMintConfig) string {
	if key := strings.TrimSpace(cfg.Key); key != "" {
		return key
	}
	keyRef := strings.TrimSpace(cfg.KeyEnv)
	if looksLikeCodexRelayKey(keyRef) {
		return keyRef
	}
	return strings.TrimSpace(os.Getenv(keyRef))
}

func (s *OpenAIGatewayService) codexCloudMintConfig() config.OpenAICodexCloudMintConfig {
	cfg := config.OpenAICodexCloudMintConfig{}
	if s != nil {
		cfg = s.openAICodexTicketConfig().CloudMint
	}
	if strings.TrimSpace(cfg.KeyEnv) == "" {
		cfg.KeyEnv = defaultCodexCloudMintKeyEnv
	}
	if strings.TrimSpace(cfg.Transport) == "" {
		cfg.Transport = "sse"
	}
	if cfg.TimeoutSeconds <= 0 {
		cfg.TimeoutSeconds = 25
	}
	return cfg
}

func normalizeCodexCloudMintURL(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if err := config.ValidateAbsoluteHTTPURL(raw); err != nil {
		return "", fmt.Errorf("cloud mint url: %w", err)
	}
	u, err := url.Parse(raw)
	if err != nil {
		return "", fmt.Errorf("cloud mint url: %w", err)
	}
	if u.User != nil || u.RawQuery != "" || u.Fragment != "" || u.Opaque != "" {
		return "", errors.New("cloud mint url must omit credentials, query, fragment, and opaque data")
	}
	if strings.EqualFold(u.Scheme, "http") {
		host := strings.TrimSpace(u.Hostname())
		ip := net.ParseIP(host)
		local := host == "localhost" || (ip != nil && ip.IsLoopback())
		if !local {
			return "", errors.New("cloud mint http url is limited to localhost")
		}
	}
	return strings.TrimRight(raw, "/"), nil
}

func normalizeCodexCloudMintTransport(raw string) (string, error) {
	transport := strings.ToLower(strings.TrimSpace(raw))
	if transport == "sse" || transport == "websocket" {
		return transport, nil
	}
	return "", errors.New("cloud mint transport must be sse or websocket")
}

func cloudMintCookiePairs(resp codexCloudMintResponse) []string {
	pairs := make([]string, 0, 2)
	for _, name := range []string{"__cflb", "__oailb"} {
		if value := strings.TrimSpace(resp.Cookies[name]); value != "" {
			pairs = append(pairs, name+"="+value)
		}
	}
	if len(pairs) == 2 {
		return pairs
	}
	for _, raw := range strings.Split(resp.CookieHeader, ";") {
		name, value, ok := strings.Cut(strings.TrimSpace(raw), "=")
		if !ok || (name != "__cflb" && name != "__oailb") || strings.TrimSpace(value) == "" {
			continue
		}
		found := false
		for _, pair := range pairs {
			if strings.HasPrefix(pair, name+"=") {
				found = true
				break
			}
		}
		if !found {
			pairs = append(pairs, name+"="+strings.TrimSpace(value))
		}
	}
	return pairs
}

func parseCodexCloudMintTime(raw string) (time.Time, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return time.Time{}, nil
	}
	value, err := time.Parse(time.RFC3339Nano, raw)
	if err != nil {
		return time.Time{}, errors.New("cloud mint timestamp is invalid")
	}
	return value, nil
}

func cloudMintErrorFromHTTP(status int, code string) error {
	code = strings.TrimSpace(code)
	switch code {
	case "mint_bad_params", "mint_bad_cookie", "bad_target", "relay_misconfigured":
		return &codexMintError{kind: code, detail: "cloud mint relay rejected the request", terminal: true}
	case "mint_exhausted", "mint_rejected":
		return &codexMintError{kind: code, detail: "cloud mint relay rejected the request", terminal: status == http.StatusUnauthorized || status == http.StatusForbidden}
	case "bad_relay_key":
		return &codexMintError{kind: "cloud_mint_auth_error", detail: "cloud mint relay authentication failed", terminal: true}
	default:
		return &codexMintError{kind: "cloud_mint_error", detail: "cloud mint relay returned an error", terminal: status == http.StatusBadRequest || status == http.StatusNotFound || status == http.StatusUnprocessableEntity}
	}
}

func cloudMintResponseCode(resp codexCloudMintResponse, header string) string {
	if code := strings.TrimSpace(header); code != "" {
		return code
	}
	if code := strings.TrimSpace(resp.Error.Code); code != "" {
		return code
	}
	return strings.TrimSpace(resp.Code)
}

func (s *OpenAIGatewayService) requestCodexCloudMintProbe(ctx context.Context, account *Account, token, model, proxy string, reserve func() bool, session string, controls CodexHarvestControls) (out codexHarvestProbeResult) {
	if s == nil || account == nil {
		out.Err = errors.New("cloud mint account unavailable")
		return
	}
	cfg := s.codexCloudMintConfig()
	endpoint, err := normalizeCodexCloudMintURL(cfg.URL)
	if err != nil {
		out.Err = err
		return
	}
	key := codexCloudMintKey(cfg)
	if key == "" {
		out.Err = errors.New("cloud mint relay key is unavailable")
		return
	}
	transport, err := normalizeCodexCloudMintTransport(cfg.Transport)
	if err != nil {
		out.Err = err
		return
	}
	target := effectiveCodex780GatewayForAccount(s.openAICodexTicketConfig(), controls, account)
	out.Transport = transport
	out.Gateway = target
	// Relay selects the upstream edge. The legacy direct edge override is not
	// part of the relay contract.
	out.EdgeIP = ""

	accountHeader := ""
	if account != nil {
		accountHeader = account.GetCredential("chatgpt_account_id")
	}
	keyHash := codex780RouteKey(account, token, accountHeader, target, transport)
	seed, cached := s.codex780Routes.get(keyHash, time.Now())
	if !cached {
		if ticket := s.lookupOpenAICodexTicket(account, model); ticket != nil && !ticket.Revoked {
			seed, _, _ = codex780Route(ticket.HarvestCookies, target, time.Now())
		}
	}
	defer func() { s.codex780Routes.update(keyHash, target, seed, out, time.Now()) }()

	if ctx.Err() != nil || (reserve != nil && !reserve()) {
		out.Err = errors.New("cloud mint request no longer admitted")
		return
	}
	cloudCtx, cancel := context.WithTimeout(ctx, time.Duration(cfg.TimeoutSeconds)*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(cloudCtx, http.MethodPost, endpoint, nil)
	if err != nil {
		out.Err = errors.New("cannot construct cloud mint request")
		return
	}
	req = req.WithContext(WithHTTPUpstreamRedirectsDisabled(WithHTTPUpstreamProfile(req.Context(), HTTPUpstreamProfileOpenAIHarvest)))
	req.Close = true
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("X-Relay-Key", key)
	if target == "any" {
		req.Header.Set("X-Relay-Mint", "1")
	} else {
		req.Header.Set("X-Relay-Mint", target)
	}
	req.Header.Set("X-Mint-Model", model)
	req.Header.Set("X-Mint-Transport", transport)
	req.Header.Set("X-Mint-Gateway", target)
	req.Header.Set("X-Mint-Len", strconv.Itoa(780))
	attempts := s.openAICodexTicketConfig().MaxProbesPerRound
	if attempts > 0 {
		req.Header.Set("X-Mint-Attempts", strconv.Itoa(attempts))
	}
	ttl := s.openAICodexTicketConfig().TTLSeconds
	if ttl <= 0 {
		ttl = 240
	}
	req.Header.Set("X-Mint-TTL", strconv.Itoa(ttl))
	if out.EdgeIP != "" {
		req.Header.Set("X-Edge-IP", out.EdgeIP)
	}
	if session != "" {
		req.Header.Set("Session-Id", session)
	}
	if len(seed) > 0 {
		req.Header.Set("Cookie", strings.Join(seed, "; "))
	}
	if err := resolveAndSetOpenAIChatGPTAccountHeaders(ctx, s.accountRepo, req.Header, account); err != nil {
		out.Err = errors.New("account identity unavailable")
		return
	}
	out.Sent = true
	cloudProxy := strings.TrimSpace(cfg.ProxyURL)
	if s.httpUpstream == nil {
		out.Err = errors.New("cloud mint upstream unavailable")
		return
	}
	resp, err := s.httpUpstream.Do(req, cloudProxy, account.ID, account.Concurrency)
	if err != nil {
		if resp != nil && resp.Body != nil {
			_ = resp.Body.Close()
		}
		out.Err = err
		return
	}
	if resp == nil {
		out.Err = errors.New("cloud mint response missing")
		return
	}
	out.Status = resp.StatusCode
	out.RetryAfter = codexHarvestRetryAfter(resp.Header.Get("Retry-After"), time.Now())
	if resp.Body == nil {
		out.Err = errors.New("cloud mint response body missing")
		return
	}
	defer func() { _ = resp.Body.Close() }()
	body, err := io.ReadAll(io.LimitReader(resp.Body, cloudMintResponseLimit+1))
	if err != nil || len(body) > cloudMintResponseLimit {
		out.Err = errors.New("cloud mint response is incomplete")
		return
	}
	var envelope codexCloudMintResponse
	if err := json.Unmarshal(body, &envelope); err != nil {
		out.Err = errors.New("cloud mint response is invalid")
		return
	}
	if resp.StatusCode != http.StatusOK {
		out.Err = cloudMintErrorFromHTTP(resp.StatusCode, cloudMintResponseCode(envelope, resp.Header.Get("X-Relay-Error")))
		return
	}

	ticket, ok := envelope.Tickets[model]
	if !ok && envelope.Model == model && strings.TrimSpace(envelope.TurnState) != "" {
		ticket = codexCloudMintTicket{TurnState: envelope.TurnState, TicketLen: envelope.TicketLen, ServedModel: envelope.ServedModel}
		ok = true
	}
	if !ok {
		out.Err = &codexMintError{kind: "ticket_missing", detail: "cloud mint response did not contain the requested model"}
		return
	}
	if strings.TrimSpace(ticket.ServedModel) != model {
		out.Err = &codexMintError{kind: "model_mismatch", detail: "cloud mint served a different model"}
		return
	}
	out.State = strings.TrimSpace(ticket.TurnState)
	if out.State == "" {
		out.Err = &codexMintError{kind: "ticket_missing", detail: "cloud mint response did not contain turn-state"}
		return
	}
	if ticket.TicketLen != len(out.State) {
		out.Err = &codexMintError{kind: "ticket_length_mismatch", detail: "cloud mint ticket length mismatch"}
		return
	}
	out.Cookies = cloudMintCookiePairs(envelope)
	out.Gateway = normalizeCodex780Gateway(envelope.Gateway)
	if out.Gateway == "" || out.Gateway == "any" {
		out.Gateway = codex780CookieGateway(out.Cookies)
	}
	if out.Gateway == "" {
		out.Gateway = target
	}
	if edge := strings.TrimSpace(envelope.EdgeIP); edge != "" {
		out.EdgeIP = edge
	}
	if ticketExpiry, err := parseCodexCloudMintTime(ticket.ExpiresAt); err != nil {
		out.Err = err
		return
	} else {
		out.ExpiresAt = ticketExpiry
	}
	if ticketIssued, err := parseCodexCloudMintTime(ticket.IssuedAt); err != nil {
		out.Err = err
		return
	} else if !ticketIssued.IsZero() {
		shape, shapeErr := parseOpenAICodexTicketShape(out.State)
		if shapeErr == nil && shape.IssuedAt.Unix() != ticketIssued.Unix() {
			out.Err = &codexMintError{kind: "ticket_timestamp_mismatch", detail: "cloud mint ticket timestamp mismatch"}
			return
		}
	}
	return
}
