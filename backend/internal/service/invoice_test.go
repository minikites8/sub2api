//go:build unit

package service

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/domain"
	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
)

// invoiceRepoStub 只记录调用，不做持久化：这里验证的是服务层的校验与状态流转。
type invoiceRepoStub struct {
	invoiceable []InvoiceableOrder
	// lookup 返回的订单。设为 nil 时退化为按 ID 过滤 invoiceable。
	lookup    []InvoiceableOrder
	createErr error

	stored        *Invoice
	storedOrders  []InvoiceableOrder
	byID          map[int64]*Invoice
	lastStatusID  int64
	lastStatusInv *Invoice
	lastRelease   bool
}

func (s *invoiceRepoStub) Create(_ context.Context, inv *Invoice, orders []InvoiceableOrder) error {
	if s.createErr != nil {
		return s.createErr
	}
	inv.ID = 1
	s.stored = inv
	s.storedOrders = orders
	return nil
}

func (s *invoiceRepoStub) GetByID(_ context.Context, id int64) (*Invoice, error) {
	if inv, ok := s.byID[id]; ok {
		clone := *inv
		return &clone, nil
	}
	return nil, ErrInvoiceNotFound
}

func (s *invoiceRepoStub) List(
	_ context.Context,
	_ pagination.PaginationParams,
	_ InvoiceListFilters,
) ([]Invoice, *pagination.PaginationResult, error) {
	return nil, &pagination.PaginationResult{}, nil
}

func (s *invoiceRepoStub) UpdateStatus(_ context.Context, id int64, inv *Invoice, releaseItems bool) error {
	s.lastStatusID = id
	clone := *inv
	s.lastStatusInv = &clone
	s.lastRelease = releaseItems
	return nil
}

func (s *invoiceRepoStub) ListInvoiceableOrders(_ context.Context, _ int64) ([]InvoiceableOrder, error) {
	return s.invoiceable, nil
}

func (s *invoiceRepoStub) LookupInvoiceableOrders(
	_ context.Context,
	_ int64,
	orderIDs []int64,
) ([]InvoiceableOrder, error) {
	if s.lookup != nil {
		return s.lookup, nil
	}
	wanted := make(map[int64]struct{}, len(orderIDs))
	for _, id := range orderIDs {
		wanted[id] = struct{}{}
	}
	out := make([]InvoiceableOrder, 0, len(orderIDs))
	for _, o := range s.invoiceable {
		if _, ok := wanted[o.OrderID]; ok {
			out = append(out, o)
		}
	}
	return out, nil
}

func baseCreateRequest() CreateInvoiceRequest {
	return CreateInvoiceRequest{
		EntityType:    domain.InvoiceEntityCompany,
		Title:         "Acme Corp LLC",
		TaxID:         "US123456789",
		DeliveryEmail: "billing@example.com",
		OrderIDs:      []int64{10, 11},
	}
}

func stubWithOrders() *invoiceRepoStub {
	return &invoiceRepoStub{
		invoiceable: []InvoiceableOrder{
			{OrderID: 10, Amount: 500, Description: "Balance recharge (TX-1)"},
			{OrderID: 11, Amount: 1250, Description: "Balance recharge (TX-2)"},
		},
	}
}

func TestInvoiceCreate_SumsSelectedOrderAmounts(t *testing.T) {
	repo := stubWithOrders()
	svc := NewInvoiceService(repo)

	inv, err := svc.Create(context.Background(), 7, baseCreateRequest())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if inv.Amount != 1750 {
		t.Fatalf("amount = %v, want 1750", inv.Amount)
	}
	if inv.Status != domain.InvoiceStatusPending {
		t.Fatalf("status = %q, want pending", inv.Status)
	}
	if inv.UserID != 7 {
		t.Fatalf("user id = %d, want 7", inv.UserID)
	}
	if len(repo.storedOrders) != 2 {
		t.Fatalf("stored %d order items, want 2", len(repo.storedOrders))
	}
}

// 少一个可开票订单就整体拒绝：宁可让用户刷新重选，也不要开出金额与预期不符的发票。
func TestInvoiceCreate_RejectsWhenAnOrderIsNoLongerInvoiceable(t *testing.T) {
	repo := stubWithOrders()
	repo.lookup = []InvoiceableOrder{{OrderID: 10, Amount: 500}}
	svc := NewInvoiceService(repo)

	_, err := svc.Create(context.Background(), 7, baseCreateRequest())
	if err == nil {
		t.Fatal("expected an error when a selected order cannot be invoiced")
	}
	if repo.stored != nil {
		t.Fatal("no invoice should be stored when validation fails")
	}
}

// 并发提交两份含同一订单的申请时，数据库的部分唯一索引会挡下第二份；
// 服务层要把这个冲突翻译成用户能理解的提示，而不是抛出原始约束错误。
func TestInvoiceCreate_TranslatesUniqueConstraintConflict(t *testing.T) {
	repo := stubWithOrders()
	repo.createErr = infraerrors.Conflict("INVOICE_ORDER_ALREADY_INVOICED", "already covered")
	svc := NewInvoiceService(repo)

	_, err := svc.Create(context.Background(), 7, baseCreateRequest())
	if err == nil {
		t.Fatal("expected the conflict to surface as an error")
	}
	if got := err.Error(); !strings.Contains(got, "no longer be invoiced") {
		t.Fatalf("error = %q, want the user-facing not-invoiceable message", got)
	}
}

func TestInvoiceCreate_RequiresTaxIDForCompanyButNotIndividual(t *testing.T) {
	svc := NewInvoiceService(stubWithOrders())

	company := baseCreateRequest()
	company.TaxID = "   "
	if _, err := svc.Create(context.Background(), 7, company); err == nil {
		t.Fatal("expected a company invoice without a tax id to be rejected")
	}

	individual := baseCreateRequest()
	individual.EntityType = domain.InvoiceEntityIndividual
	individual.TaxID = "should-be-dropped"
	inv, err := svc.Create(context.Background(), 7, individual)
	if err != nil {
		t.Fatalf("unexpected error for an individual invoice: %v", err)
	}
	if inv.TaxID != "" {
		t.Fatalf("tax id = %q, want it dropped for an individual invoice", inv.TaxID)
	}
}

func TestInvoiceCreate_RequiresAtLeastOneOrder(t *testing.T) {
	svc := NewInvoiceService(stubWithOrders())

	req := baseCreateRequest()
	// 重复项与非法 ID 去重后为空，等同于没有选择任何订单。
	req.OrderIDs = []int64{0, -3}
	if _, err := svc.Create(context.Background(), 7, req); err != ErrInvoiceNoOrders {
		t.Fatalf("error = %v, want ErrInvoiceNoOrders", err)
	}
}

func TestInvoiceCreate_DeduplicatesOrderIDs(t *testing.T) {
	repo := stubWithOrders()
	svc := NewInvoiceService(repo)

	req := baseCreateRequest()
	req.OrderIDs = []int64{10, 10, 11}

	inv, err := svc.Create(context.Background(), 7, req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// 去重后是 2 笔，金额不能把 10 号订单算两遍。
	if inv.Amount != 1750 {
		t.Fatalf("amount = %v, want 1750 with the duplicate collapsed", inv.Amount)
	}
}

// 别人的申请一律按不存在处理，避免通过错误信息探测他人申请。
func TestInvoiceGetForUser_HidesOtherUsersRequests(t *testing.T) {
	repo := &invoiceRepoStub{byID: map[int64]*Invoice{
		5: {ID: 5, UserID: 99, Status: domain.InvoiceStatusPending},
	}}
	svc := NewInvoiceService(repo)

	if _, err := svc.GetForUser(context.Background(), 7, 5); err != ErrInvoiceNotFound {
		t.Fatalf("error = %v, want ErrInvoiceNotFound", err)
	}
}

func TestInvoiceCancel_ReleasesOrdersOnlyWhilePending(t *testing.T) {
	repo := &invoiceRepoStub{byID: map[int64]*Invoice{
		5: {ID: 5, UserID: 7, Status: domain.InvoiceStatusPending},
		6: {ID: 6, UserID: 7, Status: domain.InvoiceStatusIssued},
	}}
	svc := NewInvoiceService(repo)

	if err := svc.Cancel(context.Background(), 7, 5); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !repo.lastRelease {
		t.Fatal("cancelling should release the covered orders")
	}
	if repo.lastStatusInv.Status != domain.InvoiceStatusCancelled {
		t.Fatalf("status = %q, want cancelled", repo.lastStatusInv.Status)
	}

	// 已开具的发票不能撤回，否则订单会被释放并可能重复开票。
	if err := svc.Cancel(context.Background(), 7, 6); err != ErrInvoiceNotPending {
		t.Fatalf("error = %v, want ErrInvoiceNotPending", err)
	}
}

// 已开具的申请必须继续占用订单，否则同一笔订单会被开两次票。
func TestInvoiceIssue_KeepsOrdersHeld(t *testing.T) {
	repo := &invoiceRepoStub{byID: map[int64]*Invoice{
		5: {ID: 5, UserID: 7, Status: domain.InvoiceStatusPending},
	}}
	svc := NewInvoiceService(repo)

	inv, err := svc.Issue(context.Background(), 42, 5, IssueInvoiceRequest{IssuedInvoiceNo: "0440001"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if repo.lastRelease {
		t.Fatal("issuing must not release the covered orders")
	}
	if inv.ReviewedBy != 42 {
		t.Fatalf("reviewed by = %d, want 42", inv.ReviewedBy)
	}
	if inv.ReviewedAt == nil {
		t.Fatal("reviewed at should be recorded")
	}
}

func TestInvoiceIssue_RequiresTheIssuedNumber(t *testing.T) {
	repo := &invoiceRepoStub{byID: map[int64]*Invoice{
		5: {ID: 5, UserID: 7, Status: domain.InvoiceStatusPending},
	}}
	svc := NewInvoiceService(repo)

	if _, err := svc.Issue(context.Background(), 42, 5, IssueInvoiceRequest{IssuedInvoiceNo: "  "}); err == nil {
		t.Fatal("expected issuing without an invoice number to be rejected")
	}
}

// 驳回要释放订单，用户改正抬头后才能重新提交。
func TestInvoiceReject_ReleasesOrdersAndRequiresAReason(t *testing.T) {
	repo := &invoiceRepoStub{byID: map[int64]*Invoice{
		5: {ID: 5, UserID: 7, Status: domain.InvoiceStatusPending},
	}}
	svc := NewInvoiceService(repo)

	if _, err := svc.Reject(context.Background(), 42, 5, "   "); err == nil {
		t.Fatal("expected a reject without a reason to be rejected")
	}

	inv, err := svc.Reject(context.Background(), 42, 5, "tax id does not match the company name")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !repo.lastRelease {
		t.Fatal("rejecting should release the covered orders so the user can retry")
	}
	if inv.RejectReason == "" {
		t.Fatal("reject reason should be recorded for the user to act on")
	}
}

func TestGenerateInvoiceNo_ShapeAndUniqueness(t *testing.T) {
	now := time.Date(2026, 7, 26, 0, 0, 0, 0, time.UTC)

	first, err := generateInvoiceNo(now)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.HasPrefix(first, "INV-2026-") {
		t.Fatalf("invoice no = %q, want an INV-<year>- prefix", first)
	}
	// 去掉了容易混淆的 I 和 O，避免用户抄错编号。
	suffix := strings.TrimPrefix(first, "INV-2026-")
	if strings.ContainsAny(suffix, "IO") {
		t.Fatalf("suffix %q should not contain the ambiguous letters I or O", suffix)
	}

	seen := map[string]struct{}{first: {}}
	for i := 0; i < 200; i++ {
		next, err := generateInvoiceNo(now)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		seen[next] = struct{}{}
	}
	// 随机段而非顺序编号，不应暴露开票总量；200 次里出现大量重复说明熵不足。
	if len(seen) < 190 {
		t.Fatalf("only %d unique numbers out of 201, suffix entropy looks too low", len(seen))
	}
}
