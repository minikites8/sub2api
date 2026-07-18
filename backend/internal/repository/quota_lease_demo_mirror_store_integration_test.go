//go:build integration

package repository

import (
	"context"
	"strconv"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func TestQuotaLeaseDemoMirrorStoreApplySnapshotMirrorsSchedulableAccounts(t *testing.T) {
	ctx := context.Background()
	client := testEntClient(t)
	accountRepo := newAccountRepositoryWithSQL(client, integrationDB, nil)
	store := NewQuotaLeaseDemoMirrorStore(client, integrationDB, accountRepo)
	require.NotNil(t, store)

	suffix := strconv.FormatInt(time.Now().UnixNano(), 36)
	nodeID := "node-it-" + suffix
	now := time.Now().UTC().Truncate(time.Microsecond)

	proxy := mustCreateProxy(t, client, &service.Proxy{
		Name:     "mirror-proxy-" + suffix,
		Protocol: "socks5",
		Host:     "127.0.0.1",
		Port:     19090,
		Username: "mirror-user",
		Password: "mirror-pass",
		Status:   service.StatusActive,
	})
	group := mustCreateGroup(t, client, &service.Group{
		Name:             "mirror-group-" + suffix,
		Platform:         service.PlatformOpenAI,
		Status:           service.StatusActive,
		SubscriptionType: service.SubscriptionTypeStandard,
		RateMultiplier:   1,
	})
	account := mustCreateAccount(t, client, &service.Account{
		Name:        "mirror-placeholder-" + suffix,
		Platform:    service.PlatformOpenAI,
		Type:        service.AccountTypeOAuth,
		Credentials: map[string]any{},
		Extra:       map[string]any{},
		ProxyID:     &proxy.ID,
		Status:      service.StatusActive,
		Schedulable: true,
	})
	t.Cleanup(func() {
		cleanupQuotaLeaseDemoMirrorIntegrationRows(context.Background(), t, account.ID, group.ID, proxy.ID)
	})

	notes := "mirrored notes"
	loadFactor := 4
	rateMultiplier := 1.25
	proxySnapshot := service.QuotaLeaseDemoProxySnapshot{
		ID:             proxy.ID,
		Name:           proxy.Name,
		Protocol:       proxy.Protocol,
		Host:           proxy.Host,
		Port:           proxy.Port,
		Username:       proxy.Username,
		Password:       proxy.Password,
		Status:         service.StatusActive,
		FallbackMode:   "none",
		ExpiryWarnDays: 7,
		CreatedAt:      now,
		UpdatedAt:      now,
	}
	snapshot := service.QuotaLeaseDemoMirrorSnapshot{
		NodeID:   nodeID,
		SyncedAt: now,
		Groups: []service.QuotaLeaseDemoGroupSnapshot{
			quotaLeaseDemoMirrorIntegrationGroupSnapshot(group.ID, group.Name, now),
		},
		Proxies: []service.QuotaLeaseDemoProxySnapshot{proxySnapshot},
		Accounts: []service.QuotaLeaseDemoAccountSnapshot{{
			ID:                    account.ID,
			Name:                  "mirror-account-" + suffix,
			Notes:                 &notes,
			Platform:              service.PlatformOpenAI,
			Type:                  service.AccountTypeOAuth,
			Credentials:           map[string]any{"access_token": "mirror-access-token"},
			Extra:                 map[string]any{"node_oauth_assigned_node_id": nodeID},
			ProxyID:               &proxy.ID,
			Proxy:                 &proxySnapshot,
			Status:                service.StatusActive,
			Schedulable:           true,
			Concurrency:           2,
			LoadFactor:            &loadFactor,
			Priority:              10,
			RateMultiplier:        &rateMultiplier,
			GroupIDs:              []int64{group.ID},
			AutoPauseOnExpired:    true,
			ProxyFallbackOriginID: nil,
			AccountGroups: []service.QuotaLeaseDemoAccountGroupSnapshot{{
				AccountID: account.ID,
				GroupID:   group.ID,
				Priority:  6,
				CreatedAt: now,
			}},
			CreatedAt: now,
			UpdatedAt: now,
		}},
		AccountGroups: []service.QuotaLeaseDemoAccountGroupSnapshot{{
			AccountID: account.ID,
			GroupID:   group.ID,
			Priority:  6,
			CreatedAt: now,
		}},
	}

	require.NoError(t, store.ApplySnapshot(ctx, snapshot))

	accounts, err := store.ListSchedulableAccounts(ctx, &group.ID, service.PlatformOpenAI)
	require.NoError(t, err)
	require.Len(t, accounts, 1)
	require.Equal(t, account.ID, accounts[0].ID)
	require.Equal(t, "mirror-access-token", accounts[0].Credentials["access_token"])
	require.Equal(t, true, accounts[0].Extra["quota_lease_demo_mirror"])
	require.Equal(t, nodeID, accounts[0].Extra["quota_lease_demo_mirror_node_id"])
	require.Equal(t, []int64{group.ID}, accounts[0].GroupIDs)
	require.Len(t, accounts[0].AccountGroups, 1)
	require.Equal(t, 6, accounts[0].AccountGroups[0].Priority)
	require.NotNil(t, accounts[0].Proxy)
	require.Equal(t, "socks5://mirror-user:mirror-pass@127.0.0.1:19090", accounts[0].Proxy.URL())

	snapshot.Accounts = nil
	snapshot.AccountGroups = nil
	require.NoError(t, store.ApplySnapshot(ctx, snapshot))

	accounts, err = store.ListSchedulableAccounts(ctx, &group.ID, service.PlatformOpenAI)
	require.NoError(t, err)
	require.Empty(t, accounts)

	var deleted bool
	require.NoError(t, integrationDB.QueryRowContext(ctx, `SELECT deleted_at IS NOT NULL FROM accounts WHERE id = $1`, account.ID).Scan(&deleted))
	require.True(t, deleted)
}

func quotaLeaseDemoMirrorIntegrationGroupSnapshot(id int64, name string, now time.Time) service.QuotaLeaseDemoGroupSnapshot {
	return service.QuotaLeaseDemoGroupSnapshot{
		ID:                           id,
		Name:                         name,
		Platform:                     service.PlatformOpenAI,
		RateMultiplier:               1,
		PeakRateMultiplier:           1,
		Status:                       service.StatusActive,
		SubscriptionType:             service.SubscriptionTypeStandard,
		DefaultValidityDays:          30,
		ImageRateMultiplier:          1,
		BatchImageDiscountMultiplier: 0.5,
		BatchImageHoldMultiplier:     0.6,
		VideoRateMultiplier:          1,
		SupportedModelScopes:         []string{"claude", "gemini_text", "gemini_image"},
		KiroAutoStickyEnabled:        true,
		KiroStickySessionTTLSeconds:  3600,
		KiroCacheEmulationRatio:      1,
		KiroEndpointMode:             "q",
		CreatedAt:                    now,
		UpdatedAt:                    now,
	}
}

func cleanupQuotaLeaseDemoMirrorIntegrationRows(ctx context.Context, t *testing.T, accountID, groupID, proxyID int64) {
	t.Helper()
	_, _ = integrationDB.ExecContext(ctx, `DELETE FROM account_groups WHERE account_id = $1 OR group_id = $2`, accountID, groupID)
	_, _ = integrationDB.ExecContext(ctx, `DELETE FROM scheduler_outbox WHERE account_id = $1`, accountID)
	_, _ = integrationDB.ExecContext(ctx, `DELETE FROM accounts WHERE id = $1`, accountID)
	_, _ = integrationDB.ExecContext(ctx, `DELETE FROM groups WHERE id = $1`, groupID)
	_, _ = integrationDB.ExecContext(ctx, `DELETE FROM proxies WHERE id = $1`, proxyID)
}
