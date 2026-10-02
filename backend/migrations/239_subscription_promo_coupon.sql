-- Extend promo codes with an explicit coupon type and subscription payment ratio.
-- Existing rows remain registration coupons through the default value.
ALTER TABLE promo_codes
    ADD COLUMN IF NOT EXISTS coupon_type VARCHAR(20) NOT NULL DEFAULT 'registration';

ALTER TABLE promo_codes
    ADD COLUMN IF NOT EXISTS subscription_discount_percent DECIMAL(5,2);

UPDATE promo_codes
SET coupon_type = 'registration'
WHERE coupon_type IS NULL OR BTRIM(coupon_type) = '';

DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1
        FROM pg_constraint
        WHERE conname = 'promo_codes_coupon_type_valid'
    ) THEN
        ALTER TABLE promo_codes
            ADD CONSTRAINT promo_codes_coupon_type_valid
            CHECK (coupon_type IN ('registration', 'subscription'));
    END IF;

    IF NOT EXISTS (
        SELECT 1
        FROM pg_constraint
        WHERE conname = 'promo_codes_subscription_discount_range'
    ) THEN
        ALTER TABLE promo_codes
            ADD CONSTRAINT promo_codes_subscription_discount_range
            CHECK (
                subscription_discount_percent IS NULL
                OR (subscription_discount_percent >= 0.01 AND subscription_discount_percent <= 100)
            );
    END IF;
END $$;

CREATE INDEX IF NOT EXISTS idx_promo_codes_coupon_type
    ON promo_codes(coupon_type);

COMMENT ON COLUMN promo_codes.coupon_type IS 'Coupon purpose: registration or subscription';
COMMENT ON COLUMN promo_codes.subscription_discount_percent IS 'Subscription payment ratio: 80 means the user pays 80 percent of the plan price';
