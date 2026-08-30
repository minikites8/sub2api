-- User-scoped risk-control bans. A row blocks one user from one group while
-- preserving access to other groups and the user's account status.
CREATE TABLE IF NOT EXISTS user_banned_groups (
    user_id    BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    group_id   BIGINT NOT NULL REFERENCES groups(id) ON DELETE CASCADE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    expires_at TIMESTAMPTZ,
    PRIMARY KEY (user_id, group_id)
);

CREATE INDEX IF NOT EXISTS idx_user_banned_groups_group_id
    ON user_banned_groups(group_id);

ALTER TABLE users ADD COLUMN IF NOT EXISTS disabled_until TIMESTAMPTZ;
