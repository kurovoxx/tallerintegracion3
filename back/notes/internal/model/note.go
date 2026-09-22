package model

import "time"

type Note struct {
	ID              string     `json:"id"`
	UserID          string     `json:"user_id"`
	SubjectID       *string    `json:"subject_id,omitempty"`
	Title           string     `json:"title"`
	ExternalFileID  *string    `json:"external_file_id,omitempty"`
	Visibility      string     `json:"visibility"` // public | private
	LikesCount      int        `json:"likes_count"`
	ForkedFromNoteID *string   `json:"forked_from_note_id,omitempty"`
	SyncStatus      string     `json:"sync_status,omitempty"` // synced | failed_sync
	CreatedAt       time.Time  `json:"created_at"`
	UpdatedAt       time.Time  `json:"updated_at"`
	// Content se descarga de Drive, no está en PG. Solo para respuestas GET.
	Content         *string    `json:"content,omitempty"`
}

type Attachment struct {
	ID             string    `json:"id"`
	NoteID         string    `json:"note_id"`
	ExternalFileID string    `json:"external_file_id"`
	FileURL        string    `json:"file_url"`
	FileType       string    `json:"file_type"`
	FileName       *string   `json:"file_name,omitempty"`
	FileSizeBytes  *int      `json:"file_size_bytes,omitempty"`
	IsInline       bool      `json:"is_inline"`
	CreatedAt      time.Time `json:"created_at"`
}

type SavedNote struct {
	ID      string    `json:"id"`
	UserID  string    `json:"user_id"`
	NoteID  string    `json:"note_id"`
	SavedAt time.Time `json:"saved_at"`
}

type NoteLike struct {
	ID        string    `json:"id"`
	NoteID    string    `json:"note_id"`
	UserID    string    `json:"user_id"`
	CreatedAt time.Time `json:"created_at"`
}

type SharedNote struct {
	ID                       string    `json:"id"`
	NoteID                   string    `json:"note_id"`
	GroupID                  string    `json:"group_id"`
	IsAdminNote              bool      `json:"is_admin_note"`
	AccessMode               string    `json:"access_mode"` // link | restricted
	AuthorFollowersSnapshot  int       `json:"author_followers_snapshot"`
	SharedAt                 time.Time `json:"shared_at"`
}
