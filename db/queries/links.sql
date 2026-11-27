-- name: CreateLink :one
INSERT INTO links (original_url, short_name)
VALUES ($1, $2)
RETURNING id, original_url, short_name, created_at;

-- name: GetLinkByID :one
SELECT id, original_url, short_name, created_at
FROM links
WHERE id = $1;

-- name: GetLinkByShortName :one
SELECT id, original_url, short_name, created_at
FROM links
WHERE short_name = $1;

-- name: UpdateLink :one
UPDATE links
SET original_url = $2, short_name = $3
WHERE id = $1
RETURNING id, original_url, short_name, created_at;

-- name: DeleteLink :execrows
DELETE FROM links
WHERE id = $1;

-- name: AddVisit :one
INSERT INTO link_visits (link_id, ip, user_agent, referer, status)
VALUES ($1, $2, $3, $4, $5)
RETURNING id, created_at;

-- name: ListLinksPage :many
SELECT id, original_url, short_name, created_at
FROM links
ORDER BY id ASC
LIMIT sqlc.arg(page_size)::int OFFSET sqlc.arg(page_start)::int;

-- name: CountLinks :one
SELECT COUNT(*) FROM links;

-- name: CountVisits :one
SELECT COUNT(*) FROM link_visits;

-- name: ListVisitsPage :many
SELECT id, link_id, created_at, ip, user_agent, referer, status
FROM link_visits
ORDER BY id ASC
LIMIT sqlc.arg(page_size)::int OFFSET sqlc.arg(page_start)::int;
