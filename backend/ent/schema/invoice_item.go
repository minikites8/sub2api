package schema

import (
	"time"

	"entgo.io/ent"
	"entgo.io/ent/dialect"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// InvoiceItem 记录一次开票申请覆盖了哪些订单。
//
// 金额和描述在提交时快照，不做联表读取——订单后续退款或改动不应改变已提交
// 申请的内容。
//
// active 字段配合 migration 里的部分唯一索引使用：一个订单同时只能出现在一
// 条“占用中”的申请里（pending / issued）。申请被驳回或撤回时把 active 置为
// false，订单即可重新开票。用状态列而非删除行，是为了保留申请痕迹。
type InvoiceItem struct {
	ent.Schema
}

func (InvoiceItem) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{Table: "invoice_items"},
	}
}

func (InvoiceItem) Fields() []ent.Field {
	return []ent.Field{
		field.Int64("invoice_id"),
		field.Int64("order_id"),
		field.String("description").
			MaxLen(255).
			Default(""),
		field.Float("amount").
			SchemaType(map[string]string{dialect.Postgres: "decimal(20,8)"}).
			Default(0),
		field.Time("order_created_at").
			Optional().
			Nillable().
			SchemaType(map[string]string{dialect.Postgres: "timestamptz"}),
		field.Bool("active").
			Default(true),
		field.Time("created_at").
			Immutable().
			Default(time.Now).
			SchemaType(map[string]string{dialect.Postgres: "timestamptz"}),
	}
}

func (InvoiceItem) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("invoice", Invoice.Type).
			Ref("items").
			Field("invoice_id").
			Unique().
			Required(),
	}
}

func (InvoiceItem) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("invoice_id"),
		index.Fields("order_id"),
	}
}
