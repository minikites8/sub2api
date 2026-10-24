-- GPT-Auto Phase 2: live routing decisions and audited exports.
--
--   * Additive only. Existing (Phase 1) rows keep mode = 'shadow' and empty /
--     NULL live columns; historical model choices (including the retired
--     gpt-5.6-sol) are never rewritten.
--   * Live rows describe a gpt-auto request that actually executed on
--     final_model / final_effort. Customer charge and upstream cost stay in
--     usage_logs (joined on request_id); they are not duplicated here.
--   * gpt_auto_export_audit records every export (who, scope, filters, rows),
--     never the exported data.

SET LOCAL lock_timeout = '5s';
SET LOCAL statement_timeout = '10min';

ALTER TABLE gpt_auto_shadow_decisions
    ADD COLUMN IF NOT EXISTS mode                 VARCHAR(8)       NOT NULL DEFAULT 'shadow',
    ADD COLUMN IF NOT EXISTS routing_reason       VARCHAR(32)      NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS fallback_reason      VARCHAR(64)      NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS affinity_outcome     VARCHAR(32)      NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS served_model         VARCHAR(128)     NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS upstream_account_id  BIGINT,
    ADD COLUMN IF NOT EXISTS request_status       VARCHAR(32)      NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS http_status          INTEGER,
    ADD COLUMN IF NOT EXISTS error_category       VARCHAR(64)      NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS retry_count          SMALLINT         NOT NULL DEFAULT 0,
    ADD COLUMN IF NOT EXISTS gateway_latency_ms   INTEGER,
    ADD COLUMN IF NOT EXISTS upstream_ttft_ms     INTEGER,
    ADD COLUMN IF NOT EXISTS total_latency_ms     INTEGER,
    ADD COLUMN IF NOT EXISTS billing_multiplier   NUMERIC(12,6),
    ADD COLUMN IF NOT EXISTS routing_weight       INTEGER,
    ADD COLUMN IF NOT EXISTS state_version        VARCHAR(32)      NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS classifier_schema    VARCHAR(32)      NOT NULL DEFAULT '';

COMMENT ON COLUMN gpt_auto_shadow_decisions.mode IS 'shadow (hypothetical recommendation, request ran unchanged) or live (gpt-auto request executed on final_model)';
COMMENT ON COLUMN gpt_auto_shadow_decisions.actual_model IS 'Model the customer requested (gpt-auto for live rows)';
COMMENT ON COLUMN gpt_auto_shadow_decisions.final_model IS 'Shadow: policy-adjusted hypothetical recommendation. Live: model actually executed';
COMMENT ON COLUMN gpt_auto_shadow_decisions.billing_multiplier IS 'Live: GPT-Auto profile multiplier applied to the customer charge (1 when disabled)';

CREATE TABLE IF NOT EXISTS gpt_auto_export_audit (
    id          BIGSERIAL    PRIMARY KEY,
    created_at  TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    actor_id    BIGINT       NOT NULL,
    scope       VARCHAR(16)  NOT NULL,
    format      VARCHAR(8)   NOT NULL,
    filters     JSONB        NOT NULL DEFAULT '{}',
    row_count   BIGINT       NOT NULL DEFAULT 0,
    truncated   BOOLEAN      NOT NULL DEFAULT false,
    status      VARCHAR(16)  NOT NULL DEFAULT '',
    duration_ms INTEGER      NOT NULL DEFAULT 0
);

COMMENT ON TABLE gpt_auto_export_audit IS 'Audit trail of GPT-Auto routing log exports (admin and customer); never contains exported rows';

CREATE INDEX IF NOT EXISTS idx_gpt_auto_export_audit_actor_created ON gpt_auto_export_audit (actor_id, created_at DESC);
