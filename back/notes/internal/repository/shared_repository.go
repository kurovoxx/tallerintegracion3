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
}

func NewSharedRepository(pool *pgxpool.Pool) *SharedRepository {
	return &SharedRepository{pool: pool}
}

func (r *SharedRepository) Create(ctx context.Context, db DBTX, noteID, groupID string, isAdminNote bool, accessMode string, followersSnapshot int) (*model.SharedNote, error) {
	if db == nil {
		db = r.pool
	}
	var s model.SharedNote
	err := db.QueryRow(ctx, `
		INSERT INTO notes.shared_notes (note_id, group_id, is_admin_note, access_mode, author_followers_snapshot)
		VALUES ($1,$2,$3,$4,$5)
		RETURNING id, note_id, group_id, is_admin_note, access_mode, author_followers_snapshot, shared_at
	`, noteID, groupID, isAdminNote, accessMode, followersSnapshot).Scan(
		&s.ID, &s.NoteID, &s.GroupID, &s.IsAdminNote, &s.AccessMode, &s.AuthorFollowersSnapshot, &s.SharedAt,
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
	err := db.QueryRow(ctx, `SELECT id, note_id, group_id, is_admin_note, access_mode, author_followers_snapshot, shared_at FROM notes.shared_notes WHERE id=$1`, id).Scan(
		&s.ID, &s.NoteID, &s.GroupID, &s.IsAdminNote, &s.AccessMode, &s.AuthorFollowersSnapshot, &s.SharedAt,
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
	rows, err := db.Query(ctx, `SELECT id, note_id, group_id, is_admin_note, access_mode, author_followers_snapshot, shared_at FROM notes.shared_notes WHERE note_id=$1`, noteID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*model.SharedNote
	for rows.Next() {
		var s model.SharedNote
		if err := rows.Scan(&s.ID, &s.NoteID, &s.GroupID, &s.IsAdminNote, &s.AccessMode, &s.AuthorFollowersSnapshot, &s.SharedAt); err != nil {
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
			SELECT id, note_id, group_id, is_admin_note, access_mode, author_followers_snapshot, shared_at
			FROM notes.shared_notes WHERE group_id=$1 ORDER BY is_admin_note DESC, shared_at DESC LIMIT $2`, groupID, limit+1)
	} else {
		rows, err = db.Query(ctx, `
			SELECT id, note_id, group_id, is_admin_note, access_mode, author_followers_snapshot, shared_at
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
		if err := rows.Scan(&s.ID, &s.NoteID, &s.GroupID, &s.IsAdminNote, &s.AccessMode, &s.AuthorFollowersSnapshot, &s.SharedAt); err != nil {
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
