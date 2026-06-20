package admin

import (
	"strconv"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/service"

	"github.com/gin-gonic/gin"
)

// DailyCheckinHandler handles admin daily check-in records.
type DailyCheckinHandler struct {
	dailyCheckinService *service.DailyCheckinService
}

// NewDailyCheckinHandler creates a new admin daily check-in handler.
func NewDailyCheckinHandler(dailyCheckinService *service.DailyCheckinService) *DailyCheckinHandler {
	return &DailyCheckinHandler{dailyCheckinService: dailyCheckinService}
}

// List returns paginated daily check-in reward records.
// GET /api/v1/admin/daily-checkins
func (h *DailyCheckinHandler) List(c *gin.Context) {
	page, pageSize := response.ParsePagination(c)

	userID, err := parseOptionalPositiveInt64(c.Query("user_id"))
	if err != nil {
		response.BadRequest(c, "Invalid user_id")
		return
	}
	startDate, err := parseOptionalDate(c.Query("start_date"))
	if err != nil {
		response.BadRequest(c, "Invalid start_date format, use YYYY-MM-DD")
		return
	}
	endDate, err := parseOptionalDate(c.Query("end_date"))
	if err != nil {
		response.BadRequest(c, "Invalid end_date format, use YYYY-MM-DD")
		return
	}

	records, total, err := h.dailyCheckinService.AdminListRecords(c.Request.Context(), service.DailyCheckinAdminListFilter{
		Search:    c.Query("search"),
		UserID:    userID,
		StartDate: startDate,
		EndDate:   endDate,
		Page:      page,
		PageSize:  pageSize,
		SortBy:    c.DefaultQuery("sort_by", "created_at"),
		SortOrder: c.DefaultQuery("sort_order", "desc"),
	})
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Paginated(c, records, total, page, pageSize)
}

func parseOptionalPositiveInt64(raw string) (int64, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return 0, nil
	}
	value, err := strconv.ParseInt(raw, 10, 64)
	if err != nil {
		return 0, err
	}
	if value <= 0 {
		return 0, strconv.ErrSyntax
	}
	return value, nil
}

func parseOptionalDate(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", nil
	}
	if _, err := time.Parse("2006-01-02", raw); err != nil {
		return "", err
	}
	return raw, nil
}
