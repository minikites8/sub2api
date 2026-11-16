package repository

import (
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/pkg/tlsfingerprint"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func TestOpenCodeZenTLSClientCacheIsolatesFingerprints(t *testing.T) {
	upstream := NewHTTPUpstream(nil).(*httpUpstreamService)
	get := func(profile *tlsfingerprint.Profile) *upstreamClientEntry {
		t.Helper()
		entry, err := upstream.getClientEntryWithTLS("", 1, 1, profile, service.HTTPUpstreamProfileOpenAI, false, false)
		require.NoError(t, err)
		return entry
	}
	first := get(tlsfingerprint.OpenCodeBunProfile())
	require.Same(t, first, get(tlsfingerprint.OpenCodeBunProfile()))
	custom := tlsfingerprint.OpenCodeBunProfile()
	custom.CipherSuites = custom.CipherSuites[1:]
	require.NotSame(t, first, get(custom), "profile edits must invalidate the cached client even if the name is unchanged")
	require.NotSame(t, first, get(&tlsfingerprint.Profile{Name: "configured"}))
	for _, entry := range upstream.clients {
		entry.client.CloseIdleConnections()
	}
}
