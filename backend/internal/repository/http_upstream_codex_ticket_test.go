package repository

import (
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func TestCodexTicketHarvestTransportIsolation(t *testing.T) {
	svc := &httpUpstreamService{cfg: &config.Config{}}
	mode := svc.resolveProtocolMode(service.HTTPUpstreamProfileOpenAIHarvest, "", nil)
	require.Equal(t, upstreamProtocolModeOpenAIH1NoReuse, mode)
	settings := poolSettings{maxIdleConns: 16, maxIdleConnsPerHost: 8, maxConnsPerHost: 8, idleConnTimeout: time.Minute}
	tr, err := buildUpstreamTransport(settings, nil, mode)
	require.NoError(t, err)
	defer tr.CloseIdleConnections()
	require.True(t, tr.DisableKeepAlives)
	require.False(t, tr.ForceAttemptHTTP2)
	require.NotNil(t, tr.TLSNextProto)
	normal, err := buildUpstreamTransport(settings, nil, upstreamProtocolModeOpenAIH2)
	require.NoError(t, err)
	defer normal.CloseIdleConnections()
	require.True(t, normal.ForceAttemptHTTP2)
	require.False(t, normal.DisableKeepAlives)
	var opened atomic.Int32
	server := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(200) }))
	server.Config.ConnState = func(_ net.Conn, state http.ConnState) {
		if state == http.StateNew {
			opened.Add(1)
		}
	}
	server.Start()
	defer server.Close()
	client := &http.Client{Transport: tr, Timeout: time.Second}
	for i := 0; i < 2; i++ {
		resp, err := client.Get(server.URL)
		require.NoError(t, err)
		_, err = io.Copy(io.Discard, resp.Body)
		require.NoError(t, err)
		require.NoError(t, resp.Body.Close())
	}
	require.EqualValues(t, 2, opened.Load())
}
