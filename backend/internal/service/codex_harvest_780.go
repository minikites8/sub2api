package service

import (
	"bufio"
	"context"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"regexp"
	"strings"
	"time"
)

func codexTicketLifetime(length int) time.Duration {
	if length == 780 {
		return openAICodexCredentialTTL
	}
	return time.Hour - 30*time.Second
}

var codex780GatewayRE = regexp.MustCompile(`(?:^|[.])(?:chat\.)?gateway\.(unified-[0-9]{1,5})(?:[.]|$)`)

var codex780TargetRE = regexp.MustCompile(`^(?:chat\.gateway\.)?(unified-[0-9]{1,5})(?:\.api\.openai\.com)?$`)

var codex780GatewayAliasRE = regexp.MustCompile("^(?:unified[-_.]?)?([0-9]{1,5})$")

func normalizeCodex780Gateway(target string) string {
	target = strings.ToLower(strings.TrimSpace(target))
	if target == "*" || target == "any" {
		return "any"
	}
	if match := codex780GatewayAliasRE.FindStringSubmatch(target); len(match) == 2 {
		return "unified-" + match[1]
	}
	if match := codex780TargetRE.FindStringSubmatch(target); len(match) == 2 {
		return match[1]
	}
	return target
}

func codex780GatewayAllowed(actual, target string) bool {
	return actual != "" && (target == "any" || actual == target)
}

func codex780CookieGateway(cookies []string) string {
	for _, cookie := range cookies {
		name, value, _ := strings.Cut(cookie, "=")
		if name != "__oailb" {
			continue
		}
		parts := strings.Split(value, ".")
		if len(parts) != 3 {
			return ""
		}
		raw, err := base64.RawURLEncoding.DecodeString(parts[1])
		if err != nil {
			return ""
		}
		match := codex780GatewayRE.FindSubmatch(raw)
		if len(match) == 2 {
			return string(match[1])
		}
	}
	return ""
}

// Only the two LB cookies are retained. JWT claims are routing hints, not verified identity.
func codex780Route(cookies []string, target string, now time.Time) ([]string, time.Time, error) {
	target = normalizeCodex780Gateway(target)
	bad := mintRouteError("route_cookie_invalid")
	pair := map[string]string{}
	for _, cookie := range cookies {
		name, value, ok := strings.Cut(cookie, "=")
		if name != "__cflb" && name != "__oailb" {
			continue
		}
		if !ok || value == "" || len(value) > 4096 || strings.ContainsAny(value, ";,\r\n\t ") || pair[name] != "" {
			return nil, time.Time{}, bad
		}
		for _, c := range value {
			if c < 33 || c > 126 {
				return nil, time.Time{}, bad
			}
		}
		pair[name] = value
	}
	if target == "" {
		return nil, time.Time{}, mintRouteError("route_target_missing")
	}
	if pair["__cflb"] == "" && pair["__oailb"] == "" {
		return nil, time.Time{}, mintRouteError("route_pair_missing")
	}
	if pair["__cflb"] == "" {
		return nil, time.Time{}, mintRouteError("route_cflb_missing")
	}
	if pair["__oailb"] == "" {
		return nil, time.Time{}, mintRouteError("route_oailb_missing")
	}
	parts := strings.Split(pair["__oailb"], ".")
	if len(parts) != 3 {
		return nil, time.Time{}, bad
	}
	raw, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return nil, time.Time{}, bad
	}
	var claims struct {
		Exp int64 `json:"exp"`
	}
	if json.Unmarshal(raw, &claims) != nil || claims.Exp <= 0 || claims.Exp >= 4102444800 {
		return nil, time.Time{}, mintRouteError("route_expiry_invalid")
	}
	if claims.Exp <= now.Unix() {
		return nil, time.Time{}, mintRouteError("route_pair_expired")
	}
	match := codex780GatewayRE.FindSubmatch(raw)
	if len(match) != 2 {
		return nil, time.Time{}, mintRouteError("route_gateway_unknown")
	}
	if !codex780GatewayAllowed(string(match[1]), target) {
		return nil, time.Time{}, mintRouteError("route_gateway_mismatch: got=" + string(match[1]) + " want=" + target)
	}
	return []string{"__cflb=" + pair["__cflb"], "__oailb=" + pair["__oailb"]}, time.Unix(claims.Exp, 0), nil
}

// A complete created event suffices for the model declaration. Stop early, bound reads,
// and never expose response bodies in diagnostics. This is not a capability test.
func readCodex780Created(body io.Reader, model string) error {
	scanner := bufio.NewScanner(io.LimitReader(body, 16*1024))
	scanner.Buffer(make([]byte, 1024), 16*1024)
	scanner.Split(splitCodex780SSELine)
	var data []string
	eventName := ""
	for scanner.Scan() {
		line := strings.TrimPrefix(scanner.Text(), "\ufeff")
		if line == "" {
			raw := []byte(strings.Join(data, "\n"))
			if err := codex780EventError(raw, eventName); err != nil {
				return err
			}
			var event struct {
				Type     string `json:"type"`
				Response struct {
					ID    string `json:"id"`
					Model string `json:"model"`
				} `json:"response"`
			}
			if json.Unmarshal(raw, &event) == nil {
				if event.Type == "response.created" {
					if (eventName != "" && eventName != event.Type) || strings.TrimSpace(event.Response.ID) == "" || event.Response.Model != model {
						return &codexMintError{kind: "model_mismatch", detail: "mint model declaration mismatch"}
					}
					return nil
				}
			}
			data = nil
			eventName = ""
			continue
		}
		field, value, _ := strings.Cut(line, ":")
		value = strings.TrimPrefix(value, " ")
		if field == "data" {
			data = append(data, value)
		}
		if field == "event" {
			eventName = value
		}
	}
	if err := scanner.Err(); err != nil {
		return mintTransportError(err)
	}
	return &codexMintError{kind: "response_incomplete_or_error", detail: "mint created event missing or incomplete"}
}

func (s *OpenAIGatewayService) requestCodex780Probe(ctx context.Context, account *Account, token, model, proxy string, reserve func() bool, session string) (out codexHarvestProbeResult) {
	controls, _ := s.harvestControls(ctx)
	if edge, ok := ctx.Value(codexMintEdgeContextKey{}).(string); ok {
		controls.EdgeIP = edge
	}
	return s.requestCodexCloudMintProbe(ctx, account, token, model, proxy, reserve, session, controls)
}

func codex780ResponseRoute(resp *http.Response, seed []string, target string, now time.Time) ([]string, error) {
	var incoming []string
	for _, cookie := range resp.Cookies() {
		if cookie.Name != "__cflb" && cookie.Name != "__oailb" {
			continue
		}
		if cookie.Value == "" || cookie.MaxAge < 0 || (cookie.MaxAge == 0 && !cookie.Expires.IsZero() && !now.Before(cookie.Expires)) {
			return nil, mintRouteError("route_cookie_deleted")
		}
		incoming = append(incoming, cookie.Name+"="+cookie.Value)
	}
	if len(incoming) == 0 {
		incoming = seed
	}
	// A changed pair must be complete; never combine old and new halves.
	clean, _, err := codex780Route(incoming, target, now)
	return clean, err
}

func splitCodex780SSELine(data []byte, atEOF bool) (int, []byte, error) {
	for i, b := range data {
		if b != '\r' && b != '\n' {
			continue
		}
		n := 1
		if b == '\r' {
			if i+1 == len(data) && !atEOF {
				return 0, nil, nil
			}
			if i+1 < len(data) && data[i+1] == '\n' {
				n = 2
			}
		}
		return i + n, data[:i], nil
	}
	if atEOF && len(data) > 0 {
		return len(data), data, nil
	}
	return 0, nil, nil
}
