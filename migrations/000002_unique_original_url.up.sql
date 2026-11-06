DELETE FROM short_urls a
    USING short_urls b
WHERE a.original_url = b.original_url
  AND a.id > b.id;

CREATE UNIQUE INDEX IF NOT EXISTS short_urls_original_url_idx ON short_urls (original_url);
