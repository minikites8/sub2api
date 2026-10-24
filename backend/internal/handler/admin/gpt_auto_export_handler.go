package admin

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
)

// GPTAutoExportFormat reads the requested export format (default csv).
func GPTAutoExportFormat(c *gin.Context) string {
	return strings.ToLower(strings.TrimSpace(c.DefaultQuery("format", service.GPTAutoExportFormatCSV)))
}

// parseGPTAutoExportFilter reads the admin export filters. Every filter is
// applied server-side; nothing depends on what the page has loaded.
func parseGPTAutoExportFilter(c *gin.Context) (service.GPTAutoExportFilter, bool) {
	start, ok := parseGPTAutoTime(c, "start_time")
	if !ok {
		return service.GPTAutoExportFilter{}, false
	}
	end, ok := parseGPTAutoTime(c, "end_time")
	if !ok {
		return service.GPTAutoExportFilter{}, false
	}
	groupID, ok := parseGPTAutoGroupID(c)
	if !ok {
		return service.GPTAutoExportFilter{}, false
	}
	filter := service.GPTAutoExportFilter{
		Mode:             strings.TrimSpace(c.Query("mode")),
		GroupID:          groupID,
		RequestedModel:   strings.TrimSpace(c.Query("requested_model")),
		JevModel:         strings.TrimSpace(c.Query("jev_model")),
		FinalModel:       strings.TrimSpace(c.Query("final_model")),
		FinalEffort:      strings.TrimSpace(c.Query("final_effort")),
		Category:         strings.TrimSpace(c.Query("task_category")),
		RequestStatus:    strings.TrimSpace(c.Query("request_status")),
		EvaluationStatus: strings.TrimSpace(c.Query("evaluation_status")),
		FallbackOnly:     c.Query("fallback_only") == "true",
		ClassifierSchema: strings.TrimSpace(c.Query("classifier_schema")),
		StateVersion:     strings.TrimSpace(c.Query("state_version")),
		PolicyVersion:    strings.TrimSpace(c.Query("policy_version")),
	}
	if end != nil {
		filter.EndTime = end.UTC()
	} else {
		filter.EndTime = service.GPTAutoNow().UTC()
	}
	if start != nil {
		filter.StartTime = start.UTC()
	} else {
		filter.StartTime = filter.EndTime.AddDate(0, 0, -7)
	}
	for key, dest := range map[string]**float64{
		"complexity_min": &filter.ComplexityMin, "complexity_max": &filter.ComplexityMax,
		"confidence_min": &filter.ConfidenceMin, "confidence_max": &filter.ConfidenceMax,
	} {
		raw := strings.TrimSpace(c.Query(key))
		if raw == "" {
			continue
		}
		value, err := strconv.ParseFloat(raw, 64)
		if err != nil {
			response.BadRequest(c, "Invalid "+key)
			return service.GPTAutoExportFilter{}, false
		}
		*dest = &value
	}
	if raw := strings.TrimSpace(c.Query("upstream_account_id")); raw != "" {
		id, err := strconv.ParseInt(raw, 10, 64)
		if err != nil || id <= 0 {
			response.BadRequest(c, "Invalid upstream_account_id")
			return service.GPTAutoExportFilter{}, false
		}
		filter.UpstreamAccount = &id
	}
	return filter, true
}

// CountExport previews an admin export (row count, truncation).
// GET /api/v1/admin/gpt-auto/decisions/export/count
func (h *GPTAutoHandler) CountExport(c *gin.Context) {
	filter, ok := parseGPTAutoExportFilter(c)
	if !ok {
		return
	}
	summary, err := h.shadowService.CountExport(c.Request.Context(), service.GPTAutoExportScopeAdmin, filter)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, summary)
}

// ExportDecisions streams shadow and/or live decisions as CSV or JSONL.
// GET /api/v1/admin/gpt-auto/decisions/export
func (h *GPTAutoHandler) ExportDecisions(c *gin.Context) {
	filter, ok := parseGPTAutoExportFilter(c)
	if !ok {
		return
	}
	actor := int64(0)
	if id := gptAutoAdminID(c); id != nil {
		actor = *id
	}
	StreamGPTAutoExport(c, h.shadowService, service.GPTAutoExportRequest{
		Scope: service.GPTAutoExportScopeAdmin, Format: GPTAutoExportFormat(c), ActorID: actor, Filter: filter,
	})
}

// StreamGPTAutoExport validates, reserves a slot and streams an export.
// Validation errors are returned as JSON before any byte of the file is
// written; once streaming has started a failure truncates the download.
func StreamGPTAutoExport(c *gin.Context, svc *service.GPTAutoShadowService, req service.GPTAutoExportRequest) {
	ctx := c.Request.Context()
	release, err := svc.PrepareExport(ctx, &req)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	defer release()
	contentType := "text/csv; charset=utf-8"
	extension := "csv"
	if req.Format == service.GPTAutoExportFormatJSONL {
		contentType, extension = "application/x-ndjson; charset=utf-8", "jsonl"
	}
	// The file name is built only from fixed parts and server-formatted
	// timestamps, never from request input.
	name := "gpt-auto-routing-" + req.Scope + "-" + req.Filter.StartTime.UTC().Format("20060102T150405Z") +
		"-" + req.Filter.EndTime.UTC().Format("20060102T150405Z") + "." + extension
	header := c.Writer.Header()
	header.Set("Content-Type", contentType)
	header.Set("Content-Disposition", `attachment; filename="`+name+`"`)
	header.Set("Cache-Control", "no-store")
	header.Set("X-Content-Type-Options", "nosniff")
	header.Set("X-GPT-Auto-Export-Schema", service.GPTAutoExportSchemaVersion)
	header.Set("Trailer", "X-GPT-Auto-Export-Rows, X-GPT-Auto-Export-Truncated, X-GPT-Auto-Export-Status")
	c.Status(http.StatusOK)
	flusher, _ := c.Writer.(http.Flusher)
	req.Flush = func() {
		if flusher != nil {
			flusher.Flush()
		}
	}
	summary, err := svc.Export(ctx, req, c.Writer)
	header.Set("X-GPT-Auto-Export-Rows", strconv.FormatInt(summary.Rows, 10))
	header.Set("X-GPT-Auto-Export-Truncated", strconv.FormatBool(summary.Truncated))
	status := "completed"
	if err != nil {
		status = "failed"
	}
	header.Set("X-GPT-Auto-Export-Status", status)
}
