DROP INDEX IF EXISTS short_urls_user_id_idx;

ALTER TABLE short_urls DROP COLUMN IF EXISTS user_id;
