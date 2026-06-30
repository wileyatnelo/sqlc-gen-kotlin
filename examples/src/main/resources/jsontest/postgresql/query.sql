-- name: CreateEvent :one
INSERT INTO events (name, payload, metadata)
VALUES ($1, $2, $3)
RETURNING id, name, payload, metadata;

-- name: GetEvent :one
SELECT id, name, payload, metadata FROM events
WHERE id = $1 LIMIT 1;

-- name: ListEvents :many
SELECT id, name, payload, metadata FROM events
ORDER BY id;

-- name: ListEventsByIds :many
SELECT id, name, payload, metadata FROM events
WHERE id IN (sqlc.slice('ids'))
ORDER BY id;
