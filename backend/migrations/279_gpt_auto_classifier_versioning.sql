-- GPT-Auto Phase 1: classifier versioning and margin semantics.
--
--   * gpt_auto_classifier_configs keeps every effective Jev question set
--     (operator-authored instructions, criteria, model descriptions, state
--     format) content-addressed by hash. It never holds customer content.
--   * Each decision records the classifier hash it was asked with. Rows
--     written before this migration keep '' and used the v1 questions.
--   * The existing est_gross_margin / margin_below_target stay fully loaded
--     (they always included the Jev cost); the model-only margin and the
--     estimate quality are new. Existing rows keep NULL / false / ''.

SET LOCAL lock_timeout = '5s';
SET LOCAL statement_timeout = '10min';

CREATE TABLE IF NOT EXISTS gpt_auto_classifier_configs (
    hash            VARCHAR(64)  PRIMARY KEY,
    revision        INTEGER      NOT NULL DEFAULT 0,
    schema_version  VARCHAR(32)  NOT NULL DEFAULT '',
    snapshot        JSONB        NOT NULL,
    created_by      BIGINT,
    created_at      TIMESTAMPTZ  NOT NULL DEFAULT NOW()
);

COMMENT ON TABLE gpt_auto_classifier_configs IS 'Content-addressed GPT-Auto Jev question sets (admin-authored instructions and criteria only) referenced by shadow decisions';

CREATE INDEX IF NOT EXISTS idx_gpt_auto_classifier_configs_created ON gpt_auto_classifier_configs (created_at DESC);

ALTER TABLE gpt_auto_shadow_decisions
    ADD COLUMN IF NOT EXISTS classifier_config_hash     VARCHAR(64)      NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS est_model_gross_margin     DOUBLE PRECISION,
    ADD COLUMN IF NOT EXISTS model_margin_below_target  BOOLEAN          NOT NULL DEFAULT false,
    ADD COLUMN IF NOT EXISTS estimate_quality           VARCHAR(32)      NOT NULL DEFAULT '';

COMMENT ON COLUMN gpt_auto_shadow_decisions.est_gross_margin IS 'Hypothetical fully loaded gross margin (includes Jev cost); NULL when revenue is zero or token estimates are insufficient';
COMMENT ON COLUMN gpt_auto_shadow_decisions.est_model_gross_margin IS 'Hypothetical model-only gross margin (excludes Jev cost); NULL when not estimable';
COMMENT ON COLUMN gpt_auto_shadow_decisions.classifier_config_hash IS 'gpt_auto_classifier_configs.hash of the Jev questions used; empty before classifier versioning';
