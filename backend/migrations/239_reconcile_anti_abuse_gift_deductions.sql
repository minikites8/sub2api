WITH risk_deductions AS (
    SELECT
        r.used_by AS user_id,
        ABS(r.value) AS amount,
        COALESCE(r.used_at, r.created_at) AS occurred_at,
        COALESCE(r.notes, '') AS notes
    FROM redeem_codes r
    WHERE r.type = 'risk_control_balance'
      AND r.status = 'used'
      AND r.used_by IS NOT NULL
      AND r.value < 0
      AND COALESCE(r.used_at, r.created_at) >= COALESCE(
          (SELECT applied_at FROM schema_migrations WHERE filename = '231_anti_abuse_multidimensional.sql'),
          '-infinity'::timestamptz
      )
)
INSERT INTO anti_abuse_events (
    user_id,
    event_type,
    action,
    score,
    factors,
    reasons,
    ip_address,
    email,
    user_agent,
    fingerprint_hash_count,
    ja3_hash,
    ja4_hash,
    account_attempts,
    gift_balance_deducted,
    created_at
)
SELECT
    d.user_id,
    'risk_control_gift_deduction',
    'restrict',
    CASE
        WHEN d.notes ~ 'score=[0-9]+' THEN SUBSTRING(d.notes FROM 'score=([0-9]+)')::integer
        ELSE 0
    END,
    '{"reconciled_balance_history":1}'::jsonb,
    '["reconciled from risk-control balance history"]'::jsonb,
    '',
    COALESCE(u.email, ''),
    '',
    0,
    '',
    '',
    '[]'::jsonb,
    d.amount,
    d.occurred_at
FROM risk_deductions d
LEFT JOIN users u ON u.id = d.user_id
WHERE NOT EXISTS (
    SELECT 1
    FROM anti_abuse_events e
    WHERE e.user_id = d.user_id
      AND e.gift_balance_deducted = d.amount
      AND e.created_at BETWEEN d.occurred_at - INTERVAL '30 seconds'
                           AND d.occurred_at + INTERVAL '30 seconds'
);
