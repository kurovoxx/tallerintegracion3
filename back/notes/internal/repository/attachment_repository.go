package repository

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/kurovoxx/tallerintegracion3/back/notes/internal/model"
)

type AttachmentRepository struct {
	pool *pgxpool.Pool
}

func NewAttachmentRepository(pool *pgxpool.Pool) *AttachmentRepository {
	return &AttachmentRepository{pool: pool}
}

func (r *AttachmentRepository) Create(ctx context.Context, db DBTX, noteID, externalFileID, fileURL, fileType string, fileName *string, fileSize *int, isInline bool) (*model.Attachment, error) {
	if db == nil {
		db = r.pool
	}
	var a model.Attachment
	query := `
		INSERT INTO notes.note_attachments (note_id, external_file_id, file_url, file_type, file_name, file_size_bytes, is_inline)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
		RETURNING id, note_id, external_file_id, file_url, file_type, file_name, file_size_bytes, is_inline, created_at
	`
	err := db.QueryRow(ctx, query, noteID, externalFileID, fileURL, fileType, fileName, fileSize, isInline).Scan(
		&a.ID, &a.NoteID, &a.ExternalFileID, &a.FileURL, &a.FileType, &a.FileName, &a.FileSizeBytes, &a.IsInline, &a.CreatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("create attachment: %w", err)
	}
	return &a, nil
}

func (r *AttachmentRepository) GetByID(ctx context.Context, db DBTX, id string) (*model.Attachment, error) {
	if db == nil {
		db = r.pool
	}
	var a model.Attachment
	err := db.QueryRow(ctx, `SELECT id, note_id, external_file_id, file_url, file_type, file_name, file_size_bytes, is_inline, created_at FROM notes.note_attachments WHERE id=$1`, id).Scan(
		&a.ID, &a.NoteID, &a.ExternalFileID, &a.FileURL, &a.FileType, &a.FileName, &a.FileSizeBytes, &a.IsInline, &a.CreatedAt,
	)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("get attachment: %w", err)
	}
	return &a, nil
}

func (r *AttachmentRepository) ListByNote(ctx context.Context, db DBTX, noteID string) ([]*model.Attachment, error) {
	if db == nil {
		db = r.pool
	}
	rows, err := db.Query(ctx, `SELECT id, note_id, external_file_id, file_url, file_type, file_name, file_size_bytes, is_inline, created_at FROM notes.note_attachments WHERE note_id=$1 ORDER BY created_at DESC`, noteID)
	if err != nil {
		return nil, fmt.Errorf("list attachments: %w", err)
	}
	defer rows.Close()
	var out []*model.Attachment
	for rows.Next() {
		var a model.Attachment
		if err := rows.Scan(&a.ID, &a.NoteID, &a.ExternalFileID, &a.FileURL, &a.FileType, &a.FileName, &a.FileSizeBytes, &a.IsInline, &a.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, &a)
	}
	return out, rows.Err()
}

func (r *AttachmentRepository) Delete(ctx context.Context, db DBTX, id string) error {
	if db == nil {
		db = r.pool
	}
	tag, err := db.Exec(ctx, `DELETE FROM notes.note_attachments WHERE id=$1`, id)
	if err != nil {
		return fmt.Errorf("delete attachment: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("not_found")
	}
	return nil
}

func (r *AttachmentRepository) DeleteAttachmentWithDriveCleanup(ctx context.Context, noteID, attachmentID, requesterID string) (string, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return "", fmt.Errorf("begin delete attachment: %w", err)
	}
	defer tx.Rollback(context.Background())
	// Lock the parent first, matching note deletion's lock order.
	var owner string
	err = tx.QueryRow(ctx, `SELECT user_id FROM notes.notes WHERE id = $1 FOR UPDATE`, noteID).Scan(&owner)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", ErrNotFound
	}
	if err != nil {
		return "", fmt.Errorf("lock attachment owner: %w", err)
	}
	if owner != requesterID {
		return "", ErrForbidden
	}
	var fileID string
	err = tx.QueryRow(ctx, `SELECT external_file_id FROM notes.note_attachments
		WHERE id = $1 AND note_id = $2 FOR UPDATE`, attachmentID, noteID).Scan(&fileID)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", ErrNotFound
	}
	if err != nil {
		return "", fmt.Errorf("lock attachment: %w", err)
	}
	if fileID != "" {
		if err := NewNoteRepository(r.pool).EnqueueDriveOperation(ctx, tx, "delete_attachment", noteID, attachmentID, fileID, owner, nil); err != nil {
			return "", err
		}
	}
	if err := r.Delete(ctx, tx, attachmentID); err != nil {
		return "", err
	}
	if err := tx.Commit(ctx); err != nil {
		return "", fmt.Errorf("commit delete attachment: %w", err)
	}
	return fileID, nil
}
