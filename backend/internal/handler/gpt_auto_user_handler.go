package handler

import (
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/handler/admin"
	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	middleware2 "github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
)

// GPTAutoUserHandler serves a customer's own GPT-Auto routing history. Every
// query is scoped server-side to the authenticated user and to live rows;
// no client-supplied user or key id is ever trusted for ownership.
type GPTAutoUserHandler struct {
	svc *service.GPTAutoShadowService
}

// NewGPTAutoUserHandler creates the customer GPT-Auto handler.
func NewGPTAutoUserHandler(svc *service.GPTAutoShadowService) *GPTAutoUserHandler {
	return &GPTAutoUserHandler{svc: svc}
}

func (h *GPTAutoUserHandler) userID(c *gin.Context) (int64, bool) {
	subject, ok := middleware2.GetAuthSubjectFromContext(c)
	if !ok || subject.UserID <= 0 {
		response.Unauthorized(c, "User not authenticated")
		return 0, false
	}
	return subject.UserID, true
}

// Status reports whether self-service export is enabled.
// GET /api/v1/gpt-auto/status
func (h *GPTAutoUserHandler) Status(c *gin.Context) {
	userID, ok := h.userID(c)
	if !ok {
		return
	}
	response.Success(c, gin.H{
		"has_history":           h.svc.CustomerHasHistory(c.Request.Context(), userID),
		"virtual_model":         service.GPTAutoVirtualModel,
		"export_enabled":        h.svc.CustomerExportEnabled(c.Request.Context()),
		"export_schema_version": service.GPTAutoExportSchemaVersion,
	})
}

// List pages the caller's live gpt-auto decisions.
// GET /api/v1/gpt-auto/routing-logs
func (h *GPTAutoUserHandler) List(c *gin.Context) {
	userID, ok := h.userID(c)
	if !ok {
		return
	}
	page, pageSize := response.ParsePagination(c)
	if pageSize > 100 {
		pageSize = 100
	}
	start, end, ok := parseGPTAutoExportWindow(c, service.GPTAutoExportScopeCustomer)
	if !ok {
		return
	}
	window := service.GPTAutoExportFilter{StartTime: start, EndTime: end, UserID: &userID}
	if err := service.ValidateGPTAutoExportFilter(service.GPTAutoExportScopeCustomer, &window); err != nil {
		response.ErrorFrom(c, err)
		return
	}
	filter := service.GPTAutoDecisionFilter{StartTime: &start, EndTime: &end}
	filter.Pagination.Page, filter.Pagination.PageSize = page, pageSize
	items, total, err := h.svc.ListCustomerLogs(c.Request.Context(), userID, filter)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Paginated(c, items, total, page, pageSize)
}

func (h *GPTAutoUserHandler) exportFilter(c *gin.Context, userID int64) (service.GPTAutoExportFilter, bool) {
	start, end, ok := parseGPTAutoExportWindow(c, service.GPTAutoExportScopeCustomer)
	if !ok {
		return service.GPTAutoExportFilter{}, false
	}
	return service.GPTAutoExportFilter{
		StartTime:     start,
		EndTime:       end,
		Mode:          service.GPTAutoModeLive,
		RequestStatus: strings.TrimSpace(c.Query("request_status")),
		UserID:        &userID,
	}, true
}

// CountExport previews the row count.
// GET /api/v1/gpt-auto/routing-logs/export/count
func (h *GPTAutoUserHandler) CountExport(c *gin.Context) {
	userID, ok := h.userID(c)
	if !ok {
		return
	}
	filter, ok := h.exportFilter(c, userID)
	if !ok {
		return
	}
	summary, err := h.svc.CountExport(c.Request.Context(), service.GPTAutoExportScopeCustomer, filter)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, summary)
}

// Export streams the caller's routing history as CSV or JSONL.
// GET /api/v1/gpt-auto/routing-logs/export
func (h *GPTAutoUserHandler) Export(c *gin.Context) {
	userID, ok := h.userID(c)
	if !ok {
		return
	}
	filter, ok := h.exportFilter(c, userID)
	if !ok {
		return
	}
	admin.StreamGPTAutoExport(c, h.svc, service.GPTAutoExportRequest{
		Scope: service.GPTAutoExportScopeCustomer, Format: gptAutoExportFormat(c), ActorID: userID, Filter: filter,
	})
}

// parseGPTAutoExportWindow reads start_time/end_time (RFC3339, UTC). The
// default is the last 7 days.
func parseGPTAutoExportWindow(c *gin.Context, _ string) (time.Time, time.Time, bool) {
	end := time.Now().UTC()
	start := end.Add(-7 * 24 * time.Hour)
	if raw := strings.TrimSpace(c.Query("end_time")); raw != "" {
		parsed, err := time.Parse(time.RFC3339, raw)
		if err != nil {
			response.BadRequest(c, "Invalid end_time, expect RFC3339")
			return time.Time{}, time.Time{}, false
		}
		end = parsed.UTC()
		start = end.Add(-7 * 24 * time.Hour)
	}
	if raw := strings.TrimSpace(c.Query("start_time")); raw != "" {
		parsed, err := time.Parse(time.RFC3339, raw)
		if err != nil {
			response.BadRequest(c, "Invalid start_time, expect RFC3339")
			return time.Time{}, time.Time{}, false
		}
		start = parsed.UTC()
	}
	return start, end, true
}

func gptAutoExportFormat(c *gin.Context) string {
	return admin.GPTAutoExportFormat(c)
}
