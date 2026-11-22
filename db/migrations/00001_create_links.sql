-- +goose Up
CREATE TABLE IF NOT EXISTS links (
    id           BIGSERIAL PRIMARY KEY,
    original_url TEXT NOT NULL,
    short_name   TEXT NOT NULL UNIQUE,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_links_short_name ON links (short_name);

-- +goose Down
DROP INDEX IF EXISTS idx_links_short_name;
DROP TABLE IF EXISTS links;
