package repository

import (
	"context"
	"encoding/json"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/internal/service"
)

var _ service.CodexGatewayBlacklistRepository = (*accountRepository)(nil)

func (r *accountRepository) AddCodexTicketGatewayToBlacklist(ctx context.Context, id int64, gateway string) error {
	if dbent.TxFromContext(ctx) == nil {
		tx, err := r.client.Tx(ctx)
		if err != nil {
			return err
		}
		defer func() { _ = tx.Rollback() }()
		if err := r.AddCodexTicketGatewayToBlacklist(dbent.NewTxContext(ctx, tx), id, gateway); err != nil {
			return err
		}
		if err := tx.Commit(); err != nil {
			return err
		}
		r.syncSchedulerAccountSnapshot(ctx, id)
		return nil
	}
	client := clientFromContext(ctx, r.client)
	rows, err := client.QueryContext(ctx, `SELECT COALESCE(extra, '{}'::jsonb) FROM accounts WHERE id = $1 AND deleted_at IS NULL FOR NO KEY UPDATE`, id)
	if err != nil {
		return err
	}
	if !rows.Next() {
		err := rows.Err()
		_ = rows.Close()
		if err != nil {
			return err
		}
		return service.ErrAccountNotFound
	}
	var raw []byte
	err = rows.Scan(&raw)
	_ = rows.Close()
	if err != nil {
		return err
	}
	var extra map[string]any
	if err := json.Unmarshal(raw, &extra); err != nil {
		return err
	}
	gateways := service.OpenAICodexTicketGatewayBlacklist(&service.Account{Extra: extra})
	updates := map[string]any{service.OpenAICodexTicketGatewayBlacklistExtraKey: append(gateways, gateway)}
	if err := service.NormalizeOpenAICodexTicketGatewayBlacklistExtra(updates); err != nil {
		return err
	}
	return r.UpdateExtra(ctx, id, updates)
}
