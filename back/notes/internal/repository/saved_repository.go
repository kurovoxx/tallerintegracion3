package repository

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/kurovoxx/tallerintegracion3/back/notes/internal/model"
)

type SavedRepository struct {
	pool *pgxpool.Pool
}

func NewSavedRepository(pool *pgxpool.Pool) *SavedRepository {
	return &SavedRepository{pool: pool}
}

func (r *SavedRepository) Save(ctx context.Context, db DBTX, userID, noteID string) (*model.SavedNote, error) {
	if db == nil {
		db = r.pool
	}
	var s model.SavedNote
	err := db.QueryRow(ctx, `
		INSERT INTO notes.saved_notes (user_id, note_id)
		VALUES ($1,$2)
		RETURNING id, user_id, note_id, saved_at`, userID, noteID).Scan(&s.ID, &s.UserID, &s.NoteID, &s.SavedAt)
	if err != nil {
		// 23505 unique violation
		if isUniqueViolation(err) {
			return nil, fmt.Errorf("already_saved: %w", err)
		}
		return nil, fmt.Errorf("save note: %w", err)
	}
	return &s, nil
}

func (r *SavedRepository) Exists(ctx context.Context, db DBTX, userID, noteID string) (bool, error) {
	if db == nil {
		db = r.pool
	}
	var exists bool
	err := db.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM notes.saved_notes WHERE user_id=$1 AND note_id=$2)`, userID, noteID).Scan(&exists)
	return exists, err
}

func (r *SavedRepository) Delete(ctx context.Context, db DBTX, userID, noteID string) error {
	if db == nil {
		db = r.pool
	}
	_, err := db.Exec(ctx, `DELETE FROM notes.saved_notes WHERE user_id=$1 AND note_id=$2`, userID, noteID)
	return err
}

func (r *SavedRepository) ListByUser(ctx context.Context, db DBTX, userID string) ([]*model.SavedNote, error) {
	if db == nil {
		db = r.pool
	}
	rows, err := db.Query(ctx, `SELECT id, user_id, note_id, saved_at FROM notes.saved_notes WHERE user_id=$1 ORDER BY saved_at DESC`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*model.SavedNote
	for rows.Next() {
		var s model.SavedNote
		if err := rows.Scan(&s.ID, &s.UserID, &s.NoteID, &s.SavedAt); err != nil {
			return nil, err
		}
		out = append(out, &s)
	}
	return out, rows.Err()
}

// CountByNotes devuelve los saves_count de toda la página en una consulta.
// Los apuntes sin guardados no aparecen en el mapa (0 implícito).
func (r *SavedRepository) CountByNotes(ctx context.Context, db DBTX, noteIDs []string) (map[string]int, error) {
	out := make(map[string]int, len(noteIDs))
	if len(noteIDs) == 0 {
		return out, nil
	}
	if db == nil {
		db = r.pool
	}
	rows, err := db.Query(ctx, `SELECT note_id, count(*) FROM notes.saved_notes WHERE note_id = ANY($1::uuid[]) GROUP BY note_id`, noteIDs)
	if err != nil {
		return nil, fmt.Errorf("count saved by notes: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var noteID string
		var count int
		if err := rows.Scan(&noteID, &count); err != nil {
			return nil, fmt.Errorf("scan saved count: %w", err)
		}
		out[noteID] = count
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("count saved rows: %w", err)
	}
	return out, nil
}

// ExistsByNotes resuelve en una sola consulta qué apuntes de la página guardó
// el usuario (evita N+1 al listar).
func (r *SavedRepository) ExistsByNotes(ctx context.Context, db DBTX, userID string, noteIDs []string) (map[string]bool, error) {
	out := make(map[string]bool, len(noteIDs))
	if len(noteIDs) == 0 {
		return out, nil
	}
	if db == nil {
		db = r.pool
	}
	rows, err := db.Query(ctx, `SELECT note_id FROM notes.saved_notes WHERE user_id = $1 AND note_id = ANY($2::uuid[])`, userID, noteIDs)
	if err != nil {
		return nil, fmt.Errorf("exists saved by notes: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var noteID string
		if err := rows.Scan(&noteID); err != nil {
			return nil, fmt.Errorf("scan saved note_id: %w", err)
		}
		out[noteID] = true
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("exists saved rows: %w", err)
	}
	return out, nil
}

// CountByNote devuelve cuántos usuarios tienen guardado el apunte (saves_count).
func (r *SavedRepository) CountByNote(ctx context.Context, db DBTX, noteID string) (int, error) {
	if db == nil {
		db = r.pool
	}
	var cnt int
	err := db.QueryRow(ctx, `SELECT count(*) FROM notes.saved_notes WHERE note_id=$1`, noteID).Scan(&cnt)
	return cnt, err
}

func isUniqueViolation(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	return contains(msg, "23505") || contains(msg, "duplicate") || contains(msg, "already_saved")
}

func contains(s, sub string) bool {
	return len(s) >= len(sub) && search(s, sub)
}
func search(s, sub string) bool {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}

// pgx ErrNoRows helper
var _ = pgx.ErrNoRows
