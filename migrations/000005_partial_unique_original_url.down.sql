DROP INDEX IF EXISTS short_urls_original_url_idx;

CREATE UNIQUE INDEX IF NOT EXISTS short_urls_original_url_idx ON short_urls (original_url);
