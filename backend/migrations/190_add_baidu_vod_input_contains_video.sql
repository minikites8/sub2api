ALTER TABLE baidu_vod_video_tasks
    ADD COLUMN IF NOT EXISTS input_contains_video BOOLEAN NOT NULL DEFAULT false;

COMMENT ON COLUMN baidu_vod_video_tasks.input_contains_video IS
    'Whether the submitted Seedance request contains at least one video_url input';
