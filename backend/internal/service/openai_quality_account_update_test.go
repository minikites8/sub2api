package service

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestGPTQualityMetadataSurvivesUnrelatedExtraEdit(t *testing.T) {
	account := ticketTestAccount(41)
	account.Concurrency = 2
	account.Extra = map[string]any{AccountQualityTierKey: "strong", AccountResourcePoolKey: "shared-fixture", "qos_upstream_kind": "direct"}
	repo := &upstreamBillingProbeAccountRepo{accounts: map[int64]*Account{41: account}}
	svc := &adminServiceImpl{accountRepo: repo}
	updated, err := svc.UpdateAccount(context.Background(), 41, &UpdateAccountInput{Extra: map[string]any{"custom": true}})
	require.NoError(t, err)
	require.Equal(t, "strong", updated.QualityTier())
	require.Equal(t, "shared:shared-fixture", updated.QoSResourcePool())
	require.Equal(t, "direct", updated.Extra["qos_upstream_kind"])
	updated, err = svc.UpdateAccount(context.Background(), 41, &UpdateAccountInput{Extra: map[string]any{AccountQualityTierKey: "unknown", AccountResourcePoolKey: ""}})
	require.NoError(t, err)
	require.Equal(t, "unknown", updated.QualityTier())
	require.Equal(t, "account:41", updated.QoSResourcePool())
}
