-- GPT-Auto Phase 2.1: per-group switch and independent virtual-model pricing.
--
--   * groups.gpt_auto_enabled defaults to FALSE for existing and new groups.
--     The global live switch never enables a group; an administrator must
--     turn each group on explicitly.
--   * Live decision rows record the pricing identity and the immutable tariff
--     snapshot the request was charged with. The financial source of truth
--     stays usage_logs.actual_cost; these columns explain it and are never
--     recomputed from today's tariff. Historical rows keep empty / NULL
--     values (export: pricing_mode empty = recorded before Phase 2.1).
--   * Additive only; no row is rewritten.

SET LOCAL lock_timeout = '5s';
SET LOCAL statement_timeout = '10min';

ALTER TABLE groups
    ADD COLUMN IF NOT EXISTS gpt_auto_enabled BOOLEAN NOT NULL DEFAULT FALSE;

COMMENT ON COLUMN groups.gpt_auto_enabled IS
    'Whether API keys of this group may request the gpt-auto virtual model; default false, never enabled by the global GPT-Auto live switch';

ALTER TABLE gpt_auto_shadow_decisions
    ADD COLUMN IF NOT EXISTS pricing_model             VARCHAR(64)    NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS pricing_mode              VARCHAR(48)    NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS pricing_config_version    INTEGER,
    ADD COLUMN IF NOT EXISTS pricing_source            VARCHAR(32)    NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS tariff_input_per_mtok     NUMERIC(20,10),
    ADD COLUMN IF NOT EXISTS tariff_cached_per_mtok    NUMERIC(20,10),
    ADD COLUMN IF NOT EXISTS tariff_output_per_mtok    NUMERIC(20,10),
    ADD COLUMN IF NOT EXISTS tariff_cache_write_per_mtok NUMERIC(20,10);

COMMENT ON COLUMN gpt_auto_shadow_decisions.pricing_model IS 'Live: retail pricing identity (gpt-auto in virtual_model_fixed_rates mode, the executed model in legacy mode)';
COMMENT ON COLUMN gpt_auto_shadow_decisions.pricing_mode IS 'Live: legacy_executed_model_multiplier or virtual_model_fixed_rates, captured before execution';
COMMENT ON COLUMN gpt_auto_shadow_decisions.pricing_config_version IS 'Live: GPT-Auto pricing configuration version captured before execution';
COMMENT ON COLUMN gpt_auto_shadow_decisions.pricing_source IS 'Live, fixed rates: global (GPT-Auto tariff) or group_override (exact gpt-auto entry in the group model pricing)';
COMMENT ON COLUMN gpt_auto_shadow_decisions.tariff_input_per_mtok IS 'Live, fixed rates: base uncached input price (USD / 1M tokens) before group and user multipliers';
