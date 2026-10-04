package repository

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/tlsfingerprint"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func cachedAstraSourceRequest(t *testing.T, expires time.Time) *http.Request {
	t.Helper()
	req := pinRequest(t)
	req = req.WithContext(service.WithAstraSourceCachedRoute(req.Context(), 299, "cached-route", expires))
	req.Header.Set("Cookie", "__cflb=source-cf; __oailb=cached-route")
	return req
}

func TestAstraGatewayCachedSourceCookieWithoutSetCookie(t *testing.T) {
	for _, tc := range []struct {
		name, responseCookie, stream string
		want                         string
	}{
		{"unchanged_route", "", "", "cached-route"},
		{"only_cflb_renewed", "__cflb=new; Path=/; Secure; Max-Age=3600", "", "cached-route"},
		{"explicit_new_route", "__oailb=new-route; Path=/; Secure; Max-Age=60", "", "new-route"},
		{"deleted", "__oailb=; Path=/; Secure; Max-Age=0", "", ""},
		{"insecure_replacement", "__oailb=other; Path=/; Max-Age=60", "", ""},
		{"foreign_replacement", "__oailb=other; Domain=example.com; Path=/; Secure; Max-Age=60", "", ""},
		{"truncated", "", `data: {"type":"response.created","response":{"model":"gpt-6-astra"}}` + "\n\n", ""},
		{"failed", "", `data: {"type":"response.failed","response":{"model":"gpt-6-astra"}}` + "\n\n", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			expires := time.Now().Add(time.Minute)
			pin := &codexGatewayPinUpstream{config: pinConfig(), delegate: gatewayPinDelegate{call: func(r *http.Request, _ string, _ int64, _ int, _ *tlsfingerprint.Profile) (*http.Response, error) {
				require.Equal(t, "__cflb=source-cf; __oailb=cached-route", r.Header.Get("Cookie"))
				resp := pinResponse(tc.responseCookie)
				if tc.stream != "" {
					resp.Body = io.NopCloser(strings.NewReader(tc.stream))
				}
				return resp, nil
			}}}
			resp, err := pin.Do(cachedAstraSourceRequest(t, expires), "source-proxy", 299, 1)
			consumeAffinityResponse(t, resp, err)
			cookie, _ := pin.currentCookie("/backend-api/codex/responses", time.Now())
			if tc.want == "" {
				require.Nil(t, cookie)
				return
			}
			require.NotNil(t, cookie)
			require.Equal(t, tc.want, cookie.Value)
			require.Equal(t, "source-proxy", pin.routes[299].proxy)
			if tc.want == "cached-route" {
				require.Equal(t, expires, pin.routes[299].expires)
			}
		})
	}
}

func TestAstraGatewayCachedSourcePreservesExpiryAndUseLimit(t *testing.T) {
	now := time.Now()
	expires := now.Add(36 * time.Minute)
	pin := &codexGatewayPinUpstream{config: pinConfig(), delegate: gatewayPinDelegate{call: func(_ *http.Request, _ string, _ int64, _ int, _ *tlsfingerprint.Profile) (*http.Response, error) {
		return pinResponse(""), nil
	}}}
	req := cachedAstraSourceRequest(t, expires)
	resp, err := pin.Do(req, "source-proxy", 299, 1)
	consumeAffinityResponse(t, resp, err)
	firstExpiry := pin.routes[299].expires
	require.WithinDuration(t, now.Add(230*time.Second), firstExpiry, time.Second)
	resp, err = pin.Do(req, "source-proxy", 299, 1)
	consumeAffinityResponse(t, resp, err)
	require.Equal(t, firstExpiry, pin.routes[299].expires)
	cookie, _ := pin.currentCookie(req.URL.Path, firstExpiry)
	require.Nil(t, cookie)

	shortExpiry := time.Now().Add(30 * time.Second)
	req = cachedAstraSourceRequest(t, shortExpiry)
	cached := codexGatewayCachedSourceRoute(req, 299)
	route := codexGatewayRouteFromSource(pinResponse("__oailb=cached-route; Path=/; Secure; Max-Age=3600"), req.URL.Path, time.Now(), 230, cached)
	require.Equal(t, shortExpiry, route.expires)
	// A same-value renewal received after expiry keeps the original deadline.
	route = codexGatewayRouteFromSource(pinResponse("__oailb=cached-route; Path=/; Secure; Max-Age=3600"), req.URL.Path, shortExpiry.Add(time.Second), 230, cached)
	require.Equal(t, shortExpiry, route.expires)
	require.Nil(t, codexGatewayRouteFromSource(pinResponse(""), req.URL.Path, shortExpiry.Add(time.Second), 230, cached))

	pin = &codexGatewayPinUpstream{config: pinConfig(), delegate: pin.delegate}
	req = cachedAstraSourceRequest(t, time.Now().Add(-time.Second))
	resp, err = pin.Do(req, "source-proxy", 299, 1)
	consumeAffinityResponse(t, resp, err)
	cookie, _ = pin.currentCookie(req.URL.Path, time.Now())
	require.Nil(t, cookie)
}

func TestAstraGatewayCachedSourceStillRequiresTargetProbe(t *testing.T) {
	for _, degraded := range []bool{false, true} {
		t.Run(map[bool]string{false: "passed", true: "new_ticket"}[degraded], func(t *testing.T) {
			cfg := &config.Config{}
			cfg.Gateway.CodexGatewayPin = pinConfig()
			cfg.Gateway.CodexGatewayPin.IPAffinity = true
			calls := 0
			wrapper := &astraRoutingUpstream{cfg: cfg, delegate: gatewayPinDelegate{call: func(r *http.Request, proxy string, id int64, _ int, _ *tlsfingerprint.Profile) (*http.Response, error) {
				require.Equal(t, "source-proxy", proxy)
				resp := pinResponse("")
				if id == 300 {
					calls++
					require.Equal(t, "Bearer target-own-token", r.Header.Get("Authorization"))
					require.Contains(t, r.Header.Get("Cookie"), "__oailb=cached-route")
					if degraded && calls == 2 {
						resp.Header.Set("X-Codex-Turn-State", "replacement-state")
					}
				}
				return resp, nil
			}}}
			wrapper.SetAstraGatewayPreparer(func(_ context.Context, id int64) error {
				req := cachedAstraSourceRequest(t, time.Now().Add(time.Minute))
				resp, err := wrapper.Do(req, "source-proxy", id, 1)
				consumeAffinityResponse(t, resp, err)
				return nil
			})
			require.NoError(t, wrapper.PrepareAstraGateway(t.Context()))
			require.Zero(t, wrapper.AstraGatewaySnapshot(t.Context()).ReadyRoutes)
			req := pinRequest(t)
			req.Header.Set("Authorization", "Bearer target-own-token")
			err := wrapper.VerifyAstraGatewayTarget(t.Context(), req, "target-proxy", 300, 1)
			require.Equal(t, 2, calls)
			if degraded {
				require.EqualError(t, err, "target_probe_degraded")
				require.Zero(t, wrapper.AstraGatewaySnapshot(t.Context()).ReadyRoutes)
			} else {
				require.NoError(t, err)
				require.Equal(t, 1, wrapper.AstraGatewaySnapshot(t.Context()).ReadyRoutes)
			}
		})
	}
}
