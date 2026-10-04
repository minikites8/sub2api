CREATE TABLE IF NOT EXISTS subscription_discount_coupons (
    id BIGSERIAL PRIMARY KEY,
    user_id BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    min_subscription_amount DECIMAL(20,8) NOT NULL,
    discount_percent DECIMAL(5,2) NOT NULL,
    total_uses INTEGER NOT NULL,
    status VARCHAR(20) NOT NULL DEFAULT 'active',
    created_by BIGINT NOT NULL,
    notes TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    source_type VARCHAR(20) NOT NULL DEFAULT 'admin',
    CONSTRAINT subscription_discount_coupons_min_amount_positive CHECK (min_subscription_amount > 0),
    CONSTRAINT subscription_discount_coupons_discount_range CHECK (discount_percent > 0 AND discount_percent < 100),
    CONSTRAINT subscription_discount_coupons_total_uses_positive CHECK (total_uses > 0),
    CONSTRAINT subscription_discount_coupons_status_valid CHECK (status IN ('active', 'revoked')),
    CONSTRAINT subscription_discount_coupons_source_valid CHECK (source_type = 'admin')
);

CREATE INDEX IF NOT EXISTS idx_subscription_discount_coupons_user_status
    ON subscription_discount_coupons(user_id, status);

COMMENT ON TABLE subscription_discount_coupons IS 'Subscription discounts issued directly to individual users';
COMMENT ON COLUMN subscription_discount_coupons.min_subscription_amount IS 'Minimum subscription plan price in USD, before currency conversion and fees';
COMMENT ON COLUMN subscription_discount_coupons.discount_percent IS 'Percentage of the subscription plan price charged to the user';
COMMENT ON COLUMN subscription_discount_coupons.total_uses IS 'Maximum number of reserved or paid subscription orders using this coupon';
