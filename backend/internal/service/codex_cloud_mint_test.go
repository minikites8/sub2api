package service

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
)

func cloudMintResponseBody(t *testing.T, model, served string, state string, issued, expires time.Time, pair []string) string {
	t.Helper()
	cookies := map[string]string{}
	for _, raw := range pair {
		name, value, ok := strings.Cut(raw, "=")
		require.True(t, ok)
		cookies[name] = value
	}
	body, err := json.Marshal(map[string]any{
		"transport": "sse",
		"gateway":   "unified-88",
		"cookies":   cookies,
		"tickets": map[string]any{
			model: map[string]any{
				"turn_state":   state,
				"ticket_len":   len(state),
				"served_model": served,
				"issued_at":    issued.UTC().Format(time.RFC3339Nano),
				"expires_at":   expires.UTC().Format(time.RFC3339Nano),
			},
		},
	})
	require.NoError(t, err)
	return string(body)
}

func TestCodexCloudMintProbeUsesRelayContract(t *testing.T) {
	account := ticketTestAccount(1)
	issued := time.Now().Truncate(time.Second)
	expires := issued.Add(4 * time.Minute)
	pair := mint780Pair(expires, "unified-88")
	state := mint780State(issued)
	cfg := config.OpenAICodexTicketConfig{
		TargetLength:                 780,
		TTLSeconds:                   240,
		HarvestAttemptTimeoutSeconds: 25,
		CloudMint: config.OpenAICodexCloudMintConfig{
			Enabled:        true,
			URL:            "https://relay.example/",
			KeyEnv:         "TEST_CODEX_CLOUD_MINT_KEY",
			Transport:      "sse",
			Gateway:        "unified-88",
			TimeoutSeconds: 25,
		},
	}
	t.Setenv("TEST_CODEX_CLOUD_MINT_KEY", "relay-secret")
	s := ticketTestService(t, cfg, nil)
	s.httpUpstream = &harvestProxyUpstream{do: func(req *http.Request, proxy string) (*http.Response, error) {
		require.Equal(t, "", proxy)
		require.Equal(t, "relay-secret", req.Header.Get("X-Relay-Key"))
		require.Equal(t, "gpt-6-astra", req.Header.Get("X-Mint-Model"))
		require.Equal(t, "780", req.Header.Get("X-Mint-Len"))
		require.Equal(t, "unified-88", req.Header.Get("X-Relay-Mint"))
		require.Equal(t, "unified-88", req.Header.Get("X-Mint-Gateway"))
		require.Equal(t, "6", req.Header.Get("X-Mint-Attempts"))
		require.Equal(t, HTTPUpstreamProfileOpenAIHarvest, HTTPUpstreamProfileFromContext(req.Context()))
		require.True(t, HTTPUpstreamRedirectsDisabled(req.Context()))
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Type": []string{"application/json"}},
			Body:       io.NopCloser(strings.NewReader(cloudMintResponseBody(t, "gpt-6-astra", "gpt-6-astra", state, issued, expires, pair))),
		}, nil
	}}

	out := s.requestCodex780Probe(context.Background(), account, "token", "gpt-6-astra", "", func() bool { return true }, "session")
	require.NoError(t, out.Err)
	require.Equal(t, http.StatusOK, out.Status)
	require.Equal(t, state, out.State)
	require.Equal(t, pair, out.Cookies)
	require.Equal(t, "unified-88", out.Gateway)
	require.Equal(t, "sse", out.Transport)
	require.WithinDuration(t, expires, out.ExpiresAt, time.Second)

	shape, kind := classifyCodexHarvestProbe(context.Background(), account, cfg, out)
	require.Equal(t, "success", kind)
	out.Shape = shape
	ticket := codexHarvestTicket(account, "gpt-6-astra", out, cfg, 1)
	require.WithinDuration(t, expires, ticket.ExpiresAt, time.Second)
}

func TestCodexCloudMintProbeHonorsConfiguredAnyGateway(t *testing.T) {
	account := ticketTestAccount(1)
	issued := time.Now().Truncate(time.Second)
	expires := issued.Add(4 * time.Minute)
	pair := mint780Pair(expires, "unified-84")
	state := mint780State(issued)
	cfg := config.OpenAICodexTicketConfig{
		TargetLength: 780,
		TTLSeconds:   240,
		CloudMint: config.OpenAICodexCloudMintConfig{
			Enabled:        true,
			URL:            "https://relay.example/",
			KeyEnv:         "TEST_CODEX_CLOUD_MINT_KEY",
			Transport:      "sse",
			Gateway:        "any",
			TimeoutSeconds: 25,
		},
	}
	t.Setenv("TEST_CODEX_CLOUD_MINT_KEY", "relay-secret")
	s := ticketTestService(t, cfg, nil)
	s.codexHarvest = &CodexHarvestService{
		current:     CodexHarvestControls{Transport: "sse", TargetGateway: "unified-88"},
		loadedUntil: time.Now().Add(time.Hour),
	}
	s.httpUpstream = &harvestProxyUpstream{do: func(req *http.Request, proxy string) (*http.Response, error) {
		require.Equal(t, "1", req.Header.Get("X-Relay-Mint"))
		require.Equal(t, "any", req.Header.Get("X-Mint-Gateway"))
		body, err := json.Marshal(map[string]any{
			"transport": "sse",
			"gateway":   "unified-84",
			"cookies":   map[string]string{"__cflb": strings.TrimPrefix(pair[0], "__cflb="), "__oailb": strings.TrimPrefix(pair[1], "__oailb=")},
			"tickets": map[string]any{"gpt-5.6-sol": map[string]any{
				"turn_state": state, "ticket_len": len(state), "served_model": "gpt-5.6-sol",
				"issued_at": issued.UTC().Format(time.RFC3339Nano), "expires_at": expires.UTC().Format(time.RFC3339Nano),
			}},
		})
		require.NoError(t, err)
		return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(string(body)))}, nil
	}}

	out := s.requestCodex780Probe(context.Background(), account, "token", "gpt-5.6-sol", "", func() bool { return true }, "session")
	require.NoError(t, out.Err)
	require.Equal(t, "unified-84", out.Gateway)
}
func TestCodexCloudMintProbeRejectsServedModelMismatch(t *testing.T) {
	account := ticketTestAccount(1)
	now := time.Now().Truncate(time.Second)
	pair := mint780Pair(now.Add(4*time.Minute), "unified-88")
	state := mint780State(now)
	cfg := config.OpenAICodexTicketConfig{
		TargetLength: 780,
		CloudMint: config.OpenAICodexCloudMintConfig{
			Enabled: true, URL: "https://relay.example/", KeyEnv: "TEST_CODEX_CLOUD_MINT_KEY", Gateway: "unified-88",
		},
	}
	t.Setenv("TEST_CODEX_CLOUD_MINT_KEY", "relay-secret")
	s := ticketTestService(t, cfg, nil)
	s.httpUpstream = &harvestProxyUpstream{do: func(req *http.Request, proxy string) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(strings.NewReader(cloudMintResponseBody(t, "gpt-6-astra", "gpt-5.6-luna", state, now, now.Add(4*time.Minute), pair))),
		}, nil
	}}

	out := s.requestCodex780Probe(context.Background(), account, "token", "gpt-6-astra", "", func() bool { return true }, "session")
	var mintErr *codexMintError
	require.ErrorAs(t, out.Err, &mintErr)
	require.Equal(t, "model_mismatch", mintErr.kind)
}

func cloudMintResponseBodyWithTicketLen(t *testing.T, model, served, state string, issued, expires time.Time, pair []string, ticketLen int) string {
	t.Helper()
	cookies := map[string]string{}
	for _, raw := range pair {
		name, value, ok := strings.Cut(raw, "=")
		require.True(t, ok)
		cookies[name] = value
	}
	body, err := json.Marshal(map[string]any{
		"transport": "sse",
		"gateway":   "unified-88",
		"cookies":   cookies,
		"tickets": map[string]any{
			model: map[string]any{
				"turn_state":   state,
				"ticket_len":   ticketLen,
				"served_model": served,
				"issued_at":    issued.UTC().Format(time.RFC3339Nano),
				"expires_at":   expires.UTC().Format(time.RFC3339Nano),
			},
		},
	})
	require.NoError(t, err)
	return string(body)
}

func TestCodexCloudMintProbeRejectsIncompleteTicketMetadata(t *testing.T) {
	account := ticketTestAccount(1)
	now := time.Now().Truncate(time.Second)
	pair := mint780Pair(now.Add(4*time.Minute), "unified-88")
	state := mint780State(now)
	cfg := config.OpenAICodexTicketConfig{
		TargetLength: 780,
		CloudMint: config.OpenAICodexCloudMintConfig{
			Enabled: true, URL: "https://relay.example/", KeyEnv: "TEST_CODEX_CLOUD_MINT_KEY", Gateway: "unified-88",
		},
	}
	t.Setenv("TEST_CODEX_CLOUD_MINT_KEY", "relay-secret")
	for _, tc := range []struct {
		name   string
		served string
		length int
		kind   string
	}{
		{name: "missing served model", served: "", length: len(state), kind: "model_mismatch"},
		{name: "wrong ticket length", served: "gpt-6-astra", length: len(state) - 1, kind: "ticket_length_mismatch"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := ticketTestService(t, cfg, nil)
			s.httpUpstream = &harvestProxyUpstream{do: func(req *http.Request, proxy string) (*http.Response, error) {
				body := cloudMintResponseBodyWithTicketLen(t, "gpt-6-astra", tc.served, state, now, now.Add(4*time.Minute), pair, tc.length)
				return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(body))}, nil
			}}
			out := s.requestCodex780Probe(context.Background(), account, "token", "gpt-6-astra", "", func() bool { return true }, "session")
			var mintErr *codexMintError
			require.ErrorAs(t, out.Err, &mintErr)
			require.Equal(t, tc.kind, mintErr.kind)
		})
	}
}

func TestCloudMintRelayErrorsKeepTheirCategory(t *testing.T) {
	account := ticketTestAccount(1)
	err := cloudMintErrorFromHTTP(http.StatusForbidden, "bad_relay_key")
	_, kind := classifyCodexHarvestProbe(context.Background(), account, config.OpenAICodexTicketConfig{TargetLength: 780}, codexHarvestProbeResult{
		Sent: true, Status: http.StatusForbidden, Err: err,
	})
	require.Equal(t, "cloud_mint_auth_error", kind)
}

func TestCloudMintResponseCodeReadsRelayErrorEnvelope(t *testing.T) {
	var resp codexCloudMintResponse
	resp.Error.Code = "mint_bad_cookie"
	require.Equal(t, "mint_bad_cookie", cloudMintResponseCode(resp, ""))
	require.Equal(t, "x-relay-code", cloudMintResponseCode(resp, "x-relay-code"))
}

func TestNormalizeCodexCloudMintURL(t *testing.T) {
	for _, tc := range []struct {
		name string
		raw  string
		ok   bool
	}{
		{name: "https", raw: "https://relay.example/mint", ok: true},
		{name: "localhost http", raw: "http://127.0.0.1:9101/", ok: true},
		{name: "public http", raw: "http://relay.example/", ok: false},
		{name: "query", raw: "https://relay.example/?token=secret", ok: false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := normalizeCodexCloudMintURL(tc.raw)
			if tc.ok {
				require.NoError(t, err)
			} else {
				require.Error(t, err)
			}
		})
	}
}

func TestCodexCloudMintProbeUsesAccountGatewayOverride(t *testing.T) {
	account := ticketTestAccount(2)
	account.Extra = map[string]any{OpenAICodexTicketGatewayExtraKey: "unified-95"}
	issued := time.Now().Truncate(time.Second)
	expires := issued.Add(4 * time.Minute)
	pair := mint780Pair(expires, "unified-88")
	state := mint780State(issued)
	cfg := config.OpenAICodexTicketConfig{
		TargetLength: 780,
		CloudMint: config.OpenAICodexCloudMintConfig{
			Enabled: true, URL: "https://relay.example/", KeyEnv: "TEST_CODEX_CLOUD_MINT_KEY",
			Transport: "sse", Gateway: "unified-88", TimeoutSeconds: 25,
		},
	}
	t.Setenv("TEST_CODEX_CLOUD_MINT_KEY", "relay-secret")
	s := ticketTestService(t, cfg, nil)
	s.httpUpstream = &harvestProxyUpstream{do: func(req *http.Request, _ string) (*http.Response, error) {
		require.Equal(t, "unified-95", req.Header.Get("X-Relay-Mint"))
		require.Equal(t, "unified-95", req.Header.Get("X-Mint-Gateway"))
		return &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(strings.NewReader(cloudMintResponseBody(t, "gpt-6-astra", "gpt-6-astra", state, issued, expires, pair))),
		}, nil
	}}

	out := s.requestCodex780Probe(context.Background(), account, "token", "gpt-6-astra", "", func() bool { return true }, "session")
	require.NoError(t, out.Err)
}

func TestCodexCloudMintProbeUsesDirectConfiguredKey(t *testing.T) {
	account := ticketTestAccount(1)
	issued := time.Now().Truncate(time.Second)
	expires := issued.Add(4 * time.Minute)
	pair := mint780Pair(expires, "unified-88")
	state := mint780State(issued)
	cfg := config.OpenAICodexTicketConfig{
		TargetLength: 780,
		TTLSeconds:   240,
		CloudMint: config.OpenAICodexCloudMintConfig{
			Enabled: true, URL: "https://relay.example/", Key: "direct-relay-secret", KeyEnv: "TEST_CODEX_CLOUD_MINT_KEY",
			Transport: "sse", Gateway: "any", TimeoutSeconds: 25,
		},
	}
	t.Setenv("TEST_CODEX_CLOUD_MINT_KEY", "legacy-env-secret")
	s := ticketTestService(t, cfg, nil)
	s.httpUpstream = &harvestProxyUpstream{do: func(req *http.Request, _ string) (*http.Response, error) {
		require.Equal(t, "direct-relay-secret", req.Header.Get("X-Relay-Key"))
		return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(cloudMintResponseBody(t, "gpt-6-astra", "gpt-6-astra", state, issued, expires, pair)))}, nil
	}}
	out := s.requestCodex780Probe(context.Background(), account, "token", "gpt-6-astra", "", func() bool { return true }, "session")
	require.NoError(t, out.Err)
	require.Equal(t, "direct-relay-secret", codexCloudMintKey(cfg.CloudMint))
}
