package service

import "context"

type accountQualityUpdateKey struct{}
type accountQualityUpdateFields struct {
	accountID int64
	provided  map[string]bool
}

// WithAccountQualityUpdate records only fields the administrator submitted.
// Capture this before hydration/normalization copies old metadata into extra.
func WithAccountQualityUpdate(ctx context.Context, accountID int64, extra map[string]any) context.Context {
	if fields, exists := ctx.Value(accountQualityUpdateKey{}).(accountQualityUpdateFields); exists && fields.accountID == accountID {
		return ctx
	}
	fields := accountQualityUpdateFields{accountID: accountID, provided: map[string]bool{}}
	for _, key := range []string{AccountQualityTierKey, AccountResourcePoolKey, "qos_upstream_kind"} {
		_, fields.provided[key] = extra[key]
	}
	return context.WithValue(ctx, accountQualityUpdateKey{}, fields)
}

// MergeAccountQualityMetadata prevents ordinary configuration and background
// saves from replacing administrator-owned labels with stale object snapshots.
// The repository calls this with the current row while holding its update lock.
// Explicit Unknown/empty-pool values remain valid administrator updates.
func MergeAccountQualityMetadata(ctx context.Context, accountID int64, proposed, current map[string]any) map[string]any {
	fields, _ := ctx.Value(accountQualityUpdateKey{}).(accountQualityUpdateFields)
	out := make(map[string]any, len(proposed)+4)
	for key, value := range proposed {
		out[key] = value
	}
	for _, key := range []string{AccountQualityTierKey, AccountResourcePoolKey, "qos_upstream_kind", "quality_tier_actor_id"} {
		explicit := fields.accountID == accountID && fields.provided[key]
		if key == "quality_tier_actor_id" {
			explicit = fields.accountID == accountID && fields.provided[AccountQualityTierKey]
		}
		if explicit {
			continue
		}
		if value, exists := current[key]; exists {
			out[key] = value
		} else {
			delete(out, key)
		}
	}
	return out
}
