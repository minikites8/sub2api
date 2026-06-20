INSERT INTO settings (key, value, updated_at)
VALUES ('registration_email_suffix_whitelist', '[]', NOW())
ON CONFLICT (key) DO NOTHING;
