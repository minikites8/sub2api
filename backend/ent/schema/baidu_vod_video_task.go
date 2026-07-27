package schema

import (
	"time"

	"entgo.io/ent"
	"entgo.io/ent/dialect"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

type BaiduVODVideoTask struct {
	ent.Schema
}

func (BaiduVODVideoTask) Annotations() []schema.Annotation {
	return []schema.Annotation{entsql.Annotation{Table: "baidu_vod_video_tasks"}}
}

func (BaiduVODVideoTask) Fields() []ent.Field {
	decimal := map[string]string{dialect.Postgres: "decimal(20,10)"}
	timestamp := map[string]string{dialect.Postgres: "timestamptz"}
	return []ent.Field{
		field.String("platform").MaxLen(32).Default("baidu_vod").Immutable(),
		field.String("provider").MaxLen(32).Default("happyhorse").Immutable(),
		field.String("task_id").MaxLen(128).Immutable(),
		field.String("upstream_task_id").MaxLen(128).Immutable(),
		field.String("upstream_request_id").MaxLen(128).Optional().Nillable(),
		field.Int64("user_id").Immutable(),
		field.Int64("api_key_id").Immutable(),
		field.Int64("account_id").Immutable(),
		field.Int64("group_id").Optional().Nillable().Immutable(),
		field.String("model").MaxLen(128).Immutable(),
		field.String("upstream_model").MaxLen(128).Immutable(),
		field.String("capability").MaxLen(32).Immutable(),
		field.String("status").MaxLen(32).Default("queued"),
		field.String("upstream_status").MaxLen(32).Default("PENDING"),
		field.String("resolution").MaxLen(16).Immutable(),
		field.String("ratio").MaxLen(32).Default("16:9").Immutable(),
		field.Int("requested_duration").Immutable(),
		field.Int("output_duration").Default(0),
		field.Int("input_video_duration").Default(0),
		field.Int("video_count").Default(1),
		field.Float("estimated_cost").SchemaType(decimal).Default(0).Immutable(),
		field.Float("hold_amount").SchemaType(decimal).Default(0).Immutable(),
		field.Float("actual_cost").SchemaType(decimal).Optional().Nillable(),
		field.Float("group_rate_multiplier").SchemaType(decimal).Default(1).Immutable(),
		field.Float("video_rate_multiplier").SchemaType(decimal).Default(1).Immutable(),
		field.Float("account_rate_multiplier").SchemaType(decimal).Default(1).Immutable(),
		field.String("request_hash").MaxLen(128).Immutable(),
		field.String("result_url").SchemaType(map[string]string{dialect.Postgres: "text"}).Optional().Nillable(),
		field.Time("result_expires_at").SchemaType(timestamp).Optional().Nillable(),
		field.String("last_error_code").MaxLen(128).Optional().Nillable(),
		field.String("last_error_message").SchemaType(map[string]string{dialect.Postgres: "text"}).Optional().Nillable(),
		field.Int("retry_count").Default(0),
		field.Int("version").Default(0),
		field.Time("next_poll_at").SchemaType(timestamp).Default(time.Now),
		field.Time("poll_claimed_until").SchemaType(timestamp).Optional().Nillable(),
		field.Time("last_polled_at").SchemaType(timestamp).Optional().Nillable(),
		field.Time("created_at").SchemaType(timestamp).Default(time.Now).Immutable(),
		field.Time("updated_at").SchemaType(timestamp).Default(time.Now).UpdateDefault(time.Now),
		field.Time("submitted_at").SchemaType(timestamp).Default(time.Now).Immutable(),
		field.Time("started_at").SchemaType(timestamp).Optional().Nillable(),
		field.Time("finished_at").SchemaType(timestamp).Optional().Nillable(),
		field.Time("settled_at").SchemaType(timestamp).Optional().Nillable(),
	}
}

func (BaiduVODVideoTask) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("platform", "task_id").Unique(),
		index.Fields("platform", "upstream_task_id").
			Unique().
			Annotations(entsql.IndexWhere("upstream_task_id <> ''")),
		index.Fields("user_id", "api_key_id", "created_at"),
		index.Fields("status", "next_poll_at"),
		index.Fields("account_id", "status"),
		index.Fields("result_expires_at"),
	}
}
