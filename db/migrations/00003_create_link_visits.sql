-- +goose Up
ALTER TABLE visits RENAME TO link_visits;

ALTER TABLE link_visits
    RENAME COLUMN visited_at TO created_at;

ALTER TABLE link_visits
    ADD COLUMN ip TEXT NOT NULL DEFAULT '',
    ADD COLUMN user_agent TEXT NOT NULL DEFAULT '',
    ADD COLUMN referer TEXT NOT NULL DEFAULT '',
    ADD COLUMN status INT NOT NULL DEFAULT 302;

ALTER INDEX IF EXISTS idx_visits_link_id RENAME TO idx_link_visits_link_id;

-- +goose Down
ALTER TABLE link_visits
    DROP COLUMN IF EXISTS status,
    DROP COLUMN IF EXISTS referer,
    DROP COLUMN IF EXISTS user_agent,
    DROP COLUMN IF EXISTS ip;

ALTER TABLE link_visits
    RENAME COLUMN created_at TO visited_at;

ALTER INDEX IF EXISTS idx_link_visits_link_id RENAME TO idx_visits_link_id;

ALTER TABLE link_visits RENAME TO visits;
