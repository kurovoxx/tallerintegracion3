package service

import (
	"context"
	"errors"
	"net"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/kurovoxx/tallerintegracion3/back/notes/internal/drive"
)

type cleanupDrive struct {
	*drive.MockClient
	fail    bool
	missing bool
	calls   int
}

func (d *cleanupDrive) DeleteFile(ctx context.Context, u, id string) error {
	d.calls++
	if d.fail {
		return &drive.DriveError{Code: 403, Message: "test failure"}
	}
	if d.missing {
		return &drive.DriveError{Code: 404}
	}
	return d.MockClient.DeleteFile(ctx, u, id)
}
func (d *cleanupDrive) DeleteAttachment(ctx context.Context, u, id string) error {
	d.calls++
	if d.fail {
		return &drive.DriveError{Code: 403, Message: "test failure"}
	}
	if d.missing {
		return &drive.DriveError{Code: 404}
	}
	return d.MockClient.DeleteAttachment(ctx, u, id)
}
func TestDeleteNoteAllDriveFiles(t *testing.T) {
	for _, count := range []int{0, 1, 3} {
		for _, mode := range []string{"success", "failure", "missing"} {
			t.Run(mode+string(rune('0'+count)), func(t *testing.T) {
				svc, mock, notes, attachments, _, _, _, _ := newTestService()
				ctx := context.Background()
				owner := uuid.NewString()
				d := &cleanupDrive{MockClient: mock}
				svc.drive = d
				note, err := svc.Create(ctx, owner, "test", nil, "private", stringPtr("test"), "")
				if err != nil {
					t.Fatal(err)
				}
				other, err := svc.Create(ctx, owner, "other", nil, "private", stringPtr("keep"), "")
				if err != nil {
					t.Fatal(err)
				}
				keep, err := svc.AddAttachment(ctx, owner, other.ID, "keep.png", "image/png", []byte("image"), true)
				if err != nil {
					t.Fatal(err)
				}
				ids := []string{*note.ExternalFileID}
				for i := 0; i < count; i++ {
					name, mime := "photo.png", "image/png"
					if i%2 == 1 {
						name, mime = "guide.pdf", "application/pdf"
					}
					att, err := svc.AddAttachment(ctx, owner, note.ID, name, mime, []byte("data"), i%2 == 0)
					if err != nil {
						t.Fatal(err)
					}
					ids = append(ids, att.ExternalFileID)
				}
				if err := svc.Delete(ctx, uuid.NewString(), note.ID); err == nil {
					t.Fatal("outsider deleted note")
				}
				if len(notes.driveOperations) != 0 {
					t.Fatal("outsider queued cleanup")
				}
				d.fail = mode == "failure"
				d.missing = mode == "missing"
				if d.missing {
					for _, id := range ids {
						_ = mock.DeleteFile(ctx, owner, id)
					}
				}
				if err := svc.Delete(ctx, owner, note.ID); err != nil {
					t.Fatal(err)
				}
				if d.calls != len(ids) {
					t.Fatalf("cleanup skipped files: %d want %d", d.calls, len(ids))
				}
				if got, _ := notes.GetByID(ctx, note.ID); got != nil {
					t.Fatal("metadata remains")
				}
				if got, _ := attachments.ListByNote(ctx, note.ID); len(got) != 0 {
					t.Fatal("attachment metadata remains")
				}
				if len(notes.driveOperations) != len(ids) {
					t.Fatal("cleanup not durable")
				}
				for _, id := range ids {
					if mock.HasFile(id) != (mode == "failure") {
						t.Fatal("unexpected Drive state")
					}
				}
				// Recovery uses the existing queue even though note and attachment rows are gone.
				d.fail = false
				d.missing = false
				if err := svc.reconcileDriveOperations(ctx); err != nil {
					t.Fatal(err)
				}
				for _, id := range ids {
					if mock.HasFile(id) {
						t.Fatal("orphan after reconciliation")
					}
				}
				for _, job := range notes.driveOperations {
					if job.Status != "completed" {
						t.Fatalf("job status %s", job.Status)
					}
				}
				if !mock.HasFile(keep.ExternalFileID) || !mock.HasFile(*other.ExternalFileID) {
					t.Fatal("other note affected")
				}
				if err := svc.Delete(ctx, owner, note.ID); err == nil {
					t.Fatal("repeat DELETE must be not_found")
				}
			})
		}
	}
}

type failingDriveCleanup struct {
	drive.Client
	err   error
	calls int
}

func (d *failingDriveCleanup) DeleteFile(context.Context, string, string) error {
	d.calls++
	return d.err
}

func (d *failingDriveCleanup) DeleteAttachment(context.Context, string, string) error {
	d.calls++
	return d.err
}

func TestDeleteCleanupToleratesMissingDriveFilesAndNetworkFailures(t *testing.T) {
	for _, attachment := range []bool{false, true} {
		for name, upstreamErr := range map[string]error{
			"not_found": &drive.DriveError{Code: 404, Message: "not found"},
			"network":   &net.OpError{Op: "dial", Net: "tcp", Err: errors.New("connection refused")},
		} {
			resource := "note"
			if attachment {
				resource = "attachment"
			}
			t.Run(resource+"/"+name, func(t *testing.T) {
				ctx := context.Background()
				svc, client, notes, attachments, _, _, _, _ := newTestService()
				owner := uuid.NewString()
				note, err := svc.Create(ctx, owner, "Delete cleanup", nil, "private", stringPtr("text"), "")
				if err != nil {
					t.Fatal(err)
				}
				cleanup := &failingDriveCleanup{Client: client, err: upstreamErr}
				if attachment {
					att, err := svc.AddAttachment(ctx, owner, note.ID, "photo.png", "image/png", []byte("image"), false)
					if err != nil {
						t.Fatal(err)
					}
					svc.drive = cleanup
					if err := svc.RemoveAttachment(ctx, owner, note.ID, att.ID); err != nil {
						t.Fatalf("Drive failure blocked attachment deletion: %v", err)
					}
					if stored, err := attachments.GetByID(ctx, att.ID); err != nil || stored != nil {
						t.Fatalf("attachment metadata remains: %v %v", stored, err)
					}
					if stored, err := notes.GetByID(ctx, note.ID); err != nil || stored == nil {
						t.Fatalf("attachment deletion affected parent: %v %v", stored, err)
					}
				} else {
					svc.drive = cleanup
					if err := svc.Delete(ctx, owner, note.ID); err != nil {
						t.Fatalf("Drive failure blocked note deletion: %v", err)
					}
					if stored, err := notes.GetByID(ctx, note.ID); err != nil || stored != nil {
						t.Fatalf("note metadata remains: %v %v", stored, err)
					}
				}
				if cleanup.calls == 0 {
					t.Fatal("Drive cleanup was not attempted")
				}
				jobs, err := notes.ClaimDriveOperations(ctx, 10, time.Minute)
				if err != nil || len(jobs) != 1 {
					t.Fatalf("cleanup retry must remain durable: %v %v", jobs, err)
				}
			})
		}
	}
}

func TestDeleteAttachmentDurableRecovery(t *testing.T) {
	for _, missing := range []bool{false, true} {
		svc, mock, notes, attachments, _, _, _, _ := newTestService()
		ctx := context.Background()
		owner := uuid.NewString()
		note, err := svc.Create(ctx, owner, "test", nil, "private", stringPtr("test"), "")
		if err != nil {
			t.Fatal(err)
		}
		att, err := svc.AddAttachment(ctx, owner, note.ID, "guide.pdf", "application/pdf", []byte("pdf"), false)
		if err != nil {
			t.Fatal(err)
		}
		other, err := svc.AddAttachment(ctx, owner, note.ID, "keep.png", "image/png", []byte("img"), true)
		if err != nil {
			t.Fatal(err)
		}
		d := &cleanupDrive{MockClient: mock, fail: !missing, missing: missing}
		svc.drive = d
		if missing {
			_ = mock.DeleteAttachment(ctx, owner, att.ExternalFileID)
		}
		if err := svc.RemoveAttachment(ctx, uuid.NewString(), note.ID, att.ID); err == nil {
			t.Fatal("wrong user")
		}
		if err := svc.RemoveAttachment(ctx, owner, note.ID, att.ID); err != nil {
			t.Fatal(err)
		}
		if got, _ := attachments.GetByID(ctx, att.ID); got != nil {
			t.Fatal("metadata remains")
		}
		if len(notes.driveOperations) != 1 {
			t.Fatal("missing durable cleanup")
		}
		d.fail = false
		d.missing = false
		if err := svc.reconcileDriveOperations(ctx); err != nil {
			t.Fatal(err)
		}
		if mock.HasFile(att.ExternalFileID) || !mock.HasFile(other.ExternalFileID) {
			t.Fatal("wrong file removed")
		}
	}
}
