package service

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestAccountQualityMetadataOnlyChangesOnExplicitAdminInput(t *testing.T) {
	current := map[string]any{AccountQualityTierKey: "strong", AccountResourcePoolKey: "current-pool", "qos_upstream_kind": "direct", "quality_tier_actor_id": 42}
	stale := map[string]any{AccountQualityTierKey: "unknown", AccountResourcePoolKey: "old-pool", "qos_upstream_kind": "vm2api", "quality_tier_actor_id": 0, "base_rpm": 30}
	for _, ctx := range []context.Context{context.Background(), WithAccountQualityUpdate(context.Background(), 41, map[string]any{"base_rpm": 30})} {
		merged := MergeAccountQualityMetadata(ctx, 41, stale, current)
		for key, want := range current {
			require.Equal(t, want, merged[key])
		}
		require.Equal(t, 30, merged["base_rpm"])
	}
	raw := map[string]any{AccountQualityTierKey: "unknown", "quality_tier_actor_id": 44}
	ctx := WithAccountQualityUpdate(context.Background(), 41, raw)
	merged := MergeAccountQualityMetadata(ctx, 41, raw, current)
	require.Equal(t, "unknown", merged[AccountQualityTierKey])
	require.Equal(t, 44, merged["quality_tier_actor_id"])
	require.Equal(t, "current-pool", merged[AccountResourcePoolKey])
	require.Equal(t, "direct", merged["qos_upstream_kind"])
	require.Equal(t, "strong", current[AccountQualityTierKey], "never mutate the locked snapshot")
	require.Equal(t, "unknown", stale[AccountQualityTierKey], "never mutate a caller's snapshot")
}

func TestAccountQualityMetadataDoesNotResurrectRemovedLabels(t *testing.T) {
	merged := MergeAccountQualityMetadata(context.Background(), 41, map[string]any{AccountQualityTierKey: "strong", AccountResourcePoolKey: "old-pool", "quality_tier_actor_id": 42}, nil)
	require.NotContains(t, merged, AccountQualityTierKey)
	require.NotContains(t, merged, AccountResourcePoolKey)
	require.NotContains(t, merged, "quality_tier_actor_id")
}

func TestAccountQualityIntentIsFrozenBeforeHydration(t *testing.T) {
	raw := map[string]any{AccountResourcePoolKey: ""}
	ctx := WithAccountQualityUpdate(context.Background(), 41, raw)
	// Hydration and normalization must not make unchanged labels look explicit.
	ctx = WithAccountQualityUpdate(ctx, 41, map[string]any{AccountQualityTierKey: "unknown", AccountResourcePoolKey: ""})
	merged := MergeAccountQualityMetadata(ctx, 41, map[string]any{AccountQualityTierKey: "unknown", AccountResourcePoolKey: ""}, map[string]any{AccountQualityTierKey: "strong", AccountResourcePoolKey: "current"})
	require.Equal(t, "strong", merged[AccountQualityTierKey])
	require.Equal(t, "", merged[AccountResourcePoolKey])
}

func TestAccountQualityIntentDoesNotAffectOtherAccount(t *testing.T) {
	ctx := WithAccountQualityUpdate(context.Background(), 41, map[string]any{AccountQualityTierKey: "unknown"})
	merged := MergeAccountQualityMetadata(ctx, 42, map[string]any{AccountQualityTierKey: "unknown"}, map[string]any{AccountQualityTierKey: "strong"})
	require.Equal(t, "strong", merged[AccountQualityTierKey], "a parent account edit must not change labels while updating a shadow")
}
