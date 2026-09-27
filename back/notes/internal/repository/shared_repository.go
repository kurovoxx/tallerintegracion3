package repository

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/kurovoxx/tallerintegracion3/back/notes/internal/model"
)

type SharedRepository struct {
	pool *pgxpool.Pool
	// lockSlots acota cuántos WithNoteLock pueden sostener a la vez una
	// transacción con pg_advisory_xact_lock. fn ejecuta sus queries con otras
	// conexiones del pool, así que si los locks consumieran todas las
	// conexiones el pool se agotaría y fn quedaría bloqueado para siempre
	// (deadlock). La capacidad MaxConns-1 garantiza que siempre quede al menos
	// una conexión libre para que fn progrese.
	lockSlots chan struct{}
}

func NewSharedRepository(pool *pgxpool.Pool) *SharedRepository {
	maxSlots := 1
	if pool != nil {
		if max := int(pool.Stat().MaxConns()); max > 1 {
			maxSlots = max - 1
		}
	}
	return &SharedRepository{pool: pool, lockSlots: make(chan struct{}, maxSlots)}
}

// WithNoteLock ejecuta fn sosteniendo un lock distribuido por nota
// (pg_advisory_xact_lock(hashtextextended(noteID, 0))::bigint) durante toda la
// operación. Es la serialización multi-réplica de los cambios de ACL
// Desired-State: Share, Unshare, Update y el reconciliador lo adquieren antes de
// mutar notes.drive_managed_permissions o permission_sync_status, de modo que
// dos instancias no intercalen lecturas/escrituras del estado deseado. La clave
// se deriva con hashtextextended (bigint, 64 bits) y se castea a ::bigint para
// ocupar el espacio de claves de 64 bits: hashtext (int4) reduce a 32 bits y
// hace colisiones y bloqueos espurios mucho más frecuentes. El lock se libera
// al terminar la transacción (commit o rollback), incluso si fn falla.
func (r *SharedRepository) WithNoteLock(ctx context.Context, noteID string, fn func(ctx context.Context) error) error {
	if fn == nil {
		return fmt.Errorf("note lock: callback is required")
	}
	if r.lockSlots != nil {
		select {
		case r.lockSlots <- struct{}{}:
			defer func() { <-r.lockSlots }()
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("note lock: begin: %w", err)
	}
	defer func() { _ = tx.Rollback(context.Background()) }()
	if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1, 0))::bigint`, noteID); err != nil {
		return fmt.Errorf("note lock: acquire: %w", err)
	}
	if err := fn(ctx); err != nil {
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("note lock: commit: %w", err)
	}
	return nil
}

func (r *SharedRepository) Create(ctx context.Context, db DBTX, noteID, groupID string, isAdminNote bool, accessMode string, followersSnapshot int) (*model.SharedNote, error) {
	if db == nil {
		db = r.pool
	}
	var s model.SharedNote
	err := db.QueryRow(ctx, `
		INSERT INTO notes.shared_notes (note_id, group_id, is_admin_note, access_mode, author_followers_snapshot, permission_sync_status)
		VALUES ($1,$2,$3,$4,$5,$6)
		RETURNING id, note_id, group_id, is_admin_note, access_mode, author_followers_snapshot, permission_sync_status, shared_at
	`, noteID, groupID, isAdminNote, accessMode, followersSnapshot, model.PermissionSyncPending).Scan(
		&s.ID, &s.NoteID, &s.GroupID, &s.IsAdminNote, &s.AccessMode, &s.AuthorFollowersSnapshot, &s.PermissionSyncStatus, &s.SharedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("create shared: %w", err)
	}
	return &s, nil
}

func (r *SharedRepository) GetByID(ctx context.Context, db DBTX, id string) (*model.SharedNote, error) {
	if db == nil {
		db = r.pool
	}
	var s model.SharedNote
	err := db.QueryRow(ctx, `SELECT id, note_id, group_id, is_admin_note, access_mode, author_followers_snapshot, permission_sync_status, shared_at FROM notes.shared_notes WHERE id=$1`, id).Scan(
		&s.ID, &s.NoteID, &s.GroupID, &s.IsAdminNote, &s.AccessMode, &s.AuthorFollowersSnapshot, &s.PermissionSyncStatus, &s.SharedAt,
	)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("get shared: %w", err)
	}
	return &s, nil
}

func (r *SharedRepository) ListByNote(ctx context.Context, db DBTX, noteID string) ([]*model.SharedNote, error) {
	if db == nil {
		db = r.pool
	}
	rows, err := db.Query(ctx, `SELECT id, note_id, group_id, is_admin_note, access_mode, author_followers_snapshot, permission_sync_status, shared_at FROM notes.shared_notes WHERE note_id=$1`, noteID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*model.SharedNote
	for rows.Next() {
		var s model.SharedNote
		if err := rows.Scan(&s.ID, &s.NoteID, &s.GroupID, &s.IsAdminNote, &s.AccessMode, &s.AuthorFollowersSnapshot, &s.PermissionSyncStatus, &s.SharedAt); err != nil {
			return nil, err
		}
		out = append(out, &s)
	}
	return out, rows.Err()
}

func (r *SharedRepository) ListByGroup(ctx context.Context, db DBTX, groupID string, limit int, cursor string) ([]*model.SharedNote, string, error) {
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
			SELECT id, note_id, group_id, is_admin_note, access_mode, author_followers_snapshot, permission_sync_status, shared_at
			FROM notes.shared_notes WHERE group_id=$1 ORDER BY is_admin_note DESC, shared_at DESC LIMIT $2`, groupID, limit+1)
	} else {
		rows, err = db.Query(ctx, `
			SELECT id, note_id, group_id, is_admin_note, access_mode, author_followers_snapshot, permission_sync_status, shared_at
			FROM notes.shared_notes WHERE group_id=$1 AND shared_at < (SELECT shared_at FROM notes.shared_notes WHERE id=$2)
			ORDER BY is_admin_note DESC, shared_at DESC LIMIT $3`, groupID, cursor, limit+1)
	}
	if err != nil {
		return nil, "", err
	}
	defer rows.Close()
	var out []*model.SharedNote
	for rows.Next() {
		var s model.SharedNote
		if err := rows.Scan(&s.ID, &s.NoteID, &s.GroupID, &s.IsAdminNote, &s.AccessMode, &s.AuthorFollowersSnapshot, &s.PermissionSyncStatus, &s.SharedAt); err != nil {
			return nil, "", err
		}
		out = append(out, &s)
	}
	if err := rows.Err(); err != nil {
		return nil, "", err
	}
	var next string
	if len(out) > limit {
		next = out[limit-1].ID
		out = out[:limit]
	}
	return out, next, nil
}

func (r *SharedRepository) Delete(ctx context.Context, db DBTX, id string) error {
	if db == nil {
		db = r.pool
	}
	tag, err := db.Exec(ctx, `DELETE FROM notes.shared_notes WHERE id=$1`, id)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("not_found")
	}
	return nil
}

func (r *SharedRepository) DeleteByNoteAndGroup(ctx context.Context, db DBTX, noteID, groupID string) error {
	if db == nil {
		db = r.pool
	}
	_, err := db.Exec(ctx, `DELETE FROM notes.shared_notes WHERE note_id=$1 AND group_id=$2`, noteID, groupID)
	return err
}

func (r *SharedRepository) DeleteAllByNote(ctx context.Context, db DBTX, noteID string) error {
	if db == nil {
		db = r.pool
	}
	_, err := db.Exec(ctx, `DELETE FROM notes.shared_notes WHERE note_id=$1`, noteID)
	return err
}

func (r *SharedRepository) DeleteByUserAndGroup(ctx context.Context, db DBTX, userID, groupID string) error {
	if db == nil {
		db = r.pool
	}
	// Borra shared_notes donde note.owner = userID y group_id = groupID
	_, err := db.Exec(ctx, `
		DELETE FROM notes.shared_notes
		WHERE group_id=$1 AND note_id IN (SELECT id FROM notes.notes WHERE user_id=$2)
	`, groupID, userID)
	return err
}

func (r *SharedRepository) HasAccess(ctx context.Context, db DBTX, noteID, groupID string) (bool, error) {
	if db == nil {
		db = r.pool
	}
	var exists bool
	err := db.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM notes.shared_notes WHERE note_id=$1 AND group_id=$2)`, noteID, groupID).Scan(&exists)
	return exists, err
}

func (r *SharedRepository) HasAnyShare(ctx context.Context, db DBTX, noteID string) (bool, error) {
	if db == nil {
		db = r.pool
	}
	var exists bool
	err := db.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM notes.shared_notes WHERE note_id=$1)`, noteID).Scan(&exists)
	return exists, err
}

// UpdatePermissionSyncStatus marca el estado de convergencia de un share.
func (r *SharedRepository) UpdatePermissionSyncStatus(ctx context.Context, db DBTX, id, status string) error {
	if db == nil {
		db = r.pool
	}
	tag, err := db.Exec(ctx, `UPDATE notes.shared_notes SET permission_sync_status=$2 WHERE id=$1`, id, status)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// UpdateNotePermissionSyncStatus marca el estado de convergencia de todos los
// shares de una nota (el estado deseado de permisos es por nota, no por share).
func (r *SharedRepository) UpdateNotePermissionSyncStatus(ctx context.Context, db DBTX, noteID, status string) error {
	if db == nil {
		db = r.pool
	}
	_, err := db.Exec(ctx, `UPDATE notes.shared_notes SET permission_sync_status=$2 WHERE note_id=$1`, noteID, status)
	return err
}

// ListNotesWithPendingPermissionSync devuelve IDs de notas con convergencia de
// permisos pendiente: shares fuera de in_sync o permisos administrados que aún
// no están aplicados/revocados en Drive.
func (r *SharedRepository) ListNotesWithPendingPermissionSync(ctx context.Context, db DBTX, limit int) ([]string, error) {
	if db == nil {
		db = r.pool
	}
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	rows, err := db.Query(ctx, `
		SELECT note_id FROM (
			SELECT note_id FROM notes.shared_notes WHERE permission_sync_status <> $2
			UNION
			SELECT note_id FROM notes.drive_managed_permissions WHERE sync_status <> $2
		) pending
		ORDER BY note_id
		LIMIT $1`, limit, model.PermissionSyncInSync)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		out = append(out, id)
	}
	return out, rows.Err()
}

// ListManagedPermissions devuelve el estado conocido de los permisos de Drive
// administrados por Notes para una nota (incluye filas pendientes/fallidas).
func (r *SharedRepository) ListManagedPermissions(ctx context.Context, db DBTX, noteID string) ([]*model.DriveManagedPermission, error) {
	if db == nil {
		db = r.pool
	}
	rows, err := db.Query(ctx, `
		SELECT id, note_id, external_file_id, principal_type, principal_key, drive_permission_id, role, sync_status, created_at, updated_at
		FROM notes.drive_managed_permissions WHERE note_id=$1
		ORDER BY principal_type, principal_key`, noteID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*model.DriveManagedPermission
	for rows.Next() {
		p := &model.DriveManagedPermission{}
		if err := rows.Scan(&p.ID, &p.NoteID, &p.ExternalFileID, &p.PrincipalType, &p.PrincipalKey, &p.DrivePermissionID, &p.Role, &p.SyncStatus, &p.CreatedAt, &p.UpdatedAt); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

// UpsertManagedPermission registra/actualiza el estado deseado de un permiso.
// La UNIQUE (note_id, principal_type, principal_key) hace la operación
// idempotente: reintentos del reconciliador no duplican filas.
func (r *SharedRepository) UpsertManagedPermission(ctx context.Context, db DBTX, p *model.DriveManagedPermission) (*model.DriveManagedPermission, error) {
	if db == nil {
		db = r.pool
	}
	if p.Role == "" {
		p.Role = "reader"
	}
	out := &model.DriveManagedPermission{}
	err := db.QueryRow(ctx, `
		INSERT INTO notes.drive_managed_permissions (note_id, external_file_id, principal_type, principal_key, drive_permission_id, role, sync_status)
		VALUES ($1,$2,$3,$4,$5,$6,$7)
		ON CONFLICT (note_id, principal_type, principal_key) DO UPDATE SET
			external_file_id = EXCLUDED.external_file_id,
			drive_permission_id = COALESCE(EXCLUDED.drive_permission_id, notes.drive_managed_permissions.drive_permission_id),
			role = EXCLUDED.role,
			sync_status = EXCLUDED.sync_status,
			updated_at = now()
		RETURNING id, note_id, external_file_id, principal_type, principal_key, drive_permission_id, role, sync_status, created_at, updated_at`,
		p.NoteID, p.ExternalFileID, p.PrincipalType, p.PrincipalKey, p.DrivePermissionID, p.Role, p.SyncStatus).Scan(
		&out.ID, &out.NoteID, &out.ExternalFileID, &out.PrincipalType, &out.PrincipalKey, &out.DrivePermissionID, &out.Role, &out.SyncStatus, &out.CreatedAt, &out.UpdatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("upsert managed permission: %w", err)
	}
	return out, nil
}

// DeleteManagedPermission elimina la fila tras revocar el permiso en Drive.
func (r *SharedRepository) DeleteManagedPermission(ctx context.Context, db DBTX, noteID, principalType, principalKey string) error {
	if db == nil {
		db = r.pool
	}
	_, err := db.Exec(ctx, `DELETE FROM notes.drive_managed_permissions WHERE note_id=$1 AND principal_type=$2 AND principal_key=$3`, noteID, principalType, principalKey)
	return err
}

// DeleteManagedPermissionsByNote limpia el estado local cuando la nota ya no
// existe (borrado en cascada lógico: la tabla no tiene FK a notes.notes).
func (r *SharedRepository) DeleteManagedPermissionsByNote(ctx context.Context, db DBTX, noteID string) error {
	if db == nil {
		db = r.pool
	}
	_, err := db.Exec(ctx, `DELETE FROM notes.drive_managed_permissions WHERE note_id=$1`, noteID)
	return err
}

// MarkManagedPermissionsPending fuerza la convergencia de todos los permisos de
// una nota en la próxima pasada del reconciliador (p. ej. Unshare sin poder
// calcular el estado deseado por una caída de Social).
func (r *SharedRepository) MarkManagedPermissionsPending(ctx context.Context, db DBTX, noteID string) error {
	if db == nil {
		db = r.pool
	}
	_, err := db.Exec(ctx, `UPDATE notes.drive_managed_permissions SET sync_status=$2, updated_at=now() WHERE note_id=$1 AND sync_status <> $2`, noteID, model.PermissionSyncPending)
	return err
}
