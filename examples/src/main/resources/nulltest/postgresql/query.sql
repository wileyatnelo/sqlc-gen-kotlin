-- Read-only on purpose. The write path has its own, separate nullable-parameter bug
-- (`stmt.setLong(i, x)` is emitted for a `Long?` parameter, which does not compile), so
-- these queries only exercise the row-reading path this example is here to cover. The test
-- seeds its rows with plain JDBC.

-- name: GetReading :one
SELECT id, label, count_big, count_int, count_small, ratio, ratio_real, enabled,
       required_big, required_flag
FROM readings
WHERE id = $1;

-- name: ListReadings :many
SELECT id, label, count_big, count_int, count_small, ratio, ratio_real, enabled,
       required_big, required_flag
FROM readings
ORDER BY id;
