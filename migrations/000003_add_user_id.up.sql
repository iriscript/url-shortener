ALTER TABLE short_urls ADD COLUMN IF NOT EXISTS user_id TEXT NOT NULL DEFAULT '';

CREATE INDEX IF NOT EXISTS short_urls_user_id_idx ON short_urls (user_id);
