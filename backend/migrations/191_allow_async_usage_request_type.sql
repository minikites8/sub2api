-- Asynchronous media jobs are recorded as request_type=5 so usage filters can
-- distinguish task submissions from synchronous and streaming requests.
ALTER TABLE usage_logs
    DROP CONSTRAINT IF EXISTS usage_logs_request_type_check;

ALTER TABLE usage_logs
    ADD CONSTRAINT usage_logs_request_type_check
    CHECK (request_type IN (0, 1, 2, 3, 4, 5)) NOT VALID;