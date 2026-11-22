-- +goose Up
CREATE TABLE IF NOT EXISTS visits (
    id         BIGSERIAL PRIMARY KEY,
    link_id    BIGINT NOT NULL REFERENCES links (id) ON DELETE CASCADE,
    visited_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_visits_link_id ON visits (link_id);

-- +goose Down
DROP TABLE IF EXISTS visits;
