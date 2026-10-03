package repository

import (
	"context"
	"errors"
	"os"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Regresión del 500 en DELETE /notes/:id (y DELETE attachment):
// el pool de producción usa QueryExecModeSimpleProtocol. En ese modo un
// payload []byte se interpola como literal bytea ('\x7b7d') y el cast a
// jsonb del outbox fallaba con 22P02, abortando toda la transacción de
// borrado. Los tests de integración existentes usan el protocolo extendido
// por defecto, por eso nunca lo detectaron.
//
// Estos tests fuerzan SimpleProtocol sobre una BD descartable.
func simpleProtocolTestPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	dsn := os.Getenv("NOTES_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("NOTES_TEST_DATABASE_URL is not set")
	}
	ctx := context.Background()
	admin, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(admin.Close)
	name := "notes_simpleproto_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	quoted := pgx.Identifier{name}.Sanitize()
	if _, err := admin.Exec(ctx, "CREATE DATABASE "+quoted); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if _, err := admin.Exec(ctx, "DROP DATABASE "+quoted); err != nil {
			t.Error(err)
		}
	})
	cfg, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		t.Fatal(err)
	}
	cfg.ConnConfig.Database = name
	// Igual que repository.NewPool en producción.
	cfg.ConnConfig.DefaultQueryExecMode = pgx.QueryExecModeSimpleProtocol
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	if _, err := pool.Exec(ctx, `CREATE EXTENSION IF NOT EXISTS pgcrypto`); err != nil {
		t.Fatal(err)
	}
	_, err = pool.Exec(ctx, `CREATE SCHEMA notes;
		CREATE TABLE notes.notes (id uuid PRIMARY KEY, user_id uuid NOT NULL, external_file_id text);
		CREATE TABLE notes.note_attachments (id uuid PRIMARY KEY, note_id uuid REFERENCES notes.notes(id) ON DELETE CASCADE, external_file_id text NOT NULL);
		CREATE TABLE notes.drive_reconciliation_queue (
			id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
			operation text NOT NULL,
			note_id uuid,
			attachment_id uuid,
			external_file_id text,
			owner_user_id uuid NOT NULL,
			payload jsonb NOT NULL DEFAULT '{}',
			status text NOT NULL DEFAULT 'pending',
			attempts integer NOT NULL DEFAULT 0,
			next_attempt_at timestamptz NOT NULL DEFAULT now(),
			last_error text,
			created_at timestamptz DEFAULT now(),
			updated_at timestamptz DEFAULT now(),
			completed_at timestamptz
		);
		CREATE TABLE notes.drive_managed_permissions (id uuid PRIMARY KEY DEFAULT gen_random_uuid(), note_id uuid NOT NULL);`)
	if err != nil {
		t.Fatal(err)
	}
	return pool
}

func TestSimpleProtocolDeleteNoteWithAttachments(t *testing.T) {
	pool := simpleProtocolTestPool(t)
	ctx := context.Background()
	notes := NewNoteRepository(pool)
	owner := uuid.NewString()
	noteID := uuid.NewString()
	attID := uuid.NewString()
	if _, err := pool.Exec(ctx, `INSERT INTO notes.notes (id, user_id, external_file_id) VALUES ($1, $2, $3)`, noteID, owner, "md-file"); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO notes.note_attachments (id, note_id, external_file_id) VALUES ($1, $2, $3)`, attID, noteID, "att-file"); err != nil {
		t.Fatal(err)
	}
	cleanup, err := notes.DeleteWithDriveCleanup(ctx, noteID, owner)
	if err != nil {
		t.Fatalf("delete en SimpleProtocol: %v", err)
	}
	if cleanup.FileID != "md-file" || len(cleanup.Attachments) != 1 {
		t.Fatalf("cleanup inesperado: %+v", cleanup)
	}
	var n int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM notes.notes WHERE id=$1`, noteID).Scan(&n); err != nil || n != 0 {
		t.Fatalf("nota no borrada: n=%d err=%v", n, err)
	}
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM notes.note_attachments WHERE note_id=$1`, noteID).Scan(&n); err != nil || n != 0 {
		t.Fatalf("adjuntos no borrados: n=%d err=%v", n, err)
	}
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM notes.drive_reconciliation_queue WHERE note_id=$1`, noteID).Scan(&n); err != nil || n != 2 {
		t.Fatalf("outbox debe tener 2 ops (delete_file + delete_attachment): n=%d err=%v", n, err)
	}
	if _, err := notes.DeleteWithDriveCleanup(ctx, noteID, owner); !errors.Is(err, ErrNotFound) {
		t.Fatalf("segundo delete debe ser NotFound, fue: %v", err)
	}
}

func TestSimpleProtocolDeleteSingleAttachment(t *testing.T) {
	pool := simpleProtocolTestPool(t)
	ctx := context.Background()
	attachments := NewAttachmentRepository(pool)
	owner := uuid.NewString()
	noteID := uuid.NewString()
	attID := uuid.NewString()
	if _, err := pool.Exec(ctx, `INSERT INTO notes.notes (id, user_id, external_file_id) VALUES ($1, $2, $3)`, noteID, owner, "md-file"); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO notes.note_attachments (id, note_id, external_file_id) VALUES ($1, $2, $3)`, attID, noteID, "att-file"); err != nil {
		t.Fatal(err)
	}
	fileID, err := attachments.DeleteAttachmentWithDriveCleanup(ctx, noteID, attID, owner)
	if err != nil {
		t.Fatalf("delete attachment en SimpleProtocol: %v", err)
	}
	if fileID != "att-file" {
		t.Fatalf("fileID inesperado: %q", fileID)
	}
	var n int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM notes.note_attachments WHERE id=$1`, attID).Scan(&n); err != nil || n != 0 {
		t.Fatalf("adjunto no borrado: n=%d err=%v", n, err)
	}
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM notes.notes WHERE id=$1`, noteID).Scan(&n); err != nil || n != 1 {
		t.Fatalf("la nota debe seguir existiendo: n=%d err=%v", n, err)
	}
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM notes.drive_reconciliation_queue WHERE attachment_id=$1`, attID).Scan(&n); err != nil || n != 1 {
		t.Fatalf("outbox debe tener 1 op delete_attachment: n=%d err=%v", n, err)
	}
}
