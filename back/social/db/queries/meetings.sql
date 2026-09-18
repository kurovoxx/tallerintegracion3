-- name: CreateMeeting :one
INSERT INTO social.meetings (
  group_id, title, description, scheduled_at, created_by_user_id, notify_discord
) VALUES (
  $1, $2, $3, $4, $5, $6
) RETURNING *;

-- name: GetMeetingByID :one
SELECT * FROM social.meetings
WHERE id = $1 LIMIT 1;

-- name: ListMeetingsByGroup :many
SELECT * FROM social.meetings
WHERE group_id = $1
ORDER BY scheduled_at ASC;

-- name: CreateMeetingNotification :one
INSERT INTO social.meeting_notifications (meeting_id, user_id)
VALUES ($1, $2)
RETURNING *;

-- name: ListGroupMemberUserIDs :many
SELECT user_id FROM social.group_memberships
WHERE group_id = $1;

-- name: IsGroupMember :one
SELECT EXISTS(
  SELECT 1 FROM social.group_memberships
  WHERE group_id = $1 AND user_id = $2
);

-- name: UpdateMeetingCalendarEventID :one
UPDATE social.meetings SET
  google_calendar_event_id = $2
WHERE id = $1
RETURNING *;
