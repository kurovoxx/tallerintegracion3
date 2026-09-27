package repository

import (
	"context"
	"errors"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// The URL must allow CREATE DATABASE; each test uses a disposable database.
func outboxTestPool(t *testing.T) *pgxpool.Pool {
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
	name := "notes_outbox_" + strings.ReplaceAll(uuid.NewString(), "-", "")
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
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	_, err = pool.Exec(ctx, `CREATE SCHEMA notes;
		CREATE TABLE notes.notes (id uuid PRIMARY KEY, user_id uuid NOT NULL, external_file_id text);
		CREATE TABLE notes.note_attachments (id uuid PRIMARY KEY, note_id uuid REFERENCES notes.notes(id) ON DELETE CASCADE, external_file_id text NOT NULL);`)
	if err != nil {
		t.Fatal(err)
	}
	migration, err := os.ReadFile("../../db/migrations/001_reconciliation_and_idempotency.sql")
	if err != nil {
		t.Fatal(err)
	}
	ddl := strings.Split(strings.Split(string(migration), "CREATE TABLE IF NOT EXISTS notes.drive_reconciliation_queue")[1], "CREATE INDEX")[0]
	if _, err := pool.Exec(ctx, "CREATE TABLE IF NOT EXISTS notes.drive_reconciliation_queue"+ddl); err != nil {
		t.Fatal(err)
	}
	idemDDL := strings.Split(strings.Split(string(migration), "CREATE TABLE IF NOT EXISTS notes.idempotency_keys")[1], "CREATE TABLE IF NOT EXISTS notes.drive_managed_permissions")[0]
	if _, err := pool.Exec(ctx, "CREATE TABLE IF NOT EXISTS notes.idempotency_keys"+idemDDL); err != nil {
		t.Fatal(err)
	}
	permDDL := strings.Split(strings.Split(string(migration), "CREATE TABLE IF NOT EXISTS notes.drive_managed_permissions")[1], "-- These constraints")[0]
	if _, err := pool.Exec(ctx, "CREATE TABLE IF NOT EXISTS notes.drive_managed_permissions"+permDDL); err != nil {
		t.Fatal(err)
	}
	return pool
}

func TestDriveCleanupTransactions(t *testing.T) {
	pool := outboxTestPool(t)
	ctx := context.Background()
	notes, attachments := NewNoteRepository(pool), NewAttachmentRepository(pool)
	owner, other := uuid.NewString(), uuid.NewString()
	insert := func(file any) string {
		t.Helper()
		id := uuid.NewString()
		if _, err := pool.Exec(ctx, `INSERT INTO notes.notes VALUES ($1, $2, $3)`, id, owner, file); err != nil {
			t.Fatal(err)
		}
		return id
	}
	count := func(table string) int {
		t.Helper()
		var n int
		if err := pool.QueryRow(ctx, "SELECT count(*) FROM notes."+table).Scan(&n); err != nil {
			t.Fatal(err)
		}
		return n
	}
	id := insert("drive-note")
	if _, err := notes.DeleteWithDriveCleanup(ctx, id, other); !errors.Is(err, ErrForbidden) {
		t.Fatalf("ownership: %v", err)
	}
	if count("drive_reconciliation_queue") != 0 || count("notes") != 1 {
		t.Fatal("unauthorized deletion mutated data")
	}
	file, err := notes.DeleteWithDriveCleanup(ctx, id, owner)
	if err != nil || file != "drive-note" {
		t.Fatalf("delete: %q %v", file, err)
	}
	if count("notes") != 0 || count("drive_reconciliation_queue") != 1 {
		t.Fatal("delete/outbox not committed together")
	}
	if _, err := notes.DeleteWithDriveCleanup(ctx, id, owner); !errors.Is(err, ErrNotFound) {
		t.Fatalf("second delete: %v", err)
	}
	if _, err := notes.DeleteWithDriveCleanup(ctx, insert(nil), owner); err != nil {
		t.Fatal(err)
	}
	if count("drive_reconciliation_queue") != 1 {
		t.Fatal("queued cleanup without file")
	}

	// Failure after enqueue must roll back the job and preserve the local row.
	id = insert("protected-file")
	if _, err := pool.Exec(ctx, `CREATE TABLE notes.block_delete (note_id uuid REFERENCES notes.notes(id))`); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO notes.block_delete VALUES ($1)`, id); err != nil {
		t.Fatal(err)
	}
	if _, err := notes.DeleteWithDriveCleanup(ctx, id, owner); err == nil {
		t.Fatal("expected foreign key failure")
	}
	if count("drive_reconciliation_queue") != 1 || count("notes") != 1 {
		t.Fatal("failed deletion did not roll back")
	}

	attID := uuid.NewString()
	if _, err := pool.Exec(ctx, `INSERT INTO notes.note_attachments VALUES ($1, $2, 'drive-attachment')`, attID, id); err != nil {
		t.Fatal(err)
	}
	if _, err := attachments.DeleteAttachmentWithDriveCleanup(ctx, id, attID, other); !errors.Is(err, ErrForbidden) {
		t.Fatalf("attachment ownership: %v", err)
	}
	if _, err := attachments.DeleteAttachmentWithDriveCleanup(ctx, insert(nil), attID, owner); !errors.Is(err, ErrNotFound) {
		t.Fatalf("cross-note attachment: %v", err)
	}
	file, err = attachments.DeleteAttachmentWithDriveCleanup(ctx, id, attID, owner)
	if err != nil || file != "drive-attachment" {
		t.Fatalf("attachment delete: %q %v", file, err)
	}
	if count("note_attachments") != 0 || count("drive_reconciliation_queue") != 2 {
		t.Fatal("attachment cleanup not committed")
	}
	jobs, err := notes.ClaimDriveOperations(ctx, 10, time.Minute)
	if err != nil || len(jobs) != 2 {
		t.Fatalf("claim deleted resources: %v %v", jobs, err)
	}
	for _, job := range jobs {
		if job.OwnerUserID != owner || job.NoteID == nil || job.ExternalFileID == nil {
			t.Fatalf("missing durable cleanup context: %+v", job)
		}
	}
}

func TestDriveQueueConcurrentClaimsAndRetry(t *testing.T) {
	pool := outboxTestPool(t)
	ctx := context.Background()
	repo := NewNoteRepository(pool)
	owner := uuid.NewString()
	for i := 0; i < 20; i++ {
		if err := repo.EnqueueDriveOperation(ctx, nil, "delete_file", "", "", "file", owner, map[string]any{"reason": "retry"}); err != nil {
			t.Fatal(err)
		}
	}
	var wg sync.WaitGroup
	var mu sync.Mutex
	seen := map[string]bool{}
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			jobs, err := repo.ClaimDriveOperations(ctx, 5, time.Minute)
			if err != nil {
				t.Error(err)
				return
			}
			mu.Lock()
			defer mu.Unlock()
			for _, job := range jobs {
				if seen[job.ID] {
					t.Errorf("duplicate concurrent claim: %s", job.ID)
				}
				seen[job.ID] = true
				if job.Attempts != 1 || job.Status != "processing" {
					t.Errorf("bad claim state: %+v", job)
				}
			}
		}()
	}
	wg.Wait()
	if len(seen) != 20 {
		t.Fatalf("claimed %d of 20", len(seen))
	}
	jobs, err := repo.ClaimDriveOperations(ctx, 20, time.Minute)
	if err != nil || len(jobs) != 0 {
		t.Fatalf("leased jobs reclaimed early: %v %v", jobs, err)
	}
	var id string
	for candidate := range seen {
		id = candidate
		break
	}
	if err := repo.FailDriveOperation(ctx, id, "upstream unavailable", time.Now().Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `UPDATE notes.drive_reconciliation_queue SET next_attempt_at = now() - interval '1 second' WHERE id = $1`, id); err != nil {
		t.Fatal(err)
	}
	jobs, err = repo.ClaimDriveOperations(ctx, 1, time.Minute)
	if err != nil || len(jobs) != 1 || jobs[0].ID != id || jobs[0].Attempts != 2 || jobs[0].LastError == nil {
		t.Fatalf("failed retry: %v %v", jobs, err)
	}
	// Expire the active lease to simulate a worker crash.
	if _, err := pool.Exec(ctx, `UPDATE notes.drive_reconciliation_queue SET next_attempt_at = now() - interval '1 second' WHERE id = $1`, id); err != nil {
		t.Fatal(err)
	}
	jobs, err = repo.ClaimDriveOperations(ctx, 1, time.Minute)
	if err != nil || len(jobs) != 1 || jobs[0].Attempts != 3 {
		t.Fatalf("crash recovery: %v %v", jobs, err)
	}
	if err := repo.CompleteDriveOperation(ctx, id); err != nil {
		t.Fatal(err)
	}
	var status string
	var completed *time.Time
	if err := pool.QueryRow(ctx, `SELECT status, completed_at FROM notes.drive_reconciliation_queue WHERE id = $1`, id).Scan(&status, &completed); err != nil {
		t.Fatal(err)
	}
	if status != "completed" || completed == nil {
		t.Fatal("completion not persisted")
	}
	if err := repo.FailDriveOperation(ctx, id, "late failure", time.Now()); !errors.Is(err, ErrNotFound) {
		t.Fatalf("completed job regressed: %v", err)
	}
}
