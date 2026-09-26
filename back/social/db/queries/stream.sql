-- name: CreateStreamChannel :one
INSERT INTO social.stream_channels (group_id, channel_id)
VALUES ($1, $2)
RETURNING *;

-- name: GetStreamChannelByGroup :one
SELECT * FROM social.stream_channels
WHERE group_id = $1 LIMIT 1;
