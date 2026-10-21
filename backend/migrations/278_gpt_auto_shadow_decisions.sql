-- GPT-Auto Phase 1 shadow routing storage.
--
-- Design constraints:
--   * Shadow mode only: rows describe hypothetical recommendations and never
--     feed billing, scheduling or request rewriting.
--   * No raw prompts, request bodies or credentials are stored. Classifier
--     input is reduced to structured features; Jev output and the policy
--     decision are stored as JSON so the policy can be replayed offline.
--   * Append-only with retention-based batch deletion (decision_retention_days).
--   * policy_snapshot_hash points at gpt_auto_policy_snapshots, which keeps the
--     exact model profiles a decision was evaluated against.

SET LOCAL lock_timeout = '5s';
SET LOCAL statement_timeout = '10min';

CREATE TABLE IF NOT EXISTS gpt_auto_policy_snapshots (
    hash            VARCHAR(64)  PRIMARY KEY,
    policy_version  VARCHAR(64)  NOT NULL,
    snapshot        JSONB        NOT NULL,
    created_at      TIMESTAMPTZ  NOT NULL DEFAULT NOW()
);

COMMENT ON TABLE gpt_auto_policy_snapshots IS 'Content-addressed GPT-Auto policy inputs (model profiles, margins) referenced by shadow decisions';

CREATE TABLE IF NOT EXISTS gpt_auto_shadow_decisions (
    id                        BIGSERIAL      PRIMARY KEY,
    created_at                TIMESTAMPTZ    NOT NULL DEFAULT NOW(),
    request_id                VARCHAR(128)   NOT NULL DEFAULT '',
    user_id                   BIGINT,
    api_key_id                BIGINT,
    group_id                  BIGINT,
    endpoint                  VARCHAR(32)    NOT NULL DEFAULT '',
    actual_model              VARCHAR(128)   NOT NULL DEFAULT '',
    actual_effort             VARCHAR(16)    NOT NULL DEFAULT '',
    stream                    BOOLEAN        NOT NULL DEFAULT false,
    has_tools                 BOOLEAN        NOT NULL DEFAULT false,
    evaluation_status         VARCHAR(16)    NOT NULL,
    error_class               VARCHAR(32)    NOT NULL DEFAULT '',
    decision_status           VARCHAR(32)    NOT NULL DEFAULT '',
    jev_model                 VARCHAR(64)    NOT NULL DEFAULT '',
    jev_effort                VARCHAR(16)    NOT NULL DEFAULT '',
    final_model               VARCHAR(64)    NOT NULL DEFAULT '',
    final_effort              VARCHAR(16)    NOT NULL DEFAULT '',
    complexity                DOUBLE PRECISION,
    category                  VARCHAR(32)    NOT NULL DEFAULT '',
    model_confidence          DOUBLE PRECISION,
    effort_confidence         DOUBLE PRECISION,
    classifier_model          VARCHAR(64)    NOT NULL DEFAULT '',
    classifier_input_tokens   INTEGER        NOT NULL DEFAULT 0,
    classifier_output_tokens  INTEGER        NOT NULL DEFAULT 0,
    classifier_cost           NUMERIC(20,10) NOT NULL DEFAULT 0,
    classifier_latency_ms     INTEGER        NOT NULL DEFAULT 0,
    queue_delay_ms            INTEGER        NOT NULL DEFAULT 0,
    attempts                  SMALLINT       NOT NULL DEFAULT 0,
    est_revenue               NUMERIC(20,10),
    est_upstream_cost         NUMERIC(20,10),
    est_gross_profit          NUMERIC(20,10),
    est_gross_margin          DOUBLE PRECISION,
    margin_below_target       BOOLEAN        NOT NULL DEFAULT false,
    policy_version            VARCHAR(64)    NOT NULL DEFAULT '',
    policy_snapshot_hash      VARCHAR(64)    NOT NULL DEFAULT '',
    features                  JSONB          NOT NULL DEFAULT '{}',
    classification            JSONB,
    decision                  JSONB
);

COMMENT ON TABLE gpt_auto_shadow_decisions IS 'GPT-Auto Phase 1 shadow decisions: hypothetical model/effort recommendations, never applied to live traffic';
COMMENT ON COLUMN gpt_auto_shadow_decisions.actual_model IS 'Model the customer actually requested (production behaviour)';
COMMENT ON COLUMN gpt_auto_shadow_decisions.final_model IS 'Policy-adjusted hypothetical recommendation';
COMMENT ON COLUMN gpt_auto_shadow_decisions.classifier_cost IS 'Estimated Jev cost in USD; internal overhead, never charged to customers';
COMMENT ON COLUMN gpt_auto_shadow_decisions.est_gross_margin IS 'Hypothetical gross margin estimate; NULL when revenue is zero';

CREATE INDEX IF NOT EXISTS idx_gpt_auto_decisions_created ON gpt_auto_shadow_decisions (created_at DESC, id DESC);
CREATE INDEX IF NOT EXISTS idx_gpt_auto_decisions_status_created ON gpt_auto_shadow_decisions (evaluation_status, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_gpt_auto_decisions_actual_created ON gpt_auto_shadow_decisions (actual_model, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_gpt_auto_decisions_final_created ON gpt_auto_shadow_decisions (final_model, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_gpt_auto_decisions_group_created ON gpt_auto_shadow_decisions (group_id, created_at DESC) WHERE group_id IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_gpt_auto_decisions_request_id ON gpt_auto_shadow_decisions (request_id) WHERE request_id <> '';
