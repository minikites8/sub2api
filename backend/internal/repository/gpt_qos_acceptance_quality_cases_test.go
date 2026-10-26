//go:build integration

package repository

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func (h *acceptanceHarness) qualityScenarios(t *testing.T) {
	t.Run("RealStrongStandbyPrecedesBasic", func(t *testing.T) {
		status, raw := h.admin(0, http.MethodPut, fmt.Sprintf("/accounts/%d", h.strong[1]), map[string]any{"is_fallback": true})
		require.Equal(t, 200, status, string(raw))
		// Existing ordinary slots must be visible to the real QoS Lua admission.
		cache := NewConcurrencyCache(integrationRedis, 1, 60).(*concurrencyCache)
		for i := 0; i < 10; i++ {
			ok, err := cache.AcquireAccountSlot(context.Background(), h.strong[0], 10, fmt.Sprintf("fixture-primary-busy-%d", i))
			require.NoError(t, err)
			require.True(t, ok)
		}
		defer func() {
			for i := 0; i < 10; i++ {
				_ = cache.ReleaseAccountSlot(context.Background(), h.strong[0], fmt.Sprintf("fixture-primary-busy-%d", i))
			}
			_, _ = h.admin(0, http.MethodPut, fmt.Sprintf("/accounts/%d", h.strong[1]), map[string]any{"is_fallback": false})
		}()
		status, body, _ := h.request(context.Background(), 1, h.keys["premium"][0], "standby-real", false)
		require.Equal(t, 200, status, string(body))
		require.Contains(t, string(body), fmt.Sprintf("resp_%d_", h.strong[1]))
	})
	t.Run("RealModelEligibilityBeforeQualityAndQueue", func(t *testing.T) {
		models := map[string][]string{}
		for _, gid := range h.groups {
			models[fmt.Sprint(gid)] = []string{"fixture-other-model"}
		}
		for _, id := range h.strong {
			s, raw := h.admin(0, http.MethodPut, fmt.Sprintf("/accounts/%d", id), map[string]any{"group_allowed_models": models})
			require.Equal(t, 200, s, string(raw))
		}
		defer func() {
			clear := map[string][]string{}
			for _, gid := range h.groups {
				clear[fmt.Sprint(gid)] = []string{}
			}
			for _, id := range h.strong {
				_, _ = h.admin(0, http.MethodPut, fmt.Sprintf("/accounts/%d", id), map[string]any{"group_allowed_models": clear})
			}
		}()
		require.Eventually(t, func() bool {
			ctx, cancel := context.WithTimeout(context.Background(), time.Second)
			defer cancel()
			s, b, _ := h.request(ctx, 1, h.keys["economy"][0], "model-basic-compatible", false)
			return s == 200 && (strings.Contains(string(b), fmt.Sprintf("resp_%d_", h.basic[0])) || strings.Contains(string(b), fmt.Sprintf("resp_%d_", h.basic[1])))
		}, 10*time.Second, 100*time.Millisecond)
		s, b, _ := h.request(context.Background(), 0, h.keys["premium"][0], "model-guarantee-denied", true)
		require.Equal(t, 503, s, string(b))
		require.Contains(t, string(b), "gpt_qos_no_eligible_capacity")
		h.upstream.mu.Lock()
		for _, e := range h.upstream.events {
			require.NotEqual(t, "model-guarantee-denied", e.ID)
		}
		h.upstream.mu.Unlock()
	})
	t.Run("ClientCannotPromoteQoSTier", func(t *testing.T) {
		at := time.Now().UnixMilli()
		s, b, _ := h.requestBody(context.Background(), 0, h.keys["economy"][0], map[string]any{"model": "gpt-5.2", "input": "forged-premium", "stream": false, "qos_tier": "premium", "weight": 100})
		require.Equal(t, 200, s, string(b))
		require.Eventually(t, func() bool {
			rows, _ := integrationRedis.LRange(context.Background(), "gptqos:{v1}:metrics", 0, 100).Result()
			for _, row := range rows {
				var v service.GPTQoSMetric
				if json.Unmarshal([]byte(row), &v) == nil && v.At >= at && v.GroupID == h.groups["economy"] {
					require.Equal(t, "economy", v.Tier)
					return true
				}
			}
			return false
		}, 3*time.Second, 20*time.Millisecond)
	})
}
