package repository

import (
	"context"
	"strconv"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/lib/pq"
)

// GetGPTQoSPoolAccounts projects only public admin labels in one read. It is
// separate from account hydration, so the snapshot never loads credentials.
func (r *accountRepository) GetGPTQoSPoolAccounts(ctx context.Context, pools []string) ([]service.GPTQoSPoolAccount, error) {
	ids, shared := []int64{}, []string{}
	for _, pool := range pools {
		if raw, ok := strings.CutPrefix(pool, "account:"); ok {
			if id, err := strconv.ParseInt(raw, 10, 64); err == nil && id > 0 {
				ids = append(ids, id)
			}
		} else if name, ok := strings.CutPrefix(pool, "shared:"); ok && name != "" {
			shared = append(shared, name)
		}
	}
	if len(ids)+len(shared) == 0 {
		return nil, nil
	}
	rows, err := r.sql.QueryContext(ctx, `SELECT id, name,
 CASE WHEN extra->>'quality_tier' IN ('strong','basic') THEN extra->>'quality_tier' ELSE 'unknown' END,
 COALESCE(extra->>'qos_resource_pool','')
 FROM accounts WHERE deleted_at IS NULL AND platform='openai'
 AND (id=ANY($1::bigint[]) OR extra->>'qos_resource_pool'=ANY($2::text[])) ORDER BY id`, pq.Array(ids), pq.Array(shared))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []service.GPTQoSPoolAccount
	for rows.Next() {
		var a service.GPTQoSPoolAccount
		var pool string
		if err := rows.Scan(&a.ID, &a.Name, &a.Quality, &pool); err != nil {
			return nil, err
		}
		a.PoolID = (&service.Account{ID: a.ID, Extra: map[string]any{service.AccountResourcePoolKey: pool}}).QoSResourcePool()
		out = append(out, a)
	}
	return out, rows.Err()
}
