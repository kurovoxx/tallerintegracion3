package sqlc

import (
	"context"

	"github.com/jackc/pgx/v5/pgtype"
)

const createMeeting = `-- name: CreateMeeting :one
INSERT INTO social.meetings (
  group_id, title, description, scheduled_at, created_by_user_id, notify_discord
) VALUES (
  $1, $2, $3, $4, $5, $6
) RETURNING id, group_id, title, description, scheduled_at, created_by_user_id, notify_discord, google_calendar_event_id, created_at
`

type CreateMeetingParams struct {
	GroupID         pgtype.UUID
	Title           string
	Description     pgtype.Text
	ScheduledAt     pgtype.Timestamptz
	CreatedByUserID pgtype.UUID
	NotifyDiscord   bool
}

func (q *Queries) CreateMeeting(ctx context.Context, arg CreateMeetingParams) (SocialMeeting, error) {
	row := q.db.QueryRow(ctx, createMeeting,
		arg.GroupID,
		arg.Title,
		arg.Description,
		arg.ScheduledAt,
		arg.CreatedByUserID,
		arg.NotifyDiscord,
	)
	var i SocialMeeting
	err := row.Scan(
		&i.ID,
		&i.GroupID,
		&i.Title,
		&i.Description,
		&i.ScheduledAt,
		&i.CreatedByUserID,
		&i.NotifyDiscord,
		&i.GoogleCalendarEventID,
		&i.CreatedAt,
	)
	return i, err
}

const getMeetingByID = `-- name: GetMeetingByID :one
SELECT id, group_id, title, description, scheduled_at, created_by_user_id, notify_discord, google_calendar_event_id, created_at FROM social.meetings
WHERE id = $1 LIMIT 1
`

func (q *Queries) GetMeetingByID(ctx context.Context, id pgtype.UUID) (SocialMeeting, error) {
	row := q.db.QueryRow(ctx, getMeetingByID, id)
	var i SocialMeeting
	err := row.Scan(
		&i.ID,
		&i.GroupID,
		&i.Title,
		&i.Description,
		&i.ScheduledAt,
		&i.CreatedByUserID,
		&i.NotifyDiscord,
		&i.GoogleCalendarEventID,
		&i.CreatedAt,
	)
	return i, err
}

const listMeetingsByGroup = `-- name: ListMeetingsByGroup :many
SELECT id, group_id, title, description, scheduled_at, created_by_user_id, notify_discord, google_calendar_event_id, created_at FROM social.meetings
WHERE group_id = $1
ORDER BY scheduled_at ASC
`

func (q *Queries) ListMeetingsByGroup(ctx context.Context, groupID pgtype.UUID) ([]SocialMeeting, error) {
	rows, err := q.db.Query(ctx, listMeetingsByGroup, groupID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var items []SocialMeeting
	for rows.Next() {
		var i SocialMeeting
		if err := rows.Scan(
			&i.ID,
			&i.GroupID,
			&i.Title,
			&i.Description,
			&i.ScheduledAt,
			&i.CreatedByUserID,
			&i.NotifyDiscord,
			&i.GoogleCalendarEventID,
			&i.CreatedAt,
		); err != nil {
			return nil, err
		}
		items = append(items, i)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return items, nil
}

const createMeetingNotification = `-- name: CreateMeetingNotification :one
INSERT INTO social.meeting_notifications (meeting_id, user_id)
VALUES ($1, $2)
RETURNING id, meeting_id, user_id, read_at, created_at
`

type CreateMeetingNotificationParams struct {
	MeetingID pgtype.UUID
	UserID    pgtype.UUID
}

func (q *Queries) CreateMeetingNotification(ctx context.Context, arg CreateMeetingNotificationParams) (SocialMeetingNotification, error) {
	row := q.db.QueryRow(ctx, createMeetingNotification, arg.MeetingID, arg.UserID)
	var i SocialMeetingNotification
	err := row.Scan(
		&i.ID,
		&i.MeetingID,
		&i.UserID,
		&i.ReadAt,
		&i.CreatedAt,
	)
	return i, err
}

const listGroupMemberUserIDs = `-- name: ListGroupMemberUserIDs :many
SELECT user_id FROM social.group_memberships
WHERE group_id = $1
`

func (q *Queries) ListGroupMemberUserIDs(ctx context.Context, groupID pgtype.UUID) ([]pgtype.UUID, error) {
	rows, err := q.db.Query(ctx, listGroupMemberUserIDs, groupID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var items []pgtype.UUID
	for rows.Next() {
		var userID pgtype.UUID
		if err := rows.Scan(&userID); err != nil {
			return nil, err
		}
		items = append(items, userID)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return items, nil
}

const isGroupMember = `-- name: IsGroupMember :one
SELECT EXISTS(
  SELECT 1 FROM social.group_memberships
  WHERE group_id = $1 AND user_id = $2
)
`

type IsGroupMemberParams struct {
	GroupID pgtype.UUID
	UserID  pgtype.UUID
}

func (q *Queries) IsGroupMember(ctx context.Context, arg IsGroupMemberParams) (bool, error) {
	row := q.db.QueryRow(ctx, isGroupMember, arg.GroupID, arg.UserID)
	var exists bool
	err := row.Scan(&exists)
	return exists, err
}

const updateMeetingCalendarEventID = `-- name: UpdateMeetingCalendarEventID :one
UPDATE social.meetings SET
  google_calendar_event_id = $2
WHERE id = $1
RETURNING id, group_id, title, description, scheduled_at, created_by_user_id, notify_discord, google_calendar_event_id, created_at
`

type UpdateMeetingCalendarEventIDParams struct {
	ID                    pgtype.UUID
	GoogleCalendarEventID pgtype.Text
}

func (q *Queries) UpdateMeetingCalendarEventID(ctx context.Context, arg UpdateMeetingCalendarEventIDParams) (SocialMeeting, error) {
	row := q.db.QueryRow(ctx, updateMeetingCalendarEventID, arg.ID, arg.GoogleCalendarEventID)
	var i SocialMeeting
	err := row.Scan(
		&i.ID,
		&i.GroupID,
		&i.Title,
		&i.Description,
		&i.ScheduledAt,
		&i.CreatedByUserID,
		&i.NotifyDiscord,
		&i.GoogleCalendarEventID,
		&i.CreatedAt,
	)
	return i, err
}

const createMeetingAttendee = `-- name: CreateMeetingAttendee :one
INSERT INTO social.meeting_attendees (meeting_id, email)
VALUES ($1, $2)
ON CONFLICT (meeting_id, email) DO NOTHING
RETURNING id, meeting_id, email, created_at
`

type CreateMeetingAttendeeParams struct {
	MeetingID pgtype.UUID
	Email     string
}

func (q *Queries) CreateMeetingAttendee(ctx context.Context, arg CreateMeetingAttendeeParams) (SocialMeetingAttendee, error) {
	row := q.db.QueryRow(ctx, createMeetingAttendee, arg.MeetingID, arg.Email)
	var i SocialMeetingAttendee
	err := row.Scan(
		&i.ID,
		&i.MeetingID,
		&i.Email,
		&i.CreatedAt,
	)
	return i, err
}

const listMeetingAttendees = `-- name: ListMeetingAttendees :many
SELECT id, meeting_id, email, created_at FROM social.meeting_attendees
WHERE meeting_id = $1
ORDER BY created_at ASC
`

func (q *Queries) ListMeetingAttendees(ctx context.Context, meetingID pgtype.UUID) ([]SocialMeetingAttendee, error) {
	rows, err := q.db.Query(ctx, listMeetingAttendees, meetingID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var items []SocialMeetingAttendee
	for rows.Next() {
		var i SocialMeetingAttendee
		if err := rows.Scan(
			&i.ID,
			&i.MeetingID,
			&i.Email,
			&i.CreatedAt,
		); err != nil {
			return nil, err
		}
		items = append(items, i)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return items, nil
}
