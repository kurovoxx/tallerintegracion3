package sqlc

import (
	"context"

	"github.com/jackc/pgx/v5/pgtype"
)

const getGroupByID = `-- name: GetGroupByID :one
SELECT id, name, description, owner_user_id, notes_restricted_to_staff, invite_token, created_at FROM social.groups
WHERE id = $1 LIMIT 1
`

func (q *Queries) GetGroupByID(ctx context.Context, id pgtype.UUID) (SocialGroup, error) {
	row := q.db.QueryRow(ctx, getGroupByID, id)
	var i SocialGroup
	err := row.Scan(
		&i.ID,
		&i.Name,
		&i.Description,
		&i.OwnerUserID,
		&i.NotesRestrictedToStaff,
		&i.InviteToken,
		&i.CreatedAt,
	)
	return i, err
}
