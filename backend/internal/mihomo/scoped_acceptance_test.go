package mihomo

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestScopedChannelsCollectionAndPinOverlap(t *testing.T) {
	var mu sync.Mutex
	selected := map[string]string{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPut && r.URL.Path != "/configs" {
			var body map[string]string
			_ = json.NewDecoder(r.Body).Decode(&body)
			mu.Lock()
			selected[r.URL.Path] = body["name"]
			mu.Unlock()
		}
		w.WriteHeader(204)
	}))
	defer server.Close()
	m := New(t.TempDir())
	defer m.Close()
	m.controllerURL = server.URL
	m.state.Running = true
	m.saved = saved{Secret: "fixture", Nodes: []map[string]any{{"name": "one"}, {"name": "two"}}, Disabled: map[string]string{}}
	c, err := BeginCollection(t.Context(), Endpoint, 1)
	require.NoError(t, err)
	defer c.Close()
	_, first, err := c.Next(t.Context(), 0)
	require.NoError(t, err)
	ctx, cancel := context.WithTimeout(t.Context(), 100*time.Millisecond)
	defer cancel()
	second, release, err := PinNode(ctx, "two")
	if err == nil {
		defer release()
	}
	t.Logf("collection=%s pin=%s error=%v", first, second, err)
	require.NoError(t, err)
	require.NotEqual(t, first, second)
	mu.Lock()
	defer mu.Unlock()
	require.Len(t, selected, 2)
}

func TestScopedChannelsFullCollectionLeavesPinCapacity(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(204) }))
	defer server.Close()
	m := New(t.TempDir())
	defer m.Close()
	m.controllerURL = server.URL
	m.state.Running = true
	m.saved = saved{Secret: "fixture", Disabled: map[string]string{}}
	for i := 0; i < defaultCollectLanes+1; i++ {
		m.saved.Nodes = append(m.saved.Nodes, map[string]any{"name": fmt.Sprintf("node-%d", i)})
	}
	c, err := BeginCollection(t.Context(), Endpoint, defaultCollectLanes)
	require.NoError(t, err)
	defer c.Close()
	require.Equal(t, defaultCollectLanes, c.LaneCount())
	ctx, cancel := context.WithTimeout(t.Context(), 100*time.Millisecond)
	defer cancel()
	proxy, release, err := PinNode(ctx, "node-32")
	if err == nil {
		defer release()
	}
	t.Logf("full parallel collection target-pin admission: %v", err)
	require.NoError(t, err)
	for _, port := range c.ports {
		require.NotEqual(t, fmt.Sprintf("http://127.0.0.1:%d", port), proxy)
	}
}

func TestScopedChannelsSeparateCollectionsOverlap(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(204) }))
	defer server.Close()
	m := New(t.TempDir())
	defer m.Close()
	m.controllerURL = server.URL
	m.state.Running = true
	m.saved = saved{Secret: "fixture", UseOnce: true, Nodes: []map[string]any{{"name": "one"}, {"name": "two"}}, Disabled: map[string]string{}}
	c, err := BeginCollection(t.Context(), Endpoint, 1)
	require.NoError(t, err)
	defer c.Close()
	first, proxy1, err := c.Next(t.Context(), 0)
	require.NoError(t, err)
	ctx, cancel := context.WithTimeout(t.Context(), 100*time.Millisecond)
	defer cancel()
	other, err := BeginCollection(ctx, Endpoint, 1)
	if err == nil {
		defer other.Close()
	}
	t.Logf("second collection admission: %v", err)
	require.NoError(t, err)
	second, proxy2, err := other.Next(t.Context(), 0)
	require.NoError(t, err)
	require.NotEqual(t, first, second)
	require.NotEqual(t, proxy1, proxy2)
	t.Logf("disjoint selectors=%s,%s; use-once nodes=%s,%s", proxy1, proxy2, first, second)
}

func TestScopedChannelsSeparateControllersOverlap(t *testing.T) {
	first, _ := directedFixture(t)
	second, _ := directedFixture(t)
	a, err := first.Directory(t.Context())
	require.NoError(t, err)
	b, err := second.Directory(t.Context())
	require.NoError(t, err)
	release, err := first.Acquire(t.Context(), a[0])
	require.NoError(t, err)
	defer release()
	ctx, cancel := context.WithTimeout(t.Context(), 100*time.Millisecond)
	defer cancel()
	other, err := second.Acquire(ctx, b[0])
	if err == nil {
		defer other()
	}
	t.Logf("separate controller admission: %v", err)
	require.NoError(t, err)
	same := *first
	wait, stop := context.WithTimeout(t.Context(), 20*time.Millisecond)
	defer stop()
	_, err = same.Acquire(wait, a[0])
	require.ErrorIs(t, err, context.DeadlineExceeded)
	t.Log("shared selector queues while independent controllers overlap")
}

func TestScopedChannelsManagedSidecarsAndRetirement(t *testing.T) {
	var mu sync.Mutex
	selected := map[string]string{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		if r.URL.Path == "/configs" {
			w.WriteHeader(204)
			return
		}
		if r.Method == http.MethodPut {
			var body map[string]string
			_ = json.NewDecoder(r.Body).Decode(&body)
			selected[r.URL.Path] = body["name"]
			w.WriteHeader(204)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]string{"type": "Selector", "now": selected[r.URL.Path]})
	}))
	defer server.Close()
	m := New(t.TempDir())
	defer m.Close()
	m.controllerURL = server.URL
	m.state.Running = true
	m.saved = saved{Secret: "fixture", UseOnce: true, Nodes: []map[string]any{{"name": "one"}, {"name": "two"}}, Disabled: map[string]string{}}
	first, err := LoadDirectedSidecar("", Endpoint)
	require.NoError(t, err)
	second, err := LoadDirectedSidecar("", Endpoint)
	require.NoError(t, err)
	nodes, err := first.Directory(t.Context())
	require.NoError(t, err)
	release, err := first.Acquire(t.Context(), nodes[0])
	require.NoError(t, err)
	defer release()
	ctx, cancel := context.WithTimeout(t.Context(), 100*time.Millisecond)
	defer cancel()
	other, err := second.Acquire(ctx, nodes[1])
	if err == nil {
		defer other()
	}
	t.Logf("separate managed sidecar admission: %v", err)
	require.NoError(t, err)
	require.NotEqual(t, first.ProxyURL, second.ProxyURL)
	require.NoError(t, first.Confirm(t.Context(), nodes[0]))
	require.NoError(t, second.Confirm(t.Context(), nodes[1]))
	third, err := LoadDirectedSidecar("", Endpoint)
	require.NoError(t, err)
	_, err = third.Acquire(t.Context(), nodes[0])
	require.Error(t, err)
	release()
	require.NoError(t, second.Confirm(t.Context(), nodes[1]))
	t.Log("retired nodes remain retired; releasing one lane preserves the other lane")
}
