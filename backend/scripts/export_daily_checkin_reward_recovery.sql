-- Export users whose legacy daily check-in balance was restored by migration 234.
-- Usage:
--   psql "$DATABASE_URL" -f export_daily_checkin_reward_recovery.sql > daily_checkin_reward_recovery.csv
\copy (
    SELECT r.user_id,
           COALESCE(u.email, '') AS email,
           COALESCE(u.username, '') AS username,
           SUM(r.recovered_amount)::numeric(20, 8) AS recovered_amount,
           COUNT(*) AS recovered_record_count,
           MIN(r.recovered_at) AS first_recovered_at,
           MAX(r.recovered_at) AS last_recovered_at
    FROM daily_checkin_reward_recovery r
    LEFT JOIN users u ON u.id = r.user_id
    WHERE r.recovered_amount > 0
    GROUP BY r.user_id, u.email, u.username
    ORDER BY r.user_id
) TO STDOUT WITH (FORMAT csv, HEADER true);
