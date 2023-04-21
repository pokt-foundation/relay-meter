-- name: InsertHTTPSourceRelayCount :exec
INSERT INTO http_source_relay_count (app_public_key, day, success, error)
VALUES ($1, $2, $3, $4)
ON CONFLICT (app_public_key, day) DO UPDATE
    SET success = http_source_relay_count.success + excluded.success,
        error = http_source_relay_count.error + excluded.error;
-- name: SelectHTTPSourceRelayCounts :many
SELECT app_public_key, day, success, error
FROM http_source_relay_count
WHERE day BETWEEN $1 AND $2;
