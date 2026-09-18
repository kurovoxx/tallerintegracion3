-- name: UpsertDiscordConfig :one
INSERT INTO social.discord_integrations (group_id, server_name, invite_url, webhook_url)
VALUES ($1, $2, $3, $4)
ON CONFLICT (group_id) DO UPDATE SET
  server_name = EXCLUDED.server_name,
  invite_url = EXCLUDED.invite_url,
  webhook_url = EXCLUDED.webhook_url
RETURNING *;

-- name: GetDiscordConfigByGroup :one
SELECT * FROM social.discord_integrations
WHERE group_id = $1 LIMIT 1;

-- name: GetGroupMemberRole :one
SELECT role FROM social.group_memberships
WHERE group_id = $1 AND user_id = $2 LIMIT 1;
