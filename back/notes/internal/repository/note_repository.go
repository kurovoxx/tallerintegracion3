package repository

import (
	"context"
	"fmt"
	"strings"

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

// Create inserta metadata en notes.notes. externalFileID puede ser nil brevemente.
func (r *NoteRepository) Create(ctx context.Context, db DBTX, userID string, subjectID *string, title string, externalFileID *string, visibility string, forkedFrom *string) (*model.Note, error) {
	if db == nil {
		db = r.pool
	}
	var n model.Note
	// Generar UUID en DB (gen_random_uuid) o pasar uno si se requiere
	query := `
		INSERT INTO notes.notes (user_id, subject_id, title, external_file_id, visibility, forked_from_note_id)
		VALUES ($1, $2::uuid, $3, $4, $5, $6::uuid)
		RETURNING id, user_id, subject_id, title, external_file_id, visibility, likes_count, forked_from_note_id, created_at, updated_at
	`
	// subjectID y forkedFrom pueden ser "" => NULL
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
	// Si pool es usado, QueryRow funciona; si es Tx también
	err := db.QueryRow(ctx, query, userID, subjArg, title, externalFileID, visibility, forkArg).Scan(
		&n.ID, &n.UserID, &n.SubjectID, &n.Title, &n.ExternalFileID, &n.Visibility, &n.LikesCount, &n.ForkedFromNoteID, &n.CreatedAt, &n.UpdatedAt,
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
	query := `SELECT id, user_id, subject_id, title, external_file_id, visibility, likes_count, forked_from_note_id, created_at, updated_at FROM notes.notes WHERE id = $1`
	err := db.QueryRow(ctx, query, id).Scan(&n.ID, &n.UserID, &n.SubjectID, &n.Title, &n.ExternalFileID, &n.Visibility, &n.LikesCount, &n.ForkedFromNoteID, &n.CreatedAt, &n.UpdatedAt)
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
	// cursor es id o timestamp; usamos id como cursor simple ordenado por created_at DESC, id DESC
	// Si cursor vacío => primera página
	var rows pgx.Rows
	var err error
	if cursor == "" {
		rows, err = db.Query(ctx, `
			SELECT id, user_id, subject_id, title, external_file_id, visibility, likes_count, forked_from_note_id, created_at, updated_at
			FROM notes.notes WHERE user_id = $1 ORDER BY created_at DESC, id DESC LIMIT $2`, userID, limit+1)
	} else {
		// cursor validación: debe ser uuid existente; buscamos created_at del cursor para paginar
		// Simplificación: cursor = id, filtrar created_at < (select created_at from notes where id=$cursor) o (created_at = ... y id < ...)
		rows, err = db.Query(ctx, `
			SELECT id, user_id, subject_id, title, external_file_id, visibility, likes_count, forked_from_note_id, created_at, updated_at
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
		if err := rows.Scan(&n.ID, &n.UserID, &n.SubjectID, &n.Title, &n.ExternalFileID, &n.Visibility, &n.LikesCount, &n.ForkedFromNoteID, &n.CreatedAt, &n.UpdatedAt); err != nil {
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

func (r *NoteRepository) Update(ctx context.Context, db DBTX, id string, title *string, visibility *string) (*model.Note, error) {
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
	if len(setClauses) == 0 {
		return r.GetByID(ctx, db, id)
	}
	setClauses = append(setClauses, "updated_at = now()")
	query := fmt.Sprintf(`UPDATE notes.notes SET %s WHERE id = $%d RETURNING id, user_id, subject_id, title, external_file_id, visibility, likes_count, forked_from_note_id, created_at, updated_at`,
		strings.Join(setClauses, ", "), idx)
	args = append(args, id)
	var n model.Note
	err := db.QueryRow(ctx, query, args...).Scan(&n.ID, &n.UserID, &n.SubjectID, &n.Title, &n.ExternalFileID, &n.Visibility, &n.LikesCount, &n.ForkedFromNoteID, &n.CreatedAt, &n.UpdatedAt)
	if err != nil {
		if err == pgx.ErrNoRows {
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
	_, err := db.Exec(ctx, `UPDATE notes.notes SET external_file_id = $1, updated_at = now() WHERE id = $2`, fileID, noteID)
	return err
}
