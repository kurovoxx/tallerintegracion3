package service

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/kurovoxx/tallerintegracion3/back/notes/internal/drive"
)

func TestReconcilePreservesEmptyAndUnavailableNotes(t *testing.T) {
	for _, status := range []string{"pending_drive", "failed_sync"} {
		for _, unavailable := range []bool{false, true} {
			t.Run(status+map[bool]string{false: "/empty", true: "/unavailable"}[unavailable], func(t *testing.T) {
				svc, client, notes, _, _, _, _, _ := newTestService()
				ctx := context.Background()
				owner := uuid.NewString()
				file, err := client.CreateFile(ctx, owner, "", "empty.md", "")
				if err != nil {
					t.Fatal(err)
				}
				note, err := notes.Create(ctx, "", owner, nil, "empty", &file, "private", nil, status)
				if err != nil {
					t.Fatal(err)
				}
				notes.mu.Lock()
				notes.notes[note.ID].CreatedAt = time.Now().Add(-time.Hour)
				notes.notes[note.ID].UpdatedAt = time.Now().Add(-time.Hour)
				notes.mu.Unlock()
				want := "synced"
				if unavailable {
					client.InjectGetError(file, &drive.DriveError{Code: 500, Message: "temporary outage"})
					want = "failed_sync"
				}
				if err := svc.ReconcilePendingNotes(ctx); err != nil {
					t.Fatal(err)
				}
				got, err := notes.GetByID(ctx, note.ID)
				if err != nil || got == nil || got.SyncStatus != want || !client.HasFile(file) {
					t.Fatalf("note/file must survive, status=%s: note=%+v error=%v", want, got, err)
				}
			})
		}
	}
}

func TestReconcileOutboxRetriesDeleteAndCompletes(t *testing.T) {
	svc, client, notes, _, _, _, _, _ := newTestService()
	ctx := context.Background()
	owner := uuid.NewString()
	note, err := svc.Create(ctx, owner, "delete", nil, "private", nil, "")
	if err != nil {
		t.Fatal(err)
	}
	client.DeleteErr = &drive.DriveError{Code: 500, Message: "temporary outage"}
	if err := svc.Delete(ctx, owner, note.ID); err != nil {
		t.Fatal(err)
	}
	if len(notes.driveOperations) != 1 {
		t.Fatalf("transaction must enqueue exactly one durable job, got %d", len(notes.driveOperations))
	}
	if err := svc.ReconcilePendingNotes(ctx); err != nil {
		t.Fatal(err)
	}
	for _, job := range notes.driveOperations {
		if job.Status != "failed" || job.Attempts != 1 || job.LastError == nil || !job.NextAttemptAt.After(time.Now()) {
			t.Fatalf("failed job must retain retry state: %+v", job)
		}
		job.NextAttemptAt = time.Now().Add(-time.Second)
	}
	client.DeleteErr = nil
	if err := svc.ReconcilePendingNotes(ctx); err != nil {
		t.Fatal(err)
	}
	if client.HasFile(*note.ExternalFileID) {
		t.Fatal("retry must remove remote orphan")
	}
	for _, job := range notes.driveOperations {
		if job.Status != "completed" || job.Attempts != 2 {
			t.Fatalf("retry must complete job: %+v", job)
		}
	}
}

func TestReconcileOutboxEmptyUpdateAndMissingDelete(t *testing.T) {
	svc, client, notes, _, _, _, _, _ := newTestService()
	ctx := context.Background()
	owner := uuid.NewString()
	file, err := client.CreateFile(ctx, owner, "", "restore.md", "changed")
	if err != nil {
		t.Fatal(err)
	}
	if err := notes.EnqueueDriveOperation(ctx, "update_file", "", "", file, owner, map[string]any{"content": ""}); err != nil {
		t.Fatal(err)
	}
	for _, operation := range []string{"delete_file", "delete_attachment"} {
		if err := notes.EnqueueDriveOperation(ctx, operation, "", "", "already-missing", owner, nil); err != nil {
			t.Fatal(err)
		}
	}
	if err := svc.ReconcilePendingNotes(ctx); err != nil {
		t.Fatal(err)
	}
	if content, err := client.GetFileContent(ctx, owner, file); err != nil || content != "" {
		t.Fatalf("empty rollback must be applied: %q %v", content, err)
	}
	for _, job := range notes.driveOperations {
		if job.Status != "completed" {
			t.Fatalf("operation must complete: %+v", job)
		}
	}
}

// Crash recovery: un apunte creado justo antes de un crash (fila pending_drive
// sin external_file_id) se recupera si el archivo quedó indexado en Drive con
// appProperties. El reconciliador persiste el fileID y preserva el contenido en
// lugar de descartar la fila.
func TestIdempotencyReconcileRecoversOrphanFileByAppProperties(t *testing.T) {
	svc, client, notes, _, _, _, _, _ := newTestService()
	ctx := context.Background()
	owner := uuid.NewString()
	note, err := notes.Create(ctx, "", owner, nil, "huérfano", nil, "private", nil, "pending_drive")
	if err != nil {
		t.Fatal(err)
	}
	// El alta en Drive sí se completó (con appProperties de la nota) pero el
	// proceso murió antes de persistir el external_file_id.
	orphanFile, err := client.CreateFile(ctx, owner, note.ID, "huérfano.md", "contenido rescatado")
	if err != nil {
		t.Fatal(err)
	}
	old := time.Now().UTC().Add(-20 * time.Minute)
	notes.mu.Lock()
	notes.notes[note.ID].CreatedAt = old
	notes.notes[note.ID].UpdatedAt = old
	notes.mu.Unlock()

	if err := svc.ReconcilePendingNotes(ctx); err != nil {
		t.Fatal(err)
	}
	got, err := notes.GetByID(ctx, note.ID)
	if err != nil || got == nil {
		t.Fatalf("la nota recuperada debe sobrevivir: %+v error=%v", got, err)
	}
	if got.SyncStatus != "synced" || got.ExternalFileID == nil || *got.ExternalFileID != orphanFile {
		t.Fatalf("el reconciliador debe persistir el huérfano recuperado, got %+v", got)
	}
	if !client.HasFile(orphanFile) {
		t.Fatal("el archivo recuperado no debe borrarse")
	}
	if content, err := client.GetFileContent(ctx, owner, orphanFile); err != nil || content != "contenido rescatado" {
		t.Fatalf("el contenido debe preservarse: %q %v", content, err)
	}
}

// Si la búsqueda del huérfano no puede probar su ausencia (p. ej. OAuth
// revocado), la fila se preserva como failed_sync para el siguiente tick en
// lugar de compensarse a ciegas.
func TestIdempotencyReconcilePreservesNoteWhenOrphanLookupFails(t *testing.T) {
	svc, client, notes, _, _, _, _, _ := newTestService()
	ctx := context.Background()
	owner := uuid.NewString()
	note, err := notes.Create(ctx, "", owner, nil, "sin oauth", nil, "private", nil, "pending_drive")
	if err != nil {
		t.Fatal(err)
	}
	old := time.Now().UTC().Add(-20 * time.Minute)
	notes.mu.Lock()
	notes.notes[note.ID].CreatedAt = old
	notes.notes[note.ID].UpdatedAt = old
	notes.mu.Unlock()
	client.Disconnect(owner)
	defer client.Reconnect(owner)

	if err := svc.ReconcilePendingNotes(ctx); err != nil {
		t.Fatal(err)
	}
	got, err := notes.GetByID(ctx, note.ID)
	if err != nil || got == nil {
		t.Fatalf("la nota debe preservarse cuando no se puede verificar Drive: %+v error=%v", got, err)
	}
	if got.SyncStatus != "failed_sync" {
		t.Fatalf("esperaba failed_sync para reintentar, got %q", got.SyncStatus)
	}
}
