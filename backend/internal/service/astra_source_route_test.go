package service

import (
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
)

func TestAstraSourceCachedRouteFollowsBoundTicket(t *testing.T) {
	now := time.Now().Truncate(time.Second)
	expires := now.Add(36 * time.Minute)
	account := ticketTestAccount(299)
	account.Extra = map[string]any{OpenAICodexTicketGatewayExtraKey: "unified-79"}
	ticket := &openAICodexTicket{
		AccountID: account.ID, Model: "gpt-6-astra", State: mint780State(now), Length: 780,
		CapturedAt: now, IssuedAt: now, ExpiresAt: now.Add(240 * time.Second),
		Transport: "sse", Gateway: "unified-79", HarvestCookies: mint780Pair(expires, "unified-79"),
	}
	svc := ticketTestService(t, config.OpenAICodexTicketConfig{Enabled: true, TargetLength: 780, Models: []string{"gpt-6-astra"}}, nil)
	require.NoError(t, svc.storeOpenAICodexTicket(t.Context(), account, ticket))
	req, err := http.NewRequestWithContext(WithAstraSourceAcquisition(t.Context()), http.MethodPost,
		"https://chatgpt.com/backend-api/codex/responses", strings.NewReader(`{"model":"gpt-6-astra"}`))
	require.NoError(t, err)
	require.NoError(t, svc.applyOpenAICodexTicket(req.Context(), account, ticket.Model, req.Header))
	req.Header.Set("Authorization", "Bearer own-source-token")
	prepared := svc.stickBoundCodexTicketRequest(req, account)
	value, expiry := AstraSourceCachedRouteFromRequest(prepared, account.ID)
	require.Equal(t, strings.TrimPrefix(ticket.HarvestCookies[1], "__oailb="), value)
	require.Equal(t, expires, expiry)
	require.Equal(t, "Bearer own-source-token", prepared.Header.Get("Authorization"))
	require.Equal(t, strings.Join(ticket.HarvestCookies, "; "), prepared.Header.Get("Cookie"))
	require.Empty(t, prepared.Header.Get("X-Astra-Source-Route"))
}

func TestAstraSourcePreparationCarriesCachedRouteThroughForwarding(t *testing.T) {
	account, ticket := pelicanReadyTicketAccount()
	gateway := ticketTestService(t, config.OpenAICodexTicketConfig{
		Enabled: true, TargetLength: 780, FailClosed: true, Models: []string{ticket.Model},
	}, nil)
	calls := 0
	gateway.httpUpstream = &harvestProxyUpstream{do: func(req *http.Request, _ string) (*http.Response, error) {
		calls++
		value, expires := AstraSourceCachedRouteFromRequest(req, account.ID)
		require.Equal(t, strings.TrimPrefix(ticket.HarvestCookies[1], "__oailb="), value)
		require.Equal(t, codexTicketCookiesExpiry(ticket), expires)
		require.Equal(t, "Bearer tok", req.Header.Get("Authorization"))
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Type": {"text/event-stream"}},
			Body: io.NopCloser(strings.NewReader("data: " +
				`{"type":"response.output_text.delta","delta":"OK"}` + "\n\ndata: " +
				`{"type":"response.completed","response":{"id":"source-response","status":"completed","model":"gpt-6-astra","output":[{"type":"message","content":[{"type":"output_text","text":"OK"}]}]}}` + "\n\n")),
		}, nil
	}}
	svc := ProvideAccountTestService(&pelicanTicketAccountRepo{account: account}, nil, nil, nil, nil, nil,
		gateway.httpUpstream, gateway.cfg, nil, gateway, nil, nil)
	require.NoError(t, svc.prepareAstraGatewaySource(t.Context(), account.ID))
	require.Equal(t, 1, calls)
}

func TestAstraSourceCachedRouteRejectsUntrustedOrExpiredSeed(t *testing.T) {
	for _, tc := range []struct {
		name      string
		source    bool
		seed      bool
		accountID int64
		cookie    string
		expires   time.Time
	}{
		{"ordinary_request", false, true, 299, "__oailb=cached", time.Now().Add(time.Minute)},
		{"raw_cookie_only", true, false, 299, "__oailb=cached", time.Now().Add(time.Minute)},
		{"other_account", true, true, 300, "__oailb=cached", time.Now().Add(time.Minute)},
		{"expired", true, true, 299, "__oailb=cached", time.Now().Add(-time.Second)},
		{"cookie_changed", true, true, 299, "__oailb=other", time.Now().Add(time.Minute)},
		{"cookie_removed", true, true, 299, "__cflb=other", time.Now().Add(time.Minute)},
		{"duplicate", true, true, 299, "__oailb=cached; __oailb=cached", time.Now().Add(time.Minute)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx := t.Context()
			if tc.source {
				ctx = WithAstraSourceAcquisition(ctx)
			}
			if tc.seed {
				ctx = WithAstraSourceCachedRoute(ctx, 299, "cached", tc.expires)
			}
			req, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://chatgpt.com/backend-api/codex/responses", nil)
			require.NoError(t, err)
			req.Header.Set("Cookie", tc.cookie)
			value, expiry := AstraSourceCachedRouteFromRequest(req, tc.accountID)
			require.Empty(t, value)
			require.Zero(t, expiry)
		})
	}
}
