package service

import (
	"fmt"
	"strings"
)

const (
	QuotaLeaseDemoAssignedNodeIDExtraKey  = "node_oauth_assigned_node_id"
	QuotaLeaseDemoAssignedNodeIDsExtraKey = "node_oauth_assigned_node_ids"
)

func QuotaLeaseDemoNormalizeAssignedNodeIDs(value any) []string {
	out := make([]string, 0)
	seen := make(map[string]struct{})
	var appendOne func(any)
	appendOne = func(raw any) {
		switch v := raw.(type) {
		case nil:
			return
		case string:
			nodeID := strings.TrimSpace(v)
			if nodeID == "" {
				return
			}
			if _, ok := seen[nodeID]; ok {
				return
			}
			seen[nodeID] = struct{}{}
			out = append(out, nodeID)
		case []string:
			for _, item := range v {
				appendOne(item)
			}
		case []any:
			for _, item := range v {
				appendOne(item)
			}
		default:
			appendOne(fmt.Sprint(v))
		}
	}
	appendOne(value)
	return out
}

func QuotaLeaseDemoAssignedNodeIDs(account Account) []string {
	if len(account.Extra) == 0 {
		return nil
	}
	if account.Type == AccountTypeAPIKey {
		if ids := QuotaLeaseDemoNormalizeAssignedNodeIDs(account.Extra[QuotaLeaseDemoAssignedNodeIDsExtraKey]); len(ids) > 0 {
			return ids
		}
		return QuotaLeaseDemoNormalizeAssignedNodeIDs(account.Extra[QuotaLeaseDemoAssignedNodeIDExtraKey])
	}
	ids := QuotaLeaseDemoNormalizeAssignedNodeIDs(account.Extra[QuotaLeaseDemoAssignedNodeIDExtraKey])
	if len(ids) > 0 {
		return []string{ids[0]}
	}
	ids = QuotaLeaseDemoNormalizeAssignedNodeIDs(account.Extra[QuotaLeaseDemoAssignedNodeIDsExtraKey])
	if len(ids) > 0 {
		return []string{ids[0]}
	}
	return nil
}

func QuotaLeaseDemoAssignedNodeID(account Account) string {
	ids := QuotaLeaseDemoAssignedNodeIDs(account)
	if len(ids) == 0 {
		return ""
	}
	return ids[0]
}

func QuotaLeaseDemoAccountAssignedToNode(account Account, nodeID string) bool {
	nodeID = strings.TrimSpace(nodeID)
	if nodeID == "" {
		return false
	}
	for _, assignedNodeID := range QuotaLeaseDemoAssignedNodeIDs(account) {
		if assignedNodeID == nodeID {
			return true
		}
	}
	return false
}
