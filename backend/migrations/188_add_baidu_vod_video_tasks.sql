CREATE TABLE IF NOT EXISTS baidu_vod_video_tasks (
    id BIGSERIAL PRIMARY KEY,
    platform VARCHAR(32) NOT NULL DEFAULT 'baidu_vod',
    provider VARCHAR(32) NOT NULL DEFAULT 'happyhorse',
    task_id VARCHAR(128) NOT NULL,
    upstream_task_id VARCHAR(128) NOT NULL,
    upstream_request_id VARCHAR(128),
    user_id BIGINT NOT NULL,
    api_key_id BIGINT NOT NULL,
    account_id BIGINT NOT NULL,
    group_id BIGINT,
    model VARCHAR(128) NOT NULL,
    upstream_model VARCHAR(128) NOT NULL,
    capability VARCHAR(32) NOT NULL,
    status VARCHAR(32) NOT NULL DEFAULT 'queued',
    upstream_status VARCHAR(32) NOT NULL DEFAULT 'PENDING',
    resolution VARCHAR(16) NOT NULL,
    ratio VARCHAR(32) NOT NULL DEFAULT '16:9',
    requested_duration INTEGER NOT NULL,
    output_duration INTEGER NOT NULL DEFAULT 0,
    input_video_duration INTEGER NOT NULL DEFAULT 0,
    video_count INTEGER NOT NULL DEFAULT 1,
    estimated_cost DECIMAL(20,10) NOT NULL DEFAULT 0,
    hold_amount DECIMAL(20,10) NOT NULL DEFAULT 0,
    actual_cost DECIMAL(20,10),
    group_rate_multiplier DECIMAL(20,10) NOT NULL DEFAULT 1,
    video_rate_multiplier DECIMAL(20,10) NOT NULL DEFAULT 1,
    account_rate_multiplier DECIMAL(20,10) NOT NULL DEFAULT 1,
    request_hash VARCHAR(128) NOT NULL,
    result_url TEXT,
    result_expires_at TIMESTAMPTZ,
    last_error_code VARCHAR(128),
    last_error_message TEXT,
    retry_count INTEGER NOT NULL DEFAULT 0,
    version INTEGER NOT NULL DEFAULT 0,
    next_poll_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    poll_claimed_until TIMESTAMPTZ,
    last_polled_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    submitted_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    started_at TIMESTAMPTZ,
    finished_at TIMESTAMPTZ,
    settled_at TIMESTAMPTZ
);

CREATE UNIQUE INDEX IF NOT EXISTS baidu_vod_video_tasks_platform_task_uq
    ON baidu_vod_video_tasks (platform, task_id);
CREATE UNIQUE INDEX IF NOT EXISTS baidu_vod_video_tasks_platform_upstream_task_uq
    ON baidu_vod_video_tasks (platform, upstream_task_id)
    WHERE upstream_task_id <> '';
CREATE INDEX IF NOT EXISTS baidu_vod_video_tasks_owner_created_idx
    ON baidu_vod_video_tasks (user_id, api_key_id, created_at);
CREATE INDEX IF NOT EXISTS baidu_vod_video_tasks_status_poll_idx
    ON baidu_vod_video_tasks (status, next_poll_at);
CREATE INDEX IF NOT EXISTS baidu_vod_video_tasks_account_status_idx
    ON baidu_vod_video_tasks (account_id, status);
CREATE INDEX IF NOT EXISTS baidu_vod_video_tasks_result_expires_idx
    ON baidu_vod_video_tasks (result_expires_at);

UPDATE groups
SET allow_image_generation = true
WHERE platform = 'baidu_vod'
  AND allow_image_generation = false;
