package admin

import (
	"errors"
	"net/http"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/pkg/yeteam"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

type YeTeamRedeemRequest struct {
	CardCode             string                `json:"card_code" binding:"required"`
	TargetID             string                `json:"target_id,omitempty"`
	ClientRequestID      string                `json:"client_request_id,omitempty"`
	SkipDefaultGroupBind *bool                 `json:"skip_default_group_bind,omitempty"`
	AccountOptions       *AccountImportOptions `json:"account_options,omitempty"`
}

type YeTeamRedeemResponse struct {
	OrderNo        string            `json:"order_no"`
	Status         string            `json:"status,omitempty"`
	Action         string            `json:"action,omitempty"`
	AccountCreated int               `json:"account_created"`
	AccountFailed  int               `json:"account_failed"`
	ImportErrors   []DataImportError `json:"import_errors,omitempty"`
}

// RedeemYeTeam exchanges one CDK and imports the returned sub2api account
// package through the same validation and persistence path as JSON imports.
func (h *AccountHandler) RedeemYeTeam(c *gin.Context) {
	if h == nil || h.yeTeam == nil || !h.yeTeam.Enabled() {
		response.Error(c, http.StatusServiceUnavailable, "ye.team integration is disabled")
		return
	}
	var req YeTeamRedeemRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}
	req.CardCode = strings.TrimSpace(req.CardCode)
	if req.CardCode == "" {
		response.BadRequest(c, "card_code is required")
		return
	}
	ctx := c.Request.Context()
	preview, err := h.yeTeam.Preview(ctx, yeteam.PreviewRequest{
		CardCode: req.CardCode,
		Format:   "sub2api",
		Project:  "k12",
		TargetID: req.TargetID,
	})
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	if value, ok := preview.Raw["can_fulfill"].(bool); ok && !value {
		response.Error(c, http.StatusBadRequest, preview.Message)
		return
	}
	if value, ok := preview.Raw["available"].(bool); ok && !value {
		response.Error(c, http.StatusBadRequest, preview.Message)
		return
	}
	action := strings.TrimSpace(preview.RedeemAction)
	if action == "" {
		action = strings.TrimSpace(preview.Action)
	}
	if action == "" {
		switch {
		case preview.CanRefreshBound && preview.BoundCount > 0:
			action = "refresh_bound"
		case preview.CanRedeemRemaining && preview.CardQuotaRemaining > 0:
			action = "redeem_remaining"
		default:
			action = "redeem_remaining"
		}
	}
	if action != "refresh_bound" && action != "redeem_remaining" {
		response.Error(c, http.StatusBadRequest, "ye.team preview returned an unsupported action")
		return
	}
	requestID := strings.TrimSpace(req.ClientRequestID)
	if requestID == "" {
		requestID = newYeTeamRequestID()
	}
	order, err := h.yeTeam.Redeem(ctx, yeteam.RedeemRequest{
		CardCode: req.CardCode, Format: "sub2api", Project: "k12", TargetID: req.TargetID,
		Action: action, ClientRequestID: requestID,
	})
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	if order.OrderNo == "" {
		response.ErrorFrom(c, errors.New("ye.team redeem response did not include order_no"))
		return
	}
	finalOrder, err := h.yeTeam.PollUntilDone(ctx, order.OrderNo, order.DownloadToken)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	if finalOrder.OrderNo == "" {
		finalOrder.OrderNo = order.OrderNo
	}
	token := strings.TrimSpace(finalOrder.DownloadToken)
	if token == "" {
		token = strings.TrimSpace(finalOrder.Token)
	}
	if token == "" {
		token = nestedOrderString(finalOrder.Raw, "download_token", "downloadToken", "token")
	}
	if token == "" {
		response.ErrorFrom(c, errors.New("ye.team order did not include download_token"))
		return
	}
	accountJSON, err := h.yeTeam.Download(ctx, finalOrder.OrderNo, token)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	normalized, err := yeteam.NormalizeAccountPayload(accountJSON)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	skip := true
	if req.SkipDefaultGroupBind != nil {
		skip = *req.SkipDefaultGroupBind
	}
	payload, err := parseDataImportPayload(normalized)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	if err := validateDataHeader(payload); err != nil {
		response.ErrorFrom(c, err)
		return
	}
	for i := range payload.Accounts {
		if payload.Accounts[i].Extra == nil {
			payload.Accounts[i].Extra = make(map[string]any, 1)
		}
		payload.Accounts[i].Extra["ye_team_card_code"] = req.CardCode
	}
	var accountOptions *AccountImportOptions
	if req.AccountOptions != nil {
		options := *req.AccountOptions
		options.GroupIDs = append([]int64(nil), req.AccountOptions.GroupIDs...)
		// CDK redemption is an explicit admin action, matching auto-supply's
		// trusted deployment path for mixed-channel group bindings.
		options.SkipMixedChannelCheck = true
		accountOptions = &options
	}
	result, err := h.importData(ctx, DataImportRequest{Data: normalized, SkipDefaultGroupBind: &skip, AccountOptions: accountOptions}, payload)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, YeTeamRedeemResponse{
		OrderNo: finalOrder.OrderNo, Status: finalOrder.Status, Action: action,
		AccountCreated: result.AccountCreated, AccountFailed: result.AccountFailed,
		ImportErrors: result.Errors,
	})
}

func newYeTeamRequestID() string {
	return uuid.New().String()
}

func nestedOrderString(raw map[string]any, keys ...string) string {
	for _, key := range keys {
		if value, ok := raw[key].(string); ok && strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	for _, value := range raw {
		if nested, ok := value.(map[string]any); ok {
			if found := nestedOrderString(nested, keys...); found != "" {
				return found
			}
		}
	}
	return ""
}
