//go:build integration

package repository

import (
	"context"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func TestAccountUpdateStaleConfigurationPreservesCurrentQuality(t *testing.T) {
	for _, oldTier := range []string{"", "unknown", "basic"} {
		t.Run("stale_"+oldTier, func(t *testing.T) {
			client := testEntTx(t).Client()
			ctx := context.Background()
			oldExtra := map[string]any{"base_rpm": 20}
			if oldTier != "" {
				oldExtra[service.AccountQualityTierKey] = oldTier
			}
			a := mustCreateAccount(t, client, &service.Account{Name: "quality-preserve", Platform: service.PlatformOpenAI, Type: service.AccountTypeAPIKey, Concurrency: 2, Extra: oldExtra})
			repo := NewAccountRepository(client, nil, nil)
			stale, err := repo.GetByID(ctx, a.ID)
			require.NoError(t, err)
			// An administrator labels the account after a configuration/refresh worker
			// loaded its object. Its later ordinary save must not undo that label.
			_, err = client.Account.UpdateOneID(a.ID).SetExtra(map[string]any{service.AccountQualityTierKey: "strong", service.AccountResourcePoolKey: "shared-current", "qos_upstream_kind": "direct", "quality_tier_actor_id": 42, "base_rpm": 20}).Save(ctx)
			require.NoError(t, err)
			stale.Concurrency = 8
			stale.Extra["base_rpm"] = 30
			require.NoError(t, repo.Update(ctx, stale))
			saved, err := repo.GetByID(ctx, a.ID)
			require.NoError(t, err)
			require.Equal(t, "strong", saved.QualityTier())
			require.Equal(t, "shared:shared-current", saved.QoSResourcePool())
			require.Equal(t, "direct", saved.Extra["qos_upstream_kind"])
			require.Equal(t, 8, saved.Concurrency)
			require.EqualValues(t, 30, saved.Extra["base_rpm"])
		})
	}
}

func TestAccountUpdateExplicitQualityChangeStillPersists(t *testing.T) {
	client := testEntTx(t).Client()
	ctx := context.Background()
	a := mustCreateAccount(t, client, &service.Account{Name: "quality-explicit", Platform: service.PlatformOpenAI, Type: service.AccountTypeAPIKey, Concurrency: 2, Extra: map[string]any{service.AccountQualityTierKey: "strong", service.AccountResourcePoolKey: "current-pool", "quality_tier_actor_id": 42}})
	repo := NewAccountRepository(client, nil, nil)
	for _, tier := range []string{"unknown", "basic", "strong"} {
		account, err := repo.GetByID(ctx, a.ID)
		require.NoError(t, err)
		raw := map[string]any{service.AccountQualityTierKey: tier}
		actorCtx := service.WithQualityAuditActor(ctx, 44)
		intentCtx := service.WithAccountQualityUpdate(actorCtx, a.ID, raw)
		require.NoError(t, service.ValidateQualityExtra(intentCtx, raw))
		account.Extra = raw
		require.NoError(t, repo.Update(intentCtx, account))
		saved, err := repo.GetByID(ctx, a.ID)
		require.NoError(t, err)
		require.Equal(t, tier, saved.QualityTier())
		require.Equal(t, "shared:current-pool", saved.QoSResourcePool())
		require.EqualValues(t, 44, saved.Extra["quality_tier_actor_id"])
	}
	// No spurious label changes from the stale configuration saves; explicit
	// changes still have real database audit rows with the authenticated actor.
	var count int
	rows, err := client.QueryContext(ctx, `SELECT count(*) FROM account_quality_audit WHERE account_id=$1 AND actor_user_id=44`, a.ID)
	require.NoError(t, err)
	defer rows.Close()
	require.True(t, rows.Next())
	require.NoError(t, rows.Scan(&count))
	require.NoError(t, rows.Err())
	require.Equal(t, 3, count)
}
