package service

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
)

type qosPoolLabelRepo struct {
	AccountRepository
	accounts []GPTQoSPoolAccount
	err      error
	queries  [][]string
}

func (r *qosPoolLabelRepo) GetGPTQoSPoolAccounts(_ context.Context, ids []string) ([]GPTQoSPoolAccount, error) {
	r.queries = append(r.queries, append([]string(nil), ids...))
	return r.accounts, r.err
}

type qosPoolLabelStore struct {
	schedulerTestConcurrencyCache
	GPTQoSAdmissionStore
	state map[string]any
}

func (s *qosPoolLabelStore) GPTQoSState(context.Context) (map[string]any, error) { return s.state, nil }
func (s *qosPoolLabelStore) GPTQoSMetrics(context.Context, GPTQoSMetricFilter) ([]GPTQoSMetric, error) {
	return nil, nil
}

func TestGPTQoSAdminPoolLabelsPreserveIdentityAndSharedCapacity(t *testing.T) {
	repo := &qosPoolLabelRepo{accounts: []GPTQoSPoolAccount{
		{ID: 1, Name: "正常 OAuth", Quality: "strong", PoolID: "account:1"},
		{ID: 2, Name: "账号 B", Quality: "basic", PoolID: "shared:fixture"},
		{ID: 3, Name: "账号 C", Quality: "unknown", PoolID: "shared:fixture"},
	}}
	pools := []any{map[string]any{"id": "account:1", "capacity": 3}, map[string]any{"id": "shared:fixture", "capacity": 10}}
	store := &qosPoolLabelStore{state: map[string]any{"pools": pools, "active": 2}}
	svc := &OpenAIGatewayService{accountRepo: repo, concurrencyService: NewConcurrencyService(store)}
	result := svc.GPTQoSSnapshot(context.Background())
	require.Equal(t, true, result["available"])
	require.Equal(t, [][]string{{"account:1", "shared:fixture"}}, repo.queries, "one metadata read covers all visible pools")
	require.Equal(t, "Strong:正常 OAuth", pools[0].(map[string]any)["display_name"])
	require.Equal(t, "Basic:账号 B / Unknown:账号 C", pools[1].(map[string]any)["display_name"])
	require.Equal(t, "account:1", pools[0].(map[string]any)["id"])
	require.Equal(t, "shared:fixture", pools[1].(map[string]any)["id"])
	require.Len(t, result["capacity"].(map[string]any)["pools"], 2)
	require.Equal(t, 10, pools[1].(map[string]any)["capacity"])
	encoded, err := json.Marshal(result)
	require.NoError(t, err)
	require.NotContains(t, string(encoded), "credentials")
	require.NotContains(t, string(encoded), "access_token")
}

func TestGPTQoSAdminPoolLabelFailureDoesNotHideCapacity(t *testing.T) {
	repo := &qosPoolLabelRepo{err: errors.New("metadata unavailable")}
	pool := map[string]any{"id": "account:1", "active": 2, "capacity": 3}
	store := &qosPoolLabelStore{state: map[string]any{"pools": []any{pool}}}
	svc := &OpenAIGatewayService{accountRepo: repo, concurrencyService: NewConcurrencyService(store)}
	result := svc.GPTQoSSnapshot(context.Background())
	require.Equal(t, true, result["available"])
	require.Equal(t, 3, pool["capacity"])
	require.Equal(t, 2, pool["active"])
	require.NotContains(t, pool, "display_name")
}
