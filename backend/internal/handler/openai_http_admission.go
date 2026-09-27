package handler

import (
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
)

// Local pre-send admission failures may select again using the canonical body.
// Completed/partial output and previous_response_id keep their original binding.
func openAIHTTPAdmissionMayRetry(c *gin.Context, err error, previousResponseID string, result *service.OpenAIForwardResult, writerSizeBeforeForward, switchCount, maxSwitches int) bool {
	if previousResponseID != "" || result != nil || switchCount >= maxSwitches || !openAIRequestAllowsFailoverReplay(c) || !openAIForwardMayFailover(c, writerSizeBeforeForward, nil) || service.IsResponseCommitted(c) {
		return false
	}
	switch service.OpenAITurnAdmissionReason(err) {
	case "account_unavailable", "account_ineligible", "credential_parent_ineligible", "group_membership_changed", "model_not_allowed_in_group", "account_binding_changed", "model_runtime_blocked", "account_runtime_blocked", "model_rate_limited", "model_ticket_unavailable":
		return true
	default:
		return false
	}
}
