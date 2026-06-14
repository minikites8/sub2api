ALTER TABLE users
    ADD COLUMN IF NOT EXISTS signup_ip VARCHAR(45);

CREATE INDEX IF NOT EXISTS idx_users_signup_ip_created_at ON users(signup_ip, created_at);
