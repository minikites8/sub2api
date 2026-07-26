package handler

import (
	"strconv"

	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	middleware2 "github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"

	"github.com/gin-gonic/gin"
)

// InvoiceHandler serves the user-facing invoice endpoints.
type InvoiceHandler struct {
	invoiceService *service.InvoiceService
}

func NewInvoiceHandler(invoiceService *service.InvoiceService) *InvoiceHandler {
	return &InvoiceHandler{invoiceService: invoiceService}
}

type createInvoiceBody struct {
	EntityType    string  `json:"entity_type"`
	Title         string  `json:"title"`
	TaxID         string  `json:"tax_id"`
	DeliveryEmail string  `json:"delivery_email"`
	Notes         string  `json:"notes"`
	OrderIDs      []int64 `json:"order_ids"`
}

// ListInvoiceableOrders returns the paid orders the user has not invoiced yet.
// GET /invoices/invoiceable-orders
func (h *InvoiceHandler) ListInvoiceableOrders(c *gin.Context) {
	subject, ok := middleware2.GetAuthSubjectFromContext(c)
	if !ok {
		response.Unauthorized(c, "user not authenticated")
		return
	}

	orders, err := h.invoiceService.ListInvoiceableOrders(c.Request.Context(), subject.UserID)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}

	response.Success(c, map[string]any{"orders": orders})
}

// List returns the user's own invoice requests.
// GET /invoices
func (h *InvoiceHandler) List(c *gin.Context) {
	subject, ok := middleware2.GetAuthSubjectFromContext(c)
	if !ok {
		response.Unauthorized(c, "user not authenticated")
		return
	}

	page, pageSize := response.ParsePagination(c)
	invoices, result, err := h.invoiceService.List(
		c.Request.Context(),
		pagination.PaginationParams{Page: page, PageSize: pageSize},
		service.InvoiceListFilters{
			UserID: subject.UserID,
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

// Create submits a new invoice request.
// POST /invoices
func (h *InvoiceHandler) Create(c *gin.Context) {
	subject, ok := middleware2.GetAuthSubjectFromContext(c)
	if !ok {
		response.Unauthorized(c, "user not authenticated")
		return
	}

	var body createInvoiceBody
	if err := c.ShouldBindJSON(&body); err != nil {
		response.BadRequest(c, "invalid request body")
		return
	}

	invoice, err := h.invoiceService.Create(c.Request.Context(), subject.UserID, service.CreateInvoiceRequest{
		EntityType:    body.EntityType,
		Title:         body.Title,
		TaxID:         body.TaxID,
		DeliveryEmail: body.DeliveryEmail,
		Notes:         body.Notes,
		OrderIDs:      body.OrderIDs,
	})
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}

	response.Success(c, invoice)
}

// Get returns one of the user's invoice requests.
// GET /invoices/:id
func (h *InvoiceHandler) Get(c *gin.Context) {
	subject, ok := middleware2.GetAuthSubjectFromContext(c)
	if !ok {
		response.Unauthorized(c, "user not authenticated")
		return
	}

	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		response.BadRequest(c, "invalid invoice id")
		return
	}

	invoice, err := h.invoiceService.GetForUser(c.Request.Context(), subject.UserID, id)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}

	response.Success(c, invoice)
}

// Cancel withdraws a pending invoice request, freeing its orders.
// POST /invoices/:id/cancel
func (h *InvoiceHandler) Cancel(c *gin.Context) {
	subject, ok := middleware2.GetAuthSubjectFromContext(c)
	if !ok {
		response.Unauthorized(c, "user not authenticated")
		return
	}

	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		response.BadRequest(c, "invalid invoice id")
		return
	}

	if err := h.invoiceService.Cancel(c.Request.Context(), subject.UserID, id); err != nil {
		response.ErrorFrom(c, err)
		return
	}

	response.Success(c, map[string]any{"cancelled": true})
}
