package schema

import (
	"time"

	"github.com/Wei-Shaw/sub2api/internal/domain"

	"entgo.io/ent"
	"entgo.io/ent/dialect"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// Invoice holds the schema definition for the Invoice entity.
//
// 一条 Invoice 是「一次开票申请」，不是发票本身。开票走人工流程：用户提交
// 抬头、税号、接收邮箱并勾选要开票的订单，管理员在税控系统线下开具后回填
// issued_invoice_no / issued_file_url。
//
// 删除策略：硬删除。开票申请属于财务凭据，正常流程里不删除；用户撤回用
// cancelled 状态而非删除，以便保留申请痕迹。
type Invoice struct {
	ent.Schema
}

func (Invoice) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{Table: "invoices"},
	}
}

func (Invoice) Fields() []ent.Field {
	return []ent.Field{
		field.Int64("user_id"),
		// 平台内部申请编号（如 INV-2026-A9X2B），用于用户查询与客服沟通。
		// 与管理员回填的真实发票号码 issued_invoice_no 是两回事。
		field.String("invoice_no").
			MaxLen(64).
			NotEmpty().
			Unique(),
		field.String("entity_type").
			MaxLen(20).
			Default(domain.InvoiceEntityCompany),
		// 发票抬头：企业为公司名称，个人为姓名。
		field.String("title").
			MaxLen(255).
			NotEmpty(),
		// 纳税人识别号 / VAT number。个人开票时为空。
		field.String("tax_id").
			MaxLen(64).
			Optional().
			Nillable(),
		field.String("delivery_email").
			MaxLen(255).
			NotEmpty(),
		field.String("notes").
			Optional().
			Nillable().
			SchemaType(map[string]string{dialect.Postgres: "text"}),
		// 申请金额，等于所选订单金额之和（下单币种，与 payment_orders.amount 同单位）。
		// 提交时快照，之后不随订单变化。
		field.Float("amount").
			SchemaType(map[string]string{dialect.Postgres: "decimal(20,8)"}).
			Default(0),
		field.String("status").
			MaxLen(20).
			Default(domain.InvoiceStatusPending),
		// 管理员线下开具后回填的真实发票号码。
		field.String("issued_invoice_no").
			MaxLen(128).
			Optional().
			Nillable(),
		// 发票文件地址（PDF / 图片）。人工流程下由管理员提供。
		field.String("issued_file_url").
			MaxLen(1024).
			Optional().
			Nillable(),
		field.String("reject_reason").
			Optional().
			Nillable().
			SchemaType(map[string]string{dialect.Postgres: "text"}),
		field.Int64("reviewed_by").
			Optional().
			Nillable(),
		field.Time("reviewed_at").
			Optional().
			Nillable().
			SchemaType(map[string]string{dialect.Postgres: "timestamptz"}),
		field.Time("created_at").
			Immutable().
			Default(time.Now).
			SchemaType(map[string]string{dialect.Postgres: "timestamptz"}),
		field.Time("updated_at").
			Default(time.Now).
			UpdateDefault(time.Now).
			SchemaType(map[string]string{dialect.Postgres: "timestamptz"}),
	}
}

func (Invoice) Edges() []ent.Edge {
	return []ent.Edge{
		edge.To("items", InvoiceItem.Type),
	}
}

func (Invoice) Indexes() []ent.Index {
	return []ent.Index{
		// 用户端列表：按自己的申请倒序翻页。
		index.Fields("user_id", "created_at"),
		// 管理端列表：先看待处理的。
		index.Fields("status", "created_at"),
	}
}
