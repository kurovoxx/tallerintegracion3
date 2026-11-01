package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/kurovoxx/tallerintegracion3/back/notes/internal/repository"
)

func TestMemoryDriveCleanupAndRetry(t *testing.T) {
	_, _, notes, attachments, _, _, _, _ := newTestService()
	ctx := context.Background()
	file := "note-file"
	note, err := notes.Create(ctx, "", "owner", nil, "title", &file, "private", nil, "synced")
	if err != nil {
		t.Fatal(err)
	}
	att, err := attachments.Create(ctx, note.ID, "attachment-file", "url", "image/png", nil, nil, false)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := notes.DeleteWithDriveCleanup(ctx, note.ID, "other"); !errors.Is(err, repository.ErrForbidden) {
		t.Fatalf("note ownership: %v", err)
	}
	if _, err := attachments.DeleteAttachmentWithDriveCleanup(ctx, note.ID, att.ID, "other"); !errors.Is(err, repository.ErrForbidden) {
		t.Fatalf("attachment ownership: %v", err)
	}
	if len(notes.driveOperations) != 0 {
		t.Fatal("unauthorized cleanup queued")
	}
	if id, err := attachments.DeleteAttachmentWithDriveCleanup(ctx, note.ID, att.ID, "owner"); err != nil || id != "attachment-file" {
		t.Fatalf("attachment cleanup: %q %v", id, err)
	}
	if id, err := notes.DeleteWithDriveCleanup(ctx, note.ID, "owner"); err != nil || id.FileID != file {
		t.Fatalf("note cleanup: %+v %v", id, err)
	}
	if _, err := notes.DeleteWithDriveCleanup(ctx, note.ID, "owner"); !errors.Is(err, repository.ErrNotFound) {
		t.Fatalf("second cleanup: %v", err)
	}
	jobs, err := notes.ClaimDriveOperations(ctx, 10, time.Minute)
	if err != nil || len(jobs) != 2 {
		t.Fatalf("claim: %d %v", len(jobs), err)
	}
	if claimed, err := notes.ClaimDriveOperations(ctx, 10, time.Minute); err != nil || len(claimed) != 0 {
		t.Fatalf("duplicate claims: %v %v", claimed, err)
	}
	if err := notes.CompleteDriveOperation(ctx, jobs[0].ID); err != nil {
		t.Fatal(err)
	}
	if err := notes.FailDriveOperation(ctx, jobs[1].ID, "retry", time.Now().Add(-time.Second)); err != nil {
		t.Fatal(err)
	}
	retry, err := notes.ClaimDriveOperations(ctx, 10, time.Minute)
	if err != nil || len(retry) != 1 || retry[0].Attempts != 2 || retry[0].ID != jobs[1].ID {
		t.Fatalf("retry: %v %v", retry, err)
	}
	if err := notes.FailDriveOperation(ctx, jobs[0].ID, "late", time.Now()); !errors.Is(err, repository.ErrNotFound) {
		t.Fatalf("completed regression: %v", err)
	}
}

func TestMemoryDriveCleanupCancelled(t *testing.T) {
	notes := NewMemoryNoteStore()
	file := "file"
	note, err := notes.Create(context.Background(), "", "owner", nil, "title", &file, "private", nil, "synced")
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := notes.DeleteWithDriveCleanup(ctx, note.ID, "owner"); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled delete: %v", err)
	}
	if len(notes.notes) != 1 || len(notes.driveOperations) != 0 {
		t.Fatal("cancelled operation mutated state")
	}
}
