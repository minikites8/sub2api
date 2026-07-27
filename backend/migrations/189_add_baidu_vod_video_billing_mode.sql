ALTER TABLE baidu_vod_video_tasks
    ADD COLUMN IF NOT EXISTS billing_mode VARCHAR(20) NOT NULL DEFAULT 'video';

ALTER TABLE groups
    ADD COLUMN IF NOT EXISTS video_price_4k DECIMAL(20,8);

COMMENT ON COLUMN groups.video_price_480p IS '480p 视频生成每秒单价 (Credits/s)';
COMMENT ON COLUMN groups.video_price_720p IS '720p 视频生成每秒单价 (Credits/s)';
COMMENT ON COLUMN groups.video_price_1080p IS '1080p 视频生成每秒单价 (Credits/s)';
COMMENT ON COLUMN groups.video_price_4k IS '4K 视频生成每秒单价 (Credits/s)';
