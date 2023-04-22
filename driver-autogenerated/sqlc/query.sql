-- name: InsertHTTPSourceRelayCount :exec
INSERT INTO http_source_relay_count (app_public_key, day, success, error)
VALUES ($1, $2, $3, $4)
ON CONFLICT (app_public_key, day) DO UPDATE
    SET success = http_source_relay_count.success + excluded.success,
        error = http_source_relay_count.error + excluded.error;
-- name: InsertHTTPSourceRelayCounts :exec
INSERT INTO http_source_relay_count (app_public_key, day, success, error)
SELECT
    unnest($1::char(64)[]) AS app_public_key,
    unnest($2::date[]) AS day,
    unnest($3::bigint[]) AS success,
    unnest($4::bigint[]) AS error
ON CONFLICT (app_public_key, day) DO UPDATE
    SET success = http_source_relay_count.success + excluded.success,
        error = http_source_relay_count.error + excluded.error;
-- name: SelectHTTPSourceRelayCounts :many
SELECT app_public_key, day, success, error
FROM http_source_relay_count
WHERE day BETWEEN $1 AND $2;
