-- name: InsertHTTPSourceRelayCount :exec
INSERT INTO http_source_relay_count (portal_app_id, day, success, error)
VALUES ($1, $2, $3, $4)
ON CONFLICT (portal_app_id, day) DO UPDATE
    SET success = http_source_relay_count.success + excluded.success,
        error = http_source_relay_count.error + excluded.error;
-- name: InsertHTTPSourceRelayCounts :exec
INSERT INTO http_source_relay_count (portal_app_id, day, success, error)
SELECT
    unnest($1::char(64)[]) AS portal_app_id,
    unnest($2::date[]) AS day,
    unnest($3::bigint[]) AS success,
    unnest($4::bigint[]) AS error
ON CONFLICT (portal_app_id, day) DO UPDATE
    SET success = http_source_relay_count.success + excluded.success,
        error = http_source_relay_count.error + excluded.error;
-- name: SelectHTTPSourceRelayCounts :many
SELECT portal_app_id, day, success, error
FROM http_source_relay_count
WHERE day BETWEEN $1 AND $2;
