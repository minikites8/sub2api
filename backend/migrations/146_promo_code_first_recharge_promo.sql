ALTER TABLE promo_codes
    ADD COLUMN IF NOT EXISTS first_recharge_bonus_amount DECIMAL(20,8);

ALTER TABLE promo_codes
    ADD COLUMN IF NOT EXISTS first_recharge_discount_percent DECIMAL(5,2);

CREATE INDEX IF NOT EXISTS idx_promo_codes_first_recharge_promo
    ON promo_codes(id)
    WHERE first_recharge_bonus_amount IS NOT NULL
       OR first_recharge_discount_percent IS NOT NULL;

COMMENT ON COLUMN promo_codes.first_recharge_bonus_amount IS 'Bonus balance credited on the first paid balance recharge for users registered with this promo code';
COMMENT ON COLUMN promo_codes.first_recharge_discount_percent IS 'Payment percentage applied on the first paid balance recharge for users registered with this promo code';
