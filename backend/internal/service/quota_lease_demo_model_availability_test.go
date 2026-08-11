package service

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
)

func TestQuotaLeaseDemoRemoteNodeFlushesModelAvailability(t *testing.T) {
	var received []QuotaLeaseDemoModelAvailabilityEvent
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path != "/api/v1/node-leases/demo/model-availability/batch" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		require.Equal(t, "node-us", r.Header.Get("X-Node-ID"))
		require.Equal(t, "node-secret", r.Header.Get("X-Node-Secret"))
		var req QuotaLeaseDemoModelAvailabilityBatchRequest
		require.NoError(t, json.NewDecoder(r.Body).Decode(&req))
		received = append(received, req.Events...)
		result := QuotaLeaseDemoModelAvailabilityBatchResult{
			Results: make([]QuotaLeaseDemoModelAvailabilityResult, 0, len(req.Events)),
		}
		for _, event := range req.Events {
			result.Results = append(result.Results, QuotaLeaseDemoModelAvailabilityResult{
				EventID: event.EventID,
				Applied: true,
			})
		}
		require.NoError(t, json.NewEncoder(w).Encode(result))
	}))
	defer server.Close()

	node := NewQuotaLeaseDemoService(&config.Config{
		DeploymentRole: config.DeploymentRoleNode,
		Gateway: config.GatewayConfig{
			QuotaLeaseDemo: config.GatewayQuotaLeaseDemoConfig{
				Enabled:             true,
				NodeID:              "node-us",
				ControlPlaneBaseURL: server.URL + "/api/v1/node-leases/demo",
			},
		},
	})
	node.remoteMu.Lock()
	node.remoteNodeID = "node-us"
	node.remoteNodeSecret = "node-secret"
	node.remoteMu.Unlock()

	checkedAt := time.Date(2026, 8, 11, 14, 30, 0, 0, time.UTC)
	event := NewQuotaLeaseDemoModelAvailabilityEvent("node-us", "request-1", "veo-3.1", false, checkedAt)
	node.enqueuePendingModelAvailabilityEvent(event)

	require.NoError(t, node.FlushPendingModelAvailability(context.Background()))
	require.Len(t, received, 1)
	require.Equal(t, event.EventID, received[0].EventID)
	require.Equal(t, "veo-3.1", received[0].Model)
	require.False(t, received[0].Success)
	require.Equal(t, checkedAt, received[0].CheckedAt)
	require.Empty(t, node.pendingModelAvailabilityEvents())
}

func TestQuotaLeaseDemoModelAvailabilityKeepsUnackedEvents(t *testing.T) {
	svc := NewQuotaLeaseDemoService(nil)
	first := NewQuotaLeaseDemoModelAvailabilityEvent("node-1", "request-1", "gpt-image-2", true, time.Now())
	second := NewQuotaLeaseDemoModelAvailabilityEvent("node-1", "request-2", "gpt-image-2", false, time.Now())
	svc.enqueuePendingModelAvailabilityEvent(first)
	svc.enqueuePendingModelAvailabilityEvent(second)

	svc.removePendingModelAvailabilityResults(QuotaLeaseDemoModelAvailabilityBatchResult{Results: []QuotaLeaseDemoModelAvailabilityResult{
		{EventID: first.EventID, Applied: true},
		{EventID: second.EventID, Error: "temporary failure"},
	}})

	pending := svc.pendingModelAvailabilityEvents()
	require.Len(t, pending, 1)
	require.Equal(t, second.EventID, pending[0].EventID)
}
