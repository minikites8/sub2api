package repository

import (
	"context"
	"errors"
	"fmt"
	"strings"

	entsql "entgo.io/ent/dialect/sql"
	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/ent/apikey"
	"github.com/Wei-Shaw/sub2api/ent/predicate"
	"github.com/Wei-Shaw/sub2api/internal/service"
)

func apiKeyInGroup(id int64) predicate.APIKey {
	return func(s *entsql.Selector) {
		links := entsql.Table("api_key_groups")
		s.Where(entsql.Or(
			entsql.EQ(s.C(apikey.FieldGroupID), id),
			entsql.In(s.C(apikey.FieldID), entsql.Select(links.C("api_key_id")).From(links).Where(entsql.EQ(links.C("group_id"), id))),
		))
	}
}

// Include the legacy binding and batch-load extras without N+1 list queries.
func (r *apiKeyRepository) attachGroupIDs(ctx context.Context, keys []service.APIKey) error {
	if len(keys) == 0 {
		return nil
	}
	ids := make([]any, len(keys))
	placeholders := make([]string, len(keys))
	index := make(map[int64]int, len(keys))
	orders := make([][]int64, len(keys))
	for i := range keys {
		ids[i], index[keys[i].ID] = keys[i].ID, i
		placeholders[i] = fmt.Sprintf("$%d", i+1)
		orders[i] = append([]int64(nil), keys[i].GroupIDs...)
		keys[i].GroupIDs = nil
		if keys[i].GroupID != nil {
			keys[i].GroupIDs = []int64{*keys[i].GroupID}
		}
	}
	rows, err := clientFromContext(ctx, r.client).QueryContext(ctx,
		`SELECT b.api_key_id, b.group_id FROM api_key_groups b
		JOIN groups g ON g.id = b.group_id AND g.deleted_at IS NULL
		WHERE b.api_key_id IN (`+strings.Join(placeholders, ",")+`) ORDER BY b.group_id`, ids...)
	if err != nil {
		return err
	}
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		var keyID, groupID int64
		if err := rows.Scan(&keyID, &groupID); err != nil {
			return err
		}
		i, ok := index[keyID]
		if ok && (keys[i].GroupID == nil || *keys[i].GroupID != groupID) {
			keys[i].GroupIDs = append(keys[i].GroupIDs, groupID)
		}
	}
	if err := rows.Err(); err != nil {
		return err
	}
	for i := range keys {
		remaining := make(map[int64]bool, len(keys[i].GroupIDs))
		for _, id := range keys[i].GroupIDs {
			remaining[id] = true
		}
		ordered := make([]int64, 0, len(remaining))
		for _, id := range orders[i] {
			if remaining[id] {
				ordered = append(ordered, id)
				delete(remaining, id)
			}
		}
		for _, id := range keys[i].GroupIDs {
			if remaining[id] {
				ordered = append(ordered, id)
				delete(remaining, id)
			}
		}
		keys[i].GroupIDs = ordered
	}
	return nil
}

func (r *apiKeyRepository) entityWithGroupIDs(ctx context.Context, entity *dbent.APIKey) (*service.APIKey, error) {
	keys := []service.APIKey{*apiKeyEntityToService(entity)}
	if err := r.attachGroupIDs(ctx, keys); err != nil {
		return nil, err
	}
	return &keys[0], nil
}

func (r *apiKeyRepository) mutateWithGroups(ctx context.Context, key *service.APIKey, mutate func(context.Context) error) error {
	if dbent.TxFromContext(ctx) != nil {
		if err := mutate(ctx); err != nil {
			return err
		}
		return r.replaceExtraGroups(ctx, key)
	}
	tx, err := r.client.Tx(ctx)
	if errors.Is(err, dbent.ErrTxStarted) {
		if err := mutate(ctx); err != nil {
			return err
		}
		return r.replaceExtraGroups(ctx, key)
	}
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	txCtx := dbent.NewTxContext(ctx, tx)
	if err := mutate(txCtx); err != nil {
		return err
	}
	if err := r.replaceExtraGroups(txCtx, key); err != nil {
		return err
	}
	return tx.Commit()
}

func (r *apiKeyRepository) replaceExtraGroups(ctx context.Context, key *service.APIKey) error {
	client := clientFromContext(ctx, r.client)
	if _, err := client.ExecContext(ctx, `DELETE FROM api_key_groups WHERE api_key_id = $1`, key.ID); err != nil {
		return err
	}
	if key.GroupID == nil {
		return nil
	}
	for _, id := range key.GroupIDs {
		if key.GroupID != nil && *key.GroupID == id {
			continue
		}
		if _, err := client.ExecContext(ctx, `INSERT INTO api_key_groups (api_key_id, group_id) VALUES ($1, $2)`, key.ID, id); err != nil {
			return fmt.Errorf("save API key groups: %w", err)
		}
	}
	return nil
}

func (r *apiKeyRepository) changeOrderedGroupBindings(ctx context.Context, userID, oldID, newID int64) (int64, error) {
	client := clientFromContext(ctx, r.client)
	var commit func() error
	if dbent.TxFromContext(ctx) == nil {
		tx, err := client.Tx(ctx)
		if err != nil {
			return 0, err
		}
		defer func() { _ = tx.Rollback() }()
		ctx = dbent.NewTxContext(ctx, tx)
		client = clientFromContext(ctx, r.client)
		commit = tx.Commit
	}
	query := client.APIKey.Query().Where(apikey.DeletedAtIsNil(), apikey.Or(apiKeyBoundToGroup(oldID), apiKeyInGroup(oldID)))
	if userID != 0 {
		query = query.Where(apikey.UserIDEQ(userID))
	}
	if client.Driver().Dialect() == "postgres" {
		query = query.ForUpdate()
	}
	entities, err := query.All(ctx)
	if err != nil {
		return 0, err
	}
	for _, entity := range entities {
		key, err := r.entityWithGroupIDs(ctx, entity)
		if err != nil {
			return 0, err
		}
		ids := make([]int64, 0, len(key.GroupIDs))
		seen := map[int64]bool{}
		for _, id := range key.GroupIDs {
			if id == oldID {
				id = newID
			}
			if id > 0 && !seen[id] {
				ids = append(ids, id)
				seen[id] = true
			}
		}
		key.GroupIDs = ids
		key.GroupID = nil
		update := client.APIKey.UpdateOneID(key.ID).SetGroupIds(ids)
		if len(ids) > 0 {
			key.GroupID = &ids[0]
			update.SetGroupID(ids[0])
		} else {
			update.ClearGroupID()
		}
		if _, err := update.Save(ctx); err != nil {
			return 0, err
		}
		if err := r.replaceExtraGroups(ctx, key); err != nil {
			return 0, err
		}
	}
	if commit != nil {
		if err := commit(); err != nil {
			return 0, err
		}
	}
	return int64(len(entities)), nil
}
