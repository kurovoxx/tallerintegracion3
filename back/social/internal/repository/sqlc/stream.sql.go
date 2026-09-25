package sqlc

import (
	"context"

	"github.com/jackc/pgx/v5/pgtype"
)

const createStreamChannel = `-- name: CreateStreamChannel :one
INSERT INTO social.stream_channels (group_id, channel_id)
VALUES ($1, $2)
RETURNING id, group_id, channel_id, created_at
`

type CreateStreamChannelParams struct {
	GroupID   pgtype.UUID
	ChannelID string
}

func (q *Queries) CreateStreamChannel(ctx context.Context, arg CreateStreamChannelParams) (SocialStreamChannel, error) {
	row := q.db.QueryRow(ctx, createStreamChannel, arg.GroupID, arg.ChannelID)
	var i SocialStreamChannel
	err := row.Scan(
		&i.ID,
		&i.GroupID,
		&i.ChannelID,
		&i.CreatedAt,
	)
	return i, err
}

const getStreamChannelByGroup = `-- name: GetStreamChannelByGroup :one
SELECT id, group_id, channel_id, created_at FROM social.stream_channels
WHERE group_id = $1 LIMIT 1
`

func (q *Queries) GetStreamChannelByGroup(ctx context.Context, groupID pgtype.UUID) (SocialStreamChannel, error) {
	row := q.db.QueryRow(ctx, getStreamChannelByGroup, groupID)
	var i SocialStreamChannel
	err := row.Scan(
		&i.ID,
		&i.GroupID,
		&i.ChannelID,
		&i.CreatedAt,
	)
	return i, err
}
