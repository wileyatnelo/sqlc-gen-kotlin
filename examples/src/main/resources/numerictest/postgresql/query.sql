-- name: CreateEntry :one
INSERT INTO ledger (label, amount, fee)
VALUES ($1, $2, $3)
RETURNING id, label, amount, fee;

-- name: GetEntry :one
SELECT id, label, amount, fee FROM ledger
WHERE id = $1 LIMIT 1;

-- name: ListEntries :many
SELECT id, label, amount, fee FROM ledger
ORDER BY id;

-- name: ListEntriesOver :many
SELECT id, label, amount, fee FROM ledger
WHERE amount > $1
ORDER BY id;

-- name: ListEntriesByAmounts :many
SELECT id, label, amount, fee FROM ledger
WHERE amount IN (sqlc.slice('amounts'))
ORDER BY id;
