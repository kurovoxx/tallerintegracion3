package sqlc

import (
	"github.com/jackc/pgx/v5/pgtype"
)

type SocialGroup struct {
	ID                     pgtype.UUID
	Name                   string
	Description            pgtype.Text
	OwnerUserID            pgtype.UUID
	NotesRestrictedToStaff bool
	InviteToken            pgtype.UUID
	CreatedAt              pgtype.Timestamptz
}

type SocialTodoBoard struct {
	ID        pgtype.UUID
	GroupID   pgtype.UUID
	Name      string
	CreatedAt pgtype.Timestamptz
}

type SocialTodoTask struct {
	ID             pgtype.UUID
	BoardID        pgtype.UUID
	Title          string
	Status         string
	AssigneeUserID pgtype.UUID
	DueDate        pgtype.Date
	CreatedAt      pgtype.Timestamptz
	UpdatedAt      pgtype.Timestamptz
}

type SocialSprintSheet struct {
	ID          pgtype.UUID
	GroupID     pgtype.UUID
	Name        string
	PeriodStart pgtype.Date
	PeriodEnd   pgtype.Date
	CreatedAt   pgtype.Timestamptz
}

type SocialSprintSheetTask struct {
	ID             pgtype.UUID
	SheetID        pgtype.UUID
	AssigneeUserID pgtype.UUID
	Title          string
	Priority       string
	Status         string
	EstimatedHours pgtype.Numeric
	CreatedAt      pgtype.Timestamptz
	UpdatedAt      pgtype.Timestamptz
}

type SocialSprintSheetDailyHour struct {
	ID      pgtype.UUID
	TaskID  pgtype.UUID
	LogDate pgtype.Date
	Hours   pgtype.Numeric
}

type SocialMeeting struct {
	ID                    pgtype.UUID
	GroupID               pgtype.UUID
	Title                 string
	Description           pgtype.Text
	ScheduledAt           pgtype.Timestamptz
	CreatedByUserID       pgtype.UUID
	NotifyDiscord         bool
	GoogleCalendarEventID pgtype.Text
	CreatedAt             pgtype.Timestamptz
}

type SocialMeetingNotification struct {
	ID        pgtype.UUID
	MeetingID pgtype.UUID
	UserID    pgtype.UUID
	ReadAt    pgtype.Timestamptz
	CreatedAt pgtype.Timestamptz
}
