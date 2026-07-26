-- 开票申请。开票走人工流程：用户提交抬头/税号/邮箱并勾选订单，管理员在税控
-- 系统线下开具后回填真实发票号码或附件地址。

CREATE TABLE IF NOT EXISTS invoices (
    id BIGSERIAL PRIMARY KEY,
    user_id BIGINT NOT NULL,
    -- 平台内部申请编号（INV-2026-XXXXXX），与管理员回填的真实发票号码无关。
    invoice_no VARCHAR(64) NOT NULL UNIQUE,
    entity_type VARCHAR(20) NOT NULL DEFAULT 'company',
    title VARCHAR(255) NOT NULL,
    tax_id VARCHAR(64),
    delivery_email VARCHAR(255) NOT NULL,
    notes TEXT,
    -- 所选订单金额之和，提交时快照。
    amount DECIMAL(20,8) NOT NULL DEFAULT 0,
    status VARCHAR(20) NOT NULL DEFAULT 'pending',
    issued_invoice_no VARCHAR(128),
    issued_file_url VARCHAR(1024),
    reject_reason TEXT,
    reviewed_by BIGINT,
    reviewed_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- 用户端按自己的申请倒序翻页。
CREATE INDEX IF NOT EXISTS idx_invoices_user_created
    ON invoices (user_id, created_at DESC);

-- 管理端优先看待处理的申请。
CREATE INDEX IF NOT EXISTS idx_invoices_status_created
    ON invoices (status, created_at DESC);

CREATE TABLE IF NOT EXISTS invoice_items (
    id BIGSERIAL PRIMARY KEY,
    invoice_id BIGINT NOT NULL REFERENCES invoices (id) ON DELETE CASCADE,
    order_id BIGINT NOT NULL,
    -- 描述与金额在提交时快照：订单之后退款或调整，不应改变已提交申请的内容。
    description VARCHAR(255) NOT NULL DEFAULT '',
    amount DECIMAL(20,8) NOT NULL DEFAULT 0,
    order_created_at TIMESTAMPTZ,
    -- 申请被驳回或撤回后置为 false，订单即可重新开票。
    active BOOLEAN NOT NULL DEFAULT TRUE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_invoice_items_invoice
    ON invoice_items (invoice_id);

CREATE INDEX IF NOT EXISTS idx_invoice_items_order
    ON invoice_items (order_id);

-- 一个订单同时只能被一条“占用中”的申请覆盖。服务层也会校验，但并发提交两
-- 份含同一订单的申请时，只有数据库约束能真正挡住重复开票。
CREATE UNIQUE INDEX IF NOT EXISTS uniq_invoice_items_active_order
    ON invoice_items (order_id)
    WHERE active;
