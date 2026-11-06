CREATE TABLE IF NOT EXISTS short_urls (
    id           BIGSERIAL   PRIMARY KEY,
    short_id     TEXT        NOT NULL,
    original_url TEXT        NOT NULL,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE UNIQUE INDEX IF NOT EXISTS short_urls_short_id_idx ON short_urls (short_id);
