package service

import (
	"context"
	"crypto/rand"
	"fmt"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/domain"
	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
)

var (
	ErrInvoiceNotFound = infraerrors.NotFound("INVOICE_NOT_FOUND", "invoice request not found")
	ErrInvoiceNoOrders = infraerrors.BadRequest("INVOICE_NO_ORDERS", "select at least one order to invoice")
	// 订单已被另一条申请占用，或不属于当前用户，或状态不允许开票。对外统一成
	// 一个错误：具体是哪一种由前端刷新可开票列表后自然可见，没必要在错误里
	// 泄露其他用户的订单是否存在。
	ErrInvoiceOrdersNotInvoiceable = infraerrors.BadRequest(
		"INVOICE_ORDERS_NOT_INVOICEABLE",
		"some selected orders can no longer be invoiced",
	)
	ErrInvoiceTaxIDRequired = infraerrors.BadRequest(
		"INVOICE_TAX_ID_REQUIRED",
		"tax id is required when invoicing to a company",
	)
	ErrInvoiceNotPending = infraerrors.BadRequest(
		"INVOICE_NOT_PENDING",
		"only a pending invoice request can be updated",
	)
)

// Invoice 是一次开票申请。
type Invoice struct {
	ID              int64         `json:"id"`
	UserID          int64         `json:"user_id"`
	InvoiceNo       string        `json:"invoice_no"`
	EntityType      string        `json:"entity_type"`
	Title           string        `json:"title"`
	TaxID           string        `json:"tax_id,omitempty"`
	DeliveryEmail   string        `json:"delivery_email"`
	Notes           string        `json:"notes,omitempty"`
	Amount          float64       `json:"amount"`
	Status          string        `json:"status"`
	IssuedInvoiceNo string        `json:"issued_invoice_no,omitempty"`
	IssuedFileURL   string        `json:"issued_file_url,omitempty"`
	RejectReason    string        `json:"reject_reason,omitempty"`
	ReviewedAt      *time.Time    `json:"reviewed_at,omitempty"`
	CreatedAt       time.Time     `json:"created_at"`
	UpdatedAt       time.Time     `json:"updated_at"`
	Items           []InvoiceItem `json:"items,omitempty"`

	// 处理该申请的管理员。不出 JSON：用户看不到，管理端也没有展示需求，
	// 只作为审计字段落库。
	ReviewedBy int64 `json:"-"`

	// 管理端列表用，便于运营核对申请人。用户端不填。
	UserEmail string `json:"user_email,omitempty"`
}

// InvoiceItem 是申请覆盖的一笔订单，金额与描述在提交时快照。
type InvoiceItem struct {
	ID             int64      `json:"id"`
	OrderID        int64      `json:"order_id"`
	Description    string     `json:"description"`
	Amount         float64    `json:"amount"`
	OrderCreatedAt *time.Time `json:"order_created_at,omitempty"`
}

// InvoiceableOrder 是一笔可开票的订单：已支付、未退款、且没有被占用中的申请覆盖。
type InvoiceableOrder struct {
	OrderID     int64     `json:"order_id"`
	OutTradeNo  string    `json:"out_trade_no"`
	Description string    `json:"description"`
	Amount      float64   `json:"amount"`
	CreatedAt   time.Time `json:"created_at"`
}

type InvoiceListFilters struct {
	UserID int64  // 0 表示不限（管理端）
	Status string // 空表示不限
	Search string // 匹配平台申请编号或已开具的发票号码
}

type CreateInvoiceRequest struct {
	EntityType    string
	Title         string
	TaxID         string
	DeliveryEmail string
	Notes         string
	OrderIDs      []int64
}

type IssueInvoiceRequest struct {
	IssuedInvoiceNo string
	IssuedFileURL   string
}

type InvoiceRepository interface {
	// Create 在一个事务里写入申请与条目。若某个订单已被占用中的申请覆盖，
	// 依靠 invoice_items 上的部分唯一索引返回冲突错误。
	Create(ctx context.Context, inv *Invoice, orders []InvoiceableOrder) error
	GetByID(ctx context.Context, id int64) (*Invoice, error)
	List(ctx context.Context, params pagination.PaginationParams, filters InvoiceListFilters) ([]Invoice, *pagination.PaginationResult, error)
	// UpdateStatus 改状态并回填审核信息；releaseItems 为真时把条目置为
	// 非占用，让订单可以重新开票。
	UpdateStatus(ctx context.Context, id int64, inv *Invoice, releaseItems bool) error
	// ListInvoiceableOrders 返回该用户已支付、且未被占用中申请覆盖的订单。
	ListInvoiceableOrders(ctx context.Context, userID int64) ([]InvoiceableOrder, error)
	// LookupInvoiceableOrders 按 ID 取订单，并且只返回仍然可开票的那些。
	// 返回数量少于请求数量即说明有订单不可开票。
	LookupInvoiceableOrders(ctx context.Context, userID int64, orderIDs []int64) ([]InvoiceableOrder, error)
}

type InvoiceService struct {
	repo InvoiceRepository
}

func NewInvoiceService(repo InvoiceRepository) *InvoiceService {
	return &InvoiceService{repo: repo}
}

// ListInvoiceableOrders 给「申请开票」页填充可勾选的订单。
func (s *InvoiceService) ListInvoiceableOrders(ctx context.Context, userID int64) ([]InvoiceableOrder, error) {
	return s.repo.ListInvoiceableOrders(ctx, userID)
}

func (s *InvoiceService) List(
	ctx context.Context,
	params pagination.PaginationParams,
	filters InvoiceListFilters,
) ([]Invoice, *pagination.PaginationResult, error) {
	return s.repo.List(ctx, params, filters)
}

// GetForUser 读取一条申请，并确认属于该用户。
func (s *InvoiceService) GetForUser(ctx context.Context, userID, id int64) (*Invoice, error) {
	inv, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	// 不属于自己的申请一律按「不存在」处理，避免通过错误信息探测他人申请是否存在。
	if inv.UserID != userID {
		return nil, ErrInvoiceNotFound
	}
	return inv, nil
}

func (s *InvoiceService) Create(ctx context.Context, userID int64, req CreateInvoiceRequest) (*Invoice, error) {
	entityType := strings.TrimSpace(req.EntityType)
	if entityType != domain.InvoiceEntityIndividual {
		entityType = domain.InvoiceEntityCompany
	}

	title := strings.TrimSpace(req.Title)
	if title == "" {
		return nil, infraerrors.BadRequest("INVOICE_TITLE_REQUIRED", "invoice title is required")
	}

	email := strings.TrimSpace(req.DeliveryEmail)
	if email == "" {
		return nil, infraerrors.BadRequest("INVOICE_EMAIL_REQUIRED", "delivery email is required")
	}

	taxID := strings.TrimSpace(req.TaxID)
	// 企业发票必须有纳税人识别号，个人发票没有，提交了也忽略。
	if entityType == domain.InvoiceEntityCompany && taxID == "" {
		return nil, ErrInvoiceTaxIDRequired
	}
	if entityType == domain.InvoiceEntityIndividual {
		taxID = ""
	}

	orderIDs := dedupeInt64(req.OrderIDs)
	if len(orderIDs) == 0 {
		return nil, ErrInvoiceNoOrders
	}

	// 只接受仍然可开票、且属于该用户的订单。少一个就整体拒绝——宁可让用户
	// 刷新后重选，也不要静默开出一张金额与预期不符的发票。
	orders, err := s.repo.LookupInvoiceableOrders(ctx, userID, orderIDs)
	if err != nil {
		return nil, err
	}
	if len(orders) != len(orderIDs) {
		return nil, ErrInvoiceOrdersNotInvoiceable
	}

	var amount float64
	for _, o := range orders {
		amount += o.Amount
	}

	invoiceNo, err := generateInvoiceNo(time.Now())
	if err != nil {
		return nil, err
	}

	inv := &Invoice{
		UserID:        userID,
		InvoiceNo:     invoiceNo,
		EntityType:    entityType,
		Title:         title,
		TaxID:         taxID,
		DeliveryEmail: email,
		Notes:         strings.TrimSpace(req.Notes),
		Amount:        amount,
		Status:        domain.InvoiceStatusPending,
	}

	if err := s.repo.Create(ctx, inv, orders); err != nil {
		// 并发提交两份含同一订单的申请时，部分唯一索引会在这里挡下第二份。
		if infraerrors.IsConflict(err) {
			return nil, ErrInvoiceOrdersNotInvoiceable
		}
		return nil, err
	}

	return inv, nil
}

// Cancel 让用户撤回自己尚未处理的申请，订单随之释放。
func (s *InvoiceService) Cancel(ctx context.Context, userID, id int64) error {
	inv, err := s.GetForUser(ctx, userID, id)
	if err != nil {
		return err
	}
	if inv.Status != domain.InvoiceStatusPending {
		return ErrInvoiceNotPending
	}

	inv.Status = domain.InvoiceStatusCancelled
	return s.repo.UpdateStatus(ctx, id, inv, true)
}

// Issue 由管理员在税控系统线下开票后回填结果。
func (s *InvoiceService) Issue(ctx context.Context, adminID, id int64, req IssueInvoiceRequest) (*Invoice, error) {
	inv, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if inv.Status != domain.InvoiceStatusPending {
		return nil, ErrInvoiceNotPending
	}

	issuedNo := strings.TrimSpace(req.IssuedInvoiceNo)
	if issuedNo == "" {
		return nil, infraerrors.BadRequest(
			"INVOICE_ISSUED_NO_REQUIRED",
			"the issued invoice number is required",
		)
	}

	now := time.Now()
	inv.Status = domain.InvoiceStatusIssued
	inv.IssuedInvoiceNo = issuedNo
	inv.IssuedFileURL = strings.TrimSpace(req.IssuedFileURL)
	inv.ReviewedAt = &now
	inv.ReviewedBy = adminID

	// 已开具的申请继续占用订单，防止同一笔订单被重复开票。
	if err := s.repo.UpdateStatus(ctx, id, inv, false); err != nil {
		return nil, err
	}
	return inv, nil
}

// Reject 驳回申请并释放订单，用户修正后可重新提交。
func (s *InvoiceService) Reject(ctx context.Context, adminID, id int64, reason string) (*Invoice, error) {
	inv, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if inv.Status != domain.InvoiceStatusPending {
		return nil, ErrInvoiceNotPending
	}

	trimmed := strings.TrimSpace(reason)
	if trimmed == "" {
		return nil, infraerrors.BadRequest(
			"INVOICE_REJECT_REASON_REQUIRED",
			"a reason is required so the user knows what to fix",
		)
	}

	now := time.Now()
	inv.Status = domain.InvoiceStatusRejected
	inv.RejectReason = trimmed
	inv.ReviewedAt = &now
	inv.ReviewedBy = adminID

	if err := s.repo.UpdateStatus(ctx, id, inv, true); err != nil {
		return nil, err
	}
	return inv, nil
}

// generateInvoiceNo 生成平台申请编号，形如 INV-2026-7F3A9C。
// 年份便于运营按年归档，随机段用 crypto/rand，避免顺序编号暴露开票总量。
func generateInvoiceNo(now time.Time) (string, error) {
	const alphabet = "0123456789ABCDEFGHJKLMNPQRSTUVWXYZ" // 去掉容易混淆的 I 和 O
	buf := make([]byte, 6)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("generate invoice no: %w", err)
	}
	out := make([]byte, len(buf))
	for i, b := range buf {
		out[i] = alphabet[int(b)%len(alphabet)]
	}
	return fmt.Sprintf("INV-%d-%s", now.Year(), string(out)), nil
}

func dedupeInt64(in []int64) []int64 {
	if len(in) == 0 {
		return nil
	}
	seen := make(map[int64]struct{}, len(in))
	out := make([]int64, 0, len(in))
	for _, v := range in {
		if v <= 0 {
			continue
		}
		if _, ok := seen[v]; ok {
			continue
		}
		seen[v] = struct{}{}
		out = append(out, v)
	}
	return out
}
