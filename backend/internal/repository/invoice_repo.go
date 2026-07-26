package repository

import (
	"context"
	"fmt"
	"strings"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/ent/invoice"
	"github.com/Wei-Shaw/sub2api/ent/invoiceitem"
	"github.com/Wei-Shaw/sub2api/ent/paymentorder"
	"github.com/Wei-Shaw/sub2api/internal/domain"
	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
	"github.com/Wei-Shaw/sub2api/internal/service"

	entsql "entgo.io/ent/dialect/sql"
)

// 只有真正付过款的订单才能开票。RECHARGING / COMPLETED 也算已付款，它们是
// PAID 之后的下游状态。
var invoiceableOrderStatuses = []string{
	service.OrderStatusPaid,
	service.OrderStatusRecharging,
	service.OrderStatusCompleted,
}

// 占用订单的申请状态：待处理和已开具。被驳回或撤回的申请不占用。
var occupyingInvoiceStatuses = []string{
	domain.InvoiceStatusPending,
	domain.InvoiceStatusIssued,
}

type invoiceRepository struct {
	client *dbent.Client
}

func NewInvoiceRepository(client *dbent.Client) service.InvoiceRepository {
	return &invoiceRepository{client: client}
}

func (r *invoiceRepository) Create(ctx context.Context, inv *service.Invoice, orders []service.InvoiceableOrder) error {
	// 申请与条目必须一起落库：只写了申请却没写条目，会得到一条不知道该开哪些
	// 订单的申请；只写了条目则订单被永久占用。
	tx, err := r.client.Tx(ctx)
	if err != nil {
		return err
	}
	txCtx := dbent.NewTxContext(ctx, tx)

	committed := false
	defer func() {
		if !committed {
			_ = tx.Rollback()
		}
	}()

	builder := tx.Client().Invoice.Create().
		SetUserID(inv.UserID).
		SetInvoiceNo(inv.InvoiceNo).
		SetEntityType(inv.EntityType).
		SetTitle(inv.Title).
		SetDeliveryEmail(inv.DeliveryEmail).
		SetAmount(inv.Amount).
		SetStatus(inv.Status)

	if inv.TaxID != "" {
		builder.SetTaxID(inv.TaxID)
	}
	if inv.Notes != "" {
		builder.SetNotes(inv.Notes)
	}

	created, err := builder.Save(txCtx)
	if err != nil {
		return translatePersistenceError(err, nil, infraerrors.Conflict("INVOICE_CONFLICT", "invoice request conflict"))
	}

	bulk := make([]*dbent.InvoiceItemCreate, 0, len(orders))
	for _, o := range orders {
		itemBuilder := tx.Client().InvoiceItem.Create().
			SetInvoiceID(created.ID).
			SetOrderID(o.OrderID).
			SetDescription(o.Description).
			SetAmount(o.Amount).
			SetActive(true)
		if !o.CreatedAt.IsZero() {
			itemBuilder.SetOrderCreatedAt(o.CreatedAt)
		}
		bulk = append(bulk, itemBuilder)
	}

	if _, err := tx.Client().InvoiceItem.CreateBulk(bulk...).Save(txCtx); err != nil {
		// 命中 uniq_invoice_items_active_order 说明有订单已被别的申请占用，
		// 翻译成冲突错误交给 service 层转成用户可读的提示。
		return translatePersistenceError(
			err,
			nil,
			infraerrors.Conflict("INVOICE_ORDER_ALREADY_INVOICED", "order already covered by an invoice request"),
		)
	}

	if err := tx.Commit(); err != nil {
		return err
	}
	committed = true

	inv.ID = created.ID
	inv.CreatedAt = created.CreatedAt
	inv.UpdatedAt = created.UpdatedAt
	return nil
}

func (r *invoiceRepository) GetByID(ctx context.Context, id int64) (*service.Invoice, error) {
	client := clientFromContext(ctx, r.client)

	row, err := client.Invoice.Query().
		Where(invoice.IDEQ(id)).
		WithItems(func(q *dbent.InvoiceItemQuery) {
			q.Order(dbent.Asc(invoiceitem.FieldID))
		}).
		Only(ctx)
	if err != nil {
		return nil, translatePersistenceError(err, service.ErrInvoiceNotFound, nil)
	}

	return invoiceFromEnt(row), nil
}

func (r *invoiceRepository) List(
	ctx context.Context,
	params pagination.PaginationParams,
	filters service.InvoiceListFilters,
) ([]service.Invoice, *pagination.PaginationResult, error) {
	client := clientFromContext(ctx, r.client)

	query := client.Invoice.Query()
	if filters.UserID > 0 {
		query = query.Where(invoice.UserIDEQ(filters.UserID))
	}
	if status := strings.TrimSpace(filters.Status); status != "" {
		query = query.Where(invoice.StatusEQ(status))
	}
	if search := strings.TrimSpace(filters.Search); search != "" {
		// 用户记得的可能是平台申请编号，也可能是最终发票号码，两个都匹配。
		query = query.Where(invoice.Or(
			invoice.InvoiceNoContainsFold(search),
			invoice.IssuedInvoiceNoContainsFold(search),
		))
	}

	total, err := query.Clone().Count(ctx)
	if err != nil {
		return nil, nil, err
	}

	page := params.Page
	if page < 1 {
		page = 1
	}
	pageSize := params.PageSize
	if pageSize < 1 {
		pageSize = 20
	}

	rows, err := query.
		WithItems(func(q *dbent.InvoiceItemQuery) {
			q.Order(dbent.Asc(invoiceitem.FieldID))
		}).
		Order(dbent.Desc(invoice.FieldCreatedAt)).
		Offset((page - 1) * pageSize).
		Limit(pageSize).
		All(ctx)
	if err != nil {
		return nil, nil, err
	}

	out := make([]service.Invoice, 0, len(rows))
	for _, row := range rows {
		out = append(out, *invoiceFromEnt(row))
	}

	pages := 0
	if pageSize > 0 {
		pages = (total + pageSize - 1) / pageSize
	}

	return out, &pagination.PaginationResult{
		Total:    int64(total),
		Page:     page,
		PageSize: pageSize,
		Pages:    pages,
	}, nil
}

func (r *invoiceRepository) UpdateStatus(
	ctx context.Context,
	id int64,
	inv *service.Invoice,
	releaseItems bool,
) error {
	// 状态变更与条目释放要一起生效，否则可能出现「申请已驳回但订单仍被占用」
	// 这种用户无法自行恢复的状态。
	tx, err := r.client.Tx(ctx)
	if err != nil {
		return err
	}
	txCtx := dbent.NewTxContext(ctx, tx)

	committed := false
	defer func() {
		if !committed {
			_ = tx.Rollback()
		}
	}()

	builder := tx.Client().Invoice.UpdateOneID(id).
		SetStatus(inv.Status)

	if inv.IssuedInvoiceNo != "" {
		builder.SetIssuedInvoiceNo(inv.IssuedInvoiceNo)
	}
	if inv.IssuedFileURL != "" {
		builder.SetIssuedFileURL(inv.IssuedFileURL)
	}
	if inv.RejectReason != "" {
		builder.SetRejectReason(inv.RejectReason)
	}
	if inv.ReviewedAt != nil {
		builder.SetReviewedAt(*inv.ReviewedAt)
	}
	if inv.ReviewedBy > 0 {
		builder.SetReviewedBy(inv.ReviewedBy)
	}

	if err := builder.Exec(txCtx); err != nil {
		return translatePersistenceError(err, service.ErrInvoiceNotFound, nil)
	}

	if releaseItems {
		if _, err := tx.Client().InvoiceItem.Update().
			Where(invoiceitem.InvoiceIDEQ(id), invoiceitem.ActiveEQ(true)).
			SetActive(false).
			Save(txCtx); err != nil {
			return err
		}
	}

	if err := tx.Commit(); err != nil {
		return err
	}
	committed = true
	return nil
}

func (r *invoiceRepository) ListInvoiceableOrders(ctx context.Context, userID int64) ([]service.InvoiceableOrder, error) {
	return r.queryInvoiceableOrders(ctx, userID, nil)
}

func (r *invoiceRepository) LookupInvoiceableOrders(
	ctx context.Context,
	userID int64,
	orderIDs []int64,
) ([]service.InvoiceableOrder, error) {
	if len(orderIDs) == 0 {
		return nil, nil
	}
	return r.queryInvoiceableOrders(ctx, userID, orderIDs)
}

// queryInvoiceableOrders 取该用户已付款、未退款、且没有被占用中的申请覆盖的
// 订单。orderIDs 为空表示取全部。
func (r *invoiceRepository) queryInvoiceableOrders(
	ctx context.Context,
	userID int64,
	orderIDs []int64,
) ([]service.InvoiceableOrder, error) {
	client := clientFromContext(ctx, r.client)

	query := client.PaymentOrder.Query().
		Where(
			paymentorder.UserIDEQ(userID),
			paymentorder.StatusIn(invoiceableOrderStatuses...),
		)

	if len(orderIDs) > 0 {
		query = query.Where(paymentorder.IDIn(orderIDs...))
	}

	// 排除已被占用的订单。用子查询而非先查全部再内存过滤，避免订单量大时
	// 把整张表拉到应用层。
	query = query.Where(func(s *entsql.Selector) {
		items := entsql.Table(invoiceitem.Table)
		invoices := entsql.Table(invoice.Table)
		sub := entsql.Select(items.C(invoiceitem.FieldOrderID)).
			From(items).
			Join(invoices).
			On(items.C(invoiceitem.FieldInvoiceID), invoices.C(invoice.FieldID)).
			Where(entsql.And(
				entsql.EQ(items.C(invoiceitem.FieldActive), true),
				entsql.In(
					invoices.C(invoice.FieldStatus),
					toAnySlice(occupyingInvoiceStatuses)...,
				),
			))
		s.Where(entsql.NotIn(s.C(paymentorder.FieldID), sub))
	})

	rows, err := query.
		Order(dbent.Desc(paymentorder.FieldCreatedAt)).
		All(ctx)
	if err != nil {
		return nil, err
	}

	out := make([]service.InvoiceableOrder, 0, len(rows))
	for _, row := range rows {
		out = append(out, service.InvoiceableOrder{
			OrderID:     row.ID,
			OutTradeNo:  row.OutTradeNo,
			Description: describeOrder(row),
			Amount:      row.Amount,
			CreatedAt:   row.CreatedAt,
		})
	}
	return out, nil
}

// describeOrder 给开票条目一个人看得懂的名字。订单本身没有标题字段，所以按
// 类型给一个稳定说法，再带上商户订单号便于对账。
func describeOrder(row *dbent.PaymentOrder) string {
	label := "Balance recharge"
	if strings.EqualFold(row.OrderType, "subscription") {
		label = "Subscription"
	}
	if row.OutTradeNo == "" {
		return label
	}
	return fmt.Sprintf("%s (%s)", label, row.OutTradeNo)
}

func invoiceFromEnt(row *dbent.Invoice) *service.Invoice {
	out := &service.Invoice{
		ID:            row.ID,
		UserID:        row.UserID,
		InvoiceNo:     row.InvoiceNo,
		EntityType:    row.EntityType,
		Title:         row.Title,
		DeliveryEmail: row.DeliveryEmail,
		Amount:        row.Amount,
		Status:        row.Status,
		CreatedAt:     row.CreatedAt,
		UpdatedAt:     row.UpdatedAt,
	}

	if row.TaxID != nil {
		out.TaxID = *row.TaxID
	}
	if row.Notes != nil {
		out.Notes = *row.Notes
	}
	if row.IssuedInvoiceNo != nil {
		out.IssuedInvoiceNo = *row.IssuedInvoiceNo
	}
	if row.IssuedFileURL != nil {
		out.IssuedFileURL = *row.IssuedFileURL
	}
	if row.RejectReason != nil {
		out.RejectReason = *row.RejectReason
	}
	if row.ReviewedAt != nil {
		reviewedAt := *row.ReviewedAt
		out.ReviewedAt = &reviewedAt
	}
	if row.ReviewedBy != nil {
		out.ReviewedBy = *row.ReviewedBy
	}

	if items, err := row.Edges.ItemsOrErr(); err == nil {
		out.Items = make([]service.InvoiceItem, 0, len(items))
		for _, item := range items {
			entry := service.InvoiceItem{
				ID:          item.ID,
				OrderID:     item.OrderID,
				Description: item.Description,
				Amount:      item.Amount,
			}
			if item.OrderCreatedAt != nil {
				createdAt := *item.OrderCreatedAt
				entry.OrderCreatedAt = &createdAt
			}
			out.Items = append(out.Items, entry)
		}
	}

	return out
}

func toAnySlice(in []string) []any {
	out := make([]any, 0, len(in))
	for _, v := range in {
		out = append(out, v)
	}
	return out
}
