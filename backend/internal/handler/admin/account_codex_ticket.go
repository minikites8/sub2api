package admin

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
)

type codexTicketProvider interface {
	OpenAICodexTicketStatuses(context.Context, *service.Account, time.Time) []service.OpenAICodexTicketStatus
	OpenAICodexTicketStatusBatch(context.Context, []int64) (map[int64][]service.OpenAICodexTicketStatus, error)
	OpenAICodexTicketLogs(context.Context, *service.Account, string, time.Time) (*service.OpenAICodexTicketLogs, error)
}

func (h *AccountHandler) SetCodexTicketProvider(provider codexTicketProvider) {
	h.codexTicketProvider = provider
}
func (h *AccountHandler) codexTicketStatuses(ctx context.Context, account *service.Account) []service.OpenAICodexTicketStatus {
	if h == nil || h.codexTicketProvider == nil || account == nil {
		return nil
	}
	return h.codexTicketProvider.OpenAICodexTicketStatuses(ctx, account, time.Now())
}
func (h *AccountHandler) GetCodexTicketLogs(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		response.BadRequest(c, "Invalid account ID")
		return
	}
	model := strings.TrimSpace(c.Query("model"))
	if model == "" || len(model) > 256 {
		response.BadRequest(c, "Invalid model")
		return
	}
	account, err := h.adminService.GetAccount(c.Request.Context(), id)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	if account == nil {
		response.NotFound(c, "Account not found")
		return
	}
	if !account.IsOpenAIOAuthLike() || account.IsShadow() {
		response.BadRequest(c, "Account does not support Codex tickets")
		return
	}
	if h.codexTicketProvider == nil {
		response.Error(c, http.StatusServiceUnavailable, "Codex ticket diagnostics unavailable")
		return
	}
	logs, err := h.codexTicketProvider.OpenAICodexTicketLogs(c.Request.Context(), account, model, time.Now())
	if errors.Is(err, service.ErrOpenAICodexTicketLogModel) {
		response.BadRequest(c, "Model is not configured for Codex tickets")
		return
	}
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, logs)
}
func (h *AccountHandler) GetBatchCodexTickets(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	var req struct {
		AccountIDs []int64 `json:"account_ids" binding:"required,max=200,dive,gt=0"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Supply up to 200 positive account IDs")
		return
	}
	ids := normalizeInt64IDList(req.AccountIDs)
	if h.codexTicketProvider == nil {
		response.Error(c, http.StatusServiceUnavailable, "Codex ticket diagnostics unavailable")
		return
	}
	snapshots, err := h.codexTicketProvider.OpenAICodexTicketStatusBatch(c.Request.Context(), ids)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, gin.H{"statuses": snapshots, "fetched_at": time.Now().UTC()})
}
