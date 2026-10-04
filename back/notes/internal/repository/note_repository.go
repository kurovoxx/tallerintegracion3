package repository

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/kurovoxx/tallerintegracion3/back/notes/internal/model"
)

// NoteRepository es la única capa que toca SQL para notes.notes
type NoteRepository struct {
	pool *pgxpool.Pool
}

func NewNoteRepository(pool *pgxpool.Pool) *NoteRepository {
	return &NoteRepository{pool: pool}
}

// DBTX abtrae pool o tx para queries transaccionales
type DBTX interface {
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
}

// Create inserta metadata en notes.notes. noteID permite preasignar el id de la
// fila (pre-asociación al claim de idempotencia para recuperación lógica tras un
// crash); vacío => lo genera la base de datos. externalFileID puede ser nil
// brevemente.
// syncStatus: "pending_drive" | "synced" | "failed_sync"
func (r *NoteRepository) Create(ctx context.Context, db DBTX, noteID string, userID string, subjectID *string, title string, externalFileID *string, visibility string, forkedFrom *string, syncStatus string) (*model.Note, error) {
	if db == nil {
		db = r.pool
	}
	var n model.Note
	query := `
		INSERT INTO notes.notes (id, user_id, subject_id, title, external_file_id, visibility, forked_from_note_id, sync_status)
		VALUES (COALESCE(NULLIF($1, '')::uuid, gen_random_uuid()), $2, $3::uuid, $4, $5, $6, $7::uuid, $8)
		RETURNING id, user_id, subject_id, title, external_file_id, visibility, likes_count, forked_from_note_id, sync_status, version, created_at, updated_at
	`
	var subjArg interface{}
	if subjectID != nil && strings.TrimSpace(*subjectID) != "" {
		if _, err := uuid.Parse(*subjectID); err != nil {
			return nil, fmt.Errorf("invalid_subject_id: %w", err)
		}
		subjArg = *subjectID
	}
	var forkArg interface{}
	if forkedFrom != nil && strings.TrimSpace(*forkedFrom) != "" {
		forkArg = *forkedFrom
	}
	if syncStatus == "" {
		syncStatus = "pending_drive"
	}
	err := db.QueryRow(ctx, query, noteID, userID, subjArg, title, externalFileID, visibility, forkArg, syncStatus).Scan(
		&n.ID, &n.UserID, &n.SubjectID, &n.Title, &n.ExternalFileID, &n.Visibility, &n.LikesCount, &n.ForkedFromNoteID, &n.SyncStatus, &n.Version, &n.CreatedAt, &n.UpdatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("create note: %w", err)
	}
	return &n, nil
}

func (r *NoteRepository) GetByID(ctx context.Context, db DBTX, id string) (*model.Note, error) {
	if db == nil {
		db = r.pool
	}
	var n model.Note
	query := `SELECT id, user_id, subject_id, title, external_file_id, visibility, likes_count, forked_from_note_id, sync_status, version, created_at, updated_at FROM notes.notes WHERE id = $1`
	err := db.QueryRow(ctx, query, id).Scan(&n.ID, &n.UserID, &n.SubjectID, &n.Title, &n.ExternalFileID, &n.Visibility, &n.LikesCount, &n.ForkedFromNoteID, &n.SyncStatus, &n.Version, &n.CreatedAt, &n.UpdatedAt)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("get note: %w", err)
	}
	return &n, nil
}

func (r *NoteRepository) ListByUser(ctx context.Context, db DBTX, userID string, cursor string, limit int) ([]*model.Note, string, error) {
	if db == nil {
		db = r.pool
	}
	if limit <= 0 || limit > 100 {
		limit = 20
	}
	var rows pgx.Rows
	var err error
	if cursor == "" {
		rows, err = db.Query(ctx, `
			SELECT id, user_id, subject_id, title, external_file_id, visibility, likes_count, forked_from_note_id, sync_status, version, created_at, updated_at
			FROM notes.notes WHERE user_id = $1 ORDER BY created_at DESC, id DESC LIMIT $2`, userID, limit+1)
	} else {
		rows, err = db.Query(ctx, `
			SELECT id, user_id, subject_id, title, external_file_id, visibility, likes_count, forked_from_note_id, sync_status, version, created_at, updated_at
			FROM notes.notes
			WHERE user_id = $1 AND (created_at, id) < ((SELECT created_at FROM notes.notes WHERE id = $2), $2::uuid)
			ORDER BY created_at DESC, id DESC LIMIT $3`, userID, cursor, limit+1)
	}
	if err != nil {
		return nil, "", fmt.Errorf("list notes: %w", err)
	}
	defer rows.Close()
	var notes []*model.Note
	for rows.Next() {
		var n model.Note
		if err := rows.Scan(&n.ID, &n.UserID, &n.SubjectID, &n.Title, &n.ExternalFileID, &n.Visibility, &n.LikesCount, &n.ForkedFromNoteID, &n.SyncStatus, &n.Version, &n.CreatedAt, &n.UpdatedAt); err != nil {
			return nil, "", fmt.Errorf("scan note: %w", err)
		}
		notes = append(notes, &n)
	}
	if err := rows.Err(); err != nil {
		return nil, "", err
	}
	var nextCursor string
	if len(notes) > limit {
		nextCursor = notes[limit-1].ID
		notes = notes[:limit]
	}
	return notes, nextCursor, nil
}

// Update aplica un cambio parcial de metadata con versionado optimista: la
// sentencia exige que la fila siga en expectedVersion y, si el UPDATE afecta una
// fila, incrementa version en la misma operación. Si no afecta ninguna fila pero
// la nota existe (otro writer ganó la carrera), devuelve ErrConflict; si la nota
// no existe devuelve (nil, nil), igual que antes.
func (r *NoteRepository) Update(ctx context.Context, db DBTX, id string, title *string, visibility *string, expectedVersion int64) (*model.Note, error) {
	if db == nil {
		db = r.pool
	}
	// Construir update dinámico
	setClauses := []string{}
	args := []any{}
	idx := 1
	if title != nil {
		setClauses = append(setClauses, fmt.Sprintf("title = $%d", idx))
		args = append(args, *title)
		idx++
	}
	if visibility != nil {
		setClauses = append(setClauses, fmt.Sprintf("visibility = $%d", idx))
		args = append(args, *visibility)
		idx++
	}
	// version se incrementa siempre: incluso un update sin campos de metadata
	// (p. ej. solo contenido en Drive) representa una nueva versión de la nota.
	setClauses = append(setClauses, "version = version + 1", "updated_at = now()")
	versionIdx := idx
	args = append(args, expectedVersion)
	idx++
	query := fmt.Sprintf(`UPDATE notes.notes SET %s WHERE id = $%d AND version = $%d RETURNING id, user_id, subject_id, title, external_file_id, visibility, likes_count, forked_from_note_id, sync_status, version, created_at, updated_at`,
		strings.Join(setClauses, ", "), idx, versionIdx)
	args = append(args, id)
	var n model.Note
	err := db.QueryRow(ctx, query, args...).Scan(&n.ID, &n.UserID, &n.SubjectID, &n.Title, &n.ExternalFileID, &n.Visibility, &n.LikesCount, &n.ForkedFromNoteID, &n.SyncStatus, &n.Version, &n.CreatedAt, &n.UpdatedAt)
	if err != nil {
		if err == pgx.ErrNoRows {
			// Sin fila afectada: distinguir nota inexistente (nil, nil) de
			// conflicto de versión (la nota existe y otro writer la cambió).
			var exists bool
			if checkErr := db.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM notes.notes WHERE id = $1)`, id).Scan(&exists); checkErr != nil {
				return nil, fmt.Errorf("update note: %w", checkErr)
			}
			if exists {
				return nil, ErrConflict
			}
			return nil, nil
		}
		return nil, fmt.Errorf("update note: %w", err)
	}
	return &n, nil
}

func (r *NoteRepository) Delete(ctx context.Context, db DBTX, id string) error {
	if db == nil {
		db = r.pool
	}
	tag, err := db.Exec(ctx, `DELETE FROM notes.notes WHERE id = $1`, id)
	if err != nil {
		return fmt.Errorf("delete note: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("not_found")
	}
	return nil
}

func (r *NoteRepository) IncrementLikes(ctx context.Context, db DBTX, noteID string, delta int) error {
	if db == nil {
		db = r.pool
	}
	_, err := db.Exec(ctx, `UPDATE notes.notes SET likes_count = likes_count + $1, updated_at = now() WHERE id = $2`, delta, noteID)
	if err != nil {
		return fmt.Errorf("inc likes: %w", err)
	}
	return nil
}

func (r *NoteRepository) UpdateExternalFileID(ctx context.Context, db DBTX, noteID string, fileID string) error {
	if db == nil {
		db = r.pool
	}
	tag, err := db.Exec(ctx, `UPDATE notes.notes SET external_file_id = $1, sync_status = 'synced', updated_at = now() WHERE id = $2`, fileID, noteID)
	if err != nil {
		return fmt.Errorf("update external file id: %w", err)
	}
	if tag.RowsAffected() != 1 {
		return ErrNotFound
	}
	return nil
}

func (r *NoteRepository) UpdateSyncStatus(ctx context.Context, db DBTX, noteID string, syncStatus string) error {
	if db == nil {
		db = r.pool
	}
	_, err := db.Exec(ctx, `UPDATE notes.notes SET sync_status = $1, updated_at = now() WHERE id = $2`, syncStatus, noteID)
	return err
}

// idempotencyColumns ordena las columnas leídas por las queries de idempotencia
// para que el Scan de las tres sentencias sea idéntico.
const idempotencyColumns = `user_id, operation, idempotency_key, resource_id, request_hash, status, expires_at`

func scanIdempotencyKey(row pgx.Row) (*model.IdempotencyKey, error) {
	var k model.IdempotencyKey
	if err := row.Scan(&k.UserID, &k.Operation, &k.IdempotencyKey, &k.ResourceID, &k.RequestHash, &k.Status, &k.ExpiresAt); err != nil {
		return nil, err
	}
	return &k, nil
}

// GetOrClaimIdempotencyKey reclama atómicamente la clave durable (user_id,
// operation, idempotency_key). Si la clave no existe, o existía con expires_at
// vencido (claim in_progress colgado por crash, o replay completed caducado),
// el INSERT ... ON CONFLICT la reclama en estado 'in_progress' y devuelve
// claimed=true: el llamador debe ejecutar la operación y luego
// CompleteIdempotencyKey, o liberar la clave con DeleteIdempotencyKey si falla.
//
// Un registro 'recoverable' (fallo posterior a la inserción local del recurso)
// se reclama de inmediato cuando el request_hash coincide, sin esperar a que
// venza expires_at; con un hash distinto NUNCA se reclama (el servicio responde
// 409 Conflict). El registro 'in_progress'/'completed' conserva la semántica
// de lease previa.
//
// preassignedResourceID pre-asocia el recurso antes de crearlo (crash recovery
// lógico): en un claim nuevo se persiste resource_id = preassignedResourceID.
// Al reclamar un registro vencido o recoverable se PRESERVA estrictamente el
// resource_id previo (COALESCE) para que el reintento recupere el id
// preasociado por el intento que murió, en lugar de generar uno nuevo y
// duplicar la nota.
//
// Si existe un registro vigente, claimed=false y se devuelve la fila para que
// el servicio decida: replay si status='completed' (comparando request_hash) o
// rechazo por operación en progreso si status='in_progress'.
func (r *NoteRepository) GetOrClaimIdempotencyKey(ctx context.Context, db DBTX, userID, operation, idempotencyKey, requestHash string, preassignedResourceID *string, expiresAt time.Time) (*model.IdempotencyKey, bool, error) {
	if db == nil {
		db = r.pool
	}
	if strings.TrimSpace(operation) == "" || strings.TrimSpace(idempotencyKey) == "" {
		return nil, false, fmt.Errorf("claim idempotency key: operation and key are required")
	}
	var preassigned interface{}
	if preassignedResourceID != nil && strings.TrimSpace(*preassignedResourceID) != "" {
		if _, err := uuid.Parse(*preassignedResourceID); err != nil {
			return nil, false, fmt.Errorf("claim idempotency key: invalid preassigned resource id: %w", err)
		}
		preassigned = *preassignedResourceID
	}
	claimQuery := `
		INSERT INTO notes.idempotency_keys (user_id, operation, idempotency_key, resource_id, request_hash, status, expires_at)
		VALUES ($1::uuid, $2, $3, $4::uuid, $5, 'in_progress', $6)
		ON CONFLICT (user_id, operation, idempotency_key) DO UPDATE
		SET request_hash = EXCLUDED.request_hash,
		    resource_id = COALESCE(notes.idempotency_keys.resource_id, EXCLUDED.resource_id),
		    status = 'in_progress',
		    expires_at = EXCLUDED.expires_at,
		    created_at = now()
		WHERE (notes.idempotency_keys.status = 'recoverable'
		       AND notes.idempotency_keys.request_hash = EXCLUDED.request_hash)
		   OR (notes.idempotency_keys.status <> 'recoverable'
		       AND notes.idempotency_keys.expires_at <= now())
		RETURNING ` + idempotencyColumns
	rec, err := scanIdempotencyKey(db.QueryRow(ctx, claimQuery, userID, operation, idempotencyKey, preassigned, requestHash, expiresAt))
	if err == nil {
		return rec, true, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return nil, false, fmt.Errorf("claim idempotency key: %w", err)
	}
	// Conflicto con un registro vigente: se lee para clasificarlo (replay vs
	// en progreso). Si desapareció entre ambas sentencias se reintenta el claim.
	selectQuery := `SELECT ` + idempotencyColumns + ` FROM notes.idempotency_keys WHERE user_id = $1 AND operation = $2 AND idempotency_key = $3`
	rec, err = scanIdempotencyKey(db.QueryRow(ctx, selectQuery, userID, operation, idempotencyKey))
	if errors.Is(err, pgx.ErrNoRows) {
		rec, err = scanIdempotencyKey(db.QueryRow(ctx, claimQuery, userID, operation, idempotencyKey, preassigned, requestHash, expiresAt))
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, false, fmt.Errorf("claim idempotency key: contention")
		}
		if err != nil {
			return nil, false, fmt.Errorf("claim idempotency key: %w", err)
		}
		return rec, true, nil
	}
	if err != nil {
		return nil, false, fmt.Errorf("claim idempotency key: %w", err)
	}
	return rec, false, nil
}

// CompleteIdempotencyKey marca la clave reclamada como 'completed' y asocia el
// recurso creado. El replay queda disponible durante la ventana de retención
// (24h) antes de que un nuevo claim pueda reutilizar la clave.
func (r *NoteRepository) CompleteIdempotencyKey(ctx context.Context, db DBTX, userID, operation, idempotencyKey, resourceID string) error {
	if db == nil {
		db = r.pool
	}
	tag, err := db.Exec(ctx, `UPDATE notes.idempotency_keys
		SET resource_id = $4::uuid, status = 'completed',
		    expires_at = now() + interval '24 hours'
		WHERE user_id = $1 AND operation = $2 AND idempotency_key = $3`, userID, operation, idempotencyKey, resourceID)
	if err != nil {
		return fmt.Errorf("complete idempotency key: %w", err)
	}
	if tag.RowsAffected() != 1 {
		return ErrNotFound
	}
	return nil
}

// DeleteIdempotencyKey libera una clave reclamada cuya operación falló, para
// permitir un reintento inmediato con la misma X-Idempotency-Key.
func (r *NoteRepository) DeleteIdempotencyKey(ctx context.Context, db DBTX, userID, operation, idempotencyKey string) error {
	if db == nil {
		db = r.pool
	}
	_, err := db.Exec(ctx, `DELETE FROM notes.idempotency_keys WHERE user_id = $1 AND operation = $2 AND idempotency_key = $3`, userID, operation, idempotencyKey)
	if err != nil {
		return fmt.Errorf("delete idempotency key: %w", err)
	}
	return nil
}

// MarkIdempotencyRecoverable transiciona la clave reclamada a 'recoverable'
// tras un fallo POSTERIOR a la inserción local del recurso: asocia el
// resourceID ya persistido y extiende la retención 24h para que un reintento
// inmediato con el mismo request_hash pueda retomarlo sin esperar a
// expires_at, preservando estrictamente el recurso existente (sin duplicados).
// lastErr describe la causa del fallo para el log del llamador.
func (r *NoteRepository) MarkIdempotencyRecoverable(ctx context.Context, db DBTX, userID, operation, idempotencyKey, resourceID string, _ string) error {
	if db == nil {
		db = r.pool
	}
	tag, err := db.Exec(ctx, `UPDATE notes.idempotency_keys
		SET resource_id = $4::uuid, status = 'recoverable',
		    expires_at = now() + interval '24 hours'
		WHERE user_id = $1 AND operation = $2 AND idempotency_key = $3`, userID, operation, idempotencyKey, resourceID)
	if err != nil {
		return fmt.Errorf("mark idempotency key recoverable: %w", err)
	}
	if tag.RowsAffected() != 1 {
		return ErrNotFound
	}
	return nil
}

func (r *NoteRepository) GetPendingSyncNotes(ctx context.Context, db DBTX) ([]*model.Note, error) {
	if db == nil {
		db = r.pool
	}
	rows, err := db.Query(ctx, `SELECT id, user_id, subject_id, title, external_file_id, visibility, likes_count, forked_from_note_id, sync_status, version, created_at, updated_at FROM notes.notes WHERE sync_status IN ('pending_drive', 'failed_sync')`)
	if err != nil {
		return nil, fmt.Errorf("get pending notes: %w", err)
	}
	var notes []*model.Note
	for rows.Next() {
		var n model.Note
		if err := rows.Scan(&n.ID, &n.UserID, &n.SubjectID, &n.Title, &n.ExternalFileID, &n.Visibility, &n.LikesCount, &n.ForkedFromNoteID, &n.SyncStatus, &n.Version, &n.CreatedAt, &n.UpdatedAt); err != nil {
			return nil, err
		}
		notes = append(notes, &n)
	}
	return notes, nil
}

var ErrNotFound = errors.New("not_found")
var ErrForbidden = errors.New("forbidden")

// ErrConflict señala una escritura rechazada por versionado optimista: la nota
// existe pero su version no coincide con la versión esperada por el writer.
var ErrConflict = errors.New("conflict")

// EnqueueDriveOperation accepts a transaction so the outbox and local mutation
// commit together. Empty logical references are stored as NULL.
func (r *NoteRepository) EnqueueDriveOperation(ctx context.Context, db DBTX, op, noteID, attID, fileID, ownerUserID string, payload map[string]any) error {
	if db == nil {
		db = r.pool
	}
	if payload == nil {
		payload = map[string]any{}
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("encode drive operation: %w", err)
	}
	// El pool de producción usa QueryExecModeSimpleProtocol: un []byte se
	// interpolaría como literal bytea ('\x7b7d') y el cast a jsonb fallaría
	// con 22P02. Como string viaja como literal de texto válido en ambos
	// modos de protocolo (simple y extendido).
	_, err = db.Exec(ctx, `INSERT INTO notes.drive_reconciliation_queue
		(operation, note_id, attachment_id, external_file_id, owner_user_id, payload)
		VALUES ($1, NULLIF($2, '')::uuid, NULLIF($3, '')::uuid, NULLIF($4, ''), $5::uuid, $6::jsonb)`,
		op, noteID, attID, fileID, ownerUserID, string(body))
	if err != nil {
		return fmt.Errorf("enqueue drive operation: %w", err)
	}
	return nil
}

// ClaimDriveOperations atomically leases jobs, including expired leases after a
// worker crash. next_attempt_at doubles as lease expiry while processing.
func (r *NoteRepository) ClaimDriveOperations(ctx context.Context, limit int, lockDuration time.Duration) ([]*model.DriveOperation, error) {
	if limit <= 0 || lockDuration <= 0 {
		return nil, fmt.Errorf("invalid claim limit or lock duration")
	}
	rows, err := r.pool.Query(ctx, `WITH candidates AS (
		SELECT id FROM notes.drive_reconciliation_queue
		WHERE status IN ('pending', 'failed', 'processing') AND next_attempt_at <= now()
		ORDER BY next_attempt_at, id LIMIT $1 FOR UPDATE SKIP LOCKED
	) UPDATE notes.drive_reconciliation_queue AS q
	SET status = 'processing', attempts = q.attempts + 1,
		next_attempt_at = now() + $2 * interval '1 microsecond', updated_at = now()
	FROM candidates WHERE q.id = candidates.id
	RETURNING q.id, q.operation, q.note_id, q.attachment_id, q.external_file_id,
		q.owner_user_id, q.payload, q.status, q.attempts, q.next_attempt_at,
		q.last_error, q.created_at, q.updated_at, q.completed_at`, limit, lockDuration.Microseconds())
	if err != nil {
		return nil, fmt.Errorf("claim drive operations: %w", err)
	}
	defer rows.Close()
	var ops []*model.DriveOperation
	for rows.Next() {
		var op model.DriveOperation
		if err := rows.Scan(&op.ID, &op.Operation, &op.NoteID, &op.AttachmentID, &op.ExternalFileID,
			&op.OwnerUserID, &op.Payload, &op.Status, &op.Attempts, &op.NextAttemptAt,
			&op.LastError, &op.CreatedAt, &op.UpdatedAt, &op.CompletedAt); err != nil {
			return nil, fmt.Errorf("scan drive operation: %w", err)
		}
		ops = append(ops, &op)
	}
	return ops, rows.Err()
}

func (r *NoteRepository) CompleteDriveOperation(ctx context.Context, id string) error {
	tag, err := r.pool.Exec(ctx, `UPDATE notes.drive_reconciliation_queue
		SET status = 'completed', completed_at = now(), updated_at = now(), last_error = NULL
		WHERE id = $1 AND status = 'processing'`, id)
	if err != nil {
		return fmt.Errorf("complete drive operation: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func (r *NoteRepository) FailDriveOperation(ctx context.Context, id, errStr string, nextAttempt time.Time) error {
	tag, err := r.pool.Exec(ctx, `UPDATE notes.drive_reconciliation_queue
		SET status = 'failed', last_error = $2, next_attempt_at = $3, updated_at = now()
		WHERE id = $1 AND status = 'processing'`, id, errStr, nextAttempt)
	if err != nil {
		return fmt.Errorf("fail drive operation: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func (r *NoteRepository) DeleteWithDriveCleanup(ctx context.Context, noteID, requesterID string) (model.NoteDriveCleanup, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return model.NoteDriveCleanup{}, fmt.Errorf("begin delete note: %w", err)
	}
	defer tx.Rollback(context.Background())
	var owner string
	var fileID *string
	err = tx.QueryRow(ctx, `SELECT user_id, external_file_id FROM notes.notes WHERE id = $1 FOR UPDATE`, noteID).Scan(&owner, &fileID)
	if errors.Is(err, pgx.ErrNoRows) {
		return model.NoteDriveCleanup{}, ErrNotFound
	}
	if err != nil {
		return model.NoteDriveCleanup{}, fmt.Errorf("lock note: %w", err)
	}
	if owner != requesterID {
		return model.NoteDriveCleanup{}, ErrForbidden
	}
	id := ""
	if fileID != nil {
		id = *fileID
	}
	result := model.NoteDriveCleanup{FileID: id}
	rows, err := tx.Query(ctx, `SELECT id, external_file_id FROM notes.note_attachments WHERE note_id=$1 ORDER BY id FOR UPDATE`, noteID)
	if err != nil {
		return model.NoteDriveCleanup{}, fmt.Errorf("read cleanup attachments: %w", err)
	}
	for rows.Next() {
		var attachment model.AttachmentDriveCleanup
		if err := rows.Scan(&attachment.ID, &attachment.ExternalFileID); err != nil {
			rows.Close()
			return model.NoteDriveCleanup{}, err
		}
		result.Attachments = append(result.Attachments, attachment)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return model.NoteDriveCleanup{}, err
	}
	for _, attachment := range result.Attachments {
		if attachment.ExternalFileID == "" {
			continue
		}
		if err := r.EnqueueDriveOperation(ctx, tx, "delete_attachment", noteID, attachment.ID, attachment.ExternalFileID, owner, nil); err != nil {
			return model.NoteDriveCleanup{}, err
		}
	}
	if id != "" {
		if err := r.EnqueueDriveOperation(ctx, tx, "delete_file", noteID, "", id, owner, nil); err != nil {
			return model.NoteDriveCleanup{}, err
		}
	}
	// Hardening de Development para instalaciones con FK restrictivas en vez
	// de CASCADE: borrar dependientes explícitamente (inocuo cuando CASCADE
	// existe; la metadata ya fue capturada arriba para el cleanup de Drive).
	for _, query := range []string{
		`DELETE FROM notes.note_attachments WHERE note_id = $1`,
		`DELETE FROM notes.saved_notes WHERE note_id = $1`,
		`DELETE FROM notes.note_likes WHERE note_id = $1`,
		`DELETE FROM notes.shared_notes WHERE note_id = $1`,
	} {
		if _, err := tx.Exec(ctx, query, noteID); err != nil {
			return model.NoteDriveCleanup{}, fmt.Errorf("cleanup note dependents: %w", err)
		}
	}
	// Scoping por owner como defensa en profundidad (la propiedad ya se
	// verificó con el lock); 0 filas = borrado concurrente -> not_found.
	tag, err := tx.Exec(ctx, `DELETE FROM notes.notes WHERE id = $1 AND user_id = $2`, noteID, requesterID)
	if err != nil {
		return model.NoteDriveCleanup{}, fmt.Errorf("delete note: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return model.NoteDriveCleanup{}, ErrNotFound
	}
	// La tabla de permisos administrados no tiene FK a notes.notes: el estado
	// local del desired-state se limpia en la misma transacción para no dejar
	// filas huérfanas si el proceso muere tras el borrado.
	if _, err := tx.Exec(ctx, `DELETE FROM notes.drive_managed_permissions WHERE note_id=$1`, noteID); err != nil {
		return model.NoteDriveCleanup{}, fmt.Errorf("cleanup managed permissions: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return model.NoteDriveCleanup{}, fmt.Errorf("commit delete note: %w", err)
	}
	return result, nil
}
