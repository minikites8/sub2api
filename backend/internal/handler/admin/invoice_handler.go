package admin

import (
	"strconv"

	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	middleware2 "github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"

	"github.com/gin-gonic/gin"
)

// InvoiceHandler serves the operator side of the manual invoicing flow: read a
// request, issue the invoice offline in the tax system, then record the result
// here (or reject it with a reason).
type InvoiceHandler struct {
	invoiceService *service.InvoiceService
}

func NewInvoiceHandler(invoiceService *service.InvoiceService) *InvoiceHandler {
	return &InvoiceHandler{invoiceService: invoiceService}
}

type issueInvoiceBody struct {
	IssuedInvoiceNo string `json:"issued_invoice_no"`
	IssuedFileURL   string `json:"issued_file_url"`
}

type rejectInvoiceBody struct {
	Reason string `json:"reason"`
}

// List returns every invoice request, newest first.
// GET /admin/invoices
func (h *InvoiceHandler) List(c *gin.Context) {
	page, pageSize := response.ParsePagination(c)
	invoices, result, err := h.invoiceService.List(
		c.Request.Context(),
		pagination.PaginationParams{Page: page, PageSize: pageSize},
		service.InvoiceListFilters{
			Status: c.Query("status"),
			Search: c.Query("search"),
		},
	)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}

	response.Paginated(c, invoices, result.Total, result.Page, result.PageSize)
}

// Issue records the real invoice number after it has been issued offline.
// POST /admin/invoices/:id/issue
func (h *InvoiceHandler) Issue(c *gin.Context) {
	adminID, ok := currentAdminID(c)
	if !ok {
		response.Unauthorized(c, "admin not authenticated")
		return
	}

	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		response.BadRequest(c, "invalid invoice id")
		return
	}

	var body issueInvoiceBody
	if err := c.ShouldBindJSON(&body); err != nil {
		response.BadRequest(c, "invalid request body")
		return
	}

	invoice, err := h.invoiceService.Issue(c.Request.Context(), adminID, id, service.IssueInvoiceRequest{
		IssuedInvoiceNo: body.IssuedInvoiceNo,
		IssuedFileURL:   body.IssuedFileURL,
	})
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}

	response.Success(c, invoice)
}

// Reject turns the request down and frees its orders so the user can resubmit.
// POST /admin/invoices/:id/reject
func (h *InvoiceHandler) Reject(c *gin.Context) {
	adminID, ok := currentAdminID(c)
	if !ok {
		response.Unauthorized(c, "admin not authenticated")
		return
	}

	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		response.BadRequest(c, "invalid invoice id")
		return
	}

	var body rejectInvoiceBody
	if err := c.ShouldBindJSON(&body); err != nil {
		response.BadRequest(c, "invalid request body")
		return
	}

	invoice, err := h.invoiceService.Reject(c.Request.Context(), adminID, id, body.Reason)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}

	response.Success(c, invoice)
}

func currentAdminID(c *gin.Context) (int64, bool) {
	subject, ok := middleware2.GetAuthSubjectFromContext(c)
	if !ok || subject.UserID <= 0 {
		return 0, false
	}
	return subject.UserID, true
}
