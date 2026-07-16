-- Add channel-level priority/fast service tier multiplier for token pricing.
ALTER TABLE channel_model_pricing
  ADD COLUMN IF NOT EXISTS priority_multiplier NUMERIC(10,4);

COMMENT ON COLUMN channel_model_pricing.priority_multiplier IS
  'priority/fast service tier multiplier; NULL preserves existing service tier pricing behavior';

ALTER TABLE channel_account_stats_model_pricing
  ADD COLUMN IF NOT EXISTS priority_multiplier NUMERIC(10,4);

COMMENT ON COLUMN channel_account_stats_model_pricing.priority_multiplier IS
  'priority/fast service tier multiplier snapshot for account stats pricing rules';
