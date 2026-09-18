package sqlc

import (
	"context"

	"github.com/jackc/pgx/v5/pgtype"
)

const upsertDiscordConfig = `-- name: UpsertDiscordConfig :one
INSERT INTO social.discord_integrations (group_id, server_name, invite_url, webhook_url)
VALUES ($1, $2, $3, $4)
ON CONFLICT (group_id) DO UPDATE SET
  server_name = EXCLUDED.server_name,
  invite_url = EXCLUDED.invite_url,
  webhook_url = EXCLUDED.webhook_url
RETURNING id, group_id, server_name, invite_url, webhook_url, category_id, channel_id, created_at
`

type UpsertDiscordConfigParams struct {
	GroupID    pgtype.UUID
	ServerName pgtype.Text
	InviteUrl  pgtype.Text
	WebhookUrl pgtype.Text
}

func (q *Queries) UpsertDiscordConfig(ctx context.Context, arg UpsertDiscordConfigParams) (SocialDiscordIntegration, error) {
	row := q.db.QueryRow(ctx, upsertDiscordConfig,
		arg.GroupID,
		arg.ServerName,
		arg.InviteUrl,
		arg.WebhookUrl,
	)
	var i SocialDiscordIntegration
	err := row.Scan(
		&i.ID,
		&i.GroupID,
		&i.ServerName,
		&i.InviteUrl,
		&i.WebhookUrl,
		&i.CategoryID,
		&i.ChannelID,
		&i.CreatedAt,
	)
	return i, err
}

const getDiscordConfigByGroup = `-- name: GetDiscordConfigByGroup :one
SELECT id, group_id, server_name, invite_url, webhook_url, category_id, channel_id, created_at FROM social.discord_integrations
WHERE group_id = $1 LIMIT 1
`

func (q *Queries) GetDiscordConfigByGroup(ctx context.Context, groupID pgtype.UUID) (SocialDiscordIntegration, error) {
	row := q.db.QueryRow(ctx, getDiscordConfigByGroup, groupID)
	var i SocialDiscordIntegration
	err := row.Scan(
		&i.ID,
		&i.GroupID,
		&i.ServerName,
		&i.InviteUrl,
		&i.WebhookUrl,
		&i.CategoryID,
		&i.ChannelID,
		&i.CreatedAt,
	)
	return i, err
}

const getGroupMemberRole = `-- name: GetGroupMemberRole :one
SELECT role FROM social.group_memberships
WHERE group_id = $1 AND user_id = $2 LIMIT 1
`

type GetGroupMemberRoleParams struct {
	GroupID pgtype.UUID
	UserID  pgtype.UUID
}

func (q *Queries) GetGroupMemberRole(ctx context.Context, arg GetGroupMemberRoleParams) (string, error) {
	row := q.db.QueryRow(ctx, getGroupMemberRole, arg.GroupID, arg.UserID)
	var role string
	err := row.Scan(&role)
	return role, err
}
