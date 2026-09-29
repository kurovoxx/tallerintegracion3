package model

import "time"

// NoteAccessResponse separates application permissions from remote Drive access.
type NoteAccessResponse struct {
	CanRead                 bool    `json:"can_read"`
	CanWrite                bool    `json:"can_write"`
	Visibility              string  `json:"visibility"`
	AccessMode              string  `json:"access_mode"` // owner | public | link | restricted
	DriveSyncStatus         string  `json:"drive_sync_status"`
	DriveAccessVerified     bool    `json:"drive_access_verified"`
	DriveConnectionRequired bool    `json:"drive_connection_required"`
	DriveURL                *string `json:"drive_url"`
}

type Note struct {
	ID               string  `json:"id"`
	UserID           string  `json:"user_id"`
	SubjectID        *string `json:"subject_id,omitempty"`
	Title            string  `json:"title"`
	ExternalFileID   *string `json:"external_file_id,omitempty"`
	Visibility       string  `json:"visibility"` // public | private
	LikesCount       int     `json:"likes_count"`
	ForkedFromNoteID *string `json:"forked_from_note_id,omitempty"`
	SyncStatus       string  `json:"sync_status,omitempty"` // synced | failed_sync
	// Version es el contador de versionado optimista: se incrementa en cada
	// Update de metadata y permite detectar escrituras concurrentes (409).
	Version   int64     `json:"version"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
	// Content se descarga de Drive, no está en PG. Solo para respuestas GET.
	Content *string `json:"content,omitempty"`
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
	ID                      string    `json:"id"`
	NoteID                  string    `json:"note_id"`
	GroupID                 string    `json:"group_id"`
	IsAdminNote             bool      `json:"is_admin_note"`
	AccessMode              string    `json:"access_mode"` // link | restricted
	AuthorFollowersSnapshot int       `json:"author_followers_snapshot"`
	PermissionSyncStatus    string    `json:"permission_sync_status,omitempty"` // in_sync | pending | failed
	SharedAt                time.Time `json:"shared_at"`
}

// Valores de sincronización de permisos: aplican tanto a
// notes.shared_notes.permission_sync_status como a
// notes.drive_managed_permissions.sync_status.
const (
	PermissionSyncInSync  = "in_sync"
	PermissionSyncPending = "pending"
	PermissionSyncFailed  = "failed"
)

// Tipos de principal de notes.drive_managed_permissions.
const (
	PermissionPrincipalAnyone = "anyone"
	PermissionPrincipalEmail  = "email"
)

// DriveManagedPermission es la fila de notas.drive_managed_permissions: el
// estado deseado/observado de un permiso de Drive administrado por Notes para
// una nota. Notes es dueño de estos permisos (no de los que el usuario otorgue
// manualmente); el reconciliador los converge contra el ACL remoto.
type DriveManagedPermission struct {
	ID                string    `json:"id"`
	NoteID            string    `json:"note_id"`
	ExternalFileID    string    `json:"external_file_id"`
	PrincipalType     string    `json:"principal_type"` // anyone | email
	PrincipalKey      string    `json:"principal_key"`  // "" para anyone; email normalizado
	DrivePermissionID *string   `json:"drive_permission_id,omitempty"`
	Role              string    `json:"role"`
	SyncStatus        string    `json:"sync_status"` // in_sync | pending | failed
	CreatedAt         time.Time `json:"created_at"`
	UpdatedAt         time.Time `json:"updated_at"`
}

// IdempotencyKey es una fila de notes.idempotency_keys: la intención durable de
// una operación (create/copy/update). status='in_progress' representa el claim
// vigente (expires_at actúa como lease para recuperación tras crash);
// status='completed' permite reenviar la misma respuesta mientras request_hash
// coincida con el payload original; status='recoverable' señala un fallo
// posterior a la inserción local (recurso ya persistido): un reintento
// inmediato con el mismo request_hash reclama el recurso preexistente sin
// esperar a expires_at, y un hash distinto es 409 Conflict.
type IdempotencyKey struct {
	UserID         string    `json:"user_id"`
	Operation      string    `json:"operation"`
	IdempotencyKey string    `json:"idempotency_key"`
	ResourceID     *string   `json:"resource_id,omitempty"`
	RequestHash    string    `json:"request_hash"`
	Status         string    `json:"status"`
	ExpiresAt      time.Time `json:"expires_at"`
}

const (
	IdempotencyStatusInProgress = "in_progress"
	IdempotencyStatusCompleted  = "completed"
	// IdempotencyStatusRecoverable marca un fallo DESPUÉS de insertar el recurso
	// local (Drive/timeout/OAuth): la nota ya existe y el reintento inmediato con
	// la misma clave debe retomarla, nunca duplicarla.
	IdempotencyStatusRecoverable = "recoverable"
)
