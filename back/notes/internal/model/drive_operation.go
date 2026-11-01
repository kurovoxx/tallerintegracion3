package model

import (
	"encoding/json"
	"time"
)

// DriveOperation survives deletion of its logical note/attachment references.
type DriveOperation struct {
	ID             string
	Operation      string
	NoteID         *string
	AttachmentID   *string
	ExternalFileID *string
	OwnerUserID    string
	Payload        json.RawMessage
	Status         string
	Attempts       int
	NextAttemptAt  time.Time
	LastError      *string
	CreatedAt      *time.Time
	UpdatedAt      *time.Time
	CompletedAt    *time.Time
}
