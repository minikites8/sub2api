package service

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/pkg/logger"
	"github.com/gin-gonic/gin"
	"github.com/tidwall/gjson"
	"go.uber.org/zap"
)

const openAIHTTPAdmissionFailureContextKey = "openai_http_admission_failure"

// Refresh routing only at the HTTP entry boundary, before request adaptation.
// Continuations keep their account binding and every actual send still performs
// its authoritative admission check.
func (s *OpenAIGatewayService) admitOpenAIHTTPRequest(ctx context.Context, c *gin.Context, selected *Account, body []byte) (*Account, error) {
	return s.admitOpenAIHTTPRequestForModel(ctx, c, selected, body, func(*Account) string { return extractOpenAICodexTicketModel(body) })
}

// Messages model mapping depends on the authoritative account snapshot.
func (s *OpenAIGatewayService) admitOpenAIHTTPRequestForModel(ctx context.Context, c *gin.Context, selected *Account, body []byte, resolveModel func(*Account) string) (*Account, error) {
	groupID, enforceGroup := openAITurnAdmissionGroupFromContext(c)
	latest, err := s.latestOpenAITurnAccountForGroup(ctx, selected, groupID, enforceGroup)
	if err != nil {
		return nil, err
	}
	binding := selected
	refresh := latest.IsOpenAI() && openAITurnRouteFingerprint(latest) != openAITurnRouteFingerprint(selected) && strings.TrimSpace(gjson.GetBytes(body, "previous_response_id").String()) == ""
	if refresh {
		binding = latest
	}
	admitted, err := s.admitOpenAITurnSnapshot(ctx, binding, latest, resolveModel(latest), groupID, enforceGroup)
	if err == nil && refresh {
		logger.FromContext(ctx).Info("openai.http_admission_snapshot_refreshed",
			zap.Int64("account_id", latest.ID), zap.Bool("bps_before", selected.IsExcelBPSEnabled()), zap.Bool("bps_after", admitted.IsExcelBPSEnabled()))
	}
	return admitted, err
}

func OpenAITurnAdmissionReason(err error) string {
	var denied *OpenAITurnAdmissionError
	if errors.As(err, &denied) && denied != nil {
		return denied.Reason
	}
	return ""
}

func OpenAITurnAdmissionMessage(err error) string {
	switch OpenAITurnAdmissionReason(err) {
	case "latest_state_unavailable":
		return "Account state verification temporarily unavailable (latest_state_unavailable); please retry shortly"
	case "account_binding_changed":
		return "Account routing configuration changed (account_binding_changed); please retry with complete context"
	case "account_ineligible", "account_unavailable", "credential_parent_ineligible":
		return fmt.Sprintf("Account is currently unavailable (%s); please retry with an eligible account", OpenAITurnAdmissionReason(err))
	case "group_membership_changed", "model_not_allowed_in_group":
		return fmt.Sprintf("Account no longer matches the requested group or model (%s)", OpenAITurnAdmissionReason(err))
	case "model_ticket_unavailable":
		return "Account model ticket is temporarily unavailable (model_ticket_unavailable); please retry shortly"
	case "model_rate_limited", "model_runtime_blocked", "account_runtime_blocked":
		return fmt.Sprintf("Account is temporarily limited for this request (%s); please retry shortly", OpenAITurnAdmissionReason(err))
	default:
		return fmt.Sprintf("Account admission failed (%s); please retry with complete context", OpenAITurnAdmissionReason(err))
	}
}

func recordOpenAIHTTPAdmissionFailure(ctx context.Context, c *gin.Context, account *Account, body []byte, err error) {
	reason := OpenAITurnAdmissionReason(err)
	if reason == "" || c == nil || account == nil {
		return
	}
	if c.Request != nil && c.Request.Context().Err() != nil {
		return
	}
	path := ""
	if c.Request != nil && c.Request.URL != nil {
		path = c.Request.URL.Path
	}
	model := gjson.GetBytes(body, "model").String()
	message := OpenAITurnAdmissionMessage(err)
	detail := fmt.Sprintf("reason=%s request_path=%s bps_enabled=%t", reason, path, account.IsExcelBPSEnabled())
	c.Set(OpsUpstreamStatusCodeKey, 0)
	setOpsUpstreamError(c, 0, message, detail)
	c.Set(openAIHTTPAdmissionFailureContextKey, true)
	appendOpsUpstreamError(c, OpsUpstreamErrorEvent{
		Platform: account.Platform, AccountID: account.ID, AccountName: account.Name,
		RequestedModel: model, MappedModel: account.GetMappedModel(model),
		Kind: "admission_error", Stage: "admission", Scope: "account", Reason: reason,
		Message: message, Detail: detail,
	})
	logger.FromContext(ctx).Info("openai.http_admission_rejected", zap.Int64("account_id", account.ID), zap.String("reason", reason), zap.Bool("bps_enabled", account.IsExcelBPSEnabled()))
}

func clearOpenAIHTTPAdmissionFailure(c *gin.Context) {
	if c != nil && c.GetBool(openAIHTTPAdmissionFailureContextKey) {
		c.Set(OpsUpstreamStatusCodeKey, 0)
		c.Set(OpsUpstreamErrorMessageKey, "")
		c.Set(OpsUpstreamErrorDetailKey, "")
		c.Set(openAIHTTPAdmissionFailureContextKey, false)
	}
}
