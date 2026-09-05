CREATE TABLE IF NOT EXISTS recharge_discount_coupons (
    id BIGSERIAL PRIMARY KEY,
    user_id BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    min_recharge_amount DECIMAL(20,8) NOT NULL,
    discount_percent DECIMAL(5,2) NOT NULL,
    total_uses INTEGER NOT NULL,
    status VARCHAR(20) NOT NULL DEFAULT 'active',
    created_by BIGINT NOT NULL,
    notes TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT recharge_discount_coupons_min_amount_positive CHECK (min_recharge_amount > 0),
    CONSTRAINT recharge_discount_coupons_discount_range CHECK (discount_percent > 0 AND discount_percent < 100),
    CONSTRAINT recharge_discount_coupons_total_uses_positive CHECK (total_uses > 0),
    CONSTRAINT recharge_discount_coupons_status_valid CHECK (status IN ('active', 'revoked'))
);

CREATE INDEX IF NOT EXISTS idx_recharge_discount_coupons_user_status
    ON recharge_discount_coupons(user_id, status);

CREATE INDEX IF NOT EXISTS idx_recharge_discount_coupons_created_at
    ON recharge_discount_coupons(created_at);

COMMENT ON TABLE recharge_discount_coupons IS 'Balance recharge discount coupons manually issued to users';
COMMENT ON COLUMN recharge_discount_coupons.min_recharge_amount IS 'Minimum requested recharge amount required to use the coupon';
COMMENT ON COLUMN recharge_discount_coupons.discount_percent IS 'Percentage of the requested recharge amount charged to the user';
COMMENT ON COLUMN recharge_discount_coupons.total_uses IS 'Maximum number of recharge orders that may consume this coupon';
