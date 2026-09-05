ALTER TABLE recharge_discount_coupons
    DROP CONSTRAINT IF EXISTS recharge_discount_coupons_min_amount_positive;

ALTER TABLE recharge_discount_coupons
    DROP CONSTRAINT IF EXISTS recharge_discount_coupons_total_uses_positive;

ALTER TABLE recharge_discount_coupons
    ADD CONSTRAINT recharge_discount_coupons_min_amount_non_negative
        CHECK (min_recharge_amount >= 0);

ALTER TABLE recharge_discount_coupons
    ADD CONSTRAINT recharge_discount_coupons_total_uses_non_negative
        CHECK (total_uses >= 0);

ALTER TABLE recharge_discount_coupons
    ADD COLUMN IF NOT EXISTS source_type VARCHAR(20) NOT NULL DEFAULT 'admin';

ALTER TABLE recharge_discount_coupons
    ADD COLUMN IF NOT EXISTS source_id BIGINT;

ALTER TABLE recharge_discount_coupons
    ADD COLUMN IF NOT EXISTS source_code VARCHAR(32);

ALTER TABLE recharge_discount_coupons
    ADD CONSTRAINT recharge_discount_coupons_source_type_valid
        CHECK (source_type IN ('admin', 'promo_code'));

CREATE UNIQUE INDEX IF NOT EXISTS idx_recharge_discount_coupons_promo_source
    ON recharge_discount_coupons(user_id, source_type, source_id)
    WHERE source_type = 'promo_code' AND source_id IS NOT NULL;

INSERT INTO recharge_discount_coupons (
    user_id,
    min_recharge_amount,
    discount_percent,
    total_uses,
    status,
    created_by,
    notes,
    created_at,
    updated_at,
    source_type,
    source_id,
    source_code
)
SELECT
    pcu.user_id,
    0,
    pc.first_recharge_discount_percent,
    pc.first_recharge_discount_times,
    'active',
    0,
    NULL,
    pcu.used_at,
    NOW(),
    'promo_code',
    pc.id,
    pc.code
FROM promo_code_usages pcu
JOIN promo_codes pc ON pc.id = pcu.promo_code_id
WHERE pc.first_recharge_discount_percent > 0
  AND pc.first_recharge_discount_percent < 100
ON CONFLICT (user_id, source_type, source_id)
    WHERE source_type = 'promo_code' AND source_id IS NOT NULL
    DO NOTHING;

COMMENT ON COLUMN recharge_discount_coupons.total_uses IS 'Maximum uses; 0 means unlimited';
COMMENT ON COLUMN recharge_discount_coupons.source_type IS 'Coupon origin: admin or promo_code';
COMMENT ON COLUMN recharge_discount_coupons.source_id IS 'Origin record ID, such as the promo code ID';
COMMENT ON COLUMN recharge_discount_coupons.source_code IS 'Display code captured from the coupon origin';
COMMENT ON TABLE recharge_discount_coupons IS 'Balance recharge discount coupons issued by administrators or promo codes';
