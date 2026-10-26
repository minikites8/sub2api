package service

import (
	"context"
	"strconv"
	"strings"
	"time"
)

// GPTQoSPoolAccount is an admin-only projection. Credentials and request data
// are intentionally absent; labels must never become scheduler identities.
type GPTQoSPoolAccount struct {
	ID                    int64
	Name, Quality, PoolID string
}

type GPTQoSPoolAccountReader interface {
	GetGPTQoSPoolAccounts(context.Context, []string) ([]GPTQoSPoolAccount, error)
}

func (s *OpenAIGatewayService) labelGPTQoSPools(ctx context.Context, state map[string]any) {
	reader, ok := s.accountRepo.(GPTQoSPoolAccountReader)
	pools, _ := state["pools"].([]any)
	if !ok || len(pools) == 0 {
		return
	}
	ids := make([]string, 0, len(pools))
	for _, item := range pools {
		if pool, ok := item.(map[string]any); ok {
			if id, ok := pool["id"].(string); ok {
				ids = append(ids, id)
			}
		}
	}
	// Admin snapshots only: never add account-name queries to model admission.
	readCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	accounts, err := reader.GetGPTQoSPoolAccounts(readCtx, ids)
	if err != nil {
		return
	}
	byAccount := make(map[string]string, len(accounts))
	byPool := make(map[string][]string)
	for _, a := range accounts {
		tier := "Unknown"
		if a.Quality == "strong" {
			tier = "Strong"
		}
		if a.Quality == "basic" {
			tier = "Basic"
		}
		label := tier + ":" + a.Name
		byAccount["account:"+strconv.FormatInt(a.ID, 10)] = label
		byPool[a.PoolID] = append(byPool[a.PoolID], label)
	}
	for _, item := range pools {
		pool, ok := item.(map[string]any)
		if !ok {
			continue
		}
		id, _ := pool["id"].(string)
		if label := byAccount[id]; label != "" {
			pool["display_name"] = label
		} else if labels := byPool[id]; len(labels) > 0 {
			pool["display_name"] = strings.Join(labels, " / ")
		}
	}
}
