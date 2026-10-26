//go:build integration

package repository

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func TestGPTQoSAcceptanceObservationGroups(t *testing.T) {
	h := newAcceptanceHarness(t)
	// All model requests target the harness's loopback upstream. Exercise both
	// real service processes, streaming/non-streaming and a historical tier change.
	for index, tier := range []string{"premium", "standard", "economy"} {
		for i := 0; i < 2; i++ {
			effort := "low"
			if tier == "premium" {
				effort = "high"
			}
			status, raw, _ := h.requestBody(context.Background(), index+i, h.keys[tier][0], map[string]any{
				"model": "gpt-5.2", "input": fmt.Sprintf("observations-%s-%d", tier, i), "stream": i == 1, "reasoning": map[string]any{"effort": effort},
			})
			require.Equal(t, 200, status, string(raw))
		}
	}
	policy := h.policy
	gid := h.groups["premium"]
	groupPolicy := policy.Groups[fmt.Sprint(gid)]
	groupPolicy.Tier = "standard"
	groupPolicy.Weight = 3
	groupPolicy.CapacityProtection = false
	policy.Groups[fmt.Sprint(gid)] = groupPolicy
	h.save(policy)
	status, body, _ := h.requestBody(context.Background(), 1, h.keys["premium"][0], map[string]any{
		"model": "gpt-5.2", "input": "observations-after-policy-change", "stream": false, "reasoning": map[string]any{"effort": "high"},
	})
	require.Equal(t, 200, status, string(body))
	h.drained(t)
	type snapshot struct {
		Tiers        map[string]service.GPTQoSTierMetrics    `json:"tiers"`
		Groups       map[string][]service.GPTQoSGroupMetrics `json:"groups"`
		AccountTypes map[string]int                          `json:"account_type_requests"`
	}
	for instance := 0; instance < 2; instance++ {
		var all snapshot
		require.Eventually(t, func() bool {
			status, raw := h.admin(instance, http.MethodGet, "/gpt-qos/snapshot", nil)
			return status == 200 && json.Unmarshal(raw, &all) == nil && all.Tiers["premium"].Requests == 2 && all.Tiers["standard"].Requests == 3 && all.Tiers["economy"].Requests == 2
		}, 5*time.Second, 30*time.Millisecond, "completed requests were not observed from both instances")
		require.Equal(t, map[string]int{service.AccountTypeAPIKey: 7}, all.AccountTypes)
		for tier, metrics := range all.Tiers {
			require.Equal(t, metrics.Requests, metrics.APIRequests, tier)
			require.Zero(t, metrics.OAuthRequests, tier)
			require.Equal(t, metrics.Requests, metrics.Successes, tier)
		}
		require.Len(t, all.Groups["premium"], 1)
		require.Equal(t, gid, all.Groups["premium"][0].GroupID)
		require.Equal(t, "audit-premium", all.Groups["premium"][0].GroupName)
		require.Equal(t, 2, all.Groups["premium"][0].Requests)
		require.Len(t, all.Groups["standard"], 2)
		status, raw := h.admin(instance, http.MethodGet, fmt.Sprintf("/gpt-qos/snapshot?group_id=%d&model=gpt-5.2&effort=high", gid), nil)
		require.Equal(t, 200, status, string(raw))
		var filtered snapshot
		require.NoError(t, json.Unmarshal(raw, &filtered))
		require.Len(t, filtered.Tiers, 2)
		require.Equal(t, 2, filtered.Tiers["premium"].Requests)
		require.Equal(t, 1, filtered.Tiers["standard"].Requests)
		require.Len(t, filtered.Groups["standard"], 1)
		require.Equal(t, gid, filtered.Groups["standard"][0].GroupID)
		require.Equal(t, "audit-premium", filtered.Groups["standard"][0].GroupName)
		require.Equal(t, map[string]int{service.AccountTypeAPIKey: 3}, filtered.AccountTypes)
		h.artifact(fmt.Sprintf("observation-groups-instance-%d", instance), map[string]any{"all": all, "filtered": filtered})
	}
}
